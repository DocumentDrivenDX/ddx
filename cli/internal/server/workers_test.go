package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerManagerStartAndShow(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	m.AgentRunnerFactory = func(projectRoot string) *agent.Runner {
		return agent.NewRunner(agent.Config{})
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Harness: "agent",
		Model:   "qwen/qwen3.6",
		Once:    true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, record.ID)
	assert.Equal(t, "running", record.State)
	assert.Equal(t, "agent", record.Harness)
	assert.Equal(t, "qwen/qwen3.6", record.Model)
	require.NotEmpty(t, record.SpecPath)

	// Wait for the worker to finish (it will fail quickly since there's no real agent)
	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)
	assert.Equal(t, "exited", final.State)
}

func TestWorkerManagerList(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)

	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	workers, err := m.List()
	require.NoError(t, err)
	require.Len(t, workers, 1)
	assert.Equal(t, record.ID, workers[0].ID)
}

func TestWorkerManagerStop(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)
	// Use a long poll interval so the worker stays running
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, m.Stop(record.ID))
	final := waitForWorkerExit(t, m, record.ID, 5*time.Second)
	// Cancelled worker: "exited" or "failed" depending on timing
	assert.NotEqual(t, "running", final.State)
}

func TestWorkerManagerLogs(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)

	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	stdout, stderr, err := m.Logs(record.ID)
	require.NoError(t, err)
	// Worker log should exist (even if empty for a quick failure)
	_ = stdout
	_ = stderr
}

func TestWorkerManagerWritesStatusToDisk(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		Harness: "agent",
		Once:    true,
	})
	require.NoError(t, err)

	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	// Check that status.json was written to disk
	dir := filepath.Join(root, ".ddx", "workers", record.ID)
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), record.ID)
}

func waitForWorkerExit(t *testing.T, m *WorkerManager, id string, timeout time.Duration) WorkerRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		record, err := m.Show(id)
		require.NoError(t, err)
		if !record.FinishedAt.IsZero() {
			return record
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("worker %s did not finish in time", id)
	return WorkerRecord{}
}

// setupBeadStore creates a minimal .ddx/beads.jsonl in the test dir
// so the worker can initialize the bead store without errors.
func setupBeadStore(t *testing.T, root string) {
	t.Helper()
	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	// Write empty but valid JSONL
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte(""), 0o644))
}

// TestWorkerManagerCancelledContext verifies that cancelling the context stops the worker.
func TestWorkerManagerCancelledContext(t *testing.T) {
	root := t.TempDir()
	setupBeadStore(t, root)

	m := NewWorkerManager(root)

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		PollInterval: 30 * time.Second, // long poll to keep it alive
	})
	require.NoError(t, err)

	// Verify it's running
	shown, err := m.Show(record.ID)
	require.NoError(t, err)
	assert.True(t, shown.FinishedAt.IsZero(), "worker should still be running")

	// Stop it
	require.NoError(t, m.Stop(record.ID))
	final := waitForWorkerExit(t, m, record.ID, 5*time.Second)
	assert.NotEqual(t, "running", final.State)
}

// TODO: integration test for execute-bead via worker manager needs
// a proper git repo + mock agent runner. The unit tests above cover
// the worker lifecycle (start, stop, list, show, logs, status on disk).

func setupBeadStoreWithReadyBead(t *testing.T, root string) {
	t.Helper()
	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	store := bead.NewStore(ddxDir)
	err := store.Create(&bead.Bead{
		ID:         "ddx-testbead",
		Title:      "Test bead",
		Status:     bead.StatusOpen,
		Priority:   0,
		IssueType:  bead.DefaultType,
		Acceptance: "Just a test",
	})
	require.NoError(t, err)

	// Initialize the git repo so execute-bead can find HEAD
	initGitRepo(t, root)
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "-c", "user.name=Test", "-c", "user.email=test@test.com", "commit", "-m", "init")
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v: %s", name, args, string(out))
}

func TestFormatSessionLogLines(t *testing.T) {
	lines := []string{
		`{"type":"session.start","data":{"model":"qwen/qwen3.6-plus"}}`,
		`{"type":"llm.request","data":{"attempt_index":1,"messages":[{"role":"user","content":"find .rs files"}]}}`,
		`{"type":"llm.response","data":{"model":"qwen/qwen3.6-plus-04-02","latency_ms":5491,"attempt":{"cost":{"raw":{"total_tokens":8408,"prompt_tokens":8204,"completion_tokens":204}}},"tool_calls":[{"name":"read","arguments":{"path":"docs/FEAT-006.md"}}],"finish_reason":"tool_calls"}}`,
		`{"type":"tool.call","data":{"tool":"read","input":{"path":"docs/FEAT-006.md"},"duration_ms":120,"error":""}}`,
		`{"type":"tool.call","data":{"tool":"write","input":{"path":"docs/new.md"},"duration_ms":50,"error":"permission denied"}}`,
		`{"type":"compaction.start","data":{}}`,
		`{"type":"compaction.end","data":{}}`,
		`{"type":"compaction.start","data":{}}`,
		`{"type":"compaction.end","data":{"success":true,"tokens_before":10000,"tokens_after":3000}}`,
		`{"type":"llm.delta","data":{}}`,
	}

	result := agent.FormatSessionLogLines(lines)

	assert.Contains(t, result, "session started (model: qwen/qwen3.6-plus)")
	assert.Contains(t, result, "→ llm request (attempt 1) [find .rs files]")
	assert.Contains(t, result, "← llm response (8408 tokens, 5.5s) qwen/qwen3.6-plus-04-02 → read")
	assert.Contains(t, result, "🔧 read docs/FEAT-006.md (0.1s)")
	assert.Contains(t, result, "🔧 write docs/new.md (0.1s) ❌ permission denied")
	assert.NotContains(t, result, "compacting context...") // no-op compactions are suppressed
	assert.Contains(t, result, "⚡ compacted context (10000 → 3000 tokens)")
	assert.NotContains(t, result, "llm.delta") // deltas should be suppressed
}
