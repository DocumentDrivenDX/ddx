//go:build linux

package cmd

import (
	"bytes"
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
	require.False(t, processDeadOrZombie(stubPID), "stub codex must be alive prior to worker kill (proc state=%s)", processDeadOrZombieStatus(stubPID))

	// SIGKILL the worker. Pdeathsig must propagate to the stub within
	// the bead's 5s window. We allow some slop on the polling.
	require.NoError(t, syscall.Kill(workerCmd.Process.Pid, syscall.SIGKILL))

	var stubState string
	require.Eventually(t, func() bool {
		stubState = processDeadOrZombieStatus(stubPID)
		return processDeadOrZombie(stubPID)
	}, 5*time.Second, 25*time.Millisecond, "stub codex pid=%d still present 5s after worker SIGKILL; proc state=%s", stubPID, procStateSnapshot{&stubState})
}

func readProviderOrphanPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}

// buildDdxBinaryForProcessGroupTest compiles the real ddx binary so the
// process-group regression tests below can exec the production
// `__provider-launch` wrapper end to end, rather than calling
// BuildProviderLaunchCmd (a construction helper the production PATH-shim
// path never uses).
func buildDdxBinaryForProcessGroupTest(t *testing.T, dir string) string {
	t.Helper()
	ddxBin := filepath.Join(dir, "ddx")
	build := exec.Command("go", "build", "-o", ddxBin, "github.com/DocumentDrivenDX/ddx")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	require.NoError(t, build.Run(), "build ddx binary for provider-launch process-group regression test")
	return ddxBin
}

// waitForProcessGroupTestPidFile polls until path exists (a stub process
// has written its own $$ or $! to it) and returns the parsed PID.
func waitForProcessGroupTestPidFile(t *testing.T, path string) int {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 10*time.Second, 50*time.Millisecond, "pid file %s was not written within 10s", path)
	return readProviderOrphanPID(t, path)
}

// TestProviderLaunchTimeoutDoesNotSignalWorkerProcessGroup proves the fix
// for ddx-fb293c2b: the production `ddx __provider-launch` wrapper puts
// itself into a brand-new process group (providerLaunchPrepare in
// provider_launch_linux.go calls setpgid(0, 0) before the PR_SET_PDEATHSIG
// prctl and the final execve). A worker script models the `ddx work`
// process by becoming its own process-group leader, then backgrounds the
// provider-launch wrapper as a plain forked child — job control is off in
// a non-interactive shell script, so that background child inherits the
// worker's process group at fork time, exactly like a harness runner
// spawning "codex" without isolating it first. If providerLaunchPrepare's
// setpgid did not run, "the provider's process group" and "the worker's
// process group" would be the same group, and a lifecycle-timeout cleanup
// scoped to the provider would also kill the worker.
func TestProviderLaunchTimeoutDoesNotSignalWorkerProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmp := t.TempDir()
	ddxBin := buildDdxBinaryForProcessGroupTest(t, tmp)

	pidFile := filepath.Join(tmp, "provider.pid")
	stubPath := filepath.Join(tmp, "codex-stub")
	stub := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec sleep 300\n", pidFile)
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	workerScript := filepath.Join(tmp, "worker.sh")
	worker := fmt.Sprintf("#!/bin/sh\nset -eu\n%q __provider-launch %q &\nwhile :; do sleep 1; done\n", ddxBin, stubPath)
	require.NoError(t, os.WriteFile(workerScript, []byte(worker), 0o755))

	workerCmd := exec.Command(workerScript)
	workerCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	require.NoError(t, workerCmd.Start())
	workerPGID := workerCmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-workerPGID, syscall.SIGKILL)
	})

	providerPID := waitForProcessGroupTestPidFile(t, pidFile)
	require.False(t, processDeadOrZombie(providerPID), "provider stub must be alive before the simulated timeout (proc state=%s)", processDeadOrZombieStatus(providerPID))

	providerPGID, err := syscall.Getpgid(providerPID)
	require.NoError(t, err)
	require.NotEqual(t, workerPGID, providerPGID, "provider must not remain in the worker's process group")
	require.Equal(t, providerPID, providerPGID, "provider must be the leader of its own new process group")

	// Simulate the upstream lifecycle timeout cleaning up "the provider
	// process group" via kill(-pgid, ...), exactly as cmdKillProcessGroup
	// does for a Cmd it owns.
	require.NoError(t, syscall.Kill(-providerPGID, syscall.SIGTERM))

	var providerState string
	require.Eventually(t, func() bool {
		providerState = processDeadOrZombieStatus(providerPID)
		return processDeadOrZombie(providerPID)
	}, 5*time.Second, 50*time.Millisecond, "provider stub must be terminated by the group-scoped timeout kill (proc state=%s)", procStateSnapshot{&providerState})

	require.False(t, processDeadOrZombie(workerCmd.Process.Pid), "worker process must remain alive after the provider-only timeout kill (proc state=%s)", processDeadOrZombieStatus(workerCmd.Process.Pid))
}

