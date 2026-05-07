package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/triage"
)

// triageDecisionEventKind is the event kind used to record the triage
// policy's chosen action after a non-APPROVE review verdict.
const triageDecisionEventKind = "triage-decision"

// ReviewFixableGapEventKind is the event kind appended when a review verdict
// is classified as review_fixable_gap (REQUEST_CHANGES or non-terminal BLOCK).
// The event body encodes the repair context so the caller can schedule exactly
// one bounded repair cycle with MinPower above the implementer's actual power.
const ReviewFixableGapEventKind = "review-fixable-gap"

// RepairContextFromReviewGroup encodes the per-result-rev context returned
// when a review_fixable_gap (REQUEST_CHANGES or non-terminal BLOCK) is
// detected. The caller must schedule exactly one bounded repair cycle using
// ImplActualPower as the floor for the repair's MinPower request; all other
// routing pins (harness, model, provider, profile) must be forwarded unchanged.
type RepairContextFromReviewGroup struct {
	ReviewGroupID   string `json:"review_group_id,omitempty"`
	ResultRev       string `json:"result_rev"`
	ImplActualPower int    `json:"impl_actual_power"`
	Rationale       string `json:"rationale,omitempty"`
}

// PostMergeReviewInput bundles the data RunPostMergeReview needs from its
// caller — historically the execute-bead loop body, now the per-attempt
// pipeline owned by try.Attempt (ddx-a921ff01 C3). The fields mirror what
// the inlined block read from the worker, the candidate bead, the report,
// and the loop runtime.
type PostMergeReviewInput struct {
	Bead          bead.Bead
	Report        ExecuteBeadReport
	Reviewer      BeadReviewer
	Store         ExecuteBeadLoopStore
	ProjectRoot   string
	Rcfg          config.ResolvedConfig
	NoReview      bool
	Log           io.Writer
	Assignee      string
	Now           func() time.Time
	ReviewCostCap *escalation.CostCapTracker
}

// PostMergeReviewOutput carries the review state machine's decision back to
// the caller. Report is the (possibly updated) ExecuteBeadReport; Approved
// reports whether the caller should treat this attempt as a success. When
// the review path encountered a non-fatal Store error the caller is expected
// to route StoreErr through its outcome-error handler
// (handleOutcomeStoreError on ExecuteBeadWorker), keyed by StoreErrOp.
//
// RepairContext, when non-nil, signals that review_fixable_gap was detected and
// the caller should schedule exactly one bounded repair attempt with MinPower
// above RepairContext.ImplActualPower. The review-fixable-gap event has already
// been recorded; the caller must not re-record it.
type PostMergeReviewOutput struct {
	Report        ExecuteBeadReport
	Approved      bool
	StoreErrOp    string
	StoreErr      error
	RepairContext *RepairContextFromReviewGroup
}

