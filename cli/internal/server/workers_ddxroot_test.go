package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConventionRootWorkerProject(t *testing.T, projectRoot string) *bead.Store {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_EXEC_WT_DIR", ddxroot.JoinProject(projectRoot, "exec-worktrees"))

	stateRoot := ddxroot.Path(context.Background(), projectRoot)
	require.NotEqual(t, filepath.Join(projectRoot, ddxroot.DirName), stateRoot, "test must run in convention-root mode")
	require.NoError(t, os.MkdirAll(stateRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateRoot, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"wt\"\n"), 0o644))

	store := bead.NewStore(stateRoot)
	require.NoError(t, store.Create(&bead.Bead{
		ID:         "ddx-root-worker",
		Title:      "Worker DDx root coverage",
		Status:     bead.StatusOpen,
		IssueType:  bead.DefaultType,
		Acceptance: "worker should resolve state via ddxroot",
	}))

	initGitRepo(t, projectRoot)
	return store
}

func TestWorkerManagerUsesDDxRoot(t *testing.T) {
	projectRoot := t.TempDir()
	store := setupConventionRootWorkerProject(t, projectRoot)

	m := NewWorkerManager(projectRoot)
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusSuccess,
				}, nil
			}),
		}
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Mode: "once"})
	require.NoError(t, err)

	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)
	assert.NotEqual(t, "running", final.State)

	statusPath := ddxroot.JoinProject(projectRoot, "workers", record.ID, "status.json")
	data, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	var persisted WorkerRecord
	require.NoError(t, json.Unmarshal(data, &persisted))
	assert.Equal(t, projectRoot, persisted.ProjectRoot)

	_, err = os.Stat(filepath.Join(projectRoot, ddxroot.DirName, "workers", record.ID))
	assert.True(t, os.IsNotExist(err), "worker artifacts must not be written under projectRoot/.ddx in convention-root mode")

	events, err := store.Events("ddx-root-worker")
	require.NoError(t, err)
	assert.NotEmpty(t, events, "worker execution should have used the convention-root bead store")
}
