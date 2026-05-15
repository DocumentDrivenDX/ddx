package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimCountingStore wraps bead.Store and counts Claim invocations. The rest
// of ExecuteBeadLoopStore is delegated to the embedded *bead.Store.
type claimCountingStore struct {
	*bead.Store
	claimCalls  int32
	beforeClaim func()
}

func (s *claimCountingStore) Claim(id, assignee string) error {
	if s.beforeClaim != nil {
		s.beforeClaim()
	}
	atomic.AddInt32(&s.claimCalls, 1)
	return s.Store.Claim(id, assignee)
}

func (s *claimCountingStore) ClaimWithOptions(id, assignee, session, worktree string) error {
	if s.beforeClaim != nil {
		s.beforeClaim()
	}
	atomic.AddInt32(&s.claimCalls, 1)
	return s.Store.ClaimWithOptions(id, assignee, session, worktree)
}

func (s *claimCountingStore) TouchClaimHeartbeat(id string) error {
	return s.Store.TouchClaimHeartbeat(id)
}
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
func (s *claimCountingStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	return s.Store.SetExecutionCooldown(id, until, status, detail, baseRev)
}
func (s *claimCountingStore) IncrNoChangesCount(id string) (int, error) {
	return s.Store.IncrNoChangesCount(id)
}
func (s *claimCountingStore) Reopen(id, reason, notes string) error {
	return s.Store.Reopen(id, reason, notes)
}

// TestRoutingPreflightRejectionExitsLoopWithoutClaim covers ddx-98e6e9ef:
// when RoutePreflight returns the upstream typed-incompatibility error
// (agent.ErrHarnessModelIncompatible), the loop must fail during startup
// before any bead is claimed or executed.
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

	// AC#1: preflight ran exactly once during startup.
	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightCalls), "preflight must be invoked")

	// AC#2: Claim was NEVER called on the store.
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls),
		"beadStore.Claim must not be called when preflight rejects")

	// Executor must not have been invoked either.
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCount),
		"executor must not run when preflight rejects")

	// AC#3: startup failure is surfaced once as a worker-level
	// execution_failed record before any bead-specific attempt exists.
	require.Len(t, result.Results, 1, "exactly one worker-level failure record")
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Empty(t, report.BeadID)
	assert.Equal(t, "claude", report.Harness)
	assert.Equal(t, "gpt-5", report.Model)
	assert.Contains(t, report.Detail, "claude", "detail must name rejected harness")
	assert.Contains(t, report.Detail, "gpt-5", "detail must name rejected model")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.LastFailureStatus)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, 0, result.Attempts)
	assert.Equal(t, "Preflight", result.StopCondition)
	assert.Equal(t, "preflight_failed", result.ExitReason)

	// AC#4: no bead-specific attempt events were appended.
	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "power-attempt", ev.Kind,
			"no power-attempt event must be recorded when preflight rejects at startup")
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

func TestLoop_LintHook_FiresAfterClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	var hookSeen int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			// Claim now runs before lint: the hook must NOT have fired yet.
			if atomic.LoadInt32(&hookSeen) != 0 {
				t.Fatal("Claim must run before PreDispatchLintHook")
			}
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-lint-order",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                           "worker",
		BeadQualityLintBlockThresholdScore: 1,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
			atomic.StoreInt32(&hookSeen, 1)
			return LintResult{Score: 9, Rationale: "ok"}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&hookSeen), "pre-dispatch lint hook must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim should proceed after lint")
}

func TestLintHook_WarnOnlySkipsAdvisoryLintWithoutEvent(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}
	var hookSeen int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-lint-warn",
				ResultRev: "def456",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
			atomic.StoreInt32(&hookSeen, 1)
			return LintResult{
				Score:          2,
				Rationale:      "missing acceptance criteria",
				SuggestedFixes: []string{"add numbered AC"},
				WaiversApplied: []string{"epic"},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "warn-only lint must not block claim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&hookSeen), "warn-only lint must not call model-backed advisory hook")
	require.Len(t, result.Results, 1)

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	for i := range events {
		assert.NotEqual(t, "bead-quality.lint", events[i].Kind, "warn-only advisory lint must not add bead event noise")
	}
}

func TestLintHook_BlockMode_RefusesDispatchOnLowScore(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var log bytes.Buffer
	var loop bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run when lint blocks dispatch")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                           "worker",
		BeadQualityLintBlockThresholdScore: 5,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		Log:          &log,
		EventSink:    &loop,
		PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
			return LintResult{
				Score:          2,
				Rationale:      "standalone prompt missing root cause",
				SuggestedFixes: []string{"add ROOT CAUSE with file:line"},
				WaiversApplied: []string{"none"},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Claim now runs before lint; for lint-blocked the bead is claimed then
	// immediately unclaimed. claimCalls == 1 but execution does not proceed.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim runs before lint check; blocked lint unclaims")
	assert.Contains(t, log.String(), "bead-lifecycle")
	assert.Contains(t, log.String(), "MODE: lint")
	assert.Contains(t, loop.String(), "pre_dispatch_lint.blocked")

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var lintEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "bead-quality.lint" {
			lintEvent = &events[i]
			break
		}
	}
	require.NotNil(t, lintEvent, "blocked lint must still append the lint event")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(lintEvent.Body), &body))
	assert.Equal(t, float64(2), body["score"])
	assert.Equal(t, float64(5), body["threshold_score"])
	assert.Equal(t, true, body["dispatch_blocked"])
}

func TestLintHook_SkillMissing_ProceedsWithWarning(t *testing.T) {
	runLintWarningProceedTest(t, "skill missing: bead-lifecycle", 0, func(ctx context.Context, beadID string) (LintResult, error) {
		return LintResult{}, fmt.Errorf("skill missing: bead-lifecycle")
	})
}

func TestLintHook_BadJSON_ProceedsWithWarning(t *testing.T) {
	runLintWarningProceedTest(t, "bad JSON from hook", 0, func(ctx context.Context, beadID string) (LintResult, error) {
		return LintResult{}, fmt.Errorf("bad JSON from hook")
	})
}

func TestLintHook_Timeout_ProceedsWithWarning(t *testing.T) {
	runLintWarningProceedTest(t, "context deadline exceeded", 10*time.Millisecond, func(ctx context.Context, beadID string) (LintResult, error) {
		<-ctx.Done()
		return LintResult{}, ctx.Err()
	})
}

func runLintWarningProceedTest(t *testing.T, warning string, preClaimTimeout time.Duration, hook func(context.Context, string) (LintResult, error)) {
	t.Helper()

	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-lint-warn",
				ResultRev: "warn-123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                           "worker",
		BeadQualityLintBlockThresholdScore: 5,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                true,
		PreClaimTimeout:     preClaimTimeout,
		PreDispatchLintHook: hook,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "warning lint must proceed to claim")
	require.Len(t, result.Results, 1)

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var lintEvent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "bead-quality.lint" {
			lintEvent = &events[i]
			break
		}
	}
	require.NotNil(t, lintEvent, "warning lint must append the lint event")
	assert.Contains(t, lintEvent.Body, warning)
}
