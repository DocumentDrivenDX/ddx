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

const autoDecomposeAttemptedEventKind = "auto_decompose_attempted"
const autoDecomposeFailedEventKind = "auto_decompose_failed"
const autoRemediationExhaustedEventKind = "auto_remediation_exhausted"

// AutoDecomposeEpicError indicates why an epic failed the precondition gates.
type AutoDecomposeEpicError struct {
	Reason  string
	Details string
}

func (e *AutoDecomposeEpicError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("auto_decompose_epic: %s: %s", e.Reason, e.Details)
	}
	return fmt.Sprintf("auto_decompose_epic: %s", e.Reason)
}

// AutoDecomposeEpic dispatches a preclaim decomposer for a genuinely undecomposed
// epic after validating every precondition. On precondition failure, returns
// a typed AutoDecomposeEpicError and increments the per-bead auto_decompose_attempted
// event count. On success, returns (DecomposeResult, nil). On decomposer failure,
// sets a 15m execution cooldown, emits auto_decompose_failed, and on the 3rd
// cumulative failure also emits auto_remediation_exhausted.
func AutoDecomposeEpic(
	ctx context.Context,
	store ExecuteBeadLoopStore,
	runner AgentRunner,
	rcfg config.ResolvedConfig,
	projectRoot string,
	beadID string,
) (DecomposeResult, error) {
	b, err := store.Get(ctx, beadID)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "store_error"}, fmt.Errorf("load bead: %w", err)
	}

	// 1. IssueType must be "epic"
	issueType := strings.ToLower(strings.TrimSpace(b.IssueType))
	if issueType != "epic" {
		incrementAutoDecomposeAttempted(store, beadID, "not_epic")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "not_epic",
			Details: fmt.Sprintf("IssueType=%q, want epic", b.IssueType),
		}
	}

	// 2. totalChildCount must be 0; also check we're not in EpicClosureCandidate bucket
	totalChildren, openChildren, err := countChildren(store, ctx, beadID)
	if err != nil {
		return DecomposeResult{Failed: true, Reason: "index_error"}, err
	}
	if totalChildren > 0 {
		incrementAutoDecomposeAttempted(store, beadID, "has_children")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "has_children",
			Details: fmt.Sprintf("totalChildCount=%d, want 0", totalChildren),
		}
	}
	// If epic has children and none are open, it's in the closure-candidate bucket
	if totalChildren > 0 && openChildren == 0 {
		incrementAutoDecomposeAttempted(store, beadID, "in_closure_candidate")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "in_closure_candidate",
			Details: "bead is in EpicClosureCandidate bucket",
		}
	}

	// 3. No open dep_blocked_by or closed_or_missing_parent diagnoses
	allBeads := getAllBeads(store, ctx)
	childIndex := buildChildIndex(allBeads)
	openChildIndex := buildOpenChildIndex(allBeads)
	depIndex := buildDepIndex(allBeads)

	blockReason := bead.Diagnose(
		*b,
		childIndex,
		openChildIndex,
		depIndex,
	)
	if blockReason.Code == bead.ReasonDepBlockedBy || blockReason.Code == bead.ReasonClosedOrMissingParent {
		incrementAutoDecomposeAttempted(store, beadID, "dep_blocked")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "dep_blocked",
			Details: blockReason.Detail,
		}
	}

	// 4. No labels in {manual-hold, no-auto-decompose, container}
	for _, label := range b.Labels {
		switch label {
		case "manual-hold", "no-auto-decompose", "container":
			incrementAutoDecomposeAttempted(store, beadID, "override_label")
			return DecomposeResult{}, &AutoDecomposeEpicError{
				Reason:  "override_label",
				Details: fmt.Sprintf("label=%q", label),
			}
		}
	}

	// 5. Description must have PROBLEM and PROPOSED FIX sections, at least one AC,
	// and at least one spec:/area: label
	if err := validateEpicDescription(b); err != nil {
		incrementAutoDecomposeAttempted(store, beadID, "malformed_description")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "malformed_description",
			Details: err.Error(),
		}
	}

	// 6. auto_decompose_attempted count < 3
	attemptCount := countAutoDecomposeAttempts(store, beadID)
	if attemptCount >= 3 {
		incrementAutoDecomposeAttempted(store, beadID, "attempt_cap_reached")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "attempt_cap_reached",
			Details: fmt.Sprintf("attemptCount=%d, cap=3", attemptCount),
		}
	}

	// 7. No active execution cooldown
	if hasActiveCooldown(b) {
		incrementAutoDecomposeAttempted(store, beadID, "active_cooldown")
		return DecomposeResult{}, &AutoDecomposeEpicError{
			Reason:  "active_cooldown",
			Details: extraStringValFromExtra(b.Extra, bead.ExtraRetryAfter),
		}
	}

	// All preconditions passed; dispatch runPreClaimDecomposer
	decomp, dispatchErr := runPreClaimDecomposer(ctx, store, runner, rcfg, projectRoot, beadID)
	if dispatchErr != nil || decomp == nil {
		// Decomposition failed; set cooldown and emit event
		reason := "decomposer_error"
		if dispatchErr != nil {
			reason = dispatchErr.Error()
		}
		cooldownUntil := time.Now().UTC().Add(15 * time.Minute)
		_ = store.SetExecutionCooldown(
			beadID,
			cooldownUntil,
			autoDecomposeFailedEventKind,
			fmt.Sprintf("decomposer failed: %s", reason),
			"",
		)
		_ = store.AppendEvent(beadID, bead.BeadEvent{
			Kind:      autoDecomposeFailedEventKind,
			Summary:   fmt.Sprintf("decomposition failed: %s", reason),
			Body:      fmt.Sprintf(`{"reason":"%s"}`, reason),
			Actor:     "ddx work",
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})

		// On 3rd cumulative failure, emit auto_remediation_exhausted
		if attemptCount+1 >= 3 {
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      autoRemediationExhaustedEventKind,
				Summary:   "auto-decomposition attempts exhausted",
				Body:      `{"reason":"auto_decompose_failed_3x"}`,
				Actor:     "ddx work",
				Source:    "ddx work",
				CreatedAt: time.Now().UTC(),
			})
		}

		return DecomposeResult{Failed: true, Reason: reason}, nil
	}

	// Success: create child beads from the decomposition, set execution-eligible=false
	childIDs := make([]string, 0, len(decomp.Children))
	totalCost := 0.0

	for _, child := range decomp.Children {
		nb := &bead.Bead{
			Title:       child.Title,
			Description: child.Description,
			Acceptance:  child.Acceptance,
			Labels:      append([]string(nil), child.Labels...),
			Parent:      beadID,
		}
		if err := store.Create(ctx, nb); err != nil {
			return DecomposeResult{Failed: true, Reason: "child_creation_failed"}, err
		}
		childIDs = append(childIDs, nb.ID)
	}

	// Set parent's execution-eligible to false
	_ = store.Update(ctx, beadID, func(updated *bead.Bead) {
		ensureBeadExtra(updated)
		updated.Extra[bead.ExtraExecutionElig] = false
	})

	// Emit auto_decompose_attempted event
	body, _ := json.Marshal(decomposerEventBody{
		ChildIDs: childIDs,
		CostUSD:  totalCost,
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      autoDecomposeAttemptedEventKind,
		Summary:   fmt.Sprintf("auto-decomposed into %s", strings.Join(childIDs, ", ")),
		Body:      string(body),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})

	return DecomposeResult{Failed: false, ChildIDs: childIDs, CostUSD: totalCost}, nil
}

