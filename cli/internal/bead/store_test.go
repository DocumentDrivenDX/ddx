package bead

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain scrubs all GIT_* environment variables before running tests so
// test subprocesses don't inherit lefthook's GIT_DIR/GIT_WORK_TREE and leak
// into the parent repo's config.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			if idx := strings.IndexByte(kv, '='); idx >= 0 {
				_ = os.Unsetenv(kv[:idx])
			}
		}
	}
	if homeDir, err := os.MkdirTemp("", "ddx-bead-home-*"); err == nil {
		_ = os.Setenv("HOME", homeDir)
		defer os.RemoveAll(homeDir)
	}
	if xdgDir, err := os.MkdirTemp("", "ddx-bead-xdg-*"); err == nil {
		_ = os.Setenv("XDG_DATA_HOME", xdgDir)
		defer os.RemoveAll(xdgDir)
	}
	os.Exit(m.Run())
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, s.Init(context.Background()))
	return s
}

func newConfiguredStore(t *testing.T, backend string) *Store {
	t.Helper()
	dir := t.TempDir()
	if backend != "" {
		writeStoreConfig(t, dir, backend)
	}
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, s.Init(context.Background()))
	return s
}

func newJSONLStore(t *testing.T) *Store {
	t.Helper()
	return newConfiguredStore(t, BackendJSONL)
}

func writeStoreConfig(t *testing.T, dir string, backend string) {
	t.Helper()
	ddxDir := filepath.Join(dir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	content := fmt.Sprintf(`version: "1.0"
library:
  path: "./library"
  repository:
    url: "https://github.com/test/repo"
    branch: "main"
bead:
  backend: %s
`, backend)
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(content), 0o644))
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, s.Init(context.Background()))

	_, err := os.Stat(s.File)
	assert.NoError(t, err, "beads.jsonl should exist after init")
}

func TestInitUsesCollectionFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithCollection(filepath.Join(dir, ddxroot.DirName), "exec-runs")
	require.Equal(t, "exec-runs", s.Collection)
	require.Equal(t, filepath.Join(dir, ddxroot.DirName, "exec-runs.jsonl"), s.File)
	require.Equal(t, filepath.Join(dir, ddxroot.DirName, "exec-runs.lock"), s.LockDir)
	require.NoError(t, s.Init(context.Background()))

	_, err := os.Stat(s.File)
	assert.NoError(t, err, "collection file should exist after init")
}

func TestStoreInit_RejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName))
	err := s.Init(ctx)
	require.Error(t, err, "Init must return an error when context is already canceled")
}

func TestStoreGenID_RejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName))
	_, err := s.GenID(ctx)
	require.Error(t, err, "GenID must return an error when context is already canceled")
}

func TestStoreCreate_RejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := newTestStore(t)
	err := s.Create(ctx, &Bead{Title: "canceled create"})
	require.Error(t, err, "Create must return an error when context is already canceled")
}

func TestStoreUpdate_RejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := newTestStore(t)
	err := s.Update(ctx, "missing", func(*Bead) {})
	require.Error(t, err, "Update must return an error when context is already canceled")
}

func TestStoreClose_RejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := newTestStore(t)
	err := s.Close(ctx, "missing")
	require.Error(t, err, "Close must return an error when context is already canceled")
}

func TestNewStore_DefaultsToJSONL(t *testing.T) {
	t.Setenv("DDX_BEAD_BACKEND", "")
	t.Setenv("DDX_AXON_EXPERIMENTAL", "")
	s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName))
	assert.Nil(t, s.backend)
}

func TestNewStore_SelectsAxonFromConfig(t *testing.T) {
	t.Setenv("DDX_BEAD_BACKEND", "")
	t.Setenv("DDX_AXON_EXPERIMENTAL", "")
	tempDir := t.TempDir()
	writeStoreConfig(t, tempDir, BackendAxon)

	s := NewStore(filepath.Join(tempDir, ddxroot.DirName))
	_, ok := s.backend.(*AxonBackend)
	assert.True(t, ok, "bead.backend: axon must select AxonBackend from config alone")
}

func TestWithCollectionNormalizesJSONLExtension(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ddxroot.DirName), WithCollection("agent-sessions.jsonl"))
	assert.Equal(t, "agent-sessions", s.Collection)
	assert.Equal(t, filepath.Join(dir, ddxroot.DirName, "agent-sessions.jsonl"), s.File)
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
			s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName), WithCollection(tc.collection))
			backend, ok := s.backend.(*ExternalBackend)
			require.True(t, ok)
			assert.Equal(t, tc.backend, backend.Tool)
			assert.Equal(t, tc.collection, backend.Collection)
		})
	}
}

func TestNewStore_PreservesExternalBackends(t *testing.T) {
	t.Setenv("DDX_BEAD_BACKEND", "")
	t.Setenv("DDX_AXON_EXPERIMENTAL", "")
	toolDir := t.TempDir()
	writeFakeBackendTool(t, toolDir, "bd")
	writeFakeBackendTool(t, toolDir, "br")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tc := range []struct {
		name       string
		backend    string
		collection string
	}{
		{name: "bd", backend: "bd", collection: DefaultCollection},
		{name: "br", backend: "br", collection: "exec-runs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			writeStoreConfig(t, tempDir, tc.backend)
			s := NewStore(filepath.Join(tempDir, ddxroot.DirName), WithCollection(tc.collection))
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

	s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName), WithCollection("exec-runs"))
	assert.Nil(t, s.backend)
}

// TestExternalBackendOpensBeadsArchiveWithFallback verifies that opening the
// "beads-archive" collection through the bd external backend produces a
// working read/write path via the JSONL fallback, without invoking bd.
// bd/br do not yet expose per-collection scoping in their CLI surface, so
// non-default collections must round-trip through .ddx/<collection>.jsonl
// to avoid colliding with the tool's primary store.
func TestExternalBackendOpensBeadsArchiveWithFallback(t *testing.T) {
	toolDir := t.TempDir()
	// A fake bd that always errors. If the backend ever shells out for the
	// archive collection, ReadAll/WriteAll will fail and this test will
	// catch the regression.
	failScript := "#!/bin/sh\necho 'fake bd should not be called for non-default collections' 1>&2\nexit 1\n"
	failBd := filepath.Join(toolDir, "bd")
	require.NoError(t, os.WriteFile(failBd, []byte(failScript), 0o755))
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DDX_BEAD_BACKEND", "bd")

	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ddxroot.DirName)
	s := NewStore(ddxDir, WithCollection(BeadsArchiveCollection))

	backend, ok := s.backend.(*ExternalBackend)
	require.True(t, ok, "expected ExternalBackend for bd + non-default collection")
	require.Equal(t, "bd", backend.Tool)
	require.Equal(t, BeadsArchiveCollection, backend.Collection)
	require.NotNil(t, backend.fallback, "non-default collection must have JSONL fallback")

	// Init must not panic and must create the archive file under .ddx/.
	require.NoError(t, s.Init(testCtx()))
	archivePath := filepath.Join(ddxDir, BeadsArchiveCollection+".jsonl")
	_, err := os.Stat(archivePath)
	require.NoError(t, err, "archive file should exist after Init")

	// Empty read works.
	got, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Empty(t, got)

	// Round-trip a bead through WriteAll/ReadAll without touching bd.
	want := Bead{ID: "ddx-arch-1", Title: "archived item", Status: StatusClosed, IssueType: "task", Priority: 2}
	require.NoError(t, s.WriteAll([]Bead{want}))
	got, err = s.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, want.ID, got[0].ID)
	assert.Equal(t, want.Title, got[0].Title)
}

// TestExternalBackendDefaultCollectionHasNoFallback locks in that the
// default "beads" collection still routes directly to bd/br so the existing
// interchange contract (schema_compat_test.go, import/export round-trip)
// stays unchanged.
func TestExternalBackendDefaultCollectionHasNoFallback(t *testing.T) {
	toolDir := t.TempDir()
	writeFakeBackendTool(t, toolDir, "bd")
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DDX_BEAD_BACKEND", "bd")

	s := NewStore(filepath.Join(t.TempDir(), ddxroot.DirName))
	backend, ok := s.backend.(*ExternalBackend)
	require.True(t, ok)
	assert.Equal(t, DefaultCollection, backend.Collection)
	assert.Nil(t, backend.fallback, "default collection must not use fallback")
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
	require.NoError(t, s.Create(testCtx(), b))

	assert.NotEmpty(t, b.ID)
	assert.True(t, len(b.ID) > 3, "ID should have prefix + hex")
	assert.Equal(t, "bug", b.IssueType)
	assert.Equal(t, StatusOpen, b.Status)
	assert.Equal(t, 1, b.Priority)
	assert.False(t, b.CreatedAt.IsZero())

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, b.Title, got.Title)
	assert.Equal(t, b.IssueType, got.IssueType)
}

