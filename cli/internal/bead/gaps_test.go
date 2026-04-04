package bead

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Dep existence validation on Create ──────────────────────────────

func TestCreateWithDanglingDep(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Has bad dep", Deps: []string{"nonexistent-id"}}
	err := s.Create(b)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency not found")
}

func TestCreateWithValidDep(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "Exists"}
	require.NoError(t, s.Create(a))

	b := &Bead{Title: "Depends on A", Deps: []string{a.ID}}
	require.NoError(t, s.Create(b))

	got, err := s.Get(b.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Deps, a.ID)
}

// ── Priority-sorted ready queue ──────────────────────────────────

func TestReadySortedByPriority(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.Create(&Bead{Title: "Low priority", Priority: 3}))
	require.NoError(t, s.Create(&Bead{Title: "High priority", Priority: 0}))
	require.NoError(t, s.Create(&Bead{Title: "Medium priority", Priority: 2}))

	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 3)
	assert.Equal(t, 0, ready[0].Priority)
	assert.Equal(t, 2, ready[1].Priority)
	assert.Equal(t, 3, ready[2].Priority)
}

// ── Execution-eligible filtering ──────────────────────────────────

func TestReadyExecutionFilters(t *testing.T) {
	s := newTestStore(t)

	// Write beads with HELIX-style metadata directly
	jsonl := `{"id":"hx-001","title":"Eligible","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"execution-eligible":true,"superseded-by":"","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
{"id":"hx-002","title":"Not eligible","type":"task","status":"open","priority":1,"labels":["helix","phase:review"],"deps":[],"execution-eligible":false,"superseded-by":"","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
{"id":"hx-003","title":"Superseded","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"execution-eligible":true,"superseded-by":"hx-004","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
{"id":"hx-004","title":"No metadata","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(s.File, []byte(jsonl+"\n"), 0o644))

	// Regular ready returns all 4
	all, err := s.Ready()
	require.NoError(t, err)
	assert.Len(t, all, 4)

	// Execution-filtered ready excludes non-eligible and superseded
	exec, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Len(t, exec, 2)
	assert.Equal(t, "hx-001", exec[0].ID) // eligible
	assert.Equal(t, "hx-004", exec[1].ID) // no metadata = eligible by default
}

// ── Validation hooks ──────────────────────────────────────────────

func TestValidationHookBlocks(t *testing.T) {
	s := newTestStore(t)

	// Create hook that rejects beads without "required" label
	hookDir := filepath.Join(s.Dir, "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	hookScript := `#!/bin/sh
echo "missing required label" >&2
exit 1`
	hookPath := filepath.Join(hookDir, "validate-bead-create")
	require.NoError(t, os.WriteFile(hookPath, []byte(hookScript), 0o755))

	err := s.Create(&Bead{Title: "Should fail"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required label")
}

func TestValidationHookWarning(t *testing.T) {
	s := newTestStore(t)

	hookDir := filepath.Join(s.Dir, "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	hookScript := `#!/bin/sh
echo "consider adding labels" >&2
exit 2`
	hookPath := filepath.Join(hookDir, "validate-bead-create")
	require.NoError(t, os.WriteFile(hookPath, []byte(hookScript), 0o755))

	// Warning should not block creation
	b := &Bead{Title: "Should succeed with warning"}
	err := s.Create(b)
	assert.NoError(t, err)
	assert.NotEmpty(t, b.ID)
}

func TestValidationHookPasses(t *testing.T) {
	s := newTestStore(t)

	hookDir := filepath.Join(s.Dir, "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	hookScript := `#!/bin/sh
exit 0`
	hookPath := filepath.Join(hookDir, "validate-bead-create")
	require.NoError(t, os.WriteFile(hookPath, []byte(hookScript), 0o755))

	b := &Bead{Title: "Passes hook"}
	require.NoError(t, s.Create(b))
	assert.NotEmpty(t, b.ID)
}

func TestNoHookNoError(t *testing.T) {
	s := newTestStore(t)

	// No hook installed — should work fine
	b := &Bead{Title: "No hook"}
	require.NoError(t, s.Create(b))
}

// ── Stale lock detection ──────────────────────────────────────────

func TestStaleLockByAge(t *testing.T) {
	s := newTestStore(t)

	// Create a stale lock manually
	require.NoError(t, os.MkdirAll(s.LockDir, 0o755))
	os.WriteFile(filepath.Join(s.LockDir, "pid"),
		[]byte("999999"), 0o644) // likely dead PID
	staleTime := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	os.WriteFile(filepath.Join(s.LockDir, "acquired_at"),
		[]byte(staleTime), 0o644)

	// Should break stale lock and succeed
	b := &Bead{Title: "After stale lock"}
	require.NoError(t, s.Create(b))
	assert.NotEmpty(t, b.ID)
}

func TestLockWritesAcquiredAt(t *testing.T) {
	s := newTestStore(t)

	// Acquire and release a lock, check acquired_at was written
	require.NoError(t, s.WithLock(func() error {
		data, err := os.ReadFile(filepath.Join(s.LockDir, "acquired_at"))
		require.NoError(t, err)
		_, err = time.Parse(time.RFC3339, string(data))
		assert.NoError(t, err, "acquired_at should be valid RFC3339")
		return nil
	}))
}

// ── Concurrency ──────────────────────────────────────────────────

func TestLockTimeout(t *testing.T) {
	s := newTestStore(t)
	s.LockWait = 200 * time.Millisecond // fast timeout for test

	// Hold lock from current process (definitely alive) with fresh timestamp
	// so neither PID-death nor age-based stale detection will break it
	require.NoError(t, os.MkdirAll(s.LockDir, 0o755))
	os.WriteFile(filepath.Join(s.LockDir, "pid"),
		[]byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
	os.WriteFile(filepath.Join(s.LockDir, "acquired_at"),
		[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644)

	err := s.WithLock(func() error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock timeout")

	// Clean up
	os.RemoveAll(s.LockDir)
}

func TestConcurrentCreatesSerialized(t *testing.T) {
	s := newTestStore(t)

	done := make(chan error, 2)

	// Launch two concurrent creates
	for i := 0; i < 2; i++ {
		go func(n int) {
			b := &Bead{Title: "Concurrent " + string(rune('A'+n))}
			done <- s.Create(b)
		}(i)
	}

	// Both should succeed
	for i := 0; i < 2; i++ {
		assert.NoError(t, <-done)
	}

	beads, err := s.ReadAll()
	require.NoError(t, err)
	assert.Len(t, beads, 2)
}

// ── Circular dependency detection ──────────────────────────────────

func TestCircularDepDetected(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	b := &Bead{Title: "B"}
	c := &Bead{Title: "C"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))
	require.NoError(t, s.Create(c))

	require.NoError(t, s.DepAdd(b.ID, a.ID)) // B depends on A
	require.NoError(t, s.DepAdd(c.ID, b.ID)) // C depends on B

	// A depends on C would create A→C→B→A cycle
	err := s.DepAdd(a.ID, c.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

// ── Malformed JSONL resilience ──────────────────────────────────

func TestMalformedJSONLReturnsError(t *testing.T) {
	s := newTestStore(t)

	// Write corrupt data
	require.NoError(t, os.WriteFile(s.File, []byte("not json\n"), 0o644))

	_, err := s.ReadAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestPartiallyMalformedJSONL(t *testing.T) {
	s := newTestStore(t)

	// One good line, one bad
	jsonl := `{"id":"bx-good","title":"Good","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}
not json at all`
	require.NoError(t, os.WriteFile(s.File, []byte(jsonl), 0o644))

	// Should fail on malformed data (strict mode)
	_, err := s.ReadAll()
	assert.Error(t, err)
}
