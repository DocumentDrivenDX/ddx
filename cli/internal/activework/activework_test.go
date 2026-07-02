package activework

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
