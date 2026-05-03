package graphql

// LAYER 3 of the GraphQL multi-project leak fix (ddx-5ae050dc).
// These tests assert that the Relay Node(id) resolver scopes Bead and
// ExecutionRun lookups to the request's WorkingDir (via WithWorkingDir)
// rather than leaking results across registered projects, and that
// unknown-prefix IDs return (nil, nil) without panicking.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
)

// seedBeadProject creates a minimal DDx project at root with a single bead
// having the supplied id and title.
func seedBeadProject(t *testing.T, root, beadID, title string) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ".ddx"))
	b := &bead.Bead{ID: beadID, Title: title, Status: bead.StatusOpen}
	if err := store.Create(b); err != nil {
		t.Fatalf("create bead in %s: %v", root, err)
	}
}

// seedExecRunProject persists a run record with the given runID under root's
// exec store.
func seedExecRunProject(t *testing.T, root, runID, status string) {
	t.Helper()
	store := ddxexec.NewStore(root)
	now := time.Date(2026, 5, 2, 22, 0, 0, 0, time.UTC)
	rec := ddxexec.RunRecord{
		RunManifest: ddxexec.RunManifest{
			RunID:        runID,
			DefinitionID: "exec-layer3-def@1",
			ArtifactIDs:  []string{"ART-1"},
			StartedAt:    now,
			FinishedAt:   now.Add(time.Second),
			Status:       status,
			ExitCode:     0,
		},
		Result: ddxexec.RunResult{Stdout: "ok\n"},
	}
	if err := store.SaveRunRecord(rec); err != nil {
		t.Fatalf("save run in %s: %v", root, err)
	}
}

// TestNode_ResolvesBeadByID_ScopedToProject — register two projects with
// overlapping bead IDs; query Node("bead-X") via project A's scoped working
// dir; assert returns project A's bead, not B's.
func TestNode_ResolvesBeadByID_ScopedToProject(t *testing.T) {
	root := t.TempDir()
	projA := filepath.Join(root, "proj-a")
	projB := filepath.Join(root, "proj-b")

	const sharedBeadID = "ddx-deadbeef"
	seedBeadProject(t, projA, sharedBeadID, "Alpha bead title")
	seedBeadProject(t, projB, sharedBeadID, "Beta bead title")

	r := &Resolver{WorkingDir: projA}
	q := &queryResolver{r}

	// With ctx scoped to project B, Node must return B's bead, not A's.
	ctxB := WithWorkingDir(context.Background(), projB)
	got, err := q.Node(ctxB, "bead-"+sharedBeadID)
	if err != nil {
		t.Fatalf("Node(B): %v", err)
	}
	if got == nil {
		t.Fatal("Node(B) returned nil; expected B's bead")
	}
	beadGot, ok := got.(*Bead)
	if !ok {
		t.Fatalf("Node(B) returned %T, want *Bead", got)
	}
	if beadGot.Title != "Beta bead title" {
		t.Fatalf("CROSS-PROJECT LEAK: ctx=B got title %q (want %q)", beadGot.Title, "Beta bead title")
	}

	// Sanity: with ctx scoped to project A, Node returns A's bead.
	ctxA := WithWorkingDir(context.Background(), projA)
	gotA, err := q.Node(ctxA, "bead-"+sharedBeadID)
	if err != nil {
		t.Fatalf("Node(A): %v", err)
	}
	beadA, ok := gotA.(*Bead)
	if !ok || beadA.Title != "Alpha bead title" {
		t.Fatalf("Node(A) returned %v / want Alpha bead title", gotA)
	}

	// Bead ID for an unknown bead returns (nil, nil) without error.
	miss, err := q.Node(ctxA, "bead-ddx-nopenope")
	if err != nil {
		t.Fatalf("Node(unknown bead) error: %v", err)
	}
	if miss != nil {
		t.Fatalf("Node(unknown bead) want nil, got %v", miss)
	}
}

// TestNode_ResolvesExecRunByID_ScopedToProject — same shape for execution runs.
func TestNode_ResolvesExecRunByID_ScopedToProject(t *testing.T) {
	root := t.TempDir()
	projA := filepath.Join(root, "proj-a")
	projB := filepath.Join(root, "proj-b")

	const sharedRunID = "exec-20260502T220000-abc1"
	seedExecRunProject(t, projA, sharedRunID, ddxexec.StatusSuccess)
	seedExecRunProject(t, projB, sharedRunID, ddxexec.StatusFailed)

	r := &Resolver{WorkingDir: projA}
	q := &queryResolver{r}

	// ctx=B → status from B's record.
	ctxB := WithWorkingDir(context.Background(), projB)
	got, err := q.Node(ctxB, sharedRunID)
	if err != nil {
		t.Fatalf("Node(B): %v", err)
	}
	if got == nil {
		t.Fatal("Node(B) returned nil; expected B's run")
	}
	runGot, ok := got.(*ExecutionRun)
	if !ok {
		t.Fatalf("Node(B) returned %T, want *ExecutionRun", got)
	}
	if runGot.Status != ddxexec.StatusFailed {
		t.Fatalf("CROSS-PROJECT LEAK: ctx=B got status %q (want %q)", runGot.Status, ddxexec.StatusFailed)
	}

	// ctx=A → status from A's record.
	ctxA := WithWorkingDir(context.Background(), projA)
	gotA, err := q.Node(ctxA, sharedRunID)
	if err != nil {
		t.Fatalf("Node(A): %v", err)
	}
	runA, ok := gotA.(*ExecutionRun)
	if !ok || runA.Status != ddxexec.StatusSuccess {
		t.Fatalf("Node(A) returned %v / want status success", gotA)
	}

	// Unknown run id returns nil, no error.
	miss, err := q.Node(ctxA, "exec-does-not-exist")
	if err != nil {
		t.Fatalf("Node(unknown exec) error: %v", err)
	}
	if miss != nil {
		t.Fatalf("Node(unknown exec) want nil, got %v", miss)
	}
}

// TestNode_ReturnsNilForUnknownPrefix — assert no panic, nil returned for
// any ID that does not match a known prefix.
func TestNode_ReturnsNilForUnknownPrefix(t *testing.T) {
	r := &Resolver{WorkingDir: t.TempDir()}
	q := &queryResolver{r}

	cases := []string{
		"",
		"unknown-12345",
		"foo-bar",
		"bead", // missing dash
		"exec", // missing dash
	}
	for _, id := range cases {
		got, err := q.Node(context.Background(), id)
		if err != nil {
			t.Errorf("Node(%q) returned error: %v", id, err)
		}
		if got != nil {
			t.Errorf("Node(%q) returned non-nil: %v", id, got)
		}
	}
}
