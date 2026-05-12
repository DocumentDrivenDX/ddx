package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workLoopCommitmentSnapshot struct {
	Title       string
	Description string
	Acceptance  string
	Labels      []string
	Parent      string
	DepIDs      []string
}

func snapshotWorkLoopCommitments(b *bead.Bead) workLoopCommitmentSnapshot {
	if b == nil {
		return workLoopCommitmentSnapshot{}
	}
	return workLoopCommitmentSnapshot{
		Title:       b.Title,
		Description: b.Description,
		Acceptance:  b.Acceptance,
		Labels:      append([]string(nil), b.Labels...),
		Parent:      b.Parent,
		DepIDs:      append([]string(nil), b.DepIDs()...),
	}
}

func assertWorkLoopCommitmentsPreserved(t *testing.T, got *bead.Bead, want workLoopCommitmentSnapshot) {
	t.Helper()
	require.NotNil(t, got)
	assert.Equal(t, want.Title, got.Title)
	assert.Equal(t, want.Description, got.Description)
	assert.Equal(t, want.Acceptance, got.Acceptance)
	assert.Equal(t, want.Labels, got.Labels)
	assert.Equal(t, want.Parent, got.Parent)
	assert.Equal(t, want.DepIDs, got.DepIDs())
}

func newWorkLoopOperatorOverrideFixture(t *testing.T) (*bead.Store, *bead.Bead, *parkCountingStore, PreClaimIntakeHook, config.ResolvedConfig, workLoopCommitmentSnapshot, *preClaimIntakeHookServiceStub) {
	t.Helper()

	root := newPreClaimIntakeHookTestRoot(t)
	inner, target := newPreClaimIntakeHookTestStore(t, root)

	require.NoError(t, inner.Create(&bead.Bead{
		ID:       "ddx-override-parent",
		Title:    "epic: operator override regression parent",
		Status:   bead.StatusClosed,
		Priority: 1,
	}))
	require.NoError(t, inner.Create(&bead.Bead{
		ID:       "ddx-override-dep",
		Title:    "dependency: operator override regression prerequisite",
		Status:   bead.StatusClosed,
		Priority: 1,
	}))
	require.NoError(t, inner.Update(target.ID, func(b *bead.Bead) {
		b.Title = "work: preserve operator-promoted readiness commitments"
		b.Description = "PROBLEM\nA promoted bead must not be downgraded again when the same readiness finding reappears.\n\nROOT CAUSE\ncli/internal/agent/execute_bead_loop.go:1031-1309 still owns claim-time readiness and parking.\ncli/internal/agent/execute_bead_loop.go:2903-3118 implements the park-to-proposed bridge.\ncli/internal/bead/store.go:667-676 records operator acceptance when proposed becomes open.\n\nPROPOSED FIX\nExercise the real worker loop, promote proposed back to open through Store.SetLifecycleStatus, and prove later passes stay non-proposed.\n\nNON-SCOPE\nDo not add new production policy or bypass the readiness hook.\n"
		b.Acceptance = "1. TestWorkLoopOperatorPromotedOpenDoesNotDowngradeAgain\n2. TestWorkLoopOperatorOverridePreservesCommitmentText\n3. TestWorkLoopHardManualRequiredStillParks\n4. cd cli && go test ./internal/agent/... ./cmd/... -run \"TestWorkLoop(OperatorPromotedOpenDoesNotDowngradeAgain|OperatorOverridePreservesCommitmentText|HardManualRequiredStillParks)\" passes\n5. cd cli && go test ./internal/agent/... ./internal/bead/... ./cmd/... passes\n6. lefthook run pre-commit passes"
		b.Labels = []string{"phase:2", "area:agent", "area:bead-lifecycle", "area:tests", "kind:test", "prevention", "spec:FEAT-010"}
		b.Parent = "ddx-override-parent"
		b.Dependencies = nil
		b.AddDep("ddx-override-dep", "blocks")
	}))

	// Build the preservation baseline from the mutated bead, not the helper defaults.
	updated, err := inner.Get(target.ID)
	require.NoError(t, err)
	target = updated

	store := &parkCountingStore{claimCountingStore: &claimCountingStore{Store: inner}}
	svc := &preClaimIntakeHookServiceStub{
		finalText: `{"outcome":"operator_required","reason":"ambiguous scope","detail":"need human clarification"}`,
	}
	intakeHook := NewPreClaimIntakeHook(root, store, intakeHookTestConfig(), svc, nil)

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	want := workLoopCommitmentSnapshot{
		Title:       target.Title,
		Description: target.Description,
		Acceptance:  target.Acceptance,
		Labels:      append([]string(nil), target.Labels...),
		Parent:      target.Parent,
		DepIDs:      append([]string(nil), target.DepIDs()...),
	}

	return inner, target, store, intakeHook, rcfg, want, svc
}

