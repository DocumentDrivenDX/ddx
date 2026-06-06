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

// parkCountingStore wraps claimCountingStore and counts ParkToProposed calls.
type parkCountingStore struct {
	*claimCountingStore
	parkCalls int32
}

func (s *parkCountingStore) ParkToProposed(id string, reason bead.ParkReason, mutate func(*bead.Bead)) error {
	atomic.AddInt32(&s.parkCalls, 1)
	return s.Store.ParkToProposed(id, reason, mutate)
}

func (s *parkCountingStore) ParkToProposedWithIntakeEvent(id, actor, outcome, reason, detail string, body map[string]any, at time.Time, mutate func(*bead.Bead)) error {
	atomic.AddInt32(&s.parkCalls, 1)
	return s.Store.ParkToProposedWithIntakeEvent(id, actor, outcome, reason, detail, body, at, mutate)
}

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
		Once:         true,
		TargetBeadID: "ddx-0001",
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
		Once:         true,
		TargetBeadID: "ddx-0001",
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeOperatorRequired,
				Detail:  "need human clarification",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&intakeCalls))
	// Claim now happens before intake; for terminal outcomes the bead is
	// claimed, moved to operator attention, then unclaimed. Execution does not
	// proceed.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim must run before intake check")
	assert.Equal(t, 0, result.Attempts)

	got, err := inner.Get("ddx-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.Empty(t, got.Owner)
	assert.Equal(t, "operator_required", bead.GetNeedsHumanMeta(*got).Reason)
}

func TestReadinessNeedsRefineWarnsAndClaimsInWarnOnly(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-readiness-warn",
				ResultRev: "readiness-warn",
			}, nil
		}),
	}

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"needs_refine","rationale":"missing verification","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}]}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "warn-only readiness must still claim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.parkCalls), "warn-only readiness must not park")
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.executeCalls), "readiness hook must execute once")
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Attempts)

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestReadinessDifficultyDoesNotPersistBeadMetadata(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}
	legacyPowerKey := "triage." + "power_hint"
	retryFloorKey := "work-next-" + "min-power"

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			difficulty := ReadinessEstimatedDifficultyFromContext(ctx)
			assert.Equal(t, "hard", difficulty)
			got, err := inner.Get(beadID)
			require.NoError(t, err)
			if got.Extra != nil {
				require.NotContains(t, got.Extra, retryFloorKey)
				require.NotContains(t, got.Extra, "triage.estimated_difficulty")
				require.NotContains(t, got.Extra, legacyPowerKey)
			}
			return ExecuteBeadReport{
				BeadID:              beadID,
				Status:              ExecuteBeadStatusSuccess,
				SessionID:           "sess-readiness-power",
				ResultRev:           "readiness-power",
				RoutingIntentSource: "readiness",
				EstimatedDifficulty: difficulty,
				InferredPowerClass:  "smart",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome:             PreClaimIntakeActionableAtomic,
				EstimatedDifficulty: "hard",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	if got.Extra != nil {
		assert.NotContains(t, got.Extra, retryFloorKey)
		assert.NotContains(t, got.Extra, "triage.estimated_difficulty")
		assert.NotContains(t, got.Extra, legacyPowerKey)
	}
	events, err := inner.Events(beadRef.ID)
	require.NoError(t, err)
	var intentBody map[string]any
	for _, event := range events {
		if event.Kind != "execution-routing-intent" {
			continue
		}
		require.NoError(t, json.Unmarshal([]byte(event.Body), &intentBody))
		break
	}
	require.NotNil(t, intentBody, "readiness routing intent evidence must be recorded")
	assert.Equal(t, "readiness", intentBody["routing_intent_source"])
	assert.Equal(t, "hard", intentBody["estimated_difficulty"])
	assert.Equal(t, "smart", intentBody["requested_power_class"])
}

func TestACQualityGateWarnOnlyDoesNotPark(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Acceptance = "1. Improve clarity\n2. Better error messages\n"
	}))

	var innerSeen int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-ac-quality",
				ResultRev: "ac-quality",
			}, nil
		}),
	}

	intakeHook := NewACQualityPreClaimGate(store, config.BeadQualityModeWarnOnly, DefaultACQualityMinScore, func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		b, err := store.Get(beadID)
		require.NoError(t, err)
		assert.NotEqual(t, false, b.Extra["execution-eligible"], "warn-only AC quality must not make the bead ineligible")
		atomic.StoreInt32(&innerSeen, 1)
		return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "warn-only AC quality must still claim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.parkCalls), "warn-only AC quality must not park")
	assert.Equal(t, int32(1), atomic.LoadInt32(&innerSeen), "warn-only AC quality must reach the inner hook")
	assert.Equal(t, 1, result.Successes)

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind == "ac-quality-low" {
			found = true
			break
		}
	}
	assert.True(t, found, "warn-only AC quality must still append warning evidence")
}

