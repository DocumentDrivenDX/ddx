package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
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

type preClaimReadinessClassificationPromptResult struct {
	Classification    string                            `json:"classification"`
	Tractability      string                            `json:"tractability,omitempty"`
	Score             int                               `json:"score,omitempty"`
	Rationale         string                            `json:"rationale,omitempty"`
	Detail            string                            `json:"detail,omitempty"`
	Reasoning         string                            `json:"reasoning,omitempty"`
	ReadinessChecks   preClaimReadinessChecksPayload    `json:"readiness_checks,omitempty"`
	SuggestedFixes    []preClaimReadinessSuggestedFix   `json:"suggested_fixes,omitempty"`
	SuggestedChildren []preClaimReadinessSuggestedChild `json:"suggested_child_beads,omitempty"`
	WaiversApplied    []preClaimReadinessWaiver         `json:"waivers_applied,omitempty"`
	Rewrite           preClaimIntakePromptRewrite       `json:"rewrite,omitempty"`
}

type preClaimReadinessCheck struct {
	Reason                 string `json:"reason,omitempty"`
	Verdict                string `json:"verdict,omitempty"`
	Evidence               string `json:"evidence,omitempty"`
	CheckableBeforeAttempt bool   `json:"checkable_before_attempt,omitempty"`
}

type preClaimReadinessSuggestedFix struct {
	Target string `json:"target,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type preClaimReadinessSuggestedChild struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  []string `json:"acceptance,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Deps        []string `json:"deps,omitempty"`
}