func TestWorkLoopOperatorPromotedOpenDoesNotDowngradeAgain(t *testing.T) {
	inner, target, store, intakeHook, rcfg, want, svc := newWorkLoopOperatorOverrideFixture(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				Detail:        "still blocked by the same readiness finding",
				OutcomeReason: "transport",
				SessionID:     "sess-override-regression",
				ResultRev:     "override-regression",
			}, nil
		}),
	}

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       target.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "first readiness pass must park the bead")

	got, err := inner.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assertWorkLoopCommitmentsPreserved(t, got, want)

	events, err := inner.Events(target.ID)
	require.NoError(t, err)
	var sawParkedEvidence bool
	for _, ev := range events {
		if ev.Kind != "intake.blocked" {
			continue
		}
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(ev.Body), &body))
		if body["fingerprint"] != "" && body["prompt_fingerprint"] != "" {
			sawParkedEvidence = true
			break
		}
	}
	assert.True(t, sawParkedEvidence, "first readiness pass must record blocked evidence")

	require.NoError(t, inner.SetLifecycleStatus(target.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted readiness decision",
		Actor:  "reviewer",
		Source: "test",
	}))

	for i := 0; i < 2; i++ {
		_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:               true,
			TargetBeadID:       target.ID,
			PreClaimIntakeHook: intakeHook,
		})
		require.NoError(t, err)

		got, err = inner.Get(target.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, got.Status, "later pass %d must not re-park the bead", i+1)
		assertWorkLoopCommitmentsPreserved(t, got, want)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "operator acceptance must suppress repeat parking")
	assert.Equal(t, int32(3), atomic.LoadInt32(&store.claimCalls), "same finding must still flow through the real execute loop on each pass")
	assert.Equal(t, int32(3), atomic.LoadInt32(&svc.executeCalls), "same readiness hook path must be exercised on every pass")
}

func TestWorkLoopOperatorOverridePreservesCommitmentText(t *testing.T) {
	inner, target, store, intakeHook, rcfg, want, _ := newWorkLoopOperatorOverrideFixture(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				Detail:        "still blocked by the same readiness finding",
				OutcomeReason: "transport",
				SessionID:     "sess-override-preserve",
				ResultRev:     "override-preserve",
			}, nil
		}),
	}

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       target.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)

	got, err := inner.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)

	require.NoError(t, inner.SetLifecycleStatus(target.ID, bead.StatusOpen, bead.LifecycleTransitionOptions{
		Reason: "operator accepted readiness decision",
		Actor:  "reviewer",
		Source: "test",
	}))

	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:               true,
		TargetBeadID:       target.ID,
		PreClaimIntakeHook: intakeHook,
	})
	require.NoError(t, err)

	got, err = inner.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assertWorkLoopCommitmentsPreserved(t, got, want)
}

func TestWorkLoopHardManualRequiredStillParks(t *testing.T) {
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
			t.Fatal("executor must not run for operator_required intake")
			return ExecuteBeadReport{}, nil
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

	assert.Equal(t, int32(1), atomic.LoadInt32(&store.claimCalls), "hard manual-required readiness must still claim before parking")
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.parkCalls), "hard manual-required readiness must still park the bead")

	got, err := inner.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.Empty(t, got.Owner)
}
