package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingLiveAttemptRunner struct {
	started chan<- string
	release <-chan struct{}
}

func (r blockingLiveAttemptRunner) Run(opts RunArgs) (*Result, error) {
	r.started <- opts.WorkDir
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-r.release:
		return &Result{ExitCode: 0}, nil
	case <-ctx.Done():
		return &Result{ExitCode: 1, Error: ctx.Err().Error()}, ctx.Err()
	}
}

func TestWorkConcurrentAttempts_CleanupPreservesBothLiveWorktrees(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 2)
	execRoot := config.ExecutionWorktreeRoot(projectRoot)
	require.NotEmpty(t, execRoot)

	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	runner := blockingLiveAttemptRunner{started: started, release: release}
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Harness: "test-harness"}).
		Resolve(config.CLIOverrides{})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	gitOps := &RealGitOps{}
	for i, beadID := range []string{"ddx-int-0001", "ddx-int-0002"} {
		i, beadID := i, beadID
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
				AgentRunner: runner,
				WorkerID:    "live-cleanup-test",
			}, gitOps)
		}()
	}

	worktrees := collectStartedWorktrees(t, started, 2)
	for _, path := range worktrees {
		assert.DirExists(t, path)
	}

	mgr := NewExecutionCleanupManager(projectRoot, &RealGitOps{})
	mgr.TempRoot = execRoot
	summary, err := mgr.Cleanup(context.Background())
	require.NoError(t, err)

	for _, path := range worktrees {
		assert.DirExists(t, path, "cleanup must preserve active attempt worktree")
	}
	assert.Equal(t, int64(0), summary.RemovedRegisteredWorktrees)
	assert.GreaterOrEqual(t, countObservationClass(summary.Observations, "preserved_live_attempt"), 2)
	states, err := ReadRunStates(projectRoot)
	require.NoError(t, err)
	assert.Len(t, states, 2)

	releaseAll()
	waitForAttemptGoroutines(t, &wg)
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestWorkConcurrentAttempts_DirtyCheckoutDoesNotLoseSuccessfulResult(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	const beadID = "ddx-int-0001"

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# base\n"), 0o644))
	runGitInteg(t, projectRoot, "add", "README.md")
	runGitInteg(t, projectRoot, "commit", "-m", "docs: seed readme")

	dirFile := filepath.Join(t.TempDir(), "directive.txt")
	writeDirectiveFile(t, dirFile, []string{
		"modify-line README.md ^#.* # worker edit",
		"commit feat: worker readme",
	})

	runner := NewRunner(Config{})
	rcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Model: dirFile}).
		Resolve(config.CLIOverrides{Harness: "script"})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		AgentRunner: runner,
	}, &RealGitOps{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotEqual(t, res.BaseRev, res.ResultRev, "worker must produce a successful result commit")

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# operator edit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "operator-scratch.txt"), []byte("scratch\n"), 0o644))

	landing, err := LandBeadResult(projectRoot, res, &RealGitOps{}, BeadLandingOptions{
		LandingAdvancer: func(r *ExecuteBeadResult) (*LandResult, error) {
			return Land(projectRoot, BuildLandRequestFromResult(projectRoot, r), RealLandingGitOps{})
		},
	})
	require.NoError(t, err)
	ApplyLandingToResult(res, landing)

	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	assert.Equal(t, "merged", landing.Outcome)
	readme, err := os.ReadFile(filepath.Join(projectRoot, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "# operator edit\n", string(readme))
	scratch, err := os.ReadFile(filepath.Join(projectRoot, "operator-scratch.txt"))
	require.NoError(t, err)
	assert.Equal(t, "scratch\n", string(scratch))

	readmeAtHead := runGitInteg(t, projectRoot, "show", "refs/heads/main:README.md")
	assert.Equal(t, "# worker edit", readmeAtHead)
	require.NotEmpty(t, res.ResultFile)
	resultAtHead := runGitInteg(t, projectRoot, "show", "refs/heads/main:"+res.ResultFile)
	assert.Contains(t, resultAtHead, beadID)
}

func TestWorkConcurrentAttempts_WorktreeLostLeavesBeadRetryable(t *testing.T) {
	projectRoot, _ := newScriptHarnessRepo(t, 1)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	const beadID = "ddx-int-0001"

	beadRcfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{Harness: "test-harness"}).
		Resolve(config.CLIOverrides{})
	gitOps := &RealGitOps{}
	var resultFile string

	store := makeLoopStore(t, ddxDir)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			res, err := ExecuteBeadWithConfig(ctx, projectRoot, id, beadRcfg, ExecuteBeadRuntime{
				AgentRunner: removeAttemptWorktreeRunner{},
			}, gitOps)
			if res == nil {
				return ExecuteBeadReport{
					BeadID:        id,
					Status:        ExecuteBeadStatusExecutionFailed,
					Detail:        err.Error(),
					OutcomeReason: FailureModeWorktreeLost,
				}, nil
			}
			resultFile = res.ResultFile
			report := executeBeadResultToReport(res)
			report.OutcomeReason = res.FailureMode
			return report, nil
		}),
	}

	loopOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	loopRcfg := config.NewTestConfigForLoop(loopOpts).Resolve(config.TestLoopOverrides(loopOpts))
	result, err := worker.Run(context.Background(), loopRcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Attempts)
	require.Len(t, result.Results, 1)
	assert.Equal(t, FailureModeWorktreeLost, result.Results[0].OutcomeReason)

	got, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	if got.Extra != nil {
		_, hasRetry := got.Extra["work-retry-after"]
		assert.False(t, hasRetry, "worktree_lost must remain immediately retryable")
	}

	require.NotEmpty(t, resultFile)
	raw, err := os.ReadFile(filepath.Join(projectRoot, resultFile))
	require.NoError(t, err)
	assert.Contains(t, string(raw), FailureModeWorktreeLost)
	assert.Contains(t, string(raw), `"attempt_diagnostics"`)
}

func collectStartedWorktrees(t *testing.T, started <-chan string, count int) []string {
	t.Helper()
	deadline := time.After(10 * time.Second)
	seen := map[string]bool{}
	for len(seen) < count {
		select {
		case path := <-started:
			seen[path] = true
		case <-deadline:
			t.Fatalf("timed out waiting for %d live attempts; got %d", count, len(seen))
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	return paths
}

func waitForAttemptGoroutines(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for execute-bead attempts to exit")
	}
}
