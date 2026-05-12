package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

const reframerPromptSource = "bead-lifecycle-reframe"
const reframerDefaultTimeout = 5 * time.Minute

// ReframeResult is the outcome of a runReframer invocation.
type ReframeResult struct {
	Failed         bool
	Reason         string
	NewDescription string
	NewAcceptance  string
	CostUSD        float64
}

type reframerOutput struct {
	Description *string `json:"description"`
	Acceptance  *string `json:"acceptance"`
}

type reframerEventBody struct {
	CostUSD            float64 `json:"cost_usd,omitempty"`
	DescriptionChanged bool    `json:"description_changed"`
	AcceptanceChanged  bool    `json:"acceptance_changed"`
}

// NewReframePostLadderExhaustionHook creates a PostLadderExhaustionHook that
// dispatches runReframer for SpecGap and PersistentExecutionFailed failure
// classes. TooLarge is not handled here; it belongs to the Decompose path.
func NewReframePostLadderExhaustionHook(store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string) PostLadderExhaustionHook {
	return func(ctx context.Context, beadID string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		switch failureClass {
		case SpecGap, PersistentExecutionFailed:
		default:
			return &PostLadderExhaustionResult{Attempted: false}, nil
		}
		result := runReframer(ctx, store, runner, rcfg, projectRoot, beadID)
		if result.Failed {
			return &PostLadderExhaustionResult{
				Attempted: true,
				Succeeded: false,
				Path:      Reframe,
				CostUSD:   result.CostUSD,
			}, nil
		}
		return &PostLadderExhaustionResult{
			Attempted: true,
			Succeeded: true,
			Path:      Reframe,
			CostUSD:   result.CostUSD,
		}, nil
	}
}

// runReframer dispatches a smart+ tier agent to rewrite the bead's description
// and acceptance criteria after the escalation ladder has been exhausted.
// On success, the bead description/AC are updated, the
// consecutive_ladder_exhaustions counter is cleared, and a "reframe-applied"
// event is emitted. On failure a ReframeResult with Failed=true is returned.
func runReframer(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, beadID string) ReframeResult {
	tctx, cancel := context.WithTimeout(ctx, reframerDefaultTimeout)
	defer cancel()

	b, err := store.Get(ctx, beadID)
	if err != nil {
		return ReframeResult{Failed: true, Reason: "store_error"}
	}

	prompt, err := buildReframerPrompt(store, b)
	if err != nil {
		return ReframeResult{Failed: true, Reason: "prompt_error"}
	}

	runtime := AgentRunRuntime{
		Prompt:           prompt,
		WorkDir:          projectRoot,
		PromptSource:     reframerPromptSource,
		ProfileOverride:  selectProfileForDispatch(tctx, projectRoot, nil, runner, SelectStrongestProfile),
		ClearRoutingPins: true,
		ClearProfile:     true,
		ClearMinPower:    true,
		ClearMaxPower:    true,
	}

	result, err := dispatchViaResolvedConfig(tctx, projectRoot, nil, runner, rcfg, runtime)
	if err != nil {
		if tctx.Err() != nil {
			return ReframeResult{Failed: true, Reason: "timeout"}
		}
		return ReframeResult{Failed: true, Reason: "dispatch_error"}
	}

	output := strings.TrimSpace(result.CondensedOutput)
	if output == "" {
		output = strings.TrimSpace(result.Output)
	}
	if output == "" {
		return ReframeResult{Failed: true, Reason: "empty_output", CostUSD: result.CostUSD}
	}

	candidate, ok := extractJSONCandidate(output)
	if !ok {
		return ReframeResult{Failed: true, Reason: "invalid_output", CostUSD: result.CostUSD}
	}

	var parsed reframerOutput
	if err := json.Unmarshal([]byte(candidate), &parsed); err != nil {
		return ReframeResult{Failed: true, Reason: "invalid_output", CostUSD: result.CostUSD}
	}
	if parsed.Description == nil && parsed.Acceptance == nil {
		return ReframeResult{Failed: true, Reason: "invalid_output", CostUSD: result.CostUSD}
	}

	current, err := store.Get(ctx, beadID)
	if err != nil {
		return ReframeResult{Failed: true, Reason: "store_error"}
	}

	descChanged := parsed.Description != nil &&
		strings.TrimSpace(*parsed.Description) != strings.TrimSpace(current.Description)
	accChanged := parsed.Acceptance != nil &&
		strings.TrimSpace(*parsed.Acceptance) != strings.TrimSpace(current.Acceptance)

	if !descChanged && !accChanged {
		return ReframeResult{Failed: true, Reason: "noop_edits", CostUSD: result.CostUSD}
	}

	var changedFields []string
	var rewrite PreClaimIntakeRewrite
	if descChanged {
		changedFields = append(changedFields, "description")
		rewrite.Description = strings.TrimSpace(*parsed.Description)
	}
	if accChanged {
		changedFields = append(changedFields, "acceptance")
		rewrite.Acceptance = strings.TrimSpace(*parsed.Acceptance)
	}
	rewrite.ChangedFields = changedFields

	intake := PreClaimIntakeResult{
		Outcome: PreClaimIntakeActionableButRewritten,
		Detail:  "reframer rewrote bead after exhaustion",
		Rewrite: rewrite,
	}

	if err := applyPreClaimIntakeRewrite(store, beadID, "ddx work", intake, time.Now()); err != nil {
		return ReframeResult{Failed: true, Reason: "apply_error"}
	}

	_ = store.Update(ctx, beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		delete(b.Extra, consecutiveLadderExhaustionsKey)
	})

	body, _ := json.Marshal(reframerEventBody{
		CostUSD:            result.CostUSD,
		DescriptionChanged: descChanged,
		AcceptanceChanged:  accChanged,
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "reframe-applied",
		Summary:   strings.Join(changedFields, ","),
		Body:      string(body),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})

	out := ReframeResult{
		Failed:  false,
		CostUSD: result.CostUSD,
	}
	if descChanged {
		out.NewDescription = rewrite.Description
	}
	if accChanged {
		out.NewAcceptance = rewrite.Acceptance
	}
	return out
}

