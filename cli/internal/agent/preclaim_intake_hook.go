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
	Confidence     float64                     `json:"confidence,omitempty"`
	Reasoning      string                      `json:"reasoning,omitempty"`
	Rewrite        preClaimIntakePromptRewrite `json:"rewrite,omitempty"`
}

type preClaimIntakePromptRewrite struct {
	Description   string   `json:"description,omitempty"`
	Acceptance    string   `json:"acceptance,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

// NewPreClaimIntakeHook constructs the bead-intake complexity gate used
// before claim in the work / execute-loop paths. The hook evaluates the bead
// using the repository's triage prompt and returns one of the typed intake
// outcomes so the loop can decide whether to claim or skip the candidate.
//
// The hook preserves operator-supplied passthrough constraints. When the
// resolved route cannot satisfy the strong splitter floor, it returns an
// actionable-but-blocking result with agent_power_unsatisfied in the detail so
// the loop can skip the bead instead of attempting weak decomposition.
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
		if !hasBeadLifecycleSkill(projectRoot) {
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: skill missing: bead-lifecycle")
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

		result, err := dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, AgentRunRuntime{
			Prompt:           prompt,
			WorkDir:          projectRoot,
			PromptSource:     PreClaimIntakePromptSource,
			MinPowerOverride: strongMinPower,
		})
		if err != nil {
			if isStrongPowerUnsatisfiedError(err) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeAmbiguousNeedsHuman,
					Detail:  "agent_power_unsatisfied: " + err.Error(),
				}, nil
			}
			return PreClaimIntakeResult{}, fmt.Errorf("pre-claim intake: dispatch: %w", err)
		}

		payload, err := intakeResultPayload(result)
		if err != nil {
			return PreClaimIntakeResult{}, err
		}

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
	sb.WriteString("Use rewritten only when you can preserve the bead's intent and return safe description/acceptance updates without inventing behavior, choosing product semantics, widening scope, or dropping acceptance criteria.\n")
	sb.WriteString("If the bead cannot be safely refined, classify it as ambiguous.\n")
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

const defaultStrongSplitterMinPower = 90

func resolveStrongSplitterMinPower(ctx context.Context, projectRoot string, svc agentlib.FizeauService) int {
	if svc == nil {
		return defaultStrongSplitterMinPower
	}

	models, err := svc.ListModels(ctx, agentlib.ModelFilter{})
	if err != nil {
		return defaultStrongSplitterMinPower
	}
	maxPower := 0
	for _, m := range models {
		if m.Power > maxPower {
			maxPower = m.Power
		}
	}
	if maxPower <= 0 {
		return defaultStrongSplitterMinPower
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
