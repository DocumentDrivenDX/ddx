package agent

import (
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// PreviewQueueStore is the minimal read-only interface PreviewQueue needs.
// *bead.Store satisfies this interface via ReadyExecution.
type PreviewQueueStore interface {
	ReadyExecution() ([]bead.Bead, error)
}

// PickerFilters carries the optional filter parameters that mirror the worker's
// label-filter and capabilities filtering. These are the stable, deterministic
// inputs to the picker — the per-Run attempted map is intentionally excluded
// because it is non-deterministic across runs.
type PickerFilters struct {
	// LabelFilter, when non-empty, restricts the queue to beads whose labels
	// include every comma-separated label in the filter string — identical
	// semantics to ExecuteBeadWorker.nextCandidate's label_filter check.
	LabelFilter string
	// Capabilities is reserved for future capability-matching; today it is a
	// no-op pass-through (no bead in the store carries a requires-capability
	// field that the worker checks at pick time).
	Capabilities string
}

// FilterDecision records whether a bead is eligible for the next claim.
type FilterDecision string

const (
	FilterDecisionNext     FilterDecision = "next claim"
	FilterDecisionEligible FilterDecision = "eligible"
	FilterDecisionSkipped  FilterDecision = "skipped"
)

// QueueEntry is one row in the PreviewQueue result. Position is 1-based;
// FilterDecision and Why explain why the picker would or would not pick this
// bead as the next claim.
type QueueEntry struct {
	Position       int            `json:"position"`
	BeadID         string         `json:"id"`
	Title          string         `json:"title"`
	Priority       int            `json:"priority"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Status         string         `json:"status"`
	FilterDecision FilterDecision `json:"filter_decision"`
	Why            string         `json:"why"`
}

// PreviewQueue returns the bead execution queue in picker order, annotated with
// filter decisions and skip reasons for each entry. It applies the same filter
// logic as ExecuteBeadWorker.nextCandidate (label_filter intersection) but
// intentionally omits the per-Run attempted map (non-deterministic across runs).
//
// The returned slice is ordered identically to what ReadyExecution returns —
// priority asc, then created_at asc within priority. The first entry whose
// FilterDecision is FilterDecisionNext is the bead nextCandidate would return
// on a fresh Run (before any claim).
//
// Limit controls the maximum number of entries returned. 0 means no limit
// (return all entries from ReadyExecution).
func PreviewQueue(store PreviewQueueStore, filters PickerFilters, limit int) ([]QueueEntry, error) {
	ready, err := store.ReadyExecution()
	if err != nil {
		return nil, fmt.Errorf("preview queue: %w", err)
	}

	foundFirst := false
	var entries []QueueEntry
	for _, b := range ready {
		entry := QueueEntry{
			Position:  len(entries) + 1,
			BeadID:    b.ID,
			Title:     b.Title,
			Priority:  b.Priority,
			UpdatedAt: b.UpdatedAt,
			Status:    b.Status,
		}

		// Apply label filter — same logic as nextCandidate.
		if filters.LabelFilter != "" && !HasBeadLabel(b.Labels, filters.LabelFilter) {
			entry.FilterDecision = FilterDecisionSkipped
			entry.Why = fmt.Sprintf("skipped: label_filter mismatch (bead labels=%v; worker filter=[%s])", b.Labels, filters.LabelFilter)
			entries = append(entries, entry)
			if limit > 0 && len(entries) >= limit {
				break
			}
			continue
		}

		// Capabilities filter — no-op today; reserved for future matching.
		// When implemented, add a skip reason here mirroring label_filter.

		// This bead is eligible. The first eligible bead is the "next claim".
		if !foundFirst {
			foundFirst = true
			entry.FilterDecision = FilterDecisionNext
			entry.Why = "next claim"
		} else {
			entry.FilterDecision = FilterDecisionEligible
			entry.Why = fmt.Sprintf("eligible (rank %d)", entry.Position)
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}

	return entries, nil
}