// RunPostMergeReview executes the post-merge review state machine for a
// successful bead attempt. When review is skipped (no reviewer or --no-review)
// the bead is closed and the function returns Approved=true. When review is
// active, the execute-loop asks the review coordinator to evaluate the
// close-eligible result before CloseWithEvidence runs; close only proceeds on
// unanimous evidence-backed APPROVE. REQUEST_CHANGES and BLOCK record their
// review events without reopening the bead.
//
// The review:skip label only skips when it carries a sibling
// review:skip-reason:* label; otherwise it is ignored. When review is not
// skipped, the review coordinator's verdict(s) or error drive the canonical
// event emissions (review / review-error / review-manual-required /
// review-malfunction), and on a non-APPROVE verdict the triage policy is
// still consulted without reopening the bead.
//
// Returns Approved=true on the APPROVE path and on the skip path; returns
// Approved=false on REQUEST_CHANGES, BLOCK, malformed APPROVE/BLOCK, or
// reviewer transport error. Store errors are surfaced via StoreErrOp +
// StoreErr so callers can keep the existing handleOutcomeStoreError pattern
// unchanged.
//
// This is the C3 (ddx-a921ff01) extraction of the 180-line review block
// previously inline in execute_bead_loop.go; the loop now sees a structured
// outcome and conflict-recovery / decomposition / no_changes paths remain
// in the loop body until subsequent C-series beads (C4, C5) move them too.
func RunPostMergeReview(ctx context.Context, in PostMergeReviewInput) PostMergeReviewOutput {
	report := in.Report
	out := PostMergeReviewOutput{Report: report, Approved: true}

	now := in.Now
	if now == nil {
		now = time.Now
	}

	reviewSkipped := in.Reviewer == nil || in.NoReview
	if !reviewSkipped && HasBeadLabel(in.Bead.Labels, "review:skip") {
		reviewSkipped = HasBeadLabelPrefix(in.Bead.Labels, "review:skip-reason:")
	}

	if reviewSkipped {
		if err := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); err != nil {
			out.StoreErrOp = "CloseWithEvidence"
			out.StoreErr = err
			return out
		}
		out.Report = report
		return out
	}

	implRouting := ImplementerRouting{
		Harness:     report.Harness,
		Provider:    report.Provider,
		Model:       report.Model,
		ActualPower: report.ActualPower,
		Correlation: map[string]string{
			"bead_id":    in.Bead.ID,
			"attempt_id": report.AttemptID,
			"session_id": report.SessionID,
			"result_rev": report.ResultRev,
		},
	}
	reviewGroup, reviewErr := runPreCloseReviewGroup(ctx, in.Reviewer, in.Bead.ID, report.ResultRev, implRouting)
	reviewRes, reviewGroupErr := reducePreCloseReviewGroup(reviewGroup)
	chargeReviewCost := func() bool {
		budgetExceeded := false
		if capTracker := in.ReviewCostCap; capTracker != nil && reviewGroup != nil {
			deferred := false
			for _, slot := range reviewGroup.Slots {
				if slot.Result == nil {
					continue
				}
				capTracker.Add(slot.Result.ReviewerHarness, slot.Result.CostUSD)
				if detail, capped := capTracker.Tripped(); capped && !deferred {
					_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
						Kind:      "review-cost-deferred",
						Summary:   "review-cost-deferred",
						Body:      ReviewCostDeferredEventBody(report.ResultRev, slot.Result.CostUSD, capTracker.Spent(), capTracker.MaxUSD),
						Actor:     in.Assignee,
						Source:    "ddx agent execute-loop",
						CreatedAt: now().UTC(),
					})
					if in.Log != nil {
						_, _ = fmt.Fprintf(in.Log, "review cost cap deferred (%s %s): %s\n", in.Bead.ID, report.ResultRev, detail)
					}
					deferred = true
					budgetExceeded = true
				}
			}
		}
		return budgetExceeded
	}
	if reviewGroupErr != nil {
		reviewErr = reviewGroupErr
	}
	if reviewErr != nil {
		class := ClassifyReviewError(reviewErr, reviewRes)
		prior := CountPriorReviewErrors(in.Store, in.Bead.ID, report.ResultRev)
		attemptCount := prior + 1
		maxRetries := in.Rcfg.ReviewMaxRetries()
		if maxRetries <= 0 {
			maxRetries = DefaultReviewMaxRetries
		}
		errSummary := EventBodySummary{
			InputBytes:  0,
			OutputBytes: 0,
		}
		if reviewRes != nil {
			errSummary = EventBodySummary{
				Harness:     reviewRes.ReviewerHarness,
				Model:       reviewRes.ReviewerModel,
				InputBytes:  reviewRes.InputBytes,
				OutputBytes: reviewRes.OutputBytes,
				ElapsedMS:   reviewRes.DurationMS,
			}
		}
		if attemptCount >= maxRetries {
			body := AppendEventSummary(ReviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error()), errSummary)
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-manual-required",
				Summary:   class,
				Body:      body,
				Actor:     in.Assignee,
				Source:    "ddx agent execute-loop",
				CreatedAt: now().UTC(),
			})
			_ = in.Store.Update(in.Bead.ID, func(b *bead.Bead) {
				if !HasBeadLabel(b.Labels, TriageNeedsHumanLabel) {
					b.Labels = append(b.Labels, TriageNeedsHumanLabel)
				}
			})
			parkUntil := now().UTC().Add(CapLoopCooldown(MaxLoopCooldown))
			_ = in.Store.SetExecutionCooldown(in.Bead.ID, parkUntil, "review-manual-required", class)
		} else {
			body := AppendEventSummary(ReviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error()), errSummary)
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-error",
				Summary:   class,
				Body:      body,
				Actor:     in.Assignee,
				Source:    "ddx agent execute-loop",
				CreatedAt: now().UTC(),
			})
		}
		if reviewRes != nil {
			report.ReviewVerdict = string(reviewRes.Verdict)
			report.ReviewRationale = reviewRes.Rationale
		}
		report.Status = ExecuteBeadStatusReviewMalfunction
		report.Detail = "pre-close review: " + class
		out.Report = report
		out.Approved = false
		chargeReviewCost()
		return out
	}

	report.ReviewVerdict = string(reviewRes.Verdict)
	report.ReviewRationale = reviewRes.Rationale
	artifactPath, artifactErr := PersistReviewerStream(in.ProjectRoot, in.Bead.ID, report.AttemptID, reviewRes.RawOutput)
	if artifactErr != nil && in.Log != nil {
		_, _ = fmt.Fprintf(in.Log, "reviewer stream artifact: %v\n", artifactErr)
	}

	reviewSummary := EventBodySummary{
		Harness:     reviewRes.ReviewerHarness,
		Model:       reviewRes.ReviewerModel,
		InputBytes:  reviewRes.InputBytes,
		OutputBytes: reviewRes.OutputBytes,
		ElapsedMS:   reviewRes.DurationMS,
	}
	reviewBudgetExceeded := chargeReviewCost()
	switch reviewRes.Verdict {
	case VerdictApprove:
		// Approved: record the verdict event and then close. The close must
		// land AFTER the review event so the gate sees the terminal verdict.
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "APPROVE",
			Body:      AppendEventSummary(ReviewEventBody("APPROVE", reviewRes.Rationale, artifactPath), reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		if reviewBudgetExceeded {
			// Reviewer cost tripped the budget cap; leave bead open so the
			// loop's BudgetStop can stop the drain before the next claim.
			out.Report = report
			out.Approved = false
			return out
		}
		if cerr := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); cerr != nil {
			out.Report = report
			out.StoreErrOp = "CloseWithEvidence"
			out.StoreErr = cerr
			return out
		}
	case VerdictRequestChanges:
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "REQUEST_CHANGES",
			Body:      AppendEventSummary(ReviewEventBody("REQUEST_CHANGES", reviewRes.Rationale, artifactPath), reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		report.Status = ExecuteBeadStatusReviewRequestChanges
		report.Detail = "pre-close review: REQUEST_CHANGES"
		out.Approved = false
	case VerdictBlock:
		rationale := strings.TrimSpace(reviewRes.Rationale)
		if rationale == "" {
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-malfunction",
				Summary:   "BLOCK without rationale",
				Body:      AppendEventSummary(ReviewEventBody("BLOCK without rationale", "", artifactPath), reviewSummary),
				Actor:     in.Assignee,
				Source:    "ddx agent execute-loop",
				CreatedAt: now().UTC(),
			})
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-close review: malformed BLOCK verdict (missing rationale)"
			report.ReviewRationale = ""
			out.Report = report
			out.Approved = false
			return out
		}
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "BLOCK",
			Body:      AppendEventSummary(rationale, reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		if terminalClass := classifyTerminalReviewBlock(reviewRes); terminalClass != "" {
			applyTerminalReviewBlock(in.Store, in.Bead.ID, in.Assignee, now().UTC(), terminalClass, report.ResultRev)
			report.Status = ExecuteBeadStatusReviewTerminalBlock
			report.Detail = "pre-close review: terminal " + terminalClass
			out.Report = report
			out.Approved = false
			return out
		}
		report.Status = ExecuteBeadStatusReviewBlock
		report.Detail = "pre-close review: BLOCK (flagged for human)"
		out.Approved = false
	}
	if reviewRes.Verdict == VerdictBlock || reviewRes.Verdict == VerdictRequestChanges {
		_ = applyReviewTriageDecision(in.Store, in.Bead.ID, in.Assignee, now().UTC(), report.Tier)

		// Schedule one bounded repair cycle for review_fixable_gap. The cycle is
		// capped at one per result_rev: if a prior review-fixable-gap event already
		// exists for this result_rev, no second repair is scheduled.
		if countPriorRepairCycles(in.Store, in.Bead.ID, report.ResultRev) == 0 {
			groupID := ""
			if reviewGroup != nil {
				groupID = reviewGroup.Bundle.GroupID
			}
			repairCtx := RepairContextFromReviewGroup{
				ReviewGroupID:   groupID,
				ResultRev:       report.ResultRev,
				ImplActualPower: report.ActualPower,
				Rationale:       reviewRes.Rationale,
			}
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      ReviewFixableGapEventKind,
				Summary:   "review_fixable_gap",
				Body:      reviewFixableGapEventBody(repairCtx),
				Actor:     in.Assignee,
				Source:    "ddx agent execute-loop",
				CreatedAt: now().UTC(),
			})
			out.RepairContext = &repairCtx
		}
	}

	out.Report = report
	return out
}

