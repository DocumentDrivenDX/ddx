package agent

import (
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

// classifyTerminalReviewBlock returns the terminal class label if the BLOCK
// result's rationale signals a human-required condition, or "" if the BLOCK is
// a regular fixable gap that should re-enter the automated retry cycle.
//
// Detection is substring-based on the lowercased rationale so reviewer output
// only needs to include the class label anywhere in the text.
func classifyTerminalReviewBlock(res *ReviewResult) string {
	if res == nil || res.Verdict != VerdictBlock {
		return ""
	}
	text := strings.ToLower(res.Rationale)
	switch {
	case strings.Contains(text, ReviewTerminalClassSpecGap):
		return ReviewTerminalClassSpecGap
	case strings.Contains(text, ReviewTerminalClassMissingAcceptance):
		return ReviewTerminalClassMissingAcceptance
	case strings.Contains(text, ReviewTerminalClassTooLarge):
		return ReviewTerminalClassTooLarge
	case strings.Contains(text, ReviewTerminalClassUnsafeOrOutScope):
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

// applyTerminalReviewBlock records terminal block state on a bead: adds the
// needs_human label, appends a review-terminal-block event, and parks the
// bead under MaxLoopCooldown. All store errors are best-effort — callers
// continue regardless so a store-side failure cannot strand the bead.
func applyTerminalReviewBlock(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, class, resultRev string) {
	_ = store.Update(beadID, func(b *bead.Bead) {
		if !HasBeadLabel(b.Labels, TriageNeedsHumanLabel) {
			b.Labels = append(b.Labels, TriageNeedsHumanLabel)
		}
	})
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      ReviewTerminalBlockEventKind,
		Summary:   class,
		Body:      reviewTerminalBlockEventBody(class, resultRev),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: now.UTC(),
	})
	parkUntil := now.UTC().Add(CapLoopCooldown(MaxLoopCooldown))
	_ = store.SetExecutionCooldown(beadID, parkUntil, ReviewTerminalBlockEventKind, class)
}
