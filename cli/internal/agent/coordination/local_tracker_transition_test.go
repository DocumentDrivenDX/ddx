package coordination_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinationContract_LocalTrackerTransition verifies the coordination
// tracker-transition contract against the local implementation using a real
// temporary bead store and git repository (ddx-0ce3c378).
//
// Flow: claim an open bead, apply an allowed lifecycle transition via the
// local coordinator, then replay the same idempotency key and observe
// OutcomeAlreadyApplied without call-recording mocks. Production
// LocalCoordinator.Transition invokes bead.Store.SetLifecycleStatus
// (TransitionLifecycle).
func TestCoordinationContract_LocalTrackerTransition(t *testing.T) {
	projectRoot := t.TempDir()
	execRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", execRoot)
	initGitRepo(t, projectRoot)
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-tracker-transition-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "coordination tracker transition fixture",
	}))

	// Seed commit so the worktree is a real git repository with tracker state,
	// matching how workers operate against an initialized project.
	runGit(t, projectRoot, "add", "-A")
	runGit(t, projectRoot, "commit", "-m", "seed tracker transition fixture")

	coord := coordination.NewLocalCoordinator(store)
	ctx := context.Background()

	// Claim first so the bead is in_progress (worker path) before transition.
	claim, err := coord.Claim(ctx, coordination.ClaimRequest{
		BeadID:         beadID,
		Assignee:       "worker-transition",
		IdempotencyKey: "claim-key-transition-setup",
	})
	require.NoError(t, err)
	require.Equal(t, coordination.OutcomeApplied, claim.Code)
	got, err := store.Get(ctx, beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusInProgress, got.Status)

	// Allowed transition: in_progress -> open (release back to queue).
	const transitionKey = "transition-key-in-progress-to-open"
	first, err := coord.Transition(ctx, coordination.TransitionRequest{
		BeadID:         beadID,
		ToStatus:       bead.StatusOpen,
		IdempotencyKey: transitionKey,
		Reason:         "release after claim",
		Actor:          "worker-transition",
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeApplied, first.Code, "first transition must apply")
	assert.Equal(t, beadID, first.BeadID)
	assert.Equal(t, bead.StatusInProgress, first.FromStatus)
	assert.Equal(t, bead.StatusOpen, first.ToStatus)
	assert.Empty(t, first.Reason)
	assert.Equal(t, transitionKey, first.IdempotencyKey)

	// Durable store truth after applied transition.
	got, err = store.Get(ctx, beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusOpen, got.Status)

	// Rejected transition: invalid lifecycle status. The local coordinator
	// should remember the conflict outcome and replay it as already_applied.
	const conflictKey = "transition-key-invalid-status"
	conflict, err := coord.Transition(ctx, coordination.TransitionRequest{
		BeadID:         beadID,
		ToStatus:       "not-a-status",
		IdempotencyKey: conflictKey,
		Reason:         "invalid transition",
		Actor:          "worker-transition",
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeConflict, conflict.Code, "invalid transition must conflict")
	assert.Equal(t, beadID, conflict.BeadID)
	assert.Equal(t, bead.StatusOpen, conflict.FromStatus)
	assert.Equal(t, "not-a-status", conflict.ToStatus)
	assert.Equal(t, coordination.ReasonTransitionRejected, conflict.Reason)
	assert.Equal(t, conflictKey, conflict.IdempotencyKey)

	got, err = store.Get(ctx, beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusOpen, got.Status, "rejected transition must not mutate durable state")

	conflictReplay, err := coord.Transition(ctx, coordination.TransitionRequest{
		BeadID:         beadID,
		ToStatus:       "not-a-status",
		IdempotencyKey: conflictKey,
		Reason:         "invalid transition",
		Actor:          "worker-transition",
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeAlreadyApplied, conflictReplay.Code,
		"same-key replay must be already_applied")
	assert.Equal(t, beadID, conflictReplay.BeadID)
	assert.Equal(t, bead.StatusOpen, conflictReplay.FromStatus)
	assert.Equal(t, "not-a-status", conflictReplay.ToStatus)
	assert.Equal(t, coordination.ReasonTransitionRejected, conflictReplay.Reason)
	assert.Equal(t, conflictKey, conflictReplay.IdempotencyKey)

	// Replay same idempotency key: already_applied without re-mutating.
	replay, err := coord.Transition(ctx, coordination.TransitionRequest{
		BeadID:         beadID,
		ToStatus:       bead.StatusOpen,
		IdempotencyKey: transitionKey,
		Reason:         "release after claim",
		Actor:          "worker-transition",
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeAlreadyApplied, replay.Code,
		"same-key replay must be already_applied")
	assert.Equal(t, beadID, replay.BeadID)
	assert.Equal(t, bead.StatusInProgress, replay.FromStatus,
		"already_applied echoes the prior applied from_status")
	assert.Equal(t, bead.StatusOpen, replay.ToStatus)
	assert.Equal(t, transitionKey, replay.IdempotencyKey)

	// Store remains open after idempotent replay.
	got, err = store.Get(ctx, beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusOpen, got.Status)
}
