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

	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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

	require.NoError(t, inner.Update(beadRef.ID, func(b *bead.Bead) {
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
					got, err := decodePreClaimIntakePayloadResult(tt.payload)
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

func TestIntake_UnsafeRewriteWarnsAndExecutesOriginal(t *testing.T) {
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
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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
		require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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
	require.NoError(t, inner.Update(candidate.ID, func(b *bead.Bead) {
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
