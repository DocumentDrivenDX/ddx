package bead

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAxonStore returns a *Store wired to a fresh AxonBackend rooted at a
// temp dir. Mirrors newTestStore so the existing high-level Backend method
// surface (Create/Get/Update/Claim/Close/...) gets exercised against the
// axon-shaped storage layout instead of the JSONL default.
func newAxonStore(t *testing.T) *Store {
	s, _ := newAxonStoreWithTransport(t)
	return s
}

func newAxonStoreWithTransport(t *testing.T) (*Store, *fakeAxonGraphQLTransport) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	transport := newFakeAxonGraphQLTransport()
	s := NewStore(dir)
	ax := NewAxonBackend(dir, s.LockWait)
	ax.GraphQLTransport = transport
	ax.GraphQLClient = transport
	s.backend = ax
	require.NoError(t, s.Init(testCtx()))
	return s, transport
}

type fakeAxonGraphQLTransport struct {
	mu     sync.Mutex
	beads  []axonEntityEnvelope
	events []axonEventEnvelope
}

func newFakeAxonGraphQLTransport() *fakeAxonGraphQLTransport {
	return &fakeAxonGraphQLTransport{}
}

func (t *fakeAxonGraphQLTransport) Query(_ context.Context, query string, variables map[string]any, response any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch query {
	case axonReadCorpusQuery:
		if response == nil {
			return nil
		}
		resp, ok := response.(*axonCorpusResponse)
		if !ok {
			return fmt.Errorf("unexpected response type %T", response)
		}
		resp.Beads = append(resp.Beads[:0], t.beads...)
		resp.Events = append(resp.Events[:0], t.events...)
		return nil
	case axonWriteCorpusMutation:
		var beads []axonEntityEnvelope
		var events []axonEventEnvelope
		if raw, ok := variables["beads"]; ok {
			if err := decodeJSONValue(raw, &beads); err != nil {
				return err
			}
		}
		if raw, ok := variables["events"]; ok {
			if err := decodeJSONValue(raw, &events); err != nil {
				return err
			}
		}
		t.beads = append(t.beads[:0], beads...)
		t.events = append(t.events[:0], events...)
		if response != nil {
			if resp, ok := response.(*axonWriteCorpusResponse); ok {
				resp.SaveCorpus.OK = true
			}
		}
		return nil
	default:
		return fmt.Errorf("unexpected query %q", query)
	}
}

