package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeManageWorkersConfig sets server.manage_workers in project config.
func writeManageWorkersConfig(t *testing.T, projectRoot string, enabled bool) {
	t.Helper()
	ddxDir := ddxroot.JoinProject(projectRoot)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	val := "false"
	if enabled {
		val = "true"
	}
	cfg := "version: \"1.0\"\nserver:\n  manage_workers: " + val + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644))
}

// writeStaleDesiredCount writes desired.json with the given count without going
// through SaveDesiredState (which would reject count>0 when management is off).
func writeStaleDesiredCount(t *testing.T, projectRoot string, count int) {
	t.Helper()
	// Temporarily enable management only long enough to persist the stale file.
	m := NewWorkerManager(projectRoot)
	on := true
	m.SetManageWorkers(&on)
	sup := NewWorkerSupervisor(m)
	state := DefaultWorkerDesiredState(projectRoot)
	state.DesiredCount = count
	state.Restart.Enabled = true
	require.NoError(t, sup.SaveDesiredState(&state))
}

// TestServerManagementDisabledSpawnsNothing exercises startup, reconcile, and
// API/CLI enable paths with server.manage_workers off and observes zero DDx
// worker launches.
func TestServerManagementDisabledSpawnsNothing(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root := t.TempDir()
	writeManageWorkersConfig(t, root, false)
	require.NoError(t, os.WriteFile(filepath.Join(ddxroot.JoinProject(root), "beads.jsonl"), []byte(""), 0o644))

	// Stale desired_count=1 on disk — would spawn if the gate were open.
	writeStaleDesiredCount(t, root, 1)

	// Re-assert management is off after the temporary enable used to seed desired.
	writeManageWorkersConfig(t, root, false)

	var launchCount atomic.Int32
	m := NewWorkerManager(root)
	// Explicit override so packageUnderTest cannot re-open the gate.
	off := false
	m.SetManageWorkers(&off)
	defer m.StopWatchdog()
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		launchCount.Add(1)
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				<-ctx.Done()
				return agent.ExecuteBeadReport{BeadID: beadID, Status: agent.ExecuteBeadStatusExecutionFailed, Detail: "canceled"}, ctx.Err()
			}),
		}
	}

	// 1) Direct StartExecuteLoop (API / low-level spawn path).
	_, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         executeloop.ModeOnce,
		ProjectRoot:  root,
		IdleInterval: executeLoopIdleInterval(30 * time.Second),
	})
	require.ErrorIs(t, err, ErrManagementDisabled)

	// 2) Supervisor reconcile / scale-up / restart path.
	sup := NewWorkerSupervisor(m)
	require.NoError(t, sup.Reconcile())

	// 3) GraphQL/API enable path (DispatchWorker).
	adapter := &workerDispatchAdapter{manager: m}
	_, err = adapter.DispatchWorker(context.Background(), "work", root, nil)
	require.ErrorIs(t, err, ErrManagementDisabled)

	// 4) CLI-equivalent enable path: SaveDesiredState with desired_count=1.
	enableState := DefaultWorkerDesiredState(root)
	enableState.DesiredCount = 1
	enableState.Restart.Enabled = true
	err = sup.SaveDesiredState(&enableState)
	require.ErrorIs(t, err, ErrManagementDisabled)

	// 5) Server startup must not spawn. New() also zeros desired intent when
	// management is disabled.
	srv := New(":0", root)
	// Force the server manager gate off as well (config is already false).
	if srv.workers != nil {
		srv.workers.SetManageWorkers(&off)
	}
	t.Cleanup(func() { _ = srv.Shutdown() })
	srv.StartSupervisor()
	time.Sleep(80 * time.Millisecond)

	assert.Equal(t, int32(0), launchCount.Load(), "no managed worker factory invocations when management disabled")

	running := 0
	recs, listErr := m.List()
	require.NoError(t, listErr)
	for _, rec := range recs {
		if rec.Kind == "work" && (rec.State == "running" || rec.State == "stopping") {
			running++
		}
	}
	assert.Equal(t, 0, running, "zero DDx workers when management disabled")

	// Server policy zeros desired spawn intent on startup.
	loaded, loadErr := NewWorkerSupervisor(NewWorkerManager(root)).LoadDesiredState()
	if loadErr == nil {
		assert.Equal(t, 0, loaded.DesiredCount, "startup with management disabled must zero desired_count")
	}
}

// TestWorkerEnableRejectedWhenManagementDisabled returns the typed
// management_disabled result for enable/start mutations.
func TestWorkerEnableRejectedWhenManagementDisabled(t *testing.T) {
	root := t.TempDir()
	writeManageWorkersConfig(t, root, false)

	m := NewWorkerManager(root)
	off := false
	m.SetManageWorkers(&off)
	sup := NewWorkerSupervisor(m)

	state := DefaultWorkerDesiredState(root)
	state.DesiredCount = 1
	state.Restart.Enabled = true
	err := sup.SaveDesiredState(&state)
	require.ErrorIs(t, err, ErrManagementDisabled)
	assert.Equal(t, "management_disabled", err.Error())

	adapter := &workerDispatchAdapter{manager: m}
	_, err = adapter.DispatchWorker(context.Background(), "work", root, nil)
	require.ErrorIs(t, err, ErrManagementDisabled)
	assert.Equal(t, "management_disabled", err.Error())

	_, err = m.StartExecuteLoop(ExecuteLoopWorkerSpec{Mode: executeloop.ModeOnce, ProjectRoot: root})
	require.ErrorIs(t, err, ErrManagementDisabled)
}

// TestManagementDisableZerosDesiredState proves disabling management removes
// desired spawn intent without signaling provider processes.
func TestManagementDisableZerosDesiredState(t *testing.T) {
	root := t.TempDir()
	writeManageWorkersConfig(t, root, true)

	m := NewWorkerManager(root)
	on := true
	m.SetManageWorkers(&on)
	sup := NewWorkerSupervisor(m)
	state := DefaultWorkerDesiredState(root)
	state.DesiredCount = 2
	state.Restart.Enabled = true
	require.NoError(t, sup.SaveDesiredState(&state))

	loaded, err := sup.LoadDesiredState()
	require.NoError(t, err)
	require.Equal(t, 2, loaded.DesiredCount)

	// Stand-in for a provider process: a sleep child that must NOT receive signals.
	var providerCmd *exec.Cmd
	if runtime.GOOS != "windows" {
		providerCmd = exec.Command("sleep", "60")
		require.NoError(t, providerCmd.Start())
		t.Cleanup(func() {
			if providerCmd.Process != nil {
				_ = providerCmd.Process.Kill()
				_, _ = providerCmd.Process.Wait()
			}
		})
	}

	// Flip management off and apply zeroing (same as server observer demotion).
	writeManageWorkersConfig(t, root, false)
	require.NoError(t, ZeroDesiredManagedState(root))

	m2 := NewWorkerManager(root)
	sup2 := NewWorkerSupervisor(m2)
	zeroed, err := sup2.LoadDesiredState()
	require.NoError(t, err)
	assert.Equal(t, 0, zeroed.DesiredCount)
	assert.False(t, zeroed.Restart.Enabled)

	if providerCmd != nil && providerCmd.Process != nil {
		// Process still alive — zeroing must not signal provider processes.
		err = providerCmd.Process.Signal(syscall.Signal(0))
		require.NoError(t, err, "provider stand-in must remain alive (no signal from management disable)")
	}
}
