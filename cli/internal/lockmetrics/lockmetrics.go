// Package lockmetrics emits structured acquire/release events for the
// process-shared locks DDx workers hold around git index and tracker
// mutations (.git/index.lock and .ddx/.git-tracker.lock). Each held critical
// section produces two JSON-line events — one "acquire" and one "release" —
// so operators can see how long a worker held a lock and which operation held
// it, instead of inferring from filesystem mtimes and ps output.
//
// Events are routed through a process-wide sink. The default sink is nil
// (a no-op): instrumentation at the lock seams is therefore zero-cost unless
// an entrypoint enables a sink. The `ddx work` and `ddx run` commands enable
// the file sink so lock activity for a run is captured at
// .ddx/metrics/locks.jsonl, where it can be read back via Load.
package lockmetrics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// Event is one lock lifecycle event. An acquire event carries the
// acquisition-time fields; the matching release event additionally carries
// ReleasedAt and DurationMS so the held window is fully described.
type Event struct {
	// Event is "acquire" or "release".
	Event string `json:"event"`
	// LockName identifies the lock, e.g. "index.lock" or "tracker.lock".
	LockName string `json:"lock_name"`
	// Operation labels the work that held the lock, e.g. "tracker.commit"
	// or "index.commit".
	Operation string `json:"operation"`
	// HolderPID is the pid of the process that held the lock.
	HolderPID int `json:"holder_pid"`
	// AcquiredAt is the RFC3339Nano timestamp the lock was acquired.
	AcquiredAt string `json:"acquired_at"`
	// ReleasedAt is the RFC3339Nano timestamp the lock was released.
	// Empty on the acquire event.
	ReleasedAt string `json:"released_at,omitempty"`
	// DurationMS is ReleasedAt minus AcquiredAt in milliseconds.
	// Zero on the acquire event.
	DurationMS int64 `json:"duration_ms,omitempty"`
}

var (
	sinkMu sync.RWMutex
	sink   func(Event)
)

// SetSink installs the process-wide event sink. Passing nil disables event
// emission (the default). Safe for concurrent use.
func SetSink(fn func(Event)) {
	sinkMu.Lock()
	sink = fn
	sinkMu.Unlock()
}

// Emit routes ev to the current sink if one is installed; otherwise it is a
// no-op. Safe for concurrent use.
func Emit(ev Event) {
	sinkMu.RLock()
	fn := sink
	sinkMu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

// Instrument records the held window of an already-acquired lock: it emits an
// "acquire" event, runs critical, then emits a matching "release" event with
// the measured hold duration. The holder pid is the current process. The
// release event is emitted even if critical panics. critical's error is
// returned unchanged.
func Instrument(lockName, operation string, critical func() error) (err error) {
	pid := os.Getpid()
	acquiredAt := time.Now()
	Emit(Event{
		Event:      "acquire",
		LockName:   lockName,
		Operation:  operation,
		HolderPID:  pid,
		AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
	})
	defer func() {
		releasedAt := time.Now()
		Emit(Event{
			Event:      "release",
			LockName:   lockName,
			Operation:  operation,
			HolderPID:  pid,
			AcquiredAt: acquiredAt.UTC().Format(time.RFC3339Nano),
			ReleasedAt: releasedAt.UTC().Format(time.RFC3339Nano),
			DurationMS: releasedAt.Sub(acquiredAt).Milliseconds(),
		})
	}()
	return critical()
}

// Path returns the absolute path to the lock-events stream under the project's
// DDx state root: .ddx/metrics/locks.jsonl.
func Path(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "metrics", "locks.jsonl")
}

// FileSink returns a sink that appends each event as one JSON line to
// Path(projectRoot). Appends are serialized with O_APPEND and an internal
// mutex so concurrent workers in one process do not interleave partial lines.
// Write errors are best-effort and discarded — observability must never break
// the operation being observed.
func FileSink(projectRoot string) func(Event) {
	path := Path(projectRoot)
	var mu sync.Mutex
	return func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		_ = appendEvent(path, ev)
	}
}

func appendEvent(path string, ev Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// Load reads all events from Path(projectRoot). Malformed lines are skipped.
// A missing file is not an error; it returns an empty slice.
func Load(projectRoot string) ([]Event, error) {
	f, err := os.Open(Path(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}
