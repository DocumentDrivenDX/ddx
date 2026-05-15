package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/stretchr/testify/require"
)

func TestWorktreeRegistry_OperatorRePin(t *testing.T) {
	firstProjectRoot := filepath.Join(t.TempDir(), "demo-project-a")
	initProjectWorktreeTestRepo(t, firstProjectRoot)
	runProjectWorktreeTestGit(t, firstProjectRoot, "remote", "add", "origin", "git@github.com:easel/ddx.git")

	secondProjectRoot := filepath.Join(t.TempDir(), "demo-project-b")
	initProjectWorktreeTestRepo(t, secondProjectRoot)
	runProjectWorktreeTestGit(t, secondProjectRoot, "remote", "add", "origin", "git@github.com:easel/ddx.git")

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	require.Equal(t, ddxroot.Path(context.Background(), firstProjectRoot), ddxroot.Path(context.Background(), secondProjectRoot))

	rootCmd := NewCommandFactory(firstProjectRoot).NewRootCommand()
	out, err := executeCommand(rootCmd, "project", "worktree", "master", secondProjectRoot)
	require.NoError(t, err, out)

	registry, err := ddxroot.LoadWorktreeRegistry(context.Background(), firstProjectRoot)
	require.NoError(t, err)

	secondAbs, err := filepath.Abs(secondProjectRoot)
	require.NoError(t, err)
	require.Equal(t, secondAbs, registry.Master)
}

func initProjectWorktreeTestRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	runProjectWorktreeTestGit(t, dir, "init")
}

func runProjectWorktreeTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := gitpkg.Command(context.Background(), dir, args...).CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
}
