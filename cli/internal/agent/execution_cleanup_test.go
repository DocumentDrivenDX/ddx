package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executionCleanupTestGitOps struct {
	worktrees   []string
	removed     []string
	deletedRefs []string
	pruneCalls  int
	listErr     error
	removeErr   error
	pruneErr    error
}

func (g *executionCleanupTestGitOps) HeadRev(string) (string, error)            { return "", nil }
func (g *executionCleanupTestGitOps) ResolveRev(string, string) (string, error) { return "", nil }
func (g *executionCleanupTestGitOps) WorktreeAdd(string, string, string) error  { return nil }
func (g *executionCleanupTestGitOps) WorktreeRemove(dir, wtPath string) error {
	if g.removeErr != nil {
		return g.removeErr
	}
	g.removed = append(g.removed, wtPath)
	return os.RemoveAll(wtPath)
}
func (g *executionCleanupTestGitOps) WorktreeList(string) ([]string, error) {
	if g.listErr != nil {
		return nil, g.listErr
	}
	return append([]string(nil), g.worktrees...), nil
}
func (g *executionCleanupTestGitOps) WorktreePrune(string) error {
	g.pruneCalls++
	return g.pruneErr
}
func (g *executionCleanupTestGitOps) IsDirty(string) (bool, error) { return false, nil }
func (g *executionCleanupTestGitOps) SynthesizeCommit(string, string) (bool, error) {
	return false, nil
}
func (g *executionCleanupTestGitOps) UpdateRef(string, string, string) error { return nil }
func (g *executionCleanupTestGitOps) DeleteRef(_ string, ref string) error {
	g.deletedRefs = append(g.deletedRefs, ref)
	return nil
}

type executionCleanupTestProbe struct {
	live           map[string]bool
	reason         string
	ignoreRunState bool
}

func (p executionCleanupTestProbe) IsLive(meta ExecutionCleanupMetadata, runState *RunState, now time.Time) (bool, string) {
	_ = now
	if meta.Preserved {
		return true, "preserved metadata"
	}
	key := filepath.Clean(meta.WorktreePath)
	if key == "." || key == "" {
		key = meta.AttemptID
	}
	if p.live != nil && p.live[key] {
		if strings.TrimSpace(p.reason) != "" {
			return true, p.reason
		}
		return true, "live"
	}
	if !p.ignoreRunState && runState != nil && runState.WorktreePath != "" && filepath.Clean(runState.WorktreePath) == filepath.Clean(meta.WorktreePath) {
		return true, "run-state live"
	}
	return false, "stale"
}

func writeExecutionCleanupCandidate(t *testing.T, dir string, meta ExecutionCleanupMetadata, files map[string]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, WriteExecutionCleanupMetadata(dir, meta))
	for name, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644))
	}
}

func writeExecutionCleanupCandidateWithoutMetadata(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644))
	}
}

func setupExecutionCleanupProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ddxroot.DirName), 0o755))
	return root
}

func TestExecutionCleanup_RemovesStaleUnregisteredDDXTempDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260506T154739-deadbeef")
	livePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-20260506T154739-feedface")
	otherPath := filepath.Join(tempRoot, "plain-tmp-dir")

	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260506T154739-deadbeef",
		WorktreePath: stalePath,
	}, map[string]string{"scratch.txt": "stale\n"})
	writeExecutionCleanupCandidate(t, livePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live",
		AttemptID:    "20260506T154739-feedface",
		WorktreePath: livePath,
	}, map[string]string{"scratch.txt": "live\n"})
	require.NoError(t, os.MkdirAll(otherPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(otherPath, "ignore.txt"), []byte("keep\n"), 0o644))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.Probe = executionCleanupTestProbe{
		live: map[string]bool{
			filepath.Clean(livePath): true,
		},
	}

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, livePath)
	assert.DirExists(t, otherPath)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.NotZero(t, summary.BytesReclaimed)
	assert.NotZero(t, summary.InodesReclaimed)
	require.NotEmpty(t, summary.Observations)
	assert.True(t, hasObservationClass(summary.Observations, "removed_unregistered_temp_dir"))
}

func TestExecutionCleanup_RemovesMetadataLessUnregisteredDDXTempDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-metadata-less-20260506T154739-deadbeef")
	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-no-meta-20260506T154739-feedface")
	writeExecutionCleanupCandidateWithoutMetadata(t, stalePath, map[string]string{"scratch.txt": "stale\n"})
	writeExecutionCleanupCandidateWithoutMetadata(t, activePath, map[string]string{"scratch.txt": "active\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-active-no-meta",
		AttemptID:    "20260506T154739-feedface",
		StartedAt:    time.Now().UTC(),
		WorktreePath: activePath,
	}))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, activePath)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_unregistered_temp_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))
}

