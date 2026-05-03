package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ConflictRecoveryDisposition is the routing decision RunConflictRecovery
// returns. Maps onto try.Disposition (Merged / Park / NeedsHuman) at the
// orchestrator boundary; the agent package keeps a local enum to avoid a
// try → agent → try import cycle.
type ConflictRecoveryDisposition int

const (
	// ConflictRecoveryMerged — auto-recovery (or focused-resolve agent)
	// landed the preserved iteration; bead is closed.
	ConflictRecoveryMerged ConflictRecoveryDisposition = iota
	// ConflictRecoveryPark — recovery failed; bead is unclaimed and parked
	// under cooldown as land_conflict_unresolvable.
	ConflictRecoveryPark
	// ConflictRecoveryNeedsHuman — focused-resolve agent signalled the
	// conflict requires human judgment; bead parks as
	// land_conflict_needs_human.
	ConflictRecoveryNeedsHuman
)

// ConflictAutoRecoverFn matches the signature of LandConflictAutoRecover
// (the 3-way ort -X ours path). Callers may inject a stub for tests.
type ConflictAutoRecoverFn func(wd, preserveRef string, gitOps LandingGitOps) (string, error)

// ConflictResolverFn matches the worker's optional focused-resolve callout.
// Returns the new merged tip SHA on success; isBlocking=true requests the
// land_conflict_needs_human escalation path.
type ConflictResolverFn func(ctx context.Context, beadID, preserveRef, projectRoot string) (newTip string, isBlocking bool, err error)

// ConflictRecoveryStore is the narrow subset of bead-tracker operations the
// recovery path needs. ExecuteBeadLoopStore satisfies it.
type ConflictRecoveryStore interface {
	AppendEvent(beadID string, ev bead.BeadEvent) error
	CloseWithEvidence(beadID, sessionID, sha string) error
	Unclaim(beadID string) error
	SetExecutionCooldown(beadID string, until time.Time, status, detail string) error
}

// ConflictRecoveryInput bundles the data RunConflictRecovery needs from its
// caller — historically the inline branch in execute_bead_loop.go that ran
// the recovery and then closed-or-parked the bead.
type ConflictRecoveryInput struct {
	Bead        bead.Bead
	Report      ExecuteBeadReport
	ProjectRoot string
	// AutoRecover is the 3-way ort -X ours auto-merge function. When nil,
	// LandConflictAutoRecover is used.
	AutoRecover ConflictAutoRecoverFn
	// ConflictResolver, when non-nil, is the focused-resolve agent callout
	// invoked after AutoRecover fails.
	ConflictResolver ConflictResolverFn
	Store            ConflictRecoveryStore
	Assignee         string
	Now              func() time.Time
	// Cooldown is the parkUntil offset applied when recovery fails. The
	// loop passes LandConflictCooldown (15 min); kept as a parameter rather
	// than hardcoded so the loop owns the policy knob.
	Cooldown time.Duration
}

// ConflictRecoveryOutput carries the recovery state machine's decision back
// to the caller. Report is the (possibly updated) ExecuteBeadReport;
// Disposition selects the routing decision the loop applies (Merged → close
// done, Park / NeedsHuman → cooldown set). Store errors are surfaced via
// StoreErrOp + StoreErr so callers can keep the existing
// handleOutcomeStoreError pattern unchanged.
type ConflictRecoveryOutput struct {
	Report      ExecuteBeadReport
	Disposition ConflictRecoveryDisposition
	StoreErrOp  string
	StoreErr    error
}

// ShouldAttemptConflictRecovery returns true when the loop should try to
// reuse a preserved iteration ref before discarding it. Requires a non-empty
// projectRoot so recovery git commands have a working directory, and a
// non-empty PreserveRef to actually have something to recover.
func ShouldAttemptConflictRecovery(report ExecuteBeadReport, projectRoot string) bool {
	if report.PreserveRef == "" || projectRoot == "" {
		return false
	}
	switch report.Status {
	case ExecuteBeadStatusLandConflict:
		return true
	case ExecuteBeadStatusExecutionFailed:
		// Attempt recovery when a commit was produced before the failure (e.g.
		// agent timed out mid-run). Both BaseRev and ResultRev must be set and
		// differ, because an empty BaseRev means we cannot confirm the agent
		// made any progress beyond the starting point.
		return report.BaseRev != "" && report.ResultRev != "" && report.ResultRev != report.BaseRev
	}
	return false
}

