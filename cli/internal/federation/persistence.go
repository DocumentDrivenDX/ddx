package federation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// stateFileName is the on-disk filename, under XDG_DATA_HOME / ~/.local/share.
const stateFileName = "federation-state.json"

// fileMode is mode 0600: registry may carry spoke URLs and identity, so it is
// owner-readable only (FEAT-026 NFR).
const fileMode os.FileMode = 0o600

// dirMode for the parent directory.
const dirMode os.FileMode = 0o700

// writeMu serializes concurrent SaveStateTo calls within one process so
// tmpfile+rename does not race against itself. (Cross-process safety relies
// on rename atomicity.)
var writeMu sync.Mutex

// DefaultStatePath returns ~/.local/share/ddx/federation-state.json.
// Honors XDG_DATA_HOME when set, otherwise falls back to ~/.local/share.
func DefaultStatePath() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ddx", stateFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("federation: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "ddx", stateFileName), nil
}

// LoadState reads the registry from the default path.
func LoadState() (*FederationRegistry, error) {
	path, err := DefaultStatePath()
	if err != nil {
		return nil, err
	}
	return LoadStateFrom(path)
}

// SaveState writes the registry to the default path.
func SaveState(r *FederationRegistry) error {
	path, err := DefaultStatePath()
	if err != nil {
		return err
	}
	return SaveStateTo(path, r)
}

// LoadStateFrom reads the registry from path. Behavior:
//
//   - Missing file       → fresh empty registry stamped with CurrentSchemaVersion.
//   - Malformed JSON     → fresh empty registry; the corrupt file is renamed
//     to "<path>.corrupt-<ts>" so an operator can recover it.
//   - Unknown future     → loaded as-is; caller can inspect SchemaVersion and
//     schema_version       decide whether to refuse to write.
func LoadStateFrom(path string) (*FederationRegistry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewRegistry(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("federation: read state: %w", err)
	}

	var reg FederationRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		// Quarantine the corrupt file so it's not silently overwritten on the
		// next save, then return a fresh registry. This is the recovery path
		// the bead's "malformed-file recovery" AC asks for.
		_ = quarantineCorrupt(path)
		return NewRegistry(), nil
	}

	if reg.SchemaVersion == "" {
		// Pre-versioned files are treated as v1; future migrations would key
		// off the read SchemaVersion before this defaulting.
		reg.SchemaVersion = CurrentSchemaVersion
	}
	if reg.Spokes == nil {
		reg.Spokes = []SpokeRecord{}
	}
	return &reg, nil
}

// SaveStateTo writes the registry to path atomically: marshal → write tmpfile
// in the same directory → fsync → rename onto the target. The rename is
// atomic on POSIX filesystems, so a partially-written file is never visible.
func SaveStateTo(path string, r *FederationRegistry) error {
	if r == nil {
		return fmt.Errorf("federation: nil registry")
	}
	if r.SchemaVersion == "" {
		r.SchemaVersion = CurrentSchemaVersion
	}
	if r.Spokes == nil {
		r.Spokes = []SpokeRecord{}
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("federation: mkdir: %w", err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("federation: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".federation-state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("federation: create tmpfile: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("federation: write tmpfile: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("federation: fsync tmpfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("federation: close tmpfile: %w", err)
	}
	if err := os.Chmod(tmpPath, fileMode); err != nil {
		cleanup()
		return fmt.Errorf("federation: chmod tmpfile: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("federation: rename: %w", err)
	}
	return nil
}

func quarantineCorrupt(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return err
	}
	bak := fmt.Sprintf("%s.corrupt-%d", path, info.ModTime().UnixNano())
	return os.Rename(path, bak)
}
