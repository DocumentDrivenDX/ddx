package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executionCleanupTestGitOps struct {
	worktrees  []string
	removed    []string
	pruneCalls int
	listErr    error
	removeErr  error
	pruneErr   error
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
func (g *executionCleanupTestGitOps) DeleteRef(string, string) error         { return nil }

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
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ddx"), 0o755))
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
	assert.True(t, hasObservationClass(summary.Observations, "preserved_temp_dir"))
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

	executionsDir := filepath.Join(projectRoot, ".ddx", "executions", "attempt-complete")
	require.NoError(t, os.MkdirAll(executionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "manifest.json"), []byte(`{"attempt_id":"attempt-complete"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(executionsDir, "result.json"), []byte(`{"status":"success"}`), 0o644))

	runsDir := filepath.Join(projectRoot, ".ddx", "runs", "run-complete")
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

	executionsDir := filepath.Join(projectRoot, ".ddx", "executions", "attempt-summary")
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
	require.NoError(t, os.MkdirAll(filepath.Join(staleProjectRoot, ".ddx"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(activeProjectRoot, ".ddx"), 0o755))

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

func hasObservationClass(observations []ExecutionCleanupObservation, class string) bool {
	for _, obs := range observations {
		if obs.Class == class {
			return true
		}
	}
	return false
}
