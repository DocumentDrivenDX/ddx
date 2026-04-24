package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunState_WriteReadCleanupCycle(t *testing.T) {
	projectRoot := t.TempDir()

	// Read before any write returns (nil, nil).
	if s, err := ReadRunState(projectRoot); err != nil || s != nil {
		t.Fatalf("ReadRunState with no file: got (%v, %v), want (nil, nil)", s, err)
	}

	want := RunState{
		BeadID:       "ddx-abc123",
		AttemptID:    "20260424T100000-deadbeef",
		Harness:      "claude",
		Model:        "claude-sonnet-4-6",
		StartedAt:    time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC),
		WorktreePath: "/tmp/ddx-exec-wt/.execute-bead-wt-ddx-abc123-x",
	}
	if err := WriteRunState(projectRoot, want); err != nil {
		t.Fatalf("WriteRunState: %v", err)
	}

	path := filepath.Join(projectRoot, ".ddx", "run-state.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist after write: %v", path, err)
	}

	got, err := ReadRunState(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunState: %v", err)
	}
	if got == nil {
		t.Fatalf("ReadRunState returned nil after write")
	}
	if got.BeadID != want.BeadID || got.AttemptID != want.AttemptID ||
		got.Harness != want.Harness || got.Model != want.Model ||
		got.WorktreePath != want.WorktreePath ||
		!got.StartedAt.Equal(want.StartedAt) {
		t.Fatalf("ReadRunState: got %+v, want %+v", *got, want)
	}

	// No tmp files left behind by atomic rename.
	entries, _ := os.ReadDir(filepath.Join(projectRoot, ".ddx"))
	for _, e := range entries {
		if name := e.Name(); name != "run-state.json" {
			if filepath.Ext(name) == ".tmp" || (len(name) > 4 && name[len(name)-4:] == ".tmp") {
				t.Fatalf("tmp file left over from atomic write: %s", name)
			}
		}
	}

	if err := ClearRunState(projectRoot); err != nil {
		t.Fatalf("ClearRunState: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("run-state.json still present after ClearRunState: %v", err)
	}

	// ClearRunState on missing file is a no-op.
	if err := ClearRunState(projectRoot); err != nil {
		t.Fatalf("ClearRunState on missing file: %v", err)
	}

	// ReadRunState after clear is (nil, nil).
	if s, err := ReadRunState(projectRoot); err != nil || s != nil {
		t.Fatalf("ReadRunState after clear: got (%v, %v)", s, err)
	}
}

func TestRunState_WriteOverwritesExisting(t *testing.T) {
	projectRoot := t.TempDir()
	if err := WriteRunState(projectRoot, RunState{BeadID: "first", AttemptID: "a1", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteRunState(projectRoot, RunState{BeadID: "second", AttemptID: "a2", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := ReadRunState(projectRoot)
	if err != nil || got == nil {
		t.Fatalf("ReadRunState: %v, %v", got, err)
	}
	if got.BeadID != "second" || got.AttemptID != "a2" {
		t.Fatalf("expected overwrite to second, got %+v", *got)
	}
}

// TestRecoverOrphans_CleansStaleRunState simulates a crashed worker: a stale
// run-state file points at a worktree that no longer exists. RecoverOrphans
// must sweep it so the next operator poll does not see phantom execution.
func TestRecoverOrphans_CleansStaleRunState(t *testing.T) {
	projectRoot := t.TempDir()
	staleWt := filepath.Join(projectRoot, "nonexistent-worktree")
	if err := WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-stale",
		AttemptID:    "stale-attempt",
		Harness:      "claude",
		Model:        "claude-sonnet-4-6",
		StartedAt:    time.Now().UTC(),
		WorktreePath: staleWt,
	}); err != nil {
		t.Fatalf("seed stale run-state: %v", err)
	}

	// Fake GitOps returns no worktrees — there is nothing for RecoverOrphans
	// to reap via git, but run-state still points at a vanished worktree and
	// must be cleared.
	git := &emptyWorktreeGit{}
	RecoverOrphans(git, projectRoot, "ddx-stale")

	if s, err := ReadRunState(projectRoot); err != nil || s != nil {
		t.Fatalf("stale run-state not cleaned: got (%v, %v)", s, err)
	}
}

// TestRecoverOrphans_KeepsRunStateForLiveBead ensures recovery for a
// different bead ID does not wipe run-state for a worktree that still exists.
func TestRecoverOrphans_KeepsRunStateForLiveBead(t *testing.T) {
	projectRoot := t.TempDir()
	liveWt := t.TempDir() // exists
	if err := WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-live",
		AttemptID:    "live-attempt",
		StartedAt:    time.Now().UTC(),
		WorktreePath: liveWt,
	}); err != nil {
		t.Fatalf("seed live run-state: %v", err)
	}

	git := &emptyWorktreeGit{}
	RecoverOrphans(git, projectRoot, "ddx-other")

	got, err := ReadRunState(projectRoot)
	if err != nil || got == nil {
		t.Fatalf("live run-state was cleared unexpectedly: got (%v, %v)", got, err)
	}
	if got.BeadID != "ddx-live" {
		t.Fatalf("run-state clobbered: %+v", *got)
	}
}

// emptyWorktreeGit is a minimal GitOps stub that reports no worktrees and
// succeeds on prune/remove. Only the methods RecoverOrphans invokes need to
// behave; the rest are no-ops to satisfy the interface.
type emptyWorktreeGit struct{}

func (emptyWorktreeGit) HeadRev(string) (string, error)                { return "", nil }
func (emptyWorktreeGit) ResolveRev(string, string) (string, error)     { return "", nil }
func (emptyWorktreeGit) WorktreeAdd(string, string, string) error      { return nil }
func (emptyWorktreeGit) WorktreeRemove(string, string) error           { return nil }
func (emptyWorktreeGit) WorktreeList(string) ([]string, error)         { return nil, nil }
func (emptyWorktreeGit) WorktreePrune(string) error                    { return nil }
func (emptyWorktreeGit) IsDirty(string) (bool, error)                  { return false, nil }
func (emptyWorktreeGit) SynthesizeCommit(string, string) (bool, error) { return false, nil }
func (emptyWorktreeGit) UpdateRef(string, string, string) error        { return nil }
func (emptyWorktreeGit) DeleteRef(string, string) error                { return nil }
