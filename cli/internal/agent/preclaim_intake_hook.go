package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// PreClaimIntakePromptSource identifies the pre-claim complexity-eval
// dispatch in session logs and tests.
const PreClaimIntakePromptSource = "bead-lifecycle-intake"

type preClaimIntakePromptEnvelope struct {
	Title         string                  `json:"title"`
	Description   string                  `json:"description"`
	Acceptance    string                  `json:"acceptance"`
	Labels        []string                `json:"labels"`
	PriorAttempts []preClaimIntakeAttempt `json:"prior_attempts"`
	Depth         int                     `json:"depth"`
	Parent        string                  `json:"parent,omitempty"`
	Dependencies  []string                `json:"dependencies,omitempty"`
}

type preClaimIntakeAttempt struct {
	Status    string `json:"status"`
	Rationale string `json:"rationale,omitempty"`
}

type preClaimIntakePromptResult struct {
	Classification string                      `json:"classification"`
	Confidence     any                         `json:"confidence,omitempty"`
	Reasoning      string                      `json:"reasoning,omitempty"`
	Rewrite        preClaimIntakePromptRewrite `json:"rewrite,omitempty"`
}

type preClaimIntakePromptRewrite struct {
	Description   string   `json:"description,omitempty"`
	Acceptance    string   `json:"acceptance,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// preClaimReadinessPromptResult is the canonical readiness JSON schema returned
// by skills that use the FEAT-010/ADR-023 outcome vocabulary.
type preClaimReadinessPromptResult struct {
	Outcome string                      `json:"outcome"`
	Reason  string                      `json:"reason,omitempty"`
	Detail  string                      `json:"detail,omitempty"`
	Rewrite preClaimIntakePromptRewrite `json:"rewrite,omitempty"`
}

// NewPreClaimIntakeHook constructs the bead-intake complexity gate used
// before claim in the work / execute-loop paths. The hook evaluates the bead
// using the repository's triage prompt and returns one of the typed intake
// outcomes so the loop can decide whether to claim or skip the candidate.
//
// The hook preserves operator-supplied passthrough constraints. Route/power
// failures are infrastructure failures, not bead-readiness decisions, so they
// return intake_error and let the loop use its fail-open readiness path.
func NewPreClaimIntakeHook(projectRoot string, store BeadReader, rcfg config.ResolvedConfig, svc agentlib.FizeauService, runner AgentRunner) func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
	return func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return PreClaimIntakeResult{}, err
			}
		}
		if strings.TrimSpace(projectRoot) == "" {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: project root required")
		}
		if err := ensureBeadLifecycleSkill(projectRoot); err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: %w", err)
		}
		if store == nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: bead reader required")
		}

		b, err := store.Get(beadID)
		if err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: load bead %s: %w", beadID, err)
		}

		prompt, err := buildPreClaimIntakePrompt(projectRoot, store, b)
		if err != nil {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: build prompt: %w", err)
		}

		strongMinPower := resolveStrongSplitterMinPower(ctx, projectRoot, svc)
		if rcfg.MinPower() > strongMinPower {
			strongMinPower = rcfg.MinPower()
		}

		runtime := AgentRunRuntime{
			Prompt:           prompt,
			WorkDir:          projectRoot,
			PromptSource:     PreClaimIntakePromptSource,
			ModelRefOverride: "smart",
			ClearProfile:     true,
			ClearMaxPower:    true,
		}
		if strongMinPower > 0 {
			runtime.MinPowerOverride = strongMinPower
		}
		payload, err := dispatchPreClaimIntakePayload(ctx, projectRoot, svc, runner, rcfg, runtime, strongMinPower)
		if err != nil {
			if isStrongPowerUnsatisfiedError(err) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeError,
					Detail:  preClaimIntakeRouteUnavailableDetail(err),
				}, nil
			}
			return PreClaimIntakeResult{}, err
		}

		return decodePreClaimIntakePayloadResult(payload)
	}
}

func dispatchPreClaimIntakePayload(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime, strongMinPower int) (string, error) {
	payload, err := dispatchPreClaimIntakePayloadOnce(ctx, projectRoot, svc, runner, rcfg, runtime)
	if err == nil {
		return payload, nil
	}
	if runtime.ModelRefOverride != "smart" || strongMinPower > 0 || !isSmartRouteUnavailableError(err) {
		return "", err
	}

	runtime.ModelRefOverride = ""
	return dispatchPreClaimIntakePayloadOnce(ctx, projectRoot, svc, runner, rcfg, runtime)
}

func preClaimIntakeRouteUnavailableDetail(err error) string {
	detail := ""
	if err != nil {
		detail = strings.TrimSpace(err.Error())
	}
	detail = trimDiagnosticPrefix(detail, "pre-claim intake")
	detail = strings.TrimPrefix(strings.TrimSpace(detail), "dispatch: ")
	if detail == "" {
		return "readiness route unavailable"
	}
	return "readiness route unavailable: " + detail
}

func dispatchPreClaimIntakePayloadOnce(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (string, error) {
	result, err := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, runtime)
	if err != nil {
		return "", fmt.Errorf("pre-claim intake: dispatch: %w", err)
	}
	return intakeResultPayload(result)
}

func buildPreClaimIntakePrompt(projectRoot string, store BeadReader, b *bead.Bead) (string, error) {
	env := preClaimIntakePromptEnvelope{
		Title:         strings.TrimSpace(b.Title),
		Description:   strings.TrimSpace(b.Description),
		Acceptance:    strings.TrimSpace(b.Acceptance),
		Labels:        append([]string(nil), b.Labels...),
		PriorAttempts: []preClaimIntakeAttempt{},
		Depth:         beadDecompositionDepth(projectRoot, b),
		Parent:        strings.TrimSpace(b.Parent),
		Dependencies:  append([]string(nil), b.DepIDs()...),
	}

	// Prior attempts are optional for the gate. If bead history is available,
	// include the last few attempt/result summaries to help the classifier
	// distinguish a first-pass atomic bead from repeated decomposition.
	if store != nil {
		if eventStore, ok := any(store).(interface {
			Events(id string) ([]bead.BeadEvent, error)
		}); ok {
			if events, err := eventStore.Events(b.ID); err == nil && len(events) > 0 {
				for i := len(events) - 1; i >= 0 && len(env.PriorAttempts) < 5; i-- {
					ev := events[i]
					if ev.Kind != "bead.result" && ev.Kind != "execute-bead" {
						continue
					}
					env.PriorAttempts = append(env.PriorAttempts, preClaimIntakeAttempt{
						Status:    strings.TrimSpace(ev.Summary),
						Rationale: strings.TrimSpace(ev.Body),
					})
				}
			}
		}
	}

	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("MODE: intake\n")
	sb.WriteString("You are evaluating whether this bead is atomic, decomposable, ambiguous, or safely refinable before claim.\n")
	sb.WriteString("Use rewritten to improve prompt fitness: compress stale, duplicated, or noisy description prose, or expand a vague bead with durable context grounded in the repository or governing artifacts.\n")
	sb.WriteString("Validated replacement is preferred over append-only amendment when it makes the bead a better implementation prompt.\n")
	sb.WriteString("Preservation rules: non-scope items, governing artifact references (FEAT-NNN, ADR-NNN), named test functions (TestFoo), file:line evidence, and dependency IDs (ddx-XXXXXXXX) must all appear in the replacement description.\n")
	sb.WriteString("When preservation cannot be proven from durable anchors, or when rewriting would require inventing acceptance criteria, changing scope, or choosing between conflicting requirements, classify as ambiguous_needs_human.\n")
	sb.WriteString("Return exactly one JSON object matching the intake schema with classification, confidence, reasoning, and optional rewrite fields.\n")
	sb.WriteString("When classification is rewritten, include rewrite.changed_fields, rewrite.description, and rewrite.acceptance.\n")
	sb.WriteString("Do not include prose or markdown.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func intakeResultPayload(result *Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("pre-claim intake: empty runner result")
	}
	text := strings.TrimSpace(result.CondensedOutput)
	if text == "" {
		text = strings.TrimSpace(result.Output)
	}
	if text == "" {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("pre-claim intake: runner error: %s", strings.TrimSpace(result.Error))
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("pre-claim intake: runner exited with code %d and empty output", result.ExitCode)
		}
		return "", fmt.Errorf("pre-claim intake: empty output")
	}
	candidate, ok := extractJSONCandidate(text)
	if !ok {
		return "", fmt.Errorf("pre-claim intake: no JSON object found")
	}
	return candidate, nil
}

func normalizePreClaimIntakeRewriteFields(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func resolveStrongSplitterMinPower(ctx context.Context, projectRoot string, svc agentlib.FizeauService) int {
	if svc == nil {
		return 0
	}

	models, err := svc.ListModels(ctx, agentlib.ModelFilter{})
	if err != nil {
		return 0
	}
	maxPower := 0
	for _, m := range models {
		if m.Power > maxPower {
			maxPower = m.Power
		}
	}
	return maxPower
}

func isStrongPowerUnsatisfiedError(err error) bool {
	if err == nil {
		return false
	}
	switch ClassifyFailureMode("task_failed", 1, err.Error()) {
	case FailureModeAgentPowerUnsatisfied, FailureModeBlockedByPassthroughConstraint:
		return true
	default:
		return false
	}
}

func isSmartRouteUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tier ≥ smart") ||
		strings.Contains(msg, "tier >= smart") ||
		strings.Contains(msg, "profile=smart") ||
		strings.Contains(msg, "profile smart") ||
		strings.Contains(msg, "model_ref=smart") ||
		strings.Contains(msg, "model-ref smart") ||
		strings.Contains(msg, `model ref "smart"`)
}

// decodePreClaimIntakePayloadResult decodes a JSON payload into a PreClaimIntakeResult.
// It accepts both the legacy intake schema (classification field) and the canonical
// readiness schema (outcome field), converting both into the same decision model.
func decodePreClaimIntakePayloadResult(payload string) (PreClaimIntakeResult, error) {
	var probe struct {
		Classification string `json:"classification"`
		Outcome        string `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(payload), &probe); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	if probe.Outcome != "" {
		return decodeCanonicalReadinessPayload(payload)
	}
	if probe.Classification != "" {
		return decodeLegacyIntakePayload(payload)
	}
	return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification or outcome field")
}

