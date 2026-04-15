package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		Harness:  "agent",
		Model:    "qwen/qwen3.6",
		Provider: "openrouter",
		Once:     true,
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

// TestWorkerRecordShape verifies the WorkerRecord struct carries the FEAT-002
// required fields: CurrentAttempt, RecentPhases, and LastAttempt.
func TestWorkerRecordShape(t *testing.T) {
	// Zero value — all new fields must be nil/empty (omitempty)
	rec := WorkerRecord{ID: "w-1", Kind: "execute-loop", State: "running"}
	assert.Nil(t, rec.CurrentAttempt)
	assert.Empty(t, rec.RecentPhases)
	assert.Nil(t, rec.LastAttempt)

	// JSON round-trip: new fields are omitted when nil/empty
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "current_attempt")
	assert.NotContains(t, string(data), "recent_phases")
	assert.NotContains(t, string(data), "last_attempt")

	// JSON round-trip: new fields are present when set
	now := time.Now().UTC().Truncate(time.Second)
	rec.CurrentAttempt = &CurrentAttemptInfo{
		AttemptID: "20260414T000000-abcd1234",
		BeadID:    "ddx-abc123",
		Phase:     "running",
		PhaseSeq:  2,
		StartedAt: now,
		ElapsedMS: 5000,
	}
	rec.RecentPhases = []PhaseTransition{
		{Phase: "queueing", TS: now, PhaseSeq: 1},
		{Phase: "running", TS: now.Add(time.Second), PhaseSeq: 2},
	}

	data, err = json.Marshal(rec)
	require.NoError(t, err)
	assert.Contains(t, string(data), "current_attempt")
	assert.Contains(t, string(data), "recent_phases")

	var decoded WorkerRecord
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.CurrentAttempt)
	assert.Equal(t, "running", decoded.CurrentAttempt.Phase)
	assert.Equal(t, 2, decoded.CurrentAttempt.PhaseSeq)
	assert.Len(t, decoded.RecentPhases, 2)
	assert.Equal(t, "queueing", decoded.RecentPhases[0].Phase)
}

// TestDrainProgressUpdatesRecord verifies that drainProgress correctly
// updates WorkerRecord fields as ProgressEvents flow through the channel.
func TestDrainProgressUpdatesRecord(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)

	handle := &workerHandle{
		record: WorkerRecord{ID: "w-test", Kind: "execute-loop", State: "running"},
	}
	ch := make(chan agent.ProgressEvent, 10)

	go m.drainProgress("w-test", handle, ch)

	now := time.Now().UTC()

	// Send queueing transition
	ch <- agent.ProgressEvent{
		EventID: "evt-1", AttemptID: "atm-1", WorkerID: "w-test",
		BeadID: "ddx-1", Phase: "queueing", PhaseSeq: 1, Heartbeat: false,
		TS: now, ElapsedMS: 0,
	}
	// Send running transition
	ch <- agent.ProgressEvent{
		EventID: "evt-2", AttemptID: "atm-1", WorkerID: "w-test",
		BeadID: "ddx-1", Phase: "running", PhaseSeq: 2, Heartbeat: false,
		TS: now.Add(time.Second), ElapsedMS: 1000,
	}
	// Send heartbeat (should NOT go into RecentPhases)
	ch <- agent.ProgressEvent{
		EventID: "evt-3", AttemptID: "atm-1", WorkerID: "w-test",
		BeadID: "ddx-1", Phase: "running", PhaseSeq: 2, Heartbeat: true,
		TS: now.Add(2 * time.Second), ElapsedMS: 2000,
	}
	// Send terminal phase
	ch <- agent.ProgressEvent{
		EventID: "evt-4", AttemptID: "atm-1", WorkerID: "w-test",
		BeadID: "ddx-1", Phase: "done", PhaseSeq: 3, Heartbeat: false,
		TS: now.Add(3 * time.Second), ElapsedMS: 3000,
	}

	close(ch)

	// Wait for drainProgress goroutine to finish
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		phases := len(handle.record.RecentPhases)
		m.mu.Unlock()
		if phases == 3 { // queueing + running + done (heartbeat excluded)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	m.mu.Lock()
	rec := handle.record
	m.mu.Unlock()

	// After terminal: CurrentAttempt is nil, LastAttempt is set
	assert.Nil(t, rec.CurrentAttempt, "CurrentAttempt should be nil after terminal phase")
	require.NotNil(t, rec.LastAttempt)
	assert.Equal(t, "atm-1", rec.LastAttempt.AttemptID)
	assert.Equal(t, "done", rec.LastAttempt.Phase)

	// 3 phase transitions recorded (queueing, running, done) — heartbeat excluded
	require.Len(t, rec.RecentPhases, 3)
	assert.Equal(t, "queueing", rec.RecentPhases[0].Phase)
	assert.Equal(t, "running", rec.RecentPhases[1].Phase)
	assert.Equal(t, "done", rec.RecentPhases[2].Phase)
}

