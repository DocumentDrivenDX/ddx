//go:build linux

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realProcGroup is a real, long-lived process started in its own process group
// (pgid == leader pid) so that killProcessGroup(leaderPID) — which sends
// SIGKILL to -leaderPID — terminates the whole group. These helpers let the
// reaper tests exercise the production killProcessGroup against live processes
// rather than a fake killGroup recorder, so "the process group is gone within a
// bounded grace" is actually observed.
type realProcGroup struct {
	cmd           *exec.Cmd
	leaderPID     int
	grandchildPID int
	waitOnce      sync.Once
}

// wait reaps the leader zombie exactly once so its PID disappears from the
// process table after the group has been signalled.
func (g *realProcGroup) wait() {
	g.waitOnce.Do(func() { _ = g.cmd.Wait() })
}

// reapAsync reaps the leader in the background so a polling assertion can
// observe the PID vanish without blocking on Wait.
func (g *realProcGroup) reapAsync() {
	go g.wait()
}

// spawnRealOrphanGroup starts a single long-lived process as its own process
// group leader.
func spawnRealOrphanGroup(t *testing.T) *realProcGroup {
	t.Helper()
	cmd := exec.Command("sleep", "600")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	g := &realProcGroup{cmd: cmd, leaderPID: cmd.Process.Pid}
	t.Cleanup(func() {
		_ = syscall.Kill(-g.leaderPID, syscall.SIGKILL)
		g.wait()
	})
	return g
}

// spawnRealOrphanGroupWithGrandchild starts a process-group leader that forks a
// long-lived grandchild into the same process group, returning both PIDs.
func spawnRealOrphanGroupWithGrandchild(t *testing.T) *realProcGroup {
	t.Helper()
	// The leader backgrounds a child sleep (which inherits the leader's process
	// group in a non-job-control shell), prints the child PID, then waits so the
	// leader itself stays alive.
	cmd := exec.Command("sh", "-c", "sleep 600 & echo $!; wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	g := &realProcGroup{cmd: cmd, leaderPID: cmd.Process.Pid}

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	gc, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	g.grandchildPID = gc

	t.Cleanup(func() {
		_ = syscall.Kill(-g.leaderPID, syscall.SIGKILL)
		if g.grandchildPID > 0 {
			_ = syscall.Kill(g.grandchildPID, syscall.SIGKILL)
		}
		g.wait()
	})
	return g
}

func realKillGroup(pid int) error { return killProcessGroup(pid) }

func staleLeaseOwnerPID() int { return 1 << 30 }

const (
	orphanReaperLauncherHelperEnv        = "DDX_ORPHAN_REAPER_LAUNCHER_HELPER"
	orphanReaperLauncherChildEnv         = "DDX_ORPHAN_REAPER_LAUNCHER_CHILD"
	orphanReaperLauncherWorktreeEnv      = "DDX_ORPHAN_REAPER_LAUNCHER_WORKTREE"
	orphanReaperLauncherScriptEnv        = "DDX_ORPHAN_REAPER_LAUNCHER_SCRIPT"
	orphanReaperLauncherPIDEnv           = "DDX_ORPHAN_REAPER_LAUNCHER_PID_FILE"
	orphanReaperLauncherGrandchildPIDEnv = "DDX_ORPHAN_REAPER_LAUNCHER_GRANDCHILD_PID_FILE"
)

func launchRealOrphanHarness(t *testing.T, worktree, script string) int {
	return launchRealOrphanHarnessWithEnv(t, worktree, script, nil)
}

func launchRealOrphanHarnessWithEnv(t *testing.T, worktree, script string, extraEnv map[string]string) int {
	t.Helper()

	childDir := t.TempDir()
	childPath := filepath.Join(childDir, "claude")
	require.NoError(t, os.Symlink("/bin/sh", childPath))

	pidFile := filepath.Join(t.TempDir(), "leader.pid")
	helper := exec.Command(os.Args[0], "-test.run=^TestWorkStartupReaper_RealProcLauncherHelper$")
	helper.Env = append(os.Environ(),
		orphanReaperLauncherHelperEnv+"=1",
		orphanReaperLauncherChildEnv+"="+childPath,
		orphanReaperLauncherWorktreeEnv+"="+worktree,
		orphanReaperLauncherScriptEnv+"="+script,
		orphanReaperLauncherPIDEnv+"="+pidFile,
	)
	for key, value := range extraEnv {
		helper.Env = append(helper.Env, key+"="+value)
	}
	require.NoError(t, helper.Start())
	require.NoError(t, helper.Wait())

	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = killProcessGroup(pid)
	})
	return pid
}

