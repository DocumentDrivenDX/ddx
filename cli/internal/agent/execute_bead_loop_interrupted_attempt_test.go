package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interruptedAttemptGitRun struct {
	projectRoot        string
	store              *bead.Store
	candidate          *bead.Bead
	result             *ExecuteBeadLoopResult
	err                error
	attemptID          string
	runStateRootRel    string
	runStateAttemptRel string
}

func TestExecuteLoopInterruptedAttemptCommitsTrackerCleanup(t *testing.T) {
	run := runInterruptedAttemptInGitRepo(t, false)

	require.ErrorIs(t, run.err, context.Canceled)
	require.NotNil(t, run.result)
	require.Len(t, run.result.Results, 1)

	got, err := run.store.Get(context.Background(), run.candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	status := runGitInteg(t, run.projectRoot, "status", "--short", "--",
		".ddx/beads.jsonl",
		".ddx/metrics/attempts.jsonl",
		".ddx/attachments",
	)
	assert.Empty(t, status, "interrupted cleanup must not leave tracker/audit paths dirty")

	subject := runGitInteg(t, run.projectRoot, "log", "-1", "--pretty=%s")
	assert.Equal(t, "chore: update tracker (execute-bead "+run.attemptID+")", subject)

	show := runGitInteg(t, run.projectRoot, "show", "--name-only", "--format=", "HEAD")
	assert.Contains(t, show, ".ddx/beads.jsonl")
}

func TestExecuteLoopInterruptedAttemptDoesNotCommitRunState(t *testing.T) {
	run := runInterruptedAttemptInGitRepo(t, true)

	require.ErrorIs(t, run.err, context.Canceled)
	require.NotNil(t, run.result)

	show := runGitInteg(t, run.projectRoot, "show", "--name-only", "--format=", "HEAD")
	assert.NotContains(t, show, run.runStateRootRel)
	assert.NotContains(t, show, run.runStateAttemptRel)

	status := runGitInteg(t, run.projectRoot, "status", "--short", "--", run.runStateRootRel, run.runStateAttemptRel)
	assert.Contains(t, status, run.runStateRootRel)
	assert.Contains(t, status, run.runStateAttemptRel)

	trackerStatus := runGitInteg(t, run.projectRoot, "status", "--short", "--",
		".ddx/beads.jsonl",
		".ddx/metrics/attempts.jsonl",
		".ddx/attachments",
	)
	assert.Empty(t, trackerStatus, "tracker cleanup commit must still land while run-state stays dirty")
}

func TestExecuteLoopInterruptedAttemptDoesNotRecordTerminalOutcome(t *testing.T) {
	run := runInterruptedAttemptInGitRepo(t, false)

	require.ErrorIs(t, run.err, context.Canceled)
	require.NotNil(t, run.result)
	require.Len(t, run.result.Results, 1)

	report := run.result.Results[0]
	assert.True(t, report.Disrupted)
	assert.Equal(t, "context_canceled", report.DisruptionReason)
	assert.Empty(t, report.RetryAfter)

	got, err := run.store.Get(context.Background(), run.candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.NotEqual(t, bead.StatusClosed, got.Status)
	assert.Empty(t, got.Owner)
	_, hasRetry := got.Extra["work-retry-after"]
	assert.False(t, hasRetry, "cancelled attempts must remain immediately retryable")

	events, err := run.store.Events(run.candidate.ID)
	require.NoError(t, err)
	for _, ev := range events {
		if ev.Kind != "execute-bead" {
			continue
		}
		assert.NotContains(t,
			[]string{ExecuteBeadStatusSuccess, ExecuteBeadStatusAlreadySatisfied, ExecuteBeadStatusNoChanges},
			ev.Summary,
			"interrupted cleanup must not record a terminal outcome event",
		)
	}
}

func runInterruptedAttemptInGitRepo(t *testing.T, trackDirtyRunState bool) interruptedAttemptGitRun {
	t.Helper()

	projectRoot := newDurableAuditProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName), 0o755))
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{ID: "ddx-interrupted-attempt", Title: "Interrupted attempt", Priority: 0}
	require.NoError(t, store.Create(candidate))

	attemptID := "20260515T160340-interrupted"
	runStateRootRel := filepath.ToSlash(filepath.Join(ddxroot.DirName, RunStateFileName))
	runStateAttemptRel := filepath.ToSlash(filepath.Join(ddxroot.DirName, RunStateDirName, attemptID+".json"))

	if trackDirtyRunState {
		require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ddxroot.DirName, RunStateDirName), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, filepath.FromSlash(runStateRootRel)), []byte(`{"attempt_id":"seed"}`+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, filepath.FromSlash(runStateAttemptRel)), []byte(`{"attempt_id":"seed"}`+"\n"), 0o644))
	}

	runGitInteg(t, projectRoot, "add", ".")
	if trackDirtyRunState {
		runGitInteg(t, projectRoot, "add", "-f", runStateRootRel, runStateAttemptRel)
	}
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed interrupted attempt cleanup")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

	if trackDirtyRunState {
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, filepath.FromSlash(runStateRootRel)), []byte(`{"attempt_id":"dirty-root"}`+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(projectRoot, filepath.FromSlash(runStateAttemptRel)), []byte(`{"attempt_id":"dirty-attempt"}`+"\n"), 0o644))
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, id string) (ExecuteBeadReport, error) {
			require.NoError(t, store.AppendEvent(id, bead.BeadEvent{
				Kind:      "interrupted-attempt-fixture",
				Summary:   "seed durable tracker mutation",
				Actor:     "worker",
				Source:    "test",
				CreatedAt: time.Now().UTC(),
			}))
			cancel()
			<-ctx.Done()
			return ExecuteBeadReport{
				BeadID:           id,
				AttemptID:        attemptID,
				Status:           ExecuteBeadStatusExecutionFailed,
				Detail:           "context canceled",
				Error:            "context canceled",
				SessionID:        "sess-interrupted-attempt",
				BaseRev:          head,
				ResultRev:        head,
				ProjectRoot:      projectRoot,
				RequestedProfile: "smart",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(cancelCtx, rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: projectRoot,
		FinalizeDurableAudit: func(report ExecuteBeadReport) error {
			return FinalizeDurableAttemptAudit(projectRoot, store, report)
		},
	})

	return interruptedAttemptGitRun{
		projectRoot:        projectRoot,
		store:              store,
		candidate:          candidate,
		result:             result,
		err:                err,
		attemptID:          attemptID,
		runStateRootRel:    runStateRootRel,
		runStateAttemptRel: runStateAttemptRel,
	}
}