type preClaimReadinessWaiver struct {
	Reason   string   `json:"reason,omitempty"`
	Criteria []string `json:"criteria,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
}

type preClaimReadinessChecksPayload struct {
	Checks    []preClaimReadinessCheck
	Present   bool
	Malformed string
	Evidence  string
}

func (p *preClaimReadinessChecksPayload) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	p.Present = true
	switch trimmed[0] {
	case '[':
		return json.Unmarshal(data, &p.Checks)
	case '{':
		var check preClaimReadinessCheck
		if err := json.Unmarshal(data, &check); err != nil {
			return err
		}
		p.Checks = []preClaimReadinessCheck{check}
		return nil
	case '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		p.Malformed = "readiness_checks must be an object or array"
		p.Evidence = value
		return nil
	default:
		p.Malformed = "readiness_checks must be an object or array"
		p.Evidence = trimmed
		return nil
	}
}

func (p preClaimReadinessChecksPayload) Len() int {
	return len(p.Checks)
}

// NewPreClaimIntakeHook constructs the bead-intake complexity gate used
// before claim in the work / execute-loop paths. The hook evaluates the bead
// using the repository's triage prompt and returns one of the typed intake
// outcomes so the loop can decide whether to claim or skip the candidate.
//
// The hook uses the normal service execution path. Unpinned workers get the
// lifecycle hook's strongest-profile hint; explicitly pinned workers keep their
// operator route pins. Route failures are infrastructure failures, not
// bead-readiness decisions, so they return intake_error and let the loop use
// its fail-open readiness path.
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

		runtime := AgentRunRuntime{
			Prompt:        prompt,
			WorkDir:       projectRoot,
			PromptSource:  PreClaimIntakePromptSource,
			ClearProfile:  true,
			ClearMinPower: true,
			ClearMaxPower: true,
		}
		applyLifecycleHookRouting(ctx, projectRoot, svc, runner, rcfg, &runtime, SelectStrongestProfile)
		payload, err := dispatchPreClaimIntakePayload(ctx, projectRoot, svc, runner, rcfg, runtime)
		if err != nil {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeError,
				Detail:  preClaimIntakeRouteUnavailableDetail(err),
			}, nil
		}

		return decodePreClaimIntakePayloadResultWithMode(payload, rcfg.BeadQualityMode())
	}
}

func applyLifecycleHookRouting(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime *AgentRunRuntime, selector func(ProfileSnapshot) string) {
	runtime.ClearRoutingPins = true

	pinned := false
	if harness, ok := rcfg.ExplicitHarness(); ok {
		runtime.HarnessOverride = strings.TrimSpace(harness)
		pinned = true
	}
	if provider, ok := rcfg.ExplicitProvider(); ok {
		runtime.ProviderOverride = strings.TrimSpace(provider)
		pinned = true
	}
	if model, ok := rcfg.ExplicitModel(); ok {
		runtime.ModelOverride = strings.TrimSpace(model)
		pinned = true
	}
	if pinned {
		return
	}
	runtime.ProfileOverride = selectProfileForDispatch(ctx, projectRoot, svc, runner, selector)
}

func dispatchPreClaimIntakePayload(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (string, error) {
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
	sb.WriteString("Do not rewrite bead fields in intake mode. If the bead is executable as written, classify it as ready even when the prose could be cleaner.\n")
	sb.WriteString("Return exactly one JSON object matching the readiness schema with classification, tractability, score, rationale, readiness_checks, suggested_fixes, rewrite, suggested_child_beads, and waivers_applied.\n")
	sb.WriteString("readiness_checks MUST be a JSON array; it may be empty, and every entry MUST be an object with reason, verdict, evidence, and checkable_before_attempt. It must not be an object or string.\n")
	sb.WriteString("If the bead is not executable as written but can be made executable by a narrow, semantics-preserving metadata/AC rewrite, emit rewrite with changed_fields, description, and acceptance.\n")
	sb.WriteString("Put prompt-quality improvements in suggested_fixes only; keep operator_required for actual blockers.\n")
	sb.WriteString("Preservation rules: non-scope items, governing artifact references (FEAT-NNN, ADR-NNN), named test functions (TestFoo), file:line evidence, and dependency IDs (ddx-XXXXXXXX) must all appear in the replacement description.\n")
	sb.WriteString("Classify as operator_required only when ambiguity, missing prerequisites, hidden external blockers, or unsafe scope choices prevent an implementation attempt.\n")
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

// decodePreClaimIntakePayloadResult decodes a JSON payload into a PreClaimIntakeResult.
// It accepts both the legacy intake schema (classification field) and the canonical
// readiness schema (outcome field), converting both into the same decision model.
func decodePreClaimIntakePayloadResult(payload string) (PreClaimIntakeResult, error) {
	return decodePreClaimIntakePayloadResultWithMode(payload, config.BeadQualityModeWarnOnly)
}

func decodePreClaimIntakePayloadResultWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
	var probe struct {
		Classification  string                         `json:"classification"`
		Outcome         string                         `json:"outcome"`
		Tractability    string                         `json:"tractability"`
		Rationale       string                         `json:"rationale"`
		Score           *int                           `json:"score"`
		ReadinessChecks preClaimReadinessChecksPayload `json:"readiness_checks"`
		SuggestedFixes  []struct {
			Target string `json:"target,omitempty"`
		} `json:"suggested_fixes"`
	}
	if err := json.Unmarshal([]byte(payload), &probe); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	if probe.Outcome != "" {
		return decodeCanonicalReadinessPayloadWithMode(payload, qualityMode)
	}
	if probe.Classification != "" {
		if isReadinessClassificationPayload(probe.Classification, probe.Tractability, probe.Rationale, probe.Score, probe.ReadinessChecks.Present, probe.ReadinessChecks.Len(), len(probe.SuggestedFixes)) {
			return decodeReadinessClassificationPayloadWithMode(payload, qualityMode)
		}
		return decodeLegacyIntakePayload(payload)
	}
	return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification or outcome field")
}

func isReadinessClassificationPayload(classification, tractability, rationale string, score *int, readinessChecksPresent bool, checkCount, fixCount int) bool {
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case ReadinessClassificationSystemUnready,
		"readiness_error",
		"intake_error",
		ReadinessClassificationNeedsRefine,
		ReadinessClassificationNeedsSplit,
		ReadinessClassificationOperatorRequired:
		return true
	case "ready":
		return readinessChecksPresent ||
			strings.TrimSpace(tractability) != "" ||
			strings.TrimSpace(rationale) != "" ||
			score != nil ||
			checkCount > 0 ||
			fixCount > 0
	default:
		return false
	}
}

func decodeCanonicalReadinessPayloadWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
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
		return PreClaimIntakeResult{Outcome: PreClaimIntakeTooLargeDecomposed, Reason: ReadinessReasonTooLarge, Detail: detail}, nil
	case "operator_required", "human_review_required":
		return PreClaimIntakeResult{Outcome: PreClaimIntakeOperatorRequired, Reason: ReadinessReasonAmbiguousScope, Detail: detail}, nil
	case "ambiguous_needs_human", "needs_human":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: legacy readiness outcome %q removed by lifecycle migration; use %q", out.Outcome, PreClaimIntakeOperatorRequired)
	case "readiness_error", "system_unready":
		// system_unready maps to fail-open/skip per ADR-023/FEAT-010 policy
		classified := ClassifyReadinessWithMode(ReadinessClassificationSystemUnready, nil, detail, qualityMode)
		return PreClaimIntakeResult{
			Outcome:      PreClaimIntakeError,
			Reason:       classified.Reason,
			SystemReason: classified.SystemReason,
			Detail:       detail,
		}, nil
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing outcome")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: unknown readiness outcome %q: expected one of actionable_atomic, actionable_but_rewritten, too_large_decomposed, operator_required, readiness_error, system_unready", out.Outcome)
	}
}

func decodeReadinessClassificationPayloadWithMode(payload string, qualityMode string) (PreClaimIntakeResult, error) {
	var out preClaimReadinessClassificationPromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode readiness classification result: %w", err)
	}
	if out.ReadinessChecks.Malformed != "" {
		detail := malformedReadinessChecksDetail(out.ReadinessChecks)
		classified := ClassifyReadinessWithMode(ReadinessClassificationSystemUnready, nil, detail, qualityMode)
		return PreClaimIntakeResult{
			Outcome:      classified.IntakeOutcome,
			Reason:       classified.Reason,
			SystemReason: classified.SystemReason,
			Detail:       detail,
		}, nil
	}
	reasons := failedReadinessReasons(out.ReadinessChecks.Checks)
	detail := firstNonEmptyReadinessDetail(out.Rationale, out.Detail, out.Reasoning)
	classified := ClassifyReadinessWithMode(out.Classification, reasons, detail, qualityMode)
	if detail == "" {
		detail = readinessCheckEvidence(out.ReadinessChecks.Checks)
	}
	detail = readinessDetailWithReason(classified, detail)

	rewrite := PreClaimIntakeRewrite{
		Description:   strings.TrimSpace(out.Rewrite.Description),
		Acceptance:    strings.TrimSpace(out.Rewrite.Acceptance),
		ChangedFields: normalizePreClaimIntakeRewriteFields(out.Rewrite.ChangedFields),
	}
	result := PreClaimIntakeResult{
		Outcome:      classified.IntakeOutcome,
		Reason:       classified.Reason,
		SystemReason: classified.SystemReason,
		Detail:       detail,
	}
	if classified.Classification == ReadinessClassificationNeedsRefine && hasPreClaimIntakeRewrite(rewrite) {
		result.Outcome = PreClaimIntakeActionableButRewritten
		result.Rewrite = rewrite
	}
	return result, nil
}

func failedReadinessReasons(checks []preClaimReadinessCheck) []string {
	reasons := make([]string, 0, len(checks))
	for _, check := range checks {
		verdict := strings.ToLower(strings.TrimSpace(check.Verdict))
		if verdict != "fail" {
			continue
		}
		if reason := strings.TrimSpace(check.Reason); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	return reasons
}

func malformedReadinessChecksDetail(checks preClaimReadinessChecksPayload) string {
	detail := strings.TrimSpace(checks.Malformed)
	if detail == "" {
		detail = "readiness_checks must be an object or array"
	}
	if evidence := strings.TrimSpace(checks.Evidence); evidence != "" {
		detail += ": " + strconv.Quote(evidence)
	}
	return detail
}

func readinessCheckEvidence(checks []preClaimReadinessCheck) string {
	for _, check := range checks {
		if strings.ToLower(strings.TrimSpace(check.Verdict)) != "fail" {
			continue
		}
		if evidence := strings.TrimSpace(check.Evidence); evidence != "" {
			return evidence
		}
	}
	return ""
}

func readinessDetailWithReason(classified ReadinessClassificationResult, detail string) string {
	detail = strings.TrimSpace(detail)
	reason := strings.TrimSpace(classified.Reason)
	if reason == "" {
		return detail
	}
	if detail == "" {
		return reason
	}
	if strings.Contains(strings.ToLower(detail), strings.ToLower(reason)) {
		return detail
	}
	return reason + ": " + detail
}

func firstNonEmptyReadinessDetail(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func hasPreClaimIntakeRewrite(rewrite PreClaimIntakeRewrite) bool {
	return strings.TrimSpace(rewrite.Description) != "" ||
		strings.TrimSpace(rewrite.Acceptance) != "" ||
		len(rewrite.ChangedFields) > 0
}

func decodeLegacyIntakePayload(payload string) (PreClaimIntakeResult, error) {
	var out preClaimIntakePromptResult
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: decode result: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(out.Classification)) {
	case "atomic", "ok", "ready", "actionable", "pass":
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
	case "ambiguous", "ambiguous_scope", "needs_human", "ambiguous_needs_human":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: legacy classification %q removed by lifecycle migration; use outcome %q", out.Classification, PreClaimIntakeOperatorRequired)
	case "":
		return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: missing classification")
	default:
		return PreClaimIntakeResult{}, fmt.Errorf(
			"pre-claim intake: unknown classification %q: expected one of ready, needs_refine, needs_split, operator_required, system_unready",
			out.Classification,
		)
	}
}
