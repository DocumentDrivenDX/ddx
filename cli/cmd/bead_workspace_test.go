package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

func TestBeadWorkspaceRoot_RelativeEnvInsideLinkedWorktreeUsesPrimaryWorkspace(t *testing.T) {
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "ddx")
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"ddx\"\n"), 0o644))
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

func TestBeadCreate_RelativeEnvInsideLinkedWorktreeUsesPrimaryNamingAndStore(t *testing.T) {
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "origin-tree")
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"origin\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("fixture\n"), 0o644))

	runGitForWorkspaceTest(t, projectRoot, "init")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.name", "Test")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.email", "test@example.com")
	runGitForWorkspaceTest(t, projectRoot, "config", "extensions.worktreeConfig", "true")
	runGitForWorkspaceTest(t, projectRoot, "add", "README.md", ".ddx/config.yaml", ".ddx/beads.jsonl")
	runGitForWorkspaceTest(t, projectRoot, "commit", "-m", "init")

	worktreeRoot := filepath.Join(tmp, ".execute-bead-wt-ddx-12345678-20260505T000000-deadbeef")
	runGitForWorkspaceTest(t, projectRoot, "worktree", "add", "--detach", worktreeRoot, "HEAD")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreeRoot).Run()
	})

	worktreeDDX := filepath.Join(worktreeRoot, ddxroot.DirName)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDDX, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"worktree\"\n"), 0o644))

	t.Setenv("DDX_BEAD_DIR", ".ddx")
	root := NewCommandFactory(worktreeRoot).NewRootCommand()
	out, err := executeCommand(root, "bead", "create", "worktree-created bead", "--priority", "1")
	require.NoError(t, err)

	createdID := strings.TrimSpace(out)
	require.True(t, strings.HasPrefix(createdID, "origin-"),
		"bead create from a linked execute-bead worktree must use the primary workspace naming convention, got %q", createdID)

	primaryStore := bead.NewStore(ddxDir)
	created, err := primaryStore.Get(createdID)
	require.NoError(t, err, "created bead must be written to the primary workspace store")
	require.Equal(t, "worktree-created bead", created.Title)

	worktreeStore := bead.NewStore(worktreeDDX)
	_, err = worktreeStore.Get(createdID)
	require.Error(t, err, "created bead must not be written to the isolated worktree store")
}

func TestBeadCreate_ExecuteWorktreeRealBinaryUsesOriginPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-binary worktree regression in short mode")
	}

	binary := getSmokeTestBinaryPath(t)

	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "origin-tree")
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"origin\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("fixture\n"), 0o644))

	runGitForWorkspaceTest(t, projectRoot, "init")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.name", "Test")
	runGitForWorkspaceTest(t, projectRoot, "config", "user.email", "test@example.com")
	runGitForWorkspaceTest(t, projectRoot, "config", "extensions.worktreeConfig", "true")
	runGitForWorkspaceTest(t, projectRoot, "add", "README.md", ".ddx/config.yaml", ".ddx/beads.jsonl")
	runGitForWorkspaceTest(t, projectRoot, "commit", "-m", "init")

	worktreeRoot := filepath.Join(tmp, ".execute-bead-wt-ddx-12345678-20260505T000000-deadbeef")
	runGitForWorkspaceTest(t, projectRoot, "worktree", "add", "--detach", worktreeRoot, "HEAD")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreeRoot).Run()
	})

	worktreeDDX := filepath.Join(worktreeRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(worktreeDDX, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDDX, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"worktree\"\n"), 0o644))

	cmd := exec.Command(binary, "bead", "create", "real subprocess bead", "--priority", "1")
	cmd.Dir = worktreeRoot
	cmd.Env = os.Environ()
	env := make([]string, 0, len(cmd.Env)+1)
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "DDX_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "DDX_BEAD_DIR=.ddx")
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ddx bead create failed: %s", string(out))

	createdID := strings.TrimSpace(string(out))
	require.Regexp(t, regexp.MustCompile(`^origin-[0-9a-f]{8}$`), createdID)
	require.NotRegexp(t, regexp.MustCompile(`^\.execute-bead-wt-`), createdID)

	primaryBeadsPath := filepath.Join(ddxDir, "beads.jsonl")
	primaryData, err := os.ReadFile(primaryBeadsPath)
	require.NoError(t, err)
	require.Contains(t, string(primaryData), createdID,
		"created bead must be persisted in the primary workspace .ddx/beads.jsonl")

	worktreeBeadsPath := filepath.Join(worktreeDDX, "beads.jsonl")
	if worktreeData, err := os.ReadFile(worktreeBeadsPath); err == nil {
		require.NotContains(t, string(worktreeData), createdID,
			"created bead must not be persisted in the isolated worktree .ddx/beads.jsonl")
	}

	primaryStore := bead.NewStore(ddxDir)
	created, err := primaryStore.Get(createdID)
	require.NoError(t, err)
	require.Equal(t, "real subprocess bead", created.Title)

	worktreeStore := bead.NewStore(worktreeDDX)
	_, err = worktreeStore.Get(createdID)
	require.Error(t, err, "created bead must not be written to the isolated worktree store")
}

func runGitForWorkspaceTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s\n%s", strings.Join(args, " "), string(out))
}
