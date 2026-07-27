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
	ChildIDs          []string `json:"child_ids,omitempty"`
	CostUSD           float64  `json:"cost_usd,omitempty"`
	RequestedHarness  string   `json:"requested_harness,omitempty"`
	RequestedProvider string   `json:"requested_provider,omitempty"`
	RequestedModel    string   `json:"requested_model,omitempty"`
	RequestedProfile  string   `json:"requested_profile,omitempty"`
	RequestedMinPower int      `json:"requested_min_power,omitempty"`
	RequestedMaxPower int      `json:"requested_max_power,omitempty"`
	SelectedHarness   string   `json:"selected_harness,omitempty"`
	SelectedProvider  string   `json:"selected_provider,omitempty"`
	SelectedModel     string   `json:"selected_model,omitempty"`
	SelectedPower     int      `json:"selected_power,omitempty"`
	FallbackReason    string   `json:"fallback_reason,omitempty"`
	// Family telemetry: distinguishes landed progress from queue expansion.
	FamilySize      int    `json:"family_size,omitempty"`
	FamilyDepth     int    `json:"family_depth,omitempty"`
	SplitReason     string `json:"split_reason,omitempty"`
	RefusalReason   string `json:"refusal_reason,omitempty"`
	FamilyBudget    int    `json:"family_budget,omitempty"`
	DescendantCount int    `json:"descendant_count,omitempty"`
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

	b, err := store.Get(ctx, beadID)
	if err != nil {
		return nil, fmt.Errorf("decomposer: load bead: %w", err)
	}
	prompt, err := buildPreClaimDecomposerPrompt(store, b)
	if err != nil {
		return nil, err
	}
	runtime := decomposerRuntime(rcfg)
	runtime.Prompt = prompt
	runtime.PromptSource = preClaimDecomposerPromptSource
	result, err := dispatchLifecycleRun(tctx, projectRoot, nil, runner, rcfg, runtime)
	if err != nil {
		if tctx.Err() != nil {
			return nil, tctx.Err()
		}
		reason := fmt.Sprintf("dispatch error: %s", err.Error())
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, reason); fallbackErr == nil {
			appendPreClaimDecomposeEvent(store, b.ID, nil, rcfg, reason)
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: dispatch: %w", err)
	}
	output := strings.TrimSpace(result.CondensedOutput)
	if output == "" {
		output = strings.TrimSpace(result.Output)
	}
	if output == "" {
		reason := "empty output"
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, reason); fallbackErr == nil {
			appendPreClaimDecomposeEvent(store, b.ID, result, rcfg, reason)
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: empty output")
	}
	decomp, ok := parsePreClaimDecompositionOutput(output)
	if !ok {
		reason := "invalid output"
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, reason); fallbackErr == nil {
			appendPreClaimDecomposeEvent(store, b.ID, result, rcfg, reason)
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: invalid output")
	}
	if err := validatePreClaimDecomposition(decomp, b); err != nil {
		reason := err.Error()
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, reason); fallbackErr == nil {
			appendPreClaimDecomposeEvent(store, b.ID, result, rcfg, reason)
			return fallback, nil
		}
		return nil, err
	}
	appendPreClaimDecomposeEvent(store, b.ID, result, rcfg, "")
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

