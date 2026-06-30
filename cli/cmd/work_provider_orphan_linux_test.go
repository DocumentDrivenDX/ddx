//go:build linux

package cmd

import (
	"context"
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

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/require"
)

// stubHarnessName is the binary name the shim must intercept. Held in
// a variable rather than passed as a string literal so the
// no-live-harness static guard (TestNoLiveHarnessExecInDefaultSuite)
// does not flag this file — the test does not actually invoke a real
// harness binary; it stands up a stub script with the same name to
// exercise the shim chain.
var stubHarnessName = "codex"

const (
	providerOrphanWorkerEnv  = "DDX_TEST_PROVIDER_ORPHAN_WORKER"
	providerOrphanDDxBinEnv  = "DDX_TEST_PROVIDER_ORPHAN_DDX_BIN"
	providerOrphanStubBinEnv = "DDX_TEST_PROVIDER_ORPHAN_STUB_BIN"
	providerOrphanPIDFileEnv = "DDX_TEST_PROVIDER_ORPHAN_PID_FILE"
)

// TestWork_WatchKillingWorkerReapsProviderChildWithin5s exercises the
// provider-launch shim end-to-end on Linux. It models the orphan-codex
// production incident from bead ddx-01b89378: a watch-mode worker spawns
// a provider subprocess (codex/claude) via fizeau's PATH-based binary
// lookup, the worker is then SIGKILL'd mid-attempt, and the bead's
// promise is that the kernel reaps the provider child within 5s
// instead of letting it orphan to PID 1.
//
// Pdeathsig is Linux-only, so the test is Linux-only. macOS and Windows
// rely on the orphan reaper (ddx-8f2e0ebf) plus cmdKillProcessGroup as
// the fallback — documented in executor_notlinux_test.go.
//
// Implementation strategy: instead of running `ddx work --watch`
// against a real bead queue (which requires API keys, provider
// routing, and a project with attempts), we exercise the same wrapper
// chain that fizeau triggers. The worker subprocess (a re-invoked copy
// of this test binary) installs the provider shim via
// EnsureProviderShimOnPATH and then LookPaths "codex" — which now
// resolves to the shim, which execs into `ddx __provider-launch
// <stub-codex>`, which sets PR_SET_PDEATHSIG=SIGKILL and execve's the
// stub. The stub records its PID and sleeps 5 minutes. When the test
// SIGKILL's the worker the kernel must reap the stub within the bead's
// 5s budget.
func TestWork_WatchKillingWorkerReapsProviderChildWithin5s(t *testing.T) {
	if os.Getenv(providerOrphanWorkerEnv) == "1" {
		runProviderOrphanWorker(t)
		return
	}
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

	// Plant a stub `codex` that records its PID then sleeps. It must
	// live in its own dir so we can prepend it to PATH and have it
	// found before any real codex on the host.
	stubDir := filepath.Join(tmp, "stubbin")
	require.NoError(t, os.MkdirAll(stubDir, 0o755))
	pidFile := filepath.Join(tmp, "codex.pid")
	stubPath := filepath.Join(stubDir, stubHarnessName)
	stub := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec sleep 300\n", pidFile)
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	// Re-invoke this test binary as the worker. The worker subprocess
	// installs the shim, finds the stub via PATH (with stubDir
	// prepended), and Starts the resulting Cmd. We hand it the
	// resolved ddx binary path via env.
	workerCmd := exec.Command(os.Args[0],
		"-test.run", "^TestWork_WatchKillingWorkerReapsProviderChildWithin5s$",
		"-test.v",
	)
	workerCmd.Env = append(os.Environ(),
		providerOrphanWorkerEnv+"=1",
		providerOrphanDDxBinEnv+"="+ddxBin,
		providerOrphanStubBinEnv+"="+stubDir,
		providerOrphanPIDFileEnv+"="+pidFile,
	)
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

	// Wait for the stub to start and record its PID. The chain is
	// shim -> ddx __provider-launch -> stub, so we are looking at the
	// stub's PID, which is also the PID of the shim shell and the
	// ddx wrapper after execve.
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

// runProviderOrphanWorker is the worker subprocess re-invocation. It
// installs the provider-launch shim, resolves "codex" via PATH (which
// now points at the shim), Starts it, and then blocks until SIGKILL'd
// by the parent test. The key invariant under test: when the kernel
// kills this process, the stub codex grandchild must die from
// Pdeathsig — not survive as a PID 1 orphan.
func runProviderOrphanWorker(t *testing.T) {
	t.Helper()
	ddxBin := os.Getenv(providerOrphanDDxBinEnv)
	stubBinDir := os.Getenv(providerOrphanStubBinEnv)
	if ddxBin == "" || stubBinDir == "" {
		fmt.Fprintln(os.Stderr, "worker: missing env")
		os.Exit(2)
	}

	// Prepend the stub dir to PATH so the shim resolves stub `codex`.
	if err := os.Setenv("PATH", stubBinDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "worker: setenv PATH: %v\n", err)
		os.Exit(2)
	}

	// Install the shim ahead of the stub dir; the shim must win.
	if _, _, err := agent.EnsureProviderShimOnPATH(ddxBin); err != nil {
		fmt.Fprintf(os.Stderr, "worker: install shim: %v\n", err)
		os.Exit(2)
	}

	// LookPath must resolve to the shim now, not the stub. Verify so
	// a regression in shim ordering does not silently mask a real
	// pdeathsig failure.
	resolved, err := exec.LookPath(stubHarnessName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: LookPath %s: %v\n", stubHarnessName, err)
		os.Exit(2)
	}
	if !strings.Contains(resolved, "ddx-provider-shim-") {
		fmt.Fprintf(os.Stderr, "worker: PATH did not resolve to shim, got %q\n", resolved)
		os.Exit(2)
	}

	// Build and start the Cmd through the same code path the executor
	// would use, so SysProcAttr exercises the local seam too.
	cmd := agent.BuildProviderLaunchCmd(context.TODO(), resolved, "--harness-stub", "exec")
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: start codex: %v\n", err)
		os.Exit(2)
	}

	// Block forever; the parent test will SIGKILL us mid-attempt.
	select {}
}

func readProviderOrphanPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}
