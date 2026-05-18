package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func writeAgentExecutionRootConfig(t *testing.T, projectRoot, root string) {
	t.Helper()
	path := filepath.Join(projectRoot, ddxroot.DirName, "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte("version: \"1.0\"\nexecutions:\n  temp_worktree_root: " + root + "\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestExecuteBeadWorktreePathUsesConfiguredRoot(t *testing.T) {
	t.Setenv(config.ExecutionWorktreeRootEnv, "")
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	writeAgentExecutionRootConfig(t, projectRoot, "configured-wt")

	got := executeBeadWorktreePath(projectRoot, "ddx-abc12345", "20260507T010203-deadbeef")
	want := filepath.Join(projectRoot, "configured-wt", ExecuteBeadWtPrefix+"ddx-abc12345-20260507T010203-deadbeef")
	if got != want {
		t.Fatalf("executeBeadWorktreePath = %q, want %q", got, want)
	}
}

func TestExecutionCleanupManagerUsesConfiguredRoot(t *testing.T) {
	t.Setenv(config.ExecutionWorktreeRootEnv, "")
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	writeAgentExecutionRootConfig(t, projectRoot, "configured-cleanup")

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	want := filepath.Join(projectRoot, "configured-cleanup")
	if mgr.TempRoot != want {
		t.Fatalf("TempRoot = %q, want %q", mgr.TempRoot, want)
	}
}

func TestLifecycleScratchDirUsesConfiguredScratchRoot(t *testing.T) {
	t.Setenv(config.ExecutionWorktreeRootEnv, "")
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	writeAgentExecutionRootConfig(t, projectRoot, "configured/ddx-exec-wt")

	dir, err := newLifecycleScratchDir(projectRoot)
	if err != nil {
		t.Fatalf("newLifecycleScratchDir error = %v", err)
	}
	defer os.RemoveAll(dir)

	wantPrefix := filepath.Join(projectRoot, "configured") + string(filepath.Separator)
	if !strings.HasPrefix(dir, wantPrefix) {
		t.Fatalf("lifecycle scratch dir = %q, want prefix %q", dir, wantPrefix)
	}
}