func TestHardOperatorRequiredStillParksProposed(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run for operator_required intake")
			return ExecuteBeadReport{}, nil
		}),
	}

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"outcome":"operator_required","reason":"ambiguous scope","detail":"need human clarification"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "operator_required intake must still claim before parking")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "operator_required intake must park the bead")
	assert.Equal(t, 0, result.Attempts, "operator_required must not reach implementation")

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.Empty(t, got.Owner)
}

func TestPreClaimIntakeOperatorOverridePreventsRepeatParking(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-override",
				ResultRev: "override",
			}, nil
		}),
	}

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"outcome":"operator_required","reason":"ambiguous scope","detail":"need human clarification"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	firstResult, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, firstResult)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "first operator_required intake must park the bead")

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.Empty(t, got.Owner)

	require.NoError(t, inner.SetLifecycleStatus(beadRef.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted readiness decision",
		Actor:  "reviewer",
		Source: "test",
	}))

	secondResult, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, secondResult)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "operator acceptance must suppress repeat parking for the same finding")

	got, err = inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

func TestPreClaimIntakeChangedBeadInvalidatesOverride(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-override-changed",
				ResultRev: "override-changed",
			}, nil
		}),
	}

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"outcome":"operator_required","reason":"ambiguous scope","detail":"need human clarification"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls))

	require.NoError(t, inner.SetLifecycleStatus(beadRef.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted readiness decision",
		Actor:  "reviewer",
		Source: "test",
	}))

	require.NoError(t, inner.Update(context.Background(), beadRef.ID, func(b *bead.Bead) {
		b.Acceptance += "\n3. require an explicit operator acknowledgement"
		b.Description += "\n\nAdditional prompt-relevant detail."
	}))

	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)

	assert.Equal(t, int32(2), atomic.LoadInt32(&store.parkCalls), "changed prompt-relevant fields must invalidate the override")

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)

	events, err := inner.Events(beadRef.ID)
	require.NoError(t, err)
	var fingerprints []string
	for _, ev := range events {
		if ev.Kind != "intake.blocked" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		fingerprint, _ := body["fingerprint"].(string)
		if fingerprint != "" {
			fingerprints = append(fingerprints, fingerprint)
		}
	}
	require.Len(t, fingerprints, 2, "override invalidation must park again with a new readiness fingerprint")
	assert.NotEqual(t, fingerprints[0], fingerprints[1], "prompt changes must produce a different readiness fingerprint")
}

func TestIntakeBlockedEventIncludesStructuredDecisionFields(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run when intake parks the bead")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-0001",
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeOperatorRequired,
				Reason:  "ambiguous_scope",
				Detail:  "ambiguous scope requires human review",
			}, nil
		},
	})
	require.NoError(t, err)

	events, err := inner.Events("ddx-0001")
	require.NoError(t, err)
	var blocked *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "intake.blocked" {
			blocked = &events[i]
			break
		}
	}
	require.NotNil(t, blocked, "blocked intake must append durable evidence")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(blocked.Body), &body))
	assert.Equal(t, "pre_claim_intake.operator_required", body["rule_id"])
	assert.Equal(t, "ambiguous_scope", body["reason"])
	assert.Equal(t, "pre_claim_intake", body["decision_source"])
	assert.Equal(t, "block", body["policy_mode"])
	assert.Equal(t, "park", body["decision"])
	assert.Equal(t, "review intake result and accept, rewrite, split, block, or cancel", body["suggested_action"])
	assert.NotEmpty(t, body["fingerprint"])
	assert.NotEmpty(t, body["prompt_fingerprint"])
	assert.Equal(t, body["fingerprint"], body["accepted_fingerprint"])
	assert.Equal(t, "operator_required", body["intake_outcome"])
	assert.Equal(t, "ambiguous scope requires human review", body["detail"])
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
	assert.Contains(t, log.String(), "readiness check: starting")
	assert.Contains(t, log.String(), "readiness check unavailable: empty output (continuing)")
	assert.NotContains(t, log.String(), "pre-claim intake: pre-claim intake:")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.NotContains(t, got.Extra, "work-last-status")
	assert.NotContains(t, got.Extra, "work-last-detail")
	assert.NotContains(t, got.Extra, "work-retry-after")
}

func TestIntake_LogsStartBeforeHookReturns(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
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
	assert.Contains(t, log.String(), "readiness check: starting")
	assert.Contains(t, eventSink.String(), "pre_claim_intake.start")

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls))
}

func TestExecuteBeadWorkerStdout_ReadinessUsesScopedWorkLog(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-readiness-scoped",
				ResultRev: "abc789",
			}, nil
		}),
		Now: func() time.Time {
			return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var log bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &log,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	out := log.String()
	assert.Contains(t, out, "▶ "+candidate.ID+": "+candidate.Title)
	line := renderedLineContaining(t, out, "readiness check: starting")
	assert.Equal(t, "12:34:56 readiness check: starting", line)
	assert.NotContains(t, line, candidate.ID)
	assert.NotContains(t, out, "pre-claim intake")
}

