package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExecutionCleanupAttemptProcessScanner struct {
	processes []executionCleanupAttemptProcess
	err       error
}

func (f fakeExecutionCleanupAttemptProcessScanner) Scan(context.Context) ([]executionCleanupAttemptProcess, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]executionCleanupAttemptProcess, len(f.processes))
	copy(out, f.processes)
	return out, nil
}

type fakeExecutionCleanupAttemptProcessKiller struct {
	killed []int
	err    error
}

func (f *fakeExecutionCleanupAttemptProcessKiller) KillGroup(pgid int) error {
	if f.err != nil {
		return f.err
	}
	f.killed = append(f.killed, pgid)
	return nil
}

type cleanupProcessSidecarProbe struct {
	sidecarAttempts map[string]struct{}
}

func (p cleanupProcessSidecarProbe) IsLive(meta ExecutionCleanupMetadata, runState *RunState, now time.Time) (bool, string) {
	live, reason := defaultExecutionCleanupLivenessProbe{}.IsLive(meta, runState, now)
	if live {
		return true, reason
	}
	if meta.AttemptID != "" {
		if _, ok := p.sidecarAttempts[meta.AttemptID]; ok {
			return true, "live worker sidecar"
		}
	}
	return false, reason
}

func TestExecutionCleanupManager_DryRunReportsStaleAttemptDescendantProcesses(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260608T120000-deadbeef")
	livePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-20260608T120000-feedface")

	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260608T120000-deadbeef",
		WorktreePath: stalePath,
	}, map[string]string{"payload.txt": "stale\n"})
	writeExecutionCleanupCandidate(t, livePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live",
		AttemptID:    "20260608T120000-feedface",
		WorktreePath: livePath,
		Liveness: &ExecutionCleanupLiveness{
			PID:         os.Getpid(),
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		},
	}, map[string]string{"payload.txt": "live\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-live",
		AttemptID:    "20260608T120000-feedface",
		StartedAt:    now.Add(-5 * time.Minute),
		WorktreePath: livePath,
		PID:          os.Getpid(),
	}))

	killer := &fakeExecutionCleanupAttemptProcessKiller{}
	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.DryRun = true
	mgr.Now = func() time.Time { return now }
	mgr.attemptProcessScanner = fakeExecutionCleanupAttemptProcessScanner{
		processes: []executionCleanupAttemptProcess{
			{
				PID:       1111,
				PPID:      1,
				PGID:      1111,
				Command:   "sh -lc cargo test",
				Cwd:       stalePath,
				Worktree:  stalePath,
				StartedAt: now.Add(-2 * time.Hour),
			},
			{
				PID:       1112,
				PPID:      1111,
				PGID:      1111,
				Command:   "cargo test --workspace",
				Cwd:       filepath.Join(stalePath, "src"),
				Worktree:  stalePath,
				StartedAt: now.Add(-2 * time.Hour),
			},
			{
				PID:       2222,
				PPID:      1,
				PGID:      2222,
				Command:   "bash -lc sleep 60",
				Cwd:       livePath,
				Worktree:  livePath,
				StartedAt: now.Add(-10 * time.Minute),
			},
		},
	}
	mgr.attemptProcessKiller = killer

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	require.Len(t, summary.ProcessFindings, 1)
	finding := summary.ProcessFindings[0]
	assert.Equal(t, 1111, finding.PID)
	assert.Equal(t, 1111, finding.PGID)
	assert.Equal(t, stalePath, finding.Worktree)
	assert.Equal(t, "sh -lc cargo test", finding.Command)
	assert.NotEmpty(t, finding.StaleReason)
	assert.True(t, finding.WouldKill)
	assert.False(t, finding.Terminated)
	assert.Len(t, finding.Members, 2)
	assert.Empty(t, killer.killed, "dry-run must not call the killer")
}

