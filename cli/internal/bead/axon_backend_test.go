package bead

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead/axon"
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
	beads  map[string]axon.Bead
	events map[string][]axon.BeadEvent
}

func newFakeAxonGraphQLTransport() *fakeAxonGraphQLTransport {
	return &fakeAxonGraphQLTransport{
		beads:  make(map[string]axon.Bead),
		events: make(map[string][]axon.BeadEvent),
	}
}

func (t *fakeAxonGraphQLTransport) Query(_ context.Context, query string, variables map[string]any, response any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch resp := response.(type) {
	case nil:
		return nil
	case *axon.GetBeadResponse:
		id, _ := variables["id"].(string)
		if bead, ok := t.beads[id]; ok {
			copy := bead
			resp.DDXBead = &copy
		}
		return nil
	case *axon.ListBeadsResponse:
		beads := t.sortedBeadsLocked()
		resp.DDXBeads = append(resp.DDXBeads[:0], beads...)
		return nil
	case *axon.CreateBeadResponse:
		input, err := decodeAxonValue[axon.BeadInput](variables["input"])
		if err != nil {
			return err
		}
		bead := t.createBeadLocked(input)
		resp.CreateEntity = &bead
		return nil
	case *axon.UpdateBeadResponse:
		id, _ := variables["id"].(string)
		expectedVersion, _ := variables["expectedVersion"].(int)
		input, err := decodeAxonValue[axon.BeadInput](variables["input"])
		if err != nil {
			return err
		}
		bead, err := t.updateBeadLocked(id, expectedVersion, input)
		if err != nil {
			return err
		}
		resp.UpdateEntity = &bead
		return nil
	case *axon.ListBeadEventsResponse:
		resp.DDXBeadEvents = append(resp.DDXBeadEvents[:0], t.sortedEventsLocked()...)
		return nil
	case *axon.CreateBeadEventResponse:
		input, err := decodeAxonValue[axon.BeadEvent](variables["input"])
		if err != nil {
			return err
		}
		event := t.createEventLocked(input)
		resp.CreateEntity = &event
		return nil
	case *axon.CreateLinkResponse:
		fromID, _ := variables["from"].(string)
		toID, _ := variables["to"].(string)
		t.addDependencyLocked(fromID, toID)
		resp.CreateLink = &axon.LinkResult{OK: true}
		return nil
	case *axon.DeleteLinkResponse:
		fromID, _ := variables["from"].(string)
		toID, _ := variables["to"].(string)
		t.removeDependencyLocked(fromID, toID)
		resp.DeleteLink = &axon.LinkResult{OK: true}
		return nil
	default:
		return fmt.Errorf("unexpected response type %T for query %q", response, query)
	}
}