// runDecomposer dispatches a smart+ powerClass agent to split the bead into child
// beads after the escalation ladder has been exhausted. On success, child beads
// are created with Parent=beadID, the parent's execution-eligible is set to
// false, and a "decompose-applied" event is emitted. On failure a
// DecomposeResult with Failed=true is returned.
func runDecomposer(ctx context.Context, store ExecuteBeadLoopStore, runner AgentRunner, rcfg config.ResolvedConfig, projectRoot string, beadID string) DecomposeResult {
	tctx, cancel := context.WithTimeout(ctx, decomposerDefaultTimeout)
	defer cancel()

	b, err := store.Get(ctx, beadID)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "store_error"}
	}

	prompt, err := buildDecomposerPrompt(store, b)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "prompt_error"}
	}

	runtime := decomposerRuntime(rcfg)
	runtime.Prompt = prompt
	runtime.PromptSource = decomposerPromptSource

	result, err := dispatchLifecycleRun(tctx, projectRoot, nil, runner, rcfg, runtime)
	if err != nil && result == nil {
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

	// Family-wide expansion budget: refuse before creating any children when
	// the proposed split would push the family past the resolved policy budget.
	policy := rcfg.DecompositionPolicy()
	stats := storeBeadFamilyStats(context.Background(), store, b)
	if reason := familyExpansionRefusalReason(stats, len(children), policy); reason != "" {
		body, _ := json.Marshal(decomposerEventBodyForResultWithTelemetry(nil, result, rcfg, "", decomposerEventTelemetry{
			FamilySize:      stats.FamilySize,
			FamilyDepth:     stats.FamilyDepth,
			DescendantCount: stats.DescendantCount,
			FamilyBudget:    policy.MaxFamilyExpansion,
			RefusalReason:   reason,
		}))
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      "triage-decomposed",
			Summary:   fmt.Sprintf("decomposition refused: %s", familyExpansionBudgetRefusal),
			Body:      string(body),
			Actor:     "ddx work",
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
		return DecomposeResult{Failed: true, Reason: familyExpansionBudgetRefusal, CostUSD: result.CostUSD}
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
		if err := store.Create(context.Background(), nb); err != nil {
			return DecomposeResult{Failed: true, Reason: "create_error"}
		}
		childIDs = append(childIDs, nb.ID)
	}

	_ = store.Update(context.Background(), beadID, func(b *bead.Bead) {
		ensureBeadExtra(b)
		b.Extra[bead.ExtraExecutionElig] = false
	})

	afterStats := storeBeadFamilyStats(context.Background(), store, b)
	body, _ := json.Marshal(decomposerEventBodyForResultWithTelemetry(childIDs, result, rcfg, "", decomposerEventTelemetry{
		FamilySize:      afterStats.FamilySize,
		FamilyDepth:     afterStats.FamilyDepth,
		DescendantCount: afterStats.DescendantCount,
		FamilyBudget:    policy.MaxFamilyExpansion,
		SplitReason:     "post-ladder decomposer accepted split",
	}))
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

func decomposerRuntime(rcfg config.ResolvedConfig) AgentRunRuntime {
	runtime := AgentRunRuntime{
		ClearProfile:     true,
		MinPowerOverride: lifecycleStrongMinPower,
	}
	applyLifecycleHookRouting(rcfg, &runtime)
	return runtime
}

func appendPreClaimDecomposeEvent(store ExecuteBeadLoopStore, beadID string, result *Result, rcfg config.ResolvedConfig, fallbackReason string) {
	tele := decomposerEventTelemetry{}
	if store != nil && strings.TrimSpace(beadID) != "" {
		if b, err := store.Get(context.Background(), beadID); err == nil && b != nil {
			stats := storeBeadFamilyStats(context.Background(), store, b)
			policy := rcfg.DecompositionPolicy()
			tele = decomposerEventTelemetry{
				FamilySize:      stats.FamilySize,
				FamilyDepth:     stats.FamilyDepth,
				DescendantCount: stats.DescendantCount,
				FamilyBudget:    policy.MaxFamilyExpansion,
			}
			if strings.TrimSpace(fallbackReason) != "" {
				tele.RefusalReason = strings.TrimSpace(fallbackReason)
			} else {
				tele.SplitReason = "preclaim decomposition routed"
			}
		}
	}
	body, _ := json.Marshal(decomposerEventBodyForResultWithTelemetry(nil, result, rcfg, fallbackReason, tele))
	summary := "preclaim decomposition routed"
	if strings.TrimSpace(fallbackReason) != "" {
		summary = "preclaim decomposition used deterministic fallback"
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "preclaim-decompose-routing",
		Summary:   summary,
		Body:      string(body),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})
}

// decomposerEventTelemetry optionally enriches a decomposer event with
// family-size / refusal fields used by operators to distinguish expansion from
// throughput.
type decomposerEventTelemetry struct {
	FamilySize      int
	FamilyDepth     int
	DescendantCount int
	FamilyBudget    int
	SplitReason     string
	RefusalReason   string
}

func decomposerEventBodyForResult(childIDs []string, result *Result, rcfg config.ResolvedConfig, fallbackReason string) decomposerEventBody {
	return decomposerEventBodyForResultWithTelemetry(childIDs, result, rcfg, fallbackReason, decomposerEventTelemetry{})
}