func TestExecutionCleanup_PreservesRegisteredMetadataLessWorktree(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	worktreePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-registered-no-meta-20260506T154739-c001d00d")
	writeExecutionCleanupCandidateWithoutMetadata(t, worktreePath, map[string]string{"scratch.txt": "registered\n"})

	gitOps := &executionCleanupTestGitOps{
		worktrees: []string{worktreePath},
	}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, worktreePath)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, 0, gitOps.pruneCalls)
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	require.NotEmpty(t, summary.Warnings)
	assert.Equal(t, "registered_missing_metadata", summary.Warnings[0].Class)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_registered_missing_metadata"))
}

func TestExecutionCleanup_RemovesRegisteredStaleWorktree(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	worktreePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-registered-20260506T154739-c001d00d")
	writeExecutionCleanupCandidate(t, worktreePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-registered",
		AttemptID:    "20260506T154739-c001d00d",
		WorktreePath: worktreePath,
	}, map[string]string{"scratch.txt": "registered\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-registered",
		AttemptID:    "20260506T154739-c001d00d",
		StartedAt:    time.Now().UTC(),
		WorktreePath: worktreePath,
	}))

	gitOps := &executionCleanupTestGitOps{
		worktrees: []string{worktreePath},
	}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot
	mgr.Probe = executionCleanupTestProbe{ignoreRunState: true}

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, worktreePath)
	assert.Equal(t, []string{worktreePath}, gitOps.removed)
	assert.Equal(t, 1, gitOps.pruneCalls)
	assert.Equal(t, int64(1), summary.RemovedRegisteredWorktrees)
	assert.NotZero(t, summary.BytesReclaimed)

	gotState, err := ReadRunState(projectRoot)
	require.NoError(t, err)
	assert.Nil(t, gotState, "stale run-state should be cleared when its worktree is reaped")
}

