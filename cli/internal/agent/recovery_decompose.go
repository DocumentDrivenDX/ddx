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

const decomposerPromptSource = "bead-lifecycle-decompose"
const preClaimDecomposerPromptSource = "bead-lifecycle-preclaim-decompose"
const decomposerDefaultTimeout = 8 * time.Minute

// DecomposeResult is the outcome of a runDecomposer invocation.
type DecomposeResult struct {
	Failed   bool
	Reason   string
	ChildIDs []string
	CostUSD  float64
}

type decomposerChild struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Acceptance  string   `json:"acceptance"`
	Labels      []string `json:"labels,omitempty"`
}

type decomposerEventBody struct {
	ChildIDs []string `json:"child_ids"`
	CostUSD  float64  `json:"cost_usd,omitempty"`
}

// NewPreClaimDecompositionHook dispatches an orchestrator splitter for beads
// that readiness classifies as too_large without returning concrete children.
// It returns a lossless split proposal; the execute loop owns validation,
// child creation, dependency wiring, and fallback behavior.
func NewPreClaimDecompositionHook(store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string) func(context.Context, string) (*PreClaimDecomposition, error) {
	return func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
		return runPreClaimDecomposer(ctx, store, runner, rcfg, projectRoot, beadID)
	}
}

func runPreClaimDecomposer(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, beadID string) (*PreClaimDecomposition, error) {
	tctx, cancel := context.WithTimeout(ctx, decomposerDefaultTimeout)
	defer cancel()

	b, err := store.Get(beadID)
	if err != nil {
		return nil, fmt.Errorf("decomposer: load bead: %w", err)
	}
	prompt, err := buildPreClaimDecomposerPrompt(store, b)
	if err != nil {
		return nil, err
	}
	runtime := AgentRunRuntime{
		Prompt:           prompt,
		WorkDir:          projectRoot,
		PromptSource:     preClaimDecomposerPromptSource,
		ProfileOverride:  selectProfileForDispatch(tctx, projectRoot, nil, runner, SelectStrongestProfile),
		ClearRoutingPins: true,
		ClearProfile:     true,
		ClearMinPower:    true,
		ClearMaxPower:    true,
	}
	result, err := dispatchViaResolvedConfig(tctx, projectRoot, nil, runner, rcfg, runtime)
	if err != nil {
		if tctx.Err() != nil {
			return nil, tctx.Err()
		}
		return nil, fmt.Errorf("decomposer: dispatch: %w", err)
	}
	output := strings.TrimSpace(result.CondensedOutput)
	if output == "" {
		output = strings.TrimSpace(result.Output)
	}
	if output == "" {
		return nil, fmt.Errorf("decomposer: empty output")
	}
	decomp, ok := parsePreClaimDecompositionOutput(output)
	if !ok {
		return nil, fmt.Errorf("decomposer: invalid output")
	}
	if err := validatePreClaimDecomposition(decomp); err != nil {
		return nil, err
	}
	return decomp, nil
}

// NewDecomposePostLadderExhaustionHook creates a PostLadderExhaustionHook that
// dispatches runDecomposer for TooLarge failure class.
func NewDecomposePostLadderExhaustionHook(store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string) PostLadderExhaustionHook {
	return func(ctx context.Context, beadID string, failureClass RecoveryFailureClass) (*PostLadderExhaustionResult, error) {
		if failureClass != TooLarge {
			return &PostLadderExhaustionResult{Attempted: false}, nil
		}
		result := runDecomposer(ctx, store, runner, rcfg, projectRoot, beadID)
		if result.Failed {
			return &PostLadderExhaustionResult{
				Attempted: true,
				Succeeded: false,
				Path:      Decompose,
				CostUSD:   result.CostUSD,
			}, nil
		}
		return &PostLadderExhaustionResult{
			Attempted: true,
			Succeeded: true,
			Path:      Decompose,
			CostUSD:   result.CostUSD,
		}, nil
	}
}

