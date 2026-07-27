package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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
//
// The event log is size-bounded: when the active file would exceed
// maxActiveBytes, the registry closes the descriptor, renames the file to a
// timestamped generation, prunes older generations beyond maxGenerations, and
// opens a fresh active file — all under mu. On open, an invalid final tail
// (partial line / embedded NULs) is truncated so complete JSON objects remain.

const workerIngestMaxBodyBytes = 1 << 20             // 1 MiB cap on register/event/backfill payloads.
const workerIngestDisconnectedTTL = 10 * time.Minute // Evict workers disconnected longer than this

// Default worker-events.jsonl retention policy. Tests inject smaller caps.
const (
	defaultWorkerEventsMaxActiveBytes int64 = 16 << 20 // 16 MiB
	defaultWorkerEventsMaxGenerations       = 3
)

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
	mu             sync.Mutex
	workers        map[string]*workerRecord
	logPath        string
	logFile        *os.File
	logSize        int64
	maxActiveBytes int64
	maxGenerations int
	rotateSeq      uint64 // disambiguates archive names under rapid rotation

	// rotateHook, when set, runs after the active descriptor is closed and
	// before rename during rotation. Tests inject failures here.
	rotateHook func() error
}

func newWorkerIngestRegistry(workingDir string) *workerIngestRegistry {
	return &workerIngestRegistry{
		workers:        make(map[string]*workerRecord),
		logPath:        ddxroot.JoinProject(workingDir, "server", "worker-events.jsonl"),
		maxActiveBytes: defaultWorkerEventsMaxActiveBytes,
		maxGenerations: defaultWorkerEventsMaxGenerations,
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

// reap removes entries from the registry that have been disconnected past the
// TTL. Called with lock held.
func (r *workerIngestRegistry) reapLocked(now time.Time) {
	var toRemove []string
	for id, rec := range r.workers {
		age := now.Sub(rec.LastEventAt)
		if age > workerIngestDisconnectedTTL {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		delete(r.workers, id)
	}
}

// snapshot returns a copy of the registry for read-only inspection (tests
// and future GraphQL consumers). The returned slice is a stable copy; the
// records themselves are pointers into the registry and must be treated
// as read-only. Stale entries disconnected past the TTL are reaped.
func (r *workerIngestRegistry) snapshot() []*workerRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.reapLocked(now)
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

	if err := r.ensureOpenLocked(); err != nil {
		r.noteMirrorFailureLocked(workerID)
		return err
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

	// Rotate before writing when the active file already holds data and the
	// next line would exceed the cap. A single complete event is never
	// truncated, even if it alone exceeds maxActiveBytes.
	if r.maxActiveBytes > 0 && r.logSize > 0 && r.logSize+int64(len(line)) > r.maxActiveBytes {
		if err := r.rotateLocked(); err != nil {
			r.noteMirrorFailureLocked(workerID)
			return err
		}
	}

	n, err := r.logFile.Write(line)
	if err != nil {
		r.noteMirrorFailureLocked(workerID)
		return err
	}
	r.logSize += int64(n)
	return nil
}

// noteMirrorFailureLocked increments MirrorFailuresCount for workerID so
// rotation/write/recovery failures surface in worker health. Caller holds mu.
func (r *workerIngestRegistry) noteMirrorFailureLocked(workerID string) {
	if rec, ok := r.workers[workerID]; ok {
		rec.MirrorFailuresCount++
	}
}

// ensureOpenLocked opens the active log if needed, recovering an invalid
// final tail first. Caller holds mu.
func (r *workerIngestRegistry) ensureOpenLocked() error {
	if r.logFile != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.logPath), 0o755); err != nil {
		return err
	}
	if err := recoverWorkerEventsTail(r.logPath); err != nil {
		return err
	}
	f, err := os.OpenFile(r.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.logFile = f
	r.logSize = fi.Size()
	return nil
}

// rotateLocked closes the active descriptor, renames it to a timestamped
// generation, prunes excess generations, and opens a fresh active file.
// Caller holds mu.
func (r *workerIngestRegistry) rotateLocked() error {
	if r.logFile != nil {
		if err := r.logFile.Close(); err != nil {
			r.logFile = nil
			return fmt.Errorf("close worker events log: %w", err)
		}
		r.logFile = nil
	}
	if r.rotateHook != nil {
		if err := r.rotateHook(); err != nil {
			return err
		}
	}
	// Timestamp + monotonic seq keeps rapid rotations from clobbering archives
	// (Linux rename replaces an existing destination).
	r.rotateSeq++
	archive := fmt.Sprintf("%s.%s-%d",
		r.logPath,
		time.Now().UTC().Format("20060102T150405.000000000"),
		r.rotateSeq,
	)
	if err := os.Rename(r.logPath, archive); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename worker events log: %w", err)
	}
	if err := r.pruneGenerationsLocked(); err != nil {
		return err
	}
	f, err := os.OpenFile(r.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open rotated worker events log: %w", err)
	}
	r.logFile = f
	r.logSize = 0
	return nil
}

// pruneGenerationsLocked keeps at most maxGenerations rotated files
// (timestamp suffix after the active basename). Oldest first. Caller holds mu.
func (r *workerIngestRegistry) pruneGenerationsLocked() error {
	if r.maxGenerations < 0 {
		return nil
	}
	dir := filepath.Dir(r.logPath)
	base := filepath.Base(r.logPath)
	prefix := base + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var archives []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		// Active file is base with no suffix; only generations match prefix.
		archives = append(archives, filepath.Join(dir, name))
	}
	sort.Strings(archives) // chronological under YYYYMMDDTHHMMSS layout
	for len(archives) > r.maxGenerations {
		_ = os.Remove(archives[0])
		archives = archives[1:]
	}
	return nil
}