func TestExecutionCleanup_PreservesActiveAndPreservedAttempts(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-20260506T154739-11112222")
	preservedPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-preserved-20260506T154739-33334444")
	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260506T154739-55556666")

	writeExecutionCleanupCandidate(t, activePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-active",
		AttemptID:    "20260506T154739-11112222",
		WorktreePath: activePath,
	}, map[string]string{"scratch.txt": "active\n"})
	writeExecutionCleanupCandidate(t, preservedPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-preserved",
		AttemptID:    "20260506T154739-33334444",
		WorktreePath: preservedPath,
		Preserved:    true,
	}, map[string]string{"scratch.txt": "preserved\n"})
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260506T154739-55556666",
		WorktreePath: stalePath,
	}, map[string]string{"scratch.txt": "stale\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-active",
		AttemptID:    "20260506T154739-11112222",
		StartedAt:    time.Now().UTC(),
		WorktreePath: activePath,
	}))

	executionsDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "attempt-complete")
	require.NoError(t, os.MkdirAll(executionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "manifest.json"), []byte(`{"attempt_id":"attempt-complete"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "result.json"), []byte(`{"status":"success"}`), 0o644))

	runsDir := filepath.Join(projectRoot, ddxroot.DirName, "runs", "run-complete")
	require.NoError(t, os.MkdirAll(runsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runsDir, "record.json"), []byte(`{"run_id":"run-complete"}`), 0o644))

	refsPath := filepath.Join(projectRoot, ".git", "refs", "ddx", "iterations", "ddx-preserved")
	require.NoError(t, os.MkdirAll(refsPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(refsPath, "attempt-1"), []byte("abcdef1234567890"), 0o644))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.Probe = executionCleanupTestProbe{
		live: map[string]bool{
			filepath.Clean(activePath): true,
		},
	}

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activePath)
	assert.DirExists(t, preservedPath)
	assert.FileExists(t, filepath.Join(executionsDir, "manifest.json"))
	assert.FileExists(t, filepath.Join(executionsDir, "result.json"))
	assert.FileExists(t, filepath.Join(runsDir, "record.json"))
	assert.FileExists(t, filepath.Join(refsPath, "attempt-1"))
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.Equal(t, 2, summary.CompleteEvidenceDirs)
	assert.NotEmpty(t, summary.Observations)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_temp_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "complete_evidence"))
}

func TestExecutionCleanup_PreservesMultipleRunStateAttempts(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	activeOnePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-one-20260506T154739-11112222")
	activeTwoPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-two-20260506T154739-33334444")
	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260506T154739-55556666")

	writeExecutionCleanupCandidate(t, activeOnePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-active-one",
		AttemptID:    "20260506T154739-11112222",
		WorktreePath: activeOnePath,
	}, map[string]string{"scratch.txt": "active one\n"})
	writeExecutionCleanupCandidate(t, activeTwoPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-active-two",
		AttemptID:    "20260506T154739-33334444",
		WorktreePath: activeTwoPath,
	}, map[string]string{"scratch.txt": "active two\n"})
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260506T154739-55556666",
		WorktreePath: stalePath,
	}, map[string]string{"scratch.txt": "stale\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-active-one",
		AttemptID:    "20260506T154739-11112222",
		StartedAt:    time.Now().UTC(),
		WorktreePath: activeOnePath,
	}))
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-active-two",
		AttemptID:    "20260506T154739-33334444",
		StartedAt:    time.Now().UTC(),
		WorktreePath: activeTwoPath,
	}))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activeOnePath)
	assert.DirExists(t, activeTwoPath)
	assert.NoFileExists(t, stalePath)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	states, err := ReadRunStates(projectRoot)
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestExecutionCleanup_PreservesConcurrentLiveExecuteBeadWorktrees(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 5, 7, 20, 0, 0, 0, time.UTC)

	firstPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-one-20260507T200000-11112222")
	secondPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-two-20260507T200001-33334444")
	writeExecutionCleanupCandidate(t, firstPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live-one",
		AttemptID:    "20260507T200000-11112222",
		WorktreePath: firstPath,
		Registered:   true,
	}, map[string]string{"scratch.txt": "first\n"})
	writeExecutionCleanupCandidate(t, secondPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live-two",
		AttemptID:    "20260507T200001-33334444",
		WorktreePath: secondPath,
		Registered:   true,
	}, map[string]string{"scratch.txt": "second\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-live-one",
		AttemptID:    "20260507T200000-11112222",
		StartedAt:    now,
		RefreshedAt:  now,
		ExpiresAt:    now.Add(RunStateLivenessTTL),
		WorktreePath: firstPath,
	}))
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-live-two",
		AttemptID:    "20260507T200001-33334444",
		StartedAt:    now,
		RefreshedAt:  now,
		ExpiresAt:    now.Add(RunStateLivenessTTL),
		WorktreePath: secondPath,
	}))

	gitOps := &executionCleanupTestGitOps{worktrees: []string{firstPath, secondPath}}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, firstPath)
	assert.DirExists(t, secondPath)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	assert.GreaterOrEqual(t, countObservationClass(summary.Observations, "preserved_live_attempt"), 2)
	states, err := ReadRunStates(projectRoot)
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestExecutionCleanup_RemovesExpiredAttemptOnlyAfterHeartbeatExpiry(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 5, 7, 20, 30, 0, 0, time.UTC)

	worktreePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-expiring-20260507T203000-11112222")
	meta := ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-expiring",
		AttemptID:    "20260507T203000-11112222",
		WorktreePath: worktreePath,
		Registered:   true,
		Liveness: &ExecutionCleanupLiveness{
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Minute),
		},
	}
	writeExecutionCleanupCandidate(t, worktreePath, meta, map[string]string{"scratch.txt": "active\n"})

	gitOps := &executionCleanupTestGitOps{worktrees: []string{worktreePath}}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)
	assert.DirExists(t, worktreePath)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))

	meta.Liveness.RefreshedAt = now.Add(-10 * time.Minute)
	meta.Liveness.ExpiresAt = now.Add(-time.Minute)
	require.NoError(t, WriteExecutionCleanupMetadata(worktreePath, meta))
	summary, err = mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, worktreePath)
	assert.Equal(t, []string{worktreePath}, gitOps.removed)
	assert.Equal(t, int64(1), summary.RemovedRegisteredWorktrees)
	assert.True(t, hasObservationClass(summary.Observations, "removed_registered_worktree"))
}

func TestExecutionCleanup_DoesNotDeleteRegisteredActiveWorktree(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	parent := t.TempDir()
	tempRoot := filepath.Join(parent, "ddx-exec-wt")
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
	now := time.Date(2026, 5, 7, 21, 0, 0, 0, time.UTC)

	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-20260507T210000-feedface")
	writeExecutionCleanupCandidate(t, activePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-active",
		AttemptID:    "20260507T210000-feedface",
		WorktreePath: activePath,
		Registered:   true,
		Liveness: &ExecutionCleanupLiveness{
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Minute),
		},
	}, map[string]string{"scratch.txt": "active\n"})

	staleScratch := filepath.Join(parent, "ddx-test-stale-scratch")
	require.NoError(t, os.MkdirAll(staleScratch, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleScratch, "payload.txt"), []byte("stale\n"), 0o644))
	old := now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(staleScratch, old, old))

	gitOps := &executionCleanupTestGitOps{worktrees: []string{activePath}}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot
	mgr.ScratchRoots = []string{parent}
	mgr.ScratchMinAge = time.Hour
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activePath)
	assert.NoFileExists(t, staleScratch)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	assert.Equal(t, int64(1), summary.RemovedScratchDirs)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))
	assert.True(t, hasObservationClass(summary.Observations, "removed_scratch_dir"))
}

func TestExecutionCleanup_ReportsPreservedLiveAttempt(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 5, 7, 21, 30, 0, 0, time.UTC)

	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-report-live-20260507T213000-feedface")
	writeExecutionCleanupCandidate(t, activePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-report-live",
		AttemptID:    "20260507T213000-feedface",
		WorktreePath: activePath,
		Registered:   true,
		Liveness: &ExecutionCleanupLiveness{
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Minute),
		},
	}, map[string]string{"scratch.txt": "active\n"})

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{worktrees: []string{activePath}})
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	require.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))
	var message string
	for _, obs := range summary.Observations {
		if obs.Class == "preserved_live_attempt" {
			message = obs.Message
			break
		}
	}
	assert.Equal(t, "unexpired liveness", message)
}

func TestExecutionCleanup_ReportsSummary(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-summary-20260506T154739-99990000")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-summary",
		AttemptID:    "20260506T154739-99990000",
		WorktreePath: stalePath,
	}, map[string]string{"payload.txt": strings.Repeat("x", 64)})

	executionsDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "attempt-summary")
	require.NoError(t, os.MkdirAll(executionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "manifest.json"), []byte(`{"attempt_id":"attempt-summary"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "result.json"), []byte(`{"status":"success"}`), 0o644))

	gitOps := &executionCleanupTestGitOps{listErr: errors.New("worktree list unavailable")}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.NotZero(t, summary.BytesReclaimed)
	assert.NotZero(t, summary.InodesReclaimed)
	require.NotEmpty(t, summary.Warnings)
	assert.Equal(t, "git_worktree_list", summary.Warnings[0].Class)
	require.NotEmpty(t, summary.Observations)
	assert.True(t, hasObservationClass(summary.Observations, "removed_unregistered_temp_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "complete_evidence"))
}

func TestExecutionCleanup_DryRunLeavesCandidatesOnDisk(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-dryrun-20260506T154739-abcdef01")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-dryrun",
		AttemptID:    "20260506T154739-abcdef01",
		WorktreePath: stalePath,
	}, map[string]string{"scratch.txt": "dry-run\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-dryrun",
		AttemptID:    "20260506T154739-live-abcdef01",
		StartedAt:    time.Now().UTC(),
		WorktreePath: filepath.Join(tempRoot, "missing-live-path"),
	}))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.DryRun = true

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, stalePath)
	gotState, err := ReadRunState(projectRoot)
	require.NoError(t, err)
	require.NotNil(t, gotState)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.Equal(t, int64(1), summary.RemovedRunStateFiles)
	assert.True(t, hasObservationClass(summary.Observations, "would_remove_unregistered_temp_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "would_remove_run_state"))
}

func TestExecutionCleanup_RemovesStaleDDXScratchDirs(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	scratchRoot := t.TempDir()

	stalePath := filepath.Join(scratchRoot, "ddx-test-scratch-stale")
	nonDDXPath := filepath.Join(scratchRoot, "plain-old-dir")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		WorktreePath: stalePath,
	}, map[string]string{"payload.txt": strings.Repeat("x", 32)})
	require.NoError(t, os.MkdirAll(nonDDXPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nonDDXPath, "keep.txt"), []byte("keep\n"), 0o644))
	old := now.Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stalePath, old, old))
	require.NoError(t, os.Chtimes(nonDDXPath, old, old))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.ScratchRoots = []string{scratchRoot}
	mgr.ScratchMinAge = time.Hour
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, nonDDXPath)
	assert.Equal(t, 1, summary.ScannedScratchDirs)
	assert.Equal(t, int64(1), summary.RemovedScratchDirs)
	assert.Equal(t, int64(0), summary.RemovedUnregisteredTempDirs)
	assert.Greater(t, summary.ScratchBytesReclaimed, int64(0))
	assert.Greater(t, summary.ScratchInodesReclaimed, int64(0))
	assert.True(t, hasObservationClass(summary.Observations, "removed_scratch_dir"))
}