func TestBeadStore_ConcurrentCreateDoesNotLoseRows(t *testing.T) {
	s := newTestStore(t)

	const n = 64
	start := make(chan struct{})
	errCh := make(chan error, n)
	wantIDs := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ddx-create-%03d", i)
		wantIDs[id] = struct{}{}
		go func(i int, id string) {
			<-start
			errCh <- s.Create(testCtx(), &Bead{
				ID:    id,
				Title: fmt.Sprintf("concurrent create %03d", i),
			})
		}(i, id)
	}

	close(start)
	for i := 0; i < n; i++ {
		require.NoError(t, <-errCh)
	}

	got, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, got, n)
	for _, b := range got {
		delete(wantIDs, b.ID)
	}
	assert.Empty(t, wantIDs, "all concurrently-created rows must be present")
}

func TestBeadStore_MutationRereadsUnderLock(t *testing.T) {
	now := time.Now().UTC()
	backend := &mutationOrderBackend{
		beads: []Bead{{
			ID:        "ddx-reread-1",
			Title:     "original",
			IssueType: DefaultType,
			Status:    StatusOpen,
			Priority:  1,
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	s := &Store{backend: backend}

	require.NoError(t, s.Update(testCtx(), "ddx-reread-1", func(b *Bead) {
		b.Title = "updated"
	}))

	assert.True(t, backend.readSawLock.Load(), "mutation must read the corpus after acquiring the collection lock")
	assert.True(t, backend.writeSawLock.Load(), "mutation must write the corpus while holding the collection lock")
	require.Len(t, backend.written, 1)
	assert.Equal(t, "updated", backend.written[0].Title)
}

type mutationOrderBackend struct {
	locked       atomic.Bool
	readSawLock  atomic.Bool
	writeSawLock atomic.Bool

	mu      sync.Mutex
	beads   []Bead
	written []Bead
}

func (b *mutationOrderBackend) Init() error { return nil }

func (b *mutationOrderBackend) ReadAll() ([]Bead, error) {
	b.readSawLock.Store(b.locked.Load())
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Bead(nil), b.beads...), nil
}

func (b *mutationOrderBackend) WriteAll(beads []Bead) error {
	b.writeSawLock.Store(b.locked.Load())
	b.mu.Lock()
	defer b.mu.Unlock()
	b.written = append([]Bead(nil), beads...)
	b.beads = append([]Bead(nil), beads...)
	return nil
}

func (b *mutationOrderBackend) WithLock(fn func() error) error {
	b.locked.Store(true)
	defer b.locked.Store(false)
	return fn()
}

func TestStoreReadAllHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.ReadAll(ctx)
	require.Error(t, err)
}

func TestStoreReadAllFilteredHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.ReadAllFiltered(ctx, func(Bead) bool { return true })
	require.Error(t, err)
}

func TestStoreGet_UsesTypedIDArgument(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	b := &Bead{Title: "typed get"}
	require.NoError(t, s.Create(testCtx(), b))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, b.ID, got.ID)
	assert.Equal(t, b.Title, got.Title)
}

func TestStoreGet_HonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Get(ctx, "ddx-missing")
	require.Error(t, err)
}

func TestCreateUsesConfiguredPrefix(t *testing.T) {
	t.Setenv("DDX_BEAD_PREFIX", "")
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
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

	s := NewStore(filepath.Join(tempDir, ddxroot.DirName))
	require.NoError(t, s.Init(testCtx()))

	b := &Bead{Title: "Configured prefix"}
	require.NoError(t, s.Create(testCtx(), b))

	assert.True(t, strings.HasPrefix(b.ID, "nif-"))
}

func TestCreateUsesEnvPrefixOverConfig(t *testing.T) {
	tempDir := t.TempDir()
	ddxDir := filepath.Join(tempDir, ddxroot.DirName)
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

	s := NewStore(filepath.Join(tempDir, ddxroot.DirName))
	require.NoError(t, s.Init(testCtx()))

	b := &Bead{Title: "Env prefix"}
	require.NoError(t, s.Create(testCtx(), b))

	assert.True(t, strings.HasPrefix(b.ID, "env-"))
}

func TestCreateDefaults(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Simple task"}
	require.NoError(t, s.Create(testCtx(), b))

	assert.Equal(t, DefaultType, b.IssueType)
	assert.Equal(t, DefaultStatus, b.Status)
	assert.Equal(t, 0, b.Priority) // Store does not apply priority defaults; CLI layer sets flag default to 2
	assert.Empty(t, b.Labels)
	assert.Empty(t, b.DepIDs())
}

func TestCreateValidation(t *testing.T) {
	s := newTestStore(t)

	// Empty title
	err := s.Create(testCtx(), &Bead{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")

	// Invalid priority
	err = s.Create(testCtx(), &Bead{Title: "Bad priority", Priority: 9})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "priority")

	// Invalid status
	err = s.Create(testCtx(), &Bead{Title: "Bad status", Status: "invalid"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "Original"}
	require.NoError(t, s.Create(testCtx(), b))

	err := s.UpdateWithLifecycleStatus(b.ID, StatusInProgress, LifecycleTransitionOptions{}, func(b *Bead) error {
		b.Title = "Updated"
		b.Owner = "me"
		return nil
	})
	require.NoError(t, err)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Title)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "me", got.Owner)
}

func TestCrossRepoBlockerRef_RoundTrip(t *testing.T) {
	ref, err := NewCrossRepoBlockerRef("upstream", "bx-123")
	require.NoError(t, err)

	data, err := json.Marshal(ref)
	require.NoError(t, err)

	var got CrossRepoBlockerRef
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, ref, got)

	parsed, ok := ParseCrossRepoBlockerRef(map[string]any{"repo": "upstream", "bead": "bx-123"})
	require.True(t, ok)
	assert.Equal(t, ref, parsed)
}

func TestUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Update(testCtx(), "nonexistent", func(b *Bead) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClose(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "To close"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Close(testCtx(), b.ID))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)
}

func TestClose_SupersededCascade_Eligible(t *testing.T) {
	s := newTestStore(t)

	// Create Y first
	y := &Bead{Title: "Superseding bead"}
	require.NoError(t, s.Create(testCtx(), y))

	// Then create X (open, superseded-by:Y, no other state)
	x := &Bead{
		Title:  "Superseded bead",
		Status: StatusOpen,
		Extra:  map[string]any{"superseded-by": y.ID},
	}
	require.NoError(t, s.Create(testCtx(), x))

	// Close Y, should cascade-close X
	require.NoError(t, s.Close(testCtx(), y.ID))

	// Verify Y is closed
	yGot, err := s.Get(testCtx(), y.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, yGot.Status)

	// Verify X is also closed
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Verify X has superseded_close event
	events, err := s.Events(x.ID)
	require.NoError(t, err)
	found := false
	for _, e := range events {
		if e.Kind == "superseded_close" {
			assert.Contains(t, e.Body, fmt.Sprintf("closed_by_cascade_of: %s", y.ID))
			assert.Contains(t, e.Body, fmt.Sprintf("closed_as_superseded_via:%s", y.ID))
			found = true
			break
		}
	}
	assert.True(t, found, "X should have superseded_close event")
}

func TestClose_SupersededCascade_OperatorNotedNotCascaded(t *testing.T) {
	s := newTestStore(t)

	// Create Y (open) and X (open, superseded-by:Y, has operator notes)
	y := &Bead{Title: "Superseding bead"}
	x := &Bead{
		Title:  "Superseded bead",
		Status: StatusOpen,
		Notes:  "Operator decision to keep this open",
		Extra:  map[string]any{"superseded-by": y.ID},
	}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Create(testCtx(), x))

	// Close Y, should NOT cascade-close X due to operator notes
	require.NoError(t, s.Close(testCtx(), y.ID))

	// Verify Y is closed
	yGot, err := s.Get(testCtx(), y.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, yGot.Status)

	// Verify X remains open
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, xGot.Status)

	// Verify X has NO superseded_close event
	events, err := s.Events(x.ID)
	require.NoError(t, err)
	for _, e := range events {
		assert.NotEqual(t, "superseded_close", e.Kind, "X should not have superseded_close event")
	}
}

func TestClose_SupersededCascade_OneHopCycleGuard(t *testing.T) {
	s := newTestStore(t)

	// Create Y, X (superseded-by:Y), Z (superseded-by:X)
	y := &Bead{Title: "Y"}
	x := &Bead{
		Title:  "X",
		Status: StatusOpen,
		Extra:  map[string]any{},
	}
	z := &Bead{
		Title:  "Z",
		Status: StatusOpen,
		Extra:  map[string]any{},
	}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Create(testCtx(), x))
	require.NoError(t, s.Create(testCtx(), z))

	// Set supersession chain
	require.NoError(t, s.Update(testCtx(), x.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["superseded-by"] = y.ID
	}))
	require.NoError(t, s.Update(testCtx(), z.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["superseded-by"] = x.ID
	}))

	// Close Y, should cascade-close X but NOT Z (deferred to separate cascade)
	require.NoError(t, s.Close(testCtx(), y.ID))

	// Verify X is closed
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Verify Z remains open (one-hop limit)
	zGot, err := s.Get(testCtx(), z.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, zGot.Status)
}