func TestExecutionCleanupManager_ApplyReapsOnlyStaleAttemptProcessGroups(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	otherTempRoot := t.TempDir()
	now := time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC)

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260608T130000-deadbeef")
	livePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-20260608T130000-feedface")
	registeredPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-registered-20260608T130000-c001d00d")
	otherPath := filepath.Join(otherTempRoot, ExecuteBeadWtPrefix+"ddx-other-20260608T130000-abcd1234")
	require.NoError(t, os.MkdirAll(otherPath, 0o755))

	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260608T130000-deadbeef",
		WorktreePath: stalePath,
	}, map[string]string{"payload.txt": "stale\n"})
	writeExecutionCleanupCandidate(t, livePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live",
		AttemptID:    "20260608T130000-feedface",
		WorktreePath: livePath,
		Liveness: &ExecutionCleanupLiveness{
			PID:         os.Getpid(),
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		},
	}, map[string]string{"payload.txt": "live\n"})
	writeExecutionCleanupCandidate(t, registeredPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-registered",
		AttemptID:    "20260608T130000-c001d00d",
		WorktreePath: registeredPath,
		Registered:   true,
		Liveness: &ExecutionCleanupLiveness{
			PID:         os.Getpid(),
			RefreshedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		},
	}, map[string]string{"payload.txt": "registered\n"})
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-live",
		AttemptID:    "20260608T130000-feedface",
		StartedAt:    now.Add(-5 * time.Minute),
		WorktreePath: livePath,
		PID:          os.Getpid(),
	}))

	killer := &fakeExecutionCleanupAttemptProcessKiller{}
	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{worktrees: []string{registeredPath}})
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }
	mgr.attemptProcessScanner = fakeExecutionCleanupAttemptProcessScanner{
		processes: []executionCleanupAttemptProcess{
			{
				PID:       3001,
				PPID:      1,
				PGID:      3001,
				Command:   "sh -lc cargo test",
				Cwd:       stalePath,
				Worktree:  stalePath,
				StartedAt: now.Add(-3 * time.Hour),
			},
			{
				PID:       3002,
				PPID:      3001,
				PGID:      3001,
				Command:   "cargo test --workspace",
				Cwd:       filepath.Join(stalePath, "target"),
				Worktree:  stalePath,
				StartedAt: now.Add(-3 * time.Hour),
			},
			{
				PID:       4001,
				PPID:      1,
				PGID:      4001,
				Command:   "bash -lc sleep 60",
				Cwd:       livePath,
				Worktree:  livePath,
				StartedAt: now.Add(-15 * time.Minute),
			},
			{
				PID:       5001,
				PPID:      1,
				PGID:      5001,
				Command:   "python -m http.server 9999",
				Cwd:       registeredPath,
				Worktree:  registeredPath,
				StartedAt: now.Add(-20 * time.Minute),
			},
			{
				PID:       6001,
				PPID:      1,
				PGID:      6001,
				Command:   "sh -lc sleep 60",
				Cwd:       otherPath,
				Worktree:  otherPath,
				StartedAt: now.Add(-20 * time.Minute),
			},
		},
	}
	mgr.attemptProcessKiller = killer

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	require.Len(t, summary.ProcessFindings, 1)
	finding := summary.ProcessFindings[0]
	assert.Equal(t, 3001, finding.PID)
	assert.Equal(t, 3001, finding.PGID)
	assert.Equal(t, stalePath, finding.Worktree)
	assert.True(t, finding.WouldKill)
	assert.True(t, finding.Terminated)
	assert.NotEmpty(t, finding.StaleReason)
	assert.ElementsMatch(t, []int{3001}, killer.killed)
	assert.Equal(t, []int{3001}, killer.killed)
	assert.NoFileExists(t, stalePath)
	assert.DirExists(t, livePath)
	assert.DirExists(t, registeredPath)
	assert.DirExists(t, otherPath)
}

// TestExecutionCleanupManager_DetectsNonHarnessDescendantByOwnershipEvidence
// proves that non-harness commands (sh, cargo, python http.server) are detected
// as stale because their cwd/worktree falls within a stale DDx execution
// worktree, not by matching the command name. A harness-named process with no
// DDx worktree association is NOT reported.
func TestExecutionCleanupManager_DetectsNonHarnessDescendantByOwnershipEvidence(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 6, 8, 15, 0, 0, 0, time.UTC)

	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260608T150000-aabbccdd")
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260608T150000-aabbccdd",
		WorktreePath: stalePath,
	}, map[string]string{"payload.txt": "stale\n"})

	// Write a run state for a different bead to trigger the process scan.
	// It must not match the stale path so the probe classifies it as stale.
	require.NoError(t, WriteRunState(projectRoot, RunState{
		BeadID:       "ddx-other",
		AttemptID:    "20260608T150000-other001",
		StartedAt:    now.Add(-24 * time.Hour),
		WorktreePath: "/nonexistent-probe-trigger",
		PID:          999999,
	}))

	killer := &fakeExecutionCleanupAttemptProcessKiller{}
	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{})
	mgr.TempRoot = tempRoot
	mgr.DryRun = true
	mgr.Now = func() time.Time { return now }
	mgr.attemptProcessScanner = fakeExecutionCleanupAttemptProcessScanner{
		processes: []executionCleanupAttemptProcess{
			// Non-harness commands detected by cwd/worktree ownership, not command name.
			{
				PID:       7001,
				PPID:      1,
				PGID:      7001,
				Command:   "sh -c cargo test --workspace",
				Cwd:       stalePath,
				Worktree:  stalePath,
				StartedAt: now.Add(-3 * time.Hour),
			},
			{
				PID:       7002,
				PPID:      7001,
				PGID:      7001,
				Command:   "cargo test --workspace",
				Cwd:       filepath.Join(stalePath, "target"),
				Worktree:  stalePath,
				StartedAt: now.Add(-3 * time.Hour),
			},
			{
				PID:       7003,
				PPID:      7001,
				PGID:      7001,
				Command:   "python -m http.server 9999",
				Cwd:       stalePath,
				Worktree:  stalePath,
				StartedAt: now.Add(-3 * time.Hour),
			},
			// Harness-named process with no DDx worktree: must NOT be reported.
			{
				PID:       8001,
				PPID:      1,
				PGID:      8001,
				Command:   "claude --model sonnet --worktree /home/user/project",
				Cwd:       "/home/user/project",
				Worktree:  "",
				StartedAt: now.Add(-1 * time.Hour),
			},
		},
	}
	mgr.attemptProcessKiller = killer

	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	// Exactly one stale group (PGID 7001) should be reported.
	require.Len(t, summary.ProcessFindings, 1)
	finding := summary.ProcessFindings[0]
	assert.Equal(t, 7001, finding.PID)
	assert.Equal(t, 7001, finding.PGID)
	assert.Equal(t, stalePath, finding.Worktree)
	assert.NotEmpty(t, finding.StaleReason)
	assert.True(t, finding.WouldKill)
	assert.False(t, finding.Terminated, "dry-run must not kill")
	assert.Len(t, finding.Members, 3)
	assert.Empty(t, killer.killed, "dry-run must not call the killer")

	// Harness-named process outside any DDx worktree must not appear.
	for _, f := range summary.ProcessFindings {
		assert.NotEqual(t, 8001, f.PID, "harness-named process outside DDx worktree must not be reported")
	}
}