func TestExecutionCleanup_PreservesActiveDDXScratchDirs(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	scratchRoot := t.TempDir()

	livePath := filepath.Join(scratchRoot, "ddx-e2e-live")
	freshPath := filepath.Join(scratchRoot, "ddx-test-fresh")
	nonDDXPath := filepath.Join(scratchRoot, "plain-old-dir")
	writeExecutionCleanupCandidate(t, livePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		WorktreePath: livePath,
		Liveness: &ExecutionCleanupLiveness{
			ExpiresAt: now.Add(time.Hour),
		},
	}, map[string]string{"payload.txt": "live\n"})
	writeExecutionCleanupCandidateWithoutMetadata(t, freshPath, map[string]string{"payload.txt": "fresh\n"})
	require.NoError(t, os.MkdirAll(nonDDXPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nonDDXPath, "keep.txt"), []byte("keep\n"), 0o644))
	old := now.Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(livePath, old, old))
	require.NoError(t, os.Chtimes(nonDDXPath, old, old))
	require.NoError(t, os.Chtimes(freshPath, now.Add(-10*time.Minute), now.Add(-10*time.Minute)))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.ScratchRoots = []string{scratchRoot}
	mgr.ScratchMinAge = time.Hour
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, livePath)
	assert.DirExists(t, freshPath)
	assert.DirExists(t, nonDDXPath)
	assert.Equal(t, 2, summary.ScannedScratchDirs)
	assert.Equal(t, int64(0), summary.RemovedScratchDirs)
	assert.Equal(t, int64(2), summary.PreservedActiveScratchDirs)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_active_scratch_dir"))
}

