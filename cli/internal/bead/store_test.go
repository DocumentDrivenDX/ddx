package bead

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, s.Init())
	return s
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ".ddx"))
	require.NoError(t, s.Init())

	_, err := os.Stat(s.File)
	assert.NoError(t, err, "beads.jsonl should exist after init")
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Fix auth bug", IssueType: "bug", Priority: 1}
	require.NoError(t, s.Create(b))

	assert.NotEmpty(t, b.ID)
	assert.True(t, len(b.ID) > 3, "ID should have prefix + hex")
	assert.Equal(t, "bug", b.IssueType)
	assert.Equal(t, StatusOpen, b.Status)
	assert.Equal(t, 1, b.Priority)
	assert.False(t, b.CreatedAt.IsZero())

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, b.Title, got.Title)
	assert.Equal(t, b.IssueType, got.IssueType)
}

func TestCreateUsesConfiguredPrefix(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  id_prefix: "nif"
`), 0o644))

	s := NewStore(filepath.Join(tempDir, ".ddx"))
	require.NoError(t, s.Init())

	b := &Bead{Title: "Configured prefix"}
	require.NoError(t, s.Create(b))

	assert.True(t, strings.HasPrefix(b.ID, "nif-"))
}

func TestCreateUsesEnvPrefixOverConfig(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  id_prefix: "nif"
`), 0o644))

	t.Setenv("DDX_BEAD_PREFIX", "env")

	s := NewStore(filepath.Join(tempDir, ".ddx"))
	require.NoError(t, s.Init())

	b := &Bead{Title: "Env prefix"}
	require.NoError(t, s.Create(b))

	assert.True(t, strings.HasPrefix(b.ID, "env-"))
}

func TestCreateDefaults(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Simple task"}
	require.NoError(t, s.Create(b))

	assert.Equal(t, DefaultType, b.IssueType)
	assert.Equal(t, DefaultStatus, b.Status)
	assert.Equal(t, 0, b.Priority) // Store does not apply priority defaults; CLI layer sets flag default to 2
	assert.Empty(t, b.Labels)
	assert.Empty(t, b.DepIDs())
}

func TestCreateValidation(t *testing.T) {
	s := newTestStore(t)

	// Empty title
	err := s.Create(&Bead{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")

	// Invalid priority
	err = s.Create(&Bead{Title: "Bad priority", Priority: 9})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "priority")

	// Invalid status
	err = s.Create(&Bead{Title: "Bad status", Status: "invalid"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Original"}
	require.NoError(t, s.Create(b))

	err := s.Update(b.ID, func(b *Bead) {
		b.Title = "Updated"
		b.Status = StatusInProgress
		b.Owner = "me"
	})
	require.NoError(t, err)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Title)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "me", got.Owner)
}

func TestUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Update("nonexistent", func(b *Bead) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClose(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "To close"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Close(b.ID))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
}

func TestListFilters(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.Create(&Bead{Title: "Open task", Labels: []string{"backend"}}))
	b2 := &Bead{Title: "Closed task", Labels: []string{"frontend"}}
	require.NoError(t, s.Create(b2))
	require.NoError(t, s.Close(b2.ID))

	// All
	all, err := s.List("", "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// By status
	open, err := s.List(StatusOpen, "")
	require.NoError(t, err)
	assert.Len(t, open, 1)
	assert.Equal(t, "Open task", open[0].Title)

	// By label
	fe, err := s.List("", "frontend")
	require.NoError(t, err)
	assert.Len(t, fe, 1)
	assert.Equal(t, "Closed task", fe[0].Title)
}

func TestReadyAndBlocked(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "First"}
	b := &Bead{Title: "Second"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.DepAdd(b.ID, a.ID))

	// B is blocked by A
	ready, err := s.Ready()
	require.NoError(t, err)
	assert.Len(t, ready, 1)
	assert.Equal(t, a.ID, ready[0].ID)

	blocked, err := s.Blocked()
	require.NoError(t, err)
	assert.Len(t, blocked, 1)
	assert.Equal(t, b.ID, blocked[0].ID)

	// Close A, B becomes ready
	require.NoError(t, s.Close(a.ID))

	ready, err = s.Ready()
	require.NoError(t, err)
	assert.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID)

	blocked, err = s.Blocked()
	require.NoError(t, err)
	assert.Len(t, blocked, 0)
}

