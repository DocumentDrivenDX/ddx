package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimCountingStore wraps bead.Store and counts Claim invocations. The rest
// of ExecuteBeadLoopStore is delegated to the embedded *bead.Store.
type claimCountingStore struct {
	*bead.Store
	claimCalls int32
}

func (s *claimCountingStore) Claim(id, assignee string) error {
	atomic.AddInt32(&s.claimCalls, 1)
	return s.Store.Claim(id, assignee)
}

func (s *claimCountingStore) Heartbeat(id string) error            { return s.Store.Heartbeat(id) }
func (s *claimCountingStore) Unclaim(id string) error              { return s.Store.Unclaim(id) }
func (s *claimCountingStore) ReadyExecution() ([]bead.Bead, error) { return s.Store.ReadyExecution() }
func (s *claimCountingStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	return s.Store.CloseWithEvidence(id, sessionID, commitSHA)
}
func (s *claimCountingStore) AppendEvent(id string, ev bead.BeadEvent) error {
	return s.Store.AppendEvent(id, ev)
}
func (s *claimCountingStore) Events(id string) ([]bead.BeadEvent, error) {
	return s.Store.Events(id)
}
func (s *claimCountingStore) SetExecutionCooldown(id string, until time.Time, status, detail string) error {
	return s.Store.SetExecutionCooldown(id, until, status, detail)
}
func (s *claimCountingStore) IncrNoChangesCount(id string) (int, error) {
	return s.Store.IncrNoChangesCount(id)
}
func (s *claimCountingStore) Reopen(id, reason, notes string) error {
	return s.Store.Reopen(id, reason, notes)
}

// TestRoutingPreflightRejectionExitsLoopWithoutClaim covers ddx-98e6e9ef:
// when RoutePreflight returns the upstream typed-incompatibility error
// (agent.ErrHarnessModelIncompatible), the loop must NOT claim the bead,
// must NOT invoke the executor, must record a worker-level execution_failed
// result naming the rejected (harness, model) pair, and must exit.
func TestRoutingPreflightRejectionExitsLoopWithoutClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCount, 1)
			return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess}, nil
		}),
	}

	// Fabricate a typed-incompatibility error via the upstream test seam:
	// the preflight callback returns the same error type that upstream
	// ResolveRoute returns when a harness allow-list rejects the model.
	rejected := agentlib.ErrHarnessModelIncompatible{
		Harness:         "claude",
		Model:           "gpt-5",
		SupportedModels: []string{"claude-opus-4-7", "claude-sonnet-4-6"},
	}
	var preflightCalls int32
	preflight := func(ctx context.Context, harness, model string) error {
		atomic.AddInt32(&preflightCalls, 1)
		return rejected
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee: "worker",
		Harness:  "claude",
		Model:    "gpt-5",
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:           true,
		RoutePreflight: preflight,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// AC#1: preflight ran before Claim.
	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightCalls), "preflight must be invoked")

	// AC#2: Claim was NEVER called on the store.
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls),
		"beadStore.Claim must not be called when preflight rejects")

	// Executor must not have been invoked either.
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCount),
		"executor must not run when preflight rejects")

	// AC#3: worker-level execution_failed record names the rejected pair.
	require.Len(t, result.Results, 1, "exactly one worker-level failure record")
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, candidate.ID, report.BeadID)
	assert.Equal(t, "claude", report.Harness)
	assert.Equal(t, "gpt-5", report.Model)
	assert.Contains(t, report.Detail, "claude", "detail must name rejected harness")
	assert.Contains(t, report.Detail, "gpt-5", "detail must name rejected model")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.LastFailureStatus)
	assert.Equal(t, 1, result.Failures)

	// AC#4: bead has no kind:tier-attempt event from this attempt.
	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "tier-attempt", ev.Kind,
			"no tier-attempt event must be recorded when preflight rejects pre-claim")
	}

	// Bead must remain open with no owner — confirms no Claim side-effects.
	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

// TestRoutingPreflightSuccessAllowsClaim verifies the gate is non-disruptive
// on the happy path: when RoutePreflight returns nil the loop proceeds to
// Claim and executor as normal.
func TestRoutingPreflightSuccessAllowsClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var execCount int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCount, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-ok",
				ResultRev: "abc1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "claude-opus-4-7"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			return nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "Claim must run when preflight passes")
	assert.Equal(t, int32(1), atomic.LoadInt32(&execCount), "executor must run when preflight passes")
	assert.Equal(t, 1, result.Successes)

	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

// TestRoutingPreflightDetailFormat asserts the worker-level failure detail
// format (AC#3 evidence) includes both the rejected harness and the rejected
// model in a stable, greppable form.
func TestRoutingPreflightDetailFormat(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "codex", Model: "qwen3"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			return agentlib.ErrHarnessModelIncompatible{
				Harness:         harness,
				Model:           model,
				SupportedModels: []string{"gpt-5"},
			}
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	detail := result.Results[0].Detail
	assert.True(t, strings.Contains(detail, "harness=codex"),
		"detail should carry harness=codex; got %q", detail)
	assert.True(t, strings.Contains(detail, "model=qwen3"),
		"detail should carry model=qwen3; got %q", detail)
}
