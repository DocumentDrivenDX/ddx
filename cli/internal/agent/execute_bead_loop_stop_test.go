package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteBeadWorkerStopConditionWiredIntoRun(t *testing.T) {
	t.Run("drained", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not run for drained queue")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			Mode: executeloop.ModeDrain,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.NoReadyWork)
		assert.Equal(t, "drained", exitReason)
		assert.Equal(t, string(work.StopConditionDrained), result.StopCondition)
		assert.Equal(t, exitReason, result.ExitReason)
	})

	t.Run("once complete", func(t *testing.T) {
		store, _, _ := newExecuteLoopTestStore(t)
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-once-stop",
					ResultRev: "abc123",
				}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			Mode: executeloop.ModeOnce,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, "once_complete", exitReason)
		assert.Equal(t, string(work.StopConditionOnce), result.StopCondition)
		assert.Equal(t, exitReason, result.ExitReason)
	})

	t.Run("signal canceled", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not run after cancellation")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, ctx, ExecuteBeadLoopRuntime{
			Mode: executeloop.ModeWatch,
		})
		require.ErrorIs(t, err, context.Canceled)
		require.NotNil(t, result)
		assert.Equal(t, "sigint", exitReason)
		assert.Equal(t, string(work.StopConditionSignal), result.StopCondition)
		assert.Equal(t, exitReason, result.ExitReason)
	})

	t.Run("budget stop before claim", func(t *testing.T) {
		inner, _, _ := newExecuteLoopTestStore(t)
		store := &claimCountingStore{Store: inner}
		var execCalls int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				atomic.AddInt32(&execCalls, 1)
				t.Fatalf("executor must not run after budget stop")
				return ExecuteBeadReport{}, nil
			}),
		}
		result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
			BudgetStop: func() (work.StopDecision, ExecuteBeadReport, bool) {
				decision, _ := work.ClassifyStop(work.StopInput{Budget: true})
				return decision, ExecuteBeadReport{
					Status: ExecuteBeadStatusExecutionFailed,
					Detail: "cost cap reached",
				}, true
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "budget", exitReason)
		assert.Equal(t, string(work.StopConditionBudget), result.StopCondition)
		assert.Equal(t, exitReason, result.ExitReason)
		assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls))
		assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
		require.Len(t, result.Results, 1)
		assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
		assert.Equal(t, "cost cap reached", result.Results[0].Detail)
	})
}

// TestStopCondition_BudgetAfterImplementerCostStopsBeforeNextClaim verifies
// that after an implementer attempt's cost trips the cap, the BudgetStop
// callback fires at the top of the next loop iteration and prevents the next
// bead from being claimed. This is the AC3 (ddx-b1cf1f6b) coverage path.
func TestStopCondition_BudgetAfterImplementerCostStopsBeforeNextClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}
	tracker := escalation.NewCostCapTracker(5.0, func(string) bool { return true })

	var execCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			// Accumulate $6 — trips the $5 cap; mirrors accumulateBilledCost.
			tracker.Add("openrouter", 6.0)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-budget-impl",
				ResultRev: "abc123",
			}, nil
		}),
	}

	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		BudgetStop: func() (work.StopDecision, ExecuteBeadReport, bool) {
			detail, tripped := tracker.Tripped()
			if !tripped {
				return work.StopDecision{}, ExecuteBeadReport{}, false
			}
			decision, _ := work.ClassifyStop(work.StopInput{Budget: true})
			return decision, ExecuteBeadReport{
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: detail,
			}, true
		},
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "budget", exitReason)
	assert.Equal(t, string(work.StopConditionBudget), result.StopCondition)
	assert.Equal(t, exitReason, result.ExitReason)
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCalls), "first bead must be attempted")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "second bead must not be claimed after budget stop")
}

func TestExecuteBeadLoopResult_IncludesStopReasonOnDrainExit(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run for drained queue")
			return ExecuteBeadReport{}, nil
		}),
	}
	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, string(work.StopConditionDrained), result.StopCondition)
	assert.Equal(t, "drained", result.ExitReason)
	assert.Equal(t, result.ExitReason, exitReason)
}

