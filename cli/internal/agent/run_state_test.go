package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func newRunStateProjectRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return t.TempDir()
}

func TestRunState_WriteReadCleanupCycle(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)

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

	path := runStatePath(projectRoot)
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
	entries, _ := os.ReadDir(filepath.Dir(path))
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

func TestRunState_CandidateCycleFieldsRoundTrip(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
	want := RunState{
		BeadID:              "ddx-cycle",
		AttemptID:           "attempt-cycle",
		StartedAt:           time.Date(2026, 5, 9, 3, 0, 0, 0, time.UTC),
		WorktreePath:        filepath.Join(projectRoot, "wt"),
		CandidateCyclePhase: "review",
		CandidateRef:        "refs/ddx/iterations/attempt-cycle/0",
		CandidateRev:        "abc123",
		CycleIndex:          0,
		ReviewActive:        true,
	}
	if err := WriteRunState(projectRoot, want); err != nil {
		t.Fatalf("WriteRunState: %v", err)
	}
	got, err := ReadRunState(projectRoot)
	if err != nil || got == nil {
		t.Fatalf("ReadRunState: got (%+v, %v)", got, err)
	}
	if got.CandidateCyclePhase != want.CandidateCyclePhase ||
		got.CandidateRef != want.CandidateRef ||
		got.CandidateRev != want.CandidateRev ||
		got.CycleIndex != want.CycleIndex ||
		!got.ReviewActive ||
		got.RepairActive {
		t.Fatalf("candidate-cycle fields did not round trip: got %+v want %+v", *got, want)
	}
}

func TestRunState_OldJSONWithoutCandidateCycleFields(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
	requireDir := ddxroot.InTree(projectRoot)
	if err := os.MkdirAll(requireDir, 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}
	raw := `{"bead_id":"ddx-old","attempt_id":"attempt-old","started_at":"2026-05-09T03:15:00Z","worktree_path":"/tmp/ddx-old-wt"}` + "\n"
	if err := os.WriteFile(filepath.Join(requireDir, "run-state.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy run-state: %v", err)
	}
	got, err := ReadRunState(projectRoot)
	if err != nil || got == nil {
		t.Fatalf("ReadRunState: got (%+v, %v)", got, err)
	}
	if got.CandidateCyclePhase != "" || got.CandidateRef != "" || got.CandidateRev != "" ||
		got.CycleIndex != 0 || got.ReviewActive || got.RepairActive {
		t.Fatalf("legacy run-state should default candidate-cycle fields to zero values: %+v", *got)
	}
}