func decodeJSONValue(src any, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func decodeAxonValue[T any](src any) (T, error) {
	var dst T
	if err := decodeJSONValue(src, &dst); err != nil {
		return dst, err
	}
	return dst, nil
}

func (t *fakeAxonGraphQLTransport) snapshot() ([]axon.Bead, []axon.BeadEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	beads := t.sortedBeadsLocked()
	events := t.sortedEventsLocked()
	return beads, events
}

func (t *fakeAxonGraphQLTransport) sortedBeadsLocked() []axon.Bead {
	out := make([]axon.Bead, 0, len(t.beads))
	for _, bead := range t.beads {
		copy := bead
		copy.Dependencies = append([]axon.Dependency(nil), bead.Dependencies...)
		copy.Labels = append([]string(nil), bead.Labels...)
		copy.Extra = cloneStringAnyMap(bead.Extra)
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (t *fakeAxonGraphQLTransport) sortedEventsLocked() []axon.BeadEvent {
	out := make([]axon.BeadEvent, 0)
	for _, events := range t.events {
		for _, ev := range events {
			copy := ev
			out = append(out, copy)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventOf != out[j].EventOf {
			return out[i].EventOf < out[j].EventOf
		}
		if out[i].Index != out[j].Index {
			return out[i].Index < out[j].Index
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (t *fakeAxonGraphQLTransport) createBeadLocked(input axon.BeadInput) axon.Bead {
	bead := axon.Bead{
		Version:      1,
		ID:           input.ID,
		Title:        input.Title,
		Status:       input.Status,
		Priority:     input.Priority,
		IssueType:    input.IssueType,
		Owner:        input.Owner,
		CreatedAt:    input.CreatedAt,
		CreatedBy:    input.CreatedBy,
		UpdatedAt:    input.UpdatedAt,
		Labels:       append([]string(nil), input.Labels...),
		Parent:       input.Parent,
		Description:  input.Description,
		Acceptance:   input.Acceptance,
		Notes:        input.Notes,
		Dependencies: nil,
		Extra:        cloneStringAnyMap(input.Extra),
	}
	t.beads[bead.ID] = bead
	return bead
}

func (t *fakeAxonGraphQLTransport) updateBeadLocked(id string, expectedVersion int, input axon.BeadInput) (axon.Bead, error) {
	current, ok := t.beads[id]
	if !ok {
		return axon.Bead{}, fmt.Errorf("bead %s not found", id)
	}
	if expectedVersion != 0 && current.Version != expectedVersion {
		return axon.Bead{}, fmt.Errorf("version mismatch for %s", id)
	}
	current.Version++
	current.Title = input.Title
	current.Status = input.Status
	current.Priority = input.Priority
	current.IssueType = input.IssueType
	current.Owner = input.Owner
	current.CreatedAt = input.CreatedAt
	current.CreatedBy = input.CreatedBy
	current.UpdatedAt = input.UpdatedAt
	current.Labels = append([]string(nil), input.Labels...)
	current.Parent = input.Parent
	current.Description = input.Description
	current.Acceptance = input.Acceptance
	current.Notes = input.Notes
	current.Extra = cloneStringAnyMap(input.Extra)
	// Dependencies are maintained through explicit link mutations.
	current.Dependencies = append([]axon.Dependency(nil), current.Dependencies...)
	t.beads[id] = current
	return current, nil
}

func (t *fakeAxonGraphQLTransport) createEventLocked(input axon.BeadEvent) axon.BeadEvent {
	ev := input
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	ev.Index = len(t.events[ev.EventOf])
	events := append(t.events[ev.EventOf], ev)
	events[len(events)-1] = ev
	t.events[ev.EventOf] = events
	return ev
}

func (t *fakeAxonGraphQLTransport) addDependencyLocked(fromID, toID string) {
	bead, ok := t.beads[fromID]
	if !ok {
		return
	}
	for _, dep := range bead.Dependencies {
		if dep.DependsOnID == toID {
			return
		}
	}
	bead.Dependencies = append(bead.Dependencies, axon.Dependency{
		IssueID:     fromID,
		DependsOnID: toID,
		Type:        "blocks",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	})
	t.beads[fromID] = bead
}

func (t *fakeAxonGraphQLTransport) removeDependencyLocked(fromID, toID string) {
	bead, ok := t.beads[fromID]
	if !ok {
		return
	}
	filtered := bead.Dependencies[:0]
	for _, dep := range bead.Dependencies {
		if dep.DependsOnID != toID {
			filtered = append(filtered, dep)
		}
	}
	bead.Dependencies = append([]axon.Dependency(nil), filtered...)
	t.beads[fromID] = bead
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

	plain := NewAxonBackend(dir, 3*time.Second)
	assert.Nil(t, plain.GraphQLTransport)
	assert.Nil(t, plain.GraphQLClient)
}

func TestAxonBackend_InitCreatesRootDir(t *testing.T) {
	t.Parallel()
	s := newAxonStore(t)
	ax, ok := s.backend.(*AxonBackend)
	require.True(t, ok)

	_, err := os.Stat(ax.Dir)
	assert.NoError(t, err, "axon root directory should exist after Init")
	entries, err := os.ReadDir(ax.Dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasSuffix(entry.Name(), ".jsonl"), "Init must not create JSONL snapshots")
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
	for i, row := range eventRows {
		assert.Equal(t, b.ID, row.EventOf)
		assert.Equal(t, i, row.Index)
		assert.Equal(t, fmt.Sprintf("event-%d", i), row.Summary)
	}
	if beads[0].Extra != nil {
		_, hasEvents := beads[0].Extra["events"]
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
	require.NoError(t, src.ExportTo(testCtx(), &buf))
	require.NotZero(t, buf.Len())

	// Write the export to a temp file and import into a fresh axon-backed
	// store. The expected count should equal the source corpus.
	jsonlPath := filepath.Join(t.TempDir(), "exported.jsonl")
	require.NoError(t, os.WriteFile(jsonlPath, buf.Bytes(), 0o644))

	dst := newAxonStore(t)
	count, err := dst.Import(testCtx(), "jsonl", jsonlPath)
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

// TestAxonBackend_WriteAllUsesGraphQLTransport proves WriteAll mutates the
// GraphQL transport snapshot rather than materialising local JSONL files.
func TestAxonBackend_WriteAllUsesGraphQLTransport(t *testing.T) {
	t.Parallel()
	s, transport := newAxonStoreWithTransport(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, s.Create(testCtx(), &Bead{Title: fmt.Sprintf("b-%d", i)}))
	}

	ax := s.backend.(*AxonBackend)
	beads, eventRows := transport.snapshot()
	require.Len(t, beads, 5)
	assert.Empty(t, eventRows)

	// No JSONL snapshot files should be present after a quiescent WriteAll cycle.
	entries, err := os.ReadDir(ax.Dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".jsonl"), "unexpected snapshot file %s", e.Name())
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
	transport.mu.Lock()
	transport.events["ddx-deadbeef"] = append(transport.events["ddx-deadbeef"], axon.BeadEvent{
		EventOf:   "ddx-deadbeef",
		Index:     0,
		Kind:      "orphan",
		Summary:   "orphaned",
		CreatedAt: time.Now().UTC(),
	})
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