// validateEpicDescription checks that the description contains PROBLEM, PROPOSED FIX,
// at least one numbered AC, and at least one spec:/area: label.
func validateEpicDescription(b *bead.Bead) error {
	desc := strings.TrimSpace(b.Description)
	if desc == "" {
		return fmt.Errorf("empty description")
	}
	if !strings.Contains(desc, "PROBLEM") {
		return fmt.Errorf("missing PROBLEM section")
	}
	if !strings.Contains(desc, "PROPOSED FIX") {
		return fmt.Errorf("missing PROPOSED FIX section")
	}

	// Check for at least one numbered AC (1. pattern)
	hasAC := false
	acLines := strings.Split(b.Acceptance, "\n")
	for _, line := range acLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "1.") {
			hasAC = true
			break
		}
	}
	if !hasAC {
		return fmt.Errorf("missing numbered acceptance criteria")
	}

	// Check for at least one spec:/area: label
	hasSpecOrArea := false
	for _, label := range b.Labels {
		if strings.HasPrefix(label, "spec:") || strings.HasPrefix(label, "area:") {
			hasSpecOrArea = true
			break
		}
	}
	if !hasSpecOrArea {
		return fmt.Errorf("missing spec: or area: label")
	}

	return nil
}

// incrementAutoDecomposeAttempted increments the auto_decompose_attempted event count
// for the bead and records the reason.
func incrementAutoDecomposeAttempted(store ExecuteBeadLoopStore, beadID, reason string) {
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "auto_decompose_attempt_gate_failed",
		Summary:   fmt.Sprintf("precondition gate failed: %s", reason),
		Body:      fmt.Sprintf(`{"reason":"%s"}`, reason),
		Actor:     "ddx work",
		Source:    "ddx work",
		CreatedAt: time.Now().UTC(),
	})
}