func TestRunState_WriteOverwritesExisting(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
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

func TestRunState_MultipleAttemptsDoNotClobber(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
	first := RunState{
		BeadID:       "ddx-first",
		AttemptID:    "attempt-one",
		StartedAt:    time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-one"),
	}
	second := RunState{
		BeadID:       "ddx-second",
		AttemptID:    "attempt-two",
		StartedAt:    time.Date(2026, 5, 7, 12, 1, 0, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-two"),
	}
	if err := WriteRunState(projectRoot, first); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := WriteRunState(projectRoot, second); err != nil {
		t.Fatalf("write second: %v", err)
	}

	states, err := ReadRunStates(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunStates: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("ReadRunStates len=%d, want 2: %+v", len(states), states)
	}
	got := map[string]RunState{}
	for _, state := range states {
		got[state.AttemptID] = state
	}
	if got["attempt-one"].BeadID != "ddx-first" {
		t.Fatalf("first attempt clobbered: %+v", got["attempt-one"])
	}
	if got["attempt-two"].BeadID != "ddx-second" {
		t.Fatalf("second attempt missing: %+v", got["attempt-two"])
	}
	if _, err := os.Stat(filepath.Join(runStateDirPath(projectRoot), "attempt-one.json")); err != nil {
		t.Fatalf("first per-attempt file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runStateDirPath(projectRoot), "attempt-two.json")); err != nil {
		t.Fatalf("second per-attempt file missing: %v", err)
	}
}

func TestRunState_AggregateCompatibilityView(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
	first := RunState{
		BeadID:       "ddx-first",
		AttemptID:    "attempt-one",
		StartedAt:    time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
		RefreshedAt:  time.Date(2026, 5, 7, 12, 0, 10, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-one"),
	}
	second := RunState{
		BeadID:       "ddx-second",
		AttemptID:    "attempt-two",
		StartedAt:    time.Date(2026, 5, 7, 12, 1, 0, 0, time.UTC),
		RefreshedAt:  time.Date(2026, 5, 7, 12, 1, 10, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-two"),
	}
	if err := WriteRunState(projectRoot, first); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := WriteRunState(projectRoot, second); err != nil {
		t.Fatalf("write second: %v", err)
	}

	compat, err := ReadRunState(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunState: %v", err)
	}
	if compat == nil {
		t.Fatalf("ReadRunState returned nil")
	}
	if compat.AttemptID != "attempt-two" {
		t.Fatalf("compat view attempt=%q, want attempt-two: %+v", compat.AttemptID, *compat)
	}
	states, err := ReadRunStates(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunStates: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("compat read destroyed per-attempt state: %+v", states)
	}
}

func TestRunState_ClearOneAttemptPreservesOthers(t *testing.T) {
	projectRoot := newRunStateProjectRoot(t)
	first := RunState{
		BeadID:       "ddx-first",
		AttemptID:    "attempt-one",
		StartedAt:    time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-one"),
	}
	second := RunState{
		BeadID:       "ddx-second",
		AttemptID:    "attempt-two",
		StartedAt:    time.Date(2026, 5, 7, 12, 1, 0, 0, time.UTC),
		WorktreePath: filepath.Join(projectRoot, "wt-two"),
	}
	if err := WriteRunState(projectRoot, first); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := WriteRunState(projectRoot, second); err != nil {
		t.Fatalf("write second: %v", err)
	}
	if err := ClearRunStateAttempt(projectRoot, "attempt-one"); err != nil {
		t.Fatalf("ClearRunStateAttempt: %v", err)
	}

	states, err := ReadRunStates(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunStates: %v", err)
	}
	if len(states) != 1 || states[0].AttemptID != "attempt-two" {
		t.Fatalf("got states %+v, want only attempt-two", states)
	}
	if _, err := os.Stat(filepath.Join(runStateDirPath(projectRoot), "attempt-one.json")); !os.IsNotExist(err) {
		t.Fatalf("cleared attempt file still present: %v", err)
	}
	compat, err := ReadRunState(projectRoot)
	if err != nil {
		t.Fatalf("ReadRunState: %v", err)
	}
	if compat == nil || compat.AttemptID != "attempt-two" {
		t.Fatalf("compat after clear = %+v, want attempt-two", compat)
	}
}

type runStateRefreshTestRunner struct {
	projectRoot string
}

func (r runStateRefreshTestRunner) Run(opts RunArgs) (*Result, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	var firstRefresh time.Time
	for {
		states, err := ReadRunStates(r.projectRoot)
		if err != nil {
			return &Result{ExitCode: 1, Error: err.Error()}, err
		}
		if len(states) == 1 {
			state := states[0]
			if state.SessionID != "" && state.PID != 0 && !state.RefreshedAt.IsZero() && !state.ExpiresAt.IsZero() {
				if firstRefresh.IsZero() {
					firstRefresh = state.RefreshedAt
				} else if state.RefreshedAt.After(firstRefresh) {
					return &Result{ExitCode: 0}, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return &Result{ExitCode: 1, Error: ctx.Err().Error()}, ctx.Err()
		case <-deadline.C:
			return &Result{ExitCode: 1, Error: "timed out waiting for run-state refresh"}, fmt.Errorf("timed out waiting for run-state refresh")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestExecuteBead_RefreshesAttemptLiveness(t *testing.T) {
	oldInterval := RunStateRefreshInterval
	RunStateRefreshInterval = 20 * time.Millisecond
	defer func() { RunStateRefreshInterval = oldInterval }()

	projectRoot, _ := newScriptHarnessRepo(t, 1)
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{}).Resolve(config.CLIOverrides{Harness: "test-harness"})

	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, "ddx-int-0001", rcfg, ExecuteBeadRuntime{
		AgentRunner: runStateRefreshTestRunner{projectRoot: projectRoot},
	}, &RealGitOps{})
	if err != nil {
		t.Fatalf("ExecuteBeadWithConfig: %v", err)
	}
	if res == nil || res.AttemptID == "" {
		t.Fatalf("ExecuteBeadWithConfig result missing attempt id: %+v", res)
	}
	if got, err := ReadRunState(projectRoot); err != nil || got != nil {
		t.Fatalf("run-state after completion: got (%+v, %v), want nil", got, err)
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
