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
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/DocumentDrivenDX/ddx/internal/triage"
)

// triageDecisionEventKind is the event kind used to record the triage
// policy's chosen action after a non-APPROVE review verdict.
const triageDecisionEventKind = "triage-decision"

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
// RepairContext is non-nil when the BLOCK verdict was classified as a fixable
// gap and exactly one repair cycle has been scheduled for this result_rev.
// The caller should run a repair attempt with MinPower above
// RepairContext.ImplementerActualPower while preserving all explicit
// harness/model/provider/profile pins from the original attempt.
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
// active, the work asks the review coordinator to evaluate the
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

	// AC 1: Pre-dispatch cost cap check — enforce max_billable_usd before each
	// adversarial reviewer invocation. If the drain-level budget is already
	// exhausted, classify as review-error: cost_cap_exceeded and feed the
	// review_max_retries / review-manual-required retry path so the bead is
	// left open and not silently closed.
	if capTracker := in.ReviewCostCap; capTracker != nil {
		if detail, capped := capTracker.Tripped(); capped {
			class := evidence.OutcomeReviewCostCapExceeded
			prior := CountPriorReviewErrors(in.Store, in.Bead.ID, report.ResultRev)
			attemptCount := prior + 1
			maxRetries := in.Rcfg.ReviewMaxRetries()
			if maxRetries <= 0 {
				maxRetries = DefaultReviewMaxRetries
			}
			body := ReviewErrorEventBody(class, attemptCount, report.ResultRev, detail)
			if attemptCount >= maxRetries {
				_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
					Kind:      "review-manual-required",
					Summary:   class,
					Body:      body,
					Actor:     in.Assignee,
					Source:    "ddx work",
					CreatedAt: now().UTC(),
				})
				applyReviewOperatorRequiredParking(
					in.Store,
					in.Bead.ID,
					in.Assignee,
					now().UTC(),
					"review cost cap exhausted: "+class,
					"review cost cap requires operator decision",
					"raise max_billable_usd or reset the session budget before retrying",
				)
			} else {
				_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
					Kind:      "review-error",
					Summary:   class,
					Body:      body,
					Actor:     in.Assignee,
					Source:    "ddx work",
					CreatedAt: now().UTC(),
				})
			}
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-close review: " + class
			out.Report = report
			out.Approved = false
			return out
		}
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
						Source:    "ddx work",
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
		reviewerIndex, slotScoped := reviewErrorReviewerIndex(reviewGroup, reviewRes)
		prior := CountPriorReviewErrors(in.Store, in.Bead.ID, report.ResultRev)
		if slotScoped {
			prior = CountPriorReviewErrorsForSlot(in.Store, in.Bead.ID, report.ResultRev, reviewerIndex)
		}
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
			body := ReviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error())
			if slotScoped {
				body = ReviewErrorEventBodyForSlot(class, attemptCount, report.ResultRev, reviewerIndex, reviewErr.Error())
			}
			body = AppendEventSummary(body, errSummary)
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-manual-required",
				Summary:   class,
				Body:      body,
				Actor:     in.Assignee,
				Source:    "ddx work",
				CreatedAt: now().UTC(),
			})
			applyReviewOperatorRequiredParking(
				in.Store,
				in.Bead.ID,
				in.Assignee,
				now().UTC(),
				"review retry budget exhausted: "+class,
				"review error retry budget requires operator decision",
				"review the pre-close review failure and accept, retry, block, or cancel",
			)
		} else {
			body := ReviewErrorEventBody(class, attemptCount, report.ResultRev, reviewErr.Error())
			if slotScoped {
				body = ReviewErrorEventBodyForSlot(class, attemptCount, report.ResultRev, reviewerIndex, reviewErr.Error())
			}
			body = AppendEventSummary(body, errSummary)
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-error",
				Summary:   class,
				Body:      body,
				Actor:     in.Assignee,
				Source:    "ddx work",
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
			Source:    "ddx work",
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
			Source:    "ddx work",
			CreatedAt: now().UTC(),
		})
		report.Status = ExecuteBeadStatusReviewRequestChanges
		report.Detail = "pre-close review: REQUEST_CHANGES"
		out.Approved = false
	case VerdictBlock:
		rationale := strings.TrimSpace(reviewRes.Rationale)
		classification := ClassifyReviewFindings(reviewRes)
		if rationale == "" && len(classification.Evidence) == 0 {
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-malfunction",
				Summary:   "BLOCK without rationale",
				Body:      AppendEventSummary(ReviewEventBody("BLOCK without rationale", "", artifactPath), reviewSummary),
				Actor:     in.Assignee,
				Source:    "ddx work",
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
			Source:    "ddx work",
			CreatedAt: now().UTC(),
		})
		if classification.Class == ReviewFindingClassMalfunction {
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "review-malfunction",
				Summary:   ReviewFindingClassMalfunction,
				Body:      AppendEventSummary(classification.Reason, reviewSummary),
				Actor:     in.Assignee,
				Source:    "ddx work",
				CreatedAt: now().UTC(),
			})
			report.Status = ExecuteBeadStatusReviewMalfunction
			report.Detail = "pre-close review: " + ReviewFindingClassMalfunction
			out.Report = report
			out.Approved = false
			return out
		}
		if terminalClass := classifyTerminalReviewBlock(reviewRes); terminalClass != "" {
			applyTerminalReviewBlock(in.Store, in.Bead.ID, in.Assignee, now().UTC(), terminalClass, report.ResultRev)
			report.Status = ExecuteBeadStatusReviewTerminalBlock
			report.Detail = "pre-close review: terminal " + terminalClass
			out.Report = report
			out.Approved = false
			return out
		}
		// Non-terminal BLOCK: schedule one bounded repair cycle (review_fixable_gap).
		// If a repair cycle has already been scheduled for this result_rev, fall
		// through to the regular BLOCK triage path so the policy cannot loop.
		if classification.AutomatedRepairEligible && !hasReviewFixableGapRepairScheduled(in.Store, in.Bead.ID, report.ResultRev) {
			groupID := ""
			if reviewGroup != nil {
				groupID = reviewGroup.Bundle.GroupID
			}
			repairCtx := scheduleReviewFixableGapRepair(
				in.Store, in.Bead.ID, in.Assignee, now().UTC(),
				groupID, report.ResultRev,
				strings.TrimSpace(reviewRes.Rationale),
				report.ActualPower,
			)
			report.Status = ExecuteBeadStatusReviewFixableGap
			report.Detail = "pre-close review: review_fixable_gap"
			out.Report = report
			out.Approved = false
			out.RepairContext = repairCtx
			return out
		}
		report.Status = ExecuteBeadStatusReviewBlock
		report.Detail = "pre-close review: BLOCK (flagged for human)"
		out.Approved = false
	case VerdictRequestClarification:
		// Reviewer cannot adjudicate needs-judgment AC without operator input.
		// Park the bead to proposed (operator lane) without blocking the queue —
		// unlike BLOCK, this does not trigger the automated repair cycle or triage.
		rationale := strings.TrimSpace(reviewRes.Rationale)
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      ReviewRequestClarificationEventKind,
			Summary:   "REQUEST_CLARIFICATION",
			Body:      AppendEventSummary(ReviewEventBody("REQUEST_CLARIFICATION", rationale, artifactPath), reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx work",
			CreatedAt: now().UTC(),
		})
		_ = in.Store.ParkToProposed(in.Bead.ID, bead.ParkReviewRequestClarification, nil)
		report.Status = ExecuteBeadStatusReviewRequestClarification
		report.Detail = "pre-close review: REQUEST_CLARIFICATION"
		out.Approved = false
	}
	if reviewRes.Verdict == VerdictBlock || reviewRes.Verdict == VerdictRequestChanges {
		_ = applyReviewTriageDecision(in.Store, in.Bead.ID, in.Assignee, now().UTC(), report.PowerClass)
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

func reviewErrorReviewerIndex(group *ReviewGroupResult, res *ReviewResult) (int, bool) {
	if group == nil || len(group.Slots) < 2 {
		return 0, false
	}
	if res != nil {
		for _, slot := range group.Slots {
			if slot.Result == res {
				return slot.ReviewerIndex, true
			}
		}
		return res.ReviewerIndex, true
	}
	for _, slot := range group.Slots {
		if slot.Error != "" {
			return slot.ReviewerIndex, true
		}
	}
	return 0, true
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
func applyReviewTriageDecision(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, currentPowerClass string) error {
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
	if pairedDegraded && action == triage.ActionEscalatePower {
		action = triage.ActionReAttemptWithContext
	}

	return applyTriageAction(store, beadID, actor, now, action, currentPowerClass, pairedDegraded)
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
// powerClass-pin hint into bead.Extra (escalate_power), moves the bead to proposed
// for operator-required outcomes, and always records a triage-decision event.
func applyTriageAction(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, action triage.Action, currentPowerClass string, pairedDegraded bool) error {
	body := map[string]any{
		"action": string(action),
		"mode":   string(triage.FailureModeReviewBlock),
	}
	if pairedDegraded {
		body["pairing_degraded"] = true
	}

	switch action {
	case triage.ActionEscalatePower:
		nextPowerClass := nextEscalatedPowerClass(currentPowerClass)
		body["power_hint"] = string(nextPowerClass)
		_ = store.Update(context.Background(), beadID, func(b *bead.Bead) {
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			b.Extra[TriagePowerHintKey] = string(nextPowerClass)
		})
	case triage.ActionOperatorRequired:
		if err := store.ParkToProposed(beadID, bead.ParkPostReviewMalfunction, func(b *bead.Bead) {
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			delete(b.Extra, TriagePowerHintKey)
			// Migration-only cleanup: defensive removal for legacy rows that escaped
			// the lifecycle migration or arrived via external import.
			b.Labels = removeBeadLabels(b.Labels, TriageNeedsHumanLabel, bead.LabelNeedsHuman, bead.LabelNeedsInvestigation)
			clearReviewTriageClaimMetadata(b)
			bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
				Reason:          "review BLOCK triage reached operator-required rung",
				Since:           now.UTC().Format(time.RFC3339),
				Source:          "ddx work",
				SuggestedAction: "review the blocked attempt and accept, split, block, or cancel",
				Summary:         "review BLOCK triage requires operator decision",
			})
		}); err != nil {
			return err
		}
	}

	bodyJSON, _ := json.Marshal(body)
	return store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      triageDecisionEventKind,
		Summary:   string(triage.FailureModeReviewBlock) + ": " + string(action),
		Body:      string(bodyJSON),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: now,
	})
}

func clearReviewTriageClaimMetadata(b *bead.Bead) {
	b.Owner = ""
	if b.Extra == nil {
		return
	}
	delete(b.Extra, "claimed-at")
	delete(b.Extra, "claimed-pid")
	delete(b.Extra, "claimed-machine")
	delete(b.Extra, "claimed-session")
	delete(b.Extra, "claimed-worktree")
	delete(b.Extra, "work-heartbeat-at")
}

// nextEscalatedPowerClass returns the next powerClass above `current`. An unrecognised or
// empty current powerClass defaults to standard; smart is its own ceiling.
func nextEscalatedPowerClass(current string) escalation.PowerClass {
	switch escalation.PowerClass(current) {
	case escalation.PowerCheap:
		return escalation.PowerStandard
	case escalation.PowerStandard:
		return escalation.PowerSmart
	case escalation.PowerSmart:
		return escalation.PowerSmart
	default:
		return escalation.PowerStandard
	}
}
