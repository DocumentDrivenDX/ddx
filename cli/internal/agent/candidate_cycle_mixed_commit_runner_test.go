package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mixedCommitRunnerModuleFixture(t *testing.T, includeUnrelatedFailure bool) (projectRoot, worktreePath, baseRev, resultRev string) {
	t.Helper()
	projectRoot, baseRev = initTestGitRepo(t)

	cliRoot := filepath.Join(projectRoot, "cli")
	require.NoError(t, os.MkdirAll(filepath.Join(cliRoot, "internal", "agent"), 0o755))
	if includeUnrelatedFailure {
		require.NoError(t, os.MkdirAll(filepath.Join(cliRoot, "internal", "unrelated"), 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(cliRoot, "go.mod"), []byte("module example.com/mixed\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cliRoot, "internal", "agent", "doc.go"), []byte("package agent\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cliRoot, "internal", "agent", "mixed_commit_named_test.go"), []byte(`package agent

import "testing"

func TestMixedCommitNamedACCheckRunnerPasses(t *testing.T) {}

func TestMixedCommitRunsNamedACChecksOnly(t *testing.T) {}
`), 0o644))
	if includeUnrelatedFailure {
		require.NoError(t, os.WriteFile(filepath.Join(cliRoot, "internal", "unrelated", "doc.go"), []byte("package unrelated\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(cliRoot, "internal", "unrelated", "unrelated_test.go"), []byte(`package unrelated

import "testing"

func TestMixedCommitUnrelatedPackageFails(t *testing.T) {
	t.Fatal("broad package runs must not reach unrelated packages")
}
`), 0o644))
	}

	add := fixtureGitCommand(t, projectRoot, "add", ".")
	out, err := add.CombinedOutput()
	require.NoError(t, err, "git add: %s", out)
	commit := fixtureGitCommand(t, projectRoot, "commit", "-m", "feat: mixed commit runner fixture")
	out, err = commit.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)

	rawRev, err := fixtureGitCommand(t, projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	resultRev = strings.TrimSpace(string(rawRev))
	return projectRoot, projectRoot, baseRev, resultRev
}

func TestMixedCommitNonzeroExitClassifiedBeforeChecks(t *testing.T) {
	projectRoot, baseRev, resultRev := mixedCommitCandidateFixture(t)

	var seenFailureMode string
	res := &ExecuteBeadResult{
		BeadID:             "ddx-mixed-classify",
		AttemptID:          "attempt-mixed-classify",
		BaseRev:            baseRev,
		ResultRev:          resultRev,
		Status:             ExecuteBeadStatusExecutionFailed,
		Reason:             mixedCommitAndNoChangesRationaleReason,
		Detail:             mixedCommitAndNoChangesRationaleReason,
		Error:              mixedCommitAndNoChangesRationaleReason,
		NoChangesRationale: "status: open\nreason: keep salvage candidate moving",
		ExitCode:           1,
		FailureMode:        FailureModeTimeout,
	}

	runtime := ExecuteBeadRuntime{
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
			seenFailureMode = candidate.Report.OutcomeReason
			assert.Equal(t, FailureModeTimeout, candidate.Report.OutcomeReason)
			assert.Equal(t, ExecuteBeadStatusSuccess, candidate.Report.Status)
			return CandidateCheckResult{Passed: true, Detail: "named AC checks passed"}, nil
		}),
	}

	require.NoError(t, applyWorkerCandidateCycle(context.Background(), projectRoot, projectRoot, runtime, res))
	assert.Equal(t, FailureModeTimeout, seenFailureMode)
	assert.Equal(t, FailureModeTimeout, res.FailureMode)
}

func TestMixedCommitFailedACEntersRepairNotLand(t *testing.T) {
	projectRoot, baseRev, resultRev := mixedCommitCandidateFixture(t)
	events := &inMemoryEventAppender{}
	checkCalls := 0
	repairCalls := 0

	res := &ExecuteBeadResult{
		BeadID:             "ddx-mixed-repair",
		AttemptID:          "attempt-mixed-repair",
		BaseRev:            baseRev,
		ResultRev:          resultRev,
		Status:             ExecuteBeadStatusExecutionFailed,
		Reason:             mixedCommitAndNoChangesRationaleReason,
		Detail:             mixedCommitAndNoChangesRationaleReason,
		Error:              mixedCommitAndNoChangesRationaleReason,
		NoChangesRationale: "status: open\nreason: keep salvage candidate moving",
		ExitCode:           1,
		FailureMode:        FailureModeTimeout,
	}

	coord := &AttemptCycleCoordinator{
		Pass: staticCandidateResultPass{
			candidate: CandidateResult{
				Report:       workerCandidateCycleReport(res, true),
				WorktreePath: projectRoot,
			},
		},
		Checks: candidateCheckRunnerFunc(func(_ context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
			checkCalls++
			require.NotEmpty(t, candidate.Report.CandidateRef)
			if checkCalls == 1 {
				return CandidateCheckResult{Passed: false, Detail: "TestMixedCommitFailedACEntersRepair missing"}, nil
			}
			return CandidateCheckResult{Passed: true, Detail: "candidate checks passed after repair"}, nil
		}),
		Repair: repairPassFunc(func(_ context.Context, candidate CandidateResult, prompt string) (CandidateResult, error) {
			repairCalls++
			assert.Contains(t, prompt, "TestMixedCommitFailedACEntersRepair missing")
			return CandidateResult{
				Report: ExecuteBeadReport{
					BeadID:    candidate.Report.BeadID,
					AttemptID: candidate.Report.AttemptID,
					Status:    ExecuteBeadStatusSuccess,
					BaseRev:   candidate.Report.BaseRev,
					ResultRev: candidate.Report.ResultRev,
				},
				WorktreePath: candidate.WorktreePath,
			}, nil
		}),
		NoReview:    true,
		RefStore:    &GitCandidateRefStore{},
		ProjectRoot: projectRoot,
		BeadEvents:  events,
	}

	result, err := coord.Run(context.Background(), res.BeadID)
	require.NoError(t, err)

	assert.Equal(t, 2, checkCalls, "failed ACs must be rechecked after repair")
	assert.Equal(t, 1, repairCalls, "a failed named AC must schedule repair")
	assert.False(t, result.Landed, "failed ACs must not land the candidate")
	assert.NotEmpty(t, result.Report.CandidateRef, "repair recovery must preserve the candidate ref")
	assert.Equal(t, resultRev, result.Report.ResultRev, "repair recovery must preserve the candidate revision")

	got, err := gitRevParse(t, projectRoot, result.Report.CandidateRef)
	require.NoError(t, err)
	assert.Equal(t, resultRev, got)

	require.NotEmpty(t, events.events)
	assert.Equal(t, "candidate-checks-failed", events.events[1].Kind)
}

func TestMixedCommitMachineryExitContinuesNamedChecks(t *testing.T) {
	_, worktreePath, baseRev, resultRev := mixedCommitRunnerModuleFixture(t, false)
	events := &inMemoryEventAppender{}

	runner := &mixedCommitNamedACCheckRunner{
		bead: &bead.Bead{
			ID:         "ddx-mixed-advisory",
			Acceptance: "1. TestMixedCommitNamedACCheckRunnerPasses",
		},
		beadStore: events,
	}

	result, err := runner.RunChecks(context.Background(), "ddx-mixed-advisory", CandidateResult{
		Report: ExecuteBeadReport{
			BeadID:        "ddx-mixed-advisory",
			AttemptID:     "attempt-mixed-advisory",
			BaseRev:       baseRev,
			ResultRev:     resultRev,
			OutcomeReason: FailureModeTimeout,
			Detail:        "context deadline exceeded",
		},
		WorktreePath: worktreePath,
	})
	require.NoError(t, err)

	assert.True(t, result.Passed, "transport-only exits must continue into named AC checks")
	assert.Contains(t, result.Detail, "advisory transport-only failure")
	require.Len(t, events.events, 1, "advisory transport-only exits must emit an operator attention event")
	assert.Equal(t, "operator_attention", events.events[0].Kind)
	assert.Equal(t, "mixed-commit advisory", events.events[0].Summary)
	assert.Contains(t, events.events[0].Body, `"failure_mode":"timeout"`)
}

func TestMixedCommitRunsNamedACChecksOnly(t *testing.T) {
	projectRoot, worktreePath, baseRev, resultRev := mixedCommitRunnerModuleFixture(t, true)
	res := &ExecuteBeadResult{
		BeadID:             "ddx-mixed-packages",
		AttemptID:          "attempt-mixed-packages",
		BaseRev:            baseRev,
		ResultRev:          resultRev,
		Status:             ExecuteBeadStatusExecutionFailed,
		Reason:             mixedCommitAndNoChangesRationaleReason,
		Detail:             mixedCommitAndNoChangesRationaleReason,
		Error:              mixedCommitAndNoChangesRationaleReason,
		NoChangesRationale: "status: open\nreason: keep salvage candidate moving",
		ExitCode:           1,
		FailureMode:        FailureModeTimeout,
	}

	runtime := ExecuteBeadRuntime{
		Reviewer: nil,
	}

	require.NoError(t, applyWorkerCandidateCycle(context.Background(), projectRoot, worktreePath, runtime, res))

	assert.Equal(t, ExecuteBeadStatusSuccess, res.Status, "named AC-only validation must ignore unrelated package failures")
	assert.NotEmpty(t, res.CandidateRef)
}
