package bead

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAxonStore returns a *Store wired to a fresh AxonBackend rooted at a
// temp dir. Mirrors newTestStore so the existing high-level Backend method
// surface (Create/Get/Update/Claim/Close/...) gets exercised against the
// axon-shaped storage layout instead of the JSONL default.
func newAxonStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".ddx")
	s := NewStore(dir)
	s.backend = NewAxonBackend(dir, s.LockWait)
	require.NoError(t, s.Init())
	return s
}

func TestAxonBackend_InitCreatesCollections(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)
	ax, ok := s.backend.(*AxonBackend)
	require.True(t, ok)

	for _, path := range []string{ax.BeadsFile, ax.EventsFile} {
		_, err := os.Stat(path)
		assert.NoError(t, err, "axon collection file %s should exist after Init", path)
	}
}

func TestAxonBackend_CreateAndGet(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "first", Description: "round-trip"}
	require.NoError(t, s.Create(b))
	require.NotEmpty(t, b.ID)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Title)
	assert.Equal(t, "round-trip", got.Description)
	assert.Equal(t, StatusOpen, got.Status)
}

func TestAxonBackend_UpdateMutatesAndPersists(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "to-update"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Update(b.ID, func(bb *Bead) {
		bb.Notes = "added by update"
		bb.Priority = 3
	}))

	// Re-read through a fresh Store wired to the same on-disk dir to prove
	// the change survives the in-memory closure.
	s2 := NewStore(s.Dir)
	s2.backend = NewAxonBackend(s.Dir, s.LockWait)
	got, err := s2.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, "added by update", got.Notes)
	assert.Equal(t, 3, got.Priority)
}

func TestAxonBackend_ClaimAndUnclaim(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "claimable"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Claim(b.ID, "alice"))
	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "alice", got.Owner)
	require.NotNil(t, got.Extra)
	assert.NotEmpty(t, got.Extra["claimed-at"])

	// A second claim against an active claim must fail.
	err = s.Claim(b.ID, "bob")
	require.Error(t, err)

	require.NoError(t, s.Unclaim(b.ID))
	got, err = s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

func TestAxonBackend_DepAddRemoveAndTree(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	root := &Bead{Title: "root"}
	child := &Bead{Title: "child"}
	require.NoError(t, s.Create(root))
	require.NoError(t, s.Create(child))

	require.NoError(t, s.DepAdd(child.ID, root.ID))
	got, err := s.Get(child.ID)
	require.NoError(t, err)
	require.Len(t, got.Dependencies, 1)
	assert.Equal(t, root.ID, got.Dependencies[0].DependsOnID)

	tree, err := s.DepTree(root.ID)
	require.NoError(t, err)
	assert.Contains(t, tree, root.ID)

	require.NoError(t, s.DepRemove(child.ID, root.ID))
	got, err = s.Get(child.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Dependencies)
}

func TestAxonBackend_AppendEventSplitsIntoEventsCollection(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "with-events"}
	require.NoError(t, s.Create(b))

	for i := 0; i < 3; i++ {
		require.NoError(t, s.AppendEvent(b.ID, BeadEvent{
			Kind:    "test",
			Summary: fmt.Sprintf("event-%d", i),
			Body:    fmt.Sprintf("body-%d", i),
			Actor:   "tester",
		}))
	}

	// Events round-trip through ReadAll / Events.
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 3)
	for i, e := range events {
		assert.Equal(t, fmt.Sprintf("event-%d", i), e.Summary)
	}

	// And the on-disk event collection actually carries the split entries
	// (TD-030 D3): one entity per event, linked back via event_of.
	ax := s.backend.(*AxonBackend)
	data, err := os.ReadFile(ax.EventsFile)
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	require.Len(t, lines, 3)
	for _, line := range lines {
		var env axonEventEnvelope
		require.NoError(t, json.Unmarshal(line, &env))
		assert.Equal(t, AxonEventsCollection, env.Collection)
		assert.Equal(t, b.ID, env.EventOf)
	}

	// And the bead row in ddx_beads must NOT carry inline events — events
	// live in their own collection per TD-030 D3.
	beadData, err := os.ReadFile(ax.BeadsFile)
	require.NoError(t, err)
	beadLines := bytes.Split(bytes.TrimSpace(beadData), []byte("\n"))
	require.Len(t, beadLines, 1)
	var env axonEntityEnvelope
	require.NoError(t, json.Unmarshal(beadLines[0], &env))
	assert.NotContains(t, string(env.Data), `"events"`,
		"bead row in ddx_beads must not carry inline events array")
}

