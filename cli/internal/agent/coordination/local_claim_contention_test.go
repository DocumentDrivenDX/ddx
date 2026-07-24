package coordination_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/coordination"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinationContract_LocalClaimContention verifies the coordination
// claim contract against the local implementation using a real temporary bead
// store and git repository (ddx-27ac6bcb).
//
// Two LocalCoordinator instances share one real store. The first claim wins
// (OutcomeApplied); the contending claim receives the contract-defined
// OutcomeConflict / ReasonAlreadyClaimed without call-recording mocks.
// Production LocalCoordinator.Claim invokes bead.Store.Claim.
func TestCoordinationContract_LocalClaimContention(t *testing.T) {
	projectRoot := t.TempDir()
	initGitRepo(t, projectRoot)
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	const beadID = "ddx-claim-contention-001"
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    beadID,
		Title: "coordination claim contention fixture",
	}))

	// Seed commit so the worktree is a real git repository with tracker state,
	// matching how workers operate against an initialized project.
	runGit(t, projectRoot, "add", "-A")
	runGit(t, projectRoot, "commit", "-m", "seed claim contention fixture")

	// Two local coordinators share the same real store — no claim recorders.
	coordA := coordination.NewLocalCoordinator(store)
	coordB := coordination.NewLocalCoordinator(store)

	ctx := context.Background()
	first, err := coordA.Claim(ctx, coordination.ClaimRequest{
		BeadID:         beadID,
		Assignee:       "worker-a",
		IdempotencyKey: "claim-key-worker-a-1",
	})
	require.NoError(t, err)
	assert.Equal(t, coordination.OutcomeApplied, first.Code, "first claim must apply")
	assert.Equal(t, beadID, first.BeadID)
	assert.Equal(t, "worker-a", first.Owner)
	assert.Empty(t, first.Reason)

	second, err := coordB.Claim(ctx, coordination.ClaimRequest{
		BeadID:         beadID,
		Assignee:       "worker-b",
		IdempotencyKey: "claim-key-worker-b-1",
	})
	require.NoError(t, err, "contention is a contract outcome, not a hard error")
	assert.Equal(t, coordination.OutcomeConflict, second.Code, "contending claim must be conflict")
	assert.Equal(t, coordination.ReasonAlreadyClaimed, second.Reason)
	assert.Equal(t, beadID, second.BeadID)
	assert.Equal(t, "worker-a", second.Owner, "conflict reports the durable winning owner")

	// Durable store truth: only worker-a holds the claim.
	got, err := store.Get(ctx, beadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bead.StatusInProgress, got.Status)
	assert.Equal(t, "worker-a", got.Owner)
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Coordination Test")
	runGit(t, dir, "config", "user.email", "coordination@test.local")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
}
