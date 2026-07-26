package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedWorkerRecordsProcessGroup(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, "running", record.State)
	require.Greater(t, record.PID, 0)
	require.Greater(t, record.PGID, 0)
	assert.Equal(t, record.PID, record.PGID, "managed worker should record the process-group root pid")

	statusPath := filepath.Join(ddxroot.JoinProject(root, "workers", record.ID), "status.json")
	data, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	var persisted WorkerRecord
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, record.PID, persisted.PID)
	assert.Equal(t, record.PGID, persisted.PGID)
	assert.NotZero(t, persisted.PID)
	assert.NotZero(t, persisted.PGID)

	t.Cleanup(func() {
		_ = cleanupManagedWorkerProcessTree(record.PID, nil, 0)
		_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)
	})
}

// TestManagedWorkerProcessGroupLaunchFailureLeavesActionableStatus proves a
// subprocess launch failure does not leave a server-managed worker looking
// live with PID zero / missing PGID. The pre-launch "running" snapshot is
// replaced by a terminal failed status with an actionable error.
func TestManagedWorkerProcessGroupLaunchFailureLeavesActionableStatus(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)
	missingBinary := filepath.Join(root, "no-such-ddx-binary")
	t.Setenv("DDX_BIN", missingBinary)

	m := NewWorkerManager(root)
	defer m.StopWatchdog()
	m.enableManagedLaunch()

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeWatch,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DDX_BIN")
	assert.Empty(t, record.ID, "failed launch must not return a live worker record")
	assert.Zero(t, record.PID)
	assert.Zero(t, record.PGID)

	// In-memory map must not retain a running handle for the failed launch.
	m.mu.Lock()
	activeCount := len(m.workers)
	m.mu.Unlock()
	assert.Zero(t, activeCount, "failed launch must not leave an active worker handle")

	listed, listErr := m.List()
	require.NoError(t, listErr)
	require.NotEmpty(t, listed, "launch failure should leave an actionable status.json on disk")

	foundFailed := false
	for _, rec := range listed {
		if rec.State == "running" {
			t.Fatalf("launch failure left a running worker looking live: id=%s pid=%d pgid=%d", rec.ID, rec.PID, rec.PGID)
		}
		if rec.State != "failed" {
			continue
		}
		foundFailed = true
		assert.Equal(t, "failed", rec.Status)
		assert.Zero(t, rec.PID, "failed launch must not look live with a process id")
		assert.Zero(t, rec.PGID, "failed launch must not look live with a process group")
		assert.NotEmpty(t, rec.Error, "failed launch must leave an actionable error")
		assert.Contains(t, rec.Error, "DDX_BIN")

		statusPath := filepath.Join(ddxroot.JoinProject(root, "workers", rec.ID), "status.json")
		data, readErr := os.ReadFile(statusPath)
		require.NoError(t, readErr)

		var persisted WorkerRecord
		require.NoError(t, json.Unmarshal(data, &persisted))
		assert.Equal(t, "failed", persisted.State)
		assert.Equal(t, "failed", persisted.Status)
		assert.Zero(t, persisted.PID)
		assert.Zero(t, persisted.PGID)
		assert.NotEmpty(t, persisted.Error)
		assert.Contains(t, persisted.Error, "DDX_BIN")
	}
	assert.True(t, foundFailed, "expected a failed worker status after launch failure")
}
