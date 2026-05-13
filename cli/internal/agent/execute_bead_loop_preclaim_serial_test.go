package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadWorkerConcurrentPreClaimIntakeRunsOncePerBead is AC #1:
// With two concurrent workers on one ready bead, only the worker that wins
// Store.Claim invokes the model-backed PreClaimIntakeHook. The loser loses at
// Claim (picker.claim_race), never reaches intake, and moves on.
func TestExecuteBeadWorkerConcurrentPreClaimIntakeRunsOncePerBead(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	only := &bead.Bead{ID: "ddx-concurrent-intake", Title: "concurrent intake", Priority: 0}
	require.NoError(t, store.Create(only))

	var intakeCalls atomic.Int32

	// Intake hook blocks so that Worker B has time to lose the Claim race
	// before Worker A's intake returns.
	intakeStarted := make(chan struct{}, 1)
	intakeRelease := make(chan struct{})

	makeRuntime := func(assignee string) (config.ResolvedConfig, ExecuteBeadLoopRuntime) {
		opts := config.TestLoopConfigOpts{Assignee: assignee}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
		rt := ExecuteBeadLoopRuntime{
			Once: true,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				intakeCalls.Add(1)
				select {
				case intakeStarted <- struct{}{}:
				default:
				}
				select {
				case <-intakeRelease:
				case <-ctx.Done():
				}
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
		}
		return rcfg, rt
	}

	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
	})
	workerA := &ExecuteBeadWorker{Store: store, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: store, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-a")
		_, _ = workerA.Run(context.Background(), rcfg, rt)
	}()
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-b")
		_, _ = workerB.Run(context.Background(), rcfg, rt)
	}()

	// Wait for exactly one intake invocation to start, then release it.
	// If neither starts within 5s the test hangs on wg.Wait and fails via
	// t.Cleanup timeout rather than silently passing.
	select {
	case <-intakeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("intake hook never started — no worker claimed the bead")
	}
	close(intakeRelease)
	wg.Wait()

	assert.Equal(t, int32(1), intakeCalls.Load(),
		"only the claiming worker must enter intake; the Claim-loser must not run the model-backed hook")
}

// TestExecuteBeadWorkerConcurrentPreDispatchLintDoesNotDuplicateEvents is AC #2:
// Two concurrent workers on the same bead must not append duplicate
// bead-quality.lint events. With claim-first ordering, only the owning worker
// appends the lint event; the loser never reaches the lint gate.
func TestExecuteBeadWorkerConcurrentPreDispatchLintDoesNotDuplicateEvents(t *testing.T) {
	inner := bead.NewStore(t.TempDir())
	require.NoError(t, inner.Init())
	only := &bead.Bead{ID: "ddx-concurrent-lint", Title: "concurrent lint", Priority: 0}
	require.NoError(t, inner.Create(only))

	// Lint hook blocks to widen the concurrency window.
	lintStarted := make(chan struct{}, 1)
	lintRelease := make(chan struct{})

	makeRuntime := func(assignee string) (config.ResolvedConfig, ExecuteBeadLoopRuntime) {
		opts := config.TestLoopConfigOpts{Assignee: assignee}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
		rt := ExecuteBeadLoopRuntime{
			Once: true,
			PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
				select {
				case lintStarted <- struct{}{}:
				default:
				}
				select {
				case <-lintRelease:
				case <-ctx.Done():
				}
				return LintResult{Score: 9, Rationale: "ready"}, nil
			},
		}
		return rcfg, rt
	}

	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
	})
	workerA := &ExecuteBeadWorker{Store: inner, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: inner, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-a")
		_, _ = workerA.Run(context.Background(), rcfg, rt)
	}()
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-b")
		_, _ = workerB.Run(context.Background(), rcfg, rt)
	}()

	select {
	case <-lintStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("lint hook never started")
	}
	close(lintRelease)
	wg.Wait()

	events, err := inner.Events(only.ID)
	require.NoError(t, err)
	var lintEvents []bead.BeadEvent
	for _, ev := range events {
		if ev.Kind == "bead-quality.lint" {
			lintEvents = append(lintEvents, ev)
		}
	}
	assert.Equal(t, 1, len(lintEvents),
		"exactly one bead-quality.lint event must be appended; concurrent workers must not duplicate it")
}