func TestDepAddValidation(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	require.NoError(t, s.Create(a))

	// Dep on nonexistent
	err := s.DepAdd(a.ID, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency not found")

	// Self-dep
	err = s.DepAdd(a.ID, a.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot depend on self")

	// Idempotent add
	b := &Bead{Title: "B"}
	require.NoError(t, s.Create(b))
	require.NoError(t, s.DepAdd(b.ID, a.ID))
	require.NoError(t, s.DepAdd(b.ID, a.ID)) // no error on duplicate
}

func TestDepRemove(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	b := &Bead{Title: "B"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.DepAdd(b.ID, a.ID))

	got, _ := s.Get(b.ID)
	assert.Contains(t, got.DepIDs(), a.ID)

	require.NoError(t, s.DepRemove(b.ID, a.ID))
	got, _ = s.Get(b.ID)
	assert.NotContains(t, got.DepIDs(), a.ID)
}

func TestDepTree(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "Root task"}
	b := &Bead{Title: "Child task"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.DepAdd(b.ID, a.ID))

	tree, err := s.DepTree("")
	require.NoError(t, err)
	assert.Contains(t, tree, "Root task")
	assert.Contains(t, tree, "Child task")
}

func TestStatusCounts(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	b := &Bead{Title: "B"}
	c := &Bead{Title: "C"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Create(c))
	require.NoError(t, s.DepAdd(c.ID, a.ID))
	require.NoError(t, s.Close(b.ID))

	counts, err := s.Status()
	require.NoError(t, err)
	assert.Equal(t, 3, counts.Total)
	assert.Equal(t, 2, counts.Open)
	assert.Equal(t, 1, counts.Closed)
	assert.Equal(t, 1, counts.Ready)   // A is ready (no deps)
	assert.Equal(t, 1, counts.Blocked) // C is blocked by A
}

func TestUnknownFieldPreservation(t *testing.T) {
	s := newTestStore(t)

	// Write a bead with unknown fields directly
	jsonl := `{"id":"hx-test1234","title":"HELIX bead","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"spec-id":"FEAT-001","execution-eligible":true,"claimed-at":"2026-01-01T00:00:00Z","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(s.File, []byte(jsonl+"\n"), 0o644))

	// Read back
	beads, err := s.ReadAll()
	require.NoError(t, err)
	require.Len(t, beads, 1)

	b := beads[0]
	assert.Equal(t, "hx-test1234", b.ID)
	assert.Equal(t, "HELIX bead", b.Title)
	assert.Equal(t, "FEAT-001", b.Extra["spec-id"])
	assert.Equal(t, true, b.Extra["execution-eligible"])
	assert.Equal(t, "2026-01-01T00:00:00Z", b.Extra["claimed-at"])

	// Write back and verify round-trip
	require.NoError(t, s.WriteAll(beads))
	beads2, err := s.ReadAll()
	require.NoError(t, err)
	require.Len(t, beads2, 1)
	assert.Equal(t, "FEAT-001", beads2[0].Extra["spec-id"])
	assert.Equal(t, true, beads2[0].Extra["execution-eligible"])
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReadEmptyStore(t *testing.T) {
	s := newTestStore(t)
	beads, err := s.ReadAll()
	require.NoError(t, err)
	assert.Nil(t, beads)
}

func TestReadNonexistentFile(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nope"))
	beads, err := s.ReadAll()
	require.NoError(t, err)
	assert.Nil(t, beads)
}