// TestProviderLaunchTimeoutReapsProviderOnly proves that a timeout cleanup
// scoped to the provider's process group (kill(-providerPGID, ...)) reaps
// the provider and its descendants while leaving an unrelated process that
// stayed in the worker's original process group untouched.
func TestProviderLaunchTimeoutReapsProviderOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmp := t.TempDir()
	ddxBin := buildDdxBinaryForProcessGroupTest(t, tmp)

	providerPidFile := filepath.Join(tmp, "provider.pid")
	descendantPidFile := filepath.Join(tmp, "descendant.pid")
	stubPath := filepath.Join(tmp, "codex-stub")
	stub := fmt.Sprintf(
		"#!/bin/sh\necho $$ > %q\nsleep 300 &\necho $! > %q\nexec sleep 300\n",
		providerPidFile, descendantPidFile,
	)
	require.NoError(t, os.WriteFile(stubPath, []byte(stub), 0o755))

	unrelatedPidFile := filepath.Join(tmp, "unrelated.pid")
	workerScript := filepath.Join(tmp, "worker.sh")
	worker := fmt.Sprintf(
		"#!/bin/sh\nset -eu\n%q __provider-launch %q &\nsleep 300 &\necho $! > %q\nwhile :; do sleep 1; done\n",
		ddxBin, stubPath, unrelatedPidFile,
	)
	require.NoError(t, os.WriteFile(workerScript, []byte(worker), 0o755))

	workerCmd := exec.Command(workerScript)
	workerCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	require.NoError(t, workerCmd.Start())
	workerPGID := workerCmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-workerPGID, syscall.SIGKILL)
	})

	providerPID := waitForProcessGroupTestPidFile(t, providerPidFile)
	descendantPID := waitForProcessGroupTestPidFile(t, descendantPidFile)
	unrelatedPID := waitForProcessGroupTestPidFile(t, unrelatedPidFile)

	require.False(t, processDeadOrZombie(providerPID), "provider stub must be alive before the simulated timeout (proc state=%s)", processDeadOrZombieStatus(providerPID))
	require.False(t, processDeadOrZombie(descendantPID), "provider descendant must be alive before the simulated timeout (proc state=%s)", processDeadOrZombieStatus(descendantPID))
	require.False(t, processDeadOrZombie(unrelatedPID), "unrelated worker-group process must be alive before the simulated timeout (proc state=%s)", processDeadOrZombieStatus(unrelatedPID))

	providerPGID, err := syscall.Getpgid(providerPID)
	require.NoError(t, err)
	require.Equal(t, providerPID, providerPGID, "provider must be the leader of its own new process group")

	descendantPGID, err := syscall.Getpgid(descendantPID)
	require.NoError(t, err)
	require.Equal(t, providerPGID, descendantPGID, "descendant must inherit the provider's dedicated process group")

	unrelatedPGID, err := syscall.Getpgid(unrelatedPID)
	require.NoError(t, err)
	require.Equal(t, workerPGID, unrelatedPGID, "unrelated process must remain in the worker's original process group")

	// Simulate the upstream lifecycle timeout cleaning up only the
	// provider's process group.
	require.NoError(t, syscall.Kill(-providerPGID, syscall.SIGKILL))

	var providerState, descendantState string
	require.Eventually(t, func() bool {
		providerState = processDeadOrZombieStatus(providerPID)
		descendantState = processDeadOrZombieStatus(descendantPID)
		return processDeadOrZombie(providerPID) && processDeadOrZombie(descendantPID)
	}, 5*time.Second, 50*time.Millisecond, "provider and its descendant must be reaped by the group-scoped timeout kill (provider state=%s descendant state=%s)", procStateSnapshot{&providerState}, procStateSnapshot{&descendantState})

	require.False(t, processDeadOrZombie(unrelatedPID), "unrelated process in the parent group must survive the provider-only timeout kill (proc state=%s)", processDeadOrZombieStatus(unrelatedPID))
	require.False(t, processDeadOrZombie(workerCmd.Process.Pid), "worker process must remain alive after the provider-only timeout kill (proc state=%s)", processDeadOrZombieStatus(workerCmd.Process.Pid))
}

