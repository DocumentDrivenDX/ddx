package bead

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"
)

// JSONLBackend stores beads as newline-delimited JSON in a single file under
// a .ddx directory. It is the storage shape used by Store's built-in path and
// is also the fallback ExternalBackend reaches for when bd/br cannot serve a
// non-default collection (e.g. "beads-archive"). Constructed directly only
// when a RawBackend value is needed standalone — Store's inline path predates
// this type and continues to operate on the same file format.
type JSONLBackend struct {
	Dir      string
	File     string
	LockDir  string
	LockWait time.Duration
}

// NewJSONLBackend constructs a JSONL-backed Backend rooted at dir. file and
// lockDir must be absolute (or rooted under dir) and are written/locked
// directly. lockWait bounds how long WithLock spins before giving up.
// Compile-time check: JSONLBackend satisfies RawBackend.
var _ RawBackend = (*JSONLBackend)(nil)
var _ OperationApplier = (*JSONLBackend)(nil)

func NewJSONLBackend(dir, file, lockDir string, lockWait time.Duration) *JSONLBackend {
	return &JSONLBackend{Dir: dir, File: file, LockDir: lockDir, LockWait: lockWait}
}

func (j *JSONLBackend) Init() error {
	if err := os.MkdirAll(j.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: init dir: %w", err)
	}
	f, err := os.OpenFile(j.File, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("bead: init file: %w", err)
	}
	return f.Close()
}

func (j *JSONLBackend) ReadAll() ([]Bead, error) {
	data, err := os.ReadFile(j.File)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: read %s: %w", j.File, err)
	}
	beads, _, err := parseBeadJSONL(data)
	if err != nil {
		return nil, fmt.Errorf("bead: parse %s: %w", j.File, err)
	}
	return foldLatestBeads(beads), nil
}

func (j *JSONLBackend) WriteAll(beads []Bead) error {
	sort.Slice(beads, func(a, b int) bool { return beads[a].ID < beads[b].ID })
	if err := os.MkdirAll(j.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: mkdir: %w", err)
	}
	var buf []byte
	for _, b := range beads {
		data, err := marshalBead(b)
		if err != nil {
			return fmt.Errorf("bead: marshal: %w", err)
		}
		buf = append(buf, data...)
		buf = append(buf, '\n')
	}
	return writeAtomicFile(j.File, buf)
}

// Apply mutates one bead snapshot in place and persists the updated corpus.
// JSONLBackend keeps the generic load-mutate-save path here so higher-level
// callers can opt into the OperationApplier fast path without having to know
// the backend-specific write shape yet.
func (j *JSONLBackend) Apply(ctx context.Context, id string, op Operation) error {
	if j == nil {
		return fmt.Errorf("bead: nil jsonl backend")
	}
	if op == nil {
		return fmt.Errorf("bead: nil operation")
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return j.WithLock(func() error {
		beads, err := j.ReadAll()
		if err != nil {
			return err
		}
		found := false
		now := time.Now().UTC()
		for i := range beads {
			if beads[i].ID != id {
				continue
			}
			origID := beads[i].ID
			origCreatedAt := beads[i].CreatedAt
			if err := op.Apply(&beads[i]); err != nil {
				return err
			}
			if beads[i].ID != origID || !beads[i].CreatedAt.Equal(origCreatedAt) {
				return fmt.Errorf("bead: operation may not mutate immutable fields")
			}
			beads[i].UpdatedAt = now
			found = true
			break
		}
		if !found {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return j.WriteAll(beads)
	})
}

func (j *JSONLBackend) WithLock(fn func() error) error {
	wait := j.LockWait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	if err := acquireDirLock(j.Dir, j.LockDir, wait); err != nil {
		return err
	}
	defer os.RemoveAll(j.LockDir)
	return fn()
}