// runDecomposer dispatches a smart+ tier agent to split the bead into child
// beads after the escalation ladder has been exhausted. On success, child beads
// are created with Parent=beadID, the parent's execution-eligible is set to
// false, and a "decompose-applied" event is emitted. On failure a
// DecomposeResult with Failed=true is returned.
func runDecomposer(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, beadID string) DecomposeResult {
	tctx, cancel := context.WithTimeout(ctx, decomposerDefaultTimeout)
	defer cancel()

	b, err := store.Get(beadID)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "store_error"}
	}

	prompt, err := buildDecomposerPrompt(store, b)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "prompt_error"}
	}

	runtime := AgentRunRuntime{
		Prompt:           prompt,
		WorkDir:          projectRoot,
		PromptSource:     decomposerPromptSource,
		ProfileOverride:  selectProfileForDispatch(tctx, projectRoot, nil, runner, SelectStrongestProfile),
		ClearRoutingPins: true,
		ClearProfile:     true,
		ClearMinPower:    true,
		ClearMaxPower:    true,
	}

	result, err := dispatchViaResolvedConfig(tctx, projectRoot, nil, runner, rcfg, runtime)
	if err != nil {
		if tctx.Err() != nil {
			return DecomposeResult{Failed: true, Reason: "timeout"}
		}
		return DecomposeResult{Failed: true, Reason: "dispatch_error"}
	}

	output := strings.TrimSpace(result.CondensedOutput)
	if output == "" {
		output = strings.TrimSpace(result.Output)
	}
	if output == "" {
		return DecomposeResult{Failed: true, Reason: "empty_output", CostUSD: result.CostUSD}
	}

	children, ok := parseDecomposerOutput(output)
	if !ok {
		return DecomposeResult{Failed: true, Reason: "invalid_output", CostUSD: result.CostUSD}
	}

	if len(children) < 1 || len(children) > 5 {
		return DecomposeResult{Failed: true, Reason: "invalid_count", CostUSD: result.CostUSD}
	}

	for _, child := range children {
		if strings.TrimSpace(child.Title) == "" ||
			strings.TrimSpace(child.Description) == "" ||
			strings.TrimSpace(child.Acceptance) == "" {
			return DecomposeResult{Failed: true, Reason: "malformed_child", CostUSD: result.CostUSD}
		}
	}

	childIDs := make([]string, 0, len(children))
	for _, child := range children {
		nb := &bead.Bead{
			Title:       strings.TrimSpace(child.Title),
			Description: strings.TrimSpace(child.Description),
			Acceptance:  strings.TrimSpace(child.Acceptance),
			Labels:      append([]string(nil), child.Labels...),
			Parent:      beadID,
		}
		if err := store.Create(nb); err != nil {
			return DecomposeResult{Failed: true, Reason: "create_error"}
		}
		childIDs = append(childIDs, nb.ID)
	}

	_ = store.Update(beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[bead.ExtraExecutionElig] = false
	})

	body, _ := json.Marshal(decomposerEventBody{
		ChildIDs: childIDs,
		CostUSD:  result.CostUSD,
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "decompose-applied",
		Summary:   fmt.Sprintf("decomposed into %s", strings.Join(childIDs, ", ")),
		Body:      string(body),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})

	return DecomposeResult{
		Failed:   false,
		ChildIDs: childIDs,
		CostUSD:  result.CostUSD,
	}
}

