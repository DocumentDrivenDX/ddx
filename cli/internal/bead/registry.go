package bead

import (
	"path/filepath"
	"sort"
	"sync"
)

// CollectionID is the logical name of a bead-backed collection.
// "beads" is the active queue; future collections (e.g. "beads-archive",
// "exec-runs", "agent-sessions") share the same on-disk format and store
// API but live under their own files and locks.
type CollectionID string

// CollectionSpec is the shipping description of one collection. It maps a
// logical name to the concrete on-disk artifacts the JSONL backend uses.
//
// Per TD-027 §(a), the registry is an in-process table seeded at startup;
// it is not user-editable and DDx does not auto-discover collections from
// disk. Additional fields from TD-027 (QueueSemantics, ArchivePartner,
// Attachments) are deliberately omitted at this stage — TD-027 C3 will
// introduce them when archive logic lands. This bead (C2) is scoped to the
// storage-engine refactor: registry shape plus path/lock decoupling.
type CollectionSpec struct {
	// ID is the logical collection name, e.g. "beads".
	ID CollectionID
	// JSONLFile is the file name (not a full path) under the .ddx
	// directory that the JSONL backend reads and writes. Empty means
	// "<id>.jsonl".
	JSONLFile string
	// LockDirName is the lock directory name under the .ddx directory.
	// Empty means "<id>.lock".
	LockDirName string
}

// FileName returns the JSONL filename for this collection, falling back to
// the conventional "<id>.jsonl" form when JSONLFile is unset.
func (c CollectionSpec) FileName() string {
	if c.JSONLFile != "" {
		return c.JSONLFile
	}
	return string(c.ID) + ".jsonl"
}

// LockName returns the lock-directory name for this collection, falling
// back to the conventional "<id>.lock" form when LockDirName is unset.
func (c CollectionSpec) LockName() string {
	if c.LockDirName != "" {
		return c.LockDirName
	}
	return string(c.ID) + ".lock"
}

// PathsUnder returns the absolute file and lock paths for this collection
// rooted at the given .ddx directory.
func (c CollectionSpec) PathsUnder(ddxDir string) (file, lockDir string) {
	return filepath.Join(ddxDir, c.FileName()), filepath.Join(ddxDir, c.LockName())
}

// Registry is a table of known collection specs keyed by ID. Lookups for
// IDs that are not registered are not an error: NewStore synthesizes a
// conventional spec on the fly so existing call sites that use ad-hoc
// collection names (e.g. test fixtures) keep working. The registry is the
// authoritative source for the shipping collections that DDx itself
// instantiates.
type Registry struct {
	mu    sync.RWMutex
	specs map[CollectionID]CollectionSpec
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{specs: make(map[CollectionID]CollectionSpec)}
}

// Register adds or replaces a spec.
func (r *Registry) Register(spec CollectionSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[spec.ID] = spec
}

// Lookup returns the spec for id and whether it was registered. Callers
// that want a usable spec for any name should prefer Resolve.
func (r *Registry) Lookup(id CollectionID) (CollectionSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[id]
	return spec, ok
}

// Resolve returns the spec for id, synthesizing a conventional one (with
// "<id>.jsonl" / "<id>.lock") when id is not registered. This keeps the
// JSONL backend's per-collection layout uniform whether or not a name is
// in the shipping table.
func (r *Registry) Resolve(id CollectionID) CollectionSpec {
	if spec, ok := r.Lookup(id); ok {
		return spec
	}
	return CollectionSpec{ID: id}
}

// IDs returns the registered collection IDs in lexical order.
func (r *Registry) IDs() []CollectionID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CollectionID, 0, len(r.specs))
	for id := range r.specs {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// defaultRegistry is the process-wide registry. It ships with the active
// "beads" collection and its archive partner "beads-archive" (TD-027 §a).
// Additional collections (exec-runs, agent-sessions) will be registered by
// their owning beads.
var defaultRegistry = func() *Registry {
	r := NewRegistry()
	r.Register(CollectionSpec{
		ID:          DefaultCollection,
		JSONLFile:   DefaultCollection + ".jsonl",
		LockDirName: DefaultCollection + ".lock",
	})
	r.Register(CollectionSpec{
		ID:          BeadsArchiveCollection,
		JSONLFile:   BeadsArchiveCollection + ".jsonl",
		LockDirName: BeadsArchiveCollection + ".lock",
	})
	return r
}()

// DefaultRegistry returns the process-wide collection registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}