// TestStaleAttemptProcessScanner_DetectsReparentedDescendantsByCwd proves the
// scanner classifies a reparented process (ppid=1) whose cwd sits under the
// configured execution worktree root (e.g. ~/.cache/ddx/exec-wt/) as DDx-owned
// even when the cwd does not contain the `.execute-bead-wt-*` segment.
func TestStaleAttemptProcessScanner_DetectsReparentedDescendantsByCwd(t *testing.T) {
	tempRoot := filepath.Join(t.TempDir(), "exec-wt")
	require.NoError(t, os.MkdirAll(tempRoot, 0o755))
	// A reparented descendant whose cwd lives under tempRoot but does NOT
	// contain the explicit `.execute-bead-wt-*` segment in its leaf.
	descendantCwd := filepath.Join(tempRoot, "scratch-2026-orphan")
	require.NoError(t, os.MkdirAll(descendantCwd, 0o755))

	startedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	proc := executionCleanupAttemptProcessFromWorkerStatus(
		"some-provider --child", descendantCwd, 4242, 1, 4242, startedAt, tempRoot,
	)

	assert.Equal(t, 4242, proc.PID, "PID must be preserved")
	assert.Equal(t, 1, proc.PPID, "ppid=1 reparented process must be preserved")
	assert.NotEmpty(t, proc.Worktree,
		"reparented descendant whose cwd is under tempRoot must be classified as DDx-owned")
	assert.Truef(t, isPathWithin(proc.Worktree, tempRoot),
		"classified worktree %q must be within tempRoot %q", proc.Worktree, tempRoot)

	// A process whose cwd is outside tempRoot and has no `.execute-bead-wt-*`
	// signal must NOT be classified.
	foreign := executionCleanupAttemptProcessFromWorkerStatus(
		"unrelated-command", "/home/user/project", 5555, 1, 5555, startedAt, tempRoot,
	)
	assert.Empty(t, foreign.Worktree,
		"processes outside tempRoot without a `.execute-bead-wt-*` signal must remain unclassified")

	// The existing `.execute-bead-wt-*` classification must still be honored even
	// when tempRoot is empty (default-helper call sites that pre-date the fix).
	bareWt := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-bare-20260628T120000-cafe1234")
	bare := executionCleanupAttemptProcessFromWorkerStatus(
		"sh -c sleep", bareWt, 6666, 1, 6666, startedAt, "",
	)
	assert.NotEmpty(t, bare.Worktree,
		"`.execute-bead-wt-*` classification must remain even without tempRoot")
}

