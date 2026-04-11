package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerManagerStartListShowLogs(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	m.build = func(spec ExecuteLoopWorkerSpec) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=TestWorkerManagerHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_STDOUT=worker stdout",
			"HELPER_STDERR=worker stderr",
			"HELPER_SLEEP_MS=25",
			"HELPER_EXIT_CODE=0",
		)
		return cmd, nil
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Harness: "codex", Once: true})
	require.NoError(t, err)
	require.NotEmpty(t, record.ID)

	final := waitForWorkerExit(t, m, record.ID)
	assert.Equal(t, "exited", final.State)
	require.NotNil(t, final.ExitCode)
	assert.Equal(t, 0, *final.ExitCode)

	workers, err := m.List()
	require.NoError(t, err)
	require.Len(t, workers, 1)
	assert.Equal(t, record.ID, workers[0].ID)

	shown, err := m.Show(record.ID)
	require.NoError(t, err)
	assert.Equal(t, record.ID, shown.ID)

	stdout, stderr, err := m.Logs(record.ID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "worker stdout")
	assert.Contains(t, stderr, "worker stderr")
}

func TestWorkerManagerStop(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	m.build = func(spec ExecuteLoopWorkerSpec) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=TestWorkerManagerHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"HELPER_STDOUT=still running",
			"HELPER_SLEEP_MS=5000",
			"HELPER_EXIT_CODE=0",
		)
		return cmd, nil
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Harness: "agent", PollInterval: 30 * time.Second})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))
	final := waitForWorkerExit(t, m, record.ID)
	assert.Equal(t, "exited", final.State)
	require.NotNil(t, final.ExitCode)
	assert.NotEqual(t, 0, *final.ExitCode)
}

func waitForWorkerExit(t *testing.T, m *WorkerManager, id string) WorkerRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, err := m.Show(id)
		require.NoError(t, err)
		if !record.FinishedAt.IsZero() {
			return record
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("worker %s did not finish in time", id)
	return WorkerRecord{}
}

func TestWorkerManagerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if v := os.Getenv("HELPER_STDOUT"); v != "" {
		_, _ = os.Stdout.WriteString(v + "\n")
	}
	if v := os.Getenv("HELPER_STDERR"); v != "" {
		_, _ = os.Stderr.WriteString(v + "\n")
	}
	if v := os.Getenv("HELPER_SLEEP_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
	code := 0
	if v := os.Getenv("HELPER_EXIT_CODE"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			code = parsed
		}
	}
	// Ensure logs have a stable cwd-visible path if needed while debugging.
	_ = os.MkdirAll(filepath.Join(".", ".ddx"), 0o755)
	os.Exit(code)
}