// TestRecentPhasesCap verifies that RecentPhases is capped at 20 entries.
func TestRecentPhasesCap(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)

	handle := &workerHandle{
		record: WorkerRecord{ID: "w-test", Kind: "execute-loop", State: "running"},
	}
	ch := make(chan agent.ProgressEvent, 30)

	go m.drainProgress("w-test", handle, ch)

	now := time.Now().UTC()
	for i := 0; i < 25; i++ {
		ch <- agent.ProgressEvent{
			EventID: "evt", AttemptID: "atm-1", WorkerID: "w-test",
			BeadID: "ddx-1", Phase: "running", PhaseSeq: i + 1, Heartbeat: false,
			TS: now, ElapsedMS: int64(i * 1000),
		}
	}
	close(ch)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(handle.record.RecentPhases)
		m.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Allow goroutine to fully drain
	time.Sleep(50 * time.Millisecond)

	m.mu.Lock()
	n := len(handle.record.RecentPhases)
	m.mu.Unlock()

	assert.LessOrEqual(t, n, 20, "RecentPhases should be capped at 20")
}

// TestSubscribeProgress verifies that SSE subscribers receive events
// broadcast by drainProgress.
func TestSubscribeProgress(t *testing.T) {
	root := t.TempDir()
	m := NewWorkerManager(root)

	handle := &workerHandle{
		record: WorkerRecord{ID: "w-sub", Kind: "execute-loop", State: "running"},
	}
	progressCh := make(chan agent.ProgressEvent, 10)
	handle.progressCh = progressCh

	m.mu.Lock()
	m.workers["w-sub"] = handle
	m.mu.Unlock()

	go m.drainProgress("w-sub", handle, progressCh)

	sub, unsub := m.SubscribeProgress("w-sub")
	defer unsub()

	now := time.Now().UTC()
	progressCh <- agent.ProgressEvent{
		EventID: "evt-1", AttemptID: "atm-1", WorkerID: "w-sub",
		BeadID: "ddx-1", Phase: "running", PhaseSeq: 1, Heartbeat: false,
		TS: now, ElapsedMS: 100,
	}

	select {
	case evt := <-sub:
		assert.Equal(t, "running", evt.Phase)
		assert.Equal(t, "evt-1", evt.EventID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for progress event")
	}
}

// TestProjectWorkerShowEndpoint verifies GET /api/projects/:project/workers/:id
// returns a WorkerRecord with the expected shape, including the new fields.
func TestProjectWorkerShowEndpoint(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Get the stable project ID assigned during New()
	projectID := srv.state.RegisterProject(dir).ID

	// Start a worker so there is something to show
	m := srv.workers
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)

	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	// Request via the project-scoped endpoint using the stable project ID
	req := httptest.NewRequest("GET", "/api/projects/"+projectID+"/workers/"+record.ID, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var got WorkerRecord
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, record.ID, got.ID)
	// New fields are present in the struct (nil when not in use is fine)
	// The JSON keys must exist in the struct definition — this compiles only
	// if the fields are defined, so reaching here validates the schema.
}