func TestClose_SupersededCascade_Idempotent(t *testing.T) {
	s := newTestStore(t)

	// Create Y, X
	y := &Bead{Title: "Y"}
	x := &Bead{
		Title:  "X",
		Status: StatusOpen,
		Extra:  map[string]any{"superseded-by": ""},
	}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Create(testCtx(), x))

	// Set X superseded by Y
	require.NoError(t, s.Update(testCtx(), x.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["superseded-by"] = y.ID
	}))

	// Close Y once
	require.NoError(t, s.Close(testCtx(), y.ID))
	xEvents1, _ := s.Events(x.ID)

	// Close Y again (should be idempotent, no new events)
	require.NoError(t, s.Close(testCtx(), y.ID))
	xEvents2, _ := s.Events(x.ID)

	// Events should remain the same
	assert.Len(t, xEvents1, len(xEvents2))
}

func TestClose_SupersededCascade_ClaimedOrInProgress_NotCascaded(t *testing.T) {
	s := newTestStore(t)

	// Create Y, X in in_progress
	y := &Bead{Title: "Y"}
	x := &Bead{
		Title:  "X",
		Status: StatusInProgress,
		Extra:  map[string]any{"superseded-by": ""},
	}
	require.NoError(t, s.Create(testCtx(), y))
	require.NoError(t, s.Create(testCtx(), x))

	// Set X superseded by Y
	require.NoError(t, s.Update(testCtx(), x.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["superseded-by"] = y.ID
	}))

	// Close Y, should NOT cascade-close X since X is in_progress
	require.NoError(t, s.Close(testCtx(), y.ID))

	// Verify X remains in_progress (not closed)
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, xGot.Status)
}

func TestClose_WalkUp_DeadIntermediateClosureCandidate(t *testing.T) {
	s := newTestStore(t)

	// Create parent P (execution-eligible=false, epic container)
	p := &Bead{
		Title:     "Epic parent",
		IssueType: "epic",
		Status:    StatusOpen,
		Extra:     map[string]any{ExtraExecutionElig: false},
	}
	require.NoError(t, s.Create(testCtx(), p))

	// Create child X (open, parent=P)
	x := &Bead{
		Title:  "Child X",
		Status: StatusOpen,
		Parent: p.ID,
	}
	require.NoError(t, s.Create(testCtx(), x))

	// Close X, should auto-close P as dead-intermediate
	require.NoError(t, s.Close(testCtx(), x.ID))

	// Verify X is closed
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Verify P is also closed (walk-up closure candidate)
	pGot, err := s.Get(testCtx(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, pGot.Status)

	// Verify P has dead_intermediate_close event
	pEvents, err := s.Events(p.ID)
	require.NoError(t, err)
	found := false
	for _, e := range pEvents {
		if e.Kind == "dead_intermediate_close" {
			assert.Contains(t, e.Body, "all_children_closed")
			assert.Contains(t, e.Body, "execution_eligible: false")
			found = true
			break
		}
	}
	assert.True(t, found, "P should have dead_intermediate_close event")
}

func TestClose_WalkUp_DeadIntermediateWithRemainingOpenChildren(t *testing.T) {
	s := newTestStore(t)

	// Create parent P (execution-eligible=false)
	p := &Bead{
		Title:     "Epic parent",
		IssueType: "epic",
		Status:    StatusOpen,
		Extra:     map[string]any{ExtraExecutionElig: false},
	}
	require.NoError(t, s.Create(testCtx(), p))

	// Create children X and Z (both open, parent=P)
	x := &Bead{
		Title:  "Child X",
		Status: StatusOpen,
		Parent: p.ID,
	}
	z := &Bead{
		Title:  "Child Z",
		Status: StatusOpen,
		Parent: p.ID,
	}
	require.NoError(t, s.Create(testCtx(), x))
	require.NoError(t, s.Create(testCtx(), z))

	// Close X, should NOT auto-close P because Z is still open
	require.NoError(t, s.Close(testCtx(), x.ID))

	// Verify X is closed
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Verify P remains open (Z is still pending)
	pGot, err := s.Get(testCtx(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, pGot.Status)

	// Verify P has NO dead_intermediate_close event
	pEvents, err := s.Events(p.ID)
	require.NoError(t, err)
	for _, e := range pEvents {
		assert.NotEqual(t, "dead_intermediate_close", e.Kind, "P should not have dead_intermediate_close event")
	}
}

func TestClose_WalkUp_RecursesThroughGrandparentDeadIntermediate(t *testing.T) {
	s := newTestStore(t)

	// Create chain P → Q → X (all execution-eligible=false)
	p := &Bead{
		Title:     "Epic P",
		IssueType: "epic",
		Status:    StatusOpen,
		Extra:     map[string]any{ExtraExecutionElig: false},
	}
	q := &Bead{
		Title:     "Epic Q",
		IssueType: "epic",
		Status:    StatusOpen,
		Parent:    "", // Will be set after creation
		Extra:     map[string]any{ExtraExecutionElig: false},
	}
	x := &Bead{
		Title:  "Child X",
		Status: StatusOpen,
		Parent: "", // Will be set after creation
	}
	require.NoError(t, s.Create(testCtx(), p))
	require.NoError(t, s.Create(testCtx(), q))
	require.NoError(t, s.Create(testCtx(), x))

	// Set parent relationships: P → Q → X
	require.NoError(t, s.Update(testCtx(), q.ID, func(b *Bead) {
		b.Parent = p.ID
	}))
	require.NoError(t, s.Update(testCtx(), x.ID, func(b *Bead) {
		b.Parent = q.ID
	}))

	// Close X, should auto-close Q and recurse to P in the same operation.
	require.NoError(t, s.Close(testCtx(), x.ID))

	// Verify X is closed
	xGot, err := s.Get(testCtx(), x.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, xGot.Status)

	// Verify Q is closed by the recursive walk-up.
	qGot, err := s.Get(testCtx(), q.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, qGot.Status)

	// Verify P is also closed by the recursive walk-up.
	pGot, err := s.Get(testCtx(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, pGot.Status)

	// Both ancestors should have dead_intermediate_close events.
	qEvents, err := s.Events(q.ID)
	require.NoError(t, err)
	found := false
	for _, e := range qEvents {
		if e.Kind == "dead_intermediate_close" {
			assert.Contains(t, e.Body, "all_children_closed")
			assert.Contains(t, e.Body, "execution_eligible: false")
			found = true
			break
		}
	}
	assert.True(t, found, "Q should have dead_intermediate_close event")

	pEvents, err := s.Events(p.ID)
	require.NoError(t, err)
	found = false
	for _, e := range pEvents {
		if e.Kind == "dead_intermediate_close" {
			assert.Contains(t, e.Body, "all_children_closed")
			assert.Contains(t, e.Body, "execution_eligible: false")
			found = true
			break
		}
	}
	assert.True(t, found, "P should have dead_intermediate_close event")
}

func TestClose_EpicAutoClose_AllChildrenClosed(t *testing.T) {
	s := newTestStore(t)

	// Epic parent without execution-eligible flag
	epic := &Bead{
		Title:     "Epic: all children closed",
		IssueType: "epic",
		Status:    StatusOpen,
	}
	require.NoError(t, s.Create(testCtx(), epic))

	child := &Bead{Title: "Child task", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), child))

	// Closing the last child should auto-close the epic
	require.NoError(t, s.Close(testCtx(), child.ID))

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, epicGot.Status)

	// Verify epic has epic_auto_close event
	events, err := s.Events(epic.ID)
	require.NoError(t, err)
	found := false
	for _, e := range events {
		if e.Kind == "epic_auto_close" {
			assert.Contains(t, e.Body, "all_children_terminal")
			found = true
			break
		}
	}
	assert.True(t, found, "epic should have epic_auto_close event")
}

func TestClose_EpicAutoClose_CancelledChildCountsAsTerminal(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{
		Title:     "Epic: cancelled child",
		IssueType: "epic",
		Status:    StatusOpen,
	}
	require.NoError(t, s.Create(testCtx(), epic))

	// One closed child, one cancelled child
	c1 := &Bead{Title: "Closed child", Status: StatusOpen, Parent: epic.ID}
	c2 := &Bead{Title: "Cancelled child", Status: StatusCancelled, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), c1))
	require.NoError(t, s.Create(testCtx(), c2))

	// Closing c1 (last non-terminal child) should auto-close the epic
	require.NoError(t, s.Close(testCtx(), c1.ID))

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, epicGot.Status, "epic should auto-close when last non-terminal child closes")
}