func launchRealOrphanHarnessWithGrandchild(t *testing.T, worktree string) (int, int) {
	t.Helper()

	grandchildPIDFile := filepath.Join(t.TempDir(), "grandchild.pid")
	script := fmt.Sprintf(`sleep 600 & echo $! > "$%s"; wait`, orphanReaperLauncherGrandchildPIDEnv)
	leaderPID := launchRealOrphanHarnessWithEnv(t, worktree, script, map[string]string{
		orphanReaperLauncherGrandchildPIDEnv: grandchildPIDFile,
	})

	data, err := os.ReadFile(grandchildPIDFile)
	require.NoError(t, err)
	grandchildPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return leaderPID, grandchildPID
}

func waitForOrphanHarnessProcess(t *testing.T, scanner orphanHarnessProcessScanner, pid int) orphanHarnessProcess {
	t.Helper()

	var found orphanHarnessProcess
	require.Eventually(t, func() bool {
		procs, err := scanner.Scan(context.Background())
		if err != nil {
			return false
		}
		for _, proc := range procs {
			if proc.PID == pid {
				found = proc
				return true
			}
		}
		return false
	}, 15*time.Second, 20*time.Millisecond)
	return found
}

type orphanHarnessProcessScannerFunc func(context.Context) ([]orphanHarnessProcess, error)

func (f orphanHarnessProcessScannerFunc) Scan(ctx context.Context) ([]orphanHarnessProcess, error) {
	return f(ctx)
}

func withForcedOrphanPPIDs(inner orphanHarnessProcessScanner, pids ...int) orphanHarnessProcessScanner {
	forced := make(map[int]struct{}, len(pids))
	for _, pid := range pids {
		forced[pid] = struct{}{}
	}
	return orphanHarnessProcessScannerFunc(func(ctx context.Context) ([]orphanHarnessProcess, error) {
		procs, err := inner.Scan(ctx)
		if err != nil {
			return nil, err
		}
		out := append([]orphanHarnessProcess(nil), procs...)
		for i := range out {
			if _, ok := forced[out[i].PID]; ok {
				out[i].PPID = 1
			}
		}
		return out, nil
	})
}

func realOrphanHarnessScanner(pids ...int) orphanHarnessProcessScanner {
	return withForcedOrphanPPIDs(newOrphanHarnessProcessScanner(), pids...)
}

// TestWorkStartupReaper_RealProcLauncherHelper is the subprocess entrypoint
// used by the realproc tests to start a fixture harness process and then exit.
func TestWorkStartupReaper_RealProcLauncherHelper(t *testing.T) {
	if os.Getenv(orphanReaperLauncherHelperEnv) != "1" {
		return
	}

	childPath := os.Getenv(orphanReaperLauncherChildEnv)
	worktree := os.Getenv(orphanReaperLauncherWorktreeEnv)
	script := os.Getenv(orphanReaperLauncherScriptEnv)
	pidFile := os.Getenv(orphanReaperLauncherPIDEnv)
	if childPath == "" || worktree == "" || script == "" || pidFile == "" {
		t.Fatal("launcher helper env vars are required")
	}

	cmd := exec.Command(childPath, "-c", script)
	cmd.Dir = worktree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644))
}

// TestWorkStartupReaper_ProductionScannerDiscoversFixtureHarness verifies that
// the Linux /proc scanner still discovers a live fixture harness process.
func TestWorkStartupReaper_ProductionScannerDiscoversFixtureHarness(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	beadID := "ddx-deadbeef"
	worktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+beadID+"-20260602T170011-deadbeef")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	leaderPID := launchRealOrphanHarness(t, worktree, fmt.Sprintf("/bin/sleep %d", 600))
	scanner := newOrphanHarnessProcessScanner()
	proc := waitForOrphanHarnessProcess(t, scanner, leaderPID)
	require.Equal(t, leaderPID, proc.PID)
	require.Equal(t, worktree, proc.Cwd)
	require.Contains(t, proc.Command, "claude")
}

// TestWorkStartupReaper_KillsOrphanProcessGroup plants a real harness process
// group and asserts the startup reaper kills it within a bounded grace.
func TestWorkStartupReaper_KillsOrphanProcessGroup(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	beadID := "ddx-deadbeef"
	worktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+beadID+"-20260602T170011-deadbeef")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	leaderPID := launchRealOrphanHarness(t, worktree, fmt.Sprintf("/bin/sleep %d", 600))
	require.True(t, processAlive(leaderPID), "leader must start alive")
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: staleLeaseOwnerPID()},
		},
	}
	scanner := realOrphanHarnessScanner(leaderPID)
	proc := waitForOrphanHarnessProcess(t, scanner, leaderPID)
	require.Equal(t, leaderPID, proc.PID)
	require.Equal(t, 1, proc.PPID)
	require.Equal(t, worktree, proc.Cwd)

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []string{beadID}, store.released)

	var leaderState string
	require.Eventually(t, func() bool {
		leaderState = processDeadOrZombieStatus(leaderPID)
		return processDeadOrZombie(leaderPID)
	}, 3*time.Second, 20*time.Millisecond, "orphaned harness process group should be gone (proc state=%s)", procStateSnapshot{&leaderState})
}