func TestExecutionCleanup_ReclaimsExpiredTestOwnedWorktrees(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	staleProjectRoot := filepath.Join(t.TempDir(), "stale-project")
	activeProjectRoot := filepath.Join(t.TempDir(), "active-project")
	require.NoError(t, os.MkdirAll(filepath.Join(staleProjectRoot, ddxroot.DirName), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(activeProjectRoot, ddxroot.DirName), 0o755))

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-foreign-20260508T120000-deadbeef")
	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-active-foreign-20260508T120000-feedface")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  staleProjectRoot,
		BeadID:       "ddx-stale-foreign",
		AttemptID:    "20260508T120000-deadbeef",
		WorktreePath: stalePath,
		Registered:   true,
	}, map[string]string{"scratch.txt": "stale foreign\n"})
	writeExecutionCleanupCandidate(t, activePath, ExecutionCleanupMetadata{
		ProjectRoot:  activeProjectRoot,
		BeadID:       "ddx-active-foreign",
		AttemptID:    "20260508T120000-feedface",
		WorktreePath: activePath,
		Registered:   true,
	}, map[string]string{"scratch.txt": "active foreign\n"})

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{
		worktrees: []string{activePath},
	})
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, activePath)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_unregistered_temp_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_foreign_registered_worktree"))
}

func TestExecutionCleanup_PreservesActiveAttemptCycle(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 5, 9, 2, 0, 0, 0, time.UTC)

	activePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-cycle-active-20260509T020000-feedface")
	writeExecutionCleanupCandidate(t, activePath, ExecutionCleanupMetadata{
		ProjectRoot:         projectRoot,
		BeadID:              "ddx-cycle-active",
		AttemptID:           "20260509T020000-feedface",
		WorktreePath:        activePath,
		Registered:          true,
		CandidateCyclePhase: "review",
		CandidateRef:        "refs/ddx/iterations/20260509T020000-feedface/0",
		CandidateRev:        "abc123",
		CycleIndex:          0,
		ReviewActive:        true,
	}, map[string]string{"scratch.txt": "active\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:              "ddx-cycle-active",
		AttemptID:           "20260509T020000-feedface",
		StartedAt:           now,
		RefreshedAt:         now,
		ExpiresAt:           now.Add(RunStateLivenessTTL),
		WorktreePath:        activePath,
		CandidateCyclePhase: "review",
		CandidateRef:        "refs/ddx/iterations/20260509T020000-feedface/0",
		CandidateRev:        "abc123",
		ReviewActive:        true,
	}))

	gitOps := &executionCleanupTestGitOps{worktrees: []string{activePath}}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activePath)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))
	assert.Contains(t, firstObservationMessage(summary.Observations, "preserved_live_attempt"), "phase=review")
}