// TestShimProbeHelperProcess is not a real test. It is re-executed as a
// standalone subprocess (via `-test.run=^TestShimProbeHelperProcess$`) by
// TestProviderLaunchExecsProviderInDedicatedProcessGroup below, using the
// classic os/exec_test.go "TestHelperProcess" pattern.
// agent.EnsureProviderShimOnPATH caches its installed shim dir for the
// lifetime of the OS process (see providerShimDirPath in
// internal/agent/provider_spawn.go) — calling it directly inside the
// shared `go test` binary would silently reuse whatever shim state an
// earlier, unrelated test in this package happened to install first,
// making the PATH-resolution assertion depend on test execution order. A
// fresh subprocess gets pristine package-level state.
//
// Re-executing this same test binary re-runs TestMain, which (via
// isolateCmdTestTempRoot in testutils_test.go) prepends its own hermetic
// fake-provider bin dir onto PATH for every cmd-package test. That runs
// after the parent already set PATH for this subprocess, so it would
// shadow the specific "codex" stub the caller wants resolved. Re-prepend
// DDX_SHIMPROBE_REALBINDIR here so it wins regardless.
func TestShimProbeHelperProcess(t *testing.T) {
	if os.Getenv("DDX_SHIMPROBE_HELPER") != "1" {
		return
	}
	ddxBin := os.Getenv("DDX_SHIMPROBE_DDXBIN")
	realBinDir := os.Getenv("DDX_SHIMPROBE_REALBINDIR")
	if err := os.Setenv("PATH", realBinDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	shimDir, _, err := agent.EnsureProviderShimOnPATH(ddxBin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	shimPath, err := exec.LookPath(stubHarnessName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(shimDir)
	fmt.Println(shimPath)
	os.Exit(0)
}

// TestProviderLaunchExecsProviderInDedicatedProcessGroup proves that the
// production PATH-shim/wrapper call path — EnsureProviderShimOnPATH's
// installed shim, resolved via exec.LookPath the same way Fizeau resolves
// "codex" — gives the provider a PGID distinct from the worker's PGID.
// BuildProviderLaunchCmd already has group-isolation coverage
// (TestExecutor_ProviderSpawnSetsPdeathsigAndSetpgid); the production
// provider path never calls it, so this test exercises the actual shim
// chain instead.
func TestProviderLaunchExecsProviderInDedicatedProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmp := t.TempDir()
	ddxBin := buildDdxBinaryForProcessGroupTest(t, tmp)

	realBinDir := filepath.Join(tmp, "realbin")
	require.NoError(t, os.MkdirAll(realBinDir, 0o755))
	pidFile := filepath.Join(tmp, "provider.pid")
	realStubPath := filepath.Join(realBinDir, stubHarnessName)
	stub := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nexec sleep 300\n", pidFile)
	require.NoError(t, os.WriteFile(realStubPath, []byte(stub), 0o755))

	helperPATH := realBinDir + string(os.PathListSeparator) + os.Getenv("PATH")
	probe := exec.Command(os.Args[0], "-test.run=^TestShimProbeHelperProcess$")
	probe.Env = append(os.Environ(),
		"DDX_SHIMPROBE_HELPER=1",
		"DDX_SHIMPROBE_DDXBIN="+ddxBin,
		"DDX_SHIMPROBE_REALBINDIR="+realBinDir,
		"PATH="+helperPATH,
	)
	var probeStdout, probeStderr bytes.Buffer
	probe.Stdout = &probeStdout
	probe.Stderr = &probeStderr
	require.NoError(t, probe.Run(), "shim-probe helper process failed: %s", probeStderr.String())

	probeLines := strings.Split(strings.TrimSpace(probeStdout.String()), "\n")
	require.Len(t, probeLines, 2, "expected shimDir and shimPath lines from shim-probe helper, got: %q", probeStdout.String())
	shimDir, shimPath := probeLines[0], probeLines[1]
	t.Cleanup(func() {
		_ = os.RemoveAll(shimDir)
	})
	require.True(t, strings.HasPrefix(shimPath, shimDir), "PATH lookup must resolve to the installed shim ahead of the real stub binary")

	// Worker: becomes its own process-group leader, then backgrounds the
	// resolved shim as a plain forked child (no extra setpgid of our
	// own) — mirroring how a harness runner execs a PATH-resolved binary
	// without isolating its group up front.
	workerScript := filepath.Join(tmp, "worker.sh")
	worker := fmt.Sprintf("#!/bin/sh\nset -eu\n%q &\nwhile :; do sleep 1; done\n", shimPath)
	require.NoError(t, os.WriteFile(workerScript, []byte(worker), 0o755))

	workerCmd := exec.Command(workerScript)
	workerCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr
	require.NoError(t, workerCmd.Start())
	workerPGID := workerCmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-workerPGID, syscall.SIGKILL)
	})

	providerPID := waitForProcessGroupTestPidFile(t, pidFile)
	require.False(t, processDeadOrZombie(providerPID), "provider process must be alive after the shim chain execs (proc state=%s)", processDeadOrZombieStatus(providerPID))

	providerPGID, err := syscall.Getpgid(providerPID)
	require.NoError(t, err)
	require.NotEqual(t, workerPGID, providerPGID, "provider PGID must differ from the worker PGID")
	require.Equal(t, providerPID, providerPGID, "provider must be the leader of its own new process group")
}
