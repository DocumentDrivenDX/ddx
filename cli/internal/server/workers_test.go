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
	require.NotEmpty(t, record.SpecPath)
	_, err = os.Stat(filepath.Join(root, record.SpecPath))
	require.NoError(t, err)

	final := waitForWorkerExit(t, m, record.ID)
	assert.Equal(t, "exited", final.State)
	assert.Equal(t, "exited", final.Status)
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

func TestWorkerManagerEnrichesExecuteLoopSummary(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	dir := filepath.Join(root, ".ddx", "workers", "worker-test")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	record := WorkerRecord{
		ID:          "worker-test",
		Kind:        "execute-loop",
		State:       "exited",
		ProjectRoot: root,
		StdoutPath:  relToProject(root, filepath.Join(dir, "stdout.log")),
		StderrPath:  relToProject(root, filepath.Join(dir, "stderr.log")),
	}
	require.NoError(t, m.writeRecord(dir, record))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stdout.log"), []byte(`{
  "project_root": "/tmp/project",
  "attempts": 1,
  "successes": 0,
  "failures": 1,
  "last_failure_status": "execution_failed",
  "results": [
    {
      "bead_id": "ddx-1234abcd",
      "attempt_id": "20260411T000000-abcd1234",
      "worker_id": "worker-test",
      "harness": "agent",
      "model": "qwen/qwen3.6-plus",
      "status": "execution_failed",
      "detail": "cancelled",
      "session_id": "eb-123",
      "base_rev": "abc",
      "result_rev": "def",
      "retry_after": "2026-04-11T10:00:00Z"
    }
  ]
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stderr.log"), nil, 0o644))

	shown, err := m.Show("worker-test")
	require.NoError(t, err)
	assert.Equal(t, "execution_failed", shown.Status)
	assert.Equal(t, 1, shown.Attempts)
	assert.Equal(t, 1, shown.Failures)
	assert.Equal(t, "ddx-1234abcd", shown.CurrentBead)
	assert.Equal(t, "20260411T000000-abcd1234", shown.CurrentAttempt)
	assert.Equal(t, "agent", shown.Harness)
	assert.Equal(t, "qwen/qwen3.6-plus", shown.Model)
	require.NotNil(t, shown.LastResult)
	assert.Equal(t, "ddx-1234abcd", shown.LastResult.BeadID)
	assert.Equal(t, "20260411T000000-abcd1234", shown.LastResult.AttemptID)
	assert.Equal(t, "agent", shown.LastResult.Harness)
	assert.Equal(t, "qwen/qwen3.6-plus", shown.LastResult.Model)
	assert.Equal(t, "execution_failed", shown.LastResult.Status)
	assert.Equal(t, "cancelled", shown.LastError)
}

func TestWorkerManagerEnrichesRunningWorkerFromLiveArtifacts(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)
	dir := filepath.Join(root, ".ddx", "workers", "worker-test")
	execDir := filepath.Join(root, ".ddx", "executions", "20260411T000000-abcd1234")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(execDir, 0o755))

	record := WorkerRecord{
		ID:          "worker-test",
		Kind:        "execute-loop",
		State:       "running",
		Status:      "running",
		ProjectRoot: root,
		StdoutPath:  relToProject(root, filepath.Join(dir, "stdout.log")),
		StderrPath:  relToProject(root, filepath.Join(dir, "stderr.log")),
		SpecPath:    relToProject(root, filepath.Join(dir, "spec.json")),
	}
	require.NoError(t, m.writeRecord(dir, record))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stdout.log"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stderr.log"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "manifest.json"), []byte(`{
  "attempt_id": "20260411T000000-abcd1234",
  "worker_id": "worker-test",
  "bead_id": "ddx-livebead",
  "base_rev": "abc123"
}
`), 0o644))

	shown, err := m.Show("worker-test")
	require.NoError(t, err)
	assert.Equal(t, "ddx-livebead", shown.CurrentBead)
	assert.Equal(t, "20260411T000000-abcd1234", shown.CurrentAttempt)
	assert.Equal(t, "running", shown.Status)
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
