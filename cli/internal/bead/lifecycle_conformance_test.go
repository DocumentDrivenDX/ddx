package bead

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/require"
)

// TestNoActiveLifecycleLabelDriving is a static guard that prevents regression
// of active lifecycle label driving. It verifies that LabelNeedsHuman and
// LabelNeedsInvestigation do not appear in production Go files outside an
// explicit allowlist. Adding a new active reference to any file outside the
// allowlist fails this test.
//
// Allowlisted files are those that legitimately carry legacy-label logic under
// the lifecycle epic's one-release keep-for-defensive-cleanup policy:
//   - cli/internal/bead/reconcile.go — const definitions only (routing code deleted)
//   - cli/internal/bead/migrate.go — migration code
//   - cli/internal/bead/lifecycle.go — legacy-label warning path in pure module
//   - cli/internal/bead/lifecycle_gate.go — migration gate logic
//   - cli/internal/bead/store.go — NeedsHuman() deprecation alias + isHumanReviewBlocker
//   - cli/internal/agent/execute_bead_review_terminal.go — migration-only cleanup
//   - cli/internal/agent/execute_bead_post_review.go — migration-only cleanup
//   - cli/internal/agent/execute_bead_conflict_recovery.go — migration-only cleanup
//   - cli/internal/agent/execute_bead_loop.go — migration-only cleanup
//   - cli/internal/agent/execute_bead_park_helpers.go — migration-only cleanup
//   - cli/internal/agent/try/conflict_recovery.go — migration-only cleanup
//   - cli/cmd/bead.go — operator commands, migration-only cleanup paths
func TestNoActiveLifecycleLabelDriving(t *testing.T) {
	// Allowlisted relative paths (relative to the repo root, which is two levels
	// up from cli/internal/bead/).
	allowlist := []string{
		"cli/internal/bead/reconcile.go",
		"cli/internal/bead/migrate.go",
		"cli/internal/bead/lifecycle.go",
		"cli/internal/bead/lifecycle_gate.go",
		"cli/internal/bead/store.go",
		"cli/internal/agent/execute_bead_review_terminal.go",
		"cli/internal/agent/execute_bead_post_review.go",
		"cli/internal/agent/execute_bead_conflict_recovery.go",
		"cli/internal/agent/execute_bead_loop.go",
		"cli/internal/agent/execute_bead_park_helpers.go",
		"cli/internal/agent/try/conflict_recovery.go",
		"cli/cmd/bead.go",
	}
	allowSet := make(map[string]struct{}, len(allowlist))
	for _, p := range allowlist {
		allowSet[p] = struct{}{}
	}

	patterns := [][]byte{
		[]byte("LabelNeedsHuman"),
		[]byte("LabelNeedsInvestigation"),
	}

	// Locate repo root: this file is at cli/internal/bead/lifecycle_conformance_test.go
	// so repo root is three directories up from the file's directory.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	repoRoot, rootErr := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	require.NoError(t, rootErr)

	// Walk all .go files under cli/, skipping test files.
	var violations []string
	err := filepath.WalkDir(filepath.Join(repoRoot, "cli"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		// Normalize to forward slashes for comparison.
		rel = filepath.ToSlash(rel)

		if _, allowed := allowSet[rel]; allowed {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, pattern := range patterns {
			if bytes.Contains(content, pattern) {
				violations = append(violations, rel+" (contains "+string(pattern)+")")
			}
		}
		return nil
	})
	require.NoError(t, err)

	if len(violations) > 0 {
		t.Errorf("active lifecycle label references found outside the allowlist — add the file to the allowlist or remove the reference:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestExecuteLoopSkipsProposedBeads verifies that proposed beads (status=proposed)
// are never returned by ReadyExecution and thus are never selected for execution.
func TestExecuteLoopSkipsProposedBeads(t *testing.T) {
	s := newTestStore(t)

	open := &Bead{ID: "ddx-open", Title: "open bead", Priority: 1}
	proposed := &Bead{ID: "ddx-proposed", Title: "proposed operator attention", Status: StatusProposed, Priority: 0}
	require.NoError(t, s.Create(testCtx(), open))
	require.NoError(t, s.Create(testCtx(), proposed))

	ready, err := s.ReadyExecution()
	require.NoError(t, err)

	for _, b := range ready {
		if b.ID == proposed.ID {
			t.Errorf("proposed bead %s must not appear in ReadyExecution", proposed.ID)
		}
	}

	// Open bead must be present.
	found := false
	for _, b := range ready {
		if b.ID == open.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("open bead %s should appear in ReadyExecution but was absent", open.ID)
	}
}

// TestExecuteLoopSkipsDependencyWaiting verifies that open beads with unmet
// dependencies are never returned by ReadyExecution.
func TestExecuteLoopSkipsDependencyWaiting(t *testing.T) {
	s := newTestStore(t)

	dep := &Bead{ID: "ddx-dep", Title: "unresolved dependency", Priority: 1}
	waiting := &Bead{ID: "ddx-waiting", Title: "dep-waiting bead", Priority: 0}
	waiting.AddDep(dep.ID, "blocks")
	require.NoError(t, s.Create(testCtx(), dep))
	require.NoError(t, s.Create(testCtx(), waiting))

	ready, err := s.ReadyExecution()
	require.NoError(t, err)

	for _, b := range ready {
		if b.ID == waiting.ID {
			t.Errorf("dep-waiting bead %s must not appear in ReadyExecution while dep is open", waiting.ID)
		}
	}

	// The dependency itself (open, no deps) must be ready.
	found := false
	for _, b := range ready {
		if b.ID == dep.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("dependency bead %s should be in ReadyExecution but was absent", dep.ID)
	}
}

// TestExecuteLoopSelectsExternalBlockedAfterRecheck verifies that a bead with
// status=blocked is excluded from ReadyExecution, and that after the operator
// manually transitions it to open it becomes selectable.
func TestExecuteLoopSelectsExternalBlockedAfterRecheck(t *testing.T) {
	s := newTestStore(t)

	blocked := &Bead{ID: "ddx-blocked", Title: "external blocker", Priority: 0}
	require.NoError(t, s.Create(testCtx(), blocked))

	// Transition to blocked with an explicit external blocker reason.
	require.NoError(t, s.UpdateWithLifecycleStatus(blocked.ID, StatusBlocked, LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for upstream API to launch",
		Reason:                "external dependency",
		Source:                "test",
	}, nil))

	// Blocked bead must NOT be in ReadyExecution.
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	for _, b := range ready {
		if b.ID == blocked.ID {
			t.Errorf("blocked bead %s must not appear in ReadyExecution", blocked.ID)
		}
	}

	// Operator rechecks the blocker, determines it is clear, and transitions to open.
	require.NoError(t, s.UpdateWithLifecycleStatus(blocked.ID, StatusOpen, LifecycleTransitionOptions{
		ManualReopen: false,
		Reason:       "external blocker cleared",
		Source:       "test",
	}, nil))

	// Now the bead must appear in ReadyExecution.
	ready, err = s.ReadyExecution()
	require.NoError(t, err)
	found := false
	for _, b := range ready {
		if b.ID == blocked.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("unblocked bead %s should appear in ReadyExecution after recheck but was absent", blocked.ID)
	}
}

// TestLifecycleClosedSatisfiesDependents verifies that closing a parent bead
// causes dependent children to move from dep-waiting to ready.
func TestLifecycleClosedSatisfiesDependents(t *testing.T) {
	s := newTestStore(t)

	parent := &Bead{ID: "ddx-parent", Title: "parent", Priority: 1}
	child := &Bead{ID: "ddx-child", Title: "child", Priority: 0}
	child.AddDep(parent.ID, "blocks")
	require.NoError(t, s.Create(testCtx(), parent))
	require.NoError(t, s.Create(testCtx(), child))

	// Child has unmet dep — must not be ready.
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	for _, b := range ready {
		if b.ID == child.ID {
			t.Errorf("child bead %s should not be ready while parent is open", child.ID)
		}
	}

	// Close parent — dep is now satisfied.
	require.NoError(t, s.Close(testCtx(), parent.ID))

	ready, err = s.ReadyExecution()
	require.NoError(t, err)
	found := false
	for _, b := range ready {
		if b.ID == child.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("child bead %s should be ready after parent closed but was absent", child.ID)
	}
}

// TestLifecycleCancelledDoesNotSatisfyDependents verifies that cancelling a
// parent bead does NOT unblock dependent children — they remain dep-waiting.
func TestLifecycleCancelledDoesNotSatisfyDependents(t *testing.T) {
	s := newTestStore(t)

	parent := &Bead{ID: "ddx-parent-cancel", Title: "cancelled parent", Priority: 1}
	child := &Bead{ID: "ddx-child-cancel", Title: "child of cancelled", Priority: 0}
	child.AddDep(parent.ID, "blocks")
	require.NoError(t, s.Create(testCtx(), parent))
	require.NoError(t, s.Create(testCtx(), child))

	// Cancel the parent.
	require.NoError(t, s.UpdateWithLifecycleStatus(parent.ID, StatusCancelled, LifecycleTransitionOptions{
		Reason: "cancelled by test",
		Source: "test",
	}, nil))

	// cancelled does not satisfy dependents per LifecycleStatusSatisfiesDependency.
	if LifecycleStatusSatisfiesDependency(StatusCancelled) {
		t.Fatalf("invariant violated: StatusCancelled must not satisfy dependents")
	}

	// Child must remain dep-waiting: not in ReadyExecution.
	ready, err := s.ReadyExecution()
	require.NoError(t, err)
	for _, b := range ready {
		if b.ID == child.ID {
			t.Errorf("child bead %s must remain dep-waiting after parent cancelled", child.ID)
		}
	}

	// Confirm via DependencyWaiting list.
	depWaiting, err := s.DependencyWaiting()
	require.NoError(t, err)
	found := false
	for _, b := range depWaiting {
		if b.ID == child.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("child bead %s should appear in DependencyWaiting after parent cancelled but was absent", child.ID)
	}
}

// TestStartupGateRefusesUnmigratedQueue verifies that a project whose beads.jsonl
// contains legacy lifecycle labels and lacks a lifecycle-schema.json marker
// triggers a migration-required error from DetectLifecycleMigrationRequired.
func TestStartupGateRefusesUnmigratedQueue(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, os.MkdirAll(s.Dir, 0o755))

	// Seed an unmigrated beads.jsonl: open bead with needs_human label.
	row := `{"id":"ddx-legacy","title":"legacy bead","status":"open","priority":2,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","labels":["needs_human"]}` + "\n"
	require.NoError(t, os.WriteFile(s.File, []byte(row), 0o644))

	// No lifecycle-schema.json marker exists — should require migration.
	status, err := s.DetectLifecycleMigrationRequired()
	require.NoError(t, err)
	require.True(t, status.Required(), "DetectLifecycleMigrationRequired must return Required=true for unmigrated queue with needs_human label")
	require.Equal(t, LifecycleMigrationGateCodeRequired, status.Code)
	require.NotNil(t, status.Err(), "Err() must return a non-nil error when migration is required")
	require.Contains(t, status.Error(), "ddx-legacy", "error message should name the sample legacy bead")
}
