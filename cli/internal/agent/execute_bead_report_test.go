package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DocumentDrivenDX/ddx/internal/agent/failclass"
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

func TestAttemptPolicyStageEvidenceSurvivesReportConstruction(t *testing.T) {
	cases := []struct {
		name   string
		result ExecuteBeadResult
	}{
		{
			name: "completed",
			result: ExecuteBeadResult{
				BeadID:        "ddx-completed",
				AttemptID:     "attempt-completed",
				BaseRev:       "base",
				ResultRev:     "result-completed",
				Outcome:       ExecuteBeadOutcomeTaskSucceeded,
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
		},
		{
			name: "retryable",
			result: ExecuteBeadResult{
				BeadID:        "ddx-retryable",
				AttemptID:     "attempt-retryable",
				BaseRev:       "base",
				ResultRev:     "result-retryable",
				Outcome:       ExecuteBeadOutcomeTaskFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
			},
		},
		{
			name: "unavailable-now",
			result: ExecuteBeadResult{
				BeadID:        "ddx-unavailable-now",
				AttemptID:     "attempt-unavailable-now",
				BaseRev:       "base",
				ResultRev:     "result-unavailable-now",
				Outcome:       ExecuteBeadOutcomeTaskNoChanges,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseRouteUnavailable),
				FizeauStage:   string(agentlib.SessionStageRouting),
			},
		},
		{
			name: "cancelled",
			result: ExecuteBeadResult{
				BeadID:        "ddx-cancelled",
				AttemptID:     "attempt-cancelled",
				BaseRev:       "base",
				ResultRev:     "result-cancelled",
				Outcome:       ExecuteBeadOutcomeTaskNoEvidence,
				FizeauOutcome: string(agentlib.SessionOutcomeCancelled),
				FizeauCause:   string(agentlib.TerminalCauseContextCancelled),
				FizeauStage:   string(agentlib.SessionStageCleanup),
			},
		},
		{
			name: "permanent_failure",
			result: ExecuteBeadResult{
				BeadID:        "ddx-permanent",
				AttemptID:     "attempt-permanent",
				BaseRev:       "base",
				ResultRev:     "result-permanent",
				Outcome:       ExecuteBeadOutcomeTaskFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseInternalError),
				FizeauStage:   string(agentlib.SessionStageCleanup),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.result
			populateWorkerStatus(&result)
			before := result

			report := ReportFromExecuteBeadResult(&result, "standard")

			assert.Equal(t, before, result, "report construction must not mutate the source result")
			assert.Equal(t, before.FizeauOutcome, report.FizeauOutcome)
			assert.Equal(t, before.FizeauCause, report.FizeauCause)
			assert.Equal(t, before.FizeauStage, report.FizeauStage)
			assert.Equal(t, before.DDxOwnerStage, report.DDxOwnerStage)
		})
	}
}

// TestExecuteBeadReportUsesTypedAttemptPolicy proves ReportFromExecuteBeadResult
// stamps AttemptPolicyAction/AttemptPolicyReason from the typed attempt-policy
// adapter decision (failclass.DecideAttemptPolicy, reached via
// AttemptPolicyDecisionForResult) for completed, retryable, unavailable-now,
// cancelled, and permanent-failure Fizeau lifecycle tuples — the report is
// constructed from the adapter's decision, not re-derived independently.
func TestExecuteBeadReportUsesTypedAttemptPolicy(t *testing.T) {
	cases := []struct {
		name       string
		result     ExecuteBeadResult
		wantAction failclass.AttemptPolicyAction
		wantReason string
	}{
		{
			name: "completed",
			result: ExecuteBeadResult{
				BeadID:        "ddx-completed",
				AttemptID:     "attempt-completed",
				Outcome:       ExecuteBeadOutcomeTaskSucceeded,
				FizeauOutcome: string(agentlib.SessionOutcomeSuccess),
				FizeauCause:   string(agentlib.TerminalCauseCompleted),
				FizeauStage:   string(agentlib.SessionStageHarness),
			},
			wantAction: failclass.AttemptPolicyActionClose,
			wantReason: "fizeau_outcome_success",
		},
		{
			name: "retryable",
			result: ExecuteBeadResult{
				BeadID:        "ddx-retryable",
				AttemptID:     "attempt-retryable",
				Outcome:       ExecuteBeadOutcomeTaskFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseProviderFailed),
				FizeauStage:   string(agentlib.SessionStageProvider),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name: "unavailable-now",
			result: ExecuteBeadResult{
				BeadID:        "ddx-unavailable-now",
				AttemptID:     "attempt-unavailable-now",
				Outcome:       ExecuteBeadOutcomeTaskFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseRouteUnavailable),
				FizeauStage:   string(agentlib.SessionStageRouting),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_terminal_unavailable_now",
		},
		{
			name: "cancelled",
			result: ExecuteBeadResult{
				BeadID:        "ddx-cancelled",
				AttemptID:     "attempt-cancelled",
				Outcome:       ExecuteBeadOutcomeTaskNoEvidence,
				FizeauOutcome: string(agentlib.SessionOutcomeCancelled),
				FizeauCause:   string(agentlib.TerminalCauseContextCancelled),
				FizeauStage:   string(agentlib.SessionStageCleanup),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_outcome_cancelled",
		},
		{
			name: "permanent_failure",
			result: ExecuteBeadResult{
				BeadID:        "ddx-permanent",
				AttemptID:     "attempt-permanent",
				Outcome:       ExecuteBeadOutcomeTaskFailed,
				FizeauOutcome: string(agentlib.SessionOutcomeFailed),
				FizeauCause:   string(agentlib.TerminalCauseInternalError),
				FizeauStage:   string(agentlib.SessionStageCleanup),
			},
			wantAction: failclass.AttemptPolicyActionPark,
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.result
			populateWorkerStatus(&result)

			// Independently computed adapter decision for the same typed
			// lifecycle tuple: the report must match it exactly, proving
			// construction routes through the adapter rather than a second,
			// parallel classification.
			adapterDecision := AttemptPolicyDecisionForResult(&result)
			if adapterDecision.Action != tc.wantAction || adapterDecision.Reason != tc.wantReason {
				t.Fatalf("test fixture drifted from failclass adapter: got %+v, want action=%q reason=%q", adapterDecision, tc.wantAction, tc.wantReason)
			}

			report := ReportFromExecuteBeadResult(&result, "standard")

			assert.Equal(t, string(tc.wantAction), report.AttemptPolicyAction)
			assert.Equal(t, tc.wantReason, report.AttemptPolicyReason)
			assert.Equal(t, string(adapterDecision.Action), report.AttemptPolicyAction)
			assert.Equal(t, adapterDecision.Reason, report.AttemptPolicyReason)
		})
	}
}