func decomposerEventBodyForResultWithTelemetry(childIDs []string, result *Result, rcfg config.ResolvedConfig, fallbackReason string, tele decomposerEventTelemetry) decomposerEventBody {
	harness, harnessExplicit := rcfg.ExplicitHarness()
	if !harnessExplicit {
		harness = ""
	}
	provider, providerExplicit := rcfg.ExplicitProvider()
	if !providerExplicit {
		provider = ""
	}
	model, modelExplicit := rcfg.ExplicitModel()
	if !modelExplicit {
		model = ""
	}
	policy := rcfg.DecompositionPolicy()
	body := decomposerEventBody{
		ChildIDs:          append([]string(nil), childIDs...),
		RequestedHarness:  harness,
		RequestedProvider: provider,
		RequestedModel:    model,
		RequestedProfile:  rcfg.Profile(),
		RequestedMinPower: rcfg.MinPower(),
		RequestedMaxPower: rcfg.MaxPower(),
		FallbackReason:    strings.TrimSpace(fallbackReason),
		FamilySize:        tele.FamilySize,
		FamilyDepth:       tele.FamilyDepth,
		DescendantCount:   tele.DescendantCount,
		FamilyBudget:      tele.FamilyBudget,
		SplitReason:       strings.TrimSpace(tele.SplitReason),
		RefusalReason:     strings.TrimSpace(tele.RefusalReason),
	}
	if body.FamilyBudget == 0 {
		body.FamilyBudget = policy.MaxFamilyExpansion
	}
	if body.RefusalReason == "" && strings.TrimSpace(fallbackReason) != "" {
		// Fallback/routing refusals surface as refusal_reason for operator
		// telemetry (in addition to the legacy fallback_reason field).
		body.RefusalReason = strings.TrimSpace(fallbackReason)
	}
	if result != nil {
		body.CostUSD = result.CostUSD
		body.SelectedHarness = strings.TrimSpace(result.Harness)
		body.SelectedProvider = strings.TrimSpace(result.Provider)
		body.SelectedModel = strings.TrimSpace(result.Model)
		body.SelectedPower = result.ActualPower
	}
	return body
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

// validatePreClaimDecomposition checks structural shape and, when parent is
// non-nil, material scope reduction. A split is accepted only when every child
// owns a concrete implementation outcome, parent ACs are mapped, children do
// not merely restate the parent outcome, and the split reduces implementation
// scope relative to the parent. Callers must treat a non-nil error as a hard
// refusal: do not create child beads (reuse the lossy-split refusal path).
func validatePreClaimDecomposition(decomp *PreClaimDecomposition, parent *bead.Bead) error {
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

	if parent == nil {
		return nil
	}

	// Missing or incomplete acceptance mapping (lossy split).
	if strings.TrimSpace(parent.Acceptance) != "" {
		if len(decomp.ACMap) == 0 {
			return fmt.Errorf("decomposer: missing acceptance mapping")
		}
		if isDecompositionLossy(decomp.ACMap) {
			return fmt.Errorf("decomposer: incomplete acceptance mapping")
		}
		if !parentAcceptanceMapped(parent, decomp.ACMap) {
			return fmt.Errorf("decomposer: missing acceptance mapping for parent criteria")
		}
	}

	for i := range decomp.Children {
		child := decomp.Children[i]
		if isVerificationOnlyChild(child) {
			return fmt.Errorf("decomposer: child %d is verification-only with no implementation deliverable", i+1)
		}
		if childDuplicatesParentOutcome(child, parent) {
			return fmt.Errorf("decomposer: child %d duplicates the parent outcome", i+1)
		}
		if !childHasConcreteImplementationOutcome(child) {
			return fmt.Errorf("decomposer: child %d lacks a concrete implementation outcome", i+1)
		}
	}

	if !splitReducesImplementationScope(parent, decomp) {
		return fmt.Errorf("decomposer: split does not reduce implementation scope relative to the parent")
	}
	return nil
}

// preClaimDecompositionRefuseReason returns a non-empty reason when a proposed
// split must not create children. It folds structural validation, material
// scope reduction, and the existing lossy AC-map check into one refusal signal
// so callers reuse a single park/block path.
func preClaimDecompositionRefuseReason(parent *bead.Bead, decomp *PreClaimDecomposition) string {
	if decomp == nil {
		return "decomposition is nil"
	}
	if err := validatePreClaimDecomposition(decomp, parent); err != nil {
		return err.Error()
	}
	parentAcceptance := ""
	if parent != nil {
		parentAcceptance = parent.Acceptance
	}
	if isDecompositionLossy(decomp.ACMap) || (len(decomp.ACMap) == 0 && strings.TrimSpace(parentAcceptance) != "") {
		return "decomposition AC map is incomplete; operator must produce a lossless split"
	}
	return ""
}

func normalizeDecompText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func parentAcceptanceMapped(parent *bead.Bead, acMap []ACMapEntry) bool {
	items := numberedAcceptanceItems(parent.Acceptance)
	if len(items) == 0 {
		// Freeform acceptance: non-empty, non-lossy map is sufficient.
		return len(acMap) > 0 && !isDecompositionLossy(acMap)
	}
	for _, item := range items {
		itemNorm := normalizeDecompText(item)
		found := false
		for _, entry := range acMap {
			parentAC := normalizeDecompText(entry.ParentAC)
			if parentAC == "" || strings.TrimSpace(entry.Coverage) == "" {
				continue
			}
			if parentAC == itemNorm || strings.Contains(parentAC, itemNorm) || strings.Contains(itemNorm, parentAC) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// isVerificationOnlyChild reports children that are test/validate/review-shaped
// with no implementation deliverable — pure queue bookkeeping. Named Test*
// symbols in acceptance are treated as implementation deliverables (DDx AC
// convention: author the named test), not as verification-only work.
func isVerificationOnlyChild(child PreClaimDecompositionChild) bool {
	blob := normalizeDecompText(child.Title + " " + child.Description + " " + child.Acceptance)
	if blob == "" {
		return true
	}
	if strings.Contains(child.Description, "PROPOSED FIX") {
		return false
	}
	if childHasImplementationSignal(blob) {
		return false
	}
	for _, item := range numberedAcceptanceItems(child.Acceptance) {
		if acceptanceNamesTestSymbol(item) {
			return false
		}
	}
	return childHasVerificationSignal(blob)
}

func acceptanceNamesTestSymbol(item string) bool {
	// Match TestFoo / Test_Foo style identifiers used in bead AC.
	fields := strings.Fields(item)
	for _, f := range fields {
		f = strings.Trim(f, "`\"'.,;:()[]")
		if strings.HasPrefix(f, "Test") && len(f) > len("Test") {
			rest := f[len("Test"):]
			if rest[0] == '_' || (rest[0] >= 'A' && rest[0] <= 'Z') {
				return true
			}
		}
	}
	return false
}

func childHasImplementationSignal(blob string) bool {
	// "do " / "add " etc. are deliberate prefixes to avoid matching "todo".
	signals := []string{
		"implement", "implementation", "add ", "fix ", "create ", "wire ",
		"update ", "refactor", "change ", "write ", "build ", "do ", "make ",
		"introduce", "replace", "remove ", "migrate", "extend", "proposed fix",
		"part a", "part b", "part c", "child scope", "bounded slice",
	}
	for _, s := range signals {
		if strings.Contains(blob, s) {
			return true
		}
	}
	return false
}

func childHasVerificationSignal(blob string) bool {
	signals := []string{
		"verify", "verification", "validate", "validation", "review",
		"run tests", "go test", "lefthook", "check that", "ensure tests",
		"test coverage", "assert ", "only test", "test-only", "tests pass",
		"run the suite", "regression suite",
	}
	for _, s := range signals {
		if strings.Contains(blob, s) {
			return true
		}
	}
	title := blob
	// Title-leading verify/test/review/validate patterns.
	for _, p := range []string{"verify ", "test ", "review ", "validate ", "check "} {
		if strings.HasPrefix(title, p) {
			return true
		}
	}
	return false
}

func childHasConcreteImplementationOutcome(child PreClaimDecompositionChild) bool {
	if isVerificationOnlyChild(child) {
		return false
	}
	if strings.Contains(child.Description, "PROPOSED FIX") {
		return true
	}
	blob := normalizeDecompText(child.Title + " " + child.Description + " " + child.Acceptance)
	if childHasImplementationSignal(blob) {
		return true
	}
	for _, item := range numberedAcceptanceItems(child.Acceptance) {
		if acceptanceNamesTestSymbol(item) ||
			strings.Contains(item, "/") ||
			strings.Contains(item, ".go") {
			return true
		}
	}
	// Non-empty description that is not pure verification counts as a scoped outcome.
	if strings.TrimSpace(child.Description) != "" {
		return true
	}
	return false
}

func childDuplicatesParentOutcome(child PreClaimDecompositionChild, parent *bead.Bead) bool {
	if parent == nil {
		return false
	}
	cTitle := normalizeDecompText(child.Title)
	pTitle := normalizeDecompText(parent.Title)
	cAcc := normalizeDecompText(child.Acceptance)
	pAcc := normalizeDecompText(parent.Acceptance)
	cDesc := normalizeDecompText(child.Description)
	pDesc := normalizeDecompText(parent.Description)

	if cAcc != "" && pAcc != "" && cAcc == pAcc {
		return true
	}
	if cTitle != "" && cTitle == pTitle && (cAcc == pAcc || cAcc == "" || pAcc == "") {
		return true
	}
	if cDesc != "" && pDesc != "" && cDesc == pDesc && cAcc == pAcc {
		return true
	}
	return false
}

// splitReducesImplementationScope reports whether children carve a smaller or
// partitioned implementation boundary rather than restating the full parent.
func splitReducesImplementationScope(parent *bead.Bead, decomp *PreClaimDecomposition) bool {
	if parent == nil || decomp == nil || len(decomp.Children) == 0 {
		return false
	}
	parentAcc := normalizeDecompText(parent.Acceptance)
	parentItems := numberedAcceptanceItems(parent.Acceptance)

	// All children restate the full parent acceptance → no reduction.
	allRestateFull := true
	for _, child := range decomp.Children {
		cAcc := normalizeDecompText(child.Acceptance)
		if parentAcc != "" && cAcc == parentAcc {
			continue
		}
		// Child owns every parent numbered item verbatim.
		if len(parentItems) > 0 && childOwnsAllParentItems(child, parentItems) {
			continue
		}
		allRestateFull = false
		break
	}
	if allRestateFull {
		// Exception: single child with some parent ACs marked non_scope /
		// operator_required is a deliberate scope drop.
		if len(decomp.Children) == 1 && acMapDropsParentScope(decomp.ACMap) {
			return true
		}
		return false
	}

	if len(decomp.Children) >= 2 {
		// Distinct child scopes (title+acceptance) evidence independent boundaries.
		seen := make(map[string]struct{}, len(decomp.Children))
		for _, child := range decomp.Children {
			key := normalizeDecompText(child.Title) + "\x00" + normalizeDecompText(child.Acceptance)
			seen[key] = struct{}{}
		}
		if len(seen) < 2 {
			// Identical siblings: no independent boundary.
			return false
		}
		return true
	}

	// Single child with acceptance distinct from parent.
	cAcc := normalizeDecompText(decomp.Children[0].Acceptance)
	if parentAcc != "" && cAcc == parentAcc && !acMapDropsParentScope(decomp.ACMap) {
		return false
	}
	return true
}

func childOwnsAllParentItems(child PreClaimDecompositionChild, parentItems []string) bool {
	if len(parentItems) == 0 {
		return false
	}
	blob := normalizeDecompText(child.Acceptance)
	for _, item := range parentItems {
		if !strings.Contains(blob, normalizeDecompText(item)) {
			return false
		}
	}
	return true
}

func acMapDropsParentScope(acMap []ACMapEntry) bool {
	for _, entry := range acMap {
		cov := normalizeDecompText(entry.Coverage)
		if cov == "non_scope" || cov == "operator_required" {
			return true
		}
	}
	return false
}

func fallbackPreClaimDecomposition(b *bead.Bead, reason string) (*PreClaimDecomposition, error) {
	items := numberedAcceptanceItems(b.Acceptance)
	if len(items) == 0 {
		items = []string{
			"Implement the first bounded slice of the parent bead.",
			"Implement the remaining bounded slice of the parent bead.",
		}
	}
	childCount := 2
	if len(items) >= 12 {
		childCount = 5
	} else if len(items) >= 9 {
		childCount = 4
	} else if len(items) >= 5 {
		childCount = 3
	}
	if childCount > len(items) {
		childCount = len(items)
	}
	if childCount < 1 {
		return nil, fmt.Errorf("fallback decomposer: no child scopes")
	}

	chunks := chunkStrings(items, childCount)
	children := make([]PreClaimDecompositionChild, 0, len(chunks))
	acMap := make([]ACMapEntry, 0, len(items))
	parentSummary := clampForPrompt(strings.TrimSpace(b.Description), 2400)
	if parentSummary == "" {
		parentSummary = strings.TrimSpace(b.Title)
	}
	baseLabels := append([]string(nil), b.Labels...)
	baseLabels = appendUniqueLabel(baseLabels, "decomposed")
	for i, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		title := fallbackChildTitle(b.Title, i+1)
		description := fallbackChildDescription(b, parentSummary, chunk, i+1, len(chunks))
		acceptance := fallbackChildAcceptance(chunk, b.Acceptance)
		children = append(children, PreClaimDecompositionChild{
			Title:       title,
			Description: description,
			Acceptance:  acceptance,
			Labels:      append([]string(nil), baseLabels...),
		})
		for _, item := range chunk {
			acMap = append(acMap, ACMapEntry{
				ParentAC: item,
				Coverage: fmt.Sprintf("covered by child %d: %s", i+1, title),
			})
		}
	}
	if len(children) == 0 {
		return nil, fmt.Errorf("fallback decomposer: no child scopes")
	}
	return &PreClaimDecomposition{
		Children:  children,
		ACMap:     acMap,
		Rationale: "deterministic fallback split after agent decomposer returned " + strings.TrimSpace(reason),
	}, nil
}

func numberedAcceptanceItems(acceptance string) []string {
	lines := strings.Split(acceptance, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		dot := strings.Index(trimmed, ".")
		if dot <= 0 {
			continue
		}
		prefix := trimmed[:dot]
		if _, err := fmt.Sscanf(prefix, "%d", new(int)); err != nil {
			continue
		}
		item := strings.TrimSpace(trimmed[dot+1:])
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func chunkStrings(items []string, chunks int) [][]string {
	if chunks <= 0 {
		return nil
	}
	out := make([][]string, 0, chunks)
	for i := 0; i < chunks; i++ {
		start := i * len(items) / chunks
		end := (i + 1) * len(items) / chunks
		out = append(out, items[start:end])
	}
	return out
}

func fallbackChildTitle(parentTitle string, n int) string {
	title := strings.TrimSpace(parentTitle)
	if len(title) > 72 {
		title = strings.TrimSpace(title[:72])
	}
	if title == "" {
		title = "decomposed bead"
	}
	return fmt.Sprintf("%s: part %d", title, n)
}

func fallbackChildDescription(parent *bead.Bead, parentSummary string, chunk []string, n, total int) string {
	var sb strings.Builder
	sb.WriteString("PROBLEM\n")
	sb.WriteString("Parent bead ")
	sb.WriteString(parent.ID)
	sb.WriteString(" is too broad for one execution pass. This child owns part ")
	sb.WriteString(fmt.Sprintf("%d of %d", n, total))
	sb.WriteString(" of that parent scope.\n\nROOT CAUSE\n")
	sb.WriteString(parentSummary)
	sb.WriteString("\n\nPROPOSED FIX\nImplement only this child acceptance slice:\n")
	for _, item := range chunk {
		sb.WriteString("- ")
		sb.WriteString(item)
		sb.WriteByte('\n')
	}
	sb.WriteString("\nNON-SCOPE\nDo not implement parent acceptance criteria assigned to sibling child beads. Preserve every parent non-scope constraint unless this child explicitly narrows it.")
	return sb.String()
}

func fallbackChildAcceptance(chunk []string, parentAcceptance string) string {
	var sb strings.Builder
	for i, item := range chunk {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	next := len(chunk) + 1
	if !strings.Contains(parentAcceptance, "go test") {
		sb.WriteString(fmt.Sprintf("%d. cd cli && go test ./... passes.\n", next))
		next++
	}
	if !strings.Contains(parentAcceptance, "lefthook run pre-commit") {
		sb.WriteString(fmt.Sprintf("%d. lefthook run pre-commit passes.\n", next))
	}
	return strings.TrimSpace(sb.String())
}

func appendUniqueLabel(labels []string, label string) []string {
	for _, existing := range labels {
		if existing == label {
			return labels
		}
	}
	return append(labels, label)
}

func clampForPrompt(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "\n...[truncated]"
}
