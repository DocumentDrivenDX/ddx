package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkStartup_ReapsStaleWorktrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-liveness startup cleanup test is unix-oriented")
	}

	projectRoot, tempRoot := setupWorkStartupCleanupProject(t)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")

	attemptID := "20260515T010203-deadbeef"
	worktreePath := createRegisteredStartupWorktree(t, projectRoot, tempRoot, "ddx-startup-stale", attemptID)
	old := time.Now().Add(-2 * time.Hour).UTC()
	writeStartupRunState(t, projectRoot, "ddx-startup-stale", attemptID, worktreePath, deadProcessPID(t), old)
	require.NoError(t, os.Chtimes(worktreePath, old, old))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.NoFileExists(t, worktreePath)
	assert.NotContains(t, runCleanupCommandGit(t, projectRoot, "worktree", "list", "--porcelain"), worktreePath)
}

func TestWorkStartup_PreservesLiveWorktrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-liveness startup cleanup test is unix-oriented")
	}

	projectRoot, tempRoot := setupWorkStartupCleanupProject(t)
	t.Setenv("DDX_WORKTREE_REAP_MAX_AGE", "1h")

	attemptID := "20260515T020304-feedface"
	worktreePath := createRegisteredStartupWorktree(t, projectRoot, tempRoot, "ddx-startup-live", attemptID)
	old := time.Now().Add(-2 * time.Hour).UTC()
	live := startLongLivedProcess(t)
	writeStartupRunState(t, projectRoot, "ddx-startup-live", attemptID, worktreePath, live.Process.Pid, old)
	require.NoError(t, os.Chtimes(worktreePath, old, old))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.DirExists(t, worktreePath)
	assert.Contains(t, runCleanupCommandGit(t, projectRoot, "worktree", "list", "--porcelain"), worktreePath)
}

func TestWorkStartup_ReapsStaleWorkerDirs(t *testing.T) {
	projectRoot, _ := setupWorkStartupCleanupProject(t)

	workersDir := filepath.Join(projectRoot, ddxroot.DirName, "workers")
	require.NoError(t, os.MkdirAll(workersDir, 0o755))

	deadWorkerID := "agent-loop-dead"
	staleWorkerID := "agent-loop-stale"
	freshWorkerID := "agent-loop-fresh"
	now := time.Now().UTC()

	require.NoError(t, workerstatus.WriteLiveness(projectRoot, deadWorkerID, workerstatus.LivenessRecord{
		WorkerID:       deadWorkerID,
		ProjectRoot:    projectRoot,
		PID:            deadProcessPID(t),
		LastActivityAt: now,
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, staleWorkerID, workerstatus.LivenessRecord{
		WorkerID:       staleWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: now.Add(-31 * time.Minute),
	}))
	require.NoError(t, workerstatus.WriteLiveness(projectRoot, freshWorkerID, workerstatus.LivenessRecord{
		WorkerID:       freshWorkerID,
		ProjectRoot:    projectRoot,
		LastActivityAt: now,
	}))

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.NoDirExists(t, filepath.Join(workersDir, deadWorkerID))
	assert.NoDirExists(t, filepath.Join(workersDir, staleWorkerID))
	assert.DirExists(t, filepath.Join(workersDir, freshWorkerID))
}

func TestWorkStartupCleanup_ReapsStaleAttemptDescendantProcessesBeforeClaim(t *testing.T) {
	projectRoot, _ := setupWorkStartupCleanupProject(t)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    "ddx-startup-claim",
		Title: "startup cleanup claim",
	}))

	var cleanupCalls int32
	runner := newStartupHousekeepingRunner(projectRoot)
	runner.processCleanup = func(ctx context.Context, projectRoot, tempRoot string, summary *agent.ExecutionCleanupSummary, runStates []agent.RunState, registered map[string]struct{}, now time.Time) error {
		atomic.AddInt32(&cleanupCalls, 1)
		summary.ProcessFindings = append(summary.ProcessFindings, agent.ExecutionCleanupProcessFinding{
			PID:         101,
			PGID:        101,
			Worktree:    filepath.Join(tempRoot, "ddx-stale"),
			StaleReason: "stale liveness",
			WouldKill:   true,
			Terminated:  true,
		})
		summary.Observations = append(summary.Observations, agent.ExecutionCleanupObservation{
			Path:    filepath.Join(tempRoot, "ddx-stale"),
			Class:   "reaped_stale_attempt_process",
			Message: "stale liveness",
		})
		return nil
	}

	claimGuard := &startupCleanupClaimGuardStore{
		Store:        store,
		t:            t,
		cleanupCalls: &cleanupCalls,
	}
	worker := &agent.ExecuteBeadWorker{
		Store: claimGuard,
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			return agent.ExecuteBeadReport{
				BeadID: beadID,
				Status: agent.ExecuteBeadStatusExecutionFailed,
				Detail: "test failure",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-startup"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{
		ProjectRoot:   projectRoot,
		CleanupRunner: runner,
		CleanupLog:    io.Discard,
		Once:          true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&cleanupCalls), int32(1))
}

