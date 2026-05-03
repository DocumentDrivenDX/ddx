package testutils

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// NewFixtureRepo creates a clean ddx-initialized git repo for the test under
// a temp dir, registers t.Cleanup, and returns the project path. profile must
// be one of: "minimal", "standard", "multi-project", "federated" (matches
// scripts/build-fixture-repo.sh). For multi-project / federated profiles the
// returned path is the parent dir; sub-projects live underneath (proj-a,
// proj-b / hub, spoke).
//
// The helper invokes scripts/build-fixture-repo.sh and points it at a ddx
// binary built lazily once per test process from this repo's cli/ module
// (override with DDX_BIN env var).
func NewFixtureRepo(t *testing.T, profile string) string {
	t.Helper()

	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-fixture-repo.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("fixture script not found at %s: %v", script, err)
	}

	dest := t.TempDir()
	bin := resolveDDxBinary(t, root)

	cmd := exec.Command("bash", script, dest, "--profile", profile)
	cmd.Env = append(os.Environ(), "DDX_BIN="+bin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build-fixture-repo.sh failed (profile=%s): %v\n%s", profile, err, stderr.String())
	}

	// t.TempDir() handles removal; nothing else to clean up.
	return dest
}

// repoRoot walks up from this file's location to find the directory holding
// scripts/build-fixture-repo.sh.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "build-fixture-repo.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root from " + file)
		}
		dir = parent
	}
}

var (
	ddxBuildOnce sync.Once
	ddxBuildPath string
	ddxBuildErr  error
)

// ResolveDDxBinary is the exported variant of resolveDDxBinary. Tests outside
// this package that need the ddx binary path (e.g. integration tests that
// spawn `ddx work`) call this so the lazily-built binary is shared across
// the test process.
func ResolveDDxBinary(t *testing.T) string {
	t.Helper()
	return resolveDDxBinary(t, repoRoot(t))
}

// resolveDDxBinary returns the ddx binary path the script should use:
//   - $DDX_BIN if set,
//   - else `ddx` from PATH if available,
//   - else a binary built once per test process from <repoRoot>/cli.
func resolveDDxBinary(t *testing.T, root string) string {
	t.Helper()
	if env := os.Getenv("DDX_BIN"); env != "" {
		return env
	}
	if path, err := exec.LookPath("ddx"); err == nil {
		return path
	}

	ddxBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ddx-fixture-bin-")
		if err != nil {
			ddxBuildErr = err
			return
		}
		out := filepath.Join(dir, "ddx")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = filepath.Join(root, "cli")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			ddxBuildErr = err
			ddxBuildPath = stderr.String()
			return
		}
		ddxBuildPath = out
	})
	if ddxBuildErr != nil {
		t.Fatalf("could not build ddx binary for fixture: %v\n%s", ddxBuildErr, ddxBuildPath)
	}
	return ddxBuildPath
}