// TestWorkStartupReaper_KillsOrphanGrandchildProcessGroup plants an orphaned
// harness that forks a long-lived grandchild into the same process group and
// asserts both child and grandchild are reaped.
func TestWorkStartupReaper_KillsOrphanGrandchildProcessGroup(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	beadID := "ddx-deadbeef"
	worktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+beadID+"-20260602T170011-deadbeef")
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	leaderPID, grandchildPID := launchRealOrphanHarnessWithGrandchild(t, worktree)
	require.Greater(t, grandchildPID, 0)
	require.True(t, processAlive(leaderPID), "leader must start alive")
	require.True(t, processAlive(grandchildPID), "grandchild must start alive")

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: staleLeaseOwnerPID()},
		},
	}
	scanner := realOrphanHarnessScanner(leaderPID)
	proc := waitForOrphanHarnessProcess(t, scanner, leaderPID)
	require.Equal(t, leaderPID, proc.PID)
	require.Equal(t, 1, proc.PPID)
	require.Equal(t, worktree, proc.Cwd)

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)

	var leaderState, grandState string
	require.Eventually(t, func() bool {
		leaderState = processDeadOrZombieStatus(leaderPID)
		grandState = processDeadOrZombieStatus(grandchildPID)
		return processDeadOrZombie(leaderPID) && processDeadOrZombie(grandchildPID)
	}, 3*time.Second, 20*time.Millisecond, "both harness child and grandchild should be reaped (leader state=%s grandchild state=%s)", procStateSnapshot{&leaderState}, procStateSnapshot{&grandState})
}

// TestWorkStartupReaper_DoesNotKillLiveOwnedHarness plants a live-owned harness
// (lease owner PID alive) alongside an orphaned one and asserts only the orphan
// is killed.
func TestWorkStartupReaper_DoesNotKillLiveOwnedHarness(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	orphanBead := "ddx-deadbeef"
	liveBead := "ddx-feedface"
	orphanWt := filepath.Join(tempRoot, ExecuteBeadWtPrefix+orphanBead+"-20260602T170011-deadbeef")
	liveWt := filepath.Join(tempRoot, ExecuteBeadWtPrefix+liveBead+"-20260602T170011-feedface")
	require.NoError(t, os.MkdirAll(orphanWt, 0o755))
	require.NoError(t, os.MkdirAll(liveWt, 0o755))

	orphanPID := launchRealOrphanHarness(t, orphanWt, fmt.Sprintf("/bin/sleep %d", 600))
	livePID := launchRealOrphanHarness(t, liveWt, fmt.Sprintf("/bin/sleep %d", 600))
	require.True(t, processAlive(orphanPID), "orphan harness must start alive")
	require.True(t, processAlive(livePID), "live-owned harness must start alive")

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			orphanBead: {BeadID: orphanBead, PID: staleLeaseOwnerPID()},
			liveBead:   {BeadID: liveBead, PID: os.Getpid()},
		},
	}
	scanner := realOrphanHarnessScanner(orphanPID, livePID)
	require.Equal(t, orphanPID, waitForOrphanHarnessProcess(t, scanner, orphanPID).PID)
	require.Equal(t, livePID, waitForOrphanHarnessProcess(t, scanner, livePID).PID)

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []string{orphanBead}, store.released)
	assert.Empty(t, store.events[liveBead], "live-owned harness must not be reaped")

	var orphanState string
	require.Eventually(t, func() bool {
		orphanState = processDeadOrZombieStatus(orphanPID)
		return processDeadOrZombie(orphanPID)
	}, 3*time.Second, 20*time.Millisecond, "orphan must be reaped (proc state=%s)", procStateSnapshot{&orphanState})
	var liveState string
	require.Never(t, func() bool {
		liveState = processDeadOrZombieStatus(livePID)
		return processDeadOrZombie(livePID)
	}, 300*time.Millisecond, 30*time.Millisecond, "live-owned harness must stay alive (proc state=%s)", procStateSnapshot{&liveState})
}

