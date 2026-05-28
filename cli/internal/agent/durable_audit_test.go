package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
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

func TestCommitOutcomeDurableMutationUsesAuditCommit(t *testing.T) {
	projectRoot := newDurableAuditProject(t)
	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	require.NoError(t, store.Init())
	candidate := &bead.Bead{ID: "ddx-audit-outcome", Title: "Outcome audit", Priority: 0}
	require.NoError(t, store.Create(candidate))
	runGitInteg(t, projectRoot, "add", ".")
	runGitInteg(t, projectRoot, "commit", "-m", "chore: seed tracker")
	head := runGitInteg(t, projectRoot, "rev-parse", "HEAD")

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
		FinalizeDurableAudit: func(report ExecuteBeadReport) error { return FinalizeDurableAttemptAudit(projectRoot, store, report) },
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
	require.NoError(t, store.Init())
	candidate := &bead.Bead{ID: "ddx-audit-lock", Title: "lock", Priority: 0}
	require.NoError(t, store.Create(candidate))
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