func decodeJSONValue(src any, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func (t *fakeAxonGraphQLTransport) snapshot() ([]axonEntityEnvelope, []axonEventEnvelope) {
	t.mu.Lock()
	defer t.mu.Unlock()
	beads := append([]axonEntityEnvelope(nil), t.beads...)
	events := append([]axonEventEnvelope(nil), t.events...)
	return beads, events
}

type stubAxonGraphQLTransport struct{}

func (stubAxonGraphQLTransport) Query(context.Context, string, map[string]any, any) error {
	return nil
}

func TestAxonBackend_GraphQLClientBoundary(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ddxroot.DirName)
	transport := stubAxonGraphQLTransport{}
	client := struct{ Name string }{Name: "stub"}

	ax := NewAxonBackend(dir, 3*time.Second)
	ax.GraphQLTransport = transport
	ax.GraphQLClient = client

	assert.IsType(t, transport, ax.GraphQLTransport)
	assert.Equal(t, transport, ax.GraphQLTransport)
	require.Equal(t, client, ax.GraphQLClient)
	assert.Equal(t, filepath.Join(dir, AxonDirName), ax.Dir)
	assert.Equal(t, filepath.Join(dir, AxonDirName, AxonBeadsCollection+".jsonl"), ax.BeadsFile)
	assert.Equal(t, filepath.Join(dir, AxonDirName, AxonEventsCollection+".jsonl"), ax.EventsFile)

	plain := NewAxonBackend(dir, 3*time.Second)
	assert.Nil(t, plain.GraphQLTransport)
	assert.Nil(t, plain.GraphQLClient)
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

func TestAxonBackend_GraphQLMapping_BeadsAndEvents(t *testing.T) {
	t.Parallel()

	local := Bead{
		ID:          "ddx-00000001",
		Title:       "mapped",
		Status:      StatusOpen,
		Priority:    2,
		IssueType:   DefaultType,
		Owner:       "owner",
		CreatedAt:   time.Unix(10, 0).UTC(),
		CreatedBy:   "creator",
		UpdatedAt:   time.Unix(20, 0).UTC(),
		Labels:      []string{"kind:feature"},
		Parent:      "ddx-00000002",
		Description: "local model",
		Acceptance:  "AC",
		Notes:       "notes",
		Dependencies: []Dependency{{
			IssueID:     "ddx-00000001",
			DependsOnID: "ddx-00000002",
			Type:        "blocks",
			CreatedAt:   "2026-05-04T00:00:00Z",
			CreatedBy:   "creator",
			Metadata:    "meta",
		}},
		Extra: map[string]any{
			"source": "test",
			"events": []any{
				map[string]any{"kind": "created", "summary": "one"},
				map[string]any{"kind": "updated", "summary": "two"},
			},
		},
	}

	beadRow, err := axonEncodeBead(beadWithoutInlineEvents(local))
	require.NoError(t, err)

	var beadEnv axonEntityEnvelope
	require.NoError(t, json.Unmarshal(beadRow, &beadEnv))
	assert.Equal(t, AxonBeadsCollection, beadEnv.Collection)
	assert.Equal(t, axonSchemaVersion, beadEnv.SchemaVersion)
	require.NotNil(t, beadEnv.Data.Extra)
	_, hasEvents := beadEnv.Data.Extra["events"]
	assert.False(t, hasEvents, "bead rows must not inline events")
	assert.Equal(t, local.ID, beadEnv.Data.ID)
	assert.Equal(t, local.Title, beadEnv.Data.Title)
	require.Len(t, beadEnv.Data.Dependencies, 1)
	assert.Equal(t, local.Dependencies[0].DependsOnID, beadEnv.Data.Dependencies[0].DependsOnID)

	eventRow, err := axonEncodeEvent(local.ID, 1, BeadEvent{
		Kind:      "updated",
		Summary:   "two",
		Body:      "body",
		Actor:     "tester",
		CreatedAt: time.Unix(30, 0).UTC(),
	})
	require.NoError(t, err)

	var eventEnv axonEventEnvelope
	require.NoError(t, json.Unmarshal(eventRow, &eventEnv))
	assert.Equal(t, AxonEventsCollection, eventEnv.Collection)
	assert.Equal(t, axonSchemaVersion, eventEnv.SchemaVersion)
	assert.Equal(t, local.ID, eventEnv.EventOf)
	assert.Equal(t, 1, eventEnv.Index)
	assert.Equal(t, "updated", eventEnv.Event.Kind)
	assert.Equal(t, "two", eventEnv.Event.Summary)
}

func TestAxonBackend_CreateAndGet(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "first", Description: "round-trip"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NotEmpty(t, b.ID)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Title)
	assert.Equal(t, "round-trip", got.Description)
	assert.Equal(t, StatusOpen, got.Status)
}

func TestAxonBackend_UpdateMutatesAndPersists(t *testing.T) {
	t.Parallel()
	s, transport := newAxonStoreWithTransport(t)

	b := &Bead{Title: "to-update"}
	require.NoError(t, s.Create(testCtx(), b))

	require.NoError(t, s.Update(testCtx(), b.ID, func(bb *Bead) {
		bb.Notes = "added by update"
		bb.Priority = 3
	}))

	// Re-read through a fresh Store wired to the same on-disk dir to prove
	// the change survives the in-memory closure.
	s2 := NewStore(s.Dir)
	ax2 := NewAxonBackend(s.Dir, s.LockWait)
	ax2.GraphQLTransport = transport
	ax2.GraphQLClient = transport
	s2.backend = ax2
	got, err := s2.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, "added by update", got.Notes)
	assert.Equal(t, 3, got.Priority)
}

func TestAxonBackend_ClaimAndUnclaim(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	b := &Bead{Title: "claimable"}
	require.NoError(t, s.Create(testCtx(), b))

	require.NoError(t, s.Claim(b.ID, "alice"))
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "alice", got.Owner)

	// A second claim against an active claim must fail.
	err = s.Claim(b.ID, "bob")
	require.Error(t, err)

	require.NoError(t, s.Unclaim(b.ID))
	got, err = s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
}

func TestAxonBackend_DepAddRemoveAndTree(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	root := &Bead{Title: "root"}
	child := &Bead{Title: "child"}
	require.NoError(t, s.Create(testCtx(), root))
	require.NoError(t, s.Create(testCtx(), child))

	require.NoError(t, s.DepAdd(testCtx(), child.ID, root.ID))
	got, err := s.Get(testCtx(), child.ID)
	require.NoError(t, err)
	require.Len(t, got.Dependencies, 1)
	assert.Equal(t, root.ID, got.Dependencies[0].DependsOnID)

	tree, err := s.DepTree(testCtx(), root.ID)
	require.NoError(t, err)
	assert.Contains(t, tree, root.ID)

	require.NoError(t, s.DepRemove(testCtx(), child.ID, root.ID))
	got, err = s.Get(testCtx(), child.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Dependencies)
}

