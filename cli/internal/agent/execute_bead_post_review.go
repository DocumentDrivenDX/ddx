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

// PostMergeReviewInput bundles the data RunPostMergeReview needs from its
// caller — historically the execute-bead loop body, now the per-attempt
// pipeline owned by try.Attempt (ddx-a921ff01 C3). The fields mirror what
// the inlined block read from the worker, the candidate bead, the report,
// and the loop runtime.
type PostMergeReviewInput struct {
	Bead        bead.Bead
	Report      ExecuteBeadReport
	Reviewer    BeadReviewer
	Store       ExecuteBeadLoopStore
	ProjectRoot string
	Rcfg        config.ResolvedConfig
	NoReview    bool
	Log         io.Writer
	Assignee    string
	Now         func() time.Time
}

// PostMergeReviewOutput carries the review state machine's decision back to
// the caller. Report is the (possibly updated) ExecuteBeadReport; Approved
// reports whether the caller should treat this attempt as a success. When
// the review path encountered a non-fatal Store error the caller is expected
// to route StoreErr through its outcome-error handler
// (handleOutcomeStoreError on ExecuteBeadWorker), keyed by StoreErrOp.
type PostMergeReviewOutput struct {
	Report     ExecuteBeadReport
	Approved   bool
	StoreErrOp string
	StoreErr   error
}

// RunPostMergeReview executes the post-merge review state machine for a
// successful bead attempt. When review is skipped (no reviewer, --no-review,
// or the review:skip label) the bead is closed and the function returns
// Approved=true. Otherwise the reviewer is invoked, its verdict (or error)
// drives the canonical event emissions (review / review-error /
// review-manual-required / review-malfunction), and on a non-APPROVE verdict
// the bead is reopened and the triage policy is consulted via the private
// applyReviewTriageDecision callout that previously lived as a method on
// ExecuteBeadWorker.
//
// Returns Approved=true on the APPROVE path and on the skip path; returns
// Approved=false on REQUEST_CHANGES, BLOCK, malformed BLOCK, or reviewer
// transport error. Store errors are surfaced via StoreErrOp + StoreErr so
// callers can keep the existing handleOutcomeStoreError pattern unchanged.
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

	reviewSkipped := in.Reviewer == nil || in.NoReview || HasBeadLabel(in.Bead.Labels, "review:skip")

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
	reviewRes, reviewErr := in.Reviewer.ReviewBead(ctx, in.Bead.ID, report.ResultRev, implRouting)
	if reviewErr != nil {
		// FEAT-022 §12+§14: classify the failure into the four-class
		// taxonomy and count prior review-error events scoped to the
		// current result_rev. On the Nth failure emit a terminal
		// review-manual-required event instead of yet another
		// review-error, parking the bead so a subsequent loop
		// iteration does NOT re-execute primary.
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
			// Park the bead with a long cooldown so subsequent
			// loop iterations do NOT re-pick it for primary
			// re-execution. Reviewer-failure invariant (§13) is
			// preserved: the bead is NOT closed.
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
		out.Report = report
		out.Approved = false
		return out
	}

	report.ReviewVerdict = string(reviewRes.Verdict)
	report.ReviewRationale = reviewRes.Rationale
	// Persist the full reviewer stream as an artifact so the
	// event body never carries the raw stream (ddx-f8a11202).
	// On error the artifact path is empty and the event body
	// still contains the short verdict summary; callers of this
	// loop can recover the full text from the reviewer session
	// log if the artifact write failed.
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
	switch reviewRes.Verdict {
	case VerdictApprove:
		// Approved: record the verdict event and then close.
		// Closure must land AFTER the review event so the
		// gate (ddx-e30e60a9) sees the terminal verdict.
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "APPROVE",
			Body:      AppendEventSummary(ReviewEventBody("APPROVE", reviewRes.Rationale, artifactPath), reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		if cerr := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); cerr != nil {
			out.Report = report
			out.StoreErrOp = "CloseWithEvidence"
			out.StoreErr = cerr
			return out
		}
	case VerdictRequestChanges:
		// Needs fixes: record the review verdict, then reopen
		// with findings in notes. The review event must land
		// even on non-approve paths so review-outcomes can
		// attribute the rejection to the originating tier.
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "REQUEST_CHANGES",
			Body:      AppendEventSummary(ReviewEventBody("REQUEST_CHANGES", reviewRes.Rationale, artifactPath), reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		reopenNotes := reviewRes.Rationale
		if reopenNotes == "" {
			reopenNotes = reviewRes.RawOutput
		}
		if reopenErr := in.Store.Reopen(in.Bead.ID, "review: REQUEST_CHANGES", reopenNotes); reopenErr != nil {
			out.Report = report
			out.StoreErrOp = "Reopen"
			out.StoreErr = reopenErr
			return out
		}
		report.Status = ExecuteBeadStatusReviewRequestChanges
		report.Detail = "post-merge review: REQUEST_CHANGES"
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
			report.Detail = "post-merge review: malformed BLOCK verdict (missing rationale)"
			report.ReviewRationale = ""
			out.Report = report
			out.Approved = false
			return out
		}
		// Cannot proceed: record the verdict, then reopen and
		// flag for human with BLOCK marker plus actionable rationale.
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "review",
			Summary:   "BLOCK",
			Body:      AppendEventSummary(rationale, reviewSummary),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		blockNotes := "REVIEW:BLOCK\n\n" + rationale
		if reopenErr := in.Store.Reopen(in.Bead.ID, "review: BLOCK", blockNotes); reopenErr != nil {
			out.Report = report
			out.StoreErrOp = "Reopen"
			out.StoreErr = reopenErr
			return out
		}
		report.Status = ExecuteBeadStatusReviewBlock
		report.Detail = "post-merge review: BLOCK (flagged for human)"
		out.Approved = false
	}
	// After the BLOCK / REQUEST_CHANGES branches have recorded
	// their verdict event and reopened the bead, consult the
	// triage policy for the next action (re-attempt, escalate
	// tier, or needs_human).
	if reviewRes.Verdict == VerdictBlock || reviewRes.Verdict == VerdictRequestChanges {
		_ = applyReviewTriageDecision(in.Store, in.Bead.ID, in.Assignee, now().UTC(), report.Tier)
	}

	out.Report = report
	return out
}

// applyReviewTriageDecision runs the triage policy after a BLOCK or
// REQUEST_CHANGES review verdict has been recorded for `beadID`. It reads
// prior review events to compute the BLOCK count, biases the result toward
// re-attempt when the latest BLOCK was paired with a degraded reviewer
// (kind:review-pairing-degraded), then applies the chosen action by writing
// metadata, labels, and a kind:triage-decision event.
//
// Errors from the underlying store are best-effort: callers continue
// regardless so a triage-side failure cannot strand a reopened bead. This
// callout previously lived as a method on ExecuteBeadWorker; the C3
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

// ApplyReviewTriageDecision is the package-exported entry point for callers
// outside the agent package (e.g. existing triage tests in agent_test that
// exercise the BLOCK ladder directly). It is a thin wrapper around the
// private applyReviewTriageDecision.
func ApplyReviewTriageDecision(store ExecuteBeadLoopStore, beadID, actor string, now time.Time, currentTier string) error {
	return applyReviewTriageDecision(store, beadID, actor, now, currentTier)
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
