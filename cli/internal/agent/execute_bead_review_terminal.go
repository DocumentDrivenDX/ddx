package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ReviewTerminalBlockEventKind is the event kind emitted when a BLOCK verdict
// is classified as terminal (requires human intervention, no automated retry).
const ReviewTerminalBlockEventKind = "review-terminal-block"

// Terminal review class constants. A BLOCK review verdict whose rationale
// contains one of these labels is mapped to a terminal operator-required
// outcome instead of re-entering the automated retry cycle.
const (
	ReviewTerminalClassSpecGap           = "review_spec_gap"
	ReviewTerminalClassMissingAcceptance = "review_missing_acceptance"
	ReviewTerminalClassTooLarge          = "review_too_large"
	ReviewTerminalClassUnsafeOrOutScope  = "review_unsafe_or_out_of_scope"
)

// classifyTerminalReviewBlock returns the terminal class label if structured
// review evidence identifies a human-required condition, or "" if the BLOCK is
// a regular fixable gap that may enter the automated retry cycle.
func classifyTerminalReviewBlock(res *ReviewResult) string {
	if res == nil || res.Verdict != VerdictBlock {
		return ""
	}
	classification := ClassifyReviewFindings(res)
	switch classification.Class {
	case ReviewTerminalClassSpecGap:
		return ReviewTerminalClassSpecGap
	case ReviewTerminalClassMissingAcceptance:
		return ReviewTerminalClassMissingAcceptance
	case ReviewTerminalClassTooLarge:
		return ReviewTerminalClassTooLarge
	case ReviewTerminalClassUnsafeOrOutScope:
		return ReviewTerminalClassUnsafeOrOutScope
	default:
		return ""
	}
}

// hasRecentOperatorPromotion checks if a bead has a recent triaged event where
// an operator promoted it from proposed to open. Returns true if the most recent
// triaged event in the bead's event history shows an operator accepting a
// proposed->open transition.
func hasRecentOperatorPromotion(events []bead.BeadEvent) bool {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Kind != "triaged" {
			continue
		}
		// Decode the triaged event body to check if it's a proposed->open transition.
		var body map[string]any
		if err := json.Unmarshal([]byte(ev.Body), &body); err != nil {
			continue
		}
		fromStatus, _ := body["from_status"].(string)
		toStatus, _ := body["to_status"].(string)
		operatorRequired, _ := body["operator_required"].(bool)

		// If this triaged event shows operator accepted a proposed->open transition,
		// the operator has intentionally promoted this bead.
		if fromStatus == bead.StatusProposed && toStatus == bead.StatusOpen && operatorRequired {
			return true
		}
	}
	return false
}

// hasRecentTerminalBlockEvent checks if there's a recent review-terminal-block
// event for the given result_rev. This prevents duplicate events on readiness
// re-evaluation when an operator-promoted bead is re-reviewed.
func hasRecentTerminalBlockEvent(events []bead.BeadEvent, resultRev string) bool {
	needle := fmt.Sprintf("result_rev=%s", resultRev)
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Kind != ReviewTerminalBlockEventKind {
			continue
		}
		// Check if the body mentions this result_rev.
		if strings.Contains(ev.Body, needle) {
			return true
		}
	}
	return false
}

// reviewTerminalBlockEventBody builds the structured body for a
// ReviewTerminalBlockEventKind event, encoding the terminal class and
// result_rev so operators can triage without re-parsing reviewer text.
func reviewTerminalBlockEventBody(class, resultRev string) string {
	return fmt.Sprintf("terminal_class=%s\nresult_rev=%s", class, resultRev)
}

// applyTerminalReviewBlock records terminal block state on a bead: moves it to
// status=proposed with operator-required metadata and appends a
// review-terminal-block event. If the bead has a recent operator promotion from
// proposed to open, the downgrade is skipped and the event is not appended if
// it already exists for this result_rev. All store errors are best-effort —
// callers continue regardless so a store-side failure cannot strand the bead.
func applyTerminalReviewBlock(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, class, resultRev string, events []bead.BeadEvent) {
	// If an operator has recently promoted this bead from proposed to open,
	// honor that decision and skip the terminal downgrade. Only append the event
	// if we haven't already recorded this terminal block for this result_rev.
	if hasRecentOperatorPromotion(events) {
		if !hasRecentTerminalBlockEvent(events, resultRev) {
			_ = store.AppendEvent(beadID, bead.BeadEvent{
				Kind:      ReviewTerminalBlockEventKind,
				Summary:   class,
				Body:      reviewTerminalBlockEventBody(class, resultRev),
				Actor:     actor,
				Source:    "ddx work",
				CreatedAt: now.UTC(),
			})
		}
		return
	}

	// No operator promotion: proceed with terminal parking.
	applyReviewOperatorRequiredParking(
		store,
		beadID,
		actor,
		now,
		"terminal review block: "+class,
		"review terminal block requires operator decision",
		"review terminal BLOCK and accept, split, block, or cancel",
	)
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      ReviewTerminalBlockEventKind,
		Summary:   class,
		Body:      reviewTerminalBlockEventBody(class, resultRev),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: now.UTC(),
	})
}

func applyReviewOperatorRequiredParking(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, reason, summary, suggestedAction string) {
	_ = parkToProposedWithOperatorMeta(store, beadID, bead.ParkReviewTerminal, ParkToProposedOpts{
		Reason:          reason,
		Summary:         summary,
		SuggestedAction: suggestedAction,
		Since:           now,
		CleanupLabels:   true,
	})
}
