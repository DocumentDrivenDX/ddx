package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func writeExecutionRootConfig(t *testing.T, path, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte("version: \"1.0\"\nexecutions:\n  temp_worktree_root: " + root + "\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestExecutionWorktreeRoot_ProjectConfigBeatsGlobal(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()

	writeExecutionRootConfig(t, filepath.Join(home, ddxroot.DirName, "config.yaml"), "global-root")
	writeExecutionRootConfig(t, filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), "project-root")

	got := ExecutionWorktreeRoot(projectRoot)
	want := filepath.Join(projectRoot, "project-root")
	if got != want {
		t.Fatalf("ExecutionWorktreeRoot = %q, want %q", got, want)
	}
}

func TestExecutionWorktreeRoot_GlobalConfigWhenProjectUnset(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()

	writeExecutionRootConfig(t, filepath.Join(home, ddxroot.DirName, "config.yaml"), "global-root")

	got := ExecutionWorktreeRoot(projectRoot)
	want := filepath.Join(home, "global-root")
	if got != want {
		t.Fatalf("ExecutionWorktreeRoot = %q, want %q", got, want)
	}
}

func TestExecutionWorktreeRoot_EnvOverridesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()
	t.Setenv(ExecutionWorktreeRootEnv, filepath.Join(home, "env-root"))

	writeExecutionRootConfig(t, filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), "project-root")

	got := ExecutionWorktreeRoot(projectRoot)
	want := filepath.Join(home, "env-root")
	if got != want {
		t.Fatalf("ExecutionWorktreeRoot = %q, want %q", got, want)
	}
}

func TestExecutionWorktreeRoot_ExpandsTilde(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()

	writeExecutionRootConfig(t, filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), "~/ddx-worktrees")

	got := ExecutionWorktreeRoot(projectRoot)
	want := filepath.Join(home, "ddx-worktrees")
	if got != want {
		t.Fatalf("ExecutionWorktreeRoot = %q, want %q", got, want)
	}
}

func TestExecutionTempRoot_DefaultsUnderUserCache(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheRoot := filepath.Join(home, "cache")
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	projectRoot := t.TempDir()

	got := ExecutionTempRoot(projectRoot)
	want := filepath.Join(cacheRoot, "ddx", "exec-wt")
	if got != want {
		t.Fatalf("ExecutionTempRoot = %q, want %q", got, want)
	}
}

func TestExecutionScratchRoot_SitsBesideConfiguredTempRoot(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()
	writeExecutionRootConfig(t, filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), "configured/ddx-exec-wt")

	got := ExecutionScratchRoot(projectRoot)
	want := filepath.Join(projectRoot, "configured")
	if got != want {
		t.Fatalf("ExecutionScratchRoot = %q, want %q", got, want)
	}
}

func TestMkdirExecutionScratch_UsesConfiguredScratchRoot(t *testing.T) {
	t.Setenv(ExecutionWorktreeRootEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectRoot := t.TempDir()
	writeExecutionRootConfig(t, filepath.Join(projectRoot, ddxroot.DirName, "config.yaml"), "configured/ddx-exec-wt")

	dir, err := MkdirExecutionScratch(projectRoot, "ddx-test-")
	if err != nil {
		t.Fatalf("MkdirExecutionScratch error = %v", err)
	}
	defer os.RemoveAll(dir)

	wantPrefix := filepath.Join(projectRoot, "configured") + string(filepath.Separator)
	if !strings.HasPrefix(dir, wantPrefix) {
		t.Fatalf("scratch dir = %q, want prefix %q", dir, wantPrefix)
	}
}