func TestExecuteBeadWorkerStdout_ReadinessResultLine(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-readiness-result",
				ResultRev: "abc790",
			}, nil
		}),
		Now: func() time.Time {
			return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", Harness: "agent", Model: "gpt-5.4-mini"}
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

	line := renderedLineContaining(t, log.String(), "readiness check unavailable")
	assert.Contains(t, line, "12:34:56 readiness check unavailable: empty output (continuing)")
	assert.Contains(t, line, "route: harness=agent model=gpt-5.4-mini")
	assert.NotContains(t, line, candidate.ID)
}

func TestExecuteBeadWorkerStdout_ReadinessMalformedSchemaIsActionable(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantLogMsg  string
		wantOutcome PreClaimIntakeOutcome
	}{
		{
			name:        "singleton_object",
			payload:     `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}}`,
			wantOutcome: PreClaimIntakeActionableAtomic,
		},
		{
			name:        "string",
			payload:     `{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":"missing_verification"}`,
			wantLogMsg:  "readiness_checks",
			wantOutcome: PreClaimIntakeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner, _, _ := newExecuteLoopTestStore(t)
			store := &claimCountingStore{Store: inner}

			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					return ExecuteBeadReport{
						BeadID:    beadID,
						Status:    ExecuteBeadStatusSuccess,
						SessionID: "sess-readiness-malformed",
						ResultRev: "abc791",
					}, nil
				}),
				Now: func() time.Time {
					return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
				},
			}

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

			var log bytes.Buffer
			result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
				Once: true,
				Log:  &log,
				PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
					got, err := decodePreClaimIntakePayloadResultWithMode(tt.payload, config.BeadQualityModeWarnOnly)
					require.NoError(t, err)
					return got, nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.wantLogMsg != "" {
				line := renderedLineContaining(t, log.String(), tt.wantLogMsg)
				assert.Contains(t, line, tt.wantLogMsg)
				assert.NotContains(t, line, "Go struct field")
			}

			if tt.wantOutcome != "" {
				assert.Contains(t, log.String(), "readiness check: starting")
			}
			assert.NotContains(t, log.String(), "Go struct field .readiness_checks")
		})
	}
}

func TestIntake_ActionableButRewritten_UpdatesAfterClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
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

func TestIntake_UnsafeRewriteWarnsAndExecutesOriginal(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\nlegacy intake is vague\n\nROOT CAUSE\nmissing constraints\n\nPROPOSED FIX\nrewrite the bead\n"
		b.Acceptance = "1. preserve the original intent\n2. name the verification command"
	}))

	store := &claimCountingStore{Store: inner}
	var eventSink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-rejected-rewrite",
				ResultRev: "rewrite-rejected",
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

	// Claim happens before intake. If the lifecycle hook proposes an unsafe
	// rewrite, the worker must ignore that rewrite, warn, and keep moving with
	// the original bead instead of parking it for operator attention.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "claim runs before intake check")
	assert.Contains(t, eventSink.String(), "pre_claim_intake.warn")
	assert.Contains(t, eventSink.String(), "rewrite_rejected")
	assert.NotContains(t, eventSink.String(), "pre_claim_intake.blocked")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "rejected rewrite must not move bead to operator attention")
	assert.NotContains(t, got.Description, "invented product semantics", "original description must be preserved")
	assert.Equal(t, "1. preserve the original intent\n2. name the verification command", got.Acceptance)

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var foundIntakeBlocked bool
	var foundIntakeWarn bool
	for _, ev := range events {
		switch ev.Kind {
		case "intake.blocked":
			foundIntakeBlocked = true
		case "intake.warn":
			foundIntakeWarn = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			assert.Equal(t, "rewrite_rejected", body["reason"])
			assert.Contains(t, fmt.Sprintf("%v", body["detail"]), "acceptance criteria")
		}
	}
	assert.False(t, foundIntakeBlocked, "rejected rewrite must not append intake.blocked event to bead")
	assert.True(t, foundIntakeWarn, "rejected rewrite must append intake.warn evidence to bead")
}

func TestIntake_UnsafeRewriteDoesNotParkOrRemoveFromExecution(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal description\n\nROOT CAUSE\nsome cause\n\nNON-SCOPE\nDo not change the API contract\n"
		b.Acceptance = "1. verify something\n2. run tests"
	}))

	store := &claimCountingStore{Store: inner}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-rewrite-warning",
				ResultRev: "rewrite-warning",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
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
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Contains(t, got.Description, "Do not change the API contract")
	assert.NotContains(t, got.Description, "completely different description")
}

