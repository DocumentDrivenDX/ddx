package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// staleWorktreeRegistrationGitOps simulates a `git worktree add` that fails
// on its first call because a prior attempt died without a clean `git
// worktree remove`, leaving a registration whose gitdir no longer resolves.
// The real git error for this case is:
//
//	fatal: '<path>' is a missing but already registered worktree;
//	use 'add -f' to override, or 'prune' or 'remove' to clear
type staleWorktreeRegistrationGitOps struct {
	GitOps
	addCalls   int
	prunedDirs []string
}

func (g *staleWorktreeRegistrationGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.addCalls++
	if g.addCalls == 1 {
		return fmt.Errorf("git worktree add: fatal: '%s' is a missing but already registered worktree;\nuse 'add -f' to override, or 'prune' or 'remove' to clear: exit status 128", wtPath)
	}
	return os.MkdirAll(wtPath, 0o755)
}

func (g *staleWorktreeRegistrationGitOps) WorktreePrune(dir string) error {
	g.prunedDirs = append(g.prunedDirs, dir)
	return nil
}

// TestExecuteBeadIsolation_PrunesStaleWorktreeAndRetries proves that a stale
// worktree registration for the target path does not fail isolation setup:
// WorktreeAttemptBackend.Prepare prunes the registration and retries the add
// once before giving up (ddx-2e9679ba).
func TestExecuteBeadIsolation_PrunesStaleWorktreeAndRetries(t *testing.T) {
	projectRoot := t.TempDir()
	gitOps := &staleWorktreeRegistrationGitOps{}

	ws, err := (WorktreeAttemptBackend{}).Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-stale-wt",
		AttemptID:   "20260706T143635-baffe7ee",
		BaseRev:     "deadbeef",
		GitOps:      gitOps,
	})

	require.NoError(t, err)
	require.NotNil(t, ws)
	require.Equal(t, 2, gitOps.addCalls, "expected one retry after pruning the stale registration")
	require.Len(t, gitOps.prunedDirs, 1)
	require.Equal(t, projectRoot, gitOps.prunedDirs[0])

	info, statErr := os.Stat(ws.WorkDir)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())
}

// alwaysFailingWorktreeAddGitOps simulates a genuine, non-registration
// failure (e.g. disk full) that prune-and-retry must not paper over.
type alwaysFailingWorktreeAddGitOps struct {
	GitOps
	addCalls   int
	prunedDirs []string
}

func (g *alwaysFailingWorktreeAddGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	g.addCalls++
	return fmt.Errorf("git worktree add: fatal: no space left on device: exit status 128")
}

func (g *alwaysFailingWorktreeAddGitOps) WorktreePrune(dir string) error {
	g.prunedDirs = append(g.prunedDirs, dir)
	return nil
}

func TestExecuteBeadIsolation_NonRegistrationWorktreeFailureIsNotRetried(t *testing.T) {
	projectRoot := t.TempDir()
	gitOps := &alwaysFailingWorktreeAddGitOps{}

	_, err := (WorktreeAttemptBackend{}).Prepare(context.Background(), AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      "ddx-disk-full",
		AttemptID:   "20260706T143635-baffe7ee",
		BaseRev:     "deadbeef",
		GitOps:      gitOps,
	})

	require.Error(t, err)
	require.Equal(t, 1, gitOps.addCalls, "a genuine failure must not be retried")
	require.Empty(t, gitOps.prunedDirs)
}

// TestExecuteBeadIsolation_ClassifiesRegistrationErrors proves that a
// registration stat failure surfaced during execute-bead isolation setup
// (the "preparing execute-bead git isolation: stat worktree .git" error path)
// is classified as environment_repairable, not resource_exhausted. Prior to
// this fix the generic "worktree" + "no such file" catch-all in
// classifyReadinessSystemReason misrouted this into resource_exhausted,
// which stops the work loop entirely instead of letting the next attempt
// retry (ddx-2e9679ba).
func TestExecuteBeadIsolation_ClassifiesRegistrationErrors(t *testing.T) {
	wtPath := filepath.Join(string(os.PathSeparator), "tmp", "ddx-exec-wt", ".execute-bead-wt-ddx-739ce984-20260706T143635-baffe7ee")
	detail := fmt.Sprintf("preparing execute-bead git isolation: stat worktree .git: stat %s: no such file or directory", filepath.Join(wtPath, ".git"))

	reason := classifyReadinessSystemReason(detail, nil)

	require.Equal(t, ReadinessSystemReasonEnvironmentRepairable, reason)
	require.NotEqual(t, ReadinessSystemReasonResourceExhausted, reason)

	result := ClassifyReadiness("", nil, detail)
	require.Equal(t, ReadinessClassificationSystemUnready, result.Classification)
	require.Equal(t, ReadinessSystemReasonEnvironmentRepairable, result.SystemReason)
	require.Equal(t, "recoverable", result.TriageClassification)

	var report ExecuteBeadReport
	report.Status = ExecuteBeadStatusExecutionFailed
	report.Detail = detail
	classifyLoopReportFailure(&report)
	require.Equal(t, "recoverable", report.OutcomeReason)
	require.NotEqual(t, ExecuteBeadStatusResourceExhausted, report.OutcomeReason)
}