func TestWorkStartupCleanup_PreservesUnsafeAttemptProcessesAndEmitsObservation(t *testing.T) {
	projectRoot := setupExecutionCleanupProjectRoot(t)
	tempRoot := t.TempDir()
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)

	liveRunStatePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-runstate-20260628T120000-feedface")
	liveSidecarPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-live-sidecar-20260628T120000-c0ffee00")
	registeredPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-registered-20260628T120000-baadf00d")
	foreignPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-foreign-20260628T120000-deadbeef")
	uncertainPath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-uncertain-20260628T120000-1234abcd")
	stalePath := filepath.Join(tempRoot, ExecuteBeadWtPrefix+"ddx-stale-20260628T120000-00112233")

	writeExecutionCleanupCandidate(t, liveRunStatePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live-runstate",
		AttemptID:    "20260628T120000-feedface",
		WorktreePath: liveRunStatePath,
	}, nil)
	writeExecutionCleanupCandidate(t, liveSidecarPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-live-sidecar",
		AttemptID:    "20260628T120000-c0ffee00",
		WorktreePath: liveSidecarPath,
	}, nil)
	writeExecutionCleanupCandidate(t, registeredPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-registered",
		AttemptID:    "20260628T120000-baadf00d",
		WorktreePath: registeredPath,
	}, nil)
	writeExecutionCleanupCandidate(t, foreignPath, ExecutionCleanupMetadata{
		ProjectRoot:  "/non-temp/foreign-project",
		BeadID:       "ddx-foreign",
		AttemptID:    "20260628T120000-deadbeef",
		WorktreePath: foreignPath,
	}, nil)
	writeExecutionCleanupCandidateWithoutMetadata(t, uncertainPath, nil)
	writeExecutionCleanupCandidate(t, stalePath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-stale",
		AttemptID:    "20260628T120000-00112233",
		WorktreePath: stalePath,
	}, nil)

	killer := &fakeExecutionCleanupAttemptProcessKiller{}
	mgr := NewExecutionCleanupManager(projectRoot, &executionCleanupTestGitOps{worktrees: []string{registeredPath}})
	mgr.TempRoot = tempRoot
	mgr.Now = func() time.Time { return now }
	mgr.attemptProcessScanner = fakeExecutionCleanupAttemptProcessScanner{
		processes: []executionCleanupAttemptProcess{
			{PID: 1001, PPID: 1, PGID: 1001, Command: "sh -lc cargo test", Cwd: stalePath, Worktree: stalePath, StartedAt: now.Add(-2 * time.Hour)},
			{PID: 2001, PPID: 1, PGID: 2001, Command: "bash -lc sleep 60", Cwd: liveRunStatePath, Worktree: liveRunStatePath, StartedAt: now.Add(-10 * time.Minute)},
			{PID: 3001, PPID: 1, PGID: 3001, Command: "python -m http.server", Cwd: liveSidecarPath, Worktree: liveSidecarPath, StartedAt: now.Add(-10 * time.Minute)},
			{PID: 4001, PPID: 1, PGID: 4001, Command: "node server.js", Cwd: registeredPath, Worktree: registeredPath, StartedAt: now.Add(-10 * time.Minute)},
			{PID: 5001, PPID: 1, PGID: 5001, Command: "ruby task.rb", Cwd: foreignPath, Worktree: foreignPath, StartedAt: now.Add(-10 * time.Minute)},
			{PID: 6001, PPID: 1, PGID: 6001, Command: "perl worker.pl", Cwd: uncertainPath, Worktree: uncertainPath, StartedAt: now.Add(-10 * time.Minute)},
		},
	}
	mgr.attemptProcessKiller = killer

	summary := ExecutionCleanupSummary{ProjectRoot: projectRoot, TempRoot: tempRoot}
	runStates := []RunState{
		{
			BeadID:       "ddx-live-runstate",
			AttemptID:    "20260628T120000-feedface",
			StartedAt:    now.Add(-5 * time.Minute),
			WorktreePath: liveRunStatePath,
			PID:          9999,
		},
	}

	err := mgr.CleanupAttemptProcesses(context.Background(), &summary, runStates, map[string]struct{}{filepath.Clean(registeredPath): {}}, cleanupProcessSidecarProbe{
		sidecarAttempts: map[string]struct{}{
			"20260628T120000-c0ffee00": {},
		},
	}, now)
	require.NoError(t, err)

	require.Len(t, summary.ProcessFindings, 1)
	finding := summary.ProcessFindings[0]
	assert.Equal(t, stalePath, finding.Worktree)
	assert.True(t, finding.WouldKill)
	assert.True(t, finding.Terminated)
	assert.Equal(t, []int{1001}, killer.killed)

	require.NotEmpty(t, summary.Observations)
	assert.True(t, hasObservationClass(summary.Observations, "reaped_stale_attempt_process"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_registered_worktree_process"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_foreign_project_process"))
	assert.True(t, hasObservationClass(summary.Observations, "preserved_uncertain_attempt_process"))

	var sawLiveRunState, sawLiveSidecar bool
	for _, obs := range summary.Observations {
		if obs.Class != "preserved_live_attempt_process" {
			continue
		}
		if strings.Contains(obs.Message, "matched live run-state") {
			sawLiveRunState = true
		}
		if strings.Contains(obs.Message, "live worker sidecar") {
			sawLiveSidecar = true
		}
	}
	assert.True(t, sawLiveRunState, "live run-state must be preserved")
	assert.True(t, sawLiveSidecar, "live worker sidecar must be preserved")
}