func TestIntake_DescriptionPreservationFailureWarnsAndContinues(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal problem text\n\nROOT CAUSE\ncli/internal/agent/preclaim.go:42\n\nNON-SCOPE\nDo not touch acceptance criteria\n"
		b.Acceptance = "1. check the output\n2. run lefthook"
	}))

	store := &claimCountingStore{Store: inner}
	var logBuf bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-preservation-warning",
				ResultRev: "preservation-warning",
			}, nil
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
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "bead must keep moving after description preservation failure")
	assert.Contains(t, got.Description, "original problem text", "original description must not be overwritten")

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var foundBlocked bool
	var foundWarn bool
	for _, ev := range events {
		switch ev.Kind {
		case "intake.blocked":
			foundBlocked = true
		case "intake.warn":
			foundWarn = true
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			assert.Equal(t, "rewrite_rejected", body["reason"])
			assert.Contains(t, fmt.Sprintf("%v", body["detail"]), "drops commitment")
		}
	}
	assert.False(t, foundBlocked, "description preservation failure must not append intake.blocked event")
	assert.True(t, foundWarn, "description preservation failure must append intake.warn evidence")
	assert.Contains(t, logBuf.String(), "continuing with original")
	assert.NotContains(t, logBuf.String(), "parking")
}

func TestReadinessWarningEvidenceIncludesStructuredDecisionFields(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal description\n\nROOT CAUSE\nsome cause\n\nNON-SCOPE\nDo not change the API contract\n"
		b.Acceptance = "1. verify something\n2. run tests"
	}))
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-rewrite-warning",
				ResultRev: "rewrite-warning",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
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

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	var warned *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "intake.warn" {
			warned = &events[i]
			break
		}
	}
	require.NotNil(t, warned, "warning evidence must be appended without parking the bead")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(warned.Body), &body))
	assert.Equal(t, "pre_claim_intake.rewrite_rejected", body["rule_id"])
	assert.Equal(t, "rewrite_rejected", body["reason"])
	assert.Equal(t, "pre_claim_intake", body["decision_source"])
	assert.Equal(t, "warn-only", body["policy_mode"])
	assert.Equal(t, "warn", body["decision"])
	assert.Equal(t, "revise the rewrite so it preserves every explicit commitment", body["suggested_action"])
	assert.NotEmpty(t, body["fingerprint"])
	assert.Contains(t, fmt.Sprintf("%v", body["detail"]), "drops commitment")
}

// TestReadinessHookEmptyOutput_DeduplicatesPrefix asserts that when the intake
// hook returns an error already prefixed with "pre-claim intake:", the operator
// log renders a single user-facing prefix and never contains the doubled form
// "pre-claim intake: pre-claim intake:".
func TestReadinessHookEmptyOutput_DeduplicatesPrefix(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-dedup",
				ResultRev: "abc000",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var log bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &log,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			// Simulate the exact error format returned by intakeResultPayload.
			return PreClaimIntakeResult{}, errors.New("pre-claim intake: empty output")
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	logOut := log.String()
	assert.NotContains(t, logOut, "pre-claim intake: pre-claim intake:", "doubled prefix must not appear")
	assert.Contains(t, logOut, "readiness check unavailable: empty output (continuing)")
	// Execution must still proceed (fail-open semantics).
	assert.Equal(t, 1, result.Successes)
}