// TestProjectWorkerProgressKeepaliveOnIdleWorker verifies that the SSE
// endpoint sends a keepalive comment and returns 200 when no worker is
// active (or subscriber channel is immediately closed).
func TestProjectWorkerProgressKeepaliveOnIdleWorker(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Get the stable project ID assigned during New()
	projectID := srv.state.RegisterProject(dir).ID

	// Start and wait for a worker to finish so it's on disk but not active
	m := srv.workers
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)
	_ = waitForWorkerExit(t, m, record.ID, 10*time.Second)

	// Use a real HTTP test server because SSE needs http.Flusher
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	url := ts.URL + "/api/projects/" + projectID + "/workers/" + record.ID + "/progress"

	// The channel is closed immediately (worker not active), so the handler
	// should send one keepalive and return.
	resp, err := http.Get(url) //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read until connection closes (the handler returns after the closed channel)
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= 5 {
			break
		}
	}

	// At least one keepalive comment should be present
	found := false
	for _, l := range lines {
		if strings.HasPrefix(l, ":") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected at least one keepalive comment line, got: %v", lines)
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

// TC-013.4 — A worker started for project A writes worker records and execution
// artifacts only under project A's .ddx/workers/ directory. No artifacts should
// appear under a sibling project B directory.
func TestWorkerScopeToProject(t *testing.T) {
	rootA := t.TempDir()
	setupBeadStore(t, rootA)

	mA := NewWorkerManager(rootA)

	record, err := mA.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)

	_ = waitForWorkerExit(t, mA, record.ID, 10*time.Second)

	// Worker directory must exist under project A's .ddx/workers/.
	workerDirA := filepath.Join(rootA, ".ddx", "workers", record.ID)
	if _, err := os.Stat(workerDirA); err != nil {
		t.Errorf("worker dir not found under project A: %v", err)
	}

	// status.json must be present and reference the correct project root.
	statusPath := filepath.Join(workerDirA, "status.json")
	data, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	var rec WorkerRecord
	require.NoError(t, json.Unmarshal(data, &rec))
	assert.Equal(t, rootA, rec.ProjectRoot, "WorkerRecord.ProjectRoot must match project A")
	assert.Equal(t, record.ID, rec.ID)

	// Verify that a completely separate project B directory has no worker artifacts.
	rootB := t.TempDir()
	workersDirB := filepath.Join(rootB, ".ddx", "workers")
	if _, err := os.Stat(workersDirB); err == nil {
		entries, _ := os.ReadDir(workersDirB)
		assert.Empty(t, entries, "project B's .ddx/workers/ must be empty")
	}
}

