package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// autoCommitConfig returns a YAML config that enables git.auto_commit: always.
const autoCommitConfig = `version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: chore
`

// plantStaleLock writes a .git/index.lock file with mtime set 60s in the
// past so it is safely past gitlock.StaleAge (30s). The lock has no owning
// process because we never open a handle to it from a running process.
func plantStaleLock(t *testing.T, repoDir string) {
	t.Helper()
	lockPath := filepath.Join(repoDir, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, nil, 0o644))
	old := time.Now().Add(-60 * time.Second)
	require.NoError(t, os.Chtimes(lockPath, old, old))
}

// TestBeadCreate_StaleIndexLockSelfHeals asserts that ddx bead create against
// a worktree with a stale .git/index.lock (mtime > gitlock.StaleAge, no owner)
// succeeds without operator intervention.
func TestBeadCreate_StaleIndexLockSelfHeals(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	plantStaleLock(t, env.Dir)

	rootCmd := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(rootCmd, "bead", "create", "Stale lock self-heal test")
	require.NoError(t, err, "bead create should succeed despite stale index.lock")
	assert.NotEmpty(t, strings.TrimSpace(out))

	// Lock should have been removed by recovery.
	_, statErr := os.Stat(filepath.Join(env.Dir, ".git", "index.lock"))
	assert.True(t, os.IsNotExist(statErr), "stale lock should have been removed")
}

// TestBeadUpdate_StaleIndexLockSelfHeals asserts that ddx bead update against
// a worktree with a stale .git/index.lock succeeds without operator intervention.
func TestBeadUpdate_StaleIndexLockSelfHeals(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	rootCmd := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(rootCmd, "bead", "create", "Bead to update")
	require.NoError(t, err)
	id := strings.TrimSpace(out)

	plantStaleLock(t, env.Dir)

	rootCmd = NewCommandFactory(env.Dir).NewRootCommand()
	_, err = executeCommand(rootCmd, "bead", "update", id, "--notes", "updated via stale-lock test")
	require.NoError(t, err, "bead update should succeed despite stale index.lock")

	_, statErr := os.Stat(filepath.Join(env.Dir, ".git", "index.lock"))
	assert.True(t, os.IsNotExist(statErr), "stale lock should have been removed")
}

// TestBeadClose_StaleIndexLockSelfHeals asserts that ddx bead close against
// a worktree with a stale .git/index.lock succeeds without operator intervention.
func TestBeadClose_StaleIndexLockSelfHeals(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(autoCommitConfig)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	rootCmd := NewCommandFactory(env.Dir).NewRootCommand()
	out, err := executeCommand(rootCmd, "bead", "create", "Bead to close")
	require.NoError(t, err)
	id := strings.TrimSpace(out)

	plantStaleLock(t, env.Dir)

	rootCmd = NewCommandFactory(env.Dir).NewRootCommand()
	_, err = executeCommand(rootCmd, "bead", "close", id)
	require.NoError(t, err, "bead close should succeed despite stale index.lock")

	_, statErr := os.Stat(filepath.Join(env.Dir, ".git", "index.lock"))
	assert.True(t, os.IsNotExist(statErr), "stale lock should have been removed")
}