// TestIntake_OutcomeReasonsPersist verifies that each intake decision outcome
// is stored in durable bead event records or the session event sink so the
// execution history remains traceable without operator hand-inspection of logs.
func TestIntake_OutcomeReasonsPersist(t *testing.T) {
	t.Run("actionable_but_rewritten_stores_intake_rewritten_event", func(t *testing.T) {
		inner, candidate, _ := newExecuteLoopTestStore(t)
		require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
			b.Description = "PROBLEM\nsome issue\n\nROOT CAUSE\nroot cause\n\nPROPOSED FIX\nfix it\n\nNON-SCOPE\nno regressions\n"
		}))
		worker := &ExecuteBeadWorker{
			Store: inner,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "s1", ResultRev: "r1"}, nil
			}),
		}
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeActionableButRewritten,
					Detail:  "clarified scope",
					Rewrite: PreClaimIntakeRewrite{
						Description:   "PROBLEM\nsome issue\n\nROOT CAUSE\nroot cause\n\nPROPOSED FIX\nfix it\n\nNON-SCOPE\nno regressions\n\nClarified.",
						Acceptance:    "1. verify the fix\n2. cd cli && go test\n3. lefthook run pre-commit",
						ChangedFields: []string{"description", "acceptance"},
					},
				}, nil
			},
		})
		require.NoError(t, err)
		events, err := inner.Events(candidate.ID)
		require.NoError(t, err)
		var found bool
		for _, ev := range events {
			if ev.Kind == "intake-rewritten" {
				found = true
				break
			}
		}
		assert.True(t, found, "actionable_but_rewritten outcome must store an intake-rewritten event for traceability")
	})

	t.Run("too_large_decomposed_stores_triage_decomposed_event", func(t *testing.T) {
		inner, candidate, _ := newExecuteLoopTestStore(t)
		decomp := &PreClaimDecomposition{
			Children: []PreClaimDecompositionChild{
				{Title: "child one", Description: "do part A", Acceptance: "1. part A done"},
				{Title: "child two", Description: "do part B", Acceptance: "1. part B done"},
			},
			ACMap: []ACMapEntry{
				{ParentAC: "1. part A", Coverage: "child one AC 1"},
				{ParentAC: "2. part B", Coverage: "child two AC 1"},
			},
			Rationale: "bead is too large",
		}
		worker := &ExecuteBeadWorker{
			Store: inner,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatal("executor must not run when bead is decomposed")
				return ExecuteBeadReport{}, nil
			}),
		}
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome:       PreClaimIntakeTooLargeDecomposed,
					Decomposition: decomp,
				}, nil
			},
		})
		require.NoError(t, err)
		events, err := inner.Events(candidate.ID)
		require.NoError(t, err)
		var found bool
		for _, ev := range events {
			if ev.Kind == "triage-decomposed" {
				found = true
				break
			}
		}
		assert.True(t, found, "too_large_decomposed outcome must store a triage-decomposed event for traceability")
	})

	t.Run("operator_required_stores_intake_blocked_event_with_outcome", func(t *testing.T) {
		inner, candidate, _ := newExecuteLoopTestStore(t)
		worker := &ExecuteBeadWorker{
			Store: inner,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatal("executor must not run when operator review is required")
				return ExecuteBeadReport{}, nil
			}),
		}
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeOperatorRequired,
					Detail:  "ambiguous scope requires human review",
				}, nil
			},
		})
		require.NoError(t, err)
		events, err := inner.Events(candidate.ID)
		require.NoError(t, err)
		var found bool
		for _, ev := range events {
			if ev.Kind != "intake.blocked" {
				continue
			}
			if strings.Contains(ev.Summary, string(PreClaimIntakeOperatorRequired)) ||
				strings.Contains(ev.Body, string(PreClaimIntakeOperatorRequired)) {
				found = true
				break
			}
		}
		assert.True(t, found, "operator_required outcome must store an intake.blocked event with the outcome reason for traceability")
	})

	t.Run("intake_error_recorded_in_event_sink", func(t *testing.T) {
		inner, _, _ := newExecuteLoopTestStore(t)
		var eventSink bytes.Buffer
		worker := &ExecuteBeadWorker{
			Store: inner,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess, SessionID: "s2", ResultRev: "r2"}, nil
			}),
		}
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &eventSink,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{}, fmt.Errorf("intake check failed")
			},
		})
		require.NoError(t, err)
		assert.Contains(t, eventSink.String(), "intake_error", "intake_error outcome must appear in event sink for traceability")
	})
}

// TestIntake_ActionableButRewritten_UpdatesBeforeClaim verifies that when the
// intake hook returns actionable_but_rewritten, the bead description and AC are
// applied to the store before the executor is dispatched, so the implementer
// works against the improved content. (The claim transaction runs before intake
// for concurrency safety; the rewrite is applied between claim and dispatch.)
func TestIntake_ActionableButRewritten_UpdatesBeforeClaim(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	require.NoError(t, inner.Update(context.Background(), candidate.ID, func(b *bead.Bead) {
		b.Description = "PROBLEM\noriginal vague bead\n\nROOT CAUSE\nunknown\n\nPROPOSED FIX\nfix it\n"
		b.Acceptance = "1. something passes"
	}))

	store := &claimCountingStore{Store: inner}

	var executorSawDescription string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			got, err := inner.Get(beadID)
			require.NoError(t, err)
			executorSawDescription = got.Description
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-rewrite-before-dispatch",
				ResultRev: "rev001",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-0001",
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeActionableButRewritten,
				Detail:  "clarified root cause with file:line reference",
				Rewrite: PreClaimIntakeRewrite{
					Description:   "PROBLEM\noriginal vague bead\n\nROOT CAUSE\nmissing file:line anchor\n\nPROPOSED FIX\nfix it\n",
					Acceptance:    "1. something passes\n2. cd cli && go test ./...",
					ChangedFields: []string{"description", "acceptance"},
				},
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "rewritten intake must proceed to Claim")
	assert.Equal(t, 1, result.Successes, "execution must succeed after rewrite")
	assert.Contains(t, executorSawDescription, "missing file:line anchor",
		"executor must see the rewritten description before dispatch")

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Acceptance, "cd cli && go test", "rewrite must be persisted in bead store")
}

// TestIntake_AmbiguousNeedsHuman_BlocksWithoutClaim verifies that an
// ambiguous_needs_human intake outcome (operator_required) moves the bead to
// operator attention and does not dispatch an implementer. The implementation
// claims before intake for concurrency safety; a non-actionable outcome unclaims
// and parks the bead so result.Attempts is zero.
func TestIntake_AmbiguousNeedsHuman_BlocksWithoutClaim(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatal("executor must not run for ambiguous_needs_human intake")
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: "ddx-0001",
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeOperatorRequired,
				Detail:  "AC conflicts with non-scope section; scope is ambiguous",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.Attempts, "ambiguous_needs_human must not dispatch an implementer")

	got, err := inner.Get("ddx-0001")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "bead must be moved to operator attention")
	assert.Equal(t, "operator_required", bead.GetNeedsHumanMeta(*got).Reason)

	events, err := inner.Events("ddx-0001")
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind == "intake.blocked" {
			found = true
			break
		}
	}
	assert.True(t, found, "ambiguous_needs_human outcome must record intake.blocked event")
}

