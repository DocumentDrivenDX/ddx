package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initRepoWithTrackedBeadsFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initTestRepo(t, dir)
	trackerDir := filepath.Join(dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(trackerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(trackerDir, "beads.jsonl"), []byte(`{"id":"ddx-test","status":"open"}`+"\n"), 0o644))
	runGitForBeadsHookTest(t, dir, "add", ".ddx/beads.jsonl")
	runGitForBeadsHookTest(t, dir, "commit", "-m", "seed tracker")
	return dir
}

func runGitForBeadsHookTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, string(out))
}

func runBeadsTrackerHook(t *testing.T, repoRoot string) (string, error) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "beads-tracker-health.sh"))
	require.NoError(t, err)
	cmd := exec.Command("sh", script)
	cmd.Dir = repoRoot
	cmd.Env = append(gitpkg.CleanEnv(), "DDX_BEADS_TRACKER_HEALTH_ROOT="+repoRoot)
	out, runErr := cmd.CombinedOutput()
	return string(out), runErr
}

func appendTrackerLine(t *testing.T, repoRoot, id string) {
	t.Helper()
	path := filepath.Join(repoRoot, ddxroot.DirName, "beads.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(`{"id":"` + id + `","status":"open"}` + "\n")
	require.NoError(t, err)
}

func TestBeadsTrackerHookAllowsCleanTracker(t *testing.T) {
	dir := initRepoWithTrackedBeadsFile(t)

	out, err := runBeadsTrackerHook(t, dir)
	require.NoError(t, err, out)
}

func TestBeadsTrackerHookAllowsFullyStagedTrackerChange(t *testing.T) {
	dir := initRepoWithTrackedBeadsFile(t)
	appendTrackerLine(t, dir, "ddx-staged")
	runGitForBeadsHookTest(t, dir, "add", ".ddx/beads.jsonl")

	out, err := runBeadsTrackerHook(t, dir)
	require.NoError(t, err, out)
}

func TestBeadsTrackerHookFailsOnUnstagedTrackerChange(t *testing.T) {
	dir := initRepoWithTrackedBeadsFile(t)
	appendTrackerLine(t, dir, "ddx-unstaged")

	out, err := runBeadsTrackerHook(t, dir)
	require.Error(t, err)
	assert.Contains(t, out, ".ddx/beads.jsonl has unstaged changes")
	assert.Contains(t, out, "git add .ddx/beads.jsonl")
}

func TestBeadsTrackerHookFailsOnPartiallyStagedTrackerChange(t *testing.T) {
	dir := initRepoWithTrackedBeadsFile(t)
	appendTrackerLine(t, dir, "ddx-staged")
	runGitForBeadsHookTest(t, dir, "add", ".ddx/beads.jsonl")
	appendTrackerLine(t, dir, "ddx-unstaged")

	out, err := runBeadsTrackerHook(t, dir)
	require.Error(t, err)
	assert.Contains(t, out, ".ddx/beads.jsonl has unstaged changes")
	assert.True(t, strings.Contains(out, "partially staged"), "output should mention partial staging: %s", out)
	assert.Contains(t, out, "git add .ddx/beads.jsonl")
}
