package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
	path := filepath.Join(workingDir, ddxroot.DirName, "server", "worker-events.jsonl")
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

func TestWorkerIngest_ReapsDisconnected(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Register two workers.
	id1 := registerWorker(t, srv, dir)
	id2 := registerWorker(t, srv, dir)

	snap := srv.workerIngest.snapshot()
	if len(snap) != 2 {
		t.Fatalf("initial registry size: got %d, want 2", len(snap))
	}

	// Manually advance the LastEventAt of id1 to be past the disconnect TTL.
	srv.workerIngest.mu.Lock()
	rec1 := srv.workerIngest.workers[id1]
	rec1.LastEventAt = time.Now().UTC().Add(-11 * time.Minute)
	srv.workerIngest.mu.Unlock()

	// snapshot() should reap id1 since it's disconnected past the TTL.
	snap = srv.workerIngest.snapshot()
	if len(snap) != 1 {
		t.Fatalf("after reap: got %d entries, want 1", len(snap))
	}
	if snap[0].WorkerID != id2 {
		t.Fatalf("remaining worker: got %q, want %q", snap[0].WorkerID, id2)
	}

	// Verify id1 is gone.
	if _, exists := srv.workerIngest.workers[id1]; exists {
		t.Errorf("worker %q should have been reaped", id1)
	}
}

// workerEventsLogPath returns the active worker-events.jsonl path for workingDir.
func workerEventsLogPath(workingDir string) string {
	return filepath.Join(workingDir, ddxroot.DirName, "server", "worker-events.jsonl")
}

// listWorkerEventsGenerations returns active + rotated generation paths
// (active first when present, then archives sorted by path).
func listWorkerEventsGenerations(t *testing.T, workingDir string) []string {
	t.Helper()
	dir := filepath.Join(workingDir, ddxroot.DirName, "server")
	base := "worker-events.jsonl"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("readdir %s: %v", dir, err)
	}
	var active []string
	var archives []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, name)
		switch {
		case name == base:
			active = append(active, p)
		case strings.HasPrefix(name, base+"."):
			archives = append(archives, p)
		}
	}
	sort.Strings(archives)
	return append(active, archives...)
}

