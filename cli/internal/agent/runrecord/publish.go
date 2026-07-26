package runrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

// errPublishKilled is returned when a test-injected fault aborts publish mid-flight.
// Production Publish never returns this error.
var errPublishKilled = errors.New("runrecord: publish killed (test fault)")

// publishFaultPhase names a step in the atomic publish sequence. Tests inject
// interruption at each phase to prove readers never observe partial JSON.
type publishFaultPhase int

const (
	faultAfterTempCreate publishFaultPhase = iota
	faultAfterPartialWrite
	faultAfterFullWrite
	faultAfterFsyncFile
	faultAfterRename
	faultAfterFsyncDir
)

// publishHooks is test-only instrumentation. When faultAt is set, publish aborts
// at that phase without cleanup (crash-like), leaving on-disk state as a kill
// would. onPhase observes ordered discipline steps when non-nil.
type publishHooks struct {
	faultAt *publishFaultPhase
	onPhase func(phase publishFaultPhase, recordPath, tmpPath string)
	// killed is set when a fault aborts so the defer does not unlink temp debris
	// (a real crash would not clean it either).
	killed bool
}

func (h *publishHooks) maybeFault(phase publishFaultPhase, recordPath, tmpPath string) error {
	if h == nil {
		return nil
	}
	if h.onPhase != nil {
		h.onPhase(phase, recordPath, tmpPath)
	}
	if h.faultAt != nil && *h.faultAt == phase {
		h.killed = true
		return errPublishKilled
	}
	return nil
}

// publishTestHooks routes Publish (and thus TransitionToRunning /
// TransitionToTerminal) through the same fault/observation path as the
// package-private publish used by pre-dispatch tests. Production code never
// sets this; only *_test.go helpers install hooks. Tests that install hooks
// must not run in parallel with each other.
var (
	publishTestHooksMu sync.Mutex
	publishTestHooks   *publishHooks
)

// RecordPath returns the absolute path of the durable record for runID under
// projectRoot: <projectRoot>/.ddx/runs/<run-id>/record.json.
func RecordPath(projectRoot, runID string) string {
	return filepath.Join(projectRoot, StoreDir, runID, RecordFileName)
}

// RunDir returns the absolute run directory for runID under projectRoot.
func RunDir(projectRoot, runID string) string {
	return filepath.Join(projectRoot, StoreDir, runID)
}

// Publish writes rec to .ddx/runs/<run-id>/record.json using crash-safe
// discipline: create temp in the target directory, write JSON, fsync the temp
// file, rename over record.json, fsync the parent directory.
//
// After any interruption, a reader of record.json observes either the previous
// complete record or the next complete record — never torn or partial JSON.
// Publish encodes only the typed Record schema; provider raw output, PIDs,
// process-tree metadata, and provider-session canonical state are not fields
// on Record and therefore cannot be persisted.
//
// Running and terminal phase updates call Publish, so they share this atomic
// writer contract with the pre-dispatch publisher.
func Publish(projectRoot string, rec Record) error {
	// Snapshot optional test hooks (nil in production). The brief lock is a
	// no-op when no test has installed hooks.
	publishTestHooksMu.Lock()
	h := publishTestHooks
	publishTestHooksMu.Unlock()
	return publish(projectRoot, rec, h)
}

// Read loads the durable record for runID, or returns (nil, nil) when absent.
func Read(projectRoot, runID string) (*Record, error) {
	path := RecordPath(projectRoot, runID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("runrecord: read %s: %w", path, err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("runrecord: parse %s: %w", path, err)
	}
	return &rec, nil
}

// validateRunID rejects empty and path-unsafe run identifiers before any
// directory or file is created. A run ID must be a single path segment (the
// directory name under .ddx/runs/), not an absolute path, not "."/"..", and
// not contain separators, null bytes, or cleaned-path escapes.
func validateRunID(runID string) error {
	if runID == "" || strings.TrimSpace(runID) == "" {
		return fmt.Errorf("runrecord: empty run_id")
	}
	if strings.TrimSpace(runID) != runID {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if !utf8.ValidString(runID) {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if strings.ContainsRune(runID, 0) {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if runID == "." || runID == ".." {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if filepath.IsAbs(runID) {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	// Reject OS separators and the alternate slash so callers cannot escape
	// .ddx/runs/ on either Unix or Windows path semantics.
	if strings.ContainsAny(runID, `/\`) {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if filepath.Base(runID) != runID {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	if cleaned := filepath.Clean(runID); cleaned != runID {
		return fmt.Errorf("runrecord: path-unsafe run_id %q", runID)
	}
	return nil
}

func publish(projectRoot string, rec Record, h *publishHooks) error {
	if projectRoot == "" {
		return fmt.Errorf("runrecord: empty project root")
	}
	if err := validateRunID(rec.RunID); err != nil {
		return err
	}
	// Pre-route boundary: a dispatching-phase record is published before Fizeau
	// route/result data exists. Reject concrete routing, result, cost, and
	// provider-process claims so DDx never persists knowledge it does not have.
	if rec.Phase == PhaseDispatching {
		if err := validateNoPreRouteFields(rec); err != nil {
			return err
		}
		// Normalize empty Fizeau pointer so JSON omits the block entirely.
		if rec.Fizeau != nil && rec.Fizeau.IsEmpty() {
			rec.Fizeau = nil
		}
	}
	if rec.Version == 0 {
		rec.Version = SchemaVersion
	}

	dir := RunDir(projectRoot, rec.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runrecord: mkdir %s: %w", dir, err)
	}
	finalPath := filepath.Join(dir, RecordFileName)

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("runrecord: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "."+RecordFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("runrecord: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		_ = tmp.Close()
		// On a simulated kill, leave temp debris (if any) like a crash would.
		// After a successful rename the temp path no longer exists.
		if !renamed && (h == nil || !h.killed) {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := h.maybeFault(faultAfterTempCreate, finalPath, tmpPath); err != nil {
		return err
	}

	// Partial-write kill: leave incomplete JSON only on the temp path so
	// record.json (if present) remains the prior complete document.
	if h != nil && h.faultAt != nil && *h.faultAt == faultAfterPartialWrite {
		half := len(data) / 2
		if half == 0 {
			half = len(data)
		}
		if _, err := tmp.Write(data[:half]); err != nil {
			return fmt.Errorf("runrecord: partial write temp: %w", err)
		}
		if h.onPhase != nil {
			h.onPhase(faultAfterPartialWrite, finalPath, tmpPath)
		}
		h.killed = true
		return errPublishKilled
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("runrecord: write temp: %w", err)
	}
	if err := h.maybeFault(faultAfterFullWrite, finalPath, tmpPath); err != nil {
		return err
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("runrecord: fsync temp: %w", err)
	}
	if err := h.maybeFault(faultAfterFsyncFile, finalPath, tmpPath); err != nil {
		return err
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("runrecord: close temp: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("runrecord: rename: %w", err)
	}
	renamed = true

	if err := h.maybeFault(faultAfterRename, finalPath, tmpPath); err != nil {
		return err
	}

	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("runrecord: fsync dir: %w", err)
	}
	if err := h.maybeFault(faultAfterFsyncDir, finalPath, tmpPath); err != nil {
		return err
	}
	return nil
}

// fsyncDir durability-flushes directory metadata so a rename is durable after
// power loss on platforms that support directory fsync.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		// Windows (and some network FS) do not support fsync on directories.
		// File fsync + rename still prevent torn record.json visibility on
		// process crash; power-loss durability of the directory entry is
		// best-effort there.
		if runtime.GOOS == "windows" {
			return nil
		}
		return err
	}
	return nil
}