// TestPreClaimIntakeRewriteRequiresOwnedReservation is AC #3:
// With actionable_but_rewritten intake, only the claiming worker applies
// description/acceptance rewrites. Concurrent losers cannot double-rewrite
// or append conflicting events because they never reach applyPreClaimIntakeRewrite.
func TestPreClaimIntakeRewriteRequiresOwnedReservation(t *testing.T) {
	inner := bead.NewStore(t.TempDir())
	require.NoError(t, inner.Init())

	original := &bead.Bead{
		ID:          "ddx-rewrite-concurrent",
		Title:       "rewrite ownership",
		Priority:    0,
		Description: "PROBLEM\noriginal problem\n\nROOT CAUSE\noriginal cause\n\nPROPOSED FIX\noriginal fix\n",
		Acceptance:  "1. original criterion\n2. name the test",
	}
	require.NoError(t, inner.Create(original))

	var rewriteCalls atomic.Int32
	rewriteStarted := make(chan struct{}, 1)
	rewriteRelease := make(chan struct{})

	makeRuntime := func(assignee string) (config.ResolvedConfig, ExecuteBeadLoopRuntime) {
		opts := config.TestLoopConfigOpts{Assignee: assignee}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))
		rt := ExecuteBeadLoopRuntime{
			Once: true,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				rewriteCalls.Add(1)
				select {
				case rewriteStarted <- struct{}{}:
				default:
				}
				select {
				case <-rewriteRelease:
				case <-ctx.Done():
				}
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeActionableButRewritten,
					Detail:  "safe refinement",
					Rewrite: PreClaimIntakeRewrite{
						Description:   "PROBLEM\noriginal problem\n\nROOT CAUSE\noriginal cause\n\nPROPOSED FIX\noriginal fix\n\nAdd explicit validation step.",
						Acceptance:    "1. original criterion\n2. name the test\n3. lefthook run pre-commit passes",
						ChangedFields: []string{"description", "acceptance"},
					},
				}, nil
			},
		}
		return rcfg, rt
	}

	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
	})
	workerA := &ExecuteBeadWorker{Store: inner, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: inner, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-a")
		_, _ = workerA.Run(context.Background(), rcfg, rt)
	}()
	go func() {
		defer wg.Done()
		rcfg, rt := makeRuntime("worker-b")
		_, _ = workerB.Run(context.Background(), rcfg, rt)
	}()

	select {
	case <-rewriteStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("intake hook never started")
	}
	close(rewriteRelease)
	wg.Wait()

	// Only the claiming worker runs intake and reaches the rewrite path.
	assert.Equal(t, int32(1), rewriteCalls.Load(),
		"only the claiming worker must run the rewrite intake hook; concurrent losers must not reach it")

	// The description must be rewritten exactly once: contains the added
	// text but does not contain it twice (no double-rewrite).
	got, err := inner.Get(original.ID)
	require.NoError(t, err)
	count := strings.Count(got.Description, "Add explicit validation step.")
	assert.Equal(t, 1, count,
		"description must be rewritten exactly once; double-rewrite would indicate a concurrent write")

	// Exactly one intake-rewritten event must exist.
	events, err := inner.Events(original.ID)
	require.NoError(t, err)
	var rewriteEvents int
	for _, ev := range events {
		if ev.Kind == "intake-rewritten" {
			rewriteEvents++
		}
	}
	assert.Equal(t, 1, rewriteEvents,
		"exactly one intake-rewritten event must be recorded; duplicates indicate concurrent rewrite")
}