func runPreCloseReviewGroup(ctx context.Context, reviewer BeadReviewer, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error) {
	if reviewer == nil {
		return nil, fmt.Errorf("review-group: reviewer required")
	}
	if groupReviewer, ok := reviewer.(reviewGroupReviewer); ok {
		return groupReviewer.ReviewGroup(ctx, beadID, resultRev, impl)
	}
	result, err := reviewer.ReviewBead(ctx, beadID, resultRev, impl)
	group := &ReviewGroupResult{
		BeadID:    beadID,
		ResultRev: resultRev,
		Slots: []ReviewGroupSlotResult{
			{
				ReviewerIndex: 0,
				Result:        result,
			},
		},
	}
	if err != nil {
		group.Slots[0].Error = err.Error()
	}
	return group, err
}

func reducePreCloseReviewGroup(group *ReviewGroupResult) (*ReviewResult, error) {
	if group == nil || len(group.Slots) == 0 {
		return nil, fmt.Errorf("review-group: no review slots")
	}
	var firstApprove *ReviewResult
	var firstNonApprove *ReviewResult
	var firstResult *ReviewResult
	for _, slot := range group.Slots {
		if slot.Result != nil && firstResult == nil {
			firstResult = slot.Result
		}
		if slot.Error != "" && slot.Result == nil {
			if firstResult != nil {
				return firstResult, fmt.Errorf("review-group slot %d: %s", slot.ReviewerIndex, slot.Error)
			}
			return nil, fmt.Errorf("review-group slot %d: %s", slot.ReviewerIndex, slot.Error)
		}
		if slot.Result == nil {
			if firstResult != nil {
				return firstResult, fmt.Errorf("review-group slot %d: missing review result", slot.ReviewerIndex)
			}
			return nil, fmt.Errorf("review-group slot %d: missing review result", slot.ReviewerIndex)
		}
		res := slot.Result
		switch res.Verdict {
		case VerdictApprove:
			if !reviewResultHasACEvidence(res) {
				return res, fmt.Errorf("%w: approve verdict without per-AC evidence", ErrReviewVerdictUnparseable)
			}
			if firstApprove == nil {
				firstApprove = res
			}
		case VerdictRequestChanges, VerdictBlock:
			if firstNonApprove == nil {
				firstNonApprove = res
			}
		default:
			if firstNonApprove == nil {
				firstNonApprove = res
			}
		}
	}
	if firstNonApprove != nil {
		return firstNonApprove, nil
	}
	return firstApprove, nil
}