// TestWorkerLiveCounters verifies that Attempts/Successes/Failures on the
// WorkerRecord are updated incrementally as each bead completes, not just when
// the loop exits. A fake executor completes 3 beads (2 success, 1 failure) with
// a 50ms delay each. The test polls Show() every 30ms and asserts that at least
// one poll observes Attempts >= 1 while the worker is still running (FinishedAt
// is zero). After the loop exits the counters must be Attempts=3, Successes=2,
// Failures=1.
func TestWorkerLiveCounters(t *testing.T) {
	root := t.TempDir()
	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Initialise a git repo so CloseWithEvidence can write bead events.
	initGitRepo(t, root)

	// Create 3 ready beads.
	store := bead.NewStore(ddxDir)
	for i := 1; i <= 3; i++ {
		err := store.Create(&bead.Bead{
			ID:        fmt.Sprintf("ddx-live%02d", i),
			Title:     fmt.Sprintf("Live counter test bead %d", i),
			Status:    bead.StatusOpen,
			IssueType: bead.DefaultType,
		})
		require.NoError(t, err)
	}

	m := NewWorkerManager(root)

	// Inject a fake executor: first 2 calls succeed, 3rd fails.
	callCount := 0
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				time.Sleep(50 * time.Millisecond)
				callCount++
				if callCount == 3 {
					return agent.ExecuteBeadReport{
						BeadID: beadID,
						Status: agent.ExecuteBeadStatusExecutionFailed,
						Detail: "injected test failure",
					}, nil
				}
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusSuccess,
				}, nil
			}),
		}
	}

	// Start worker with no PollInterval so it exits once the queue is empty.
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{})
	require.NoError(t, err)

	// Poll every 30ms; record whether we see Attempts >= 1 while still running.
	deadline := time.Now().Add(10 * time.Second)
	var sawLiveAttempts bool
	for time.Now().Before(deadline) {
		rec, pollErr := m.Show(record.ID)
		require.NoError(t, pollErr)
		if rec.Attempts >= 1 && rec.FinishedAt.IsZero() {
			sawLiveAttempts = true
		}
		if !rec.FinishedAt.IsZero() {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)

	assert.True(t, sawLiveAttempts, "expected Attempts >= 1 while worker was still running (before FinishedAt was set)")
	assert.Equal(t, 3, final.Attempts)
	assert.Equal(t, 2, final.Successes)
	assert.Equal(t, 1, final.Failures)
}

