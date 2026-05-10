// Package testutils provides shared test helpers for DDx CLI tests.
package testutils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	builtBinaryOnce sync.Once
	builtBinaryPath string
)

// NewFixtureRepo creates a ddx-initialized git repo under a t.TempDir()-scoped
// directory and registers t.Cleanup to remove it. The returned path is the
// project root (or the parent dir for multi-project / federated profiles).
//
// Valid profiles: minimal, standard, multi-project, federated.
//
// It uses $DDX_BIN if set, falls back to ddx on PATH, or builds the binary
// from cli/ once per test binary invocation.
func NewFixtureRepo(t *testing.T, profile string) string {
	t.Helper()

	dest := t.TempDir()
	ddxBin := resolveDDxBinary(t)

	_, filename, _, _ := runtime.Caller(0)
	// fixture.go is at cli/internal/testutils/fixture.go; script is at cli/../scripts/
	scriptPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "scripts", "build-fixture-repo.sh")
	scriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("testutils.NewFixtureRepo: cannot resolve script path: %v", err)
	}

	cmd := exec.Command("bash", scriptPath, dest, "--profile", profile)
	cmd.Env = append(os.Environ(), "DDX_BIN="+ddxBin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("testutils.NewFixtureRepo: build-fixture-repo.sh failed: %v\n%s", err, out)
	}

	return dest
}

// resolveDDxBinary returns the path to the ddx binary for fixture scripts.
// Priority: $DDX_BIN > ddx on PATH > build from cli/ (cached per process).
func resolveDDxBinary(t *testing.T) string {
	t.Helper()

	if bin := os.Getenv("DDX_BIN"); bin != "" {
		return bin
	}
	if path, err := exec.LookPath("ddx"); err == nil {
		return path
	}

	// Build once per test binary run.
	builtBinaryOnce.Do(func() {
		_, filename, _, _ := runtime.Caller(0)
		cliDir := filepath.Join(filepath.Dir(filename), "..", "..")
		cliDir, _ = filepath.Abs(cliDir)

		tmpBin, err := os.CreateTemp("", "ddx-testutils-*")
		if err != nil {
			return
		}
		tmpBin.Close()

		buildCmd := exec.Command("go", "build", "-buildvcs=false", "-o", tmpBin.Name(), ".")
		buildCmd.Dir = cliDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			_ = os.Remove(tmpBin.Name())
			// Leave builtBinaryPath empty; callers will fail with a clear message.
			_ = out
			return
		}
		_ = os.Chmod(tmpBin.Name(), 0755)
		builtBinaryPath = tmpBin.Name()
	})

	if builtBinaryPath == "" {
		t.Fatal("testutils.NewFixtureRepo: ddx binary not found and build failed; set $DDX_BIN")
	}
	return builtBinaryPath
}