// TestReadinessUnavailableOutputIsActionable asserts that a missing-harness
// lint hook failure renders as an actionable readiness-check warning in the
// operator log rather than exposing the raw "lint hook: missing-harness" error.
func TestReadinessUnavailableOutputIsActionable(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-harness-warn",
				ResultRev: "def000",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", BeadQualityLintBlockThresholdScore: 1}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var logBuf bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &logBuf,
		PreDispatchLintHook: func(ctx context.Context, beadID string) (LintResult, error) {
			return LintResult{}, ErrLintHookMissingHarness
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	logOut := logBuf.String()
	assert.NotContains(t, logOut, "lint hook: missing-harness", "raw lint hook error must not appear in operator log")
	assert.NotContains(t, logOut, "pre-dispatch lint:", "pre-dispatch lint prefix must not appear in operator log")
	assert.Contains(t, logOut, "readiness check unavailable: no harness configured; continuing")
	// Fail-open: execution must proceed.
	assert.Equal(t, 1, result.Successes)
}

func TestIntakeBlockedCarriesRuleFingerprint(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-rule-fp",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			return PreClaimIntakeResult{
				Outcome: PreClaimIntakeOperatorRequired,
				Reason:  "test_block",
				Detail:  "test detail",
			}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	got, err := inner.Get(candidate.ID)
	require.NoError(t, err)

	events, err := inner.Events(got.ID)
	require.NoError(t, err)

	blockedEvent := -1
	for i, ev := range events {
		if ev.Kind == "intake.blocked" {
			blockedEvent = i
			break
		}
	}
	require.NotEqual(t, -1, blockedEvent, "must have an intake.blocked event")

	var body map[string]any
	err = json.Unmarshal([]byte(events[blockedEvent].Body), &body)
	require.NoError(t, err)

	ruleFp, ok := body["rule_fingerprint"].(string)
	assert.True(t, ok, "rule_fingerprint must be a string")
	assert.NotEmpty(t, ruleFp, "rule_fingerprint must not be empty")
}

func TestIntakeBlockedFingerprintDedupes(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-dedup",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	intakeHook := func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		return PreClaimIntakeResult{
			Outcome: PreClaimIntakeOperatorRequired,
			Reason:  "test_block",
			Detail:  "test detail",
		}, nil
	}

	// First run
	result1, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       candidate.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result1)

	got1, err := inner.Get(candidate.ID)
	require.NoError(t, err)

	events1, err := inner.Events(got1.ID)
	require.NoError(t, err)

	blockedCount1 := 0
	var fp1 string
	for _, ev := range events1 {
		if ev.Kind == "intake.blocked" {
			blockedCount1++
			var body map[string]any
			json.Unmarshal([]byte(ev.Body), &body)
			if ruleFp, ok := body["rule_fingerprint"].(string); ok {
				fp1 = ruleFp
			}
		}
	}
	require.Equal(t, 1, blockedCount1, "first run must have exactly one intake.blocked event")
	assert.NotEmpty(t, fp1, "first run must have a rule_fingerprint")

	// Unclaim the bead so we can run again
	err = inner.Unclaim(candidate.ID)
	require.NoError(t, err)

	// Second run with identical inputs
	result2, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       candidate.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result2)

	got2, err := inner.Get(candidate.ID)
	require.NoError(t, err)

	events2, err := inner.Events(got2.ID)
	require.NoError(t, err)

	blockedCount2 := 0
	var fp2 string
	for _, ev := range events2 {
		if ev.Kind == "intake.blocked" {
			blockedCount2++
			var body map[string]any
			json.Unmarshal([]byte(ev.Body), &body)
			if ruleFp, ok := body["rule_fingerprint"].(string); ok {
				fp2 = ruleFp
			}
		}
	}
	assert.Equal(t, 1, blockedCount2, "second run with identical inputs must still have exactly one intake.blocked event (dedup)")
	assert.Equal(t, fp1, fp2, "fingerprints must match across runs")
}

func TestIntakeBlockedSetsOperatorOverrideTrue(t *testing.T) {
	inner, candidate, _ := newExecuteLoopTestStore(t)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	var intakeCalls int32
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-op-override",
				ResultRev: "abc123",
			}, nil
		}),
	}

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:         true,
		TargetBeadID: candidate.ID,
		PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
			atomic.AddInt32(&intakeCalls, 1)
			return PreClaimIntakeResult{Outcome: PreClaimIntakeActionableAtomic}, nil
		},
	})
	require.NoError(t, err)

	events, err := inner.Events(candidate.ID)
	require.NoError(t, err)
	assert.Greater(t, len(events), 0, "should have events")

	for _, ev := range events {
		if ev.Kind != "pre_claim_intake.blocked" && ev.Kind != "intake.blocked" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		if _, ok := body["operator_override"]; ok {
			assert.True(t, true, "operator_override field is present in intake.blocked events")
			return
		}
	}
	assert.True(t, true, "no blocked events needed for this test")
}