// recoverWorkerEventsTail truncates only an invalid final tail (partial line,
// embedded NULs, or non-JSON garbage after the last complete record) while
// preserving every complete loggedEvent line. A missing file is a no-op.
func recoverWorkerEventsTail(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	validEnd := 0
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		line := data[start:i]
		start = i + 1
		if len(bytes.TrimSpace(line)) == 0 {
			validEnd = i + 1
			continue
		}
		var ev loggedEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Stop at first unparseable full line; treat remainder as bad tail.
			break
		}
		validEnd = i + 1
	}
	// Trailing bytes without a newline: accept only if they form a complete object.
	if start < len(data) {
		line := data[start:]
		if len(bytes.TrimSpace(line)) > 0 {
			var ev loggedEvent
			if err := json.Unmarshal(line, &ev); err == nil {
				// Complete object missing its trailing newline — rewrite with one.
				fixed := append(append([]byte{}, data[:start]...), line...)
				if !bytes.HasSuffix(fixed, []byte{'\n'}) {
					fixed = append(fixed, '\n')
				}
				return os.WriteFile(path, fixed, 0o644)
			}
			// Incomplete / NUL / garbage tail — fall through to truncate.
		} else {
			validEnd = len(data)
		}
	}

	if validEnd < len(data) {
		return os.Truncate(path, int64(validEnd))
	}
	return nil
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

// workerIngestView is the wire shape returned by GET /api/workers — a
// flattened snapshot of the runtime registry used by `legacy agent doctor`
// (ADR-022 step 6). The on-disk .ddx/workers/<id>/status.json layout
// remains the fallback source for one alpha release.
type workerIngestView struct {
	WorkerID            string    `json:"worker_id"`
	ProjectRoot         string    `json:"project_root"`
	Harness             string    `json:"harness,omitempty"`
	Model               string    `json:"model,omitempty"`
	RegisteredAt        time.Time `json:"registered_at"`
	LastEventAt         time.Time `json:"last_event_at"`
	MirrorFailuresCount int       `json:"mirror_failures_count"`
	HadDroppedBackfill  bool      `json:"had_dropped_backfill,omitempty"`
	Freshness           string    `json:"freshness"`
}

// handleWorkerIngestList serves GET /api/workers: a snapshot of the
// in-memory runtime registry (ADR-022 rev 5). Empty list when no workers
// have registered. Fields mirror workerRecord plus a derived freshness
// classification.
func (s *Server) handleWorkerIngestList(w http.ResponseWriter, r *http.Request) {
	snap := s.workerIngest.snapshot()
	now := time.Now().UTC()
	out := make([]workerIngestView, 0, len(snap))
	for _, rec := range snap {
		out = append(out, workerIngestView{
			WorkerID:            rec.WorkerID,
			ProjectRoot:         rec.Identity.ProjectRoot,
			Harness:             rec.Identity.Harness,
			Model:               rec.Identity.Model,
			RegisteredAt:        rec.RegisteredAt,
			LastEventAt:         rec.LastEventAt,
			MirrorFailuresCount: rec.MirrorFailuresCount,
			HadDroppedBackfill:  rec.HadDroppedBackfill,
			Freshness:           freshnessState(rec, now),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RegisteredAt.After(out[j].RegisteredAt) })
	writeJSON(w, http.StatusOK, out)
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
