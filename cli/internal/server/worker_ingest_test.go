package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// postLoopback sends a JSON POST to path with body and returns the recorder.
// Sets the loopback RemoteAddr so the requireTrusted gate passes.
func postLoopback(t *testing.T, srv *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

// readWorkerEventsLog returns each line of .ddx/server/worker-events.jsonl as
// a parsed loggedEvent. Returns an empty slice when the file does not exist.
func readWorkerEventsLog(t *testing.T, workingDir string) []loggedEvent {
	t.Helper()
	path := filepath.Join(workingDir, ".ddx", "server", "worker-events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open events log: %v", err)
	}
	defer f.Close()
	var out []loggedEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev loggedEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("decode log line %q: %v", sc.Text(), err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	return out
}

// registerWorker POSTs a register payload and returns the issued worker_id.
func registerWorker(t *testing.T, srv *Server, projectRoot string) string {
	t.Helper()
	w := postLoopback(t, srv, "/api/workers/register", workerIdentity{
		ProjectRoot:  projectRoot,
		Harness:      "claude",
		ExecutorPID:  4242,
		ExecutorHost: "host.local",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("register: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if resp.WorkerID == "" {
		t.Fatal("register: empty worker_id")
	}
	if !strings.HasPrefix(resp.WorkerID, "wkr-") {
		t.Errorf("worker_id %q missing wkr- prefix", resp.WorkerID)
	}
	return resp.WorkerID
}

func TestWorkerRegister_HappyPath(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	id1 := registerWorker(t, srv, dir)
	id2 := registerWorker(t, srv, dir)

	if id1 == id2 {
		t.Errorf("expected distinct worker_ids, got %q twice", id1)
	}

	snap := srv.workerIngest.snapshot()
	if len(snap) != 2 {
		t.Fatalf("registry size: got %d, want 2", len(snap))
	}
	for _, rec := range snap {
		if rec.Identity.Harness != "claude" {
			t.Errorf("harness: got %q, want claude", rec.Identity.Harness)
		}
		if rec.RegisteredAt.IsZero() {
			t.Error("RegisteredAt unset")
		}
		if rec.LastEventAt.IsZero() {
			t.Error("LastEventAt unset")
		}
	}
}

func TestWorkerEvent_AppendsToJSONL(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	workerID := registerWorker(t, srv, dir)

	evs := []workerEvent{
		{BeadID: "ddx-aaaa", AttemptID: "att-1", Kind: "attempt.started"},
		{BeadID: "ddx-aaaa", AttemptID: "att-1", Kind: "picker.priority_skip", Body: json.RawMessage(`{"skipped":"ddx-bbbb"}`)},
		{BeadID: "ddx-aaaa", AttemptID: "att-1", Kind: "result", Body: json.RawMessage(`{"outcome":"closed"}`)},
	}
	for _, ev := range evs {
		w := postLoopback(t, srv, "/api/workers/"+workerID+"/event", ev)
		if w.Code != http.StatusNoContent {
			t.Fatalf("event %s: status=%d body=%s", ev.Kind, w.Code, w.Body.String())
		}
	}

	logged := readWorkerEventsLog(t, dir)
	if len(logged) != len(evs) {
		t.Fatalf("log lines: got %d, want %d", len(logged), len(evs))
	}
	for i, ev := range evs {
		if logged[i].WorkerID != workerID {
			t.Errorf("line %d worker_id: got %q, want %q", i, logged[i].WorkerID, workerID)
		}
		if logged[i].Kind != ev.Kind {
			t.Errorf("line %d kind: got %q, want %q", i, logged[i].Kind, ev.Kind)
		}
		if logged[i].BeadID != ev.BeadID {
			t.Errorf("line %d bead_id: got %q, want %q", i, logged[i].BeadID, ev.BeadID)
		}
		if logged[i].Timestamp.IsZero() {
			t.Errorf("line %d timestamp unset", i)
		}
	}

	snap := srv.workerIngest.snapshot()
	if len(snap) != 1 {
		t.Fatalf("registry size: got %d, want 1", len(snap))
	}
	if snap[0].LastEventAt.Equal(snap[0].RegisteredAt) {
		t.Error("LastEventAt should advance past RegisteredAt after event")
	}
}

func TestWorkerBackfill_PostsBufferedEvents(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	workerID := registerWorker(t, srv, dir)

	req := workerBackfillRequest{
		Events: []workerEvent{
			{BeadID: "ddx-aaaa", AttemptID: "att-1", Kind: "attempt.started"},
			{BeadID: "ddx-aaaa", AttemptID: "att-1", Kind: "result", Body: json.RawMessage(`{"outcome":"closed"}`)},
			{BeadID: "ddx-bbbb", AttemptID: "att-2", Kind: "attempt.started"},
		},
		Dropped: true,
	}
	w := postLoopback(t, srv, "/api/workers/"+workerID+"/backfill", req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("backfill: status=%d body=%s", w.Code, w.Body.String())
	}

	logged := readWorkerEventsLog(t, dir)
	if len(logged) != len(req.Events) {
		t.Fatalf("backfill log lines: got %d, want %d", len(logged), len(req.Events))
	}
	for i, ev := range req.Events {
		if logged[i].Kind != ev.Kind {
			t.Errorf("line %d kind: got %q, want %q", i, logged[i].Kind, ev.Kind)
		}
		if logged[i].BeadID != ev.BeadID {
			t.Errorf("line %d bead_id: got %q, want %q", i, logged[i].BeadID, ev.BeadID)
		}
	}

	snap := srv.workerIngest.snapshot()
	if len(snap) != 1 {
		t.Fatalf("registry size: got %d, want 1", len(snap))
	}
	if !snap[0].HadDroppedBackfill {
		t.Error("HadDroppedBackfill should be true after dropped=true backfill")
	}
}

// childEnvVar is set on the subprocess invocation of
// TestHelperPostRegisterChild and carries the URL of the parent's
// httptest.Server. Its presence is what activates the child code path; if
// unset the child test simply skips so a normal `go test ./...` run is a
// no-op for the helper.
const childEnvVar = "DDX_INGESTION_E2E_REGISTER_URL"

// TestServerIngestion_RealWorkerCanRegister is the wired-in integration
// proof for ADR-022 step 1: it stands up the production HTTP path
// (httptest.Server wrapping srv.Handler() — same mux, same requireTrusted
// gate, same handler, real TCP socket) and then re-execs the test binary as
// a subprocess to POST /api/workers/register over the network. The
// assertion is on the parent's in-memory registry, proving the round-trip
// from a real external HTTP client landed in the worker view.
func TestServerIngestion_RealWorkerCanRegister(t *testing.T) {
	if os.Getenv(childEnvVar) != "" {
		// Defensive: should never happen because the child invocation runs
		// only TestHelperPostRegisterChild, but if a future test runner
		// fans out differently we don't want this test to spin up its own
		// nested subprocess.
		t.Skip("child invocation; covered by TestHelperPostRegisterChild")
	}
	dir := setupTestDir(t)
	srv := New(":0", dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cmd := exec.Command(os.Args[0],
		"-test.run", "^TestHelperPostRegisterChild$",
		"-test.v",
		"-test.count=1",
		"-test.timeout=30s",
	)
	cmd.Env = append(os.Environ(), childEnvVar+"="+ts.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}

	deadline := time.Now().Add(2 * time.Second)
	var snap []*workerRecord
	for time.Now().Before(deadline) {
		snap = srv.workerIngest.snapshot()
		if len(snap) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(snap) != 1 {
		t.Fatalf("registry size: got %d, want 1; subprocess output:\n%s", len(snap), out)
	}
	rec := snap[0]
	if rec.Identity.Harness != "subprocess-harness" {
		t.Errorf("identity.harness: got %q, want subprocess-harness", rec.Identity.Harness)
	}
	if rec.Identity.ExecutorHost != "subprocess-host" {
		t.Errorf("identity.executor_host: got %q, want subprocess-host", rec.Identity.ExecutorHost)
	}
	if rec.WorkerID == "" || !strings.HasPrefix(rec.WorkerID, "wkr-") {
		t.Errorf("worker_id %q missing wkr- prefix", rec.WorkerID)
	}
	if rec.RegisteredAt.IsZero() {
		t.Error("RegisteredAt unset")
	}
}

// TestHelperPostRegisterChild is the subprocess half of
// TestServerIngestion_RealWorkerCanRegister. It is a normal-looking Go
// test that no-ops unless re-invoked with childEnvVar set, in which case
// it performs a real net/http POST against the parent's httptest.Server.
func TestHelperPostRegisterChild(t *testing.T) {
	url := os.Getenv(childEnvVar)
	if url == "" {
		t.Skip("not invoked as subprocess child; parent test re-execs us")
	}
	body, err := json.Marshal(workerIdentity{
		ProjectRoot:  "/tmp/subprocess-project",
		Harness:      "subprocess-harness",
		Model:        "claude-opus-4",
		ExecutorPID:  os.Getpid(),
		ExecutorHost: "subprocess-host",
		StartedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	resp, err := http.Post(url+"/api/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/workers/register: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, respBody)
	}
	var out struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("decode response %q: %v", respBody, err)
	}
	if out.WorkerID == "" {
		t.Fatal("empty worker_id in response")
	}
	fmt.Printf("subprocess registered worker_id=%s\n", out.WorkerID)
}

func TestWorkerEvent_410_TriggersReregister(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Worker that never registered (or whose registration was wiped by a
	// server restart) POSTs an event with a stale worker_id. The server
	// MUST respond 410 so the worker re-registers within the same probe
	// cycle (ADR-022 §Probe + freshness state model).
	w := postLoopback(t, srv, "/api/workers/wkr-missing/event", workerEvent{
		BeadID: "ddx-aaaa",
		Kind:   "attempt.started",
	})
	if w.Code != http.StatusGone {
		t.Fatalf("event with unknown worker_id: status=%d, want 410", w.Code)
	}

	// Same situation for backfill: 410 keeps the worker's buffer intact and
	// triggers re-registration.
	w = postLoopback(t, srv, "/api/workers/wkr-missing/backfill", workerBackfillRequest{
		Events: []workerEvent{{BeadID: "ddx-aaaa", Kind: "result"}},
	})
	if w.Code != http.StatusGone {
		t.Fatalf("backfill with unknown worker_id: status=%d, want 410", w.Code)
	}

	// After re-registration the worker_id is valid and the event lands.
	workerID := registerWorker(t, srv, dir)
	w = postLoopback(t, srv, "/api/workers/"+workerID+"/event", workerEvent{
		BeadID: "ddx-aaaa",
		Kind:   "attempt.started",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("event after reregister: status=%d body=%s", w.Code, w.Body.String())
	}
	logged := readWorkerEventsLog(t, dir)
	if len(logged) != 1 {
		t.Fatalf("log lines after reregister: got %d, want 1", len(logged))
	}
}