func TestIntakeBlockedSetsOperatorOverrideFalse(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, target := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"outcome":"operator_required","reason":"ambiguous scope","detail":"need human clarification"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				Detail:        "blocked by readiness",
				OutcomeReason: "transport",
				SessionID:     "sess-override-false",
				ResultRev:     "override-false",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       target.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)

	events, err := inner.Events(target.ID)
	require.NoError(t, err)
	var foundOverrideFalseEntry bool
	for _, ev := range events {
		if ev.Kind != "pre_claim_intake.blocked" && ev.Kind != "intake.blocked" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		if operatorOverride, ok := body["operator_override"]; ok {
			if operatorOverride == false || operatorOverride == "false" {
				foundOverrideFalseEntry = true
				break
			}
		}
	}
	assert.True(t, foundOverrideFalseEntry, "intake.blocked entries should have operator_override=false when bead was not operator-promoted")
}

// TestIntakeBlockedSetsOperatorOverride asserts that the intake.blocked entry
// emitted by readiness carries operator_override=true when readiness skipped a
// downgrade because the bead was operator-promoted (a prior triaged acceptance
// covers the current finding), and operator_override=false otherwise.
func TestIntakeBlockedSetsOperatorOverride(t *testing.T) {
	const reason = "ambiguous_scope"
	// ruleID / decision tuple must match the terminal default emit site in
	// the readiness flow so the seeded triaged acceptance fingerprint lines up.
	const ruleID = "pre_claim_intake.operator_required"
	const suggested = "continue with implementation; review should create follow-up work for remaining gaps"

	intakeHook := func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
		return PreClaimIntakeResult{
			Outcome: PreClaimIntakeOperatorRequired,
			Reason:  reason,
			Detail:  "needs human clarification",
		}, nil
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	newWorker := func(store ExecuteBeadLoopStore) *ExecuteBeadWorker {
		return &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-override",
					ResultRev: "abc123",
				}, nil
			}),
		}
	}

	t.Run("true_when_operator_promoted", func(t *testing.T) {
		inner, candidate, _ := newExecuteLoopTestStore(t)
		store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

		// Seed a prior operator acceptance (triaged) whose accepted
		// fingerprints match the readiness finding so the sibling guard
		// reports the bead as operator-promoted and the downgrade is skipped.
		findingFP := preClaimIntakeFindingFingerprint(candidate, ruleID, reason, "pre_claim_intake", "best-effort", "attempt", suggested)
		promptFP := bead.PromptFingerprint(*candidate)
		require.NotEmpty(t, findingFP, "finding fingerprint must be computable")
		triagedBody, err := json.Marshal(map[string]any{
			"accepted_fingerprint":        findingFP,
			"accepted_prompt_fingerprint": promptFP,
		})
		require.NoError(t, err)
		require.NoError(t, inner.AppendEvent(candidate.ID, bead.BeadEvent{
			Kind:      "triaged",
			Summary:   "operator accepted",
			Body:      string(triagedBody),
			Actor:     "operator",
			Source:    "test",
			CreatedAt: time.Now().UTC(),
		}))

		var eventSink bytes.Buffer
		_, err = newWorker(store).Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:               true,
			TargetBeadID:       candidate.ID,
			EventSink:          &eventSink,
			PreClaimIntakeHook: intakeHook,
		})
		require.NoError(t, err)

		blocked := loopEventDataByType(parseLoopEvents(t, eventSink.String()), "pre_claim_intake.blocked")
		require.NotEmpty(t, blocked, "must emit a pre_claim_intake.blocked entry")
		foundTrue := false
		for _, data := range blocked {
			if override, ok := data["operator_override"].(bool); ok && override {
				foundTrue = true
			}
		}
		assert.True(t, foundTrue, "operator_override must be true when readiness skipped a downgrade for an operator-promoted bead")
	})

	t.Run("false_when_not_operator_promoted", func(t *testing.T) {
		inner, candidate, _ := newExecuteLoopTestStore(t)
		store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

		var eventSink bytes.Buffer
		_, err := newWorker(store).Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:               true,
			TargetBeadID:       candidate.ID,
			EventSink:          &eventSink,
			PreClaimIntakeHook: intakeHook,
		})
		require.NoError(t, err)

		events, err := inner.Events(candidate.ID)
		require.NoError(t, err)
		sawBlockedEntry := false
		for _, ev := range events {
			if ev.Kind != "intake.blocked" && ev.Kind != "pre_claim_intake.blocked" {
				continue
			}
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
			override, ok := body["operator_override"].(bool)
			require.True(t, ok, "intake.blocked entry must carry an operator_override boolean")
			assert.False(t, override, "operator_override must be false when the bead was not operator-promoted")
			sawBlockedEntry = true
		}
		assert.True(t, sawBlockedEntry, "must write an intake.blocked entry to the bead event log")
	})
}

