//go:build !windows

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestScript(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o755))
}

func writeSleepingWorkerScript(t *testing.T, path string) {
	t.Helper()
	writeTestScript(t, path, `
sleep 60 &
wait
`)
}

func waitForProcessGroupEmpty(t *testing.T, pgid int) {
	t.Helper()
	require.NotZero(t, pgid)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := scanWorkerProcesses(context.Background())
		require.NoError(t, err)
		found := false
		for _, row := range rows {
			if row.PGID == pgid {
				found = true
				break
			}
		}
		if !found {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process group %d still has members", pgid)
}

func waitForPIDFileRows(t *testing.T, path string, rows int) []int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= rows*2 {
				out := make([]int, 0, len(fields))
				for _, field := range fields {
					pid, err := strconv.Atoi(field)
					require.NoError(t, err)
					out = append(out, pid)
				}
				return out
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d pid rows in %s", rows, path)
	return nil
}

func writeProviderTreeScripts(t *testing.T, binDir, pidFile string) string {
	t.Helper()
	providerBody := `
sleep 60 &
echo "$$ $!" >> "$PID_FILE"
wait
`
	writeTestScript(t, filepath.Join(binDir, "claude"), providerBody)
	writeTestScript(t, filepath.Join(binDir, "codex"), providerBody)
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeTestScript(t, workerPath, `
"$FAKE_BIN/claude" &
"$FAKE_BIN/codex" &
wait
`)
	t.Setenv("FAKE_BIN", binDir)
	t.Setenv("PID_FILE", pidFile)
	return workerPath
}

func TestServerManagedWorkerEnvExtendsSparseServicePath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	exe := filepath.Join(home, ".local", "bin", "ddx")
	systemBin := filepath.Join(t.TempDir(), "system-bin")
	t.Setenv("HOME", home)
	t.Setenv("PATH", systemBin+string(os.PathListSeparator)+filepath.Join(home, ".local", "bin"))

	env := serverManagedWorkerEnv(exe)
	var path string
	pathEntries := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			pathEntries++
			path = strings.TrimPrefix(kv, "PATH=")
		}
	}
	require.Equal(t, 1, pathEntries)

	parts := filepath.SplitList(path)
	expected := []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".local", "share", "mise", "shims"),
		filepath.Join(home, "bin"),
		"/home/linuxbrew/.linuxbrew/bin",
		"/home/linuxbrew/.linuxbrew/sbin",
		systemBin,
	}
	for _, want := range expected {
		assert.Contains(t, parts, want)
	}
	assert.Equal(t, filepath.Join(home, ".local", "bin"), parts[0])
	assert.Equal(t, systemBin, parts[5])
	assert.Equal(t, 1, countPathEntry(parts, filepath.Join(home, ".local", "bin")))
}

func countPathEntry(values []string, needle string) int {
	count := 0
	for _, value := range values {
		if value == needle {
			count++
		}
	}
	return count
}

func TestManagedWorkerRecordsProcessGroup(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	binDir := t.TempDir()
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeSleepingWorkerScript(t, workerPath)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogKillGrace = 100 * time.Millisecond
	defer m.StopAll()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	require.NotZero(t, rec.PID)
	require.NotZero(t, rec.PGID)

	stored, err := m.readRecord(filepath.Join(m.rootDir, rec.ID))
	require.NoError(t, err)
	assert.Equal(t, rec.PID, stored.PID)
	assert.Equal(t, rec.PGID, stored.PGID)
	assert.Equal(t, rec.PID, rec.PGID)

	require.NoError(t, m.Stop(rec.ID))
	waitForProcessGroupEmpty(t, rec.PGID)
}

func TestManagedWorkerStopKillsClaudeCodexDescendants(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	binDir := t.TempDir()
	pidFile := filepath.Join(root, "provider-pids.txt")
	workerPath := writeProviderTreeScripts(t, binDir, pidFile)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogKillGrace = 100 * time.Millisecond
	defer m.StopAll()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	_ = waitForPIDFileRows(t, pidFile, 2)

	require.NoError(t, m.Stop(rec.ID))
	waitForProcessGroupEmpty(t, rec.PGID)
}

func TestManagedWorkerWatchdogReapKillsProcessTree(t *testing.T) {
	root := t.TempDir()
	store := seedClaimedBead(t, root, "ddx-watchdog-tree")

	binDir := t.TempDir()
	pidFile := filepath.Join(root, "provider-pids.txt")
	workerPath := writeProviderTreeScripts(t, binDir, pidFile)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogDeadline = time.Millisecond
	m.StallDeadline = time.Millisecond
	m.WatchdogKillGrace = 100 * time.Millisecond
	defer m.StopAll()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	_ = waitForPIDFileRows(t, pidFile, 2)

	now := time.Now().UTC()
	m.mu.Lock()
	h := m.workers[rec.ID]
	require.NotNil(t, h)
	h.record.StartedAt = now.Add(-time.Second)
	h.record.CurrentBead = "ddx-watchdog-tree"
	h.record.CurrentAttempt = &CurrentAttemptInfo{
		AttemptID: rec.ID + "-attempt",
		BeadID:    "ddx-watchdog-tree",
		Phase:     "running",
		StartedAt: now.Add(-time.Second),
	}
	h.lastPhaseTS = now.Add(-time.Second)
	m.mu.Unlock()

	m.watchdogSweep(now)
	waitForProcessGroupEmpty(t, rec.PGID)

	final := waitForWorkerExit(t, m, rec.ID, 5*time.Second)
	assert.Equal(t, "reaped", final.State)

	b, err := store.Get(context.Background(), "ddx-watchdog-tree")
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, b.Status)
}

func TestWorkerManagerShutdownStopsManagedProcessTrees(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	binDir := t.TempDir()
	pidFile := filepath.Join(root, "provider-pids.txt")
	workerPath := writeProviderTreeScripts(t, binDir, pidFile)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogKillGrace = 100 * time.Millisecond

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	_ = waitForPIDFileRows(t, pidFile, 2)

	m.StopAll()
	waitForProcessGroupEmpty(t, rec.PGID)
}

func TestManagedWorkerStopDoesNotKillUnrelatedProcessGroup(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	cmdPID := 0
	done := make(chan struct{})
	pidFile := filepath.Join(root, "unrelated.pid")
	writeTestScript(t, filepath.Join(root, "unrelated.sh"), `
echo "$$" > "$PID_FILE"
sleep 60 &
wait
`)
	t.Setenv("PID_FILE", pidFile)
	cmd := exec.Command(filepath.Join(root, "unrelated.sh"))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	proc := cmd.Process
	cmdPID = proc.Pid
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		if cmdPID > 0 {
			_ = syscall.Kill(-cmdPID, syscall.SIGKILL)
		}
		<-done
	})

	binDir := t.TempDir()
	workerPath := filepath.Join(binDir, "ddx-worker")
	writeSleepingWorkerScript(t, workerPath)

	m := NewWorkerManager(root)
	m.workerBinaryPath = workerPath
	m.WatchdogKillGrace = 100 * time.Millisecond
	defer m.StopAll()

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Mode:         "watch",
		IdleInterval: executeLoopIdleInterval(time.Second),
	})
	require.NoError(t, err)
	require.NoError(t, m.Stop(rec.ID))
	require.NoError(t, m.Stop(rec.ID))
	waitForProcessGroupEmpty(t, rec.PGID)

	assert.NoError(t, syscall.Kill(cmdPID, 0), "unrelated process group must remain alive")
}
