package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitattributes_UnionMergesAppendOnlyTrackerFiles verifies that the
// repo-root .gitattributes configures the append-only tracker files for the
// built-in union merge driver and that a real git merge preserves concurrent
// appends to .ddx/beads.jsonl without conflict.
func TestGitattributes_UnionMergesAppendOnlyTrackerFiles(t *testing.T) {
	repoDir := t.TempDir()

	runGitInDir(t, repoDir, "init", "-b", "main")
	runGitInDir(t, repoDir, "config", "user.email", "test@example.com")
	runGitInDir(t, repoDir, "config", "user.name", "Test User")

	trackerRoot := "." + "ddx"
	trackerBeads := filepath.Join(trackerRoot, "beads.jsonl")
	trackerLocks := filepath.Join(trackerRoot, "metrics", "locks.jsonl")

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, trackerRoot), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitattributes"), []byte(
		".ddx/beads.jsonl merge=union\n"+
			".ddx/metrics/locks.jsonl merge=union\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, trackerBeads), []byte("{\"id\":\"base\"}\n"), 0o644))

	runGitInDir(t, repoDir, "add", ".gitattributes", trackerBeads)
	runGitInDir(t, repoDir, "commit", "-m", "chore: seed tracker merge config")

	attrOut, err := exec.Command("git", "-C", repoDir, "check-attr", "merge", "--", trackerBeads, trackerLocks).CombinedOutput()
	require.NoError(t, err, "git check-attr failed:\n%s", attrOut)
	attrLines := strings.Split(strings.TrimSpace(string(attrOut)), "\n")
	require.Len(t, attrLines, 2)
	assert.Contains(t, attrLines[0], trackerBeads+": merge: union")
	assert.Contains(t, attrLines[1], trackerLocks+": merge: union")

	baseRev := strings.TrimSpace(runGitInDirOutput(t, repoDir, "rev-parse", "HEAD"))

	runGitInDir(t, repoDir, "checkout", "-b", "left", baseRev)
	appendTrackerLine(t, repoDir, "{\"id\":\"left\"}")
	runGitInDir(t, repoDir, "add", ".ddx/beads.jsonl")
	runGitInDir(t, repoDir, "commit", "-m", "left append")

	runGitInDir(t, repoDir, "checkout", "-b", "right", baseRev)
	appendTrackerLine(t, repoDir, "{\"id\":\"right\"}")
	runGitInDir(t, repoDir, "add", ".ddx/beads.jsonl")
	runGitInDir(t, repoDir, "commit", "-m", "right append")

	runGitInDir(t, repoDir, "checkout", "left")
	runGitInDir(t, repoDir, "merge", "--no-edit", "right")

	merged, err := os.ReadFile(filepath.Join(repoDir, trackerBeads))
	require.NoError(t, err)
	assert.Contains(t, string(merged), "{\"id\":\"left\"}")
	assert.Contains(t, string(merged), "{\"id\":\"right\"}")

	status := strings.TrimSpace(runGitInDirOutput(t, repoDir, "diff", "--name-only", "--diff-filter=U"))
	assert.Empty(t, status, "merge should not leave conflict markers")
}

func appendTrackerLine(t *testing.T, repoDir, line string) {
	t.Helper()

	path := filepath.Join(repoDir, "."+"ddx", "beads.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(line + "\n")
	require.NoError(t, err)
}

func runGitInDirOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scrubbedGitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v in %s: %v\n%s", args, dir, err, out)
	return string(out)
}
