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

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "intake infra errors must fail open and continue to Claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Contains(t, eventSink.String(), "pre_claim_intake.warn")
	assert.Contains(t, eventSink.String(), "intake_error")
	assert.Contains(t, eventSink.String(), "intake service unavailable")
}

func TestIntake_HookRunsAfterClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	var preflightSeen int32
	var intakeSeen int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			if atomic.LoadInt32(&preflightSeen) == 0 {
				t.Fatal("route preflight must run before Claim")
			}
			// Claim now happens BEFORE intake: intake must NOT have run yet.
			if atomic.LoadInt32(&intakeSeen) != 0 {
				t.Fatal("Claim must run before PreClaimIntakeHook")
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
			if atomic.LoadInt32(&store.claimCalls) == 0 {
				t.Fatal("PreClaimIntakeHook must run after Claim")
			}
			atomic.StoreInt32(&intakeSeen, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&preflightSeen), "route preflight must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeSeen), "intake hook must run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "Claim must run before intake")
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
	// Claim now happens before intake; for terminal outcomes the bead is
	// claimed then immediately unclaimed, so claimCalls == 1 but the bead
	// returns to StatusOpen and execution does not proceed.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim must run before intake check")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get("ddx-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

func TestIntake_ErrorContinuesToClaimWithoutParkingCandidate(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}
	now := time.Date(2026, 5, 7, 4, 30, 0, 0, time.UTC)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-error-open",
				ResultRev: "abc456",
			}, nil
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

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "intake errors must not block claim")
	assert.Equal(t, 1, result.Attempts)
	assert.Contains(t, log.String(), "pre-claim intake: starting "+candidate.ID)
	assert.Contains(t, log.String(), "pre-claim intake warning: empty output (continuing with "+candidate.ID+")")
	assert.NotContains(t, log.String(), "pre-claim intake: pre-claim intake:")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.NotContains(t, got.Extra, "execute-loop-last-status")
	assert.NotContains(t, got.Extra, "execute-loop-last-detail")
	assert.NotContains(t, got.Extra, "execute-loop-retry-after")
}

func TestIntake_LogsStartBeforeHookReturns(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-intake-start",
				ResultRev: "abc789",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var log bytes.Buffer
	var eventSink bytes.Buffer

	go func() {
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:      true,
			Log:       &log,
			EventSink: &eventSink,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				close(started)
				<-release
				return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
			},
		})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("intake hook did not start")
	}
	assert.Contains(t, log.String(), "pre-claim intake: starting "+candidate.ID)
	assert.Contains(t, eventSink.String(), "pre_claim_intake.start")

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls))
}

func TestIntake_ActionableButRewritten_UpdatesAfterClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\nlegacy intake is vague\n\nROOT CAUSE\nmissing constraints\n\nPROPOSED FIX\nrewrite the bead\n\nNON-SCOPE\nchange no product semantics\n"
		b.Acceptance = "1. preserve the original intent\n2. name the verification command"
	}))

	var claimSeenBeforeRewrite int32
	store := &claimCountingStore{
		Store: inner,
		beforeClaim: func() {
			// Claim now happens BEFORE intake, so the description must NOT
			// be rewritten yet when beforeClaim is called.
			got, err := inner.Get(candidate.ID)
			require.NoError(t, err)
			if strings.Contains(got.Description, "Add an explicit validation step.") {
				t.Fatalf("description must NOT be rewritten before Claim; got %q", got.Description)
			}
			atomic.StoreInt32(&claimSeenBeforeRewrite, 1)
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
	assert.Equal(t, int32(1), atomic.LoadInt32(&claimSeenBeforeRewrite), "claim must happen before rewrite")
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

	// Claim now happens before intake; for unsafe rewrites the bead is
	// claimed then immediately unclaimed, so claimCalls == 1 but execution
	// does not proceed and the bead description/acceptance must be unchanged.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim runs before intake check")
	assert.Contains(t, eventSink.String(), "pre_claim_intake.blocked")
	assert.Contains(t, eventSink.String(), "ambiguous_needs_human")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "rejected rewrite must leave bead open")
	assert.NotContains(t, got.Description, "invented product semantics", "original description must be preserved")
	assert.Equal(t, "1. preserve the original intent\n2. name the verification command", got.Acceptance)
	assert.Contains(t, got.Labels, bead.LabelNeedsHuman, "rejected rewrite must add needs_human label")

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var foundIntakeBlocked bool
	for _, ev := range events {
		if ev.Kind != "intake.blocked" {
			continue
		}
		foundIntakeBlocked = true
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		assert.Equal(t, "ambiguous_needs_human", body["intake_outcome"])
	}
	assert.True(t, foundIntakeBlocked, "rejected rewrite must append intake.blocked event to bead")
}

func TestIntake_UnsafeRewriteRemovedFromReadyExecution(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal description\n\nROOT CAUSE\nsome cause\n\nNON-SCOPE\nDo not change the API contract\n"
		b.Acceptance = "1. verify something\n2. run tests"
	}))

	store := &claimCountingStore{Store: inner}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run for unsafe rewrites")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeActionableButRewritten,
				Detail:  "rewrote description without preserving original",
				Rewrite: PreClaimIntakeRewrite{
					Description:   "PROBLEM\ncompletely different description\n",
					Acceptance:    "1. verify something\n2. run tests",
					ChangedFields: []string{"description"},
				},
			}, nil
		},
	})
	require.NoError(t, err)

	ready, err := inner.ReadyExecution()
	require.NoError(t, err)
	for _, b := range ready {
		if b.ID == candidate.ID {
			t.Fatalf("parked bead %s must not appear in ReadyExecution after unsafe rewrite rejection", candidate.ID)
		}
	}
}

func TestIntake_DescriptionPreservationFailureParksForHuman(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal problem text\n\nROOT CAUSE\ncli/internal/agent/preclaim.go:42\n\nNON-SCOPE\nDo not touch acceptance criteria\n"
		b.Acceptance = "1. check the output\n2. run lefthook"
	}))

	store := &claimCountingStore{Store: inner}
	var logBuf bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run for description preservation failures")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &logBuf,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeActionableButRewritten,
				Detail:  "description must preserve original text",
				Rewrite: PreClaimIntakeRewrite{
					Description:   "PROBLEM\ncompletely different text that drops original\n",
					Acceptance:    "1. check the output\n2. run lefthook",
					ChangedFields: []string{"description"},
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open after description preservation failure")
	assert.Contains(t, got.Labels, bead.LabelNeedsHuman, "description preservation failure must add needs_human label")
	assert.Contains(t, got.Description, "original problem text", "original description must not be overwritten")

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var foundBlocked bool
	for _, ev := range events {
		if ev.Kind != "intake.blocked" {
			continue
		}
		foundBlocked = true
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		assert.Equal(t, "ambiguous_needs_human", body["intake_outcome"])
		assert.Contains(t, fmt.Sprintf("%v", body["detail"]), "drops commitment")
	}
	assert.True(t, foundBlocked, "description preservation failure must append intake.blocked event")
	assert.Contains(t, logBuf.String(), "parking")
}