func TestAxonBackend_AppendEventSplitsIntoEventsCollection(t *testing.T) {
	t.Parallel()
	s, transport := newAxonStoreWithTransport(t)

	b := &Bead{Title: "with-events"}
	require.NoError(t, s.Create(testCtx(), b))

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

	// The GraphQL corpus stores one bead row plus one event entity per
	// appended event, linked back via event_of.
	beads, eventRows := transport.snapshot()
	require.Len(t, beads, 1)
	require.Len(t, eventRows, 3)
	for _, row := range eventRows {
		assert.Equal(t, AxonEventsCollection, row.Collection)
		assert.Equal(t, b.ID, row.EventOf)
	}
	if beads[0].Data.Extra != nil {
		_, hasEvents := beads[0].Data.Extra["events"]
		assert.False(t, hasEvents, "bead row must not inline events in GraphQL storage")
	}
}

func TestAxonBackend_ListReadyBlocked(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)

	a := &Bead{Title: "a"}
	b := &Bead{Title: "b"}
	c := &Bead{Title: "c"}
	blockedStatus := &Bead{Title: "blocked status", Status: StatusBlocked}
	proposedStatus := &Bead{Title: "proposed status", Status: StatusProposed}
	cancelledStatus := &Bead{Title: "cancelled status", Status: StatusCancelled}
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Create(testCtx(), c))
	require.NoError(t, s.Create(testCtx(), blockedStatus))
	require.NoError(t, s.Create(testCtx(), proposedStatus))
	require.NoError(t, s.Create(testCtx(), cancelledStatus))
	// b depends on a; c depends on a closed-bead-to-be.
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID))

	all, err := s.List("", "", nil)
	require.NoError(t, err)
	assert.Len(t, all, 6)
	byID := beadByID(all)
	assert.Equal(t, StatusBlocked, byID[blockedStatus.ID].Status)
	assert.Equal(t, StatusProposed, byID[proposedStatus.ID].Status)
	assert.Equal(t, StatusCancelled, byID[cancelledStatus.ID].Status)

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
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "before-close", Summary: "x"}))
	require.NoError(t, s.Close(testCtx(), b.ID))

	got, err := s.Get(testCtx(), b.ID)
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
	require.NoError(t, src.Create(context.Background(), a))
	require.NoError(t, src.Create(context.Background(), bb))
	require.NoError(t, src.DepAdd(testCtx(), bb.ID, a.ID))
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

	gotA, err := dst.Get(context.Background(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, "alpha", gotA.Title)

	gotB, err := dst.Get(context.Background(), bb.ID)
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
		require.NoError(t, s.Create(testCtx(), &Bead{Title: fmt.Sprintf("b-%d", i)}))
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
	require.NoError(t, s.Create(testCtx(), b))

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

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
}

// TestAxonBackend_OrphanEventsDropped verifies that an event entity whose
// event_of points at a non-existent bead is silently dropped on read. This
// is the partial-write recovery story documented in WriteAll.
func TestAxonBackend_OrphanEventsDropped(t *testing.T) {
	t.Parallel()
	s, transport := newAxonStoreWithTransport(t)

	b := &Bead{Title: "live"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "k", Summary: "live-event"}))

	// Manually append an orphan event for a bead that does not exist.
	orphan := axonEventEnvelope{
		Collection:    AxonEventsCollection,
		SchemaVersion: axonSchemaVersion,
		EventOf:       "ddx-deadbeef",
		Index:         0,
		Event:         BeadEvent{Kind: "orphan", Summary: "orphaned", CreatedAt: time.Now().UTC()},
	}
	transport.mu.Lock()
	transport.events = append(transport.events, orphan)
	transport.mu.Unlock()

	// Live bead's events still readable; orphan does not surface.
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "live-event", events[0].Summary)

	// And there is no phantom bead with the orphan id.
	_, err = s.Get(testCtx(), "ddx-deadbeef")
	assert.Error(t, err)
}

func beadIDSet(beads []Bead) map[string]bool {
	out := make(map[string]bool, len(beads))
	for _, b := range beads {
		out[b.ID] = true
	}
	return out
}

func beadByID(beads []Bead) map[string]Bead {
	out := make(map[string]Bead, len(beads))
	for _, b := range beads {
		out[b.ID] = b
	}
	return out
}
