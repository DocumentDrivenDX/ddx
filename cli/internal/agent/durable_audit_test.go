package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirtyDurableAuditPathsPreservesLeadingDotForTrackedFiles(t *testing.T) {
	status := " M .ddx/beads.jsonl\n" +
		"M  .ddx/metrics/attempts.jsonl\n" +
		"A  .ddx/beads-archive.jsonl\n" +
		"?? .ddx/attachments/ddx-example/events.jsonl\n"

	assert.Equal(t, []string{
		".ddx/beads.jsonl",
		".ddx/metrics/attempts.jsonl",
		".ddx/beads-archive.jsonl",
		".ddx/attachments/ddx-example/events.jsonl",
	}, dirtyDurableAuditPaths(status))
}

func TestCommitDurableAuditOutputsForceStagesIgnoredManagedPaths(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// Commit initial state with .gitignore that ignores DDx-managed audit dirs.
	gitignore := ".ddx/metrics/\n.ddx/attachments/\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(gitignore), 0o644))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed with ignore rules")

	// Write managed audit files that are now ignored by .gitignore.
	metricsDir := filepath.Join(ddxDir, "metrics")
	require.NoError(t, os.MkdirAll(metricsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "attempts.jsonl"), []byte("{\"attempt_id\":\"test\"}\n"), 0o644))
	attDir := filepath.Join(ddxDir, "attachments", "ddx-test")
	require.NoError(t, os.MkdirAll(attDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(attDir, "events.jsonl"), []byte("{\"event\":\"close\"}\n"), 0o644))

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260627T000000-force-stage"))

	// Managed ignored files must be committed (status clean).
	metricsStatus := runGitInteg(t, projectRoot, "status", "--short", "--ignored", "--", ".ddx/metrics/attempts.jsonl")
	assert.Empty(t, metricsStatus, "managed ignored metrics file must be committed")
	attStatus := runGitInteg(t, projectRoot, "status", "--short", "--ignored", "--", ".ddx/attachments")
	assert.Empty(t, attStatus, "managed ignored attachments must be committed")

	subject := runGitInteg(t, projectRoot, "log", "-1", "--pretty=%s")
	assert.Equal(t, "chore: update tracker (execute-bead 20260627T000000-force-stage)", subject)
}