func TestExecutionCleanup_RecoversPinnedCandidateAfterCrash(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	crashedPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-cycle-crashed-20260509T021000-cafed00d")
	candidateRef := "refs/ddx/iterations/20260509T021000-cafed00d/1"
	writeExecutionCleanupCandidate(t, crashedPath, ExecutionCleanupMetadata{
		ProjectRoot:         projectRoot,
		BeadID:              "ddx-cycle-crashed",
		AttemptID:           "20260509T021000-cafed00d",
		WorktreePath:        crashedPath,
		CandidateCyclePhase: "repair",
		CandidateRef:        candidateRef,
		CandidateRev:        "def456",
		CycleIndex:          1,
		RepairActive:        true,
	}, map[string]string{"scratch.txt": "crashed\n"})

	gitOps := &executionCleanupTestGitOps{}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, crashedPath)
	assert.Empty(t, gitOps.removed)
	assert.Equal(t, int64(0), summary.RemovedUnregisteredTempDirs)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_live_attempt"))
	msg := firstObservationMessage(summary.Observations, "preserved_live_attempt")
	assert.Contains(t, msg, "candidate_ref="+candidateRef)
	assert.Contains(t, msg, "repair_active=true")
}

func TestExecutionCleanup_ReclaimsStaleUnpinnedCandidateWorktree(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-cycle-stale-20260509T022000-deadbeef")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:         projectRoot,
		BeadID:              "ddx-cycle-stale",
		AttemptID:           "20260509T022000-deadbeef",
		WorktreePath:        stalePath,
		CandidateCyclePhase: "review",
		CandidateRev:        "abc999",
		CycleIndex:          0,
	}, map[string]string{"scratch.txt": "stale\n"})

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, stalePath)
	assert.Equal(t, int64(1), summary.RemovedUnregisteredTempDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_unregistered_temp_dir"))
}

func TestExecutionCleanup_PrunesTemporaryCandidateRefs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	candidateRef := "refs/ddx/iterations/20260509T023000-aabbccdd/0"
	retainedRef := "refs/ddx/iterations/20260509T023100-eeff0011/0"

	resultDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "20260509T023000-aabbccdd")
	require.NoError(t, os.MkdirAll(resultDir, 0o755))
	require.NoError(t, writeArtifactJSON(filepath.Join(resultDir, "manifest.json"), map[string]string{"attempt_id": "20260509T023000-aabbccdd"}))
	require.NoError(t, writeArtifactJSON(filepath.Join(resultDir, "result.json"), map[string]any{
		"status":        ExecuteBeadStatusSuccess,
		"candidate_ref": candidateRef,
		"cycle_index":   0,
		"attempt_id":    "20260509T023000-aabbccdd",
		"base_rev":      "base",
		"result_rev":    "result",
	}))

	retainedDir := filepath.Join(projectRoot, ddxroot.DirName, "executions", "20260509T023100-eeff0011")
	require.NoError(t, os.MkdirAll(retainedDir, 0o755))
	require.NoError(t, writeArtifactJSON(filepath.Join(retainedDir, "manifest.json"), map[string]string{"attempt_id": "20260509T023100-eeff0011"}))
	require.NoError(t, writeArtifactJSON(filepath.Join(retainedDir, "result.json"), map[string]any{
		"status":        ExecuteBeadStatusLandConflict,
		"candidate_ref": retainedRef,
		"cycle_index":   0,
		"attempt_id":    "20260509T023100-eeff0011",
		"base_rev":      "base",
		"result_rev":    "result",
	}))

	gitOps := &executionCleanupTestGitOps{}
	mgr := NewExecutionCleanupManager(projectRoot, gitOps)
	mgr.TempRoot = tempRoot

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{candidateRef}, gitOps.deletedRefs)
	assert.Equal(t, int64(1), summary.PrunedCandidateRefs)
	assert.True(t, hasObservationClass(summary.Observations, "pruned_candidate_ref"))
}