// countAutoDecomposeAttempts counts cumulative auto_decompose_attempted events
// (both successes and gate failures).
func countAutoDecomposeAttempts(store ExecuteBeadLoopStore, beadID string) int {
	events, err := store.Events(beadID)
	if err != nil {
		return 0
	}
	count := 0
	for _, ev := range events {
		if ev.Kind == autoDecomposeAttemptedEventKind || ev.Kind == "auto_decompose_attempt_gate_failed" || ev.Kind == autoDecomposeFailedEventKind {
			count++
		}
	}
	return count
}

// hasActiveCooldown checks if the bead has a retry-after cooldown that is still active.
func hasActiveCooldown(b *bead.Bead) bool {
	retryAfterStr := extraStringValFromExtra(b.Extra, bead.ExtraRetryAfter)
	if retryAfterStr == "" {
		return false
	}
	retryAfter, err := time.Parse(time.RFC3339, retryAfterStr)
	if err != nil {
		return false
	}
	return retryAfter.After(time.Now().UTC())
}

// extraStringValFromExtra extracts a string value from the bead's Extra map.
func extraStringValFromExtra(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// countChildren returns the total and open child counts for a bead.
func countChildren(store ExecuteBeadLoopStore, ctx context.Context, beadID string) (total, open int, err error) {
	allBeads := getAllBeads(store, ctx)
	for _, candidate := range allBeads {
		if candidate.Parent == beadID {
			total++
			if candidate.Status != bead.StatusClosed && candidate.Status != bead.StatusCancelled {
				open++
			}
		}
	}
	return total, open, nil
}

// getAllBeads fetches all beads from the store via ReadyExecution().
func getAllBeads(store ExecuteBeadLoopStore, ctx context.Context) []bead.Bead {
	readyBeads, err := store.ReadyExecution()
	if err == nil && len(readyBeads) > 0 {
		return readyBeads
	}
	// Fallback: return empty slice if ReadyExecution fails
	return []bead.Bead{}
}

// buildChildIndex builds a child index from all beads.
// This is used by Diagnose; beads are keyed by parent ID with roots under "".
func buildChildIndex(beads []bead.Bead) map[string][]bead.Bead {
	index := make(map[string][]bead.Bead)
	for _, b := range beads {
		parent := b.Parent
		if parent == "" {
			parent = ""
		}
		index[parent] = append(index[parent], b)
	}
	// Ensure roots are in the index
	index[""] = append(index[""], beads...)
	return index
}

// buildOpenChildIndex builds an open-child index from all beads.
// Only beads with open status are included.
func buildOpenChildIndex(beads []bead.Bead) map[string][]bead.Bead {
	index := make(map[string][]bead.Bead)
	for _, b := range beads {
		if b.Status == bead.StatusClosed || b.Status == bead.StatusCancelled {
			continue
		}
		parent := b.Parent
		if parent == "" {
			parent = ""
		}
		index[parent] = append(index[parent], b)
	}
	return index
}

// buildDepIndex builds a dependency index from all beads.
// Maps bead_id -> outgoing Dependency entries.
func buildDepIndex(beads []bead.Bead) map[string][]bead.Dependency {
	index := make(map[string][]bead.Dependency)
	for _, b := range beads {
		index[b.ID] = append(index[b.ID], b.Dependencies...)
	}
	return index
}