// readLoggedEventsFile decodes every non-empty line of path as loggedEvent.
func readLoggedEventsFile(t *testing.T, path string) []loggedEvent {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []loggedEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev loggedEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("decode %s line %q: %v", path, sc.Text(), err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

// TestWorkerEvents_LogRotationKeepsActiveFileBounded appends enough events to
// cross a small injected cap and proves the active file and bounded generation
// set stay within the configured limits.
func TestWorkerEvents_LogRotationKeepsActiveFileBounded(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	const (
		maxActive      int64 = 800
		maxGenerations       = 2
	)
	srv.workerIngest.maxActiveBytes = maxActive
	srv.workerIngest.maxGenerations = maxGenerations

	workerID := registerWorker(t, srv, dir)

	// Each event line is ~150–250 bytes; append enough to force multiple rotations.
	for i := 0; i < 80; i++ {
		ev := workerEvent{
			BeadID:    fmt.Sprintf("ddx-bound-%04d", i),
			AttemptID: fmt.Sprintf("att-%04d", i),
			Kind:      "attempt.started",
			Body:      json.RawMessage(fmt.Sprintf(`{"n":%d,"pad":"xxxxxxxxxxxxxxxxxxxxxxxx"}`, i)),
		}
		if err := srv.workerIngest.recordEvent(workerID, ev); err != nil {
			t.Fatalf("recordEvent %d: %v", i, err)
		}
	}

	activePath := workerEventsLogPath(dir)
	fi, err := os.Stat(activePath)
	if err != nil {
		t.Fatalf("stat active log: %v", err)
	}
	if fi.Size() > maxActive {
		t.Fatalf("active log size %d exceeds cap %d", fi.Size(), maxActive)
	}

	gens := listWorkerEventsGenerations(t, dir)
	var archives []string
	for _, p := range gens {
		if filepath.Base(p) != "worker-events.jsonl" {
			archives = append(archives, p)
			afi, err := os.Stat(p)
			if err != nil {
				t.Fatalf("stat generation %s: %v", p, err)
			}
			// Rotated files were closed at the cap; they should not greatly exceed it
			// (one line may push slightly under maxActive before rotate on next write).
			if afi.Size() > maxActive {
				t.Fatalf("generation %s size %d exceeds cap %d", p, afi.Size(), maxActive)
			}
		}
	}
	if len(archives) > maxGenerations {
		t.Fatalf("retained generations: got %d, want <= %d (%v)", len(archives), maxGenerations, archives)
	}
	if len(archives) == 0 {
		t.Fatal("expected at least one rotated generation after crossing cap")
	}
}

// TestWorkerEvents_RotatedFilesRemainValidJSONL decodes every line from active
// and rotated generations as loggedEvent after concurrent recordEvent and
// recordBackfill calls.
func TestWorkerEvents_RotatedFilesRemainValidJSONL(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	srv.workerIngest.maxActiveBytes = 600
	// Retain enough generations that concurrent writes are not pruned mid-test
	// so every emitted line remains on disk for the validity check.
	srv.workerIngest.maxGenerations = 100

	workerID := registerWorker(t, srv, dir)

	const (
		eventN    = 40
		backfillN = 20
	)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < eventN; i++ {
			ev := workerEvent{
				BeadID:    fmt.Sprintf("ddx-evt-%04d", i),
				AttemptID: "att-evt",
				Kind:      "attempt.started",
				Body:      json.RawMessage(fmt.Sprintf(`{"src":"event","n":%d}`, i)),
			}
			if err := srv.workerIngest.recordEvent(workerID, ev); err != nil {
				errCh <- fmt.Errorf("recordEvent %d: %w", i, err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < backfillN; i++ {
			req := workerBackfillRequest{
				Events: []workerEvent{
					{
						BeadID:    fmt.Sprintf("ddx-bf-%04d", i),
						AttemptID: "att-bf",
						Kind:      "result",
						Body:      json.RawMessage(fmt.Sprintf(`{"src":"backfill","n":%d}`, i)),
					},
				},
			}
			if err := srv.workerIngest.recordBackfill(workerID, req); err != nil {
				errCh <- fmt.Errorf("recordBackfill %d: %w", i, err)
				return
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	gens := listWorkerEventsGenerations(t, dir)
	if len(gens) < 2 {
		t.Fatalf("expected active + at least one rotated generation, got %v", gens)
	}
	total := 0
	sawEvent, sawBackfill := false, false
	for _, p := range gens {
		evs := readLoggedEventsFile(t, p)
		total += len(evs)
		for _, ev := range evs {
			if ev.WorkerID != workerID {
				t.Errorf("%s: worker_id %q want %q", p, ev.WorkerID, workerID)
			}
			if ev.Kind == "" {
				t.Errorf("%s: empty kind", p)
			}
			if strings.HasPrefix(ev.BeadID, "ddx-evt-") {
				sawEvent = true
			}
			if strings.HasPrefix(ev.BeadID, "ddx-bf-") {
				sawBackfill = true
			}
		}
	}
	if total != eventN+backfillN {
		t.Fatalf("decoded events across generations: got %d, want %d", total, eventN+backfillN)
	}
	if !sawEvent || !sawBackfill {
		t.Fatalf("expected both event and backfill sources; event=%v backfill=%v", sawEvent, sawBackfill)
	}
}

// TestWorkerEvents_StartupRecoversInvalidTail seeds complete JSONL records
// followed by a partial/NUL tail, starts the registry, and proves complete
// records are preserved while subsequent appends remain parseable.
func TestWorkerEvents_StartupRecoversInvalidTail(t *testing.T) {
	dir := setupTestDir(t)
	logPath := workerEventsLogPath(dir)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}

	complete := []loggedEvent{
		{
			WorkerID:  "wkr-seed-1",
			Timestamp: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
			BeadID:    "ddx-seed-a",
			AttemptID: "att-1",
			Kind:      "attempt.started",
		},
		{
			WorkerID:  "wkr-seed-1",
			Timestamp: time.Date(2026, 7, 13, 12, 0, 1, 0, time.UTC),
			BeadID:    "ddx-seed-a",
			AttemptID: "att-1",
			Kind:      "result",
			Body:      json.RawMessage(`{"outcome":"closed"}`),
		},
	}
	var buf bytes.Buffer
	for _, ev := range complete {
		line, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	// Partial/NUL tail matching production corruption shape.
	buf.WriteString(`{"worker_id":"wkr-seed-1","kind":"partial`)
	buf.Write(bytes.Repeat([]byte{0}, 64))
	buf.WriteString(`not-json-tail`)
	if err := os.WriteFile(logPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// New registry/server: first append must recover the tail.
	srv := New(":0", dir)
	workerID := registerWorker(t, srv, dir)
	if err := srv.workerIngest.recordEvent(workerID, workerEvent{
		BeadID:    "ddx-after-recover",
		AttemptID: "att-new",
		Kind:      "attempt.started",
	}); err != nil {
		t.Fatalf("recordEvent after recover: %v", err)
	}

	// Active file must decode fully as loggedEvent lines; seed records kept.
	evs := readLoggedEventsFile(t, logPath)
	if len(evs) != 3 {
		t.Fatalf("events after recover: got %d, want 3 (2 seed + 1 new)", len(evs))
	}
	if evs[0].BeadID != "ddx-seed-a" || evs[0].Kind != "attempt.started" {
		t.Errorf("first seed event corrupted: %+v", evs[0])
	}
	if evs[1].BeadID != "ddx-seed-a" || evs[1].Kind != "result" {
		t.Errorf("second seed event corrupted: %+v", evs[1])
	}
	if evs[2].BeadID != "ddx-after-recover" || evs[2].WorkerID != workerID {
		t.Errorf("new event missing/corrupt: %+v", evs[2])
	}

	// File must not contain NULs after recovery + append.
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte{0}) {
		t.Fatal("active log still contains NUL bytes after recovery")
	}
}

// TestWorkerEvents_RotationFailureIncrementsMirrorFailures proves a
// close/rename/open/write failure is surfaced in worker health and does not
// silently discard the diagnosis.
func TestWorkerEvents_RotationFailureIncrementsMirrorFailures(t *testing.T) {
	dir := setupTestDir(t)
	srv := New(":0", dir)

	// Tiny cap so the second event forces rotation.
	srv.workerIngest.maxActiveBytes = 200
	srv.workerIngest.maxGenerations = 2
	srv.workerIngest.rotateHook = func() error {
		return errors.New("injected rotation failure")
	}

	workerID := registerWorker(t, srv, dir)

	// First event opens and fills the active log without rotating.
	ev1 := workerEvent{
		BeadID:    "ddx-rot-fail-1",
		AttemptID: "att-1",
		Kind:      "attempt.started",
		Body:      json.RawMessage(`{"pad":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`),
	}
	if err := srv.workerIngest.recordEvent(workerID, ev1); err != nil {
		t.Fatalf("first recordEvent: %v", err)
	}

	// Second event should attempt rotation and fail via rotateHook.
	ev2 := workerEvent{
		BeadID:    "ddx-rot-fail-2",
		AttemptID: "att-2",
		Kind:      "result",
		Body:      json.RawMessage(`{"pad":"yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy"}`),
	}
	err := srv.workerIngest.recordEvent(workerID, ev2)
	if err == nil {
		t.Fatal("expected rotation failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected rotation failure") {
		t.Fatalf("error: got %v, want injected rotation failure", err)
	}

	snap := srv.workerIngest.snapshot()
	if len(snap) != 1 {
		t.Fatalf("registry size: got %d, want 1", len(snap))
	}
	if snap[0].MirrorFailuresCount < 1 {
		t.Fatalf("MirrorFailuresCount: got %d, want >= 1", snap[0].MirrorFailuresCount)
	}

	// Health wire shape (GET /api/workers) must expose the same count.
	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/workers: status=%d body=%s", w.Code, w.Body.String())
	}
	var views []workerIngestView
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode workers: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("workers list: got %d, want 1", len(views))
	}
	if views[0].MirrorFailuresCount < 1 {
		t.Fatalf("wire mirror_failures_count: got %d, want >= 1", views[0].MirrorFailuresCount)
	}
}
