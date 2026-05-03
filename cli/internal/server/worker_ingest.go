package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ADR-022 §Worker-server interface (rev 5). Workers are autonomous; the
// server's view of who's working is derived from best-effort event reports.
// This file implements the three ingestion endpoints:
//
//   POST /api/workers/register        — issue a worker_id for an identity envelope
//   POST /api/workers/{id}/event      — append a worker-emitted event
//   POST /api/workers/{id}/backfill   — replay buffered events after reconnect
//
// In-memory derived view backed by an append-only JSONL log at
// .ddx/server/worker-events.jsonl under the server's WorkingDir. Both the
// registry and the log are best-effort: if the server crashes the workers
// keep operating and re-register on next probe.

const workerIngestMaxBodyBytes = 1 << 20 // 1 MiB cap on register/event/backfill payloads.

// errUnknownWorker signals that a worker_id is not present in the in-memory
// registry. Surfaced as 410 Gone so the worker re-registers within the same
// probe cycle (ADR-022 §Probe + freshness state model).
var errUnknownWorker = errors.New("unknown_worker")

// workerIdentity is the thin envelope POSTed on /api/workers/register.
type workerIdentity struct {
	ProjectRoot  string    `json:"project_root"`
	Harness      string    `json:"harness"`
	Model        string    `json:"model,omitempty"`
	ExecutorPID  int       `json:"executor_pid"`
	ExecutorHost string    `json:"executor_host"`
	StartedAt    time.Time `json:"started_at"`
}

// workerEvent mirrors a single bead-event the worker would write to its
// local event log. The wire payload is intentionally minimal — the bead's
// local log remains the authoritative copy.
type workerEvent struct {
	BeadID    string          `json:"bead_id"`
	AttemptID string          `json:"attempt_id"`
	Kind      string          `json:"kind"`
	Body      json.RawMessage `json:"body,omitempty"`
}

// workerBackfillRequest carries the worker's NotConnected ring buffer.
// dropped=true means oldest events were silently dropped at the worker
// (rev 5: HadDroppedBackfill flag surfaces "consult bead-local logs").
type workerBackfillRequest struct {
	Events  []workerEvent `json:"events"`
	Dropped bool          `json:"dropped,omitempty"`
}

// workerRecord is the per-worker derived state the server holds in memory.
type workerRecord struct {
	WorkerID            string         `json:"worker_id"`
	Identity            workerIdentity `json:"identity"`
	RegisteredAt        time.Time      `json:"registered_at"`
	LastEventAt         time.Time      `json:"last_event_at"`
	MirrorFailuresCount int            `json:"mirror_failures_count"`
	HadDroppedBackfill  bool           `json:"had_dropped_backfill"`
	CurrentBead         string         `json:"current_bead,omitempty"`
	CurrentAttempt      string         `json:"current_attempt,omitempty"`
}

// workerIngestRegistry holds the in-memory derived view + owns the
// append-only event log. All exported methods are safe for concurrent use.
type workerIngestRegistry struct {
	mu       sync.Mutex
	workers  map[string]*workerRecord
	logPath  string
	logFile  *os.File
	openOnce sync.Once
	openErr  error
}

func newWorkerIngestRegistry(workingDir string) *workerIngestRegistry {
	return &workerIngestRegistry{
		workers: make(map[string]*workerRecord),
		logPath: filepath.Join(workingDir, ".ddx", "server", "worker-events.jsonl"),
	}
}

// register stores a new worker record and returns the issued worker_id.
func (r *workerIngestRegistry) register(id workerIdentity) *workerRecord {
	now := time.Now().UTC()
	rec := &workerRecord{
		WorkerID:     newWorkerID(),
		Identity:     id,
		RegisteredAt: now,
		LastEventAt:  now,
	}
	r.mu.Lock()
	r.workers[rec.WorkerID] = rec
	r.mu.Unlock()
	return rec
}

// recordEvent appends ev to the JSONL log and refreshes the worker's
// last_event_at timestamp. Returns errUnknownWorker if workerID is not
// registered.
func (r *workerIngestRegistry) recordEvent(workerID string, ev workerEvent) error {
	r.mu.Lock()
	rec, ok := r.workers[workerID]
	if !ok {
		r.mu.Unlock()
		return errUnknownWorker
	}
	rec.LastEventAt = time.Now().UTC()
	if ev.BeadID != "" {
		rec.CurrentBead = ev.BeadID
	}
	if ev.AttemptID != "" {
		rec.CurrentAttempt = ev.AttemptID
	}
	r.mu.Unlock()
	return r.append(workerID, ev)
}

