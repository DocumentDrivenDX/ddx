package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ReviewFixableGapEventKind is the event kind emitted when a BLOCK verdict is
// classified as a fixable gap — an automated repair cycle is scheduled instead
// of routing to a terminal operator-required state.
const ReviewFixableGapEventKind = "review-fixable-gap"

// RepairContextFromReviewGroup carries the review-group reference needed to
// schedule one bounded repair attempt after a review_fixable_gap verdict. The
// caller is responsible for computing the repair MinPower from
// ImplementerActualPower using the escalation ladder and for invoking the
// repair executor with the original harness/model/provider/profile pins
// unchanged.
type RepairContextFromReviewGroup struct {
	// ReviewGroupID is the shared bundle ID of the review group whose BLOCK
	// triggered the repair. Empty when the reviewer is a single-slot reviewer
	// rather than a full review group.
	ReviewGroupID string
	// ResultRev is the result revision that the reviewer blocked. One repair
	// cycle is permitted per ResultRev; a second review_fixable_gap on the
	// same ResultRev falls through to the regular BLOCK triage path.
	ResultRev string
	// ReviewRationale is the reviewer's actionable rationale forwarded to the
	// repair executor as context.
	ReviewRationale string
	// ImplementerActualPower is the routing power of the model that produced
	// the blocked result. The repair attempt must use a MinPower strictly
	// above this value.
	ImplementerActualPower int
}

// reviewFixableGapEventBody builds the structured body for a
// review-fixable-gap event.
func reviewFixableGapEventBody(groupID, resultRev, rationale string) string {
	b := fmt.Sprintf("review_group_id=%s\nresult_rev=%s", groupID, resultRev)
	if rationale != "" {
		b += "\n" + rationale
	}
	return b
}

// hasReviewFixableGapRepairScheduled reports whether a repair cycle has
// already been recorded for the given resultRev on this bead. Used to enforce
// the one-cycle-per-result limit: a second review_fixable_gap on the same
// resultRev falls through to the regular BLOCK triage path instead of
// scheduling another repair.
func hasReviewFixableGapRepairScheduled(store ExecuteBeadLoopStore, beadID, resultRev string) bool {
	events, err := store.Events(beadID)
	if err != nil {
		return false
	}
	needle := "result_rev=" + resultRev
	for _, ev := range events {
		if ev.Kind == ReviewFixableGapEventKind && strings.Contains(ev.Body, needle) {
			return true
		}
	}
	return false
}

// scheduleReviewFixableGapRepair records a review-fixable-gap event on the
// bead and returns the RepairContextFromReviewGroup the caller needs to run
// the repair attempt. Callers must check hasReviewFixableGapRepairScheduled
// before calling this function to enforce the one-cycle limit.
func scheduleReviewFixableGapRepair(
	store ExecuteBeadLoopStore,
	beadID, actor string,
	now time.Time,
	reviewGroupID, resultRev, rationale string,
	implActualPower int,
) *RepairContextFromReviewGroup {
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      ReviewFixableGapEventKind,
		Summary:   "review_fixable_gap",
		Body:      reviewFixableGapEventBody(reviewGroupID, resultRev, rationale),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: now.UTC(),
	})
	return &RepairContextFromReviewGroup{
		ReviewGroupID:          reviewGroupID,
		ResultRev:              resultRev,
		ReviewRationale:        rationale,
		ImplementerActualPower: implActualPower,
	}
}
