package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoCommit_RelativeNestedPath(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalDir) }()

	require.NoError(t, os.Chdir(repoDir))

	relPath := filepath.Join("cli", "cmd", "bead.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(relPath), 0o755))
	require.NoError(t, os.WriteFile(relPath, []byte("package cmd\n"), 0o644))

	sha, err := AutoCommit(relPath, "bead-123", "stamp reviewed", AutoCommitConfig{
		AutoCommit:   "always",
		CommitPrefix: "docs",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sha)

	cmd := exec.Command("git", "show", "--name-only", "--format=oneline", sha)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git show failed: %s", string(out))
	assert.Contains(t, string(out), relPath)
}