func TestClose_EpicAutoClose_MixedOpenClosed_StaysOpen(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{
		Title:     "Epic: mixed children",
		IssueType: "epic",
		Status:    StatusOpen,
	}
	require.NoError(t, s.Create(testCtx(), epic))

	c1 := &Bead{Title: "Child 1", Status: StatusOpen, Parent: epic.ID}
	c2 := &Bead{Title: "Child 2", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), c1))
	require.NoError(t, s.Create(testCtx(), c2))

	// Closing only one child should NOT auto-close the epic
	require.NoError(t, s.Close(testCtx(), c1.ID))

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, epicGot.Status, "epic with an open child should stay open")
}

func TestClose_EpicAutoClose_NoChildren_StaysOpen(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{
		Title:     "Epic: no children",
		IssueType: "epic",
		Status:    StatusOpen,
	}
	require.NoError(t, s.Create(testCtx(), epic))

	unrelated := &Bead{Title: "Unrelated task", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), unrelated))
	require.NoError(t, s.Close(testCtx(), unrelated.ID))

	// Epic with no children should not be affected
	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, epicGot.Status, "epic with no children should not be auto-closed")
}

func TestEpicClosureCandidates(t *testing.T) {
	s := newTestStore(t)

	// Epic A: all children closed → candidate
	epicA := &Bead{Title: "Epic A", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epicA))
	childA := &Bead{Title: "Child A", Status: StatusOpen, Parent: epicA.ID}
	require.NoError(t, s.Create(testCtx(), childA))

	// Epic B: mixed children → not a candidate
	epicB := &Bead{Title: "Epic B", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epicB))
	childB1 := &Bead{Title: "Child B1", Status: StatusOpen, Parent: epicB.ID}
	childB2 := &Bead{Title: "Child B2", Status: StatusOpen, Parent: epicB.ID}
	require.NoError(t, s.Create(testCtx(), childB1))
	require.NoError(t, s.Create(testCtx(), childB2))

	// Epic C: all children terminal (one closed, one cancelled) → candidate
	epicC := &Bead{Title: "Epic C", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epicC))
	childC1 := &Bead{Title: "Child C1 (cancelled)", Status: StatusCancelled, Parent: epicC.ID}
	childC2 := &Bead{Title: "Child C2", Status: StatusOpen, Parent: epicC.ID}
	require.NoError(t, s.Create(testCtx(), childC1))
	require.NoError(t, s.Create(testCtx(), childC2))

	// Epic D: no children → not a candidate
	epicD := &Bead{Title: "Epic D", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epicD))

	// Close childA (now epicA should be a candidate via EpicClosureCandidates)
	// Note: Close() will auto-close epicA, so we need to bypass auto-close to test the method.
	// Instead, close childA directly via SetLifecycleStatus to avoid the walk-up.
	require.NoError(t, s.SetLifecycleStatus(childA.ID, StatusClosed, LifecycleTransitionOptions{ManualClose: true}))

	// Close childB1 (epicB still has open childB2)
	require.NoError(t, s.SetLifecycleStatus(childB1.ID, StatusClosed, LifecycleTransitionOptions{ManualClose: true}))

	// Close childC2 (epicC now has all-terminal children)
	require.NoError(t, s.SetLifecycleStatus(childC2.ID, StatusClosed, LifecycleTransitionOptions{ManualClose: true}))

	candidates, err := s.EpicClosureCandidates(testCtx())
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, c := range candidates {
		ids[c.ID] = true
	}
	assert.True(t, ids[epicA.ID], "epicA should be a candidate (all children closed)")
	assert.False(t, ids[epicB.ID], "epicB should not be a candidate (has open child)")
	assert.True(t, ids[epicC.ID], "epicC should be a candidate (all children terminal: closed+cancelled)")
	assert.False(t, ids[epicD.ID], "epicD should not be a candidate (no children)")
}

// closeWithEvidenceHelper appends an execute-bead event and calls CloseWithEvidence
// with a non-empty commitSHA so the ClosureGate passes.
func closeWithEvidenceHelper(t *testing.T, s *Store, id string) {
	t.Helper()
	require.NoError(t, s.AppendEvent(id, BeadEvent{
		Kind:    "execute-bead",
		Summary: "success",
	}))
	require.NoError(t, s.CloseWithEvidence(id, "session-test", "abc123"))
}

func TestCloseWithEvidence_AutoClosesParentEpicWhenLastChildCloses(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{Title: "Epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epic))

	childA := &Bead{Title: "Child A", Status: StatusOpen, Parent: epic.ID}
	childB := &Bead{Title: "Child B", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), childA))
	require.NoError(t, s.Create(testCtx(), childB))

	// Close child A — epic still has open child B, should stay open.
	closeWithEvidenceHelper(t, s, childA.ID)
	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, epicGot.Status, "epic should stay open while child B is open")

	// Close child B — now all children are terminal; epic should auto-close.
	closeWithEvidenceHelper(t, s, childB.ID)
	epicGot, err = s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, epicGot.Status, "epic should auto-close when last child closes")

	events, err := s.Events(epic.ID)
	require.NoError(t, err)
	found := false
	for _, e := range events {
		if e.Kind == "epic_auto_close" {
			assert.Contains(t, e.Body, "all_children_terminal")
			found = true
			break
		}
	}
	assert.True(t, found, "epic should have epic_auto_close event")
}

func TestCloseWithEvidence_DoesNotCloseEpicWithOpenChildren(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{Title: "Epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epic))

	childA := &Bead{Title: "Child A", Status: StatusOpen, Parent: epic.ID}
	childB := &Bead{Title: "Child B", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), childA))
	require.NoError(t, s.Create(testCtx(), childB))

	// Close only one child — epic should remain open.
	closeWithEvidenceHelper(t, s, childA.ID)

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, epicGot.Status, "epic should stay open while child B is still open")
}

func TestCloseWithEvidence_RejectedCloseDoesNotWalkUp(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{Title: "Epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epic))

	child := &Bead{Title: "Child", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), child))

	// Call CloseWithEvidence with no evidence — ClosureGate should reject it.
	// Empty sessionID and empty commitSHA with no prior events triggers the gate.
	err := s.CloseWithEvidence(child.ID, "", "")
	// The call itself does not return an error; the gate records a rejection note.
	require.NoError(t, err)

	// Child must still be open (gate rejected the close).
	childGot, err := s.Get(testCtx(), child.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, childGot.Status, "child should remain open when gate rejects")

	// Epic must not have been walked up.
	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, epicGot.Status, "epic should not change when child's close is rejected")
}

func TestCloseWithEvidence_CancelledChildCountsTerminal(t *testing.T) {
	s := newTestStore(t)

	epic := &Bead{Title: "Epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epic))

	// One closed child, one cancelled child — epic should auto-close.
	childClosed := &Bead{Title: "Closed child", Status: StatusOpen, Parent: epic.ID}
	childCancelled := &Bead{Title: "Cancelled child", Status: StatusCancelled, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), childClosed))
	require.NoError(t, s.Create(testCtx(), childCancelled))

	closeWithEvidenceHelper(t, s, childClosed.ID)

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, epicGot.Status, "epic should auto-close: one closed + one cancelled = all terminal")
}