func reviewResultHasACEvidence(res *ReviewResult) bool {
	if res == nil || res.Verdict != VerdictApprove {
		return false
	}
	if len(res.PerAC) == 0 {
		return false
	}
	for _, ac := range res.PerAC {
		if strings.TrimSpace(ac.Evidence) == "" {
			return false
		}
	}
	return true
}

// applyReviewTriageDecision runs the triage policy after a BLOCK or
// REQUEST_CHANGES review verdict has been recorded for `beadID`. It reads
// prior review events to compute the BLOCK count, biases the result toward
// re-attempt when the latest BLOCK was paired with a degraded reviewer
// (kind:review-pairing-degraded), then applies the chosen action by writing
// metadata, labels, and a kind:triage-decision event.
//
// Errors from the underlying store are best-effort: callers continue
// regardless so a triage-side failure cannot strand a bead in an
// inconsistent review state. This callout previously lived as a method on
// ExecuteBeadWorker; the C3
// extraction (ddx-a921ff01) moves it inside the post-merge-review pipeline
// driven by RunPostMergeReview / try.Attempt.
func applyReviewTriageDecision(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, currentTier string) error {
	events, err := store.Events(beadID)
	if err != nil {
		return err
	}

	var blockTimestamps []time.Time
	var pairingDegraded []time.Time
	for _, ev := range events {
		switch {
		case ev.Kind == "review" && strings.TrimSpace(ev.Summary) == "BLOCK":
			blockTimestamps = append(blockTimestamps, ev.CreatedAt)
		case ev.Kind == ReviewPairingDegradedEventKind:
			pairingDegraded = append(pairingDegraded, ev.CreatedAt)
		}
	}

	// History excludes the current attempt. The just-appended BLOCK (if any)
	// is the last entry in blockTimestamps and must not be counted as prior
	// history per triage.Decide()'s contract.
	historyCount := len(blockTimestamps)
	if historyCount > 0 {
		historyCount--
	}
	history := make([]triage.FailureMode, 0, historyCount)
	for i := 0; i < historyCount; i++ {
		history = append(history, triage.FailureModeReviewBlock)
	}

	policy := triage.DefaultPolicy()
	action := policy.Decide(beadID, triage.FailureModeReviewBlock, history)

	pairedDegraded := latestBlockPairedDegraded(blockTimestamps, pairingDegraded)
	if pairedDegraded && action == triage.ActionEscalateTier {
		action = triage.ActionReAttemptWithContext
	}

	return applyTriageAction(store, beadID, actor, now, action, currentTier, pairedDegraded)
}

