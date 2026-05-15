package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkStartup_ReapsStaleWorktrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-liveness startup cleanup test is unix-oriented")
	}

	projectRoot, tempRoot := setupWorkStartupCleanupProject(t)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")

	attemptID := "20260515T010203-deadbeef"
	worktreePath := createRegisteredStartupWorktree(t, projectRoot, tempRoot, "ddx-startup-stale", attemptID)
	old := time.Now().Add(-2 * time.Hour).UTC()
	writeStartupRunState(t, projectRoot, "ddx-startup-stale", attemptID, worktreePath, deadProcessPID(t), old)
	require.NoError(t, os.Chtimes(worktreePath, old, old))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.NoFileExists(t, worktreePath)
	assert.NotContains(t, runCleanupCommandGit(t, projectRoot, "worktree", "list", "--porcelain"), worktreePath)
}

func TestWorkStartup_PreservesLiveWorktrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-liveness startup cleanup test is unix-oriented")
	}

	projectRoot, tempRoot := setupWorkStartupCleanupProject(t)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")

	attemptID := "20260515T020304-feedface"
	worktreePath := createRegisteredStartupWorktree(t, projectRoot, tempRoot, "ddx-startup-live", attemptID)
	old := time.Now().Add(-2 * time.Hour).UTC()
	live := startLongLivedProcess(t)
	writeStartupRunState(t, projectRoot, "ddx-startup-live", attemptID, worktreePath, live.Process.Pid, old)
	require.NoError(t, os.Chtimes(worktreePath, old, old))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.DirExists(t, worktreePath)
	assert.Contains(t, runCleanupCommandGit(t, projectRoot, "worktree", "list", "--porcelain"), worktreePath)
}

func TestWorkStartup_ReapsStaleWorkerDirs(t *testing.T) {
	projectRoot, _ := setupWorkStartupCleanupProject(t)

	workersDir := filepath.Join(projectRoot, ".ddx", "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))

	deadWorkerID := "agent-loop-dead"
	staleWorkerID := "agent-loop-stale"
	freshWorkerID := "agent-loop-fresh"
	now := time.Now().UTC()

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, deadWorkerID, workerstatus.LivenessRecord{
		WorkerID:       deadWorkerID,
		ProjectRoot:    projectRoot,
		PID:            deadProcessPID(t),
		LastActivityAt: now,
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, staleWorkerID, workerstatus.LivenessRecord{
		WorkerID:       staleWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: now.Add(-31 * time.Minute),
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, freshWorkerID, workerstatus.LivenessRecord{
		WorkerID:       freshWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: now,
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.NoDirExists(t, filepath.Join(workersDir, deadWorkerID))
	assert.NoDirExists(t, filepath.Join(workersDir, staleWorkerID))
	assert.DirExists(t, filepath.Join(workersDir, freshWorkerID))
}

func TestExecutionsRetention_ArchivesOldEvidence(t *testing.T) {
	t.Run("archives old evidence under executions-archive", func(t *testing.T) {
		projectRoot := setupWorkStartupCleanupProjectRoot(t)
		t.Setenv(executionRetentionOverrideEnv, "7")

		oldTime := time.Now().AddDate(0, 0, -10).UTC()
		oldAttemptID := "20260101T000000-deadbeef"
		oldDir := setupWorkStartupEvidenceDir(t, projectRoot, oldAttemptID, oldTime)
		recentAttemptID := "20260501T000000-feedface"
		recentDir := setupWorkStartupEvidenceDir(t, projectRoot, recentAttemptID, time.Now().UTC())

		runner := newStartupHousekeepingRunner(projectRoot)
		report, err := runner.scan(context.Background(), true)
		require.NoError(t, err)

		archivedDir := filepath.Join(projectRoot, ".ddx", "executions-archive", "2026", "01", oldAttemptID)
		assert.NoDirExists(t, oldDir)
		assert.DirExists(t, archivedDir)
		assert.DirExists(t, recentDir)
		assert.Equal(t, int64(1), report.StaleExecutionDirs)
		assert.Equal(t, int64(1), report.ArchivedExecutionDirs)
		assert.Equal(t, int64(0), report.DeletedExecutionDirs)
	})

	t.Run("deletes old evidence when retain-days override is zero", func(t *testing.T) {
		projectRoot := setupWorkStartupCleanupProjectRoot(t)
		t.Setenv(executionRetentionOverrideEnv, "0")

		oldTime := time.Now().AddDate(0, 0, -10).UTC()
		oldAttemptID := "20260101T000000-cafebabe"
		oldDir := setupWorkStartupEvidenceDir(t, projectRoot, oldAttemptID, oldTime)

		runner := newStartupHousekeepingRunner(projectRoot)
		report, err := runner.scan(context.Background(), true)
		require.NoError(t, err)

		assert.NoDirExists(t, oldDir)
		assert.NoDirExists(t, filepath.Join(projectRoot, ".ddx", "executions-archive", "2026", "01", oldAttemptID))
		assert.Equal(t, int64(1), report.StaleExecutionDirs)
		assert.Equal(t, int64(0), report.ArchivedExecutionDirs)
		assert.Equal(t, int64(1), report.DeletedExecutionDirs)
	})
}

func setupWorkStartupCleanupProject(t *testing.T) (string, string) {
	t.Helper()

	projectRoot, tempRoot := setupCleanupCommandProject(t)
	runCleanupCommandGit(t, projectRoot, "init", "-b", "main")
	runCleanupCommandGit(t, projectRoot, "config", "user.email", "test@ddx.test")
	runCleanupCommandGit(t, projectRoot, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "seed.txt"), []byte("seed\n"), 0o644))
	runCleanupCommandGit(t, projectRoot, "add", "seed.txt")
	runCleanupCommandGit(t, projectRoot, "commit", "-m", "chore: seed work startup cleanup repo")
	return projectRoot, tempRoot
}

func setupWorkStartupCleanupProjectRoot(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	return projectRoot
}

func setupWorkStartupEvidenceDir(t *testing.T, projectRoot, attemptID string, mtime time.Time) string {
	t.Helper()

	dir := filepath.Join(projectRoot, ".ddx", "executions", attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"status":"success"}`), 0o644))
	require.NoError(t, os.Chtimes(dir, mtime, mtime))
	return dir
}

func createRegisteredStartupWorktree(t *testing.T, projectRoot, tempRoot, beadID, attemptID string) string {
	t.Helper()

	worktreePath := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+beadID+"-"+attemptID)
	runCleanupCommandGit(t, projectRoot, "worktree", "add", "--detach", worktreePath, "HEAD")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreePath).Run()
	})
	require.NoError(t, agent.WriteExecutionCleanupMetadata(worktreePath, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       beadID,
		AttemptID:    attemptID,
		WorktreePath: worktreePath,
		Registered:   true,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "scratch.txt"), []byte("payload\n"), 0o644))
	return worktreePath
}

func writeStartupRunState(t *testing.T, projectRoot, beadID, attemptID, worktreePath string, pid int, refreshedAt time.Time) {
	t.Helper()

	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       beadID,
		AttemptID:    attemptID,
		StartedAt:    refreshedAt.Add(-5 * time.Minute),
		WorktreePath: worktreePath,
		PID:          pid,
		RefreshedAt:  refreshedAt,
		ExpiresAt:    refreshedAt.Add(5 * time.Minute),
	}))
}

func deadProcessPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}

func startLongLivedProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}
