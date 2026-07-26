// Package scratchowner defines a process-lifetime scratch ownership marker
// that producers (e.g. internal/testutils) and consumers (e.g. internal/agent
// cleanup) can share without an import cycle.
//
// A directory name prefix is never ownership proof. Ownership is claimed only
// by a well-formed marker file. Missing or malformed markers yield unowned or
// uncertain status so host-global cleanup preserves the path rather than
// synthesizing ownership (see ddx-05e3d16b / ddx-1eb1ba93).
package scratchowner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MarkerFileName is the durable ownership marker written into a process-
// lifetime scratch directory.
const MarkerFileName = "scratch-owner.json"

// Well-known resource kinds for process-lifetime binary scratch. Producers
// may use other kind strings; cleanup treats kind as opaque evidence, not a
// trust decision.
const (
	KindFixtureBinary         = "fixture-bin"
	KindFizeauTestSeamBinary  = "fizeau-testseam-bin"
)

// OwnerStatus classifies ownership evidence for a scratch directory.
type OwnerStatus string

const (
	// StatusUnowned means no marker is present. Consistent with host-global
	// ownership rules: absence is not an ownership claim.
	StatusUnowned OwnerStatus = "unowned"
	// StatusUncertain means the marker is missing required fields, malformed,
	// or the owner process identity cannot be validated safely. Callers must
	// preserve the directory.
	StatusUncertain OwnerStatus = "uncertain"
	// StatusLive means a well-formed marker names a process that is still
	// alive under a compatible start identity.
	StatusLive OwnerStatus = "live"
	// StatusDead means a well-formed marker names a process that is gone, or
	// whose start identity conclusively no longer matches (PID reuse).
	StatusDead OwnerStatus = "dead"
)

// Marker is the durable ownership evidence for one process-lifetime scratch
// directory.
type Marker struct {
	// Kind identifies the resource class (e.g. fixture-bin). Required.
	Kind string `json:"kind"`
	// OwnerPID is the producing process. Required and must be > 0.
	OwnerPID int `json:"owner_pid"`
	// CreatedAt is when the marker was written (UTC).
	CreatedAt time.Time `json:"created_at"`
	// ProcessStartIdentity is a platform-specific process-start token when
	// the OS exposes one (e.g. Linux /proc/<pid>/stat starttime). Empty when
	// unavailable. When present, validate against the live process to detect
	// PID reuse.
	ProcessStartIdentity string `json:"process_start_identity,omitempty"`
}

// Path returns the marker file path inside dir.
func Path(dir string) string {
	return filepath.Join(dir, MarkerFileName)
}

// NewMarkerForCurrentProcess builds a marker for the calling process.
func NewMarkerForCurrentProcess(kind string) Marker {
	pid := os.Getpid()
	return Marker{
		Kind:                 strings.TrimSpace(kind),
		OwnerPID:             pid,
		CreatedAt:            time.Now().UTC(),
		ProcessStartIdentity: processStartIdentity(pid),
	}
}

// WriteForCurrentProcess records ownership of dir by the calling process.
// The write is atomic (temp file in dir + rename onto MarkerFileName).
func WriteForCurrentProcess(dir, kind string) (Marker, error) {
	m := NewMarkerForCurrentProcess(kind)
	if err := Write(dir, m); err != nil {
		return Marker{}, err
	}
	return m, nil
}

// Write persists m under dir using temp-file + rename so concurrent readers
// never observe a partial marker. A prefix or directory name is never written
// as ownership proof — only the explicit marker fields are.
func Write(dir string, m Marker) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("scratchowner: dir is empty")
	}
	if err := m.validateShape(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("scratchowner: mkdir: %w", err)
	}

	// Normalize CreatedAt to UTC for stable round-trips.
	if !m.CreatedAt.IsZero() {
		m.CreatedAt = m.CreatedAt.UTC()
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("scratchowner: marshal: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, MarkerFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("scratchowner: create tmp: %w", err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		_ = tmp.Close()
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("scratchowner: write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("scratchowner: close tmp: %w", err)
	}
	if err := os.Rename(tmpName, Path(dir)); err != nil {
		return fmt.Errorf("scratchowner: rename: %w", err)
	}
	renamed = true
	return nil
}

// Read loads the marker from dir. Returns os.ErrNotExist when absent.
// Malformed JSON yields a non-nil error that is not os.ErrNotExist.
func Read(dir string) (Marker, error) {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return Marker{}, err
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, fmt.Errorf("scratchowner: parse marker: %w", err)
	}
	if !m.CreatedAt.IsZero() {
		m.CreatedAt = m.CreatedAt.UTC()
	}
	return m, nil
}

// Evaluate classifies ownership for dir.
//
//   - missing marker → StatusUnowned (no ownership claim)
//   - malformed / incomplete marker → StatusUncertain
//   - well-formed + live owner identity → StatusLive
//   - well-formed + dead or conclusively mismatched identity → StatusDead
//   - well-formed but identity cannot be validated safely → StatusUncertain
//
// Unexpected I/O errors (e.g. permission) are returned as err. Missing and
// parse failures never produce StatusLive.
func Evaluate(dir string) (OwnerStatus, Marker, error) {
	m, err := Read(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusUnowned, Marker{}, nil
		}
		// Unreadable or malformed: not an ownership claim.
		return StatusUncertain, Marker{}, nil
	}
	return Classify(m), m, nil
}

// Classify reports owner status for an already-decoded marker without
// touching the filesystem for the marker itself. It still probes process
// liveness / start identity on the host.
func Classify(m Marker) OwnerStatus {
	if err := m.validateShape(); err != nil {
		return StatusUncertain
	}
	if !processAlive(m.OwnerPID) {
		return StatusDead
	}

	recorded := strings.TrimSpace(m.ProcessStartIdentity)
	if recorded == "" {
		// Platform did not expose a start identity when the marker was
		// written. PID liveness alone is the best available signal.
		return StatusLive
	}

	current := processStartIdentity(m.OwnerPID)
	if current == "" {
		// Process appears alive but we cannot re-read start identity.
		// Preserve rather than reclaim.
		return StatusUncertain
	}
	if current != recorded {
		// PID reuse: different process now occupies the recorded PID.
		return StatusDead
	}
	return StatusLive
}

func (m Marker) validateShape() error {
	if strings.TrimSpace(m.Kind) == "" {
		return errors.New("scratchowner: kind is required")
	}
	if m.OwnerPID <= 0 {
		return errors.New("scratchowner: owner_pid must be > 0")
	}
	if m.CreatedAt.IsZero() {
		return errors.New("scratchowner: created_at is required")
	}
	return nil
}