// latestBlockPairedDegraded reports whether the most recent BLOCK event was
// paired with a review-pairing-degraded event from the same attempt window
// (between the previous BLOCK and this one).
func latestBlockPairedDegraded(blocks, pairing []time.Time) bool {
	if len(blocks) == 0 || len(pairing) == 0 {
		return false
	}
	latest := blocks[len(blocks)-1]
	var prev time.Time
	if len(blocks) >= 2 {
		prev = blocks[len(blocks)-2]
	}
	for _, t := range pairing {
		if t.After(prev) && !t.After(latest) {
			return true
		}
	}
	return false
}

// applyTriageAction performs the side effects for a chosen Action: writes a
// tier-pin hint into bead.Extra (escalate_tier), appends a needs_human label
// (needs_human), and always records a kind:triage-decision event.
func applyTriageAction(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, action triage.Action, currentTier string, pairedDegraded bool) error {
	body := map[string]any{
		"action": string(action),
		"mode":   string(triage.FailureModeReviewBlock),
	}
	if pairedDegraded {
		body["pairing_degraded"] = true
	}

	switch action {
	case triage.ActionEscalateTier:
		nextTier := nextEscalatedTier(currentTier)
		body["tier_hint"] = string(nextTier)
		_ = store.Update(beadID, func(b *bead.Bead) {
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			b.Extra[TriageTierHintKey] = string(nextTier)
		})
	case triage.ActionNeedsHuman:
		_ = store.Update(beadID, func(b *bead.Bead) {
			if !HasBeadLabel(b.Labels, TriageNeedsHumanLabel) {
				b.Labels = append(b.Labels, TriageNeedsHumanLabel)
			}
		})
	}

	bodyJSON, _ := json.Marshal(body)
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      triageDecisionEventKind,
		Summary:   string(triage.FailureModeReviewBlock) + ": " + string(action),
		Body:      string(bodyJSON),
		Actor:     actor,
		Source:    "ddx agent execute-loop",
		CreatedAt: now,
	})
}

// nextEscalatedTier returns the next tier above `current`. An unrecognised or
// empty current tier defaults to standard; smart is its own ceiling.
func nextEscalatedTier(current string) escalation.ModelTier {
	switch escalation.ModelTier(current) {
	case escalation.TierCheap:
		return escalation.TierStandard
	case escalation.TierStandard:
		return escalation.TierSmart
	case escalation.TierSmart:
		return escalation.TierSmart
	default:
		return escalation.TierStandard
	}
}

// countPriorRepairCycles returns the number of review-fixable-gap events
// already recorded for beadID with the given resultRev. Used to enforce the
// one-cycle-per-result-rev repair cap.
func countPriorRepairCycles(store ExecuteBeadLoopStore, beadID, resultRev string) int {
	events, _ := store.Events(beadID)
	n := 0
	for _, ev := range events {
		if ev.Kind == ReviewFixableGapEventKind && strings.Contains(ev.Body, "result_rev="+resultRev) {
			n++
		}
	}
	return n
}

// reviewFixableGapEventBody builds the structured body for a
// ReviewFixableGapEventKind event. The body encodes review_group_id (when
// known), result_rev, impl_actual_power, and the reviewer's rationale so the
// repair executor has enough context to target its next attempt.
func reviewFixableGapEventBody(rc RepairContextFromReviewGroup) string {
	parts := make([]string, 0, 4)
	if rc.ReviewGroupID != "" {
		parts = append(parts, "review_group_id="+rc.ReviewGroupID)
	}
	parts = append(parts, "result_rev="+rc.ResultRev)
	parts = append(parts, fmt.Sprintf("impl_actual_power=%d", rc.ImplActualPower))
	if rc.Rationale != "" {
		rationale := rc.Rationale
		if len(rationale) > 500 {
			rationale = rationale[:500] + "..."
		}
		parts = append(parts, "rationale="+rationale)
	}
	return strings.Join(parts, "\n")
}
