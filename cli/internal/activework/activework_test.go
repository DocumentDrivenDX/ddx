package activework

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveWorkSnapshotIgnoresStaleWorkerSidecars(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	staleSidecar := &bead.Bead{ID: "ddx-active-stale-sidecar", Title: "Stale sidecar"}
	staleClaim := &bead.Bead{ID: "ddx-active-stale-claim", Title: "Stale claim"}
	require.NoError(t, store.Create(context.Background(), staleSidecar))
	require.NoError(t, store.Create(context.Background(), staleClaim))
	require.NoError(t, store.Claim(staleClaim.ID, "worker-stale"))

	oldClaimTTL := bead.HeartbeatTTL
	oldLivenessTTL := workerstatus.LivenessTTL
	t.Cleanup(func() {
		bead.HeartbeatTTL = oldClaimTTL
		workerstatus.LivenessTTL = oldLivenessTTL
	})
	bead.HeartbeatTTL = -time.Nanosecond
	workerstatus.LivenessTTL = time.Second

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-stale", workerstatus.LivenessRecord{
		WorkerID:       "worker-stale",
		ProjectRoot:    projectRoot,
		CurrentBead:    staleSidecar.ID,
		AttemptID:      "att-stale",
		Phase:          "running",
		LastActivityAt: time.Now().Add(-2 * time.Second).UTC(),
	}))

	snap, err := Collect(projectRoot, store, time.Now().UTC())
	require.NoError(t, err)

	assert.Empty(t, snap.Records, "stale sidecars and stale claims must not count as active work")
	assert.Empty(t, snap.BeadIDs)
	assert.Zero(t, snap.Count)
}

func TestActiveWorkSnapshotIgnoresFreshRunStateWhenPIDIsDead(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	now := time.Now().UTC()

	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-stale-run-state",
		AttemptID:    "20260613T034907-3d00f60f",
		StartedAt:    now.Add(-time.Minute),
		WorktreePath: filepath.Join(projectRoot, ".ddx-exec-wt", ".execute-bead-wt-ddx-stale-run-state"),
		PID:          deadActiveWorkPID(t),
		RefreshedAt:  now,
		ExpiresAt:    now.Add(time.Minute),
	}))

	snap, err := Collect(projectRoot, nil, now)
	require.NoError(t, err)
	assert.Empty(t, snap.Records, "fresh-looking run-state with a dead owner pid must not jam active work")
	assert.Empty(t, snap.BeadIDs)
	assert.Zero(t, snap.Count)
}

func TestActiveWorkMergeKeepsEqualBeadIDsAcrossProjects(t *testing.T) {
	now := time.Now().UTC()
	snap := Merge(
		Snapshot{Records: []Record{{
			ProjectRoot:    "/repo/a",
			WorkerID:       "worker-a",
			BeadID:         "same-bead",
			Source:         "claim",
			LastActivityAt: now,
		}}},
		Snapshot{Records: []Record{{
			ProjectRoot:    "/repo/b",
			WorkerID:       "worker-b",
			BeadID:         "same-bead",
			Source:         "claim",
			LastActivityAt: now.Add(time.Second),
		}}},
	)

	require.Equal(t, 2, snap.Count)
	assert.Equal(t, []string{"same-bead"}, snap.BeadIDs)
	byProject := make(map[string]Record, len(snap.Records))
	for _, rec := range snap.Records {
		byProject[rec.ProjectRoot] = rec
	}
	require.Contains(t, byProject, "/repo/a")
	require.Contains(t, byProject, "/repo/b")
	assert.Equal(t, "worker-a", byProject["/repo/a"].WorkerID)
	assert.Equal(t, "worker-b", byProject["/repo/b"].WorkerID)
}

func deadActiveWorkPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}
