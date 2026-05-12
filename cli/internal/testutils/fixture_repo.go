package testutils

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// NewFixtureRepo creates a fresh ddx-initialized project under t.TempDir()
// using scripts/build-fixture-repo.sh. The returned path is cleaned up by
// the testing package automatically.
func NewFixtureRepo(t *testing.T, profile string) string {
	t.Helper()

	dest := t.TempDir()
	script := fixtureRepoScriptPath(t)

	bin := os.Getenv("DDX_BIN")
	if bin == "" {
		if path, err := exec.LookPath("ddx"); err == nil {
			bin = path
		} else {
			bin = buildDDXBinary(t)
		}
	}

	cmd := exec.Command("bash", script, dest, "--profile", profile)
	cmd.Env = append(os.Environ(), "DDX_BIN="+bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build-fixture-repo.sh failed (profile=%s): %v\n%s", profile, err, out)
	}

	return dest
}

func fixtureRepoScriptPath(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cliRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	return filepath.Join(cliRoot, "scripts", "build-fixture-repo.sh")
}

func buildDDXBinary(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	cliRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ddx")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, ".")
	cmd.Dir = cliRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ddx: %v\n%s", err, out)
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		t.Fatalf("chmod ddx binary: %v", err)
	}
	return binPath
}