// TestExecuteBeadWorkerReadinessRejectReleasesOrParksClaim is AC #4:
// Covers the post-claim outcomes when intake or lint rejects the bead:
//   - infra error: fail-open, proceed to execution
//   - operator_required during queue drain: warn and proceed to execution
//   - operator_required during targeted execution: move bead to status=proposed and unclaim
//   - lint-blocked: unclaim bead, no execution
//   - actionable_atomic: proceed to implementation normally
func TestExecuteBeadWorkerReadinessRejectReleasesOrParksClaim(t *testing.T) {
	t.Run("intake_infra_error_proceeds_to_execution", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var execCalled atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				execCalled.Add(1)
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
			}),
		}
		opts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once: true,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{}, fmt.Errorf("intake service timeout")
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, int32(1), execCalled.Load(), "infra error must fail open and proceed to execution")
		assert.Equal(t, 1, result.Successes)

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status, "bead must be closed after successful execution")
	})

	t.Run("operator_required_warns_and_executes_during_queue_drain", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		var execCalled atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				execCalled.Add(1)
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
			}),
		}
		opts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &eventSink,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeOperatorRequired,
					Detail:  "missing root cause with file:line",
				}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, int32(1), execCalled.Load(), "broad queue drain should attempt work despite intake uncertainty")
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, 1, result.Successes)

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status, "successful best-effort attempt should close normally")
		assert.NotContains(t, got.Labels, bead.LabelNeedsHuman,
			"best-effort intake warning must not rely on needs_human label parking")
		assert.Empty(t, bead.GetNeedsHumanMeta(*got).Reason)

		// An intake.warn event is still recorded as evidence for review/follow-up,
		// but it does not park the bead in broad queue-drain mode.
		events, err := store.Events(candidate.ID)
		require.NoError(t, err)
		var foundWarn bool
		for _, ev := range events {
			if ev.Kind != "intake.warn" {
				continue
			}
			foundWarn = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			assert.Equal(t, "readiness_best_effort", body["reason"])
		}
		assert.True(t, foundWarn, "an intake.warn event must be recorded as review evidence")

		// The event stream must contain pre_claim_intake.blocked.
		assert.Contains(t, eventSink.String(), "pre_claim_intake.blocked")
	})

	t.Run("operator_required_targeted_proposes_and_unclaims", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatal("executor must not run for terminal intake outcomes in targeted mode")
				return ExecuteBeadReport{}, nil
			}),
		}
		opts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			EventSink:    &eventSink,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeOperatorRequired,
					Detail:  "missing root cause with file:line",
				}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.Attempts, "targeted terminal intake outcome must not count as an attempt")

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusProposed, got.Status, "targeted execution may park for operator attention")
		assert.Empty(t, got.Owner, "bead must be unclaimed after targeted terminal intake")
		assert.NotContains(t, got.Labels, bead.LabelNeedsHuman,
			"terminal intake outcome must not rely on needs_human label parking")
		assert.Equal(t, "operator_required", bead.GetNeedsHumanMeta(*got).Reason)

		events, err := store.Events(candidate.ID)
		require.NoError(t, err)
		var foundBlocked bool
		for _, ev := range events {
			if ev.Kind != "intake.blocked" {
				continue
			}
			foundBlocked = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			assert.Equal(t, string(PreClaimIntakeOperatorRequired), body["intake_outcome"])
		}
		assert.True(t, foundBlocked, "an intake.blocked event must be recorded when targeted execution parks")
		assert.Contains(t, eventSink.String(), "pre_claim_intake.blocked")
	})

	t.Run("timeout_unclaims_and_continues_in_watch_mode", func(t *testing.T) {
		store, first, second := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var execCalled atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				if beadID == first.ID {
					t.Fatal("timed-out readiness bead must not reach execution")
				}
				execCalled.Add(1)
				cancel()
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
			}),
		}
		opts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:            executeloop.ModeWatch,
			IdleInterval:    time.Hour,
			EventSink:       &eventSink,
			PreClaimTimeout: 20 * time.Millisecond,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				if beadID != first.ID {
					return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
				}
				<-ctx.Done()
				return PreClaimIntakeResult{}, ctx.Err()
			},
		})
		require.ErrorIs(t, err, context.Canceled)
		require.NotNil(t, result)

		assert.Equal(t, int32(1), execCalled.Load(), "watch mode must continue to the next ready bead after a readiness timeout")
		assert.Equal(t, 1, result.Successes)
		assert.Equal(t, 1, result.Attempts)

		gotFirst, err := store.Get(first.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, gotFirst.Status, "timed-out readiness bead must be released back to the queue")
		assert.Empty(t, gotFirst.Owner)
		assert.NotEmpty(t, gotFirst.Extra["execute-loop-retry-after"])

		gotSecond, err := store.Get(second.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, gotSecond.Status)
		assert.Contains(t, eventSink.String(), "pre_claim_intake.warn")
	})

	t.Run("lint_blocked_warns_and_executes_during_queue_drain", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		var execCalled atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				execCalled.Add(1)
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
			}),
		}

		opts := config.TestLoopConfigOpts{
			Assignee:                           "worker",
			BeadQualityLintBlockThresholdScore: 7,
		}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &eventSink,
			PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
				return LintResult{Score: 4, Rationale: "incomplete AC"}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, int32(1), execCalled.Load(), "broad queue drain should attempt work despite lint uncertainty")
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, 1, result.Successes)

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status, "successful best-effort attempt should close normally")

		assert.Contains(t, eventSink.String(), "pre_dispatch_lint.blocked")
	})

	t.Run("lint_blocked_targeted_unclaims_without_execution", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatal("executor must not run when targeted lint blocks dispatch")
				return ExecuteBeadReport{}, nil
			}),
		}

		opts := config.TestLoopConfigOpts{
			Assignee:                           "worker",
			BeadQualityLintBlockThresholdScore: 7,
		}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			EventSink:    &eventSink,
			PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
				return LintResult{Score: 4, Rationale: "incomplete AC"}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.Attempts, "targeted lint block must not count as an attempt")

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, got.Status, "targeted lint block should unclaim without parking")

		assert.Contains(t, eventSink.String(), "pre_dispatch_lint.blocked")
	})

	t.Run("actionable_atomic_proceeds_to_implementation", func(t *testing.T) {
		store, candidate, _ := newExecuteLoopTestStore(t)
		var execCalled atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				execCalled.Add(1)
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, ResultRev: "rev"}, nil
			}),
		}
		opts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(opts).Resolve(config.TestLoopOverrides(opts))

		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once: true,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, int32(1), execCalled.Load(), "actionable_atomic must proceed to implementation")
		assert.Equal(t, 1, result.Successes)

		got, err := store.Get(candidate.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, got.Status)
	})
}
