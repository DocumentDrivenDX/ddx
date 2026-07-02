package git

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootGitattributesConfiguresUnionMergeForTrackerFiles(t *testing.T) {
	repoRoot := gitattributesRepoRoot(t)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitattributes"))
	require.NoError(t, err)

	attrs := string(data)
	require.Contains(t, attrs, ".ddx/beads.jsonl merge=union")
	require.Contains(t, attrs, ".ddx/metrics/locks.jsonl merge=union")
}

func TestRootGitattributesMergesDivergentBeadAppendsWithoutConflict(t *testing.T) {
	repoRoot := gitattributesRepoRoot(t)
	attrs := mustReadFile(t, filepath.Join(repoRoot, ".gitattributes"))

	repoDir := t.TempDir()
	trackerDir := filepath.Join(repoDir, trackerDirName())
	require.NoError(t, os.MkdirAll(trackerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitattributes"), attrs, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(trackerDir, "beads.jsonl"), nil, 0o644))

	runGitInDir(t, repoDir, "init")
	runGitInDir(t, repoDir, "config", "user.email", "test@example.com")
	runGitInDir(t, repoDir, "config", "user.name", "Test User")
	runGitInDir(t, repoDir, "add", ".gitattributes", trackerRelFile("beads.jsonl"))
	runGitInDir(t, repoDir, "commit", "-m", "base")

	runGitInDir(t, repoDir, "checkout", "-b", "branch-a")
	appendJSONLLine(t, filepath.Join(trackerDir, "beads.jsonl"), `{"id":"ddx-branch-a","status":"open"}`)
	runGitInDir(t, repoDir, "add", trackerRelFile("beads.jsonl"))
	runGitInDir(t, repoDir, "commit", "-m", "branch-a")

	runGitInDir(t, repoDir, "checkout", "-b", "branch-b", "HEAD~1")
	appendJSONLLine(t, filepath.Join(trackerDir, "beads.jsonl"), `{"id":"ddx-branch-b","status":"open"}`)
	runGitInDir(t, repoDir, "add", trackerRelFile("beads.jsonl"))
	runGitInDir(t, repoDir, "commit", "-m", "branch-b")

	runGitInDir(t, repoDir, "checkout", "branch-a")
	runGitInDir(t, repoDir, "merge", "--no-edit", "branch-b")

	merged := mustReadFile(t, filepath.Join(trackerDir, "beads.jsonl"))
	lines := nonEmptyLines(string(merged))
	require.Len(t, lines, 2)
	require.Contains(t, lines, `{"id":"ddx-branch-a","status":"open"}`)
	require.Contains(t, lines, `{"id":"ddx-branch-b","status":"open"}`)
}

func gitattributesRepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	// cli/internal/git/gitattributes_merge_test.go lives three directories below the repo root.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func trackerDirName() string {
	return "." + "ddx"
}

func trackerRelFile(name string) string {
	return filepath.Join(trackerDirName(), name)
}

func appendJSONLLine(t *testing.T, path, line string) {
	t.Helper()

	current := mustReadFile(t, path)
	current = append(current, []byte(line+"\n")...)
	require.NoError(t, os.WriteFile(path, current, 0o644))
}

func nonEmptyLines(content string) []string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