func decodeCanonicalReadinessPayload(payload string) (PreClaimIntakeResult, error) {
	var out preClaimReadinessPromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode readiness result: %w", err)
	}
	detail := strings.TrimSpace(out.Reason)
	if detail == "" {
		detail = strings.TrimSpace(out.Detail)
	}
	rewrite := PreClaimIntakeRewrite{
		Description:   strings.TrimSpace(out.Rewrite.Description),
		Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
		ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
	}
	switch strings.ToLower(strings.TrimSpace(out.Outcome)) {
	case "actionable_atomic":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic, Detail: detail}, nil
	case "actionable_but_rewritten":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableButRewritten, Detail: detail, Rewrite: rewrite}, nil
	case "too_large_decomposed":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed, Detail: detail}, nil
	case "ambiguous_needs_human":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeAmbiguousNeedsHuman, Detail: detail}, nil
	case "readiness_error", "system_unready":
		// system_unready maps to fail-open/skip per ADR-023/FEAT-010 policy
		return PreClaimIntakeResult{Outcome: PreClaimIntakeError, Detail: detail}, nil
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing outcome")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: unknown readiness outcome %q: expected one of actionable_atomic, actionable_but_rewritten, too_large_decomposed, ambiguous_needs_human, readiness_error, system_unready", out.Outcome)
	}
}

func decodeLegacyIntakePayload(payload string) (PreClaimIntakeResult, error) {
	var out preClaimIntakePromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(out.Classification)) {
	case "atomic":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic, Detail: strings.TrimSpace(out.Reasoning)}, nil
	case "rewritten":
		return PreClaimIntakeResult{
			Outcome: PreClaimIntakeActionableButRewritten,
			Detail:  strings.TrimSpace(out.Reasoning),
			Rewrite: PreClaimIntakeRewrite{
				Description:   strings.TrimSpace(out.Rewrite.Description),
				Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
				ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
			},
		}, nil
	case "decomposable":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed, Detail: strings.TrimSpace(out.Reasoning)}, nil
	case "ambiguous":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeAmbiguousNeedsHuman, Detail: strings.TrimSpace(out.Reasoning)}, nil
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: unknown classification %q", out.Classification)
	}
}