func TestCommitDurableAuditOutputsDoesNotForceStageUnmanagedIgnoredFiles(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)

	// .gitignore ignores both a managed path and an unmanaged path.
	gitignore := ".ddx/metrics/\nunmanaged-ignored.txt\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte(gitignore), 0o644))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed with ignore rules")

	// Create the ignored managed file and an unrelated ignored file.
	metricsDir := filepath.Join(ddxDir, "metrics")
	require.NoError(t, os.MkdirAll(metricsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "attempts.jsonl"), []byte("{\"attempt_id\":\"test\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "unmanaged-ignored.txt"), []byte("secret\n"), 0o644))

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260627T000001-no-unmanaged"))

	// The managed file must be committed.
	managedStatus := runGitInteg(t, projectRoot, "status", "--short", "--ignored", "--", ".ddx/metrics/attempts.jsonl")
	assert.Empty(t, managedStatus, "managed ignored file must be committed")

	// The unmanaged file must NOT be staged or committed.
	unmanagedStatus := runGitInteg(t, projectRoot, "status", "--short", "--ignored", "--", "unmanaged-ignored.txt")
	assert.NotEmpty(t, unmanagedStatus, "unmanaged ignored file must remain uncommitted")
	headFiles := runGitInteg(t, projectRoot, "show", "--name-only", "--pretty=format:", "HEAD")
	assert.NotContains(t, headFiles, "unmanaged-ignored.txt", "unmanaged ignored file must not appear in the commit")
}

func TestCommitDurableAuditOutputsPreservesLeadingDotForUnstagedTrackedPaths(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	metricsDir := filepath.Join(ddxDir, "metrics")
	require.NoError(t, os.MkdirAll(metricsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte("initial\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "attempts.jsonl"), []byte("initial\n"), 0o644))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed durable audit files")

	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "beads.jsonl"), []byte("updated\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "attempts.jsonl"), []byte("updated\n"), 0o644))
	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260515T111500-pathspec"))

	status := runGitInteg(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl")
	assert.Empty(t, status)

	subject := runGitInteg(t, projectRoot, "log", "-1", "--pretty=%s")
	assert.Equal(t, "chore: update tracker (execute-bead 20260515T111500-pathspec)", subject)
	show := runGitInteg(t, projectRoot, "show", "--name-only", "--pretty=format:", "HEAD")
	assert.Contains(t, show, ".ddx/beads.jsonl")
	assert.Contains(t, show, ".ddx/metrics/attempts.jsonl")
}

func TestDurableAuditCommitOutsideTrackerLock(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	dirtyPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(dirtyPath), 0o755))
	require.NoError(t, os.WriteFile(dirtyPath, []byte("{\"attempt_id\":\"outside-lock\"}\n"), 0o644))

	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })
	var commitStarted atomic.Bool
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "rev-parse" || args[0] == "status" || args[0] == "add" || args[0] == "diff" || args[0] == "commit") {
			if args[0] == "commit" {
				commitStarted.Store(true)
			}
			_, err := os.Stat(trackerLockPath(projectRoot))
			require.True(t, os.IsNotExist(err), "Git %s began while tracker lock still existed: %v", args[0], err)
		}
		return origRunner(ctx, gitDir, args...)
	}

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260717T000001-outside-lock"))
	assert.True(t, commitStarted.Load(), "test must observe the durable audit commit")
}

func TestDurableAuditFlushesMultipleFinalizationsAtIterationEpilogue(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	candidate := &bead.Bead{ID: "ddx-audit-epilogue", Title: "audit epilogue", Priority: 0}
	require.NoError(t, store.Create(context.Background(), candidate))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed tracker")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	audit := NewDurableAuditAccumulator(projectRoot, store)
	var commits atomic.Int32
	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "commit" {
			commits.Add(1)
		}
		return origRunner(ctx, gitDir, args...)
	}

	worker := &ExecuteBeadWorker{Store: store, Executor: ExecuteBeadExecutorFunc(func(context.Context, string) (ExecuteBeadReport, error) {
		return ExecuteBeadReport{BeadID: candidate.ID, AttemptID: "20260717T000003-first", Status: ExecuteBeadStatusExecutionFailed, BaseRev: head, ResultRev: head, ProjectRoot: projectRoot}, nil
	})}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
		FinalizeDurableAudit: func(report ExecuteBeadReport) error {
			require.NoError(t, audit.Finalize(report))
			report.AttemptID = "20260717T000003-second"
			return audit.Finalize(report)
		},
		FlushDurableAudit: audit.Flush,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), commits.Load(), "the iteration epilogue must flush both finalizations once")
	contents, readErr := os.ReadFile(filepath.Join(ddxroot.JoinProject(projectRoot), "metrics", "attempts.jsonl"))
	require.NoError(t, readErr)
	assert.Contains(t, string(contents), "20260717T000003-first")
	assert.Contains(t, string(contents), "20260717T000003-second")
}

