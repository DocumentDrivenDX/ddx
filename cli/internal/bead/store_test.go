package bead

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestInitUsesCollectionFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithCollection(filepath.Join(dir, ".ddx"), "exec-runs")
	require.Equal(t, "exec-runs", s.Collection)
	require.Equal(t, filepath.Join(dir, ".ddx", "exec-runs.jsonl"), s.File)
	require.Equal(t, filepath.Join(dir, ".ddx", "exec-runs.lock"), s.LockDir)
	require.NoError(t, s.Init())

	_, err := os.Stat(s.File)
	assert.NoError(t, err, "collection file should exist after init")
}

func TestWithCollectionNormalizesJSONLExtension(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ".ddx"), WithCollection("agent-sessions.jsonl"))
	assert.Equal(t, "agent-sessions", s.Collection)
	assert.Equal(t, filepath.Join(dir, ".ddx", "agent-sessions.jsonl"), s.File)
}

func TestExternalBackendCarriesLogicalCollectionName(t *testing.T) {
	toolDir := t.TempDir()
	writeFakeBackendTool(t, toolDir, "bd")
	writeFakeBackendTool(t, toolDir, "br")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tc := range []struct {
		name       string
		backend    string
		collection string
	}{
		{name: "default-bd", backend: "bd", collection: DefaultCollection},
		{name: "exec-runs-bd", backend: "bd", collection: "exec-runs"},
		{name: "agent-sessions-br", backend: "br", collection: "agent-sessions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DDX_BEAD_BACKEND", tc.backend)
			s := NewStore(filepath.Join(t.TempDir(), ".ddx"), WithCollection(tc.collection))
			backend, ok := s.backend.(*ExternalBackend)
			require.True(t, ok)
			assert.Equal(t, tc.backend, backend.Tool)
			assert.Equal(t, tc.collection, backend.Collection)
		})
	}
}

func TestExternalBackendFallsBackWhenToolMissing(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("DDX_BEAD_BACKEND", "bd")

	s := NewStore(filepath.Join(t.TempDir(), ".ddx"), WithCollection("exec-runs"))
	assert.Nil(t, s.backend)
}

func writeFakeBackendTool(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit 0\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
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
	t.Setenv("DDX_BEAD_PREFIX", "")
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
	all, err := s.List("", "", nil)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// By status
	open, err := s.List(StatusOpen, "", nil)
	require.NoError(t, err)
	assert.Len(t, open, 1)
	assert.Equal(t, "Open task", open[0].Title)

	// By label
	fe, err := s.List("", "frontend", nil)
	require.NoError(t, err)
	assert.Len(t, fe, 1)
	assert.Equal(t, "Closed task", fe[0].Title)
}

func TestListWhereFilter(t *testing.T) {
	s := newTestStore(t)

	b1 := &Bead{Title: "Spec task"}
	require.NoError(t, s.Create(b1))
	// Set spec-id in Extra via Update
	require.NoError(t, s.Update(b1.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["spec-id"] = "FEAT-006"
	}))

	b2 := &Bead{Title: "Other task"}
	require.NoError(t, s.Create(b2))
	require.NoError(t, s.Update(b2.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["spec-id"] = "FEAT-007"
	}))

	// Filter by Extra field
	got, err := s.List("", "", map[string]string{"spec-id": "FEAT-006"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Spec task", got[0].Title)

	// Filter by known field (status)
	got, err = s.List("", "", map[string]string{"status": StatusOpen})
	require.NoError(t, err)
	assert.Len(t, got, 2)

	// Filter by known field with no match
	got, err = s.List("", "", map[string]string{"status": StatusClosed})
	require.NoError(t, err)
	assert.Len(t, got, 0)

	// No where filter returns all
	got, err = s.List("", "", nil)
	require.NoError(t, err)
	assert.Len(t, got, 2)
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

func TestUnclaimDoesNotReopenClosedBead(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Create a bead
	b := &Bead{ID: "test-unclaim-001", Title: "Test bead", IssueType: "task", Status: StatusOpen}
	require.NoError(t, s.Create(b))

	// Claim and close it
	require.NoError(t, s.Claim(b.ID, "worker"))
	require.NoError(t, s.Close(b.ID))

	// Verify it's closed
	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)

	// Unclaim should NOT reopen it
	require.NoError(t, s.Unclaim(b.ID))

	got, err = s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status, "unclaim must not reopen a closed bead")
}

// withHeartbeat temporarily overrides HeartbeatInterval and HeartbeatTTL for
// the duration of a test.
func withHeartbeat(t *testing.T, interval, ttl time.Duration) {
	t.Helper()
	origInterval := HeartbeatInterval
	origTTL := HeartbeatTTL
	HeartbeatInterval = interval
	HeartbeatTTL = ttl
	t.Cleanup(func() {
		HeartbeatInterval = origInterval
		HeartbeatTTL = origTTL
	})
}

func TestHeartbeatReclaimStaleInProgressBead(t *testing.T) {
	withHeartbeat(t, 10*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-hb-stale", Title: "Stale claim"}
	require.NoError(t, s.Create(b))

	// First worker claims it normally.
	require.NoError(t, s.Claim(b.ID, "worker-a"))

	// Forge a stale heartbeat by rewriting the store under its lock.
	require.NoError(t, s.Update(b.ID, func(bd *Bead) {
		if bd.Extra == nil {
			bd.Extra = map[string]any{}
		}
		stale := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		bd.Extra["execute-loop-heartbeat-at"] = stale
		bd.Extra["claimed-at"] = stale
	}))

	// A fresh worker must be able to reclaim the stalled bead.
	require.NoError(t, s.Claim(b.ID, "worker-b"))
	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "worker-b", got.Owner)

	// The ready-execution queue must surface the stale bead too.
	s2 := newTestStore(t)
	orig := &Bead{ID: "ddx-hb-stale-2", Title: "Stale claim 2"}
	require.NoError(t, s2.Create(orig))
	require.NoError(t, s2.Claim(orig.ID, "worker-a"))
	require.NoError(t, s2.Update(orig.ID, func(bd *Bead) {
		stale := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		bd.Extra["execute-loop-heartbeat-at"] = stale
	}))
	ready, err := s2.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, orig.ID, ready[0].ID)
}

func TestHeartbeatKeepsActiveClaimAlive(t *testing.T) {
	withHeartbeat(t, 5*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-hb-live", Title: "Live claim"}
	require.NoError(t, s.Create(b))

	require.NoError(t, s.Claim(b.ID, "worker-a"))

	// Actively refresh the heartbeat for longer than the TTL.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.NoError(t, s.Heartbeat(b.ID))
		time.Sleep(5 * time.Millisecond)
	}

	// The last heartbeat was just written — well within the TTL window.
	// Worker B must NOT reclaim an actively-heartbeating bead.
	err := s.Claim(b.ID, "worker-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot claim")

	// It must also not appear in the ready-execution queue.
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Len(t, ready, 0)

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, "worker-a", got.Owner)
}

func TestAtomicClaimUnderContention(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-atomic-claim", Title: "Only one wins"}
	require.NoError(t, s.Create(b))

	const n = 16
	var wg sync.WaitGroup
	var successes atomic.Int32
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			if err := s.Claim(b.ID, "worker"); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load(), "exactly one goroutine must win the race")

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
}
