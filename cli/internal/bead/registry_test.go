package bead

import (
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRegistryShipsBeadsAndArchive(t *testing.T) {
	r := DefaultRegistry()
	_, beadsOK := r.Lookup(CollectionID(DefaultCollection))
	require.True(t, beadsOK, "default registry must register the active beads collection")
	_, archiveOK := r.Lookup(CollectionID(BeadsArchiveCollection))
	require.True(t, archiveOK, "C3 ships the archive partner alongside the active beads collection")
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
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	assert.Equal(t, DefaultCollection, s.Collection)
	assert.Equal(t, filepath.Join(dir, ddxroot.DirName, "beads.jsonl"), s.File)
	assert.Equal(t, filepath.Join(dir, ddxroot.DirName, "beads.lock"), s.LockDir)
}
