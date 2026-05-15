package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
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
