package activework

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectIgnoresAttemptLessCandidateResolvingSidecars(t *testing.T) {
	projectRoot := t.TempDir()
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{ID: "ddx-active-candidate-resolving", Title: "Candidate resolving"}
	require.NoError(t, store.Create(context.Background(), candidate))

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, "worker-candidate", workerstatus.LivenessRecord{
		WorkerID:       "worker-candidate",
		ProjectRoot:    projectRoot,
		CurrentBead:    candidate.ID,
		Phase:          "resolving",
		PID:            os.Getpid(),
		LastActivityAt: time.Now().UTC(),
	}))

	snap, err := Collect(projectRoot, store, time.Now().UTC())
	require.NoError(t, err)

	assert.Zero(t, snap.Count, "attempt-less candidate resolving liveness must not count as active work")
	assert.Empty(t, snap.BeadIDs)
	assert.Empty(t, snap.Records)
}
