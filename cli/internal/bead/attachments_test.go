package bead

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttachmentClosingExternalizesEvents asserts that closing a bead moves
// its inline event history into a sidecar attachment under
// .ddx/attachments/<id>/events.jsonl and clears the inline copy from the row.
//
// Acceptance: AC1 (closed beads write events to the attachment store, not
// inline) and AC3 (bead-row size shrinks).
func TestAttachmentClosingExternalizesEvents(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "to close"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "review", Summary: "APPROVE", Body: "looks good"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "summary", Summary: "completed"}))

	// Capture the row size before close so we can demonstrate AC3 shrinkage.
	beforeRowBytes := len(beadRowBytes(t, s, b.ID))

	require.NoError(t, s.Close(b.ID))

	// Sidecar exists.
	attPath := s.eventsAttachmentPath(b.ID)
	info, err := os.Stat(attPath)
	require.NoError(t, err, "events.jsonl sidecar should exist after close")
	assert.True(t, info.Size() > 0)

	// The on-disk row no longer carries inline events; it carries an
	// attachment ref instead.
	rowBytes := beadRowBytes(t, s, b.ID)
	assert.NotContains(t, string(rowBytes), `"events":`)
	assert.Contains(t, string(rowBytes), `"`+EventsAttachmentExtraKey+`":`)

	// AC3: the row shrank.
	assert.Less(t, len(rowBytes), beforeRowBytes,
		"closed bead row should be smaller after externalization")

	// Sidecar carries both events with their bodies intact.
	events, err := s.readEventsAttachment(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "review", events[0].Kind)
	assert.Equal(t, "APPROVE", events[0].Summary)
	assert.Equal(t, "looks good", events[0].Body)
	assert.Equal(t, "summary", events[1].Kind)
}

// TestAttachmentShowReadsTransparently exercises AC2: callers read events
// through the normal Events() API regardless of where they live, and the
// `--json` show projection inlines events into the marshaled object.
func TestAttachmentShowReadsTransparently(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "show me"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "model=opus"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "review", Summary: "APPROVE", Body: "rationale"}))
	require.NoError(t, s.Close(b.ID))

	// Events() returns the externalized log transparently.
	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "routing", events[0].Kind)
	assert.Equal(t, "review", events[1].Kind)

	// LoadEventsInline replays the events into Extra["events"] so the JSON
	// show path projects a single uniform shape.
	got, err := s.Get(b.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NoError(t, s.LoadEventsInline(got))
	raw, ok := got.Extra["events"]
	require.True(t, ok, "LoadEventsInline must populate Extra[events]")
	inlined := decodeBeadEvents(raw)
	require.Len(t, inlined, 2)
	_, attachStillSet := got.Extra[EventsAttachmentExtraKey]
	assert.False(t, attachStillSet, "in-memory inlining drops the attachment ref")

	// On-disk row was not modified by the inline operation.
	rowBytes := beadRowBytes(t, s, b.ID)
	assert.NotContains(t, string(rowBytes), `"events":`)
	assert.Contains(t, string(rowBytes), `"`+EventsAttachmentExtraKey+`":`)
}

// TestAttachmentExportInlinesEvents covers AC4: export emits a single inline
// `events` array per row so bd/br interchange survives.
func TestAttachmentExportInlinesEvents(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "export me"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "model=opus"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "review", Summary: "APPROVE", Body: "ok"}))
	require.NoError(t, s.Close(b.ID))

	var buf bytes.Buffer
	require.NoError(t, s.ExportTo(&buf))

	out := strings.TrimSpace(buf.String())
	require.NotEmpty(t, out)

	var row map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &row))

	events, ok := row["events"].([]any)
	require.True(t, ok, "export must include inline events array; got %v", row["events"])
	require.Len(t, events, 2)
	first := events[0].(map[string]any)
	assert.Equal(t, "routing", first["kind"])

	// The attachment ref must NOT appear in exported JSON — bd/br doesn't
	// know how to resolve it and would otherwise see a dangling pointer.
	_, refStillThere := row[EventsAttachmentExtraKey]
	assert.False(t, refStillThere, "export must drop the attachment ref")
}

