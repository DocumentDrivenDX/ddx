package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourcePressure_ReportIncludesWorkerTempAndExecutionCounts asserts
// that a non-destructive housekeeping scan (apply=false) surfaces the
// worker subprocess count, temp worktree count, and stale execution dir
// count needed to diagnose resource pressure before EMFILE.
func TestResourcePressure_ReportIncludesWorkerTempAndExecutionCounts(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)

	liveWorktree := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-pressure-live-20260708T010203-deadbeef")
	require.NoError(t, os.MkdirAll(liveWorktree, 0o755))

	workerID := "agent-loop-pressure-live"
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, workerID, workerstatus.LivenessRecord{
		WorkerID:       workerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: time.Now().UTC(),
	}))

	t.Setenv(executionRetentionOverrideEnv, "7")
	staleAttemptID := "20260101T000000-feedface"
	staleEvidence := setupWorkStartupEvidenceDir(t, projectRoot, staleAttemptID, time.Now().AddDate(0, 0, -10).UTC())

	runner := newStartupHousekeepingRunner(projectRoot)
	report, err := runner.scan(context.Background(), false)
	require.NoError(t, err)

	assert.Equal(t, int64(1), report.WorkerSubprocessCount)
	assert.Equal(t, int64(1), report.TempWorktreeCount)
	assert.Equal(t, int64(1), report.StaleExecutionDirs)
	assert.DirExists(t, staleEvidence)
}

// TestResourcePressure_ReportCountsDoNotCleanup asserts that observing
// resource pressure counts (scan with apply=false) never removes, archives,
// kills, or prunes the temp worktrees, worker liveness dirs, or execution
// dirs it counts.
func TestResourcePressure_ReportCountsDoNotCleanup(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")

	staleWorktree := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-pressure-stale-20260515T010203-deadbeef")
	require.NoError(t, os.MkdirAll(staleWorktree, 0o755))
	oldTime := time.Now().Add(-2 * time.Hour).UTC()
	require.NoError(t, os.Chtimes(staleWorktree, oldTime, oldTime))

	staleWorkerID := "agent-loop-pressure-stale"
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, staleWorkerID, workerstatus.LivenessRecord{
		WorkerID:       staleWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: time.Now().Add(-31 * time.Minute).UTC(),
	}))

	t.Setenv(executionRetentionOverrideEnv, "7")
	staleAttemptID := "20260101T000000-cafebabe"
	staleEvidence := setupWorkStartupEvidenceDir(t, projectRoot, staleAttemptID, time.Now().AddDate(0, 0, -10).UTC())

	runner := newStartupHousekeepingRunner(projectRoot)
	report, err := runner.scan(context.Background(), false)
	require.NoError(t, err)

	assert.Equal(t, int64(1), report.StaleWorktrees)
	assert.Equal(t, int64(1), report.StaleWorkerDirs)
	assert.Equal(t, int64(1), report.StaleExecutionDirs)
	assert.Equal(t, int64(0), report.RemovedRegisteredWorktrees)
	assert.Equal(t, int64(0), report.RemovedUnregisteredTempDirs)
	assert.Equal(t, int64(0), report.RemovedWorkerDirs)
	assert.Equal(t, int64(0), report.ArchivedExecutionDirs)
	assert.Equal(t, int64(0), report.DeletedExecutionDirs)

	assert.DirExists(t, staleWorktree)
	assert.DirExists(t, filepath.Join(workerstatus.LivenessDir(projectRoot), staleWorkerID))
	assert.DirExists(t, staleEvidence)
}