func buildReframerPrompt(store ExecuteBeadLoopStore, b *bead.Bead) (string, error) {
	type failureEntry struct {
		Summary string `json:"summary"`
		Body    string `json:"body,omitempty"`
	}
	type envelope struct {
		Title          string         `json:"title"`
		Description    string         `json:"description"`
		Acceptance     string         `json:"acceptance"`
		FailureHistory []failureEntry `json:"failure_history"`
	}

	env := envelope{
		Title:       strings.TrimSpace(b.Title),
		Description: strings.TrimSpace(b.Description),
		Acceptance:  strings.TrimSpace(b.Acceptance),
	}

	if events, err := store.Events(b.ID); err == nil {
		for i := len(events) - 1; i >= 0 && len(env.FailureHistory) < 5; i-- {
			ev := events[i]
			switch ev.Kind {
			case "bead.result", "execute-bead", "per-bead-budget-exhausted":
			default:
				continue
			}
			env.FailureHistory = append(env.FailureHistory, failureEntry{
				Summary: strings.TrimSpace(ev.Summary),
				Body:    strings.TrimSpace(ev.Body),
			})
		}
	}

	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", fmt.Errorf("reframer: encode prompt: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("MODE: reframe\n")
	sb.WriteString("You are rewriting a bead's description and acceptance criteria after repeated execution failures.\n")
	sb.WriteString("Your goal is to produce a clearer, more specific, implementation-ready bead.\n")
	sb.WriteString("Preservation rules: governing artifact references (FEAT-NNN, ADR-NNN), named test functions (TestFoo), file:line evidence, dependency IDs (ddx-XXXXXXXX), and NON-SCOPE bullets must appear in any rewrite.\n")
	sb.WriteString("Return exactly one JSON object: {\"description\": string|null, \"acceptance\": string|null}\n")
	sb.WriteString("Use null to leave a field unchanged. At least one field must be non-null and meaningfully changed.\n")
	sb.WriteString("Do not include prose or markdown; return only the JSON object.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}