func TestDurableAuditFlushesOncePerDrainIteration(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	// A drain iteration can write more than one durable attempt event before its
	// final audit flush. Both rows must land in the one resulting audit commit.
	for _, attemptID := range []string{"20260717T000002-first", "20260717T000002-second"} {
		require.NoError(t, attemptmetrics.AppendRow(projectRoot, attemptmetrics.AttemptRow{
			SchemaVersion: attemptmetrics.SchemaVersion,
			AttemptID:     attemptID,
			BeadID:        "ddx-durable-audit-batch",
			TSEnd:         attemptmetrics.Rfc3339(time.Now().UTC()),
			Outcome:       ExecuteBeadStatusExecutionFailed,
		}))
	}

	origRunner := durableAuditGitRunner
	t.Cleanup(func() { durableAuditGitRunner = origRunner })
	var operations []string
	var operationsMu sync.Mutex
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && (args[0] == "add" || args[0] == "commit") {
			operationsMu.Lock()
			operations = append(operations, args[0])
			operationsMu.Unlock()
		}
		return origRunner(ctx, gitDir, args...)
	}

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, "20260717T000002-drain"))
	operationsMu.Lock()
	assert.Equal(t, []string{"add", "commit"}, operations, "one ordered Git flush is required for the iteration")
	operationsMu.Unlock()

	stateRoot := ddxroot.Path(context.Background(), projectRoot)
	contents, err := os.ReadFile(filepath.Join(stateRoot, "metrics", "attempts.jsonl"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), "20260717T000002-first")
	assert.Contains(t, string(contents), "20260717T000002-second")
	assert.Equal(t, "chore: update tracker (execute-bead 20260717T000002-drain)", runGitInteg(t, stateRoot, "log", "-1", "--pretty=%s"))
}

func TestDurableAuditConcurrentFlushesPreserveAllManagedPaths(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	metricsPath := filepath.Join(ddxroot.JoinProject(projectRoot), "metrics", "attempts.jsonl")
	attachmentPath := filepath.Join(ddxroot.JoinProject(projectRoot), "attachments", "ddx-concurrent", "events.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(metricsPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(attachmentPath), 0o755))
	require.NoError(t, os.WriteFile(metricsPath, []byte("{\"attempt\":\"one\"}\n"), 0o644))
	require.NoError(t, os.WriteFile(attachmentPath, []byte("{\"event\":\"two\"}\n"), 0o644))

	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, attemptID := range []string{"20260717T000004-one", "20260717T000004-two"} {
		go func(id string) {
			<-start
			errs <- CommitDurableAuditOutputs(projectRoot, id)
		}(attemptID)
	}
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	assert.Empty(t, runGitInteg(t, projectRoot, "status", "--short", "--", ".ddx/metrics/attempts.jsonl", ".ddx/attachments/ddx-concurrent/events.jsonl"))
	metrics, err := os.ReadFile(metricsPath)
	require.NoError(t, err)
	attachments, err := os.ReadFile(attachmentPath)
	require.NoError(t, err)
	assert.Contains(t, string(metrics), "one")
	assert.Contains(t, string(attachments), "two")
}

func TestCommitDurableAuditOutputs_ConcurrentLockHolderRetriesAndRecovers(t *testing.T) {
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))
	projectRoot := testutils.NewFixtureRepo(t, "standard")
	attemptID := "20260606T081500-audit-lock-recover"
	dirtyPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(dirtyPath), 0o755))
	require.NoError(t, os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644))
	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	origTimeout := durableAuditGitTimeout
	origContextFactory := durableAuditCommandContext
	t.Cleanup(func() {
		durableAuditGitTimeout = origTimeout
		durableAuditCommandContext = origContextFactory
	})

	durableAuditGitTimeout = 100 * time.Millisecond
	var ctxMu sync.Mutex
	var firstContextAt time.Time
	durableAuditCommandContext = func() (context.Context, context.CancelFunc) {
		ctxMu.Lock()
		if firstContextAt.IsZero() {
			firstContextAt = time.Now()
		}
		ctxMu.Unlock()
		return context.WithTimeout(context.Background(), durableAuditGitTimeout)
	}

	locked := make(chan struct{})
	release := make(chan struct{})
	lockErrCh := make(chan error, 1)
	go func() {
		lockErrCh <- withTrackerLock(projectRoot, "durable_audit_test", func() error {
			close(locked)
			<-release
			return nil
		})
	}()

	<-locked
	time.Sleep(3 * durableAuditGitTimeout)

	commitErrCh := make(chan error, 1)
	go func() {
		commitErrCh <- CommitDurableAuditOutputs(projectRoot, attemptID)
	}()

	releaseAt := time.Now()
	close(release)
	require.NoError(t, <-lockErrCh)

	firstErr := <-commitErrCh
	if firstErr != nil {
		require.True(t, isTransientGitContention(firstErr), "expected transient contention error after lock contention, got %v", firstErr)
	}

	ctxMu.Lock()
	require.False(t, firstContextAt.IsZero(), "expected a git subprocess deadline context to be created")
	assert.False(t, firstContextAt.Before(releaseAt), "git subprocess deadline budget started before the tracker lock was released")
	ctxMu.Unlock()

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, attemptID))

	statusArgs := append([]string{"status", "--short", "--"}, trackerpaths.ManagedPathspecs()...)
	assert.Empty(t, runGitInteg(t, projectRoot, statusArgs...))
	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	assert.NotEqual(t, headBefore, headAfter)
}