// TestExportStreamsArchiveAndAttachmentEvents asserts that after `ddx bead
// migrate` moves closed beads into the beads-archive collection (with their
// events externalized to the attachment store), `ddx bead export` still
// streams a single JSONL document covering both the active and archived
// rows, with archived beads' events inlined back into the row so bd/br
// importers see no dangling attachment refs (ADR-004 compatibility guard #3
// for AC2/AC3/AC4).
func TestExportStreamsArchiveAndAttachmentEvents(t *testing.T) {
	s := newTestStore(t)

	// Active open bead with no events — it must survive export.
	open := &Bead{Title: "still open"}
	require.NoError(t, s.Create(open))

	// A closed bead with inline events. Migrate will externalize then archive it.
	closed := &Bead{Title: "to archive"}
	require.NoError(t, s.Create(closed))
	require.NoError(t, s.AppendEvent(closed.ID, BeadEvent{Kind: "routing", Summary: "model=opus"}))
	require.NoError(t, s.AppendEvent(closed.ID, BeadEvent{Kind: "review", Summary: "APPROVE", Body: "ship"}))
	require.NoError(t, s.Close(closed.ID))

	stats, err := s.Migrate()
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Archived, 1, "migrate should archive the closed bead")

	// Sanity: the closed bead is gone from the active collection.
	activeBeads, err := s.ReadAll()
	require.NoError(t, err)
	for _, b := range activeBeads {
		assert.NotEqual(t, closed.ID, b.ID, "archived bead must not remain in active store")
	}

	var buf bytes.Buffer
	require.NoError(t, s.ExportTo(&buf))

	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 2, "export must include both the active and archived bead")

	rowsByID := make(map[string]map[string]any, 2)
	for _, line := range lines {
		var row map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		id, _ := row["id"].(string)
		rowsByID[id] = row
	}

	openRow, ok := rowsByID[open.ID]
	require.True(t, ok, "active open bead must be in export")
	assert.Equal(t, "open", openRow["status"])

	archivedRow, ok := rowsByID[closed.ID]
	require.True(t, ok, "archived bead must be streamed inline alongside active rows")
	assert.Equal(t, "closed", archivedRow["status"])

	// Events must be inlined back into the row — bd/br round-trip relies
	// on this. The attachment ref must NOT survive the export.
	events, ok := archivedRow["events"].([]any)
	require.True(t, ok, "archived row must carry inline events; got %v", archivedRow["events"])
	require.Len(t, events, 2)
	assert.Equal(t, "routing", events[0].(map[string]any)["kind"])
	assert.Equal(t, "review", events[1].(map[string]any)["kind"])
	_, refStillThere := archivedRow[EventsAttachmentExtraKey]
	assert.False(t, refStillThere, "export must drop the attachment ref")

	// Round-trip: the export stream is consumable as bd-format JSONL —
	// every line parses as a bead record carrying bd-compatible fields.
	for _, line := range lines {
		b, uerr := unmarshalBead([]byte(line))
		require.NoError(t, uerr, "exported line must parse as a bd-compatible bead")
		assert.NotEmpty(t, b.ID)
		assert.NotEmpty(t, b.Status)
	}
}

// TestAttachmentAppendEventAfterCloseAppendsToSidecar covers a subtle case:
// once a bead is closed the inline path is gone, so a follow-up AppendEvent
// must extend the sidecar rather than re-introducing inline storage.
func TestAttachmentAppendEventAfterCloseAppendsToSidecar(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "post-close evt"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "first"}))
	require.NoError(t, s.Close(b.ID))

	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "audit", Summary: "post-close"}))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	_, hasInline := got.Extra["events"]
	assert.False(t, hasInline, "post-close append must not re-inline events")

	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "audit", events[1].Kind)
	assert.Equal(t, "post-close", events[1].Summary)
}

// TestAttachmentReopenInlinesEvents exercises Reopen's promise: a reopened
// bead returns to the active queue with its full event history back inline,
// the attachment ref cleared, and a fresh "reopen" event appended.
func TestAttachmentReopenInlinesEvents(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "reopen me"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "routing", Summary: "first"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "review", Summary: "APPROVE", Body: "ok"}))
	require.NoError(t, s.Close(b.ID))

	// Sanity: closed bead has the attachment.
	require.FileExists(t, s.eventsAttachmentPath(b.ID))

	require.NoError(t, s.Reopen(b.ID, "needs more work", ""))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, StatusOpen, got.Status)
	_, attachStillSet := got.Extra[EventsAttachmentExtraKey]
	assert.False(t, attachStillSet, "reopen must drop the attachment ref")

	events, err := s.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 3, "two prior events plus the reopen event")
	assert.Equal(t, "reopen", events[2].Kind)
	assert.Equal(t, "needs more work", events[2].Summary)

	// The sidecar file is removed once events are inlined back.
	_, statErr := os.Stat(s.eventsAttachmentPath(b.ID))
	assert.True(t, os.IsNotExist(statErr),
		"sidecar should be deleted after reopen inlines events")
}

// TestAttachmentClosureGateAcceptsAttachedEvents guards against the gate
// rejecting a follow-up close on a bead whose events live in the sidecar
// (the gate's existing check only inspected Extra[events]).
func TestAttachmentClosureGateAcceptsAttachedEvents(t *testing.T) {
	b := &Bead{
		ID: "ddx-gate-att",
		Extra: map[string]any{
			EventsAttachmentExtraKey: "ddx-gate-att/events.jsonl",
			"closing_commit_sha":     "deadbeef",
		},
	}
	// No inline events, but the attachment ref + closing commit together
	// must satisfy the gate (the inline-only path would otherwise reject).
	assert.NoError(t, ClosureGate(b))
}

// beadRowBytes returns the raw JSONL line for the given bead ID from the
// store's primary file, so tests can compare row sizes before/after
// externalization (AC3).
func beadRowBytes(t *testing.T, s *Store, id string) []byte {
	t.Helper()
	data, err := os.ReadFile(s.File)
	require.NoError(t, err)
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if rid, _ := m["id"].(string); rid == id {
			return line
		}
	}
	t.Fatalf("bead %s not found in %s", id, s.File)
	return nil
}