func setupEvidenceDir(t *testing.T, projectRoot, attemptID string, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(projectRoot, ddxroot.DirName, "executions", attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"status":"success"}`), 0o644))
	require.NoError(t, os.Chtimes(dir, mtime, mtime))
	return dir
}

func TestExecutionCleanup_DeletesOldDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	oldTime := time.Now().AddDate(0, 0, -10)
	oldDir := setupEvidenceDir(t, projectRoot, "20260101T000000-deadbeef", oldTime)

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoDirExists(t, oldDir)
	assert.Equal(t, int64(1), summary.RemovedEvidenceDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_evidence_dir"))
}

func TestExecutionCleanup_PreservesActiveDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	oldTime := time.Now().AddDate(0, 0, -10)
	activeAttemptID := "20260101T000000-aabbccdd"
	activeDir := setupEvidenceDir(t, projectRoot, activeAttemptID, oldTime)

	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:    "ddx-active",
		AttemptID: activeAttemptID,
		StartedAt: time.Now().UTC(),
	}))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activeDir)
	assert.Equal(t, int64(0), summary.RemovedEvidenceDirs)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_active_evidence_dir"))
}

func TestExecutionCleanup_DefaultRetainDays90WithoutConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := setupExecutionCleanupProjectRoot(t)
	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	assert.Equal(t, defaultEvidenceRetainDays, mgr.RetainDays)
	assert.Equal(t, 90, mgr.RetainDays)
}

func TestExecutionCleanup_RetainDaysZero_DisablesRetention(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	oldTime := time.Now().AddDate(0, 0, -10)
	oldDir := setupEvidenceDir(t, projectRoot, "20260101T000000-eeff0011", oldTime)

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 0 // disabled

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, oldDir)
	assert.Equal(t, int64(0), summary.RemovedEvidenceDirs)
}

func TestExecutionCleanup_RecordsCount(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	oldTime := time.Now().AddDate(0, 0, -10)
	for _, id := range []string{"20260101T000000-aaaa1111", "20260101T000001-bbbb2222", "20260101T000002-cccc3333"} {
		setupEvidenceDir(t, projectRoot, id, oldTime)
	}

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(3), summary.RemovedEvidenceDirs)
}

func TestExecutionCleanup_DeletesOldExecutionDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()

	oldTime := time.Now().AddDate(0, 0, -20)
	oldDir := setupEvidenceDir(t, projectRoot, "20260101T000000-exec-old", oldTime)
	recentDir := setupEvidenceDir(t, projectRoot, "20260101T000001-exec-recent", time.Now())

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoDirExists(t, oldDir)
	assert.DirExists(t, recentDir)
	assert.Equal(t, int64(1), summary.RemovedEvidenceDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_evidence_dir"))
}

func TestExecutionCleanup_DeletesOldAgentLogs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	logDir := filepath.Join(projectRoot, ddxroot.DirName, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	oldTime := time.Now().AddDate(0, 0, -20)
	oldAgent := filepath.Join(logDir, "agent-oldsession.jsonl")
	oldSvc := filepath.Join(logDir, "svc-oldsession.jsonl")
	recentAgent := filepath.Join(logDir, "agent-recentsession.jsonl")
	otherFile := filepath.Join(logDir, "mirror.log")

	for _, p := range []string{oldAgent, oldSvc, recentAgent, otherFile} {
		require.NoError(t, os.WriteFile(p, []byte("data\n"), 0o644))
	}
	require.NoError(t, os.Chtimes(oldAgent, oldTime, oldTime))
	require.NoError(t, os.Chtimes(oldSvc, oldTime, oldTime))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, oldAgent)
	assert.NoFileExists(t, oldSvc)
	assert.FileExists(t, recentAgent)
	assert.FileExists(t, otherFile)
	assert.Equal(t, int64(2), summary.RemovedAgentLogs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_agent_log"))
}

func TestExecutionCleanup_DeletesOldWorkerDirs(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	workersDir := filepath.Join(projectRoot, ddxroot.DirName, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))

	oldTime := time.Now().AddDate(0, 0, -20)
	oldWorker := filepath.Join(workersDir, "worker-old")
	recentWorker := filepath.Join(workersDir, "worker-recent")
	require.NoError(t, os.MkdirAll(oldWorker, 0o755))
	require.NoError(t, os.MkdirAll(recentWorker, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(oldWorker, "status.json"), []byte(`{"id":"worker-old","state":"exited"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(recentWorker, "status.json"), []byte(`{"id":"worker-recent","state":"running"}`), 0o644))
	require.NoError(t, os.Chtimes(oldWorker, oldTime, oldTime))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.NoDirExists(t, oldWorker)
	assert.DirExists(t, recentWorker)
	assert.Equal(t, int64(1), summary.RemovedWorkerDirs)
	assert.True(t, hasObservationClass(summary.Observations, "removed_worker_dir"))
}