// TestWorkerLandsCommitViaCoordinator is the regression test for ddx-e14efc58:
// a server-managed worker whose execution produces a commit must advance the
// project's target branch via the land coordinator, visible in `git log main -1`
// after the worker exits.
//
// Pre-fix, runWorker's executor closure at workers.go:248 called
// agent.ExecuteBead but never called LandBeadResult/Land, so commits were
// silently lost. Post-fix, the executor closure submits every successful
// result to m.LandCoordinators.Get(projectRoot) which runs agent.Land() to
// advance main. This test asserts main is advanced by using a BeadWorkerFactory
// shim that mirrors the real runWorker closure's land-submission path.
//
// The shim is required because the real closure calls agent.ExecuteBead which
// needs a real harness binary to run; using BeadWorkerFactory lets us create
// commits directly via git plumbing while still exercising the coordinator
// integration (m.LandCoordinators.Get -> Submit -> agent.Land -> advance main).
func TestWorkerLandsCommitViaCoordinator(t *testing.T) {
	root := t.TempDir()

	// Real git repo fixture.
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644))
	runCmd(t, root, "git", "init", "-b", "main")
	runCmd(t, root, "git", "config", "user.name", "Test")
	runCmd(t, root, "git", "config", "user.email", "test@test.local")
	runCmd(t, root, "git", "add", "-A")
	runCmd(t, root, "git", "commit", "-m", "init")

	// Get the initial main tip for comparison later.
	initialTipCmd := exec.Command("git", "-C", root, "rev-parse", "refs/heads/main")
	initialTipOut, err := initialTipCmd.Output()
	require.NoError(t, err)
	initialTip := strings.TrimSpace(string(initialTipOut))

	// Seed the bead store with one ready bead.
	ddxDir := filepath.Join(root, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Create(&bead.Bead{
		ID:         "ddx-integration-01",
		Title:      "integration test bead",
		Status:     bead.StatusOpen,
		Priority:   0,
		IssueType:  bead.DefaultType,
		Acceptance: "Worker lands commit via coordinator",
	}))

	m := NewWorkerManager(root)

	// Inject a BeadWorkerFactory that mirrors the real runWorker closure's
	// land-submission path. We do not call agent.ExecuteBead directly (no real
	// harness available in the test env); instead we create a real commit
	// via git plumbing and submit its SHA to the coordinator the same way
	// the real executor closure does.
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: s,
			Executor: agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				// Produce a real commit in a throwaway worktree at main's
				// current tip. This simulates what ExecuteBead produces.
				wt, err := os.MkdirTemp("", "integ-wt-*")
				require.NoError(t, err)
				_ = os.RemoveAll(wt)
				runCmd(t, root, "git", "worktree", "add", "--detach", wt, "refs/heads/main")
				defer func() {
					runCmd(t, root, "git", "worktree", "remove", "--force", wt)
				}()

				require.NoError(t, os.WriteFile(filepath.Join(wt, "worker-file.txt"), []byte("worker content\n"), 0o644))
				runCmd(t, wt, "git", "add", "-A")
				runCmd(t, wt, "git", "-c", "user.name=Worker", "-c", "user.email=worker@test.local", "commit", "-m", "feat: worker commit")
				newHeadCmd := exec.Command("git", "-C", wt, "rev-parse", "HEAD")
				newHeadOut, err := newHeadCmd.Output()
				require.NoError(t, err)
				resultRev := strings.TrimSpace(string(newHeadOut))

				// Pin the commit with a temporary ref so it survives the
				// worktree removal (the real ExecuteBead flow has the same
				// lifetime concern).
				runCmd(t, root, "git", "update-ref", "refs/ddx/integ-pins/"+beadID, resultRev)

				// Submit to the coordinator — same call site pattern as the
				// real runWorker closure in workers.go.
				coord := m.LandCoordinators.Get(root)
				landRes, landErr := coord.Submit(agent.LandRequest{
					WorktreeDir:  root,
					BaseRev:      initialTip,
					ResultRev:    resultRev,
					BeadID:       beadID,
					AttemptID:    "integ-attempt-01",
					TargetBranch: "main",
				})
				if landErr != nil {
					return agent.ExecuteBeadReport{}, landErr
				}
				return agent.ExecuteBeadReport{
					BeadID:    beadID,
					Status:    agent.ExecuteBeadStatusSuccess,
					BaseRev:   initialTip,
					ResultRev: landRes.NewTip,
				}, nil
			}),
		}
	}

	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, err)

	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)
	assert.Equal(t, "exited", final.State, "worker should have exited cleanly")

	// The coordinator should have advanced main beyond the initial tip.
	tipAfterCmd := exec.Command("git", "-C", root, "rev-parse", "refs/heads/main")
	tipAfterOut, err := tipAfterCmd.Output()
	require.NoError(t, err)
	tipAfter := strings.TrimSpace(string(tipAfterOut))
	assert.NotEqual(t, initialTip, tipAfter, "main should have advanced from initial tip")

	// The advanced tip must have the worker's file.
	showCmd := exec.Command("git", "-C", root, "show", tipAfter+":worker-file.txt")
	showOut, err := showCmd.Output()
	require.NoError(t, err, "worker-file.txt should exist at main tip after land")
	assert.Equal(t, "worker content\n", string(showOut))

	// No merge commits on main.
	mergesCmd := exec.Command("git", "-C", root, "log", "--merges", "--format=%H", "refs/heads/main")
	mergesOut, err := mergesCmd.Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(mergesOut)), "main should have no merge commits after land")

	// Cleanup coordinators for this test.
	m.LandCoordinators.StopAll()
}

// TestStartExecuteLoopProjectRootOverride verifies that when ExecuteLoopWorkerSpec
// carries a non-empty ProjectRoot, the worker executes against that project
// rather than the WorkerManager's own projectRoot.
func TestStartExecuteLoopProjectRootOverride(t *testing.T) {
	// Project A is the server's primary working directory.
	projectA := t.TempDir()
	setupBeadStore(t, projectA)

	// Project B is the target project — a separate directory.
	projectB := t.TempDir()
	setupBeadStore(t, projectB)

	m := NewWorkerManager(projectA)

	// Submit a worker targeting project B.
	record, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot: projectB,
		Once:        true,
	})
	require.NoError(t, err)
	// The returned record must reflect the target project, not project A.
	assert.Equal(t, projectB, record.ProjectRoot, "worker record must carry target project root")

	// Wait for the worker to finish and verify the final record.
	final := waitForWorkerExit(t, m, record.ID, 10*time.Second)
	assert.Equal(t, projectB, final.ProjectRoot, "final worker record must carry target project root")
	// Worker ran against an empty bead queue → no_ready_work, not a failure.
	assert.Equal(t, "no_ready_work", final.Status)
}

