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

func TestAutoCommit_TargetOnlyPreservesUnrelatedStagedChanges(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	beadsPath := filepath.Join(repoDir, ddxDirSegment, "beads.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(beadsPath), 0o755))
	require.NoError(t, os.WriteFile(beadsPath, []byte("initial\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "unrelated.txt"), []byte("initial\n"), 0o644))
	runGitInDir(t, repoDir, "add", ".ddx/beads.jsonl", "unrelated.txt")
	runGitInDir(t, repoDir, "commit", "-m", "track files")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "unrelated.txt"), []byte("staged change\n"), 0o644))
	runGitInDir(t, repoDir, "add", "unrelated.txt")
	require.NoError(t, os.WriteFile(beadsPath, []byte("tracker change\n"), 0o644))

	sha, err := AutoCommit(beadsPath, "beads", "update tracker", AutoCommitConfig{
		AutoCommit:   "always",
		CommitPrefix: "chore",
	})
	require.NoError(t, err)
	require.NotEmpty(t, sha)

	showCmd := exec.Command("git", "show", "--name-only", "--pretty=format:", sha)
	showCmd.Dir = repoDir
	showCmd.Env = scrubbedGitEnv()
	showOut, err := showCmd.CombinedOutput()
	require.NoError(t, err, "git show failed: %s", string(showOut))
	assert.Contains(t, string(showOut), ".ddx/beads.jsonl")
	assert.NotContains(t, string(showOut), "unrelated.txt")

	diffCmd := exec.Command("git", "diff", "--cached", "--name-only")
	diffCmd.Dir = repoDir
	diffCmd.Env = scrubbedGitEnv()
	diffOut, err := diffCmd.CombinedOutput()
	require.NoError(t, err, "git diff --cached failed: %s", string(diffOut))
	assert.Contains(t, string(diffOut), "unrelated.txt")
	assert.NotContains(t, string(diffOut), ".ddx/beads.jsonl")
}
