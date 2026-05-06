package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cleanupCommandJSON struct {
	DryRun                      bool             `json:"dry_run"`
	ProjectRoot                 string           `json:"project_root"`
	TempRoot                    string           `json:"temp_root"`
	ScannedTempDirs             int              `json:"scanned_temp_dirs"`
	ScannedEvidenceDirs         int              `json:"scanned_evidence_dirs"`
	CompleteEvidenceDirs        int              `json:"complete_evidence_dirs"`
	RemovedUnregisteredTempDirs int64            `json:"removed_unregistered_temp_dirs"`
	RemovedRegisteredWorktrees  int64            `json:"removed_registered_worktrees"`
	RemovedRunStateFiles        int64            `json:"removed_run_state_files"`
	BytesReclaimed              int64            `json:"bytes_reclaimed"`
	InodesReclaimed             int64            `json:"inodes_reclaimed"`
	Warnings                    []map[string]any `json:"warnings"`
	BlockedErrors               []map[string]any `json:"blocked_errors"`
	Observations                []map[string]any `json:"observations"`
}

func setupCleanupCommandProject(t *testing.T) (string, string) {
	t.Helper()
	projectRoot := t.TempDir()
	tempRoot := t.TempDir()
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	return projectRoot, tempRoot
}

func writeCleanupCommandCandidate(t *testing.T, root, name, projectRoot, attemptID string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, agent.WriteExecutionCleanupMetadata(path, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-cleanup",
		AttemptID:    attemptID,
		WorktreePath: path,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("stale\n"), 0o644))
	return path
}

func TestCleanupCommand_DryRunReportsWithoutDeleting(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	stalePath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-dryrun-20260506T154739-deadbeef", projectRoot, "20260506T154739-deadbeef")
	runStatePath := filepath.Join(projectRoot, ".ddx", agent.RunStateFileName)
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-live-feedface",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup")
	require.NoError(t, err)

	assert.Contains(t, out, "cleanup: would remove 1 stale temp dir(s), 0 registered worktree(s), 1 run-state file(s)")
	assert.Contains(t, out, "run again with --apply")
	assert.DirExists(t, stalePath)
	assert.FileExists(t, runStatePath)
}

func TestCleanupCommand_ApplyRemovesStaleDDxResources(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	stalePath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-apply-20260506T154739-feedface", projectRoot, "20260506T154739-feedface")
	runStatePath := filepath.Join(projectRoot, ".ddx", agent.RunStateFileName)
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-live-cafebabe",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))
	evidenceDir := filepath.Join(projectRoot, ".ddx", "executions", "attempt-complete")
	require.NoError(t, os.MkdirAll(evidenceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "manifest.json"), []byte(`{"attempt_id":"attempt-complete"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "result.json"), []byte(`{"status":"success"}`), 0o644))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup", "--apply")
	require.NoError(t, err)

	assert.Contains(t, out, "cleanup: removed 1 stale temp dir(s), 0 registered worktree(s), 1 run-state file(s)")
	assert.NoFileExists(t, stalePath)
	assert.NoFileExists(t, runStatePath)
	assert.FileExists(t, filepath.Join(evidenceDir, "manifest.json"))
	assert.FileExists(t, filepath.Join(evidenceDir, "result.json"))
}

func TestCleanupCommand_JSONShape(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-json-20260506T154739-12345678", projectRoot, "20260506T154739-12345678")
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-live-90abcdef",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup", "--json")
	require.NoError(t, err)

	var report cleanupCommandJSON
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	assert.True(t, report.DryRun)
	assert.Equal(t, int64(1), report.RemovedUnregisteredTempDirs)
	assert.Equal(t, int64(1), report.RemovedRunStateFiles)
	assert.NotEmpty(t, report.Warnings)
	assert.NotNil(t, report.BlockedErrors)
	assert.NotNil(t, report.Observations)
	assert.Greater(t, report.BytesReclaimed, int64(0))
	assert.Greater(t, report.InodesReclaimed, int64(0))
}

func TestCleanupCommand_DoesNotRemovePreservedEvidence(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	stalePath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-preserved-20260506T154739-abcdef12", projectRoot, "20260506T154739-abcdef12")
	preservedPath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-preserved-20260506T154739-34567890", projectRoot, "20260506T154739-34567890")
	require.NoError(t, agent.WriteExecutionCleanupMetadata(preservedPath, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-34567890",
		WorktreePath: preservedPath,
		Preserved:    true,
	}))
	evidenceDir := filepath.Join(projectRoot, ".ddx", "executions", "attempt-complete")
	require.NoError(t, os.MkdirAll(evidenceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "manifest.json"), []byte(`{"attempt_id":"attempt-complete"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "result.json"), []byte(`{"status":"success"}`), 0o644))
	runsDir := filepath.Join(projectRoot, ".ddx", "runs", "run-complete")
	require.NoError(t, os.MkdirAll(runsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runsDir, "record.json"), []byte(`{"run_id":"run-complete"}`), 0o644))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup", "--apply")
	require.NoError(t, err)

	assert.Contains(t, out, "cleanup: removed 1 stale temp dir(s), 0 registered worktree(s), 0 run-state file(s)")
	assert.Contains(t, out, "cleanup: preserved 2 complete evidence bundle(s)")
	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, preservedPath)
	assert.FileExists(t, filepath.Join(evidenceDir, "manifest.json"))
	assert.FileExists(t, filepath.Join(evidenceDir, "result.json"))
	assert.FileExists(t, filepath.Join(runsDir, "record.json"))
}
