package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersion_StaleWarning_PrintsToStderr(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	repoDir := initVersionTestRepo(t, "https://github.com/DocumentDrivenDX/ddx.git")
	initialSHA := gitCommitFile(t, repoDir, "cli/cmd/main.go", "package main\n", "initial")
	_ = gitCommitFile(t, repoDir, "cli/cmd/stale.go", "package main\n", "stale")

	stdout, stderr, err := runVersionCommand(t, repoDir, "v1.2.3", initialSHA)
	require.NoError(t, err)

	assert.Contains(t, stdout, "DDx v1.2.3")
	assert.Contains(t, stderr, "WARNING: installed ddx is built from")
	assert.Contains(t, stderr, "commits ahead")
	assert.Contains(t, stderr, "Run \"make install\" to refresh.")
}

func TestVersion_NoWarningWhenInSync(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	repoDir := initVersionTestRepo(t, "https://github.com/DocumentDrivenDX/ddx.git")
	headSHA := gitCommitFile(t, repoDir, "cli/cmd/main.go", "package main\n", "initial")

	_, stderr, err := runVersionCommand(t, repoDir, "v1.2.3", headSHA)
	require.NoError(t, err)

	assert.Empty(t, stderr)
}

func TestVersion_NoWarningOutsideDDxRepo(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	repoDir := initVersionTestRepo(t, "https://github.com/example/not-ddx.git")
	initialSHA := gitCommitFile(t, repoDir, "cli/cmd/main.go", "package main\n", "initial")
	_ = gitCommitFile(t, repoDir, "cli/cmd/stale.go", "package main\n", "stale")

	_, stderr, err := runVersionCommand(t, repoDir, "v1.2.3", initialSHA)
	require.NoError(t, err)

	assert.Empty(t, stderr)
}

func TestVersion_HandlesDetachedHead(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	repoDir := initVersionTestRepo(t, "https://github.com/DocumentDrivenDX/ddx.git")
	initialSHA := gitCommitFile(t, repoDir, "cli/cmd/main.go", "package main\n", "initial")
	headSHA := gitCommitFile(t, repoDir, "cli/cmd/detached.go", "package main\n", "detached")

	detach := exec.Command("git", "checkout", "--detach", headSHA)
	detach.Dir = repoDir
	detach.Env = gitpkg.CleanEnv()
	out, err := detach.CombinedOutput()
	require.NoError(t, err, "git checkout --detach: %s", string(out))

	stdout, stderr, err := runVersionCommand(t, repoDir, "v1.2.3", initialSHA)
	require.NoError(t, err)

	assert.Contains(t, stdout, "DDx v1.2.3")
	assert.NotEmpty(t, stderr)
}

func initVersionTestRepo(t *testing.T, originURL string) string {
	t.Helper()

	repoDir := t.TempDir()
	runVersionGit(t, repoDir, "init")
	runVersionGit(t, repoDir, "config", "user.email", "test@example.com")
	runVersionGit(t, repoDir, "config", "user.name", "Test User")
	runVersionGit(t, repoDir, "remote", "add", "origin", originURL)
	return repoDir
}

func gitCommitFile(t *testing.T, repoDir, relPath, content, commitMsg string) string {
	t.Helper()

	fullPath := filepath.Join(repoDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	runVersionGit(t, repoDir, "add", relPath)
	runVersionGit(t, repoDir, "commit", "-m", commitMsg)
	return runVersionGitOutput(t, repoDir, "rev-parse", "HEAD")
}

func runVersionCommand(t *testing.T, workDir, version, commit string) (string, string, error) {
	t.Helper()

	factory := NewCommandFactory(workDir)
	factory.Version = version
	factory.Commit = commit
	factory.Date = "2026-05-05T00:00:00Z"

	root := factory.NewRootCommand()
	root.SetArgs([]string{"version"})

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	root.SetOut(&stdoutBuf)
	root.SetErr(&stderrBuf)

	err := root.Execute()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func runVersionGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, string(out))
}

func runVersionGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.Output()
	require.NoError(t, err, "git %v", args)
	return string(bytes.TrimSpace(out))
}
