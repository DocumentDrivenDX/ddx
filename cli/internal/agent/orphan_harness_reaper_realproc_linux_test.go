//go:build linux

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

// TestWorkStartupReaper_KillsWorkspaceScopedOrphanHarness plants a real harness
// process group whose recorded owner PID is gone and asserts the startup reaper
// kills the group within a bounded grace.
func TestWorkStartupReaper_KillsWorkspaceScopedOrphanHarness(t *testing.T) {
	projectRoot := t.TempDir()
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(ddxroot.JoinProject(projectRoot), 0o755))
	t.Setenv(config.ExecutionWorktreeRootEnv, tempRoot)

	beadID := "ddx-deadbeef"
	worktree := filepath.Join(tempRoot, ExecuteBeadWtPrefix+beadID+"-20260602T170011-deadbeef")
	require.NoError(t, os.MkdirAll(worktree, 0o755))

	grp := spawnRealOrphanGroup(t)
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: deadPID(t)},
		},
	}
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{{
			PID:     grp.leaderPID,
			PPID:    1,
			Command: "claude --print -p --output-format stream-json " + worktree,
			Cwd:     worktree,
		}},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []string{beadID}, store.released)

	grp.reapAsync()
	require.Eventually(t, func() bool { return !processAlive(grp.leaderPID) },
		3*time.Second, 20*time.Millisecond, "orphaned harness process group should be gone")
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

	grp := spawnRealOrphanGroupWithGrandchild(t)
	require.Greater(t, grp.grandchildPID, 0)
	require.True(t, processAlive(grp.leaderPID), "leader must start alive")
	require.True(t, processAlive(grp.grandchildPID), "grandchild must start alive")

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: deadPID(t)},
		},
	}
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{{
			PID:     grp.leaderPID,
			PPID:    1,
			Command: "codex exec --json -C " + worktree,
			Cwd:     worktree,
		}},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)

	grp.reapAsync()
	require.Eventually(t, func() bool {
		return !processAlive(grp.leaderPID) && !processAlive(grp.grandchildPID)
	}, 3*time.Second, 20*time.Millisecond, "both harness child and grandchild should be reaped")
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

	orphan := spawnRealOrphanGroup(t)
	live := spawnRealOrphanGroup(t)

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			orphanBead: {BeadID: orphanBead, PID: deadPID(t)},
			liveBead:   {BeadID: liveBead, PID: os.Getpid()},
		},
	}
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{
			{PID: orphan.leaderPID, PPID: 1, Command: "claude exec -C " + orphanWt, Cwd: orphanWt},
			{PID: live.leaderPID, PPID: 1, Command: "claude exec -C " + liveWt, Cwd: liveWt},
		},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	require.Equal(t, []string{orphanBead}, store.released)
	assert.Empty(t, store.events[liveBead], "live-owned harness must not be reaped")

	orphan.reapAsync()
	require.Eventually(t, func() bool { return !processAlive(orphan.leaderPID) },
		3*time.Second, 20*time.Millisecond, "orphan must be reaped")
	require.Never(t, func() bool { return !processAlive(live.leaderPID) },
		300*time.Millisecond, 30*time.Millisecond, "live-owned harness must stay alive")
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

	projProc := spawnRealOrphanGroup(t)
	otherProc := spawnRealOrphanGroup(t)

	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			projBead:  {BeadID: projBead, PID: deadPID(t)},
			otherBead: {BeadID: otherBead, PID: deadPID(t)},
		},
	}
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{
			{PID: projProc.leaderPID, PPID: 1, Command: "claude exec -C " + projWt, Cwd: projWt},
			{PID: otherProc.leaderPID, PPID: 1, Command: "codex exec -C " + otherWt, Cwd: otherWt},
		},
	}

	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{}, nil, realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped, "only the harness within this project's execution root should be reaped")
	require.Equal(t, []string{projBead}, store.released)
	assert.Empty(t, store.events[otherBead], "other-workspace harness must not be reaped")

	projProc.reapAsync()
	require.Eventually(t, func() bool { return !processAlive(projProc.leaderPID) },
		3*time.Second, 20*time.Millisecond, "this project's orphan must be reaped")
	require.Never(t, func() bool { return !processAlive(otherProc.leaderPID) },
		300*time.Millisecond, 30*time.Millisecond, "other-workspace harness must stay alive")
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

	grp := spawnRealOrphanGroup(t)
	ownerPID := deadPID(t)
	store := &fakeOrphanHarnessLeaseStore{
		leases: map[string]bead.ClaimLeaseRecord{
			beadID: {BeadID: beadID, PID: ownerPID},
		},
	}
	scanner := fakeOrphanHarnessScanner{
		processes: []orphanHarnessProcess{{
			PID:     grp.leaderPID,
			PPID:    1,
			Command: "claude --print -p " + worktree,
			Cwd:     worktree,
		}},
	}

	var emitted []map[string]any
	reaped, err := reapOrphanedHarnessChildren(
		context.Background(), projectRoot, scanner, store, store, store,
		"worker-a", &bytes.Buffer{},
		func(_ string, fields map[string]any) { emitted = append(emitted, fields) },
		realKillGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	grp.reapAsync()

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
}