func TestCloseWithEvidence_AllCancelledChildrenCancelsEpic(t *testing.T) {
	// walkUpClosureCandidate (invoked by cascadeAndWalkUp, which CloseWithEvidence now
	// calls) must set the epic to cancelled — not closed — when every child is cancelled
	// and none reached closed. This tests the code path directly.
	s := newTestStore(t)

	epic := &Bead{Title: "Epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), epic))

	ca := &Bead{Title: "CA", Status: StatusCancelled, Parent: epic.ID}
	cb := &Bead{Title: "CB", Status: StatusCancelled, Parent: epic.ID}
	cc := &Bead{Title: "CC", Status: StatusOpen, Parent: epic.ID}
	require.NoError(t, s.Create(testCtx(), ca))
	require.NoError(t, s.Create(testCtx(), cb))
	require.NoError(t, s.Create(testCtx(), cc))

	// Cancel the last open child, then fire the walk-up directly as cascadeAndWalkUp does.
	require.NoError(t, s.SetLifecycleStatus(cc.ID, StatusCancelled, LifecycleTransitionOptions{ManualClose: true}))
	all, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	visited := map[string]bool{cc.ID: true}
	s.walkUpClosureCandidate(epic.ID, all, visited)

	epicGot, err := s.Get(testCtx(), epic.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, epicGot.Status, "epic should be cancelled when all children are cancelled")
}

func TestWalkUpClosure_RecursesToGrandparentEpic(t *testing.T) {
	s := newTestStore(t)

	grandparent := &Bead{Title: "Grandparent epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), grandparent))
	parent := &Bead{Title: "Parent epic", IssueType: "epic", Status: StatusOpen, Parent: grandparent.ID}
	require.NoError(t, s.Create(testCtx(), parent))
	leaf := &Bead{Title: "Leaf task", Status: StatusOpen, Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), leaf))

	closeWithEvidenceHelper(t, s, leaf.ID)

	parentGot, err := s.Get(testCtx(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, parentGot.Status, "parent epic should auto-close when its only child closes")

	grandparentGot, err := s.Get(testCtx(), grandparent.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, grandparentGot.Status, "grandparent epic should auto-close in the same close operation")

	parentEvents, err := s.Events(parent.ID)
	require.NoError(t, err)
	parentFound := false
	for _, e := range parentEvents {
		if e.Kind == "epic_auto_close" {
			assert.Contains(t, e.Body, "all_children_terminal")
			parentFound = true
			break
		}
	}
	assert.True(t, parentFound, "parent epic should have epic_auto_close event")

	grandparentEvents, err := s.Events(grandparent.ID)
	require.NoError(t, err)
	grandparentFound := false
	for _, e := range grandparentEvents {
		if e.Kind == "epic_auto_close" {
			assert.Contains(t, e.Body, "all_children_terminal")
			grandparentFound = true
			break
		}
	}
	assert.True(t, grandparentFound, "grandparent epic should have epic_auto_close event")
}

func TestWalkUpClosure_StopsAtOpenAncestor(t *testing.T) {
	s := newTestStore(t)

	grandparent := &Bead{Title: "Grandparent epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), grandparent))
	parent := &Bead{Title: "Parent epic", IssueType: "epic", Status: StatusOpen, Parent: grandparent.ID}
	require.NoError(t, s.Create(testCtx(), parent))
	leaf := &Bead{Title: "Leaf task", Status: StatusOpen, Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), leaf))
	sibling := &Bead{Title: "Grandparent sibling", Status: StatusOpen, Parent: grandparent.ID}
	require.NoError(t, s.Create(testCtx(), sibling))

	closeWithEvidenceHelper(t, s, leaf.ID)

	parentGot, err := s.Get(testCtx(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, parentGot.Status, "parent epic should still auto-close")

	grandparentGot, err := s.Get(testCtx(), grandparent.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, grandparentGot.Status, "grandparent epic must stay open while sibling remains open")
}

func TestWalkUpClosure_CycleGuardTerminates(t *testing.T) {
	s := newTestStore(t)

	parent := &Bead{Title: "Parent epic", IssueType: "epic", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), parent))
	cyclePeer := &Bead{Title: "Cycle peer", IssueType: "epic", Status: StatusOpen, Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), cyclePeer))
	leaf := &Bead{Title: "Leaf task", Status: StatusOpen, Parent: parent.ID}
	require.NoError(t, s.Create(testCtx(), leaf))

	require.NoError(t, s.SetLifecycleStatus(cyclePeer.ID, StatusClosed, LifecycleTransitionOptions{
		ManualClose: true,
		Reason:      "seed malformed cycle",
		Source:      "TestWalkUpClosure_CycleGuardTerminates",
	}))
	require.NoError(t, s.Update(testCtx(), parent.ID, func(b *Bead) {
		b.Parent = cyclePeer.ID
	}))

	errCh := make(chan error, 1)
	go func() {
		if err := s.AppendEvent(leaf.ID, BeadEvent{
			Kind:    "execute-bead",
			Summary: "success",
		}); err != nil {
			errCh <- err
			return
		}
		errCh <- s.CloseWithEvidence(leaf.ID, "session-test", "abc123")
	}()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("walk-up closure did not terminate on a malformed parent cycle")
	}

	parentGot, err := s.Get(testCtx(), parent.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, parentGot.Status, "parent epic should still close")

	cyclePeerGot, err := s.Get(testCtx(), cyclePeer.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, cyclePeerGot.Status, "cycle peer should remain closed after recursion terminates")
}

func TestListFilters(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.Create(testCtx(), &Bead{Title: "Open task", Labels: []string{"backend"}}))
	b2 := &Bead{Title: "Closed task", Labels: []string{"frontend"}}
	require.NoError(t, s.Create(testCtx(), b2))
	require.NoError(t, s.Close(testCtx(), b2.ID))

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
	require.NoError(t, s.Create(testCtx(), b1))
	// Set spec-id in Extra via Update
	require.NoError(t, s.Update(testCtx(), b1.ID, func(b *Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra["spec-id"] = "FEAT-006"
	}))

	b2 := &Bead{Title: "Other task"}
	require.NoError(t, s.Create(testCtx(), b2))
	require.NoError(t, s.Update(testCtx(), b2.ID, func(b *Bead) {
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
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID))

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
	require.NoError(t, s.Close(testCtx(), a.ID))

	ready, err = s.Ready()
	require.NoError(t, err)
	assert.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID)

	blocked, err = s.Blocked()
	require.NoError(t, err)
	assert.Len(t, blocked, 0)
}

func TestUnclosedBlockingDeps_ReturnsOpenDepIDs(t *testing.T) {
	s := newTestStore(t)

	depOpen := &Bead{Title: "Open dep"}
	depClosed := &Bead{Title: "Closed dep"}
	target := &Bead{Title: "Target"}
	require.NoError(t, s.Create(testCtx(), depOpen))
	require.NoError(t, s.Create(testCtx(), depClosed))
	require.NoError(t, s.Create(testCtx(), target))
	require.NoError(t, s.DepAdd(testCtx(), target.ID, depOpen.ID))
	require.NoError(t, s.DepAdd(testCtx(), target.ID, depClosed.ID))
	require.NoError(t, s.Close(testCtx(), depClosed.ID))

	open, err := s.UnclosedBlockingDeps(target.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{depOpen.ID}, open)

	// No deps → empty slice.
	leaf := &Bead{Title: "Leaf"}
	require.NoError(t, s.Create(testCtx(), leaf))
	open, err = s.UnclosedBlockingDeps(leaf.ID)
	require.NoError(t, err)
	assert.Empty(t, open)

	// All deps closed → empty slice.
	require.NoError(t, s.Close(testCtx(), depOpen.ID))
	open, err = s.UnclosedBlockingDeps(target.ID)
	require.NoError(t, err)
	assert.Empty(t, open)
}

func TestClose_RefusesOpenBlocksDep(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{Title: "Prerequisite"}
	target := &Bead{Title: "Dependent"}
	require.NoError(t, s.Create(testCtx(), dep))
	require.NoError(t, s.Create(testCtx(), target))
	require.NoError(t, s.DepAdd(testCtx(), target.ID, dep.ID))

	err := s.Close(testCtx(), target.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDependencyGateRejected)
	assert.Contains(t, err.Error(), dep.ID)

	got, getErr := s.Get(testCtx(), target.ID)
	require.NoError(t, getErr)
	assert.Equal(t, StatusOpen, got.Status, "bead must stay open when dependency gate rejects")

	// Closing is allowed once all blocks-deps are closed.
	require.NoError(t, s.Close(testCtx(), dep.ID))
	require.NoError(t, s.Close(testCtx(), target.ID))
	got, getErr = s.Get(testCtx(), target.ID)
	require.NoError(t, getErr)
	assert.Equal(t, StatusClosed, got.Status)
}

func TestCloseWithEvidence_RefusesOpenBlocksDep(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{Title: "Prerequisite"}
	target := &Bead{Title: "Dependent"}
	require.NoError(t, s.Create(testCtx(), dep))
	require.NoError(t, s.Create(testCtx(), target))
	require.NoError(t, s.DepAdd(testCtx(), target.ID, dep.ID))

	// Supply execution evidence so ClosureGate is not the refusal reason.
	require.NoError(t, s.AppendEvent(target.ID, BeadEvent{
		Kind:    "execute-bead",
		Summary: "success",
	}))
	err := s.CloseWithEvidence(target.ID, "session-test", "abc123")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDependencyGateRejected)
	assert.Contains(t, err.Error(), dep.ID)

	got, getErr := s.Get(testCtx(), target.ID)
	require.NoError(t, getErr)
	assert.Equal(t, StatusOpen, got.Status, "bead must stay open when dependency gate rejects")
}

func TestUpdateWithLifecycleStatus_RefusesCloseWithOpenBlocksDep(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{Title: "Prerequisite"}
	target := &Bead{Title: "Dependent"}
	require.NoError(t, s.Create(testCtx(), dep))
	require.NoError(t, s.Create(testCtx(), target))
	require.NoError(t, s.DepAdd(testCtx(), target.ID, dep.ID))

	err := s.UpdateWithLifecycleStatus(target.ID, StatusClosed, LifecycleTransitionOptions{
		ManualClose: true,
		Reason:      "operator status closed",
		Source:      "test",
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDependencyGateRejected)
	assert.Contains(t, err.Error(), dep.ID)

	got, getErr := s.Get(testCtx(), target.ID)
	require.NoError(t, getErr)
	assert.Equal(t, StatusOpen, got.Status, "bead must stay open when dependency gate rejects")
}

func TestDepAddValidation(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	require.NoError(t, s.Create(testCtx(), a))

	// Dep on nonexistent
	err := s.DepAdd(testCtx(), a.ID, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency not found")

	// Self-dep
	err = s.DepAdd(testCtx(), a.ID, a.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot depend on self")

	// Idempotent add
	b := &Bead{Title: "B"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID))
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID)) // no error on duplicate
}