func TestCommitDurableAuditOutputs_ResumesAfterPartialKill(t *testing.T) {
	t.Setenv("DDX_BIN", testutils.BuildDDxBinary(t))
	projectRoot := testutils.NewFixtureRepo(t, "standard")
	attemptID := "20260606T081600-audit-partial-kill"
	dirtyPath := filepath.Join(projectRoot, ddxroot.DirName, "metrics", "attempts.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(dirtyPath), 0o755))
	require.NoError(t, os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644))
	headBefore := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	origRunner := durableAuditGitRunner
	t.Cleanup(func() {
		durableAuditGitRunner = origRunner
	})

	var commitAttempts int32
	durableAuditGitRunner = func(ctx context.Context, gitDir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "commit" && atomic.AddInt32(&commitAttempts, 1) == 1 {
			return []byte("signal: killed"), errors.New("signal: killed")
		}
		return origRunner(ctx, gitDir, args...)
	}

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, attemptID))
	require.GreaterOrEqual(t, atomic.LoadInt32(&commitAttempts), int32(2), "the killed commit must be retried")
	headAfter := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	assert.NotEqual(t, headBefore, headAfter)
	statusArgs := append([]string{"status", "--short", "--"}, trackerpaths.ManagedPathspecs()...)
	assert.Empty(t, runGitInteg(t, projectRoot, statusArgs...))

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, attemptID))
	headAfter = runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	assert.NotEqual(t, headBefore, headAfter)
	assert.Empty(t, runGitInteg(t, projectRoot, statusArgs...))

	require.NoError(t, CommitDurableAuditOutputs(projectRoot, attemptID))
	assert.Equal(t, headAfter, runGitInteg(t, projectRoot, "rev-parse", "HEAD"))
	assert.Empty(t, runGitInteg(t, projectRoot, statusArgs...))
}

func TestCommitOutcomeDurableMutationUsesAuditCommit(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	candidate := &bead.Bead{ID: "ddx-audit-outcome", Title: "Outcome audit", Priority: 0}
	require.NoError(t, store.Create(context.Background(), candidate))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed tracker")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")
	audit := NewDurableAuditAccumulator(projectRoot, store)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:           beadID,
				AttemptID:        "20260515T101828-audit-commit",
				Status:           ExecuteBeadStatusExecutionFailed,
				Detail:           "implementation failed",
				BaseRev:          head,
				ResultRev:        head,
				SessionID:        "sess-audit-commit",
				ProjectRoot:      projectRoot,
				RequestedProfile: "smart",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                 true,
		ProjectRoot:          projectRoot,
		FinalizeDurableAudit: audit.Finalize,
		FlushDurableAudit:    audit.Flush,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Attempts)
	require.Equal(t, 1, result.Failures)

	status := runGitInteg(t, projectRoot, "status", "--short", "--", ".ddx/beads.jsonl", ".ddx/metrics/attempts.jsonl", ".ddx/attachments")
	assert.Empty(t, status)

	stateRoot := ddxroot.Path(context.Background(), projectRoot)
	subject := runGitInteg(t, stateRoot, "log", "-1", "--pretty=%s")
	assert.Equal(t, "chore: update tracker (execute-bead 20260515T101828-audit-commit)", subject)

	stateStatus := runGitInteg(t, stateRoot, "status", "--short", "--", "beads.jsonl", "metrics/attempts.jsonl", "attachments")
	assert.Empty(t, stateStatus)

	rows, err := attemptmetrics.LoadRows(projectRoot)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "20260515T101828-audit-commit", rows[0].AttemptID)

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	require.Empty(t, got.Owner)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, got.Extra["work-last-status"])
	_, hasRetry := got.Extra["work-retry-after"]
	assert.True(t, hasRetry)
}

