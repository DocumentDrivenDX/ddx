package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctor_ReportsAndAppliesHousekeepingCounts(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")
	t.Setenv(executionRetentionOverrideEnv, "7")

	oldTime := time.Now().Add(-2 * time.Hour).UTC()

	staleWorktree := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-doctor-stale-20260515T010203-deadbeef")
	require.NoError(t, os.MkdirAll(staleWorktree, 0o755))
	require.NoError(t, agent.WriteExecutionCleanupMetadata(staleWorktree, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-doctor-stale",
		AttemptID:    "20260515T010203-deadbeef",
		WorktreePath: staleWorktree,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(staleWorktree, "scratch.txt"), []byte("payload\n"), 0o644))
	require.NoError(t, os.Chtimes(staleWorktree, oldTime, oldTime))

	staleWorkerID := "agent-loop-doctor-stale"
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, staleWorkerID, workerstatus.LivenessRecord{
		WorkerID:       staleWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: time.Now().Add(-31 * time.Minute).UTC(),
	}))

	oldAttemptID := "20260101T000000-feedface"
	oldEvidence := setupWorkStartupEvidenceDir(t, projectRoot, oldAttemptID, time.Now().AddDate(0, 0, -10).UTC())

	factory := NewCommandFactory(projectRoot)

	dryRunOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, dryRunOutput, "stale worktrees=1, stale worker dirs=1, stale execution dirs=1")
	assert.DirExists(t, staleWorktree)
	assert.DirExists(t, filepath.Join(projectRoot, ".ddx", "workers", staleWorkerID))
	assert.DirExists(t, oldEvidence)

	applyOutput, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--apply")
	require.NoError(t, err)
	assert.Contains(t, applyOutput, "stale worktrees=1, stale worker dirs=1, stale execution dirs=1 (applied)")
	assert.NoDirExists(t, staleWorktree)
	assert.NoDirExists(t, filepath.Join(projectRoot, ".ddx", "workers", staleWorkerID))
	assert.NoDirExists(t, oldEvidence)
	assert.DirExists(t, filepath.Join(projectRoot, ".ddx", "executions-archive", "2026", "01", oldAttemptID))

	if strings.Contains(applyOutput, "Execution housekeeping scan failed") {
		t.Fatalf("doctor --apply reported housekeeping failure:\n%s", applyOutput)
	}
}
