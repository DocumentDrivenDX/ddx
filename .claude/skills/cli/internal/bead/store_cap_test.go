package bead

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppendEvent_CapsOversizedBody covers ddx-f8a11202 AC #3: every writer
// appending to bead.events[].body goes through AppendEvent, which enforces
// the per-field cap. A caller passing a 1MB body gets truncation with a
// byte-count marker; the bead line stays well under bd's 65,535-byte field
// limit so `bd import` still accepts it.
func TestAppendEvent_CapsOversizedBody(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &Bead{ID: "ddx-cap-test", Title: "cap test", Priority: 2}
	require.NoError(t, store.Create(b))

	huge := strings.Repeat("A", 1024*1024) // 1MB body
	require.NoError(t, store.AppendEvent("ddx-cap-test", BeadEvent{
		Kind: "test",
		Body: huge,
	}))

	events, err := store.Events("ddx-cap-test")
	require.NoError(t, err)
	require.Len(t, events, 1)

	gotBody := events[0].Body
	assert.LessOrEqual(t, len(gotBody), MaxFieldBytes,
		"AppendEvent must enforce the per-field cap — any path that skips it reintroduces the tracker-corruption surface")
	assert.Contains(t, gotBody, "truncated",
		"truncation must leave a visible marker so downstream consumers know the body was clipped")
	assert.Contains(t, gotBody, "1048576 bytes",
		"marker must cite the original size so operators can correlate with upstream writer logs")
	assert.True(t, strings.HasPrefix(gotBody, "AAAA"),
		"head-heavy truncation — the opening of the rationale is usually the actionable part")
}

// TestAppendEvent_UndersizedBodyUnchanged asserts the cap is a no-op for the
// common case: normal-sized events round-trip verbatim.
func TestAppendEvent_UndersizedBodyUnchanged(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &Bead{ID: "ddx-cap-test", Title: "cap test", Priority: 2}
	require.NoError(t, store.Create(b))

	body := "APPROVE\nAll acceptance clauses met."
	require.NoError(t, store.AppendEvent("ddx-cap-test", BeadEvent{
		Kind: "review",
		Body: body,
	}))

	events, err := store.Events("ddx-cap-test")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, body, events[0].Body,
		"under-cap bodies must never be touched — any silent mutation invalidates audit trails")
}
