package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeadAutoCommitReturnsError(t *testing.T) {
	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
git:
  auto_commit: always
  commit_prefix: chore
`)
	gitAddAndCommit(t, env.Dir, "track ddx config", ".ddx/config.yaml")

	lockPath := filepath.Join(env.Dir, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("locked\n"), 0o644))

	rootCmd := NewCommandFactory(env.Dir).NewRootCommand()
	_, err := executeCommand(rootCmd, "bead", "create", "Auto-commit failure")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto-commit beads tracker after create")
	assert.Contains(t, err.Error(), "git add failed")
}
