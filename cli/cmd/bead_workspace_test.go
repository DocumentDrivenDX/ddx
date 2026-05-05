package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeadWorkspaceRoot_RelativeEnvInsideLinkedWorktreeUsesPrimaryWorkspace(t *testing.T) {
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "ddx")
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".ddx", "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"ddx\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("fixture\n"), 0o644))

	runGitForWorkspaceTest(t, projectRoot, "init")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.name", "Test")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.email", "test@example.com")
	runGitForWorkspaceTest(t, projectRoot, "config", "extensions.worktreeConfig", "true")
	runGitForWorkspaceTest(t, projectRoot, "add", "README.md", ".ddx/config.yaml")
	runGitForWorkspaceTest(t, projectRoot, "commit", "-m", "init")

	worktreeRoot := filepath.Join(tmp, ".execute-bead-wt-ddx-12345678-20260505T000000-deadbeef")
	runGitForWorkspaceTest(t, projectRoot, "worktree", "add", "--detach", worktreeRoot, "HEAD")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreeRoot).Run()
	})

	t.Setenv("DDX_BEAD_DIR", ".ddx")
	factory := NewCommandFactory(worktreeRoot)

	got := factory.beadWorkspaceRoot()
	require.Equal(t, projectRoot, got,
		"relative DDX_BEAD_DIR inside execute-bead worktrees must resolve to the primary workspace, not the worktree")
}

func runGitForWorkspaceTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s\n%s", strings.Join(args, " "), string(out))
}