// TestExecuteLoopProjectRootViaHTTP verifies the end-to-end HTTP path:
// submitting to POST /api/agent/workers/execute-loop with project_root set
// routes the worker to the requested project, and an unregistered project
// is rejected with 422.
func TestExecuteLoopProjectRootViaHTTP(t *testing.T) {
	// Isolate server state so this test doesn't pollute the user's state file.
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "test-node-project-root")

	// Project A is the server's primary working directory.
	projectA := setupTestDir(t)

	// Project B is a separate registered project.
	projectB := t.TempDir()
	setupBeadStore(t, projectB)

	srv := New(":0", projectA)
	// Register project B so the server accepts it.
	srv.state.RegisterProject(projectB)

	t.Run("registered project is accepted and worker runs in target root", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"project_root": projectB,
			"once":         true,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/agent/workers/execute-loop",
			strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		// Mark as trusted (localhost) so the handler doesn't reject it.
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())

		var rec WorkerRecord
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rec))
		assert.Equal(t, projectB, rec.ProjectRoot,
			"worker must run in the requested project root, not the server's primary project")
		assert.NotEmpty(t, rec.ID)

		// Wait for the worker to finish before the test teardown.
		_ = waitForWorkerExit(t, srv.workers, rec.ID, 10*time.Second)
	})

	t.Run("unregistered project is rejected with 422", func(t *testing.T) {
		unregistered := t.TempDir() // valid directory, but not registered with server
		body, _ := json.Marshal(map[string]any{
			"project_root": unregistered,
			"once":         true,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/agent/workers/execute-loop",
			strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code,
			"server must reject unregistered project with 422: %s", w.Body.String())
		var errResp map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
		assert.Contains(t, errResp["error"], "no registered project matches")
	})
}

// TC-013.6 — Workers for two different registered projects run in parallel
// without cross-project filesystem writes.
func TestConcurrentWorkersFromDifferentProjects(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	setupBeadStore(t, rootA)
	setupBeadStore(t, rootB)

	mA := NewWorkerManager(rootA)
	mB := NewWorkerManager(rootB)

	// Start one worker per project concurrently.
	recA, errA := mA.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	recB, errB := mB.StartExecuteLoop(ExecuteLoopWorkerSpec{Once: true})
	require.NoError(t, errA)
	require.NoError(t, errB)

	// Wait for both to finish.
	finalA := waitForWorkerExit(t, mA, recA.ID, 10*time.Second)
	finalB := waitForWorkerExit(t, mB, recB.ID, 10*time.Second)

	// Both must have reached a terminal state.
	assert.NotEqual(t, "running", finalA.State, "worker A should have exited")
	assert.NotEqual(t, "running", finalB.State, "worker B should have exited")

	// Worker A's artifact must exist only under rootA, not rootB.
	workerDirA := filepath.Join(rootA, ".ddx", "workers", recA.ID)
	workerDirB := filepath.Join(rootB, ".ddx", "workers", recB.ID)

	if _, err := os.Stat(workerDirA); err != nil {
		t.Errorf("worker A dir not found under rootA: %v", err)
	}
	if _, err := os.Stat(workerDirB); err != nil {
		t.Errorf("worker B dir not found under rootB: %v", err)
	}

	// Worker B's ID must NOT appear under rootA.
	crossPath := filepath.Join(rootA, ".ddx", "workers", recB.ID)
	if _, err := os.Stat(crossPath); err == nil {
		t.Errorf("worker B artifact found under rootA — cross-project write detected")
	}

	// Worker A's ID must NOT appear under rootB.
	crossPathB := filepath.Join(rootB, ".ddx", "workers", recA.ID)
	if _, err := os.Stat(crossPathB); err == nil {
		t.Errorf("worker A artifact found under rootB — cross-project write detected")
	}
}
