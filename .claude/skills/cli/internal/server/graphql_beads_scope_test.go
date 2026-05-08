package server

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestBeadsByProject_OpensOnlyTargetProjectStore covers ddx-9ce6842a AC §3:
// BeadsByProject (via GetBeadSnapshotsForProject) opens exactly one project's
// bead.Store and never iterates other projects.
func TestBeadsByProject_OpensOnlyTargetProjectStore(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "scope-test")

	root := t.TempDir()
	const projectCount = 5
	projectPaths := make([]string, 0, projectCount)
	for i := 0; i < projectCount; i++ {
		projectPath := filepath.Join(root, fmt.Sprintf("project-%02d", i))
		seedScopeTestBeads(t, projectPath, i, 20)
		projectPaths = append(projectPaths, projectPath)
	}

	srv := New(":0", projectPaths[0])
	t.Cleanup(func() { _ = srv.Shutdown() })
	for _, p := range projectPaths[1:] {
		srv.state.RegisterProject(p)
	}

	projects := srv.state.GetProjects()
	if len(projects) != projectCount {
		t.Fatalf("setup: registered %d projects, want %d", len(projects), projectCount)
	}
	// Pick the last-registered project as the scoped target — that's the
	// worst case for a naive "iterate every project" loop.
	target := projects[len(projects)-1]

	// Install the open hook; assert only the target project is ever touched.
	var mu sync.Mutex
	opened := map[string]int{}
	beadStoreOpenHook = func(projectPath string) {
		mu.Lock()
		defer mu.Unlock()
		opened[projectPath]++
	}
	t.Cleanup(func() { beadStoreOpenHook = nil })

	snaps := srv.state.GetBeadSnapshotsForProject(target.ID, "", "", "")
	if len(snaps) != 20 {
		t.Fatalf("scoped snapshots: want 20, got %d", len(snaps))
	}
	for _, s := range snaps {
		if s.ProjectID != target.ID {
			t.Errorf("scoped snapshot leaked project: want=%s got=%s (bead=%s)", target.ID, s.ProjectID, s.ID)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 1 {
		t.Fatalf("scoped call opened %d project stores, want 1: %v", len(opened), openedPaths(opened))
	}
	if _, ok := opened[target.Path]; !ok {
		t.Fatalf("scoped call opened unexpected project store: %v", openedPaths(opened))
	}
}

// TestBeadsByProject_UnknownProjectReturnsNil guards against silently
// falling through to a cross-project scan on a malformed projectID.
func TestBeadsByProject_UnknownProjectReturnsNil(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "scope-test-missing")

	root := t.TempDir()
	projectPath := filepath.Join(root, "project-only")
	seedScopeTestBeads(t, projectPath, 0, 10)

	srv := New(":0", projectPath)
	t.Cleanup(func() { _ = srv.Shutdown() })

	var openedCount int
	beadStoreOpenHook = func(string) { openedCount++ }
	t.Cleanup(func() { beadStoreOpenHook = nil })

	snaps := srv.state.GetBeadSnapshotsForProject("proj-does-not-exist", "", "", "")
	if snaps != nil {
		t.Errorf("unknown project: want nil snapshots, got %d", len(snaps))
	}
	if openedCount != 0 {
		t.Errorf("unknown project opened %d stores, want 0", openedCount)
	}
}

// TestGetBeadSnapshots_PushdownFiltersStatusAtStoreLayer exercises the
// filter pushdown that ddx-9ce6842a AC §4 requires. With a seeded mix of
// open/closed beads we expect status filtering to materialize only the
// matching subset — checked here via the count returned from the store.
func TestGetBeadSnapshots_PushdownFiltersStatusAtStoreLayer(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "pushdown-test")

	root := t.TempDir()
	projectPath := filepath.Join(root, "project-mixed")
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var beads []bead.Bead
	for i := 0; i < 60; i++ {
		status := bead.StatusOpen
		if i%3 == 0 {
			status = bead.StatusClosed
		}
		beads = append(beads, bead.Bead{
			ID:        fmt.Sprintf("ddx-pushdown-%03d", i),
			Title:     "pushdown fixture",
			Status:    status,
			Priority:  bead.DefaultPriority,
			IssueType: bead.DefaultType,
			CreatedAt: base,
			UpdatedAt: base,
			Labels:    []string{"alpha"},
		})
	}
	if err := store.WriteAll(beads); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := New(":0", projectPath)
	t.Cleanup(func() { _ = srv.Shutdown() })
	projects := srv.state.GetProjects()
	if len(projects) != 1 {
		t.Fatalf("setup: want 1 project, got %d", len(projects))
	}

	// Status=open must return only the 40 open beads; status=closed the 20.
	open := srv.state.GetBeadSnapshotsForProject(projects[0].ID, "open", "", "")
	if len(open) != 40 {
		t.Errorf("pushdown status=open: want 40, got %d", len(open))
	}
	closed := srv.state.GetBeadSnapshotsForProject(projects[0].ID, "closed", "", "")
	if len(closed) != 20 {
		t.Errorf("pushdown status=closed: want 20, got %d", len(closed))
	}
}

func seedScopeTestBeads(t *testing.T, projectPath string, projectIdx, n int) {
	t.Helper()
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beads := make([]bead.Bead, 0, n)
	for bi := 0; bi < n; bi++ {
		beads = append(beads, bead.Bead{
			ID:        fmt.Sprintf("ddx-scope-%02d-%03d", projectIdx, bi),
			Title:     "scope fixture",
			Status:    bead.StatusOpen,
			Priority:  bead.DefaultPriority,
			IssueType: bead.DefaultType,
			CreatedAt: base,
			UpdatedAt: base,
			Labels:    []string{"scope"},
		})
	}
	if err := store.WriteAll(beads); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func openedPaths(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