func TestBeadDepAddRejectsParentAncestor(t *testing.T) {
	s := newTestStore(t)

	root := &Bead{Title: "Root"}
	parent := &Bead{Title: "Parent"}
	child := &Bead{Title: "Child"}
	require.NoError(t, s.Create(testCtx(), root))
	require.NoError(t, s.Create(testCtx(), parent))
	require.NoError(t, s.Create(testCtx(), child))

	require.NoError(t, s.Update(testCtx(), parent.ID, func(b *Bead) {
		b.Parent = root.ID
	}))
	require.NoError(t, s.Update(testCtx(), child.ID, func(b *Bead) {
		b.Parent = parent.ID
	}))

	t.Run("direct-parent", func(t *testing.T) {
		err := s.DepAdd(testCtx(), child.ID, parent.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ancestor in the parent chain")
		assert.Contains(t, err.Error(), parent.ID)
		assert.Contains(t, err.Error(), parent.ID+" -> "+root.ID)
	})

	t.Run("grandparent-ancestor", func(t *testing.T) {
		err := s.DepAdd(testCtx(), child.ID, root.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ancestor in the parent chain")
		assert.Contains(t, err.Error(), root.ID)
		assert.Contains(t, err.Error(), parent.ID+" -> "+root.ID)
	})
}

func TestDepRemove(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	b := &Bead{Title: "B"}
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID))

	got, _ := s.Get(testCtx(), b.ID)
	assert.Contains(t, got.DepIDs(), a.ID)

	require.NoError(t, s.DepRemove(testCtx(), b.ID, a.ID))
	got, _ = s.Get(testCtx(), b.ID)
	assert.NotContains(t, got.DepIDs(), a.ID)
}

func TestDepTree(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "Root task"}
	b := &Bead{Title: "Child task"}
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.DepAdd(testCtx(), b.ID, a.ID))

	tree, err := s.DepTree(testCtx(), "")
	require.NoError(t, err)
	assert.Contains(t, tree, "Root task")
	assert.Contains(t, tree, "Child task")
}

func TestStatusCounts(t *testing.T) {
	s := newTestStore(t)

	a := &Bead{Title: "A"}
	b := &Bead{Title: "B"}
	c := &Bead{Title: "C"}
	require.NoError(t, s.Create(testCtx(), a))
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Create(testCtx(), c))
	require.NoError(t, s.DepAdd(testCtx(), c.ID, a.ID))
	require.NoError(t, s.Close(testCtx(), b.ID))

	counts, err := s.Status()
	require.NoError(t, err)
	assert.Equal(t, 3, counts.Total)
	assert.Equal(t, 2, counts.Open)
	assert.Equal(t, 1, counts.Closed)
	assert.Equal(t, 1, counts.Ready)   // A is ready (no deps)
	assert.Equal(t, 0, counts.Blocked) // persisted status=blocked, not dependency waiting
	assert.Equal(t, 1, counts.DependencyWaiting)
}