type fatalReadyStore struct {
	*bead.Store
	err error
}

func (s *fatalReadyStore) ReadyExecution() ([]bead.Bead, error) {
	return nil, s.err
}

func TestExecuteBeadLoopResult_IncludesStopReasonOnFatalConfigExit(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	fatalErr := errors.New("ready execution unavailable")

	worker := &ExecuteBeadWorker{
		Store: &fatalReadyStore{Store: store, err: fatalErr},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run after fatal queue error")
			return ExecuteBeadReport{}, nil
		}),
	}
	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.ErrorIs(t, err, fatalErr)
	require.NotNil(t, result)
	assert.Equal(t, "FatalConfig", result.StopCondition)
	assert.Equal(t, "fatal_config", result.ExitReason)
	assert.Equal(t, result.ExitReason, exitReason)
}

func TestExecuteBeadLoopResult_PreservesLoopEndExitReason(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not run for drained queue")
			return ExecuteBeadReport{}, nil
		}),
	}
	result, exitReason, err := runStopConditionCase(t, worker, context.Background(), ExecuteBeadLoopRuntime{
		Mode: executeloop.ModeDrain,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, result.ExitReason, exitReason)
}

// TestStopCondition_BudgetAfterReviewCostPreventsClose verifies that when
// reviewer cost trips the budget cap in the APPROVE path, RunPostMergeReview
// records the review events but does NOT call CloseWithEvidence and returns
// Approved=false. This is the AC2 (ddx-b1cf1f6b) coverage path.
func TestStopCondition_BudgetAfterReviewCostPreventsClose(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	// Pre-charge $4 of $5 cap; reviewer will add $1.5, tripping the cap.
	tracker := escalation.NewCostCapTracker(5.0, func(string) bool { return true })
	tracker.Add("openrouter", 4.0)

	reviewer := makeReviewerWithCost(VerdictApprove, "### Verdict: APPROVE\n\nAll good.", 1.5)
	out := RunPostMergeReview(context.Background(), PostMergeReviewInput{
		Bead: *first,
		Report: ExecuteBeadReport{
			BeadID:    first.ID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess-budget-review",
			ResultRev: "deadcafe",
			CostUSD:   4.0,
		},
		Reviewer:      reviewer,
		Store:         store,
		ProjectRoot:   t.TempDir(),
		Rcfg:          config.NewTestConfigForLoop(config.TestLoopConfigOpts{Assignee: "worker"}).Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{Assignee: "worker"})),
		Now:           time.Now,
		Assignee:      "worker",
		ReviewCostCap: tracker,
	})

	require.False(t, out.Approved, "APPROVE whose reviewer cost trips the budget cap must not close the bead")
	require.NoError(t, out.StoreErr)

	got, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	require.NotEqual(t, bead.StatusClosed, got.Status, "bead must remain open when budget is exceeded by reviewer cost")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var foundApprove, foundDeferred bool
	for _, ev := range events {
		if ev.Kind == "review" && ev.Summary == "APPROVE" {
			foundApprove = true
		}
		if ev.Kind == "review-cost-deferred" {
			foundDeferred = true
		}
	}
	assert.True(t, foundApprove, "APPROVE review event must be recorded even when budget exceeded")
	assert.True(t, foundDeferred, "review-cost-deferred event must be recorded when budget exceeded by reviewer cost")
}

func runStopConditionCase(t *testing.T, worker *ExecuteBeadWorker, ctx context.Context, runtime ExecuteBeadLoopRuntime) (*ExecuteBeadLoopResult, string, error) {
	t.Helper()

	var sink bytes.Buffer
	runtime.EventSink = &sink
	runtime.SessionID = "sess-stop-condition"
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(ctx, rcfg, runtime)
	exitReason := loopEndExitReason(t, sink.String())
	return result, exitReason, err
}

func loopEndExitReason(t *testing.T, raw string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["type"] != "loop.end" {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		require.True(t, ok, "loop.end data missing")
		reason, _ := data["exit_reason"].(string)
		return reason
	}
	require.Fail(t, "loop.end event not found", raw)
	return ""
}