// TestExecuteBeadReportDecisionIsSingular proves that, across a matrix of
// synthetic Fizeau service outcomes (lifecycle tuple) and DDx-owned evidence,
// failclass.DecideAttemptPolicy — the function ReportFromExecuteBeadResult
// consults to populate AttemptPolicyAction — always yields exactly one
// decision from the closed set {current_attempt_repair, new_attempt_retry,
// minimum_strength_escalation_request, park, land, close}, and that the
// report's own AttemptPolicyAction is always one of those six values.
func TestExecuteBeadReportDecisionIsSingular(t *testing.T) {
	validActions := map[failclass.AttemptPolicyAction]bool{
		failclass.AttemptPolicyActionCurrentAttemptRepair:      true,
		failclass.AttemptPolicyActionNewAttemptRetry:           true,
		failclass.AttemptPolicyActionMinimumStrengthEscalation: true,
		failclass.AttemptPolicyActionPark:                      true,
		failclass.AttemptPolicyActionLand:                      true,
		failclass.AttemptPolicyActionClose:                     true,
	}

	lifecycleCases := []struct {
		name   string
		result ExecuteBeadResult
	}{
		{"no_tuple_unknown", ExecuteBeadResult{}},
		{"completed", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeSuccess), FizeauCause: string(agentlib.TerminalCauseCompleted), FizeauStage: string(agentlib.SessionStageHarness)}},
		{"retryable_provider_failed", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseProviderFailed), FizeauStage: string(agentlib.SessionStageProvider)}},
		{"retryable_harness_failed", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseHarnessFailed), FizeauStage: string(agentlib.SessionStageHarness)}},
		{"unavailable_now_route", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseRouteUnavailable), FizeauStage: string(agentlib.SessionStageRouting)}},
		{"unavailable_now_budget", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseBudgetHalted), FizeauStage: string(agentlib.SessionStageHarness)}},
		{"cancelled_by_outcome", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeCancelled), FizeauCause: string(agentlib.TerminalCauseCompleted), FizeauStage: string(agentlib.SessionStageHarness)}},
		{"cancelled_by_cause", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseContextCancelled), FizeauStage: string(agentlib.SessionStageCleanup)}},
		{"permanent_internal_error", ExecuteBeadResult{FizeauOutcome: string(agentlib.SessionOutcomeFailed), FizeauCause: string(agentlib.TerminalCauseInternalError), FizeauStage: string(agentlib.SessionStageCleanup)}},
	}

	evidenceCases := []struct {
		name     string
		evidence failclass.AttemptPolicyEvidence
	}{
		{"no_evidence", failclass.AttemptPolicyEvidence{}},
		{"land_ready", failclass.AttemptPolicyEvidence{LandReady: true}},
		{"current_attempt_repairable", failclass.AttemptPolicyEvidence{CurrentAttemptRepairable: true}},
		{"new_attempt_retry_allowed", failclass.AttemptPolicyEvidence{NewAttemptRetryAllowed: true}},
		{"request_minimum_strength", failclass.AttemptPolicyEvidence{RequestMinimumStrength: true}},
	}

	seen := map[failclass.AttemptPolicyAction]bool{}
	for _, lc := range lifecycleCases {
		for _, ec := range evidenceCases {
			t.Run(lc.name+"/"+ec.name, func(t *testing.T) {
				result := lc.result
				populateWorkerStatus(&result)

				// BuildAttemptPolicyInput + failclass.DecideAttemptPolicy is
				// the exact call sequence AttemptPolicyDecisionForResult (and
				// therefore ReportFromExecuteBeadResult) uses; vary evidence
				// here to reach the full decision space.
				input := BuildAttemptPolicyInput(&result, ec.evidence, failclass.AttemptPolicyAudit{})
				decision := failclass.DecideAttemptPolicy(input)
				if !validActions[decision.Action] {
					t.Fatalf("DecideAttemptPolicy(%s/%s) returned invalid action %q", lc.name, ec.name, decision.Action)
				}
				seen[decision.Action] = true

				report := ReportFromExecuteBeadResult(&result, "standard")
				if !validActions[failclass.AttemptPolicyAction(report.AttemptPolicyAction)] {
					t.Fatalf("ReportFromExecuteBeadResult(%s).AttemptPolicyAction = %q is not one of the six valid decisions", lc.name, report.AttemptPolicyAction)
				}
			})
		}
	}

	for action := range validActions {
		if !seen[action] {
			t.Fatalf("test matrix never exercised action %q; extend lifecycle/evidence cases to cover it", action)
		}
	}
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
	cmd := fixtureGitCommand(t, dir, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v\n%s", args, string(out))
	return strings.TrimSpace(string(out))
}
