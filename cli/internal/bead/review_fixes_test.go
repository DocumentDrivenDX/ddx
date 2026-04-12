package bead

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Finding #1: Update core validation ──────────────────────────────

func TestUpdateRejectsInvalidStatus(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Valid"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Status = "garbage"
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestUpdateRejectsEmptyTitle(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Valid"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Title = ""
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestUpdateRejectsInvalidPriority(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Valid"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Priority = 99
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "priority")
}

func TestUpdateValidMutationSucceeds(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{Title: "Valid"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Title = "Updated"
		b.Status = StatusInProgress
	})
	assert.NoError(t, err)

	got, _ := s.Get(b.ID)
	assert.Equal(t, "Updated", got.Title)
	assert.Equal(t, StatusInProgress, got.Status)
}

// ── Finding #3: Duplicate ID rejection ──────────────────────────────

func TestCreateRejectsDuplicateID(t *testing.T) {
	s := newTestStore(t)

	b1 := &Bead{Title: "First"}
	require.NoError(t, s.Create(b1))

	b2 := &Bead{ID: b1.ID, Title: "Duplicate"}
	err := s.Create(b2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate id")
}

// ── Finding #5: Hooks see post-default state ────────────────────────

func TestCreateHookSeesDefaults(t *testing.T) {
	s := newTestStore(t)

	// Hook that checks Type and Status are populated
	hookDir := filepath.Join(s.Dir, "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	// Hook reads stdin JSON; fails if type is empty
	hookScript := `#!/bin/sh
INPUT=$(cat)
TYPE=$(echo "$INPUT" | grep -o '"type":"[^"]*"' | head -1)
if echo "$TYPE" | grep -q '"type":""'; then
  echo "type is empty" >&2
  exit 1
fi
exit 0`
	require.NoError(t, os.WriteFile(
		filepath.Join(hookDir, "validate-bead-create"),
		[]byte(hookScript), 0o755))

	// Create without explicit type — defaults should be applied before hook
	b := &Bead{Title: "Hook test"}
	err := s.Create(b)
	assert.NoError(t, err)
	assert.Equal(t, DefaultType, b.IssueType)
}

// ── Finding #7: Import error reporting ──────────────────────────────

func TestImportAllMalformedReportsError(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "bad.jsonl")
	require.NoError(t, os.WriteFile(importFile, []byte("not json\nalso bad\n"), 0o644))

	_, err := s.Import("jsonl", importFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed")
}

// ── Finding #8: Export uses writer ──────────────────────────────────

func TestExportToWriter(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Create(&Bead{Title: "Export test"}))

	var buf bytes.Buffer
	require.NoError(t, s.ExportTo(&buf))

	assert.Contains(t, buf.String(), "Export test")
	assert.Contains(t, buf.String(), `"title"`)
}

// ── Finding #2: Import validates status/priority ────────────────────

func TestImportNormalizesInvalidStatus(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	jsonl := `{"id":"bx-bad00001","title":"Bad status","type":"task","status":"garbage","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := s.Get("bx-bad00001")
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status) // normalized
}

func TestImportClampsPriority(t *testing.T) {
	s := newTestStore(t)

	importFile := filepath.Join(t.TempDir(), "import.jsonl")
	jsonl := `{"id":"bx-pri00001","title":"High pri","type":"task","status":"open","priority":99,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(importFile, []byte(jsonl), 0o644))

	n, err := s.Import("jsonl", importFile)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := s.Get("bx-pri00001")
	require.NoError(t, err)
	assert.Equal(t, MaxPriority, got.Priority) // clamped
}