func TestExecutionCleanup_PreservesActiveItems(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	oldTime := time.Now().AddDate(0, 0, -20)
	activeSessionID := "activesession123"

	// execution dir: active via run-state
	activeExecID := "20260101T000000-exec-active"
	activeExecDir := setupEvidenceDir(t, projectRoot, activeExecID, oldTime)
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:    "ddx-active",
		AttemptID: activeExecID,
		SessionID: activeSessionID,
		StartedAt: time.Now().UTC(),
	}))

	// agent log: same session ID as active run-state
	logDir := filepath.Join(projectRoot, ddxroot.DirName, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	activeLog := filepath.Join(logDir, "agent-"+activeSessionID+".jsonl")
	require.NoError(t, os.WriteFile(activeLog, []byte("data\n"), 0o644))
	require.NoError(t, os.Chtimes(activeLog, oldTime, oldTime))

	// worker dir: PID 0 but not old enough to be pruned (use recent mtime to preserve)
	workersDir := filepath.Join(projectRoot, ddxroot.DirName, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))
	preservedWorker := filepath.Join(workersDir, "worker-preserved")
	require.NoError(t, os.MkdirAll(preservedWorker, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(preservedWorker, "status.json"), []byte(`{"id":"worker-preserved","state":"running","pid":0}`), 0o644))
	// keep recent mtime so it is not pruned by age

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, activeExecDir)
	assert.FileExists(t, activeLog)
	assert.DirExists(t, preservedWorker)
	assert.Equal(t, int64(0), summary.RemovedEvidenceDirs)
	assert.Equal(t, int64(0), summary.RemovedAgentLogs)
	assert.Equal(t, int64(0), summary.RemovedWorkerDirs)
	assert.True(t, hasObservationClass(summary.Observations, "preserved_active_evidence_dir"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_active_agent_log"))
}

func TestExecutionCleanup_DefaultRetainDays90(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := setupExecutionCleanupProjectRoot(t)
	configPath := filepath.Join(projectRoot, ddxroot.DirName, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("version: \"1.0\"\n"), 0o644))

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	assert.Equal(t, 90, mgr.RetainDays)
}

func TestExecutionCleanup_RecordsCounts(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	oldTime := time.Now().AddDate(0, 0, -20)

	// 2 old execution dirs
	for _, id := range []string{"20260101T000000-cnt-exec1", "20260101T000001-cnt-exec2"} {
		setupEvidenceDir(t, projectRoot, id, oldTime)
	}

	// 3 old agent logs
	logDir := filepath.Join(projectRoot, ddxroot.DirName, "agent-logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	for _, name := range []string{"agent-s1.jsonl", "agent-s2.jsonl", "svc-s1.jsonl"} {
		p := filepath.Join(logDir, name)
		require.NoError(t, os.WriteFile(p, []byte("data\n"), 0o644))
		require.NoError(t, os.Chtimes(p, oldTime, oldTime))
	}

	// 2 old worker dirs
	workersDir := filepath.Join(projectRoot, ddxroot.DirName, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))
	for _, id := range []string{"worker-cnt1", "worker-cnt2"} {
		d := filepath.Join(workersDir, id)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "status.json"), []byte(`{"id":"`+id+`","state":"exited"}`), 0o644))
		require.NoError(t, os.Chtimes(d, oldTime, oldTime))
	}

	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.RetainDays = 7

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(2), summary.RemovedEvidenceDirs)
	assert.Equal(t, int64(3), summary.RemovedAgentLogs)
	assert.Equal(t, int64(2), summary.RemovedWorkerDirs)
}

func hasObservationClass(observations []ExecutionCleanupObservation, class string) bool {
	for _, obs := range observations {
		if obs.Class == class {
			return true
		}
	}
	return false
}

func firstObservationMessage(observations []ExecutionCleanupObservation, class string) string {
	for _, obs := range observations {
		if obs.Class == class {
			return obs.Message
		}
	}
	return ""
}

func countObservationClass(observations []ExecutionCleanupObservation, class string) int {
	count := 0
	for _, obs := range observations {
		if obs.Class == class {
			count++
		}
	}
	return count
}
