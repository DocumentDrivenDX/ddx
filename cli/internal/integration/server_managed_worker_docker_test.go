package integration

// server_managed_worker_docker_test.go — Docker-backed end-to-end proof that
// server-managed worker cleanup leaves no process leaks (plan
// IP-2026-06-13-server-managed-workers, Phase 4 / Testing Strategy).
//
// The heavy lifting lives in scripts/integration/server-managed-workers-docker.sh,
// which builds ddx from the current source, bakes it plus the in-container
// scenario into a throwaway image, and runs the scenario in an isolated
// container. This Go test is the thin wrapper that the documented command
//
//	cd cli && go test ./internal/integration/... ./internal/server/... \
//	    -run 'ServerManagedWorker|NoProcessLeaks'
//
// dispatches: it skips with a clear message when Docker is unavailable (or in
// -short mode) and otherwise asserts the scenario exits 0 with no leaks.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRootFromTest walks up from this test file to the repository root (the
// directory containing scripts/integration/), so the test is independent of the
// working directory go test runs it from.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate repo root")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "integration")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find scripts/integration walking up from %s", filepath.Dir(thisFile))
		}
		dir = parent
	}
}

// dockerAvailable reports whether a usable Docker daemon is reachable.
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// TestIntegration_ServerManagedWorker_NoProcessLeaks builds and runs the
// isolated Docker scenario that proves explicit stop, double stop, watchdog
// reap, and server shutdown each leave no managed `ddx work`, fake claude/codex,
// shell, or sleep descendants behind.
//
// It SKIPS with a clear message when Docker is unavailable (so it is safe on
// developer machines without a daemon) and in -short mode (the scenario takes
// roughly two minutes, dominated by one real watchdog sweep interval).
func TestIntegration_ServerManagedWorker_NoProcessLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: builds a Docker image and runs a ~2m multi-phase scenario")
	}
	if !dockerAvailable() {
		t.Skip("integration: Docker is unavailable (CLI missing or daemon unreachable); " +
			"run on a Docker-enabled host to exercise server-managed worker leak checks")
	}

	root := repoRootFromTest(t)
	script := filepath.Join(root, "scripts", "integration", "server-managed-workers-docker.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("docker scenario script missing: %v", err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	t.Logf("server-managed-workers-docker.sh output:\n%s", out)
	if err != nil {
		t.Fatalf("docker scenario failed: %v", err)
	}

	output := string(out)
	// The script exits 0 and prints this skip marker when Docker turned out to
	// be unavailable after all — surface that as a skip rather than a false pass.
	if strings.Contains(output, "SKIP: docker unavailable") {
		t.Skip("integration: docker became unavailable during the run")
	}
	if !strings.Contains(output, "SCENARIO PASS") {
		t.Fatalf("scenario did not report SCENARIO PASS; output above")
	}
}