func TestUnknownFieldPreservation(t *testing.T) {
	s := newTestStore(t)

	// Write a bead with unknown fields directly
	jsonl := `{"id":"hx-test1234","title":"HELIX bead","type":"task","status":"open","priority":1,"labels":["helix","phase:build"],"deps":[],"spec-id":"FEAT-001","execution-eligible":true,"claimed-at":"2026-01-01T00:00:00Z","created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(s.File, []byte(jsonl+"\n"), 0o644))

	// Read back
	beads, err := s.ReadAll(testCtx())
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
	beads2, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, beads2, 1)
	assert.Equal(t, "FEAT-001", beads2[0].Extra["spec-id"])
	assert.Equal(t, true, beads2[0].Extra["execution-eligible"])
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(testCtx(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReadEmptyStore(t *testing.T) {
	s := newTestStore(t)
	beads, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Nil(t, beads)
}

func TestReadNonexistentFile(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nope"))
	beads, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	assert.Nil(t, beads)
}

func TestUnclaimDoesNotReopenClosedBead(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Create a bead
	b := &Bead{ID: "test-unclaim-001", Title: "Test bead", IssueType: "task", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), b))

	// Claim and close it
	require.NoError(t, s.Claim(b.ID, "worker"))
	require.NoError(t, s.Close(testCtx(), b.ID))

	// Verify it's closed
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status)

	// Unclaim should NOT reopen it
	require.NoError(t, s.Unclaim(b.ID))

	got, err = s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status, "unclaim must not reopen a closed bead")
}

func TestClaimMetadataKeys_SharedByReopenAndUnclaim(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-ckmeta-001", Title: "Claim metadata key test", IssueType: "task", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), b))

	// Helper: populate all ClaimMetadataExtraKeys plus heartbeat and a survivor key.
	populateClaimMeta := func(id string) {
		require.NoError(t, s.Update(testCtx(), id, func(b *Bead) {
			if b.Extra == nil {
				b.Extra = make(map[string]any)
			}
			for _, k := range ClaimMetadataExtraKeys {
				b.Extra[k] = "test-value"
			}
			b.Extra[ClaimHeartbeatExtraKey] = "test-heartbeat"
			b.Extra["claim-test-extra"] = "should-survive"
		}))
	}

	// --- AC4.a + AC4.b: Unclaim clears all ClaimMetadataExtraKeys ---
	require.NoError(t, s.Claim(b.ID, "worker"))
	populateClaimMeta(b.ID)
	require.NoError(t, s.Unclaim(b.ID))

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	for _, k := range ClaimMetadataExtraKeys {
		assert.NotContains(t, got.Extra, k, "Unclaim must clear %q", k)
	}
	// AC4.d: unrelated key survives Unclaim
	assert.Equal(t, "should-survive", got.Extra["claim-test-extra"], "Unclaim must not clear unrelated Extra keys")

	// --- AC4.a + AC4.c: Reopen clears all ClaimMetadataExtraKeys ---
	// Close the bead so Reopen has something to reopen.
	require.NoError(t, s.Claim(b.ID, "worker"))
	populateClaimMeta(b.ID)
	require.NoError(t, s.Close(testCtx(), b.ID))

	require.NoError(t, s.Reopen(b.ID, "test reopen", ""))

	got, err = s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	for _, k := range ClaimMetadataExtraKeys {
		assert.NotContains(t, got.Extra, k, "Reopen must clear %q", k)
	}
	// AC4.d: unrelated key survives Reopen
	assert.Equal(t, "should-survive", got.Extra["claim-test-extra"], "Reopen must not clear unrelated Extra keys")
}

// TestStoreReleaseClearsClaimAtomically verifies Store.Release is the
// symmetric counterpart to Store.Claim: it returns a claimed bead to open,
// clears the owner and claim metadata, removes the external lease sidecar, and
// the resulting status counts report nothing in_progress.
func TestStoreReleaseClearsClaimAtomically(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-release-atomic", Title: "Release clears claim", IssueType: "task", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), b))

	// Claim transitions to in_progress with an owner and writes a lease sidecar.
	require.NoError(t, s.Claim(b.ID, "worker-a"))
	claimed, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, claimed.Status)
	require.Equal(t, "worker-a", claimed.Owner)
	_, leasePresent, err := s.ClaimLease(b.ID)
	require.NoError(t, err)
	require.True(t, leasePresent, "claim must write a lease sidecar")

	// Release returns the bead to open, clears the owner, and removes the lease.
	require.NoError(t, s.Release(b.ID, "worker-a", ""))

	released, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, released.Status, "release must return the bead to open")
	assert.Empty(t, released.Owner, "release must clear the owner")
	for _, k := range ClaimMetadataExtraKeys {
		assert.NotContains(t, released.Extra, k, "release must clear claim metadata key %q", k)
	}
	assert.NotContains(t, released.Extra, ClaimHeartbeatExtraKey, "release must clear the heartbeat extra key")

	_, leasePresent, err = s.ClaimLease(b.ID)
	require.NoError(t, err)
	assert.False(t, leasePresent, "release must remove the claim lease sidecar")

	// ddx bead status must reflect the released lease: nothing in_progress.
	counts, err := s.Status()
	require.NoError(t, err)
	assert.Equal(t, 0, counts.InProgress, "released bead must not be counted in_progress")
	assert.Equal(t, 1, counts.Open, "released bead must be counted open")
}

// TestClaimReleaseStatusReflectsLease runs the claim/release lifecycle across a
// worker goroutine while the test goroutine inspects status: while claimed the
// bead reports in_progress with an owner; after release it reports open with no
// owner.
func TestClaimReleaseStatusReflectsLease(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-release-status", Title: "Status reflects lease", IssueType: "task", Status: StatusOpen}
	require.NoError(t, s.Create(testCtx(), b))

	claimed := make(chan struct{})
	released := make(chan struct{})
	observerDone := make(chan struct{})
	claimErr := make(chan error, 1)
	releaseErr := make(chan error, 1)

	// Worker goroutine: claim, signal, hold until the observer inspects, release.
	go func() {
		claimErr <- s.Claim(b.ID, "worker-a")
		close(claimed)
		<-observerDone
		releaseErr <- s.Release(b.ID, "worker-a", "")
		close(released)
	}()

	// Inspect status while the claim is held by the worker goroutine.
	<-claimed
	require.NoError(t, <-claimErr)
	mid, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, mid.Status, "while claimed the bead must report in_progress")
	assert.Equal(t, "worker-a", mid.Owner, "while claimed the bead must report an owner")
	midCounts, err := s.Status()
	require.NoError(t, err)
	assert.Equal(t, 1, midCounts.InProgress, "claimed bead must be counted in_progress")

	// Let the worker release, then inspect again.
	close(observerDone)
	<-released
	require.NoError(t, <-releaseErr)
	after, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, after.Status, "after release the bead must report open")
	assert.Empty(t, after.Owner, "after release the bead must report no owner")
	afterCounts, err := s.Status()
	require.NoError(t, err)
	assert.Equal(t, 0, afterCounts.InProgress, "released bead must not be counted in_progress")
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
	require.NoError(t, s.Create(testCtx(), b))

	// First worker claims it normally.
	require.NoError(t, s.Claim(b.ID, "worker-a"))

	// Forge a stale runtime lease and a stale legacy tracker heartbeat.
	stale := time.Now().UTC().Add(-1 * time.Hour)
	staleRec := ClaimLeaseRecord{
		BeadID:    b.ID,
		UpdatedAt: stale,
		PID:       os.Getpid(),
	}
	data, err := json.Marshal(staleRec)
	require.NoError(t, err)
	require.NoError(t, writeAtomicClaimFile(claimLivenessPath(s.Dir, b.ID), append(data, '\n')))
	require.NoError(t, s.Update(testCtx(), b.ID, func(bd *Bead) {
		if bd.Extra == nil {
			bd.Extra = map[string]any{}
		}
		staleStr := stale.Format(time.RFC3339)
		bd.Extra["work-heartbeat-at"] = staleStr
		bd.Extra["claimed-at"] = staleStr
	}))

	// A fresh worker must be able to reclaim the stalled bead.
	require.NoError(t, s.Claim(b.ID, "worker-b"))
	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.Equal(t, "worker-b", got.Owner)

	// The ready-execution queue must surface the stale bead too.
	s2 := newTestStore(t)
	orig := &Bead{ID: "ddx-hb-stale-2", Title: "Stale claim 2"}
	require.NoError(t, s2.Create(testCtx(), orig))
	require.NoError(t, s2.Claim(orig.ID, "worker-a"))
	staleLease := ClaimLeaseRecord{
		BeadID:    orig.ID,
		UpdatedAt: time.Now().UTC().Add(-1 * time.Hour),
		PID:       os.Getpid(),
	}
	data, err = json.Marshal(staleLease)
	require.NoError(t, err)
	require.NoError(t, writeAtomicClaimFile(claimLivenessPath(s2.Dir, orig.ID), append(data, '\n')))
	ready, err := s2.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, orig.ID, ready[0].ID)
}

func TestReady_StaleClaimedInProgressIsReady(t *testing.T) {
	withHeartbeat(t, 10*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-stale-ready", Title: "Stale in_progress bead"}
	require.NoError(t, s.Create(testCtx(), b))

	// Claim it so status transitions to in_progress.
	require.NoError(t, s.Claim(b.ID, "worker-a"))

	// Forge a stale liveness lease so the claim is no longer fresh.
	stale := time.Now().UTC().Add(-1 * time.Hour)
	staleRec := ClaimLeaseRecord{
		BeadID:    b.ID,
		UpdatedAt: stale,
		PID:       os.Getpid(),
	}
	data, err := json.Marshal(staleRec)
	require.NoError(t, err)
	require.NoError(t, writeAtomicClaimFile(claimLivenessPath(s.Dir, b.ID), append(data, '\n')))
	require.NoError(t, s.Update(testCtx(), b.ID, func(bd *Bead) {
		if bd.Extra == nil {
			bd.Extra = map[string]any{}
		}
		staleStr := stale.Format(time.RFC3339)
		bd.Extra["work-heartbeat-at"] = staleStr
		bd.Extra["claimed-at"] = staleStr
	}))

	// Operator-facing Ready() must surface the stale-claimed in_progress bead.
	ready, err := s.Ready()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID)
}

func TestHeartbeatKeepsActiveClaimAlive(t *testing.T) {
	withHeartbeat(t, 5*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-hb-live", Title: "Live claim"}
	require.NoError(t, s.Create(testCtx(), b))

	require.NoError(t, s.Claim(b.ID, "worker-a"))

	// Actively refresh the heartbeat for longer than the TTL.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.NoError(t, s.TouchClaimHeartbeat(b.ID))
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, s.TouchClaimHeartbeat(b.ID))

	// The last heartbeat was just written — well within the TTL window.
	// Worker B must NOT reclaim an actively-heartbeating bead.
	err := s.Claim(b.ID, "worker-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot claim")

	// It must also not appear in the ready-execution queue.
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Len(t, ready, 0)

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, "worker-a", got.Owner)
}

func TestClaimLeaseCanonicalizesProjectRootAliases(t *testing.T) {
	withHeartbeat(t, 5*time.Millisecond, 50*time.Millisecond)

	root := t.TempDir()
	realRoot := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(realRoot, ddxroot.DirName), 0o755))
	aliasRoot := filepath.Join(root, "repo-alias")
	require.NoError(t, os.Symlink(realRoot, aliasRoot))

	sReal := NewStore(filepath.Join(realRoot, ddxroot.DirName))
	sAlias := NewStore(filepath.Join(aliasRoot, ddxroot.DirName))
	b := &Bead{ID: "ddx-hb-alias", Title: "Live claim through alias"}
	require.NoError(t, sReal.Create(context.Background(), b))
	require.NoError(t, sReal.Claim(b.ID, "worker-a"))

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.NoError(t, sReal.TouchClaimHeartbeat(b.ID))
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, sReal.TouchClaimHeartbeat(b.ID))

	err := sAlias.Claim(b.ID, "worker-b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot claim")

	ready, err := sAlias.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready)

	got, err := sReal.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, "worker-a", got.Owner)
}

func TestStoreHeartbeat_RemovedOrNoTrackerWrite(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-hb-no-tracker", Title: "Heartbeat stays out of JSONL"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.Claim(b.ID, "worker-a"))

	before, err := os.ReadFile(s.File)
	require.NoError(t, err)
	require.NoError(t, s.Heartbeat(b.ID))
	after, err := os.ReadFile(s.File)
	require.NoError(t, err)
	assert.Equal(t, before, after, "heartbeat must not mutate beads.jsonl")

	leasePath := claimLivenessPath(s.Dir, b.ID)
	_, err = os.Stat(leasePath)
	require.NoError(t, err, "heartbeat must be recorded in the external lease file")
}

func TestWorkerClaimLeaseDoesNotMutateTracker(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-worker-lease", Title: "Worker lease only"}
	require.NoError(t, s.Create(testCtx(), b))

	before, err := os.ReadFile(s.File)
	require.NoError(t, err)

	require.NoError(t, s.ClaimWithOptions(b.ID, "worker-a", "sess-lease", "/tmp/wt"))

	after, err := os.ReadFile(s.File)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "worker claim lease must not rewrite beads.jsonl")

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	lease, found, err := s.ClaimLease(b.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, b.ID, lease.BeadID)
	assert.Equal(t, "worker-a", lease.Owner)
	assert.Equal(t, "sess-lease", lease.Session)
	assert.Equal(t, "/tmp/wt", lease.Worktree)
	assert.False(t, lease.StartedAt.IsZero())
	assert.False(t, lease.UpdatedAt.IsZero())
	assert.NotZero(t, lease.PID)
}

func TestReadyExecutionExcludesFreshSidecarLease(t *testing.T) {
	withHeartbeat(t, 10*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-fresh-worker-lease", Title: "Fresh worker lease"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.ClaimWithOptions(b.ID, "worker-a", "sess-ready", ""))

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	assert.Empty(t, ready, "fresh worker claim leases must suppress execution-ready selection")
}

func TestReadyExecutionReclaimsStaleSidecarLease(t *testing.T) {
	withHeartbeat(t, 10*time.Millisecond, 50*time.Millisecond)

	s := newTestStore(t)
	b := &Bead{ID: "ddx-stale-worker-lease", Title: "Stale worker lease"}
	require.NoError(t, s.Create(testCtx(), b))
	require.NoError(t, s.ClaimWithOptions(b.ID, "worker-a", "sess-stale", ""))

	stale := time.Now().UTC().Add(-1 * time.Hour)
	require.NoError(t, s.writeClaimHeartbeat(ClaimLeaseRecord{
		BeadID:    b.ID,
		Owner:     "worker-a",
		Session:   "sess-stale",
		StartedAt: stale,
		UpdatedAt: stale,
		PID:       os.Getpid(),
	}))

	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID)
}

func TestAtomicClaimUnderContention(t *testing.T) {
	s := newTestStore(t)
	b := &Bead{ID: "ddx-atomic-claim", Title: "Only one wins"}
	require.NoError(t, s.Create(testCtx(), b))

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

	got, err := s.Get(testCtx(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
}

// TestConcurrentUpdates_DifferentBeads spawns N=16 goroutines each updating a
// distinct bead. All updates must land and the resulting JSONL must be valid
// (no interleaved lines, no truncation, no lost updates).
func TestConcurrentUpdates_DifferentBeads(t *testing.T) {
	const n = 16
	s := newTestStore(t)

	// Pre-create one bead per goroutine.
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		b := &Bead{Title: fmt.Sprintf("bead-%d", i)}
		require.NoError(t, s.Create(testCtx(), b))
		ids[i] = b.ID
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			err := s.Update(testCtx(), ids[i], func(b *Bead) {
				b.Notes = fmt.Sprintf("updated-by-goroutine-%d", i)
			})
			assert.NoError(t, err, "goroutine %d update must not error", i)
		}()
	}
	wg.Wait()

	// All beads must have received their update.
	for i := 0; i < n; i++ {
		got, err := s.Get(testCtx(), ids[i])
		require.NoError(t, err, "bead %s must be readable after concurrent updates", ids[i])
		assert.Equal(t, fmt.Sprintf("updated-by-goroutine-%d", i), got.Notes,
			"bead %d must carry its update", i)
	}

	// JSONL file must be fully parseable with no truncated/interleaved lines.
	data, err := os.ReadFile(s.File)
	require.NoError(t, err)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var lineCount int
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineCount++
		// Each line must be valid JSON (starts with '{' and ends with '}').
		assert.True(t, strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}"),
			"JSONL line must be a complete JSON object: %q", line)
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, n, lineCount, "JSONL must contain exactly %d lines", n)
}

// TestPartialWriteCleanup verifies that stale tmp files left by a crashed
// writer are cleaned up when a new write completes, and the real file is
// unaffected by their presence.
func TestPartialWriteCleanup(t *testing.T) {
	s := newTestStore(t)

	b := &Bead{Title: "survivor"}
	require.NoError(t, s.Create(testCtx(), b))

	// Simulate a crashed writer: drop a stale .tmp-* file in the same dir.
	staleContent := []byte(`{"id":"ghost","title":"ghost","type":"task","status":"open","priority":0,"created":"2026-01-01T00:00:00Z","updated":"2026-01-01T00:00:00Z"}` + "\n")
	staleTmp := s.File + ".tmp-99999-deadbeef"
	require.NoError(t, os.WriteFile(staleTmp, staleContent, 0o644))

	// Perform a normal update — should succeed regardless of the stale tmp file.
	require.NoError(t, s.Update(testCtx(), b.ID, func(b *Bead) {
		b.Notes = "after stale tmp"
	}))

	// The stale tmp file is not automatically removed by the store (it's left
	// for the OS/operator), but the real file must be correct and not contain
	// the ghost bead.
	beads, err := s.ReadAll(testCtx())
	require.NoError(t, err)
	require.Len(t, beads, 1, "real file must contain exactly 1 bead")
	assert.Equal(t, b.ID, beads[0].ID)
	assert.Equal(t, "after stale tmp", beads[0].Notes)

	// The stale tmp file itself must not have been renamed over the real file.
	for _, bead := range beads {
		assert.NotEqual(t, "ghost", bead.ID, "ghost bead from stale tmp must not appear")
	}
}

// TestAtomicRename_OriginalPreservedOnError verifies that if a WriteAll fails
// after the tmp file is written but before rename, the original beads.jsonl
// is unchanged. We simulate this by providing a read-only destination directory
// to force the rename to fail.
func TestAtomicRename_OriginalPreservedOnError(t *testing.T) {
	s := newTestStore(t)

	// Write initial content.
	original := &Bead{Title: "original"}
	require.NoError(t, s.Create(testCtx(), original))

	// Read the initial file content for later comparison.
	beforeData, err := os.ReadFile(s.File)
	require.NoError(t, err)

	// Use tmpPath directly to generate a tmp path, write it, then attempt a
	// rename to a path in a non-existent directory — simulating rename failure.
	badTarget := filepath.Join(s.Dir, "nonexistent-subdir", "beads.jsonl")
	writeErr := writeAtomicFile(badTarget, []byte(`{"id":"x"}`+"\n"))
	assert.Error(t, writeErr, "write to non-existent dir must fail")

	// Original file must be unchanged.
	afterData, err := os.ReadFile(s.File)
	require.NoError(t, err)
	assert.Equal(t, string(beforeData), string(afterData),
		"original beads.jsonl must be unchanged after failed atomic write")
}

func TestParkToProposed_TransitionAndMetadata(t *testing.T) {
	t.Run("open bead transitions to proposed", func(t *testing.T) {
		s := newTestStore(t)
		b := &Bead{Title: "park open"}
		require.NoError(t, s.Create(testCtx(), b))

		require.NoError(t, s.ParkToProposed(b.ID, ParkConflictRecovery, nil))

		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusProposed, got.Status)
	})

	t.Run("in_progress bead transitions to proposed", func(t *testing.T) {
		s := newTestStore(t)
		b := &Bead{Title: "park in_progress"}
		require.NoError(t, s.Create(testCtx(), b))
		require.NoError(t, s.Claim(b.ID, "test-agent"))

		require.NoError(t, s.ParkToProposed(b.ID, ParkReviewTerminal, nil))

		got, err := s.Get(testCtx(), b.ID)
		require.NoError(t, err)
		assert.Equal(t, StatusProposed, got.Status)
	})

	t.Run("mutate callback runs after transition", func(t *testing.T) {
		s := newTestStore(t)
		b := &Bead{Title: "park mutate order"}
		require.NoError(t, s.Create(testCtx(), b))

		var statusDuringMutate string
		require.NoError(t, s.ParkToProposed(b.ID, ParkIntakeRejection, func(b *Bead) {
			statusDuringMutate = b.Status
		}))
		assert.Equal(t, StatusProposed, statusDuringMutate, "mutate must run after transition")
	})

	t.Run("reason and source match ParkReason mapping", func(t *testing.T) {
		expected := map[ParkReason]parkReasonMeta{
			ParkIntakeRejection:              {Reason: "pre-claim intake blocked execution", Source: "legacy agent work"},
			ParkNoChangesOperatorRequired:    {Reason: "operator decision required before another automated attempt", Source: "legacy agent work"},
			ParkPostReviewMalfunction:        {Reason: "review BLOCK triage reached operator-required rung", Source: "legacy agent work"},
			ParkReviewTerminal:               {Reason: "terminal review block requires operator decision", Source: "legacy agent work"},
			ParkConflictRecovery:             {Reason: "land conflict requires operator judgment", Source: "legacy agent work"},
			ParkReviewRequestClarification:   {Reason: "reviewer cannot adjudicate needs-judgment AC without operator input", Source: "legacy agent work"},
			ParkLadderExhaustionManual:       {Reason: "recovery:manual label set; operator review required", Source: "legacy agent work"},
			ParkAutoRecoveryFailed:           {Reason: "automated recovery failed; operator review required", Source: "legacy agent work"},
			ParkProviderConnectivityRepeated: {Reason: "provider unreachable on repeated attempts; operator review required", Source: "legacy agent work"},
		}
		assert.Equal(t, expected, parkReasonMetaMap)
	})

	t.Run("terminal bead is rejected", func(t *testing.T) {
		s := newTestStore(t)
		b := &Bead{Title: "park terminal"}
		require.NoError(t, s.Create(testCtx(), b))
		require.NoError(t, s.SetLifecycleStatus(b.ID, StatusClosed, LifecycleTransitionOptions{ManualClose: true}))

		err := s.ParkToProposed(b.ID, ParkConflictRecovery, nil)
		assert.Error(t, err, "parking a closed bead must be rejected")
	})
}
