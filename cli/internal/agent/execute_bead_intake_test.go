package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntake_ActionableAtomic_ClaimsNormally(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeCalls), "intake hook must run once")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "actionable atomic intake must proceed to Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestIntake_InfrastructureErrorFailsOpen(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var eventSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-warn",
				ResultRev: "def456",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &eventSink,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{}, fmt.Errorf("intake service unavailable")
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "intake_error must fail open to Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Contains(t, eventSink.String(), "pre_claim_intake.warn")
	assert.Contains(t, eventSink.String(), "intake_error")
	assert.Contains(t, eventSink.String(), "intake service unavailable")
}

func TestIntake_HookRunsBeforeClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	var preflightSeen int32
	var intakeSeen int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			if atomic.LoadInt32(&preflightSeen) == 0 {
				t.Fatal("route preflight must run before Claim")
			}
			if atomic.LoadInt32(&intakeSeen) == 0 {
				t.Fatal("PreClaimIntakeHook must run before Claim")
			}
		},
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-order",
				ResultRev: "fedcba",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "claude", Model: "claude-sonnet-4-6"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		RoutePreflight: func(ctx context.Context, harness, model string) error {
			atomic.StoreInt32(&preflightSeen, 1)
			return nil
		},
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			if atomic.LoadInt32(&preflightSeen) == 0 {
				t.Fatal("PreClaimIntakeHook must run after route preflight")
			}
			atomic.StoreInt32(&intakeSeen, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightSeen), "route preflight must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeSeen), "intake hook must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "Claim must run after preflight and intake")
}

func TestIntake_NonAtomicSkipsClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	var intakeCalls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run when intake is non-atomic")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeAmbiguousNeedsHuman,
				Detail:  "need human clarification",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "non-atomic intake must skip Claim")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get("ddx-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestIntake_ErrorSkipsClaimAndParksCandidate(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}
	now := time.Date(2026, 5, 7, 4, 30, 0, 0, time.UTC)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run when intake errors")
			return ExecuteBeadReport{}, nil
		}),
		Now: func() time.Time {
			return now
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var log bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &log,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{}, errors.New("pre-claim intake: empty output")
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "intake errors must not claim")
	assert.Equal(t, 0, result.Attempts)
	assert.Contains(t, log.String(), "pre-claim intake: pre-claim intake: empty output (skipping "+candidate.ID+")")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Equal(t, string(PreClaimIntakeError), got.Extra["execute-loop-last-status"])
	assert.Equal(t, "pre-claim intake: empty output", got.Extra["execute-loop-last-detail"])
	assert.Equal(t, now.Add(PreClaimIntakeCooldown).Format(time.RFC3339), got.Extra["execute-loop-retry-after"])
}

func TestIntake_ActionableButRewritten_UpdatesBeforeClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\nlegacy intake is vague\n\nROOT CAUSE\nmissing constraints\n\nPROPOSED FIX\nrewrite the bead\n\nNON-SCOPE\nchange no product semantics\n"
		b.Acceptance = "1. preserve the original intent\n2. name the verification command"
	}))

	var claimSawRewrite int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			got, err := inner.Get(candidate.ID)
			require.NoError(t, err)
			if !strings.Contains(got.Description, "Add an explicit validation step.") {
				t.Fatalf("description must be rewritten before Claim; got %q", got.Description)
			}
			if !strings.Contains(got.Acceptance, "lefthook run pre-commit") {
				t.Fatalf("acceptance must be rewritten before Claim; got %q", got.Acceptance)
			}
			atomic.StoreInt32(&claimSawRewrite, 1)
		},
	}

	var eventSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-rewrite",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &eventSink,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeActionableButRewritten,
				Detail:  "tightened scope and verification",
				Rewrite: PreClaimIntakeRewrite{
					Description:   "PROBLEM\nlegacy intake is vague\n\nROOT CAUSE\nmissing constraints\n\nPROPOSED FIX\nrewrite the bead\n\nNON-SCOPE\nchange no product semantics\n\nAdd an explicit validation step.",
					Acceptance:    "1. preserve the original intent\n2. name the verification command\n3. lefthook run pre-commit",
					ChangedFields: []string{"description", "acceptance"},
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "rewritten intake must still proceed to Claim")
	assert.Equal(t, int32(1), atomic.LoadInt32(&claimSawRewrite), "rewrite must land before Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Description, "Add an explicit validation step.")
	assert.Contains(t, got.Acceptance, "lefthook run pre-commit")

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind != "intake-rewritten" {
			continue
		}
		found = true
		assert.Contains(t, ev.Summary, "description")
		assert.Contains(t, ev.Summary, "acceptance")
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		assert.Equal(t, "tightened scope and verification", body["rationale"])
		assert.Contains(t, body, "before")
		assert.Contains(t, body, "after")
		assert.NotContains(t, ev.Body, "MODE: intake")
		assert.NotContains(t, ev.Body, "legacy intake is vague\n\nROOT CAUSE")
	}
	require.True(t, found, "rewritten intake must record an intake-rewritten event")
}

func TestIntake_UnsafeRewriteBlocksForHuman(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\nlegacy intake is vague\n\nROOT CAUSE\nmissing constraints\n\nPROPOSED FIX\nrewrite the bead\n"
		b.Acceptance = "1. preserve the original intent\n2. name the verification command"
	}))

	store := &claimCountingStore{Store: inner}
	var eventSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run for unsafe rewrites")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &eventSink,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeActionableButRewritten,
				Detail:  "attempted to drop acceptance criteria",
				Rewrite: PreClaimIntakeRewrite{
					Description:   "PROBLEM\ninvented product semantics\n",
					Acceptance:    "1. preserve the original intent",
					ChangedFields: []string{"description", "acceptance"},
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(0), atomic.LoadInt32(&store.claimCalls), "unsafe rewrite must not proceed to Claim")
	assert.Contains(t, eventSink.String(), "pre_claim_intake.blocked")
	assert.Contains(t, eventSink.String(), "ambiguous_needs_human")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.NotContains(t, got.Description, "invented product semantics")
	assert.Equal(t, "1. preserve the original intent\n2. name the verification command", got.Acceptance)
}
