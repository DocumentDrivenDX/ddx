// Package testfixtures provides deterministic, reusable fixtures for
// behavioral end-to-end tests of the execute-bead loop. The fixtures
// install into ExecuteBeadLoopRuntime / ExecuteBeadWorker via the
// existing test-injection seams (ExecuteBeadWorker.Executor and
// ExecuteBeadWorker.Reviewer) so callers do not need to reinvent the
// review-retry plumbing per test.
//
// See SD-024 / TD-024 §Bead 6.5.
package testfixtures

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// ReviewFailureRunner produces a deterministic Executor + Reviewer pair
// that drive the "N reviewer failures, then 1 success" scenario against
// a real bead store. Beads 7-9 of SD-024 use this fixture to verify
// that ReviewMaxRetries (and the related threshold knobs) wired through
// .ddx/config.yaml reach the running loop.
//
// Behavior:
//
//   - The executor returned by Executor() always reports
//     ExecuteBeadStatusSuccess with ResultRev fixed to r.ResultRev.
//   - The reviewer returned by Reviewer() returns a typed reviewer
//     error for the first FailUntilCall invocations, then returns
//     a clean APPROVE verdict on every subsequent invocation.
//
// The runner is stateful: invocation counters increment on each call,
// so the same instance must not be shared across parallel subtests.
type ReviewFailureRunner struct {
	// ResultRev is stamped on every executor success report. When empty
	// the runner uses a fixed sentinel value so review-error events
	// scoped by result_rev still group correctly.
	ResultRev string

	// FailureClass is the canonical review-error class returned for
	// failed reviews. Empty falls back to evidence.OutcomeReviewProviderEmpty.
	FailureClass string

	// FailUntilCall is the number of reviewer invocations that fail
	// before the runner switches to APPROVE. Reviewer calls
	// 1..FailUntilCall return a typed error; calls FailUntilCall+1
	// onward return a clean APPROVE.
	FailUntilCall int

	reviewCalls atomic.Int32
	execCalls   atomic.Int32
}

const defaultRunnerResultRev = "deadbeef"

func (r *ReviewFailureRunner) resultRev() string {
	if r.ResultRev == "" {
		return defaultRunnerResultRev
	}
	return r.ResultRev
}

func (r *ReviewFailureRunner) failureClass() string {
	if r.FailureClass == "" {
		return evidence.OutcomeReviewProviderEmpty
	}
	return r.FailureClass
}

// Executor returns an ExecuteBeadExecutor that stamps a successful
// report on every invocation. The report carries ResultRev and a
// stable SessionID derived from the executor's call count so review
// retry counters (which scope by result_rev) behave deterministically.
func (r *ReviewFailureRunner) Executor() agent.ExecuteBeadExecutor {
	return agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		n := r.execCalls.Add(1)
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			SessionID: fmt.Sprintf("rfr-sess-%d", n),
			ResultRev: r.resultRev(),
		}, nil
	})
}

// Reviewer returns a BeadReviewer that fails the first FailUntilCall
// invocations with a typed reviewer error, then returns APPROVE on
// every subsequent invocation. The error path mirrors production
// failingReviewer shape (FEAT-022 §12 taxonomy): a non-nil error plus
// a *ReviewResult whose Error field carries the canonical class.
func (r *ReviewFailureRunner) Reviewer() agent.BeadReviewer {
	return agent.BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*agent.ReviewResult, error) {
		n := int(r.reviewCalls.Add(1))
		if n <= r.FailUntilCall {
			class := r.failureClass()
			return &agent.ReviewResult{
					Verdict:   agent.VerdictBlock,
					Error:     class,
					ResultRev: resultRev,
				}, fmt.Errorf("review-failure-runner: %s: %w", class,
					errors.New("simulated reviewer failure"))
		}
		return &agent.ReviewResult{
			Verdict:   agent.VerdictApprove,
			Rationale: "review-failure-runner: APPROVE",
			ResultRev: resultRev,
		}, nil
	})
}

// ReviewCalls returns the cumulative number of reviewer invocations.
func (r *ReviewFailureRunner) ReviewCalls() int {
	return int(r.reviewCalls.Load())
}

// ExecCalls returns the cumulative number of executor invocations.
func (r *ReviewFailureRunner) ExecCalls() int {
	return int(r.execCalls.Load())
}