// TestWorkStartupCleanup_ReapsReparentedAttemptDescendantsBeforeClaim proves
// that the startup pre-claim cleanup pass invokes the broadened cwd-based
// classifier from ddx-54d9c455: a reparented descendant whose cwd lives under
// the configured execution worktree root but whose leaf directory does NOT
// carry the `.execute-bead-wt-*` suffix is reaped before the worker attempts
// its first ready-execution claim.
func TestWorkStartupCleanup_ReapsReparentedAttemptDescendantsBeforeClaim(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process scanning only available on Linux")
	}

	projectRoot, tempRoot := setupWorkStartupCleanupProject(t)

	// A reparented descendant whose cwd lives under tempRoot but does NOT
	// contain the explicit `.execute-bead-wt-*` segment in its leaf.
	descendantCwd := filepath.Join(tempRoot, "reparented-orphan-20260628T170000-feedface")
	require.NoError(t, os.MkdirAll(descendantCwd, 0o755))
	require.NoError(t, agent.WriteExecutionCleanupMetadata(descendantCwd, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       "ddx-startup-reparented",
		AttemptID:    "20260628T170000-feedface",
		WorktreePath: descendantCwd,
	}))

	// Spawn a real process in its own process group with cwd = the reparented
	// descendant. The startup pre-claim pass must reap it via the broadened
	// classifier even though the cwd lacks the `.execute-bead-wt-*` segment.
	sleepCmd := exec.Command("sleep", "3600")
	sleepCmd.Dir = descendantCwd
	sleepCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, sleepCmd.Start())
	pid := sleepCmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_, _ = sleepCmd.Process.Wait()
	})

	root := NewCommandFactory(projectRoot).NewRootCommand()
	out, err := executeCommand(root, "work", "--json", "--once", "--project", projectRoot)
	require.NoError(t, err)

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)

	// Reap the leader zombie so the polling check sees the PID disappear.
	go func() { _, _ = sleepCmd.Process.Wait() }()
	require.Eventually(t, func() bool {
		return sleepCmd.Process.Signal(syscall.Signal(0)) != nil
	}, 3*time.Second, 20*time.Millisecond, "startup pre-claim pass must reap the reparented descendant process group")
}

func TestWorkStartupCleanup_UsesCleanupLockForProcessReaping(t *testing.T) {
	projectRoot, _ := setupWorkStartupCleanupProject(t)
	store := bead.NewStore(filepath.Join(projectRoot, ddxroot.DirName))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    "ddx-lock-1",
		Title: "lock one",
	}))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:    "ddx-lock-2",
		Title: "lock two",
	}))

	blocker := &blockingStartupProcessCleanup{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	runner := newStartupHousekeepingRunner(projectRoot)
	runner.processCleanup = blocker.Run

	newWorker := func() *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store: store,
			Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
				return agent.ExecuteBeadReport{
					BeadID: beadID,
					Status: agent.ExecuteBeadStatusExecutionFailed,
					Detail: "test failure",
				}, nil
			}),
		}
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-lock"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	runWorker := func() {
		defer wg.Done()
		_, err := newWorker().Run(context.Background(), rcfg, agent.ExecuteBeadLoopRuntime{
			ProjectRoot:   projectRoot,
			CleanupRunner: runner,
			CleanupLog:    io.Discard,
			Once:          true,
		})
		errs <- err
	}

	wg.Add(2)
	go runWorker()
	<-blocker.started
	go runWorker()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&blocker.calls) >= 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&blocker.maxActive))
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&blocker.calls), "second worker must skip the process pass while the cleanup lock is held")

	close(blocker.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestExecutionsRetention_ArchivesOldEvidence(t *testing.T) {
	t.Run("archives old evidence under executions-archive", func(t *testing.T) {
		projectRoot := setupWorkStartupCleanupProjectRoot(t)
		t.Setenv(executionRetentionOverrideEnv, "7")

		oldTime := time.Now().AddDate(0, 0, -10).UTC()
		oldAttemptID := "20260101T000000-deadbeef"
		oldDir := setupWorkStartupEvidenceDir(t, projectRoot, oldAttemptID, oldTime)
		recentAttemptID := "20260501T000000-feedface"
		recentDir := setupWorkStartupEvidenceDir(t, projectRoot, recentAttemptID, time.Now().UTC())

		runner := newStartupHousekeepingRunner(projectRoot)
		report, err := runner.scan(context.Background(), true)
		require.NoError(t, err)

		archivedDir := filepath.Join(projectRoot, ddxroot.DirName, "executions-archive", "2026", "01", oldAttemptID)
		assert.NoDirExists(t, oldDir)
		assert.DirExists(t, archivedDir)
		assert.DirExists(t, recentDir)
		assert.Equal(t, int64(1), report.StaleExecutionDirs)
		assert.Equal(t, int64(1), report.ArchivedExecutionDirs)
		assert.Equal(t, int64(0), report.DeletedExecutionDirs)
	})

	t.Run("deletes old evidence when retain-days override is zero", func(t *testing.T) {
		projectRoot := setupWorkStartupCleanupProjectRoot(t)
		t.Setenv(executionRetentionOverrideEnv, "0")

		oldTime := time.Now().AddDate(0, 0, -10).UTC()
		oldAttemptID := "20260101T000000-cafebabe"
		oldDir := setupWorkStartupEvidenceDir(t, projectRoot, oldAttemptID, oldTime)

		runner := newStartupHousekeepingRunner(projectRoot)
		report, err := runner.scan(context.Background(), true)
		require.NoError(t, err)

		assert.NoDirExists(t, oldDir)
		assert.NoDirExists(t, filepath.Join(projectRoot, ddxroot.DirName, "executions-archive", "2026", "01", oldAttemptID))
		assert.Equal(t, int64(1), report.StaleExecutionDirs)
		assert.Equal(t, int64(0), report.ArchivedExecutionDirs)
		assert.Equal(t, int64(1), report.DeletedExecutionDirs)
	})
}

func setupWorkStartupCleanupProject(t *testing.T) (string, string) {
	t.Helper()

	projectRoot, tempRoot := setupCleanupCommandProject(t)
	runCleanupCommandGit(t, projectRoot, "init", "-b", "main")
	runCleanupCommandGit(t, projectRoot, "config", "user.email", "test@ddx.test")
	runCleanupCommandGit(t, projectRoot, "config", "user.name", "DDx Test")
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "seed.txt"), []byte("seed\n"), 0o644))
	runCleanupCommandGit(t, projectRoot, "add", "seed.txt")
	runCleanupCommandGit(t, projectRoot, "commit", "-m", "chore: seed work startup cleanup repo")
	return projectRoot, tempRoot
}