func TestAxonBackend_ListReadyBlocked(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	a := &Bead{Title: "a"}
	b := &Bead{Title: "b"}
	c := &Bead{Title: "c"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Create(c))
	// b depends on a; c depends on a closed-bead-to-be.
	require.NoError(t, s.DepAdd(b.ID, a.ID))

	all, err := s.List("", "", nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	ready, err := s.Ready()
	require.NoError(t, err)
	readyIDs := beadIDSet(ready)
	assert.Contains(t, readyIDs, a.ID, "a has no deps")
	assert.Contains(t, readyIDs, c.ID, "c has no deps")
	assert.NotContains(t, readyIDs, b.ID, "b depends on open a")

	blocked, err := s.Blocked()
	require.NoError(t, err)
	blockedIDs := beadIDSet(blocked)
	assert.Contains(t, blockedIDs, b.ID)
}

func TestAxonBackend_CloseAndArchivePolicy(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "to-close"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "before-close", Summary: "x"}))
	require.NoError(t, s.Close(b.ID))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status, "closed beads stay in ddx_beads (TD-030 archive policy)")

	// Per TD-030 §"Archive policy under axon", closed beads remain in the
	// same ddx_beads collection; the events stay accessible via Events()
	// (the Store externalises them to the attachment sidecar on close).
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

// TestAxonBackend_JSONLImportExportRoundTrip verifies the bd/br interchange
// contract: a bead exported through the axon-backed store and re-imported
// re-creates the same bead corpus. This is the AC §"JSONL export/import for
// bd/br round-trip" knot.
func TestAxonBackend_JSONLImportExportRoundTrip(t *testing.T) {
	t.Parallel()
	src := newAxonStore(t)

	// Seed a small corpus with deps + events.
	a := &Bead{Title: "alpha"}
	bb := &Bead{Title: "beta"}
	require.NoError(t, src.Create(a))
	require.NoError(t, src.Create(bb))
	require.NoError(t, src.DepAdd(bb.ID, a.ID))
	require.NoError(t, src.AppendEvent(a.ID, BeadEvent{Kind: "k", Summary: "s"}))

	var buf bytes.Buffer
	require.NoError(t, src.ExportTo(&buf))
	require.NotZero(t, buf.Len())

	// Write the export to a temp file and import into a fresh axon-backed
	// store. The expected count should equal the source corpus.
	jsonlPath := filepath.Join(t.TempDir(), "exported.jsonl")
	require.NoError(t, os.WriteFile(jsonlPath, buf.Bytes(), 0o644))

	dst := newAxonStore(t)
	count, err := dst.Import("jsonl", jsonlPath)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	gotA, err := dst.Get(a.ID)
	require.NoError(t, err)
	assert.Equal(t, "alpha", gotA.Title)

	gotB, err := dst.Get(bb.ID)
	require.NoError(t, err)
	assert.Equal(t, "beta", gotB.Title)
	require.Len(t, gotB.Dependencies, 1)
	assert.Equal(t, a.ID, gotB.Dependencies[0].DependsOnID)
}

// TestAxonBackend_AtomicWriteSnapshotIntact proves WriteAll uses temp+rename
// (writeAtomicFile) and does not leak partial state between writes.
func TestAxonBackend_AtomicWriteSnapshotIntact(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, s.Create(&Bead{Title: fmt.Sprintf("b-%d", i)}))
	}

	ax := s.backend.(*AxonBackend)
	// No temp files should be visible after a quiescent WriteAll cycle.
	entries, err := os.ReadDir(ax.Dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), AxonBeadsCollection+".jsonl.tmp-"),
			"unexpected temp file %s", e.Name())
		assert.False(t, strings.HasPrefix(e.Name(), AxonEventsCollection+".jsonl.tmp-"),
			"unexpected temp file %s", e.Name())
	}

	all, err := s.List("", "", nil)
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

