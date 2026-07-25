package runrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
func Publish(projectRoot string, rec Record) error {
	return publish(projectRoot, rec, nil)
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

func publish(projectRoot string, rec Record, h *publishHooks) error {
	if projectRoot == "" {
		return fmt.Errorf("runrecord: empty project root")
	}
	if rec.RunID == "" {
		return fmt.Errorf("runrecord: empty run_id")
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
