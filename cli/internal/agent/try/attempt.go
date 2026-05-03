// Package try — Attempt is the C2 shell that wraps the existing
// agent.ExecuteBeadWithConfig + agent.LandBeadResult + (optional)
// agent.DefaultBeadReviewer pipeline and returns a try.Outcome via the C1
// ToOutcome adapter. It has no production callers at end of C2; the
// orchestrator continues to drive the legacy executor directly. Callers
// migrate in C3+.
package try

import (
	"context"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// AttemptOpts bundles the knobs Attempt needs to drive the legacy
// ExecuteBeadWithConfig + LandBeadResult + DefaultBeadReviewer pipeline.
//
// Tests inject Executor to supply a canned ExecuteBeadReport without
// touching git or a real harness. When Executor is non-nil it short-circuits
// the legacy pipeline entirely; the remaining fields are ignored.
type AttemptOpts struct {
	// Executor is an optional injection seam. When non-nil, Attempt delegates
	// to it instead of calling ExecuteBeadWithConfig + LandBeadResult.
	Executor agent.ExecuteBeadExecutor

	// ProjectRoot is the repo the bead executes in.
	ProjectRoot string

	// Rcfg is the resolved harness/model/profile config for the attempt.
	Rcfg config.ResolvedConfig

	// Runtime carries non-durable injection seams (BeadEvents, AgentRunner, …).
	Runtime agent.ExecuteBeadRuntime

	// GitOps is the worker-side git surface (worktree add/remove, head, …).
	GitOps agent.GitOps

	// OrchestratorGitOps is the parent-side git surface (UpdateRef).
	OrchestratorGitOps agent.OrchestratorGitOps

	// LandingOpts controls how LandBeadResult lands a completed worker result.
	LandingOpts agent.BeadLandingOptions

	// Reviewer, when non-nil, is invoked after a successful merge to validate
	// the commit against the bead's AC. When nil, post-merge review is skipped.
	Reviewer agent.BeadReviewer
}

// Attempt runs one bead attempt and returns a try.Outcome.
//
// When opts.Executor is non-nil, Attempt delegates to it and converts the
// returned ExecuteBeadReport via ToOutcome — the path covered by
// TestAttempt_WrapsLegacyExecutor.
//
// Otherwise Attempt drives the legacy pipeline:
//
//  1. agent.ExecuteBeadWithConfig — run the agent in an isolated worktree.
//  2. agent.LandBeadResult — merge or preserve, evaluate gates.
//  3. opts.Reviewer.ReviewBead (best effort) — optional post-merge review.
//
// The store argument is reserved for the C3+ migration where Attempt will
// own bead-tracker bookkeeping (claim, heartbeat, close). At C2 it is
// accepted but not yet used; the legacy loop still owns those calls.
func Attempt(ctx context.Context, store agent.ExecuteBeadLoopStore, beadID string, opts AttemptOpts) (Outcome, error) {
	_ = store // reserved for C3+ migration; legacy loop still owns store I/O.

	if opts.Executor != nil {
		report, err := opts.Executor.Execute(ctx, beadID)
		if err != nil {
			out := ToOutcome(report)
			if out.BeadID == "" {
				out.BeadID = beadID
			}
			return out, err
		}
		out := ToOutcome(report)
		if out.BeadID == "" {
			out.BeadID = beadID
		}
		return out, nil
	}

	res, execErr := agent.ExecuteBeadWithConfig(ctx, opts.ProjectRoot, beadID, opts.Rcfg, opts.Runtime, opts.GitOps)
	if execErr != nil && res == nil {
		return Outcome{BeadID: beadID}, execErr
	}

	if res != nil {
		landing, landErr := agent.LandBeadResult(opts.ProjectRoot, res, opts.OrchestratorGitOps, opts.LandingOpts)
		if landErr == nil {
			agent.ApplyLandingToResult(res, landing)
		} else if execErr == nil {
			execErr = landErr
		}
	}

	if opts.Reviewer != nil && res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
		// C3 (ddx-a921ff01): drive the full post-merge review state machine
		// — verdict events, close-on-APPROVE, reopen-on-REQUEST_CHANGES /
		// BLOCK, triage callout — via the extracted RunPostMergeReview that
		// replaces the 180-line inline review block in execute_bead_loop.go.
		// Store errors are swallowed at this layer because the legacy loop
		// (which currently owns claim/heartbeat/close bookkeeping) handles
		// them via handleOutcomeStoreError; the C3 shell predates that
		// migration (C7) and so cannot route the same way.
		report := agent.ExecuteBeadReport{
			BeadID:      beadID,
			AttemptID:   res.AttemptID,
			Harness:     res.Harness,
			Provider:    res.Provider,
			Model:       res.Model,
			ActualPower: res.ActualPower,
			SessionID:   res.SessionID,
			BaseRev:     res.BaseRev,
			ResultRev:   res.ResultRev,
		}
		_ = agent.RunPostMergeReview(ctx, agent.PostMergeReviewInput{
			Bead:        bead.Bead{ID: beadID},
			Report:      report,
			Reviewer:    opts.Reviewer,
			Store:       store,
			ProjectRoot: opts.ProjectRoot,
			Rcfg:        opts.Rcfg,
			Assignee:    opts.Rcfg.Assignee(),
			Now:         time.Now,
		})
	}

	report := reportFromResult(res)
	out := ToOutcome(report)
	if out.BeadID == "" {
		out.BeadID = beadID
	}
	return out, execErr
}

// reportFromResult projects the subset of ExecuteBeadResult fields that
// ToOutcome consumes. It is intentionally narrow — the loop's full report
// synthesis (tier, profile telemetry, escalation count) is not needed for
// the C2 shell and gets built out as callers migrate in C3+.
func reportFromResult(res *agent.ExecuteBeadResult) agent.ExecuteBeadReport {
	if res == nil {
		return agent.ExecuteBeadReport{}
	}
	return agent.ExecuteBeadReport{
		BeadID:             res.BeadID,
		AttemptID:          res.AttemptID,
		WorkerID:           res.WorkerID,
		Harness:            res.Harness,
		Provider:           res.Provider,
		Model:              res.Model,
		ActualPower:        res.ActualPower,
		Status:             res.Status,
		Detail:             res.Detail,
		SessionID:          res.SessionID,
		BaseRev:            res.BaseRev,
		ResultRev:          res.ResultRev,
		PreserveRef:        res.PreserveRef,
		NoChangesRationale: res.NoChangesRationale,
		CostUSD:            res.CostUSD,
		DurationMS:         int64(res.DurationMS),
	}
}