func newDurableAuditProject(t *testing.T) string {
	t.Helper()

	setExecutionWorktreeRootForTest(t)
	root := t.TempDir()
	runGitInteg(t, root, "init", "-b", "main")
	runGitInteg(t, root, "config", "user.email", "test@ddx.test")
	runGitInteg(t, root, "config", "user.name", "DDx Test")
	testutils.MakeInitializedDDxRoot(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# audit\n"), 0o644))
	return root
}

// runAuditWithFinalizeErr drives one failed attempt whose durable-audit commit
// returns finalizeErr, and returns the loop result so callers can assert the
// stop behavior (ddx-23ac2796).
func runAuditWithFinalizeErr(t *testing.T, finalizeErr error) *ExecuteBeadLoopResult {
	t.Helper()
	projectRoot := newDurableAuditProject(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	candidate := &bead.Bead{ID: "ddx-audit-lock", Title: "lock", Priority: 0}
	require.NoError(t, store.Create(context.Background(), candidate))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed tracker")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				AttemptID:   "20260527T060000-audit-lock",
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "implementation failed",
				BaseRev:     head,
				ResultRev:   head,
				SessionID:   "sess-audit-lock",
				ProjectRoot: projectRoot,
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                 true,
		ProjectRoot:          projectRoot,
		FinalizeDurableAudit: func(report ExecuteBeadReport) error { return finalizeErr },
	})
	require.NoError(t, err)
	return result
}

// TestWork_IndexLockContentionDuringAuditCommitIsTransientNotFatal asserts that
// a transient .git/index.lock failure on the durable-audit commit does NOT halt
// the worker (ddx-23ac2796).
func TestWork_IndexLockContentionDuringAuditCommitIsTransientNotFatal(t *testing.T) {
	// Each of these transient git index/ref contention forms must be retried,
	// not stop the worker (ddx-23ac2796 + sibling variants seen 2026-05-27).
	for name, errMsg := range map[string]string{
		"index_lock_file_exists":    "staging tracker: fatal: Unable to create '/x/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository: exit status 128",
		"unable_to_write_new_index": "committing durable audit outputs: fatal: unable to write new index file: exit status 128",
		"cannot_lock_ref":           "committing durable audit outputs: error: cannot lock ref 'refs/heads/main': exit status 128",
		"signal_killed_deadline":    "commit durable audit outputs: committing durable audit outputs: : signal: killed",
		"tracker_lock_timeout":      "commit durable audit outputs: tracker lock timeout (max elapsed, lock: .ddx/.git-tracker.lock, owner: 424293)",
	} {
		t.Run(name, func(t *testing.T) {
			result := runAuditWithFinalizeErr(t, fmt.Errorf("%s", errMsg))
			require.Nil(t, result.OperatorAttention, "transient git contention during audit commit must not stop the worker")
			require.NotEqual(t, "operator_attention", result.ExitReason)
		})
	}
}