func setupWorkStartupCleanupProjectRoot(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	return projectRoot
}

func setupWorkStartupEvidenceDir(t *testing.T, projectRoot, attemptID string, mtime time.Time) string {
	t.Helper()

	dir := filepath.Join(projectRoot, ddxroot.DirName, "executions", attemptID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"status":"success"}`), 0o644))
	require.NoError(t, os.Chtimes(dir, mtime, mtime))
	return dir
}

func createRegisteredStartupWorktree(t *testing.T, projectRoot, tempRoot, beadID, attemptID string) string {
	t.Helper()

	worktreePath := filepath.Join(tempRoot, agent.ExecuteBeadWtPrefix+beadID+"-"+attemptID)
	runCleanupCommandGit(t, projectRoot, "worktree", "add", "--detach", worktreePath, "HEAD")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", worktreePath).Run()
	})
	require.NoError(t, agent.WriteExecutionCleanupMetadata(worktreePath, agent.ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       beadID,
		AttemptID:    attemptID,
		WorktreePath: worktreePath,
		Registered:   true,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "scratch.txt"), []byte("payload\n"), 0o644))
	return worktreePath
}

func writeStartupRunState(t *testing.T, projectRoot, beadID, attemptID, worktreePath string, pid int, refreshedAt time.Time) {
	t.Helper()

	require.NoError(t, agent.WriteRunState(projectRoot, agent.RunState{
		BeadID:       beadID,
		AttemptID:    attemptID,
		StartedAt:    refreshedAt.Add(-5 * time.Minute),
		WorktreePath: worktreePath,
		PID:          pid,
		RefreshedAt:  refreshedAt,
		ExpiresAt:    refreshedAt.Add(5 * time.Minute),
	}))
}

func deadProcessPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}

func startLongLivedProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

type startupCleanupClaimGuardStore struct {
	*bead.Store
	t            *testing.T
	cleanupCalls *int32
	readyCalls   int32
}

func (s *startupCleanupClaimGuardStore) ReadyExecution() ([]bead.Bead, error) {
	s.t.Helper()
	if atomic.AddInt32(&s.readyCalls, 1) == 1 && atomic.LoadInt32(s.cleanupCalls) == 0 {
		s.t.Fatalf("cleanup must run before the first ready claim")
	}
	return s.Store.ReadyExecution()
}

type blockingStartupProcessCleanup struct {
	started   chan struct{}
	release   chan struct{}
	calls     int32
	active    int32
	maxActive int32
}

func (r *blockingStartupProcessCleanup) Run(ctx context.Context, projectRoot, tempRoot string, summary *agent.ExecutionCleanupSummary, runStates []agent.RunState, registered map[string]struct{}, now time.Time) error {
	_ = projectRoot
	_ = tempRoot
	_ = summary
	_ = runStates
	_ = registered
	_ = now
	atomic.AddInt32(&r.calls, 1)
	active := atomic.AddInt32(&r.active, 1)
	for {
		prev := atomic.LoadInt32(&r.maxActive)
		if active <= prev || atomic.CompareAndSwapInt32(&r.maxActive, prev, active) {
			break
		}
	}
	if r.started != nil {
		select {
		case <-r.started:
		default:
			close(r.started)
		}
	}
	select {
	case <-ctx.Done():
	case <-r.release:
	}
	atomic.AddInt32(&r.active, -1)
	return nil
}