// RunConflictRecovery attempts to recover a preserved iteration before the
// loop parks the bead as failed. Step 1: 3-way ort auto-merge (-X ours).
// Step 2 (if step 1 fails and ConflictResolver is set): focused
// conflict-resolve agent. On success the bead is closed and Disposition is
// Merged; on failure the bead is unclaimed and parked under in.Cooldown with
// Disposition=Park (land_conflict_unresolvable) or Disposition=NeedsHuman
// (land_conflict_needs_human).
//
// This is the C4 (ddx-358f2457) extraction of the runConflictRecovery
// method on ExecuteBeadWorker plus the surrounding success/park bookkeeping
// previously inline in execute_bead_loop.go. The loop now sees a structured
// outcome and routes it via the same handleOutcomeStoreError path used for
// the C3 RunPostMergeReview extraction.
func RunConflictRecovery(ctx context.Context, in ConflictRecoveryInput) ConflictRecoveryOutput {
	report := in.Report
	out := ConflictRecoveryOutput{Report: report}

	now := in.Now
	if now == nil {
		now = time.Now
	}

	autoFn := in.AutoRecover
	if autoFn == nil {
		autoFn = LandConflictAutoRecover
	}

	var gitOps LandingGitOps = RealLandingGitOps{}

	newTip, autoErr := autoFn(in.ProjectRoot, report.PreserveRef, gitOps)
	if autoErr == nil && newTip != "" {
		_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
			Kind:      "land-conflict-auto-recovered",
			Summary:   "preserved iteration auto-recovered onto current tip via ort",
			Body:      fmt.Sprintf("preserve_ref=%s\nnew_tip=%s", report.PreserveRef, newTip),
			Actor:     in.Assignee,
			Source:    "ddx agent execute-loop",
			CreatedAt: now().UTC(),
		})
		report.Status = ExecuteBeadStatusSuccess
		report.ResultRev = newTip
		report.Detail = "auto-recovered: merged preserved iteration onto current tip via ort"
		if err := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); err != nil {
			out.StoreErrOp = "CloseWithEvidence"
			out.StoreErr = err
			out.Report = report
			return out
		}
		out.Report = report
		out.Disposition = ConflictRecoveryMerged
		return out
	}

	if in.ConflictResolver != nil {
		resolvedTip, isBlocking, resolveErr := in.ConflictResolver(ctx, in.Bead.ID, report.PreserveRef, in.ProjectRoot)
		if resolveErr == nil && resolvedTip != "" {
			_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
				Kind:      "land-conflict-auto-recovered",
				Summary:   "preserved iteration resolved by focused conflict-resolve agent",
				Body:      fmt.Sprintf("preserve_ref=%s\nnew_tip=%s", report.PreserveRef, resolvedTip),
				Actor:     in.Assignee,
				Source:    "ddx agent execute-loop",
				CreatedAt: now().UTC(),
			})
			report.Status = ExecuteBeadStatusSuccess
			report.ResultRev = resolvedTip
			report.Detail = "auto-recovered: focused conflict-resolve agent landed preserved iteration"
			if err := in.Store.CloseWithEvidence(in.Bead.ID, report.SessionID, report.ResultRev); err != nil {
				out.StoreErrOp = "CloseWithEvidence"
				out.StoreErr = err
				out.Report = report
				return out
			}
			out.Report = report
			out.Disposition = ConflictRecoveryMerged
			return out
		}
		if isBlocking {
			report.Status = ExecuteBeadStatusLandConflictNeedsHuman
		} else {
			report.Status = ExecuteBeadStatusLandConflictUnresolvable
		}
	} else {
		report.Status = ExecuteBeadStatusLandConflictUnresolvable
	}

	autoErrMsg := ""
	if autoErr != nil {
		autoErrMsg = autoErr.Error()
	}
	body, mErr := json.Marshal(map[string]any{
		"preserve_ref":     report.PreserveRef,
		"base_rev":         report.BaseRev,
		"result_rev":       report.ResultRev,
		"session_id":       report.SessionID,
		"auto_merge_error": autoErrMsg,
	})
	bodyStr := report.PreserveRef
	if mErr == nil {
		bodyStr = string(body)
	}
	eventKind := "land-conflict-unresolvable"
	if report.Status == ExecuteBeadStatusLandConflictNeedsHuman {
		eventKind = "land-conflict-needs-human"
	}
	_ = in.Store.AppendEvent(in.Bead.ID, bead.BeadEvent{
		Kind:      eventKind,
		Summary:   "preserved iteration could not be auto-recovered; parked for operator",
		Body:      bodyStr,
		Actor:     in.Assignee,
		Source:    "ddx agent execute-loop",
		CreatedAt: now().UTC(),
	})
	report.Detail = report.Status + ": preserve_ref=" + report.PreserveRef

	if err := in.Store.Unclaim(in.Bead.ID); err != nil {
		out.StoreErrOp = "Unclaim"
		out.StoreErr = err
		out.Report = report
		return out
	}
	parkUntil := now().UTC().Add(in.Cooldown)
	if err := in.Store.SetExecutionCooldown(in.Bead.ID, parkUntil, report.Status, report.Detail); err != nil {
		out.StoreErrOp = "SetExecutionCooldown"
		out.StoreErr = err
		out.Report = report
		return out
	}
	report.RetryAfter = parkUntil.Format(time.RFC3339)

	out.Report = report
	if report.Status == ExecuteBeadStatusLandConflictNeedsHuman {
		out.Disposition = ConflictRecoveryNeedsHuman
	} else {
		out.Disposition = ConflictRecoveryPark
	}
	return out
}