// TestWorkStartupReaper_DoesNotKillOtherWorkspaceHarness plants an orphan tied
// to another workspace/project (outside this project's execution root) and
// asserts the current workspace reaper leaves it alive while still reaping its
// own orphan.
func TestWorkStartupReaper_DoesNotKillOtherWorkspaceHarness(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	otherTempRoot := filepath.Join(t.TempDir(), "other-exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	require.NoError(t, os.MkdirAll(otherTempRoot, 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	projBead := "ddx-11111111"
	otherBead := "ddx-22222222"
	projWt := filepath.Join(tempRoot, ExecuteBeadWtPrefix+projBead+"-20260602T170011-aaaaaaaa")
	otherWt := filepath.Join(otherTempRoot, ExecuteBeadWtPrefix+otherBead+"-20260602T170011-bbbbbbbb")
	require.NoError(t, os.MkdirAll(projWt, 0o755))
	require.NoError(t, os.MkdirAll(otherWt, 0o755))

	projPID := launchRealOrphanHarness(t, projWt, fmt.Sprintf("/bin/sleep %d", 600))
	otherPID := launchRealOrphanHarness(t, otherWt, fmt.Sprintf("/bin/sleep %d", 600))
	require.True(t, processAlive(projPID), "project harness must start alive")
	require.True(t, processAlive(otherPID), "other-project harness must start alive")

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			projBead:  {BeadID: projBead, PID: staleLeaseOwnerPID()},
			otherBead: {BeadID: otherBead, PID: staleLeaseOwnerPID()},
		},
	}
	scanner := realOrphanHarnessScanner(projPID, otherPID)
	require.Equal(t, projPID, waitForOrphanHarnessProcess(t, scanner, projPID).PID)
	require.Equal(t, otherPID, waitForOrphanHarnessProcess(t, scanner, otherPID).PID)
	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped, "only the harness within this project's execution root should be reaped")
	require.Equal(t, []string{projBead}, store.released)
	assert.Empty(t, store.events[otherBead], "other-workspace harness must not be reaped")

	var projState string
	require.Eventually(t, func() bool {
		projState = processDeadOrZombieStatus(projPID)
		return processDeadOrZombie(projPID)
	}, 3*time.Second, 20*time.Millisecond, "this project's orphan must be reaped (proc state=%s)", procStateSnapshot{&projState})
	var otherState string
	require.Never(t, func() bool {
		otherState = processDeadOrZombieStatus(otherPID)
		return processDeadOrZombie(otherPID)
	}, 300*time.Millisecond, 30*time.Millisecond, "other-workspace harness must stay alive (proc state=%s)", procStateSnapshot{&otherState})
}

// TestWorkStartupReaper_RecordsOperatorAttention verifies cleanup emits durable
// operator-attention evidence (and a lease release) carrying the bead id, owner
// PID status, and a diagnosis.
func TestWorkStartupReaper_RecordsOperatorAttention(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	beadID := "ddx-deadbeef"
	worktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+beadID+"-20260602T170011-deadbeef")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	leaderPID := launchRealOrphanHarness(t, worktree, fmt.Sprintf("/bin/sleep %d", 600))
	ownerPID := staleLeaseOwnerPID()
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: ownerPID},
		},
	}
	scanner := realOrphanHarnessScanner(leaderPID)
	require.Equal(t, leaderPID, waitForOrphanHarnessProcess(t, scanner, leaderPID).PID)

	var emitted []map[string]any
	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{},
		func(_ string, fields map[string]any) { emitted = append(emitted, fields) },
		realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)

	require.Equal(t, []string{beadID}, store.released, "stale lease must be released")

	events := store.events[beadID]
	require.Len(t, events, 1)
	assert.Equal(t, "operator_attention", events[0].Kind)
	assert.Equal(t, "orphaned_harness_child", events[0].Summary)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(events[0].Body), &body))
	assert.Equal(t, beadID, body["bead_id"], "evidence must carry the bead id")
	assert.Equal(t, float64(ownerPID), body["claim_owner_pid"], "evidence must carry the owner PID status")
	diagnosis, _ := body["diagnosis"].(string)
	require.NotEmpty(t, diagnosis, "evidence must carry a diagnosis")
	assert.Contains(t, diagnosis, "is gone", "diagnosis must describe the owner PID status")

	require.NotEmpty(t, emitted, "operator-attention telemetry must be emitted")
	assert.Equal(t, "orphaned_harness_child", emitted[0]["reason"])
	assert.Equal(t, beadID, emitted[0]["bead_id"])
	var leaderState string
	require.Eventually(t, func() bool {
		leaderState = processDeadOrZombieStatus(leaderPID)
		return processDeadOrZombie(leaderPID)
	}, 3*time.Second, 20*time.Millisecond, "orphaned harness must be reaped (proc state=%s)", procStateSnapshot{&leaderState})
}