func buildDecomposerPrompt(store ExecuteBeadLoopStore, b *bead.Bead) (string, error) {
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
		return "", fmt.Errorf("decomposer: encode prompt: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("MODE: decompose\n")
	sb.WriteString("You are splitting a bead that is too large into 2-5 independently executable child beads.\n")
	sb.WriteString("Each child must have a self-contained description, 3-6 numbered ACs with at least one named test function.\n")
	sb.WriteString("Return exactly one JSON array of child spec objects: [{\"title\":...,\"description\":...,\"acceptance\":...,\"labels\":[...]}]\n")
	sb.WriteString("Do not include prose or markdown; return only the JSON array.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

// parseDecomposerOutput extracts a JSON array of child specs from agent output.
func parseDecomposerOutput(output string) ([]decomposerChild, bool) {
	if c, ok := lastFencedBlock(output, "json"); ok {
		var children []decomposerChild
		if err := json.Unmarshal([]byte(strings.TrimSpace(c)), &children); err == nil {
			return children, true
		}
	}
	if c, ok := lastFencedBlock(output, ""); ok {
		trimmed := strings.TrimSpace(c)
		if strings.HasPrefix(trimmed, "[") {
			var children []decomposerChild
			if err := json.Unmarshal([]byte(trimmed), &children); err == nil {
				return children, true
			}
		}
	}
	// Scan for first '[' and balance brackets to find a raw JSON array.
	start := strings.Index(output, "[")
	if start == -1 {
		return nil, false
	}
	depth := 0
	for i := start; i < len(output); i++ {
		switch output[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				var children []decomposerChild
				if err := json.Unmarshal([]byte(output[start:i+1]), &children); err == nil {
					return children, true
				}
				return nil, false
			}
		}
	}
	return nil, false
}

func buildPreClaimDecomposerPrompt(store ExecuteBeadLoopStore, b *bead.Bead) (string, error) {
	type failureEntry struct {
		Summary string `json:"summary"`
		Body    string `json:"body,omitempty"`
	}
	type envelope struct {
		Title          string         `json:"title"`
		Description    string         `json:"description"`
		Acceptance     string         `json:"acceptance"`
		Labels         []string       `json:"labels,omitempty"`
		Parent         string         `json:"parent,omitempty"`
		FailureHistory []failureEntry `json:"failure_history,omitempty"`
	}

	env := envelope{
		Title:       strings.TrimSpace(b.Title),
		Description: strings.TrimSpace(b.Description),
		Acceptance:  strings.TrimSpace(b.Acceptance),
		Labels:      append([]string(nil), b.Labels...),
		Parent:      strings.TrimSpace(b.Parent),
	}
	if events, err := store.Events(b.ID); err == nil {
		for i := len(events) - 1; i >= 0 && len(env.FailureHistory) < 5; i-- {
			ev := events[i]
			switch ev.Kind {
			case "bead.result", "execute-bead", "intake.blocked", "pre_claim_intake.blocked", "no_changes_autonomous_retry":
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
		return "", fmt.Errorf("decomposer: encode prompt: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("MODE: preclaim-decompose\n")
	sb.WriteString("Split this too-large bead into 2-5 independently executable child beads.\n")
	sb.WriteString("Each child must be self-contained: include PROBLEM, ROOT CAUSE with file:line, PROPOSED FIX, NON-SCOPE, and numbered AC with named Test* symbols plus go test and lefthook gates.\n")
	sb.WriteString("Preserve every parent non-scope item, governing artifact reference, dependency ID, and verification requirement unless the AC map explicitly marks it non_scope.\n")
	sb.WriteString("Return exactly one JSON object: {\"children\":[{\"title\":...,\"description\":...,\"acceptance\":...,\"labels\":[...]}],\"ac_map\":[{\"parent_ac\":...,\"coverage\":...}],\"rationale\":...}.\n")
	sb.WriteString("Every parent acceptance criterion must appear in ac_map with non-empty coverage naming the child AC or non_scope/operator_required. Do not include prose or markdown.\n\n")
	sb.WriteString("```json\n")
	sb.Write(body)
	sb.WriteString("\n```\n")
	return sb.String(), nil
}

func parsePreClaimDecompositionOutput(output string) (*PreClaimDecomposition, bool) {
	if c, ok := lastFencedBlock(output, "json"); ok {
		output = strings.TrimSpace(c)
	} else if c, ok := extractJSONCandidate(output); ok {
		output = strings.TrimSpace(c)
	}
	var decomp PreClaimDecomposition
	if err := json.Unmarshal([]byte(output), &decomp); err != nil {
		return nil, false
	}
	return &decomp, true
}

func validatePreClaimDecomposition(decomp *PreClaimDecomposition) error {
	if decomp == nil {
		return fmt.Errorf("decomposer: nil decomposition")
	}
	if len(decomp.Children) < 1 || len(decomp.Children) > 5 {
		return fmt.Errorf("decomposer: invalid child count %d", len(decomp.Children))
	}
	for i := range decomp.Children {
		child := &decomp.Children[i]
		child.Title = strings.TrimSpace(child.Title)
		child.Description = strings.TrimSpace(child.Description)
		child.Acceptance = strings.TrimSpace(child.Acceptance)
		if child.Title == "" || child.Description == "" || child.Acceptance == "" {
			return fmt.Errorf("decomposer: malformed child %d", i+1)
		}
	}
	for i := range decomp.ACMap {
		decomp.ACMap[i].ParentAC = strings.TrimSpace(decomp.ACMap[i].ParentAC)
		decomp.ACMap[i].Coverage = strings.TrimSpace(decomp.ACMap[i].Coverage)
	}
	decomp.Rationale = strings.TrimSpace(decomp.Rationale)
	return nil
}