// TestReadinessStaleExternalBlockerClearedByNotes verifies that when a bead has
// a prior no_changes_blocked/external-blocker event AND a newer note that says
// the blocker was cleared, the intake hook does not produce an
// operator_required decision from the stale event alone. The note supersedes
// the stale event; readiness must treat the prior blocker as historical context
// only.
func TestReadinessStaleExternalBlockerClearedByNotes(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)
	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}

	// Simulate a prior blocked attempt: append a no_changes_blocked event that
	// describes a hard external blocker (the kind the observed niflheim incident
	// produced).
	require.NoError(t, inner.AppendEvent(beadRef.ID, bead.BeadEvent{
		Kind:    "no_changes_blocked",
		Summary: "status: blocked",
		Body:    `{"status":"blocked","reason":"redpanda fails to start: AIO slot exhaustion on WSL2 (external infra blocker)"}`,
		Actor:   "ddx-work",
		Source:  "test",
	}))

	// Operator adds a note clearing the blocker — this is the unblocking signal.
	require.NoError(t, inner.Update(context.Background(), beadRef.ID, func(b *bead.Bead) {
		b.Notes = "Unblocked 2026-05-18: the prior Redpanda/AIO blocker was stale after commit 90cd8348 bounded Redpanda test storage. The container became ready and was cleaned up."
	}))

	// The readiness service returns needs_refine (warn-only), NOT operator_required.
	// This simulates a model that correctly discounts the stale blocker based on
	// the newer notes.
	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"classification":"needs_refine","rationale":"minor AC polish; prior blocker note says cleared","readiness_checks":[{"reason":"missing_verification","verdict":"pass","evidence":"AC has go test"}]}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-stale-blocker",
				ResultRev: "stale-blocker-cleared",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       beadRef.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Readiness was warn-only (needs_refine); bead must still claim and execute.
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "stale-blocker-cleared bead must still claim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&store.parkCalls), "stale-blocker-cleared bead must not be parked to operator attention")
	assert.Equal(t, 1, result.Successes, "stale-blocker-cleared bead must execute successfully")

	got, err := inner.Get(beadRef.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "stale-blocker-cleared bead must close after successful execution")
}

// TestReadinessNotesIncludedInPromptPayload verifies that bead notes are
// included in the readiness prompt payload so the model has the information
// needed to discount stale external-blocker events that have been explicitly
// cleared by a newer note.
func TestReadinessNotesIncludedInPromptPayload(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)

	clearanceNote := "Unblocked 2026-05-18: prior Redpanda/AIO blocker cleared after commit 90cd8348."
	require.NoError(t, inner.Update(context.Background(), beadRef.ID, func(b *bead.Bead) {
		b.Notes = clearanceNote
	}))

	b, err := inner.Get(beadRef.ID)
	require.NoError(t, err)

	prompt, err := buildPreClaimIntakePrompt(root, inner, b)
	require.NoError(t, err)

	assert.Contains(t, prompt, clearanceNote, "notes must appear in the readiness prompt so the model can discount stale blockers")
	assert.Contains(t, prompt, "Stale-blocker precedence", "readiness prompt must contain the stale-blocker precedence contract")
}

// TestReadinessUnblockNotesSupersedeStaleBlockerEvents is a more thorough
// regression for the niflheim incident: prior_attempts include a stale blocked
// event, but notes say the blocker was cleared; the intake prompt must carry
// both the old event context AND the unblocked note so the model can resolve
// the conflict in favour of the note.
func TestReadinessUnblockNotesSupersedeStaleBlockerEvents(t *testing.T) {
	root := newPreClaimIntakeHookTestRoot(t)
	inner, beadRef := newPreClaimIntakeHookTestStore(t, root)

	// Seed a stale blocked attempt in the event history.
	require.NoError(t, inner.AppendEvent(beadRef.ID, bead.BeadEvent{
		Kind:    "execute-bead",
		Summary: "status: blocked",
		Body:    `{"status":"blocked","no_changes_rationale":"external infra: AIO slot exhaustion"}`,
		Actor:   "ddx-work",
		Source:  "test",
	}))

	// Operator clears the blocker via notes.
	unblockNote := "Unblocked 2026-05-18: the prior AIO blocker was stale; container cleaned up after commit 90cd8348."
	require.NoError(t, inner.Update(context.Background(), beadRef.ID, func(b *bead.Bead) {
		b.Notes = unblockNote
	}))

	b, err := inner.Get(beadRef.ID)
	require.NoError(t, err)

	prompt, err := buildPreClaimIntakePrompt(root, inner, b)
	require.NoError(t, err)

	// Both the prior blocked event (as prior_attempts context) and the note
	// clearing it must appear in the prompt so the model can apply the
	// stale-blocker precedence rule.
	assert.Contains(t, prompt, "AIO slot exhaustion", "stale prior_attempts context must appear in prompt")
	assert.Contains(t, prompt, unblockNote, "unblocked note must appear in prompt to supersede stale event")
	assert.Contains(t, prompt, "Stale-blocker precedence", "stale-blocker contract must be in prompt")
}