// TestWork_NonLockAuditCommitFailureStillStopsWorker is the regression guard: a
// genuine (non-contention) durable-audit commit failure must still surface
// operator attention and stop the worker.
func TestWork_NonLockAuditCommitFailureStillStopsWorker(t *testing.T) {
	result := runAuditWithFinalizeErr(t, fmt.Errorf("committing durable audit outputs: fatal: insufficient permission for adding an object to repository database .git/objects: exit status 128"))
	require.NotNil(t, result.OperatorAttention, "a genuine audit-commit failure must still surface operator attention")
	require.Equal(t, "durable_audit_commit_failed", result.OperatorAttention.Reason)
}

func TestFinalizeDurableAuditOrStop_TrackerLockTimeoutDoesNotStopWorker(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))
	candidate := &bead.Bead{ID: "ddx-audit-tracker-lock", Title: "Tracker lock", Priority: 0}
	require.NoError(t, store.Create(context.Background(), candidate))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed tracker")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	var sink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				AttemptID:   "20260527T060100-audit-lock",
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "implementation failed",
				BaseRev:     head,
				ResultRev:   head,
				SessionID:   "sess-audit-lock",
				ProjectRoot: projectRoot,
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	finalizeErr := fmt.Errorf("commit durable audit outputs: %w", &TrackerLockTimeoutError{
		Why:      "max elapsed",
		LockDir:  trackerLockPath(projectRoot),
		OwnerPID: "424293",
	})

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                 true,
		ProjectRoot:          projectRoot,
		EventSink:            &sink,
		FinalizeDurableAudit: func(report ExecuteBeadReport) error { return finalizeErr },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.OperatorAttention, "tracker lock timeout must remain transient")
	assert.Equal(t, "once_complete", result.ExitReason)

	events := parseLoopEvents(t, sink.String())
	var sawTransient bool
	for _, ev := range events {
		if ev.Type != "loop.durable_audit_transient" {
			continue
		}
		sawTransient = true
		assert.Equal(t, candidate.ID, ev.Data["bead_id"])
		assert.Equal(t, "git_tracker_contention", ev.Data["reason"])
		assert.Contains(t, fmt.Sprint(ev.Data["detail"]), "tracker lock timeout")
	}
	assert.True(t, sawTransient, "loop.durable_audit_transient event must be emitted")
}

// TestIsTransientGitContention_SignalKilledAndDeadline asserts that the
// SIGKILL / context-deadline class and the tracker-lock-timeout string are
// each classified as transient (ddx-83361480).
func TestIsTransientGitContention_SignalKilledAndDeadline(t *testing.T) {
	trueFor := []struct {
		name string
		err  error
	}{
		{"unable_to_write_new_index_file", fmt.Errorf("committing tracker: fatal: unable to write new index file: exit status 128")},
		{"signal_killed_in_message", fmt.Errorf("commit durable audit outputs: committing durable audit outputs: : signal: killed")},
		{"context_deadline_exceeded_string", fmt.Errorf("context deadline exceeded")},
		{"tracker_lock_timeout_string", fmt.Errorf("tracker lock timeout (max elapsed, lock: .ddx/.git-tracker.lock, owner: 99)")},
		{"context_deadline_exceeded_sentinel", context.DeadlineExceeded},
		{"context_deadline_exceeded_wrapped", fmt.Errorf("commit durable audit: %w", context.DeadlineExceeded)},
	}
	for _, tc := range trueFor {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, isTransientGitContention(tc.err), "must be classified as transient")
		})
	}

	// Regression: genuine non-transient errors must not be misclassified.
	falseFor := []error{
		fmt.Errorf("committing tracker: fatal: insufficient permission for adding an object to repository database .git/objects: exit status 128"),
		fmt.Errorf("committing durable audit outputs: fatal: insufficient permission for adding an object to repository database .git/objects: exit status 128"),
		errors.New("disk full"),
		context.Canceled,
	}
	for _, err := range falseFor {
		require.False(t, isTransientGitContention(err), "must NOT be classified as transient: %v", err)
	}
}
