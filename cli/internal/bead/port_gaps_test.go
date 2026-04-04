package bead

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Claim and Unclaim ────────────────────────────────────────────

func TestClaimRecordsMetadata(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Claimable"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Claim(b.ID, "agent-1"))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "agent-1", got.Owner)
	assert.NotEmpty(t, got.Extra["claimed-at"])
	assert.NotEmpty(t, got.Extra["claimed-pid"])
}

func TestUnclaimClearsMetadata(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "To unclaim"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Claim(b.ID, "agent-1"))

	require.NoError(t, s.Unclaim(b.ID))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Nil(t, got.Extra["claimed-at"])
	assert.Nil(t, got.Extra["claimed-pid"])
}

func TestInProgressNotReady(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Claimed"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Claim(b.ID, "agent"))

	ready, err := s.Ready()
	require.NoError(t, err)
	assert.Len(t, ready, 0) // in_progress should not be in ready queue
}

// ── Import cycle detection ──────────────────────────────────────

func TestImportRejectsCircularDeps(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "cycle.jsonl")
	jsonl := `{"id":"bx-cyc001","title":"A","issue_type":"task","status":"open","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"issue_id":"bx-cyc001","depends_on_id":"bx-cyc002","type":"blocks"}]}
{"id":"bx-cyc002","title":"B","issue_type":"task","status":"open","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"issue_id":"bx-cyc002","depends_on_id":"bx-cyc001","type":"blocks"}]}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	_, err := s.Import("jsonl", importFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestImportNoCycleSucceeds(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "dag.jsonl")
	jsonl := `{"id":"bx-dag001","title":"Root","issue_type":"task","status":"open","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"id":"bx-dag002","title":"Child","issue_type":"task","status":"open","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","dependencies":[{"issue_id":"bx-dag002","depends_on_id":"bx-dag001","type":"blocks"}]}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

// ── Export error handling ────────────────────────────────────────

type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n > 1 {
		return 0, os.ErrClosed
	}
	return len(p), nil
}

func TestExportToChecksWriteErrors(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Create(&Bead{Title: "A"}))
	require.NoError(t, s.Create(&Bead{Title: "B"}))

	err := s.ExportTo(&failWriter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "export write")
}

// ── Export round-trip field preservation ─────────────────────────

func TestExportRoundTripPreservesFields(t *testing.T) {
	s := newTestStore(t)

	orig := &Bead{
		Title:       "Full bead",
		IssueType:        "bug",
		Priority:    1,
		Labels:      []string{"backend", "urgent"},
		Description: "A real bug",
		Acceptance:  "Tests pass",
	}
	require.NoError(t, s.Create(orig))

	// Export
	var buf bytes.Buffer
	require.NoError(t, s.ExportTo(&buf))

	// Import into fresh store
	s2 := newTestStore(t)
	importFile := filepath.Join(t.TempDir(), "roundtrip.jsonl")
	require.NoError(t, os.WriteFile(importFile, buf.Bytes(), 0o644))
	n, err := s2.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := s2.Get(orig.ID)
	require.NoError(t, err)
	assert.Equal(t, orig.Title, got.Title)
	assert.Equal(t, orig.IssueType, got.IssueType)
	assert.Equal(t, orig.Priority, got.Priority)
	assert.Equal(t, orig.Labels, got.Labels)
	assert.Equal(t, orig.Description, got.Description)
	assert.Equal(t, orig.Acceptance, got.Acceptance)
}

// ── Structural field updates via Extra ──────────────────────────

func TestUpdateStructuralFields(t *testing.T) {
	s := newTestStore(t)

	// Create a bead with HELIX-style metadata
	jsonl := `{"id":"hx-struct01","title":"Build auth","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"spec-id":"FEAT-001","parent":"hx-epic01","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(s.File, []byte(jsonl+"\n"), 0o644))

	// Update a structural field via Extra
	err := s.Update("hx-struct01", func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["spec-id"] = "FEAT-002"
		b.Parent = "hx-epic02"
	})
	require.NoError(t, err)

	got, err := s.Get("hx-struct01")
	require.NoError(t, err)
	assert.Equal(t, "FEAT-002", got.Extra["spec-id"])
	assert.Equal(t, "hx-epic02", got.Parent)
}
