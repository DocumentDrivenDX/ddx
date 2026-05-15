package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
	ScannedScratchDirs          int              `json:"scanned_scratch_dirs"`
	RemovedUnregisteredTempDirs int64            `json:"removed_unregistered_temp_dirs"`
	RemovedRegisteredWorktrees  int64            `json:"removed_registered_worktrees"`
	RemovedRunStateFiles        int64            `json:"removed_run_state_files"`
	RemovedScratchDirs          int64            `json:"removed_scratch_dirs"`
	PreservedActiveScratchDirs  int64            `json:"preserved_active_scratch_dirs"`
	BytesReclaimed              int64            `json:"bytes_reclaimed"`
	InodesReclaimed             int64            `json:"inodes_reclaimed"`
	ScratchBytesReclaimed       int64            `json:"scratch_bytes_reclaimed"`
	ScratchInodesReclaimed      int64            `json:"scratch_inodes_reclaimed"`
	Warnings                    []map[string]any `json:"warnings"`
	BlockedErrors               []map[string]any `json:"blocked_errors"`
	Observations                []map[string]any `json:"observations"`
}

func setupCleanupCommandProject(t *testing.T) (string, string) {
	t.Helper()
	projectRoot := t.TempDir()
	tempParent := t.TempDir()
	tempRoot := filepath.Join(tempParent, "ddx-exec-wt")
	t.Setenv("DDX_EXEC_WT_DIR", tempRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
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

func runCleanupCommandGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s\n%s", strings.Join(args, " "), string(out))
	return strings.TrimSpace(string(out))
}

func TestCleanupCommand_DryRunReportsWithoutDeleting(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	stalePath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-dryrun-20260506T154739-deadbeef", projectRoot, "20260506T154739-deadbeef")
	runStatePath := filepath.Join(projectRoot, ddxroot.DirName, agent.RunStateFileName)
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
	runStatePath := filepath.Join(projectRoot, ddxroot.DirName, agent.RunStateFileName)
	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-live-cafebabe",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))
	evidenceDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "attempt-complete")
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

func TestCleanupRunStateDoesNotDirtyTrackedRepo(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	runCleanupCommandGit(t, projectRoot, "init", "-b", "main")
	runCleanupCommandGit(t, projectRoot, "config", "user.email", "test@ddx.test")
	runCleanupCommandGit(t, projectRoot, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "seed.txt"), []byte("seed\n"), 0o644))
	runCleanupCommandGit(t, projectRoot, "add", "seed.txt")
	runCleanupCommandGit(t, projectRoot, "commit", "-m", "chore: seed cleanup repo")

	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(filepath.Join(ddxDir, "run-state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "run-state.json"), []byte(`{"attempt_id":"tracked-root"}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "run-state", "tracked-attempt.json"), []byte(`{"attempt_id":"tracked-attempt"}`+"\n"), 0o644))
	runCleanupCommandGit(t, projectRoot, "add", ".ddx/run-state.json", ".ddx/run-state/tracked-attempt.json")
	runCleanupCommandGit(t, projectRoot, "commit", "-m", "test: track legacy run-state")

	root := NewCommandFactory(projectRoot).NewRootCommand()
	_, err := executeCommand(root, "init")
	require.NoError(t, err)
	assert.Empty(t, runCleanupCommandGit(t, projectRoot, "ls-files", "--", ".ddx/run-state.json", ".ddx/run-state"))

	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       "ddx-cleanup",
		AttemptID:    "20260506T154739-live-cafebabe",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))
	attemptPath := filepath.Join(projectRoot, ddxroot.DirName, agent.RunStateDirName, "20260506T154739-live-cafebabe.json")
	require.FileExists(t, attemptPath)

	out, err := executeCommand(root, "cleanup", "--apply")
	require.NoError(t, err)

	assert.Contains(t, out, "cleanup: removed 0 stale temp dir(s), 0 registered worktree(s), 2 run-state file(s), 0 scratch dir(s)")
	assert.NoFileExists(t, filepath.Join(projectRoot, ddxroot.DirName, agent.RunStateFileName))
	assert.NoFileExists(t, attemptPath)
	assert.Empty(t, runCleanupCommandGit(t, projectRoot, "status", "--short", "--", ".ddx/run-state.json", ".ddx/run-state"),
		"cleanup must not leave tracked run-state deletions behind after migration")
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
	evidenceDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "attempt-complete")
	require.NoError(t, os.MkdirAll(evidenceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "manifest.json"), []byte(`{"attempt_id":"attempt-complete"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "result.json"), []byte(`{"status":"success"}`), 0o644))
	runsDir := filepath.Join(projectRoot, ddxroot.DirName, "runs", "run-complete")
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

func TestCleanupEndToEnd_PreservesPublishedEvidence(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	stalePath := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-published-20260508T120000-abcdef12", projectRoot, "20260508T120000-abcdef12")

	executionsDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "attempt-published")
	require.NoError(t, os.MkdirAll(executionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "manifest.json"), []byte(`{"attempt_id":"attempt-published"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "result.json"), []byte(`{"status":"success"}`), 0o644))

	runsDir := filepath.Join(projectRoot, ddxroot.DirName, "runs", "run-published")
	require.NoError(t, os.MkdirAll(runsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runsDir, "record.json"), []byte(`{"run_id":"run-published"}`), 0o644))

	refPath := filepath.Join(projectRoot, ".git", "refs", "ddx", "iterations", "ddx-published", "attempt-1")
	require.NoError(t, os.MkdirAll(filepath.Dir(refPath), 0o755))
	require.NoError(t, os.WriteFile(refPath, []byte("abcdef1234567890abcdef1234567890abcdef12\n"), 0o644))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup", "--apply")
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.FileExists(t, filepath.Join(executionsDir, "manifest.json"))
	assert.FileExists(t, filepath.Join(executionsDir, "result.json"))
	assert.FileExists(t, filepath.Join(runsDir, "record.json"))
	assert.FileExists(t, refPath)
	assert.Contains(t, out, "cleanup: preserved 2 complete evidence bundle(s)")
}

