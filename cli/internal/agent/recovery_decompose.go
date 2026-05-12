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
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, fmt.Sprintf("dispatch error: %s", err.Error())); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: dispatch: %w", err)
	}
	output := strings.TrimSpace(result.CondensedOutput)
	if output == "" {
		output = strings.TrimSpace(result.Output)
	}
	if output == "" {
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, "empty output"); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: empty output")
	}
	decomp, ok := parsePreClaimDecompositionOutput(output)
	if !ok {
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, "invalid output"); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("decomposer: invalid output")
	}
	if err := validatePreClaimDecomposition(decomp); err != nil {
		if fallback, fallbackErr := fallbackPreClaimDecomposition(b, err.Error()); fallbackErr == nil {
			return fallback, nil
		}
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
