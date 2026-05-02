package bead

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRegistryShipsBeadsAndArchive(t *testing.T) {
	r := DefaultRegistry()
	ids := r.IDs()
	require.Equal(t,
		[]CollectionID{CollectionID(DefaultCollection), CollectionID(BeadsArchiveCollection)},
		ids,
		"C3 ships the archive partner alongside the active beads collection")
}

func TestRegistryResolveUsesRegisteredSpec(t *testing.T) {
	r := DefaultRegistry()
	spec := r.Resolve(CollectionID(DefaultCollection))
	assert.Equal(t, "beads.jsonl", spec.FileName())
	assert.Equal(t, "beads.lock", spec.LockName())
}

func TestRegistryResolveSynthesizesUnknownCollection(t *testing.T) {
	r := NewRegistry()
	spec := r.Resolve(CollectionID("exec-runs"))
	assert.Equal(t, "exec-runs.jsonl", spec.FileName())
	assert.Equal(t, "exec-runs.lock", spec.LockName())
}

func TestRegistryRegisterOverridesDefaults(t *testing.T) {
	r := NewRegistry()
	r.Register(CollectionSpec{
		ID:          CollectionID("custom"),
		JSONLFile:   "alt.jsonl",
		LockDirName: "alt.lock",
	})
	spec, ok := r.Lookup(CollectionID("custom"))
	require.True(t, ok)
	assert.Equal(t, "alt.jsonl", spec.FileName())
	assert.Equal(t, "alt.lock", spec.LockName())
}

func TestStoreUsesRegistrySpecForBeads(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ".ddx"))
	assert.Equal(t, DefaultCollection, s.Collection)
	assert.Equal(t, filepath.Join(dir, ".ddx", "beads.jsonl"), s.File)
	assert.Equal(t, filepath.Join(dir, ".ddx", "beads.lock"), s.LockDir)
}