// TestAxonBackend_ConcurrentClaimsSerialised reproduces the chaos contract
// that two claimers must not both succeed: WithLock + writeAtomicFile must
// keep the read-modify-write race correct under the axon backend.
func TestAxonBackend_ConcurrentClaimsSerialised(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "race"}
	require.NoError(t, s.Create(b))

	const goroutines = 8
	var (
		wg       sync.WaitGroup
		successN int
		mu       sync.Mutex
	)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.Claim(b.ID, fmt.Sprintf("worker-%d", i))
			if err == nil {
				mu.Lock()
				successN++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 1, successN, "exactly one claim must win the race")

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
}

// TestAxonBackend_SelectableViaConfigKnobWithFlag proves AC §3: the backend
// is selected by beads.backend=axon (via DDX_BEAD_BACKEND) only when the
// experimental flag is set. Without the flag, beads.backend=axon must fall
// through to the JSONL default and not silently corrupt the workspace.
func TestAxonBackend_SelectableViaConfigKnobWithFlag(t *testing.T) {
	t.Setenv("DDX_BEAD_BACKEND", BackendAxon)
	t.Setenv(AxonExperimentalEnv, "1")

	dir := filepath.Join(t.TempDir(), ".ddx")
	s := NewStore(dir)
	_, ok := s.backend.(*AxonBackend)
	assert.True(t, ok, "DDX_BEAD_BACKEND=axon plus the experimental flag must select AxonBackend")
}

func TestAxonBackend_ConfigKnobIgnoredWithoutFlag(t *testing.T) {
	t.Setenv("DDX_BEAD_BACKEND", BackendAxon)
	t.Setenv(AxonExperimentalEnv, "")

	dir := filepath.Join(t.TempDir(), ".ddx")
	s := NewStore(dir)
	assert.Nil(t, s.backend, "without the feature flag, beads.backend=axon must fall through to the built-in JSONL path")
}

func TestAxonExperimentalEnabledTruthyValues(t *testing.T) {
	for _, val := range []string{"1", "true", "TRUE", "Yes", "on"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv(AxonExperimentalEnv, val)
			assert.True(t, AxonExperimentalEnabled(), "%q should enable the flag", val)
		})
	}
	for _, val := range []string{"", "0", "false", "no", "off", "garbage"} {
		t.Run("disabled-"+val, func(t *testing.T) {
			t.Setenv(AxonExperimentalEnv, val)
			assert.False(t, AxonExperimentalEnabled(), "%q should not enable the flag", val)
		})
	}
}

// TestAxonBackend_OrphanEventsDropped verifies that an event entity whose
// event_of points at a non-existent bead is silently dropped on read. This
// is the partial-write recovery story documented in WriteAll.
func TestAxonBackend_OrphanEventsDropped(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "live"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "k", Summary: "live-event"}))

	// Manually append an orphan event for a bead that does not exist.
	ax := s.backend.(*AxonBackend)
	orphan := axonEventEnvelope{
		Collection:    AxonEventsCollection,
		SchemaVersion: axonSchemaVersion,
		EventOf:       "ddx-deadbeef",
		Index:         0,
		Event:         BeadEvent{Kind: "orphan", Summary: "orphaned", CreatedAt: time.Now().UTC()},
	}
	row, err := json.Marshal(orphan)
	require.NoError(t, err)
	f, err := os.OpenFile(ax.EventsFile, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.Write(append(row, '\n'))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Live bead's events still readable; orphan does not surface.
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "live-event", events[0].Summary)

	// And there is no phantom bead with the orphan id.
	_, err = s.Get("ddx-deadbeef")
	assert.Error(t, err)
}

func beadIDSet(beads []Bead) map[string]bool {
	out := make(map[string]bool, len(beads))
	for _, b := range beads {
		out[b.ID] = true
	}
	return out
}
