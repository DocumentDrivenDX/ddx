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
