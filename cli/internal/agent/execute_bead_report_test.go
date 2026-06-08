package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealLandingGitOpsUpdateRefToRefusesHEAD(t *testing.T) {
	err := (RealLandingGitOps{}).UpdateRefTo(t.TempDir(), "HEAD", "deadbeef", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to update HEAD directly")
}

func TestMarkResultExecutionErrorReturnsStructuredReport(t *testing.T) {
	res := &ExecuteBeadResult{
		BeadID:    "ddx-error",
		AttemptID: "attempt-1",
		BaseRev:   "base",
		ResultRev: "base",
		ExitCode:  1,
		Outcome:   ExecuteBeadOutcomeTaskFailed,
	}

	MarkResultExecutionError(res, errors.New("failed to read worktree HEAD: exit status 128"))
	report := ReportFromExecuteBeadResult(res, "standard")

	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, "standard", report.PowerClass)
	assert.Contains(t, report.Detail, "failed to read worktree HEAD")
}

func TestMarkResultLandErrorReconcilesAlreadyLandedWorkerCommit(t *testing.T) {
	repo := initReportTestRepo(t)
	base := gitReportTest(t, repo, "rev-parse", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "worker.txt"), []byte("worker\n"), 0o644))
	gitReportTest(t, repo, "add", "worker.txt")
	gitReportTest(t, repo, "commit", "-m", "worker result")
	result := gitReportTest(t, repo, "rev-parse", "HEAD")

	res := &ExecuteBeadResult{
		BeadID:    "ddx-land",
		AttemptID: "20260507T020000-test",
		BaseRev:   base,
		ResultRev: result,
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	MarkResultLandError(repo, res, errors.New("git update-ref refs/heads/main: fatal: cannot lock ref 'refs/heads/main': is at abc but expected def: exit status 128"))

	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status)
	assert.Contains(t, res.Detail, "land coordination reconciled")
	assert.Equal(t, result, res.ImplementationRev)
	assert.Equal(t, result, res.ResultRev)
	assert.Empty(t, res.PreserveRef)
	assert.Empty(t, res.FailureMode)
}

func TestMarkResultLandErrorClassifiesStagedGeneratedEvidenceAsRetryLand(t *testing.T) {
	res := &ExecuteBeadResult{
		BeadID:    "ddx-land",
		AttemptID: "20260507T020000-test",
		BaseRev:   "base",
		ResultRev: "result",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	MarkResultLandError(t.TempDir(), res, errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl\nM\t.ddx/executions/20260507T020000-test/result.json"))

	assert.Equal(t, ExecuteBeadStatusLandRetry, res.Status)
	assert.Equal(t, FailureModeLandRetry, res.FailureMode)
	assert.Contains(t, res.Detail, "land coordination retry")
}

func TestMarkResultLandErrorClassifiesStagedImplementationWorkAsOperatorAttention(t *testing.T) {
	res := &ExecuteBeadResult{
		BeadID:    "ddx-land",
		AttemptID: "20260507T020000-test",
		BaseRev:   "base",
		ResultRev: "result",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
	}

	MarkResultLandError(t.TempDir(), res, errors.New("landing worktree has staged changes after waiting 2s:\nM\t.ddx/beads.jsonl\nM\tcli/internal/agent/foo.go"))

	assert.Equal(t, ExecuteBeadStatusLandOperatorAttention, res.Status)
	assert.Equal(t, FailureModeLandOperatorAttention, res.FailureMode)
	assert.Contains(t, res.Detail, "land coordination operator attention")
}

func initReportTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitReportTest(t, repo, "init", "-b", "main")
	gitReportTest(t, repo, "config", "user.name", "DDx Test")
	gitReportTest(t, repo, "config", "user.email", "ddx-test@example.invalid")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644))
	gitReportTest(t, repo, "add", "README.md")
	gitReportTest(t, repo, "commit", "-m", "base")
	return repo
}

func gitReportTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v\n%s", args, string(out))
	return strings.TrimSpace(string(out))
}
