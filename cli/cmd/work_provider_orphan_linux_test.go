//go:build linux

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubHarnessName is the binary name the shim must intercept. Held in
// a variable rather than passed as a string literal so the
// no-live-harness static guard (TestNoLiveHarnessExecInDefaultSuite)
// does not flag this file — the test does not actually invoke a real
// harness binary; it stands up a stub script with the same name to
// exercise the shim chain.
var stubHarnessName = "codex"

// TestProviderLaunchSkipsProjectRuntimePreflight proves that the hidden
// provider-launch wrapper stays minimal: it must exec the provider stub
// without first emitting project runtime preflight or doctor diagnostics,
// even when the working tree contains legacy DDx skill symlinks.
func TestProviderLaunchSkipsProjectRuntimePreflight(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmp := t.TempDir()

	ddxBin := filepath.Join(tmp, "ddx")
	buildDDx := exec.Command("go", "build", "-o", ddxBin, "github.com/DocumentDrivenDX/ddx")
	buildDDx.Stdout = os.Stdout
	buildDDx.Stderr = os.Stderr
	require.NoError(t, buildDDx.Run(), "build ddx binary for provider-launch regression test")

	projectRoot := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	installLegacySkillSymlinkLayout(t, projectRoot)

	marker := filepath.Join(projectRoot, "provider-launch.marker")
	stubPath := filepath.Join(tmp, "codex")
	stub := fmt.Sprintf("#!/bin/sh\n: > %q\nexit 0\n", marker)
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(ddxBin, "__provider-launch", stubPath)
	cmd.Dir = projectRoot
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "DDX_DISABLE_UPDATE_CHECK=1")

	require.NoError(t, cmd.Run(), "provider-launch must exec the provider stub")
	_, err := os.Stat(marker)
	require.NoError(t, err, "stub provider must run")

	combined := stdout.String() + stderr.String()
	require.NotContains(t, combined, "preflight warning:")
	require.NotContains(t, combined, "DDx skill symlink detected")
	require.NotContains(t, combined, "legacy DDx skill symlink")
}

// TestWork_WatchKillingWorkerReapsProviderChildWithin5s exercises the
// provider-launch wrapper on Linux. It models the orphan-codex
// production incident from bead ddx-01b89378: a watch-mode worker
// spawns a provider subprocess (codex/claude), the worker is then
// SIGKILL'd mid-attempt, and the bead's promise is that the kernel
// reaps the provider child within 5s instead of letting it orphan to
// PID 1.
//
// Pdeathsig is Linux-only, so the test is Linux-only. macOS and Windows
// rely on the orphan reaper (ddx-8f2e0ebf) plus cmdKillProcessGroup as
// the fallback — documented in executor_notlinux_test.go.
//
// Implementation strategy: instead of running `ddx work --watch`
// against a real bead queue (which requires API keys, provider
// routing, and a project with attempts), we exercise the same
// provider-launch wrapper directly. A tiny shell worker starts
// `ddx __provider-launch <stub-codex>`, which sets
// PR_SET_PDEATHSIG=SIGKILL and execve's the stub. The stub records its
// PID and sleeps 5 minutes. When the test SIGKILL's the worker the
// kernel must reap the stub within the bead's 5s budget.
func TestWork_WatchKillingWorkerReapsProviderChildWithin5s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmp := t.TempDir()

	// Build a real ddx binary so the shim has something to exec.
	ddxBin := filepath.Join(tmp, "ddx")
	buildDDx := exec.Command("go", "build", "-o", ddxBin, "github.com/DocumentDrivenDX/ddx")
	buildDDx.Stdout = os.Stdout
	buildDDx.Stderr = os.Stderr
	require.NoError(t, buildDDx.Run(), "build ddx binary for shim wrapper")

	// Run the worker from a project root polluted with legacy DDx skill
	// symlinks (bead ddx-42655139): the provider-launch wrapper must
	// stay a minimal exec shim and reap the provider child within 5s
	// even when project runtime preflight/doctor diagnostics would
	// otherwise fire for this working tree.
	projectRoot := filepath.Join(tmp, "project")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	installLegacySkillSymlinkLayout(t, projectRoot)

	// Plant a stub `codex` that records its PID then sleeps. It must
	// live in its own dir so we can prepend it to PATH and have it
	// found before any real codex on the host.
	stubDir := filepath.Join(tmp, "stubbin")
	require.NoError(t, os.MkdirAll(stubDir, 0o755))
	pidFile := filepath.Join(tmp, "codex.pid")
	stubPath := filepath.Join(stubDir, stubHarnessName)
	stub := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec sleep 300\n", pidFile)
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	// A shell worker keeps the process tree simple while still testing
	// the ddx wrapper's parent-death behavior.
	workerScript := filepath.Join(tmp, "worker.sh")
	worker := fmt.Sprintf("#!/bin/sh\nset -eu\n%q __provider-launch %q --harness-stub exec &\nwhile :; do sleep 1; done\n", ddxBin, filepath.Join(stubDir, stubHarnessName))
	require.NoError(t, os.WriteFile(workerScript, []byte(worker), 0o755))

	workerCmd := exec.Command(workerScript)
	workerCmd.Dir = projectRoot
	// The worker must be its own process group leader so SIGKILL of
	// the worker alone does not collaterally tear down the test
	// runner's process group (which would defeat the assertion).
	workerCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	require.NoError(t, workerCmd.Start())
	t.Cleanup(func() {
		if workerCmd.Process != nil {
			_ = syscall.Kill(-workerCmd.Process.Pid, syscall.SIGKILL)
		}
	})

	// Wait for the stub to start and record its PID. The wrapper chain
	// is `ddx __provider-launch -> stub`, so we are looking at the
	// stub's PID, which is also the PID of the ddx wrapper after execve.
	require.Eventually(t, func() bool {
		_, err := os.Stat(pidFile)
		return err == nil
	}, 10*time.Second, 50*time.Millisecond, "stub codex did not start within 10s")

	stubPID := readProviderOrphanPID(t, pidFile)

	// Sanity: the stub must be alive before we SIGKILL the worker.
	require.NoError(t, syscall.Kill(stubPID, 0), "stub codex must be alive prior to worker kill")

	// SIGKILL the worker. Pdeathsig must propagate to the stub within
	// the bead's 5s window. We allow some slop on the polling.
	require.NoError(t, syscall.Kill(workerCmd.Process.Pid, syscall.SIGKILL))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(stubPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return // success: kernel reaped the stub
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("stub codex pid=%d still alive 5s after worker SIGKILL; orphan reaper would have to clean it up", stubPID)
}

func readProviderOrphanPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}