// recordBackfill appends every event in the buffer; flips
// HadDroppedBackfill when the worker reports dropped overflow.
func (r *workerIngestRegistry) recordBackfill(workerID string, req workerBackfillRequest) error {
	r.mu.Lock()
	rec, ok := r.workers[workerID]
	if !ok {
		r.mu.Unlock()
		return errUnknownWorker
	}
	if req.Dropped {
		rec.HadDroppedBackfill = true
	}
	if len(req.Events) > 0 {
		rec.LastEventAt = time.Now().UTC()
		last := req.Events[len(req.Events)-1]
		if last.BeadID != "" {
			rec.CurrentBead = last.BeadID
		}
		if last.AttemptID != "" {
			rec.CurrentAttempt = last.AttemptID
		}
	}
	r.mu.Unlock()
	for _, ev := range req.Events {
		if err := r.append(workerID, ev); err != nil {
			return err
		}
	}
	return nil
}

// snapshot returns a copy of the registry for read-only inspection (tests
// and future GraphQL consumers). The returned slice is a stable copy; the
// records themselves are pointers into the registry and must be treated
// as read-only.
func (r *workerIngestRegistry) snapshot() []*workerRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*workerRecord, 0, len(r.workers))
	for _, rec := range r.workers {
		copy := *rec
		out = append(out, &copy)
	}
	return out
}

// loggedEvent is the on-disk shape of a single line in worker-events.jsonl.
// Wraps the worker-supplied event with the receiving worker_id and the
// server's timestamp.
type loggedEvent struct {
	WorkerID  string          `json:"worker_id"`
	Timestamp time.Time       `json:"timestamp"`
	BeadID    string          `json:"bead_id,omitempty"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Kind      string          `json:"kind"`
	Body      json.RawMessage `json:"body,omitempty"`
}

func (r *workerIngestRegistry) append(workerID string, ev workerEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.openOnce.Do(func() {
		if err := os.MkdirAll(filepath.Dir(r.logPath), 0o755); err != nil {
			r.openErr = err
			return
		}
		f, err := os.OpenFile(r.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			r.openErr = err
			return
		}
		r.logFile = f
	})
	if r.openErr != nil {
		return r.openErr
	}
	line, err := json.Marshal(loggedEvent{
		WorkerID:  workerID,
		Timestamp: time.Now().UTC(),
		BeadID:    ev.BeadID,
		AttemptID: ev.AttemptID,
		Kind:      ev.Kind,
		Body:      ev.Body,
	})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = r.logFile.Write(line)
	return err
}

// close releases the log file handle. Safe to call when the file was
// never opened. Used on server shutdown / test cleanup.
func (r *workerIngestRegistry) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.logFile == nil {
		return nil
	}
	err := r.logFile.Close()
	r.logFile = nil
	return err
}

func newWorkerID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable; fall back to a timestamp
		// stub so the path is non-empty (registration still succeeds and
		// the operator sees a malformed-looking ID in the UI).
		return fmt.Sprintf("wkr-%d", time.Now().UnixNano())
	}
	return "wkr-" + hex.EncodeToString(b[:])
}

// --- HTTP handlers -----------------------------------------------------

func (s *Server) handleWorkerRegister(w http.ResponseWriter, r *http.Request) {
	var id workerIdentity
	if err := decodeWorkerIngestBody(r, &id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if id.ProjectRoot == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_root required"})
		return
	}
	rec := s.workerIngest.register(id)
	writeJSON(w, http.StatusOK, map[string]string{"worker_id": rec.WorkerID})
}

func (s *Server) handleWorkerEvent(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	if workerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "worker id required"})
		return
	}
	var ev workerEvent
	if err := decodeWorkerIngestBody(r, &ev); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if ev.Kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind required"})
		return
	}
	if err := s.workerIngest.recordEvent(workerID, ev); err != nil {
		if errors.Is(err, errUnknownWorker) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "unknown_worker"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWorkerBackfill(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	if workerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "worker id required"})
		return
	}
	var req workerBackfillRequest
	if err := decodeWorkerIngestBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.workerIngest.recordBackfill(workerID, req); err != nil {
		if errors.Is(err, errUnknownWorker) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "unknown_worker"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeWorkerIngestBody(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, workerIngestMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// freshnessState classifies a worker by recency of its last_event_at against
// the probe interval (ADR-022 rev 5: connected ≤2× probe; stale ≤10× probe;
// disconnected otherwise). Probe default is 30s, hard-coded here because the
// per-worker poll_interval is not part of the rev-5 register payload.
func freshnessState(rec *workerRecord, now time.Time) string {
	const probeInterval = 30 * time.Second
	age := now.Sub(rec.LastEventAt)
	switch {
	case age <= 2*probeInterval:
		return "connected"
	case age <= 10*probeInterval:
		return "stale"
	default:
		return "disconnected"
	}
}