func TestWorkCleanupEndToEnd_PrunesStaleRegisteredAndUnregisteredWorktrees(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	runCleanupCommandGit(t, projectRoot, "init", "-b", "main")
	runCleanupCommandGit(t, projectRoot, "config", "user.email", "test@ddx.test")
	runCleanupCommandGit(t, projectRoot, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "seed.txt"), []byte("seed\n"), 0o644))
	runCleanupCommandGit(t, projectRoot, "add", "seed.txt")
	runCleanupCommandGit(t, projectRoot, "commit", "-m", "chore: seed cleanup repo")

	staleRegistered := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-registered-20260508T120000-deadbeef")
	activeRegistered := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-active-20260508T120000-feedface")
	staleUnregistered := writeCleanupCommandCandidate(t, tempRoot, agent.ExecuteBeadWtPrefix+"ddx-cleanup-unregistered-20260508T120000-cafebabe", projectRoot, "20260508T120000-cafebabe")
	nonDDXPath := filepath.Join(tempRoot, "plain-worktree-looking-dir")
	require.NoError(t, os.MkdirAll(nonDDXPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nonDDXPath, "keep.txt"), []byte("keep\n"), 0o644))

	runCleanupCommandGit(t, projectRoot, "worktree", "add", "--detach", staleRegistered, "HEAD")
	runCleanupCommandGit(t, projectRoot, "worktree", "add", "--detach", activeRegistered, "HEAD")
	require.NoError(t, agent.WriteExecutionCleanupMetadata(staleRegistered, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-cleanup-registered",
		AttemptID:    "20260508T120000-deadbeef",
		WorktreePath: staleRegistered,
		Registered:   true,
	}))
	require.NoError(t, agent.WriteExecutionCleanupMetadata(activeRegistered, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-cleanup-active",
		AttemptID:    "20260508T120000-feedface",
		WorktreePath: activeRegistered,
		Registered:   true,
		Preserved:    true,
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "cleanup", "--apply")
	require.NoError(t, err)

	assert.Contains(t, out, "cleanup: removed 1 stale temp dir(s), 1 registered worktree(s), 0 run-state file(s), 0 scratch dir(s)")
	assert.NoFileExists(t, staleRegistered)
	assert.NoFileExists(t, staleUnregistered)
	assert.DirExists(t, activeRegistered)
	assert.DirExists(t, nonDDXPath)
	wtList := runCleanupCommandGit(t, projectRoot, "worktree", "list", "--porcelain")
	assert.NotContains(t, wtList, staleRegistered)
	assert.Contains(t, wtList, activeRegistered)
}

func TestCleanupCommand_ReportsScratchReclamation(t *testing.T) {
	projectRoot, tempRoot := setupCleanupCommandProject(t)
	scratchPath, err := os.MkdirTemp(filepath.Dir(tempRoot), "ddx-test-cleanup-scratch-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(scratchPath) })
	require.NoError(t, os.WriteFile(filepath.Join(scratchPath, "payload.txt"), []byte("stale scratch\n"), 0o644))
	staleTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(scratchPath, staleTime, staleTime))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	textOut, err := executeCommand(root, "cleanup")
	require.NoError(t, err)
	assert.Contains(t, textOut, "scratch dir(s)")
	assert.Contains(t, textOut, "scratch scope would remove")
	assert.DirExists(t, scratchPath)

	root = NewCommandFactory(projectRoot).NewRootCommand()
	jsonOut, err := executeCommand(root, "cleanup", "--json")
	require.NoError(t, err)

	var report cleanupCommandJSON
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &report))
	assert.True(t, report.DryRun)
	assert.GreaterOrEqual(t, report.RemovedScratchDirs, int64(1))
	assert.Greater(t, report.ScratchBytesReclaimed, int64(0))
	assert.Greater(t, report.ScratchInodesReclaimed, int64(0))
}
