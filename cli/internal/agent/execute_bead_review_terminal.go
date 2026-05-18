package agent

import (
	"fmt"
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

// reviewTerminalBlockEventBody builds the structured body for a
// ReviewTerminalBlockEventKind event, encoding the terminal class and
// result_rev so operators can triage without re-parsing reviewer text.
func reviewTerminalBlockEventBody(class, resultRev string) string {
	return fmt.Sprintf("terminal_class=%s\nresult_rev=%s", class, resultRev)
}

// applyTerminalReviewBlock records terminal block state on a bead: moves it to
// status=proposed with operator-required metadata and appends a
// review-terminal-block event. All store errors are best-effort — callers
// continue regardless so a store-side failure cannot strand the bead.
func applyTerminalReviewBlock(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, class, resultRev string) {
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
