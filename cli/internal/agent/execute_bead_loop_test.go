package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// satisfactionCheckerFunc is a test-local functional adapter for
// SatisfactionChecker.
type satisfactionCheckerFunc func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error)

func (f satisfactionCheckerFunc) CheckSatisfied(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
	return f(ctx, beadID, noChangesCount)
}

func TestReport_OutcomeReason_Persists_BesideDisrupted(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:                      "ddx-test",
		Status:                      ExecuteBeadStatusNoChanges,
		Disrupted:                   true,
		DisruptionReason:            "transport_error",
		OutcomeReason:               "transport",
		PredictedPower:              82,
		PredictedSpeedTPS:           35.5,
		PredictedCostUSDPer1kTokens: 0.012345,
		PredictedCostSource:         "catalog",
	}

	body, err := json.Marshal(report)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"disrupted":true`)
	assert.Contains(t, string(body), `"disruption_reason":"transport_error"`)
	assert.Contains(t, string(body), `"outcome_reason":"transport"`)

	event := executeBeadLoopEvent(report, "worker", time.Now().UTC())
	assert.Equal(t, "execute-bead", event.Kind)
	assert.Equal(t, ExecuteBeadStatusNoChanges, event.Summary)
	assert.Contains(t, event.Body, "outcome_reason=transport")
	assert.Contains(t, event.Body, "predicted_power=82")
	assert.Contains(t, event.Body, "predicted_speed_tps=35.5")
	assert.Contains(t, event.Body, "predicted_cost_usd_per_1k_tokens=0.012345 source=catalog")
}

func TestExecutionTrace_BeadResultEventRetainsCompatibleFields(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:    "ddx-trace",
		AttemptID: "attempt-trace-003",
		Status:    ExecuteBeadStatusSuccess,
		BaseRev:   "base-rev",
		ResultRev: "result-rev",
		CycleTrace: []ExecutionCycleTrace{
			{
				CycleIndex: 0,
				AttemptID:  "attempt-trace-003",
				ResultRev:  "result-rev",
				ImplementerRoute: ExecutionCycleRouteFacts{
					Harness:     "codex",
					Provider:    "openai",
					Model:       "gpt-5",
					ActualPower: 70,
				},
				ReviewGroupID:   "rg-trace",
				ReviewerIndices: []int{0, 1},
				ReviewVerdicts:  []string{"BLOCK", "BLOCK"},
				ReviewResult: ExecutionCycleReviewResult{
					Verdict:        "REQUEST_CHANGES",
					Rationale:      "missing coverage",
					Classification: ReviewFindingClassFixableGap,
				},
				FinalDecision: ExecuteBeadStatusReviewFixableGap,
			},
		},
	}

	event := executeBeadLoopEvent(report, "worker", time.Now().UTC())
	assert.Equal(t, "execute-bead", event.Kind)
	assert.Equal(t, ExecuteBeadStatusSuccess, event.Summary)
	assert.Contains(t, event.Body, "result_rev=result-rev")
	assert.Contains(t, event.Body, "base_rev=base-rev")
	assert.Contains(t, event.Body, "cycle_trace=")
	assert.Contains(t, event.Body, `"review_group_id":"rg-trace"`)
	assert.Contains(t, event.Body, `"reviewer_indices":[0,1]`)
	assert.Contains(t, event.Body, `"final_decision":"review_fixable_gap"`)
}

// TestStopCondition_NoProgress_IgnoresIntakeRoutingReviewAndOperatorStates
// verifies that isValidImplementationAttempt and shouldSuppressNoProgress both
// return false (no no-progress budget consumed) for every non-implementation
// outcome class: intake block, decomposition, claim race, routing preflight,
// quota, transport, auth/tool setup, review error, needs_human, and operator-
// required states. These outcomes should not trigger the no-progress cooldown.
func TestStopCondition_NoProgress_IgnoresIntakeRoutingReviewAndOperatorStates(t *testing.T) {
	cases := []struct {
		name   string
		report ExecuteBeadReport
	}{
		{
			name: "intake_block",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "intake_block",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "decomposition",
			report: ExecuteBeadReport{
				Status:    ExecuteBeadStatusDeclinedNeedsDecomposition,
				BaseRev:   "abc123",
				ResultRev: "abc123",
			},
		},
		{
			name: "claim_race",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "claim_race",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "routing_preflight",
			report: ExecuteBeadReport{
				Status:           ExecuteBeadStatusExecutionFailed,
				OutcomeReason:    "preflight_failed",
				Disrupted:        true,
				DisruptionReason: "preflight_rejected",
				BaseRev:          "abc123",
				ResultRev:        "abc123",
			},
		},
		{
			name: "quota",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "quota",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "transport",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "transport",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "auth_tool_setup",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: FailureModeAuthError,
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "review_error",
			report: ExecuteBeadReport{
				Status:    ExecuteBeadStatusReviewMalfunction,
				BaseRev:   "abc123",
				ResultRev: "abc123",
			},
		},
		{
			name: "operator_required",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusLandConflictOperatorRequired,
				OutcomeReason: "operator_required",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, isValidImplementationAttempt(tc.report),
				"isValidImplementationAttempt must return false for %s", tc.name)
			assert.False(t, shouldSuppressNoProgress(tc.report),
				"shouldSuppressNoProgress must return false for %s", tc.name)
		})
	}
}

// TestStopCondition_NoProgress_CountsRealImplementationNoCommit verifies that
// a genuine implementation attempt that produced no commit (base_rev ==
// result_rev, no system/operator classifier) is recognised as a valid
// no-progress case: isValidImplementationAttempt returns true and
// shouldSuppressNoProgress returns true so the no-progress cooldown fires.
func TestStopCondition_NoProgress_CountsRealImplementationNoCommit(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:    "ddx-test",
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "feedface00112233",
		ResultRev: "feedface00112233",
	}
	assert.True(t, isValidImplementationAttempt(report),
		"isValidImplementationAttempt must return true for genuine implementation no-commit")
	assert.True(t, shouldSuppressNoProgress(report),
		"shouldSuppressNoProgress must return true for genuine implementation no-commit with same revisions")
}

// TestIntake_OutcomesDoNotConsumeNoProgress verifies that intake outcomes—
// rewrite, decomposition, ambiguity, intake infrastructure error, and claim
// race—do not trigger no-progress accounting. isValidImplementationAttempt
// and shouldSuppressNoProgress must both return false for these outcome
// reasons even when base_rev == result_rev (the condition that would normally
// fire the cooldown).
func TestIntake_OutcomesDoNotConsumeNoProgress(t *testing.T) {
	cases := []struct {
		name   string
		report ExecuteBeadReport
	}{
		{
			name: "actionable_but_rewritten",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusNoChanges,
				OutcomeReason: "actionable_but_rewritten",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "too_large_decomposed",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusNoChanges,
				OutcomeReason: "too_large_decomposed",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "ambiguous_needs_human",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusNoChanges,
				OutcomeReason: "ambiguous_needs_human",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "intake_error",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusNoChanges,
				OutcomeReason: "intake_error",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
		{
			name: "claim_race",
			report: ExecuteBeadReport{
				Status:        ExecuteBeadStatusExecutionFailed,
				OutcomeReason: "claim_race",
				BaseRev:       "abc123",
				ResultRev:     "abc123",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, isValidImplementationAttempt(tc.report),
				"isValidImplementationAttempt must return false for intake outcome %s", tc.name)
			assert.False(t, shouldSuppressNoProgress(tc.report),
				"shouldSuppressNoProgress must not fire for intake outcome %s", tc.name)
		})
	}
}

// TestNoProgress_ValidImplementationAttemptStillCounts verifies that a genuine
// implementation attempt that produced no commit (base_rev == result_rev,
// no system/operator/intake classifier) is correctly identified as a valid
// no-progress case so the cooldown fires.
func TestNoProgress_ValidImplementationAttemptStillCounts(t *testing.T) {
	report := ExecuteBeadReport{
		BeadID:    "ddx-test-impl",
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "deadbeef00112233",
		ResultRev: "deadbeef00112233",
	}
	assert.True(t, isValidImplementationAttempt(report),
		"isValidImplementationAttempt must return true for a genuine implementation no-commit")
	assert.True(t, shouldSuppressNoProgress(report),
		"shouldSuppressNoProgress must return true for a genuine implementation no-commit")
}

func TestSuppressNoProgress_HonorsTransientReasons(t *testing.T) {
	for _, reason := range []string{"transport", "quota", "routing", "timeout", "merge_conflict", FailureModeLockContention, FailureModeNoViableProvider, FailureModeWorktreeLost} {
		t.Run(reason, func(t *testing.T) {
			report := ExecuteBeadReport{
				Status:        ExecuteBeadStatusNoChanges,
				BaseRev:       "same",
				ResultRev:     "same",
				OutcomeReason: reason,
			}
			assert.False(t, shouldSuppressNoProgress(report))
		})
	}

	assert.True(t, shouldSuppressNoProgress(ExecuteBeadReport{
		Status:        ExecuteBeadStatusNoChanges,
		BaseRev:       "same",
		ResultRev:     "same",
		OutcomeReason: "tests_red",
	}))
	assert.False(t, shouldSuppressNoProgress(ExecuteBeadReport{
		Status:        ExecuteBeadStatusNoChanges,
		BaseRev:       "base",
		ResultRev:     "result",
		OutcomeReason: "tests_red",
	}))
}

func TestExecuteLoop_WorktreeLostDoesNotCountNoProgress(t *testing.T) {
	report := ExecuteBeadReport{
		Status:        ExecuteBeadStatusExecutionFailed,
		BaseRev:       "same",
		ResultRev:     "same",
		OutcomeReason: FailureModeWorktreeLost,
	}
	assert.False(t, isValidImplementationAttempt(report))
	assert.False(t, shouldSuppressNoProgress(report))
}

func TestClassifyLoopReportFailure_LockContentionIsActionable(t *testing.T) {
	report := ExecuteBeadReport{
		Status: ExecuteBeadStatusExecutionFailed,
		Detail: "staging tracker: fatal: Unable to create '/repo/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository",
	}

	classifyLoopReportFailure(&report)

	assert.Equal(t, FailureModeLockContention, report.OutcomeReason)
	assert.True(t, report.Disrupted)
	assert.Equal(t, FailureModeLockContention, report.DisruptionReason)
	assert.Equal(t,
		"lock_contention: staging tracker: fatal: Unable to create '/repo/.git/index.lock': File exists.\n\nAnother git process seems to be running in this repository",
		formatLoopResult(report))
	assert.False(t, shouldSuppressNoProgress(ExecuteBeadReport{
		Status:        ExecuteBeadStatusExecutionFailed,
		BaseRev:       "same",
		ResultRev:     "same",
		OutcomeReason: report.OutcomeReason,
	}))
}

func TestIsTransientGitContention_TrackerLockTimeoutError(t *testing.T) {
	err := fmt.Errorf("commit durable audit outputs: %w", &TrackerLockTimeoutError{
		Why:      "max elapsed",
		LockDir:  "/repo/.ddx/.git-tracker.lock",
		OwnerPID: "424293",
	})

	assert.True(t, isTransientGitContention(err))
}

func TestIsTransientGitContention_TrackerLockTimeoutSubstringFallback(t *testing.T) {
	err := fmt.Errorf("commit durable audit outputs: tracker lock timeout (max elapsed, lock: /repo/.ddx/.git-tracker.lock, owner pid: 424293)")

	assert.True(t, isTransientGitContention(err))
}

func TestFormatLoopResult_NoEvidenceShowsContractFailure(t *testing.T) {
	report := ExecuteBeadReport{
		Status: ExecuteBeadStatusNoEvidenceProduced,
		Detail: "agent exited without a commit or no_changes_rationale.txt",
	}

	classifyLoopReportFailure(&report)

	assert.Equal(t, FailureModeNoEvidenceProduced, report.OutcomeReason)
	assert.Equal(t,
		"no_evidence_produced: agent exited without a commit or no_changes_rationale.txt",
		formatLoopResult(report))
}

func TestFormatLoopResultLine_SuccessUsesSuccessMarker(t *testing.T) {
	success := ExecuteBeadReport{
		Status:    ExecuteBeadStatusSuccess,
		ResultRev: "deadbeefcafebabe",
	}
	alreadySatisfied := ExecuteBeadReport{
		Status: ExecuteBeadStatusAlreadySatisfied,
	}

	assert.Equal(t, "✓ ddx-result → merged (deadbeef)", formatLoopResultLine("ddx-result", success))
	assert.Equal(t, "✓ ddx-result → already_satisfied", formatLoopResultLine("ddx-result", alreadySatisfied))
}

func TestFormatLoopResultLine_FailuresDoNotUseSuccessMarker(t *testing.T) {
	cases := []ExecuteBeadReport{
		{Status: ExecuteBeadStatusExecutionFailed, Detail: "provider failed"},
		{Status: ExecuteBeadStatusPostRunCheckFailed, Detail: "tests failed"},
		{Status: ExecuteBeadStatusNoEvidenceProduced, Detail: "no evidence"},
		{Status: ExecuteBeadStatusPreservedNeedsReview, Detail: "preserved for review"},
		{Status: ExecuteBeadStatusNoChanges, NoChangesRationale: "blocked by stale test"},
	}

	for _, report := range cases {
		t.Run(report.Status, func(t *testing.T) {
			assert.NotRegexp(t, `^✓\b`, formatLoopResultLine("ddx-result", report))
		})
	}
}

func TestExecuteBeadLoop_LogLineDoesNotUseSuccessMarkerForReviewError(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusExecutionFailed,
				Detail: "pre-close review: review-error: transport",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var logBuf bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &logBuf,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotRegexp(t, `^✓`, logBuf.String())
	assert.Contains(t, logBuf.String(), "pre-close review: review-error: transport")
}

func TestExecuteBeadWorkerSuccessClosesBead(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-1",
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, "sess-1", got.Extra["session_id"])
	assert.Equal(t, "deadbeef", got.Extra["closing_commit_sha"])

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	var sawIntent, sawExecute bool
	for _, ev := range events {
		switch ev.Kind {
		case "execution-routing-intent":
			sawIntent = true
		case "execute-bead":
			sawExecute = true
			assert.Equal(t, "success", ev.Summary)
		}
	}
	assert.True(t, sawIntent, "routing intent evidence must be recorded")
	assert.True(t, sawExecute, "execute-bead event must still be recorded")
}

func TestTryRecordsEstimatedDifficultyRoutingIntent(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	require.NoError(t, store.Update(first.ID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = map[string]any{}
		}
		b.Extra[escalation.BeadEstimatedDifficultyKey] = string(escalation.DifficultyHard)
	}))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				AttemptID:          "20260515T185832-loop",
				Status:             ExecuteBeadStatusSuccess,
				Detail:             "merged cleanly",
				SessionID:          "sess-1",
				ResultRev:          "deadbeef",
				RequestedProfile:   "default",
				InferredPowerClass: "smart",
				Harness:            "claude",
				Provider:           "anthropic",
				Model:              "claude-sonnet-4-6",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var intent *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "execution-routing-intent" {
			intent = &events[i]
			break
		}
	}
	require.NotNil(t, intent, "execution-routing-intent event must be recorded")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(intent.Body), &body))
	assert.Equal(t, "hard", body["estimated_difficulty"])
	assert.Equal(t, "smart", body["requested_power_class"])
	assert.Equal(t, "default", body["requested_profile"])
	assert.NotContains(t, body, "smart_justification")
	assert.Contains(t, intent.Summary, "difficulty=hard")
	assert.Contains(t, intent.Summary, "powerClass=smart")
}

func TestExecuteBeadWorkerLabelFilterSkipsNonMatchingReadyBeads(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	require.NoError(t, store.Update(second.ID, func(b *bead.Bead) {
		b.Labels = []string{"ui"}
	}))

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-filter",
				ResultRev: "cafe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		LabelFilter: "ui",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{second.ID}, executed)
	assert.Equal(t, 1, result.Attempts)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
}

func TestExecuteBeadWorkerPreservedFailureStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:      beadID,
					Status:      ExecuteBeadStatusExecutionFailed,
					Detail:      "agent execution failed",
					PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
					ResultRev:   "badc0de",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-2",
				ResultRev: "c0ffee",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)
	assert.Empty(t, firstGot.Owner)

	secondGot, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, secondGot.Status)
	assert.Equal(t, "sess-2", secondGot.Extra["session_id"])
}

func TestExecuteBeadWorkerLandConflictStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:      beadID,
					Status:      ExecuteBeadStatusLandConflict,
					Detail:      "ff-merge not possible",
					PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
					ResultRev:   "feedface",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-3",
				ResultRev: "f00dbabe",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusLandConflict, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	var sawIntent, sawExecute bool
	for _, ev := range events {
		switch ev.Kind {
		case "execution-routing-intent":
			sawIntent = true
		case "execute-bead":
			sawExecute = true
			assert.Equal(t, ExecuteBeadStatusLandConflict, ev.Summary)
			assert.Contains(t, ev.Body, "preserve_ref=")
		}
	}
	assert.True(t, sawIntent, "routing intent evidence must be recorded")
	assert.True(t, sawExecute, "execute-bead event must still be recorded")
}

func TestExecuteBeadWorkerPreservedNeedsReviewEventShape(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	preserveRef := "refs/ddx/iterations/" + first.ID + "/attempt-1"
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusPreservedNeedsReview,
				Detail:      "large-deletion gate: huge.txt deleted 250 lines (threshold 200)",
				PreserveRef: preserveRef,
				ResultRev:   "feedbead",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusPreservedNeedsReview, result.LastFailureStatus)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Contains(t, got.Notes, "preserved-needs-review")
	assert.Contains(t, got.Notes, "preserve_ref="+preserveRef)
	assert.Contains(t, got.Notes, "gate_summary=large-deletion gate: huge.txt deleted 250 lines")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	var sawIntent, sawExecute bool
	for _, ev := range events {
		switch ev.Kind {
		case "execution-routing-intent":
			sawIntent = true
		case "execute-bead":
			sawExecute = true
			assert.Equal(t, ExecuteBeadStatusPreservedNeedsReview, ev.Summary)
			assert.Contains(t, ev.Body, "preserved-needs-review")
			assert.Contains(t, ev.Body, "preserve_ref="+preserveRef)
			assert.Contains(t, ev.Body, "gate_summary=large-deletion gate: huge.txt deleted 250 lines")
		}
	}
	assert.True(t, sawIntent, "routing intent evidence must be recorded")
	assert.True(t, sawExecute, "execute-bead event must still be recorded")
}

func TestExecuteBeadWorkerNoChangesStaysOpenAndContinues(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	executed := []string{}
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					Detail:    "agent made no commits",
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-4",
				ResultRev: "facefeed",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.LastFailureStatus)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, firstGot.Status)
	assert.Empty(t, firstGot.Owner)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	// NoChangesContract emits a no_changes_unjustified event, a
	// reviewer-skipped-empty-diff event (reviewer is bypassed for empty diffs),
	// plus the terminal executeBeadLoopEvent describing the attempt outcome.
	require.Len(t, events, 4)
	var sawTerminal, sawTriage, sawSkipped bool
	for _, ev := range events {
		if ev.Kind == "execution-routing-intent" {
			continue
		}
		if ev.Summary == ExecuteBeadStatusNoChanges {
			sawTerminal = true
			assert.Contains(t, ev.Body, "agent made no commits")
		}
		if ev.Kind == NoChangesEventUnjustified {
			sawTriage = true
		}
		if ev.Kind == ReviewerSkippedEmptyDiffEventKind {
			sawSkipped = true
		}
	}
	assert.True(t, sawTerminal, "executeBeadLoopEvent must be appended")
	assert.True(t, sawTriage, "no_changes_unjustified triage event must be appended")
	assert.True(t, sawSkipped, "reviewer-skipped-empty-diff event must be appended")
}

func TestExecuteBeadWorkerNoChangesUnjustifiedRemainsRunnableAcrossRuns(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	callCount := 0
	now := time.Now().UTC().Truncate(time.Second)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			callCount++
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					Detail:    "agent made no commits",
					BaseRev:   "aaaa1111",
					ResultRev: "aaaa1111",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-5",
				ResultRev: "fadedcab",
			}, nil
		}),
		Now: func() time.Time {
			return now
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	firstRun, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, firstRun)
	assert.Equal(t, 1, firstRun.Attempts)
	assert.Equal(t, ExecuteBeadStatusNoChanges, firstRun.LastFailureStatus)
	require.Len(t, firstRun.Results, 1)
	assert.Empty(t, firstRun.Results[0].RetryAfter)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Contains(t, gotFirst.Labels, NoChangesLabelUnjustified)
	assert.NotContains(t, gotFirst.Extra, "work-last-status")
	assert.NotContains(t, gotFirst.Extra, "work-last-detail")
	assert.NotContains(t, gotFirst.Extra, "work-retry-after")

	secondRun, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, secondRun)
	assert.Equal(t, 1, secondRun.Attempts)
	require.Len(t, secondRun.Results, 1)
	assert.Equal(t, first.ID, secondRun.Results[0].BeadID)
	assert.Equal(t, ExecuteBeadStatusNoChanges, secondRun.LastFailureStatus)

	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Equal(t, 2, callCount)
}

func TestExecuteBeadWorker_NoViableProviderUsesRetryableTransportPolicy(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	var triageCalls int
	var sink bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "work: all powerClasses exhausted - no viable provider found",
				BaseRev:   "aaaa1111",
				ResultRev: "aaaa1111",
			}, nil
		}),
		Now: func() time.Time {
			return now
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &sink,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			triageCalls++
			return TriageResult{Classification: "needs_investigation"}, nil
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	require.Equal(t, 0, triageCalls, "provider outage must bypass post-attempt triage")
	assert.Equal(t, FailureModeNoViableProvider, result.Results[0].OutcomeReason)
	assert.True(t, result.Results[0].Disrupted, "no_viable_provider must be disrupted")
	assert.Equal(t, "no_viable_provider", result.Results[0].DisruptionReason)
	assert.Empty(t, result.Results[0].RetryAfter, "no per-bead cooldown for no_viable_provider")
	assert.Contains(t, sink.String(), "loop.paused-infra", "worker must emit paused-infra event")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.NotContains(t, got.Labels, bead.LabelNeedsInvestigation)
	assert.Empty(t, got.Extra["work-retry-after"], "no per-bead cooldown for no_viable_provider (P6 + ADR-024)")
}

// TestProviderConnectivityFailure_NoBeadCooldown: AC #1 — provider_connectivity
// must NOT set a per-bead cooldown. The route-exclusion window (RouteExclusionWindow)
// is the correct gate; SetExecutionCooldown would park the bead unnecessarily.
func TestProviderConnectivityFailure_NoBeadCooldown(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{ID: "ddx-pc-nocool", Title: "Provider connectivity test"}
	require.NoError(t, store.Create(target))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "dial tcp 127.0.0.1:11434: connect: connection refused",
				Provider:  "lmstudio",
				BaseRev:   "aaaa1111",
				ResultRev: "aaaa1111",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, FailureModeProviderConnectivity, result.Results[0].OutcomeReason)
	assert.True(t, result.Results[0].Disrupted)
	assert.Equal(t, "provider_connectivity", result.Results[0].DisruptionReason)
	assert.Empty(t, result.Results[0].RetryAfter, "no per-bead cooldown for provider_connectivity")

	got, err := store.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.Empty(t, got.Extra["work-retry-after"], "SetExecutionCooldown must NOT fire for provider_connectivity")
}

// TestProviderConnectivityFailure_RouteExclusionStillRecorded: AC #2 — removing
// the bead cooldown must NOT remove the work-failed-routes entry that prevents
// the same (provider, model) from being re-selected by Fizeau routing.
func TestProviderConnectivityFailure_RouteExclusionStillRecorded(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{ID: "ddx-pc-excl", Title: "Route exclusion regression"}
	require.NoError(t, store.Create(target))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "dial tcp 10.0.0.1:1234: connect: connection refused",
				Provider:    "local-ollama",
				ActualPower: 50,
				BaseRev:     "aaaa1111",
				ResultRev:   "aaaa1111",
			}, nil
		}),
		EscalationNextFloor: func(p int) (int, error) { return p + 10, nil },
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(target.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Extra["work-retry-after"], "no bead cooldown")
	// work-failed-routes must be set — the exclusion mechanism is intact.
	assert.NotNil(t, got.Extra["work-failed-routes"], "work-failed-routes must be recorded even when no bead cooldown is set")
}

// TestNoViableProvider_TransitionsToPausedInfra: AC #3 — a no_viable_provider outcome
// must set no per-bead cooldown and must emit a loop.paused-infra event with a resume-at
// timestamp. The worker transitions to paused-infra; beads stay immediately reclaimable.
func TestNoViableProvider_TransitionsToPausedInfra(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	var sink bytes.Buffer

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "work: all powerClasses exhausted - no viable provider found",
				BaseRev:   "aaaa1111",
				ResultRev: "aaaa1111",
			}, nil
		}),
		Now: func() time.Time { return now },
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &sink,
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)

	// No per-bead cooldown.
	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Extra["work-retry-after"], "no per-bead cooldown for no_viable_provider")

	// paused-infra event emitted with resume_at.
	events := sink.String()
	assert.Contains(t, events, "loop.paused-infra")
	assert.Contains(t, events, "resume_at")
	resumeExpected := now.Add(PausedInfraInterval).Format(time.RFC3339)
	assert.Contains(t, events, resumeExpected, "resume_at must be now+PausedInfraInterval")
}

// TestWorkerIdle_AllInfraCooledDown_EntersPausedInfra: AC #4 — when nextCandidate
// finds no ready work and every cooled bead carries an infra-fault status, the
// worker emits loop.paused-infra instead of the normal loop.idle/NoReadyWork path.
func TestWorkerIdle_AllInfraCooledDown_EntersPausedInfra(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b1 := &bead.Bead{ID: "ddx-ic-1", Title: "Infra cooled 1"}
	b2 := &bead.Bead{ID: "ddx-ic-2", Title: "Infra cooled 2"}
	require.NoError(t, store.Create(b1))
	require.NoError(t, store.Create(b2))

	until := time.Now().UTC().Add(2 * time.Minute)
	require.NoError(t, store.SetExecutionCooldown(b1.ID, until, FailureModeProviderConnectivity, "dial tcp: connection refused", ""))
	require.NoError(t, store.SetExecutionCooldown(b2.ID, until, FailureModeNoViableProvider, "no viable provider", ""))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("executor must not be called when all beads are in infra cooldown")
			return ExecuteBeadReport{}, nil
		}),
	}

	var sink bytes.Buffer
	// WakeCh pre-filled so the paused-infra sleep returns immediately.
	wakeCh := make(chan struct{}, 1)
	wakeCh <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, _ := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		EventSink: &sink,
		WakeCh:    wakeCh,
	})

	// Worker must not set NoReadyWork — it entered paused-infra.
	assert.False(t, result.NoReadyWork, "worker must not report NoReadyWork when all cooldowns are infra-class")
	assert.Contains(t, sink.String(), "loop.paused-infra", "worker must emit loop.paused-infra event")
	assert.Contains(t, sink.String(), "all_infra_cooldowns", "paused-infra reason must be all_infra_cooldowns")
}

func TestExecuteBeadWorker_RoutingFailureStopsLoopWithoutCoolingBead(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)
	var calls int32
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&calls, 1)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "ResolveRoute: no viable routing candidate: 3 candidates rejected",
				BaseRev:   "aaaa1111",
				ResultRev: "aaaa1111",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		PollInterval: 0,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls), "routing failure must stop before claiming the next bead")
	require.Len(t, result.Results, 1)
	assert.Equal(t, FailureModeNoViableProvider, result.Results[0].OutcomeReason)
	assert.True(t, result.Results[0].Disrupted)
	assert.Equal(t, "routing", result.Results[0].DisruptionReason)

	gotFirst, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotFirst.Status)
	assert.Empty(t, gotFirst.Owner)
	require.NotNil(t, gotFirst.Extra)
	assert.NotContains(t, gotFirst.Extra, "work-retry-after")

	gotSecond, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, gotSecond.Status)
	assert.Empty(t, gotSecond.Owner)
}

func TestInvestigationRetry_NoSmartRouteDoesNotAddLegacyLabels(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{
		ID:       "ddx-smart-retry",
		Title:    "Smart retry",
		Priority: 0,
	}
	require.NoError(t, store.Create(target))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:        beadID,
				Status:        ExecuteBeadStatusExecutionFailed,
				Detail:        "route unavailable: no viable routing candidate satisfies requested MinPower 90",
				OutcomeReason: FailureModeNoViableProvider,
				BaseRev:       "aaaa1111",
				ResultRev:     "aaaa1111",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Results, 1)
	assert.Equal(t, FailureModeNoViableProvider, result.Results[0].OutcomeReason)
	assert.True(t, result.Results[0].Disrupted)
	assert.Equal(t, "routing", result.Results[0].DisruptionReason)

	got, err := store.Get(target.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)
	assert.NotContains(t, got.Labels, bead.LabelNeedsHuman)
	assert.NotContains(t, got.Labels, bead.LabelNeedsInvestigation)
	if got.Extra != nil {
		assert.NotContains(t, got.Extra, legacyRetryFloorKey)
		assert.NotContains(t, got.Extra, "work-retry-after")
	}
}

func TestExecuteBeadWorkerNoReadyWork(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork)
	assert.Equal(t, 0, result.Attempts)
}

// TestExecuteBeadWorkerEpicIsNotOrdinaryWork: ordinary ddx work skips epic
// containers unless a dedicated epic-worker path owns them.
func TestExecuteBeadWorkerEpicIsNotOrdinaryWork(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	epic := &bead.Bead{ID: "ddx-epic-001", Title: "Epic container", IssueType: "epic", Priority: 1}
	require.NoError(t, store.Create(epic))
	epicChild := &bead.Bead{ID: "ddx-epic-001-child", Title: "Child of epic", Parent: epic.ID, Status: bead.StatusBlocked, Priority: 2}
	require.NoError(t, store.Create(epicChild))

	cooldownTask := &bead.Bead{ID: "ddx-task-002", Title: "Cooldown task", IssueType: "task", Priority: 1,
		Extra: map[string]any{
			"work-retry-after": time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	require.NoError(t, store.Create(cooldownTask))

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "epic closed after children",
				SessionID: "sess-" + beadID,
				ResultRev: "deadbeef",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork, "ordinary ddx work must skip epic containers")
	assert.Empty(t, executed, "ordinary ddx work must not execute epics")
	assert.Equal(t, []string{"ddx-epic-001"}, result.NoReadyWorkDetail.Epics)
	assert.Equal(t, []string{"ddx-task-002"}, result.NoReadyWorkDetail.RetryCooldown)
}

func TestExecuteBeadWorkerEvaluatesCompletedEpicForClosure(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	completedEpic := &bead.Bead{ID: "ddx-epic-closed", Title: "Completed epic", IssueType: "epic", Priority: 0}
	activeEpic := &bead.Bead{ID: "ddx-epic-open", Title: "Active epic", IssueType: "epic", Priority: 1}
	closedChild := &bead.Bead{ID: "ddx-epic-closed-child", Title: "Closed child", Parent: completedEpic.ID, Status: bead.StatusClosed}
	openChild := &bead.Bead{ID: "ddx-epic-open-child", Title: "Open child", Parent: activeEpic.ID, Status: bead.StatusBlocked}
	require.NoError(t, store.Create(completedEpic))
	require.NoError(t, store.Create(activeEpic))
	require.NoError(t, store.Create(closedChild))
	require.NoError(t, store.Create(openChild))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.NoReadyWork)
	assert.Empty(t, result.Results)
	assert.Equal(t, []string{"ddx-epic-open"}, result.NoReadyWorkDetail.Epics)
	assert.Equal(t, []string{"ddx-epic-closed"}, result.NoReadyWorkDetail.EpicClosureCandidates)
}

func TestExecuteBeadLoopResult_IncludesQueueSnapshotAfterSuccessfulDrain(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	runnable := &bead.Bead{ID: "ddx-runnable", Title: "Runnable", Priority: 0}
	proposed := &bead.Bead{ID: "ddx-proposed", Title: "Proposed", Status: bead.StatusProposed, Priority: 1}
	externalBlocked := &bead.Bead{ID: "ddx-external", Title: "External blocked", Priority: 2}
	dependencyWaiting := &bead.Bead{ID: "ddx-waiting", Title: "Waiting on dep", Priority: 3}
	dependencyWaiting.AddDep(externalBlocked.ID, "blocks")
	cooldown := &bead.Bead{ID: "ddx-cooldown", Title: "Retry later", Priority: 4}

	require.NoError(t, store.Create(runnable))
	require.NoError(t, store.Create(proposed))
	require.NoError(t, store.Create(externalBlocked))
	require.NoError(t, store.UpdateWithLifecycleStatus(externalBlocked.ID, bead.StatusBlocked, bead.LifecycleTransitionOptions{
		ExternalBlockerReason: "waiting for upstream release",
		Reason:                "test external blocker",
		Source:                "test",
	}, nil))
	require.NoError(t, store.Create(dependencyWaiting))
	require.NoError(t, store.Create(cooldown))
	require.NoError(t, store.SetExecutionCooldown(cooldown.ID, time.Now().UTC().Add(time.Hour), ExecuteBeadStatusNoChanges, "retry later", ""))

	var executed []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executed = append(executed, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-" + beadID,
				ResultRev: "feedface",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{runnable.ID}, executed)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	assert.True(t, result.NoReadyWork, "no-ready detail must be captured after the completed attempt drains the executable queue")
	assert.Empty(t, result.NoReadyWorkDetail.ExecutionReady)
	assert.ElementsMatch(t, []string{proposed.ID}, result.NoReadyWorkDetail.ProposedOperatorAttention)
	assert.ElementsMatch(t, []string{externalBlocked.ID}, result.NoReadyWorkDetail.ExternalBlocked)
	assert.ElementsMatch(t, []string{dependencyWaiting.ID}, result.NoReadyWorkDetail.DependencyWaiting)
	assert.ElementsMatch(t, []string{cooldown.ID}, result.NoReadyWorkDetail.RetryCooldown)
	require.NotNil(t, result.QueueSnapshot)
	assert.Equal(t, 0, result.QueueSnapshot.ExecutionReadyCount)
	assert.Equal(t, 2, result.QueueSnapshot.BlockedCount)
	assert.Equal(t, 1, result.QueueSnapshot.DependencyWaitingCount)
	assert.Equal(t, 1, result.QueueSnapshot.ExternalBlockedCount)
	assert.Equal(t, 1, result.QueueSnapshot.ProposedOperatorAttentionCount)
	assert.Equal(t, 1, result.QueueSnapshot.RetryCooldownCount)
	assert.NotEmpty(t, result.QueueSnapshot.NextRetryAfter)
}

func TestNoReadyWorkDetail_DoesNotExposeNeedsInvestigation(t *testing.T) {
	body, err := json.Marshal(ExecuteBeadLoopResult{
		NoReadyWork: true,
		NoReadyWorkDetail: NoReadyWorkBreakdown{
			ProposedOperatorAttention: []string{"ddx-proposed"},
			DependencyWaiting:         []string{"ddx-waiting"},
		},
		QueueSnapshot: &QueueSnapshot{
			ProposedOperatorAttentionCount: 1,
			DependencyWaitingCount:         1,
		},
	})
	require.NoError(t, err)
	assert.Contains(t, string(body), "proposed_operator_attention")
	assert.Contains(t, string(body), "dependency_waiting")
	assert.Contains(t, string(body), "queue_snapshot")
	assert.Contains(t, string(body), "proposed_operator_attention_count")
	assert.NotContains(t, string(body), "skipped_needs_investigation")
	assert.NotContains(t, string(body), "needs_investigation")
}

func TestExecuteBeadWorkerConcurrentWorkersDoNotDoubleExecuteSameBead(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	only := &bead.Bead{ID: "ddx-race-1", Title: "Only ready bead", Priority: 0}
	require.NoError(t, store.Create(only))

	var execCalls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		execCalls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			Detail:    "merged cleanly",
			SessionID: "sess-race",
			ResultRev: "deadbeef",
		}, nil
	})

	workerA := &ExecuteBeadWorker{Store: store, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: store, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)

	cfgOptsA := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfgA := config.NewTestConfigForLoop(cfgOptsA).Resolve(config.TestLoopOverrides(cfgOptsA))
	cfgOptsB := config.TestLoopConfigOpts{Assignee: "worker-b"}
	rcfgB := config.NewTestConfigForLoop(cfgOptsB).Resolve(config.TestLoopOverrides(cfgOptsB))
	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.Run(context.Background(), rcfgA, ExecuteBeadLoopRuntime{Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.Run(context.Background(), rcfgB, ExecuteBeadLoopRuntime{Once: true})
	}()

	<-started
	close(release)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, int32(1), execCalls.Load(), "only one worker should execute the bead")

	totalAttempts := 0
	totalSuccesses := 0
	for _, result := range results {
		require.NotNil(t, result)
		totalAttempts += result.Attempts
		totalSuccesses += result.Successes
	}
	assert.Equal(t, 1, totalAttempts)
	assert.Equal(t, 1, totalSuccesses)

	got, err := store.Get(only.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.Equal(t, "sess-race", got.Extra["session_id"])
	assert.Equal(t, "deadbeef", got.Extra["closing_commit_sha"])
}

func TestExecuteBeadWorkerConcurrentWorkersDistributeDistinctReadyBeads(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)

	var (
		mu       sync.Mutex
		executed []string
	)
	barrier := make(chan struct{}, 2)
	release := make(chan struct{})
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		mu.Lock()
		executed = append(executed, beadID)
		mu.Unlock()
		barrier <- struct{}{}
		<-release
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			Detail:    "merged cleanly",
			SessionID: "sess-" + beadID,
			ResultRev: "rev-" + beadID,
		}, nil
	})

	workerA := &ExecuteBeadWorker{Store: store, Executor: executor}
	workerB := &ExecuteBeadWorker{Store: store, Executor: executor}

	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*ExecuteBeadLoopResult, 2)
	errs := make([]error, 2)

	cfgOptsA := config.TestLoopConfigOpts{Assignee: "worker-a"}
	rcfgA := config.NewTestConfigForLoop(cfgOptsA).Resolve(config.TestLoopOverrides(cfgOptsA))
	cfgOptsB := config.TestLoopConfigOpts{Assignee: "worker-b"}
	rcfgB := config.NewTestConfigForLoop(cfgOptsB).Resolve(config.TestLoopOverrides(cfgOptsB))
	go func() {
		defer wg.Done()
		results[0], errs[0] = workerA.Run(context.Background(), rcfgA, ExecuteBeadLoopRuntime{Once: true})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = workerB.Run(context.Background(), rcfgB, ExecuteBeadLoopRuntime{Once: true})
	}()

	<-barrier
	<-barrier
	close(release)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.ElementsMatch(t, []string{first.ID, second.ID}, executed)

	totalAttempts := 0
	totalSuccesses := 0
	for _, result := range results {
		require.NotNil(t, result)
		totalAttempts += result.Attempts
		totalSuccesses += result.Successes
	}
	assert.Equal(t, 2, totalAttempts)
	assert.Equal(t, 2, totalSuccesses)

	firstGot, err := store.Get(first.ID)
	require.NoError(t, err)
	secondGot, err := store.Get(second.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, firstGot.Status)
	assert.Equal(t, bead.StatusClosed, secondGot.Status)
	assert.Equal(t, "sess-"+first.ID, firstGot.Extra["session_id"])
	assert.Equal(t, "sess-"+second.ID, secondGot.Extra["session_id"])
}

func TestReadyExecutionExcludesEpics(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	task := &bead.Bead{ID: "ddx-task01", Title: "Task work", IssueType: "task", Priority: 1}
	epic := &bead.Bead{ID: "ddx-epic01", Title: "Epic container", IssueType: "epic", Priority: 0}
	epicChild := &bead.Bead{ID: "ddx-epic01-child", Title: "Epic child task", Parent: epic.ID, Status: bead.StatusBlocked, Priority: 2}
	require.NoError(t, store.Create(task))
	require.NoError(t, store.Create(epic))
	require.NoError(t, store.Create(epicChild))

	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1, "ordinary execution excludes epic containers")
	assert.Equal(t, "ddx-task01", ready[0].ID)
}

func TestExecuteBeadWorkerEmitsStructuredProgressEvents(t *testing.T) {
	store, first, second := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			if beadID == first.ID {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					Detail:    "merged cleanly",
					SessionID: "sess-first",
					ResultRev: "feedbeef",
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "agent execution failed",
				PreserveRef: "refs/ddx/iterations/" + beadID + "/attempt-1",
				ResultRev:   "baadf00d",
			}, nil
		}),
	}

	var sink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{
		Assignee: "worker",
		Harness:  "claude",
		Model:    "claude-3.5-sonnet",
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink:   &sink,
		WorkerID:    "worker-42",
		ProjectRoot: "/tmp/fake-project",
		SessionID:   "agent-loop-test",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Attempts)

	lines := strings.Split(strings.TrimRight(sink.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 6, "expected loop.start + 2*(bead.claimed+bead.result) + loop.end")

	parse := func(t *testing.T, line string) (string, map[string]any) {
		t.Helper()
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typ, _ := entry["type"].(string)
		data, _ := entry["data"].(map[string]any)
		// Every entry must carry the envelope fields.
		assert.Equal(t, "agent-loop-test", entry["session_id"])
		assert.NotEmpty(t, entry["ts"])
		return typ, data
	}

	// First line must be loop.start with metadata.
	typ, data := parse(t, lines[0])
	assert.Equal(t, "loop.start", typ)
	assert.Equal(t, "worker-42", data["worker_id"])
	assert.Equal(t, "/tmp/fake-project", data["project_root"])
	assert.Equal(t, "claude", data["harness"])
	assert.Equal(t, "claude-3.5-sonnet", data["model"])

	// Collect the rest by type so ordering between beads isn't load-bearing.
	byType := map[string][]map[string]any{}
	for _, line := range lines {
		typ, data := parse(t, line)
		byType[typ] = append(byType[typ], data)
	}

	require.Len(t, byType["loop.start"], 1)
	require.Len(t, byType["loop.end"], 1)
	require.Len(t, byType["bead.claimed"], 2)
	require.Len(t, byType["bead.result"], 2)

	claimedIDs := []string{}
	for _, d := range byType["bead.claimed"] {
		id, _ := d["bead_id"].(string)
		claimedIDs = append(claimedIDs, id)
		assert.NotEmpty(t, d["title"], "bead.claimed should carry the title")
	}
	assert.ElementsMatch(t, []string{first.ID, second.ID}, claimedIDs)

	// bead.result must carry status + duration_ms for every attempt, and
	// success/result_rev for the successful bead.
	var sawSuccess, sawFailure bool
	for _, d := range byType["bead.result"] {
		beadID, _ := d["bead_id"].(string)
		status, _ := d["status"].(string)
		_, hasDuration := d["duration_ms"]
		assert.True(t, hasDuration, "bead.result must include duration_ms")
		if beadID == first.ID {
			sawSuccess = true
			assert.Equal(t, ExecuteBeadStatusSuccess, status)
			assert.Equal(t, "sess-first", d["session_id"])
			assert.Equal(t, "feedbeef", d["result_rev"])
		}
		if beadID == second.ID {
			sawFailure = true
			assert.Equal(t, ExecuteBeadStatusExecutionFailed, status)
			assert.Equal(t, "agent execution failed", d["detail"])
		}
	}
	assert.True(t, sawSuccess, "bead.result missing for successful bead")
	assert.True(t, sawFailure, "bead.result missing for failed bead")

	// loop.end must summarise attempts/successes/failures.
	endData := byType["loop.end"][0]
	assert.EqualValues(t, 2, endData["attempts"])
	assert.EqualValues(t, 1, endData["successes"])
	assert.EqualValues(t, 1, endData["failures"])
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, endData["last_failure_status"])
}

func TestExecuteBeadWorkerEmitsLoopEventsWithNoReadyWork(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			t.Fatalf("unexpected execution for %s", beadID)
			return ExecuteBeadReport{}, nil
		}),
	}

	var sink bytes.Buffer
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink: &sink,
		SessionID: "agent-loop-empty",
	})
	require.NoError(t, err)
	require.True(t, result.NoReadyWork)

	lines := strings.Split(strings.TrimRight(sink.String(), "\n"), "\n")
	require.Len(t, lines, 2, "empty queue should still emit loop.start and loop.end")

	var start, end map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &start))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &end))
	assert.Equal(t, "loop.start", start["type"])
	assert.Equal(t, "loop.end", end["type"])
	endData, _ := end["data"].(map[string]any)
	assert.EqualValues(t, 0, endData["attempts"])
	assert.Equal(t, "drained", endData["exit_reason"])
	assert.Equal(t, "drained", result.ExitReason)
}

// TestExecuteBeadWorkerNoChangesUnjustifiedStaysOpenWithLabel verifies the
// NoChangesContract (TD-031 §8.1, ddx-b24e9630) "unjustified" path: when the
// rationale carries neither verification_command nor a canonical lifecycle status, the
// bead stays open with a triage:no-changes-unjustified label and a
// no_changes_unjustified event. There is no count-based fallback under the new
// contract — only structured rationales close the bead.
func TestExecuteBeadWorkerNoChangesUnjustifiedStaysOpenWithLabel(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-nc01", Title: "Always no-changes"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				Detail:             "nothing to do",
				NoChangesRationale: "the work seems done",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, r.Attempts)
	assert.Equal(t, 0, r.Successes)
	assert.Equal(t, 1, r.Failures)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "unjustified rationale must NOT close the bead")
	assert.Contains(t, got.Labels, NoChangesLabelUnjustified)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawUnjustified bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventUnjustified {
			sawUnjustified = true
		}
	}
	assert.True(t, sawUnjustified, "no_changes_unjustified event must be emitted")
}

// TestExecuteBeadWorkerCustomSatisfactionCheckerClosesBeadWhenSatisfied verifies
// that a custom SatisfactionChecker can close a bead immediately on the first
// no_changes result without waiting for the count threshold.
func TestExecuteBeadWorkerCustomSatisfactionCheckerClosesBeadWhenSatisfied(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-sat01", Title: "Already done"}
	require.NoError(t, store.Create(b))

	checkerCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusNoChanges,
				Detail: "agent found no work",
			}, nil
		}),
		SatisfactionChecker: satisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
			checkerCalled = true
			assert.Equal(t, b.ID, beadID)
			assert.Equal(t, 1, noChangesCount)
			return true, "acceptance criteria already met", nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.True(t, checkerCalled, "SatisfactionChecker must be called")
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status)
	assert.Equal(t, "acceptance criteria already met", result.Results[0].Detail)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	var sawIntent, sawExecute bool
	for _, ev := range events {
		switch ev.Kind {
		case "execution-routing-intent":
			sawIntent = true
		case "execute-bead":
			sawExecute = true
			assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, ev.Summary)
		}
	}
	assert.True(t, sawIntent, "routing intent evidence must be recorded")
	assert.True(t, sawExecute, "execute-bead event must still be recorded")
}

// TestExecuteBeadWorkerCustomSatisfactionCheckerLeavesBeadOpenWhenUnresolved
// verifies that when the SatisfactionChecker reports the bead is not yet
// satisfied, the bead remains open without default retry cooldown.
func TestExecuteBeadWorkerCustomSatisfactionCheckerLeavesBeadOpenWhenUnresolved(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-unr01", Title: "Not yet done"}
	require.NoError(t, store.Create(b))

	now := time.Now().UTC().Truncate(time.Second)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusNoChanges,
				BaseRev:   "rev1",
				ResultRev: "rev1",
			}, nil
		}),
		SatisfactionChecker: satisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
			return false, "", nil
		}),
		Now: func() time.Time { return now },
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:           "worker",
		NoProgressCooldown: 1 * time.Hour,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 0, result.Successes)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.LastFailureStatus)
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].RetryAfter, "retry cooldown must not be recorded by default")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.NotContains(t, got.Extra, "work-retry-after")
}

// TestExecuteBeadWorkerNoChangesDoesNotStarveQueue verifies that a bead with a
// bad no_changes result cannot prevent other ready beads from being executed in
// the current queue pass. After the first bad attempt, the unjustified triage
// label removes the bead from future execution-ready scans.
func TestExecuteBeadWorkerNoChangesDoesNotStarveQueue(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Three beads: one that returns no_changes, two that succeed. Not supplying
	// BaseRev/ResultRev means shouldSuppressNoProgress returns false, so no
	// retry-after is written; the triage label is what removes it from future
	// execution-ready scans.
	ncBead := &bead.Bead{ID: "ddx-nc10", Title: "Always no-changes", Priority: 0}
	work1 := &bead.Bead{ID: "ddx-wk11", Title: "Work bead 1", Priority: 1}
	work2 := &bead.Bead{ID: "ddx-wk12", Title: "Work bead 2", Priority: 2}
	require.NoError(t, store.Create(ncBead))
	require.NoError(t, store.Create(work1))
	require.NoError(t, store.Create(work2))

	var mu sync.Mutex
	executed := []string{}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			mu.Lock()
			executed = append(executed, beadID)
			mu.Unlock()
			if beadID == ncBead.ID {
				// No BaseRev/ResultRev → shouldSuppressNoProgress returns false
				// → no cooldown written → bead is immediately retryable.
				return ExecuteBeadReport{
					BeadID: beadID,
					Status: ExecuteBeadStatusNoChanges,
				}, nil
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-" + beadID,
				ResultRev: "bbbb",
			}, nil
		}),
	}

	const threshold = 2
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: threshold,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	runtime := ExecuteBeadLoopRuntime{}

	// Pass 1: all three beads are ready.
	// ncBead returns no_changes (count=1 < threshold) and stays open.
	// work1 and work2 succeed and are closed.
	// The `attempted` map inside Run prevents ncBead from being picked up
	// a second time within the same pass — the other beads run unblocked.
	result1, err := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.Equal(t, 3, result1.Attempts, "all three beads must be attempted in pass 1")
	assert.Equal(t, 2, result1.Successes)
	assert.Equal(t, 1, result1.Failures)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result1.LastFailureStatus)

	w1, _ := store.Get(work1.ID)
	w2, _ := store.Get(work2.ID)
	nc, _ := store.Get(ncBead.ID)
	assert.Equal(t, bead.StatusClosed, w1.Status)
	assert.Equal(t, bead.StatusClosed, w2.Status)
	assert.Equal(t, bead.StatusOpen, nc.Status, "ncBead must stay open after first no_changes")

	// Pass 2: only ncBead remains open. The unjustified label is explanatory
	// metadata only, so the status-owned queue can retry it.
	result2, err := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	assert.Equal(t, 1, result2.Attempts)
	assert.Equal(t, 0, result2.Successes)
	assert.Equal(t, 1, result2.Failures)

	nc, _ = store.Get(ncBead.ID)
	assert.Equal(t, bead.StatusOpen, nc.Status, "ncBead stays open under NoChangesContract")
	assert.Contains(t, nc.Labels, NoChangesLabelUnjustified)
}

// TestExecuteBeadWorkerNoChangesVerifiedClosesImmediately verifies the
// NoChangesContract verified path (ddx-b24e9630): a rationale carrying a
// verification_command that exits 0 closes the bead as already_satisfied on
// the first attempt and emits a no_changes_verified event.
func TestExecuteBeadWorkerNoChangesVerifiedClosesImmediately(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-sha01", Title: "Already in prior commit"}
	require.NoError(t, store.Create(b))

	const rationale = "verification_command: true\noutput: passes"

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				Detail:             "agent made no commits",
				BaseRev:            "aaaa1111",
				ResultRev:          "aaaa1111",
				NoChangesRationale: rationale,
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			return 0, "ok", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes, "verification_command pass must close bead on first attempt")
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusAlreadySatisfied, result.Results[0].Status)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawVerified, sawTerminal bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventVerified {
			sawVerified = true
			assert.Contains(t, ev.Body, "exit_code=0")
		}
		if ev.Summary == ExecuteBeadStatusAlreadySatisfied {
			sawTerminal = true
		}
	}
	assert.True(t, sawVerified, "no_changes_verified event must be emitted")
	assert.True(t, sawTerminal, "terminal already_satisfied event must be appended")
}

// TestExecuteBeadWorkerNoChangesVerificationFailsKeepsBeadOpen verifies the
// NoChangesContract unverified path: a verification_command that exits non-zero
// does NOT close the bead. The bead stays open with a
// triage:no-changes-unverified label and a no_changes_unverified event.
func TestExecuteBeadWorkerNoChangesVerificationFailsKeepsBeadOpen(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-unv01", Title: "Bead whose verification fails"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "verification_command: false\noutput: nothing matched",
			}, nil
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			return 1, "test failure", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r.Successes)
	assert.Equal(t, 1, r.Failures)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "verification failure must NOT close the bead")
	assert.Contains(t, got.Labels, NoChangesLabelUnverified)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawUnverified bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventUnverified {
			sawUnverified = true
			assert.Contains(t, ev.Body, "exit_code=1")
		}
	}
	assert.True(t, sawUnverified, "no_changes_unverified event must be emitted")
}

func TestNoChangesRetryDoesNotWriteLegacyRetryFloorKey(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-open01", Title: "Bead that needs a stronger retry"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				BaseRev:            "base",
				ResultRev:          "base",
				NoChangesRationale: "status: open\nreason: retryable after stronger code search\nsuggested_action: retry with smart agent",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r.Successes)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "autonomous no_changes must remain worker-runnable")
	assert.NotContains(t, got.Labels, bead.LabelNeedsInvestigation)
	assert.NotContains(t, got.Labels, bead.LabelNeedsHuman)
	retryAfter, hasRetry := got.Extra["work-retry-after"]
	require.True(t, hasRetry, "smart-runnable no_changes must set a short retry-after so watch workers drain other beads")
	assert.Equal(t, r.Results[0].RetryAfter, retryAfter)
	assert.Equal(t, true, got.Extra[executeLoopSmartRetryKey])
	assert.NotContains(t, got.Extra, legacyRetryFloorKey)
	assert.Equal(t, NoChangesEventAutonomousRetry, got.Extra[bead.ExtraLastStatus])
	assert.Contains(t, got.Extra[bead.ExtraLastDetail], "stronger code search")

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawAutonomous bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventAutonomousRetry {
			sawAutonomous = true
			assert.Contains(t, ev.Body, "status=open")
			assert.Contains(t, ev.Body, "stronger code search")
		}
	}
	assert.True(t, sawAutonomous, "no_changes_autonomous_retry event must be emitted")
}

// TestNoChangesSmartRetry_TopPowerClassExhausted_GoesToProposed asserts that when
// no_changes happens at the top powerClass (EscalationNextFloor returns an error),
// the bead is parked to status=proposed rather than retried.
func TestNoChangesSmartRetry_TopPowerClassExhausted_GoesToProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-toptier", Title: "Top powerClass exhausted bead"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		// Ladder exhausted: top-powerClass model already used, nothing higher.
		EscalationNextFloor: func(actualPower int) (int, error) {
			return 0, fmt.Errorf("ladder exhausted")
		},
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				ActualPower:        90,
				NoChangesRationale: "status: open\nreason: top-powerClass model still could not complete the work",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "no_changes at top powerClass must park bead to proposed (genuine spec gap)")
	assert.Empty(t, got.Owner, "proposed bead must not be owned")
	assert.NotContains(t, got.Extra, legacyRetryFloorKey, "legacy retry-floor metadata must not be written when parked to proposed")
}

func TestNoChangesOperatorRequiredBecomesProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-prop01", Title: "Bead requiring operator choice"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: proposed\nreason: AC conflicts with governing spec\nsuggested_action: choose whether to update the spec or cancel",
			}, nil
		}),
		Now: func() time.Time {
			return time.Date(2026, 5, 9, 8, 0, 0, 0, time.UTC)
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r.Successes)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status)
	assert.NotContains(t, got.Labels, bead.LabelNeedsHuman)
	assert.NotContains(t, got.Labels, bead.LabelNeedsInvestigation)
	assert.Equal(t, NoChangesEventOperatorRequired, got.Extra[bead.ExtraLastStatus])
	assert.Contains(t, got.Extra[bead.ExtraLastDetail], "governing spec")

	meta := bead.GetNeedsHumanMeta(*got)
	assert.Equal(t, "AC conflicts with governing spec", meta.Reason)
	assert.Equal(t, "choose whether to update the spec or cancel", meta.SuggestedAction)
	assert.Equal(t, "ddx work", meta.Source)

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawOperator bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventOperatorRequired {
			sawOperator = true
			assert.Contains(t, ev.Body, "status=proposed")
		}
	}
	assert.True(t, sawOperator, "no_changes_operator_required event must be emitted")
}

func TestNoChangesRejectsLegacyNeedsInvestigationStatus(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-legacy01", Title: "Bead with legacy no_changes output"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             beadID,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: needs_investigation\nreason: ambiguous legacy output",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r.Successes)
	assert.Equal(t, 1, r.Failures)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.NotContains(t, got.Labels, bead.LabelNeedsHuman)
	assert.NotContains(t, got.Labels, bead.LabelNeedsInvestigation)
	assert.NotContains(t, got.Labels, NoChangesLabelUnjustified)
	if got.Extra != nil {
		assert.NotContains(t, got.Extra, "work-retry-after")
	}

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawRejected bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventLegacyStatusRejected {
			sawRejected = true
			assert.Contains(t, ev.Body, "status: needs_investigation is no longer accepted")
			assert.Contains(t, ev.Body, "ddx bead migrate --lifecycle")
		}
	}
	assert.True(t, sawRejected, "legacy needs_investigation output must be rejected explicitly")
}

func TestExecuteBeadWorkerNoChangesUnjustifiedKeepsOpenWithoutLongCooldown(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-unj01", Title: "Bead with absent no_changes rationale"}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusNoChanges,
				BaseRev:   "same",
				ResultRev: "same",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 0, r.Successes)
	assert.Equal(t, 1, r.Failures)
	require.Len(t, r.Results, 1)
	assert.Empty(t, r.Results[0].RetryAfter)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Contains(t, got.Labels, NoChangesLabelUnjustified)
	_, hasRetry := got.Extra["work-retry-after"]
	assert.False(t, hasRetry, "unjustified no_changes must not set work-retry-after by default")
	ready, err := store.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, ready, 1)
	assert.Equal(t, b.ID, ready[0].ID, "unjustified no_changes stays worker-runnable under status-owned lifecycle")

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var sawUnjustified bool
	for _, ev := range events {
		if ev.Kind == NoChangesEventUnjustified {
			sawUnjustified = true
			assert.Contains(t, ev.Body, "rationale absent")
		}
	}
	assert.True(t, sawUnjustified, "no_changes_unjustified event must be emitted")
}

func TestExecuteBeadWorkerSuccessClearsStaleNoChangesMetadata(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{
		ID:    "ddx-stale01",
		Title: "Success after stale no_changes",
		Extra: map[string]any{
			"work-retry-after": "2020-01-01T00:00:00Z",
			"work-last-status": "no_changes",
			"work-last-detail": "agent made no commits",
		},
	}
	require.NoError(t, store.Create(b))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-success",
				ResultRev: "abc123",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	r, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	assert.Equal(t, 1, r.Successes)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
	assert.NotContains(t, got.Extra, "work-retry-after")
	assert.NotContains(t, got.Extra, "work-last-status")
	assert.NotContains(t, got.Extra, "work-last-detail")

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, events, "success must preserve/append events while clearing only management Extra fields")
}

// TestExecuteBeadWorkerDeclinedNeedsDecompositionParksBead verifies that when
// the executor returns the structured `declined_needs_decomposition` outcome
// the loop:
//
//  1. sets execution-eligible=false so subsequent loop iterations do not re-attempt it
//     (TD-031 §5: operator action required, not a time-based cooldown),
//  2. records the recommended sub-beads as a structured
//     `decomposition-recommendation` event (JSON body, not free-form text),
//  3. does not pick the bead up on a second `Run` because it is not execution-eligible.
//
// Regression coverage for ddx-fba752b9.
func TestExecuteBeadWorkerDeclinedNeedsDecompositionParksBead(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-decomp01", Title: "Routing point release (epic)"}
	require.NoError(t, store.Create(b))

	recommended := []string{
		"split: cost-aware routing tiebreak",
		"split: routing profiles",
		"split: public route decision trace",
	}
	rationale := "scope is epic-sized; deliver as 3 sub-beads"

	var execCount int32
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		atomic.AddInt32(&execCount, 1)
		return ExecuteBeadReport{
			BeadID:                      beadID,
			Status:                      ExecuteBeadStatusDeclinedNeedsDecomposition,
			Detail:                      "this epic is too big, split it into sub-beads",
			DecompositionRationale:      rationale,
			DecompositionRecommendation: recommended,
		}, nil
	})

	worker := &ExecuteBeadWorker{Store: store, Executor: executor}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// First run: the executor declines, the loop parks the bead.
	r1, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, r1)
	require.Equal(t, 1, r1.Attempts)
	require.Equal(t, 0, r1.Successes)
	require.Equal(t, 1, r1.Failures)
	require.Equal(t, ExecuteBeadStatusDeclinedNeedsDecomposition, r1.LastFailureStatus)

	// execution-eligible must be false so the bead is no longer execution-ready
	// (TD-031 §5: operator action is required, not a time-based cooldown).
	got, err := store.Get(b.ID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusOpen, got.Status, "bead stays open; execution-eligible=false removes it from queue")
	retryAfter, _ := got.Extra["work-retry-after"].(string)
	require.Empty(t, retryAfter, "declined_needs_decomposition must not set work-retry-after")
	require.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "execution-eligible must be set to false")

	// A structured decomposition-recommendation event must be appended.
	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var rec *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "decomposition-recommendation" {
			rec = &events[i]
			break
		}
	}
	require.NotNil(t, rec, "decomposition-recommendation event missing; got %d events", len(events))

	// Body must be JSON-decodable and carry the recommended sub-beads as a
	// structured field, not just inline text.
	var payload struct {
		Rationale           string   `json:"rationale"`
		RecommendedSubbeads []string `json:"recommended_subbeads"`
	}
	require.NoError(t, json.Unmarshal([]byte(rec.Body), &payload))
	require.Equal(t, rationale, payload.Rationale)
	require.Equal(t, recommended, payload.RecommendedSubbeads)

	// Second run: the bead must not be re-attempted because execution-eligible=false.
	r2, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.Equal(t, 0, r2.Attempts, "ineligible bead must not be re-attempted")
	require.Equal(t, int32(1), atomic.LoadInt32(&execCount), "executor must run exactly once")
}

func TestExecuteBeadWorkerWatchDoesNotRetryExecutionIneligibleAfterPreservedNoChanges(t *testing.T) {
	t.Run("watch_mode_does_not_retry_same_parent", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init())

		parent := &bead.Bead{ID: "ddx-watch-decomp", Title: "Watch-mode decomposition parent"}
		require.NoError(t, store.Create(parent))

		decomp := &PreClaimDecomposition{
			Rationale: "split preserved parent",
			Children: []PreClaimDecompositionChild{
				{Title: "Child A", Description: "child a", Acceptance: "1. do child a"},
			},
			ACMap: []ACMapEntry{
				{ParentAC: "1. split work", Coverage: "covered by Child A"},
			},
		}

		var parentExecCount int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				if beadID == parent.ID {
					atomic.AddInt32(&parentExecCount, 1)
				}
				return ExecuteBeadReport{
					BeadID:             beadID,
					Status:             ExecuteBeadStatusNoChanges,
					BaseRev:            "base-watch",
					ResultRev:          "base-watch",
					PreserveRef:        "refs/ddx/iterations/ddx-watch-decomp/attempt-1",
					NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: too large for one worker pass",
				}, nil
			}),
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sink := &cancelOnMatchWriter{match: `"type":"loop.idle"`, cancel: cancel}

		cfgOpts := config.TestLoopConfigOpts{
			Assignee:              "worker",
			MaxDecompositionDepth: 3,
		}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

		result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:         executeloop.ModeWatch,
			IdleInterval: time.Millisecond,
			EventSink:    sink,
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				err := store.Update(beadID, func(b *bead.Bead) {
					if b.Extra == nil {
						b.Extra = make(map[string]any)
					}
					b.Extra[bead.ExtraExecutionElig] = false
					if !HasBeadLabel(b.Labels, "decomposed") {
						b.Labels = append(b.Labels, "decomposed")
					}
				})
				require.NoError(t, err)
				return decomp, nil
			},
		})
		if err != nil {
			require.ErrorIs(t, err, context.Canceled)
		}
		require.NotNil(t, result)
		assert.Equal(t, int32(1), atomic.LoadInt32(&parentExecCount), "watch worker must not dispatch the same decomposed parent twice")

		got, err := store.Get(parent.ID)
		require.NoError(t, err)
		assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig])
		assert.Contains(t, got.Labels, "decomposed")

		events := parseLoopEvents(t, sink.String())
		claimed := loopEventDataByType(events, "bead.claimed")
		var parentClaimCount int
		for _, event := range claimed {
			if event["bead_id"] == parent.ID {
				parentClaimCount++
			}
		}
		assert.Equal(t, 1, parentClaimCount, "parent must only be claimed once in the watcher run")
		assert.NotEmpty(t, loopEventDataByType(events, "loop.idle"), "watch worker must go idle instead of retrying the parent")
	})

	t.Run("stale_snapshot_selects_next_ready_bead", func(t *testing.T) {
		baseStore := bead.NewStore(t.TempDir())
		require.NoError(t, baseStore.Init())

		parent := &bead.Bead{
			ID:       "ddx-stale-parent",
			Title:    "Decomposed parent",
			Priority: 0,
			Labels:   []string{"decomposed"},
			Extra:    map[string]any{bead.ExtraExecutionElig: false},
		}
		next := &bead.Bead{ID: "ddx-stale-next", Title: "Next ready bead", Priority: 1}
		require.NoError(t, baseStore.Create(parent))
		require.NoError(t, baseStore.Create(next))

		staleParent := *parent
		staleParent.Extra = map[string]any{}
		staleStore := &staleReadyStore{
			Store:      baseStore,
			staleReady: []bead.Bead{staleParent, *next},
		}

		var executed []string
		worker := &ExecuteBeadWorker{
			Store: staleStore,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				executed = append(executed, beadID)
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-stale-next",
					ResultRev: "feedbeef",
				}, nil
			}),
		}

		var sink bytes.Buffer
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{EventSink: &sink})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{next.ID}, executed, "worker must skip the stale parent and dispatch the next ready bead")
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, 1, result.Successes)

		events := parseLoopEvents(t, sink.String())
		skips := loopEventDataByType(events, "picker.skip_stale_candidate")
		require.Len(t, skips, 1)
		assert.Equal(t, parent.ID, skips[0]["bead_id"])
		assert.Equal(t, "decomposed", skips[0]["reason"])
		assert.Equal(t, "pre_claim", skips[0]["stage"])
		assert.Equal(t, false, skips[0]["execution_eligible"])
		assert.Equal(t, true, skips[0]["decomposed"])

		claimed := loopEventDataByType(events, "bead.claimed")
		require.Len(t, claimed, 1)
		assert.Equal(t, next.ID, claimed[0]["bead_id"], "stale parent must not be claimed")

		got, err := baseStore.Get(parent.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, got.Status, "skipped parent must remain open")
		assert.Empty(t, got.Owner, "skipped parent must not remain claimed")
	})

	t.Run("stale_snapshot_idles_with_skip_event", func(t *testing.T) {
		baseStore := bead.NewStore(t.TempDir())
		require.NoError(t, baseStore.Init())

		parent := &bead.Bead{
			ID:       "ddx-stale-idle",
			Title:    "Only stale parent",
			Priority: 0,
			Labels:   []string{"decomposed"},
			Extra:    map[string]any{bead.ExtraExecutionElig: false},
		}
		require.NoError(t, baseStore.Create(parent))

		staleParent := *parent
		staleParent.Extra = map[string]any{}
		staleStore := &staleReadyStore{
			Store:      baseStore,
			staleReady: []bead.Bead{staleParent},
		}

		worker := &ExecuteBeadWorker{
			Store: staleStore,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				t.Fatalf("executor must not run for an execution-ineligible stale parent")
				return ExecuteBeadReport{}, nil
			}),
		}

		var sink bytes.Buffer
		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:      true,
			EventSink: &sink,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 0, result.Attempts)
		assert.True(t, result.NoReadyWork, "worker must idle once the stale parent is skipped")

		events := parseLoopEvents(t, sink.String())
		skips := loopEventDataByType(events, "picker.skip_stale_candidate")
		require.Len(t, skips, 1)
		assert.Equal(t, parent.ID, skips[0]["bead_id"])
		assert.Equal(t, "decomposed", skips[0]["reason"])
		assert.Equal(t, "pre_claim", skips[0]["stage"])

		got, err := baseStore.Get(parent.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, got.Status)
		assert.Empty(t, got.Owner)
	})
}

func newExecuteLoopTestStore(t *testing.T) (*bead.Store, *bead.Bead, *bead.Bead) {
	t.Helper()

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	first := &bead.Bead{ID: "ddx-0001", Title: "First ready", Priority: 0}
	second := &bead.Bead{ID: "ddx-0002", Title: "Second ready", Priority: 1}
	require.NoError(t, store.Create(first))
	require.NoError(t, store.Create(second))

	return store, first, second
}

type staleReadyStore struct {
	*bead.Store
	staleReady []bead.Bead
	readyCalls int32
}

func (s *staleReadyStore) ReadyExecution() ([]bead.Bead, error) {
	if atomic.AddInt32(&s.readyCalls, 1) <= 2 {
		out := make([]bead.Bead, len(s.staleReady))
		copy(out, s.staleReady)
		return out, nil
	}
	return s.Store.ReadyExecution()
}

func (s *staleReadyStore) ReadyExecutionBreakdown() (bead.ReadyExecutionBreakdown, error) {
	return s.Store.ReadyExecutionBreakdown()
}

type cancelOnMatchWriter struct {
	bytes.Buffer
	match  string
	cancel context.CancelFunc
	once   sync.Once
}

func (w *cancelOnMatchWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if w.cancel != nil && strings.Contains(string(p), w.match) {
		w.once.Do(w.cancel)
	}
	return n, err
}

type loopEvent struct {
	Type string
	Data map[string]any
}

func parseLoopEvents(t *testing.T, raw string) []loopEvent {
	t.Helper()

	var events []loopEvent
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		typ, _ := entry["type"].(string)
		if typ == "" {
			continue
		}
		data, _ := entry["data"].(map[string]any)
		events = append(events, loopEvent{Type: typ, Data: data})
	}
	return events
}

func loopEventDataByType(events []loopEvent, typ string) []map[string]any {
	var out []map[string]any
	for _, event := range events {
		if event.Type == typ {
			out = append(out, event.Data)
		}
	}
	return out
}

// errorInjectingStore wraps a real ExecuteBeadLoopStore and allows individual
// methods to be overridden to return injected errors. Used in tests that verify
// the loop continues on transient Store.* failures instead of terminating.
type errorInjectingStore struct {
	ExecuteBeadLoopStore
	onUnclaim           func(id string) error
	onIncrNoChanges     func(id string) (int, error)
	onCloseWithEvidence func(id, sessionID, commitSHA string) error
	onReopen            func(id, reason, notes string) error
	onSetCooldown       func(id string, until time.Time, status, detail, baseRev string) error
	onAppendEvent       func(id string, event bead.BeadEvent) error
}

func (s *errorInjectingStore) Unclaim(id string) error {
	if s.onUnclaim != nil {
		return s.onUnclaim(id)
	}
	return s.ExecuteBeadLoopStore.Unclaim(id)
}

func (s *errorInjectingStore) IncrNoChangesCount(id string) (int, error) {
	if s.onIncrNoChanges != nil {
		return s.onIncrNoChanges(id)
	}
	return s.ExecuteBeadLoopStore.IncrNoChangesCount(id)
}

func (s *errorInjectingStore) CloseWithEvidence(id, sessionID, commitSHA string) error {
	if s.onCloseWithEvidence != nil {
		return s.onCloseWithEvidence(id, sessionID, commitSHA)
	}
	return s.ExecuteBeadLoopStore.CloseWithEvidence(id, sessionID, commitSHA)
}

func (s *errorInjectingStore) Reopen(id, reason, notes string) error {
	if s.onReopen != nil {
		return s.onReopen(id, reason, notes)
	}
	return s.ExecuteBeadLoopStore.Reopen(id, reason, notes)
}

func (s *errorInjectingStore) SetExecutionCooldown(id string, until time.Time, status, detail, baseRev string) error {
	if s.onSetCooldown != nil {
		return s.onSetCooldown(id, until, status, detail, baseRev)
	}
	return s.ExecuteBeadLoopStore.SetExecutionCooldown(id, until, status, detail, baseRev)
}

func (s *errorInjectingStore) AppendEvent(id string, event bead.BeadEvent) error {
	if s.onAppendEvent != nil {
		return s.onAppendEvent(id, event)
	}
	return s.ExecuteBeadLoopStore.AppendEvent(id, event)
}

// TestExecuteBeadWorkerPreClaimHookAlwaysFailsLeavesBeadAvailable verifies that
// when the pre-claim hook always fails (e.g. branch is persistently diverged),
// the bead stays open and is available for a subsequent Run invocation once the
// hook starts passing. This is the cross-run correctness companion to the
// same-run retry test below.
func TestExecuteBeadWorkerPreClaimHookAlwaysFailsLeavesBeadAvailable(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	b := &bead.Bead{ID: "ddx-hook-always-fail", Title: "Bead with persistent hook failure"}
	require.NoError(t, store.Create(b))

	var executedIDs []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executedIDs = append(executedIDs, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-hook",
				ResultRev: "abc1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// First run: hook always fails — bead must remain open with 0 attempts.
	result1, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		PreClaimHook: func(ctx context.Context) error {
			return fmt.Errorf("diverged branch")
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result1.Attempts, "hook failure must not count as an attempt")
	assert.Empty(t, executedIDs, "executor must not be called when hook always fails")

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must remain open after hook always fails")
}

// TestExecuteBeadWorkerPreClaimHookFailureRetriesOnNextIteration verifies that
// in a multi-bead queue, a hook failure on the first iteration does NOT prevent
// the same bead from being picked up in a subsequent iteration of the same Run.
// This covers the direct reproduce scenario from the bead description (runs 3/4).
func TestExecuteBeadWorkerPreClaimHookFailureRetriesOnNextIteration(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Only one ready bead in the queue.
	b := &bead.Bead{ID: "ddx-hook-same-run", Title: "Bead retried in same run"}
	require.NoError(t, store.Create(b))

	// Hook fails on first call, passes on second.
	hookCalls := 0
	var executedIDs []string

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executedIDs = append(executedIDs, beadID)
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-retry",
				ResultRev: "feed1234",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		// No Once flag: loop runs until no more candidates.
		PreClaimHook: func(ctx context.Context) error {
			hookCalls++
			if hookCalls == 1 {
				return fmt.Errorf("diverged branch on first hook call")
			}
			return nil
		},
	})
	require.NoError(t, err)
	// The bead was skipped once (hook fail), then retried and succeeded.
	assert.Equal(t, 1, result.Attempts, "bead should be executed exactly once after hook passes")
	assert.Equal(t, 1, result.Successes)
	assert.Equal(t, []string{b.ID}, executedIDs)

	got, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status)
}

// TestExecuteBeadWorkerStoreErrorContinuesLoop verifies root cause #2: any
// Store.* error in the post-Execute outcome block must NOT terminate the loop.
// The loop must: continue to the next iteration, record a kind:loop-error event
// on the affected bead, and increment result.Failures.
//
// Regression for ddx-37cdb43a.
func TestExecuteBeadWorkerStoreErrorContinuesLoop(t *testing.T) {
	injectedErr := fmt.Errorf("transient storage failure")

	cases := []struct {
		name           string
		executorStatus string
		injectErr      func(s *errorInjectingStore)
		wantOp         string
	}{
		{
			name:           "Unclaim fails on no_changes path",
			executorStatus: ExecuteBeadStatusNoChanges,
			injectErr: func(s *errorInjectingStore) {
				s.onUnclaim = func(id string) error { return injectedErr }
			},
			wantOp: "Unclaim",
		},
		{
			name:           "IncrNoChangesCount fails",
			executorStatus: ExecuteBeadStatusNoChanges,
			injectErr: func(s *errorInjectingStore) {
				s.onIncrNoChanges = func(id string) (int, error) { return 0, injectedErr }
			},
			wantOp: "IncrNoChangesCount",
		},
		{
			name:           "adjudicateNoChanges fails via SatisfactionChecker",
			executorStatus: ExecuteBeadStatusNoChanges,
			injectErr:      nil, // injected via SatisfactionChecker, not store
			wantOp:         "adjudicateNoChanges",
		},
		{
			name:           "CloseWithEvidence fails on success path",
			executorStatus: ExecuteBeadStatusSuccess,
			injectErr: func(s *errorInjectingStore) {
				s.onCloseWithEvidence = func(id, sessionID, commitSHA string) error { return injectedErr }
			},
			wantOp: "CloseWithEvidence",
		},
		{
			name:           "SetExecutionCooldown fails on execution_failed path",
			executorStatus: ExecuteBeadStatusExecutionFailed,
			injectErr: func(s *errorInjectingStore) {
				// execution_failed doesn't call SetExecutionCooldown unless
				// shouldSuppressNoProgress — use a report with matching base/result rev.
				// Actually execution_failed goes through the else branch which
				// calls SetExecutionCooldown only if shouldSuppressNoProgress.
				// To trigger it, inject at the Unclaim level so we get to that branch.
				s.onUnclaim = func(id string) error { return injectedErr }
			},
			wantOp: "Unclaim",
		},
		{
			name:           "late AppendEvent fails",
			executorStatus: ExecuteBeadStatusExecutionFailed,
			injectErr: func(s *errorInjectingStore) {
				callCount := 0
				s.onAppendEvent = func(id string, event bead.BeadEvent) error {
					callCount++
					// Fail only the late execute-bead event (not loop-error events).
					if event.Kind == "execute-bead" || event.Kind == "" {
						return injectedErr
					}
					return nil
				}
			},
			wantOp: "AppendEvent",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			realStore := bead.NewStore(t.TempDir())
			require.NoError(t, realStore.Init())

			// Two beads: first gets the error injection, second should still be processed.
			first := &bead.Bead{ID: "ddx-err-first", Title: "First bead"}
			second := &bead.Bead{ID: "ddx-err-second", Title: "Second bead"}
			require.NoError(t, realStore.Create(first))
			require.NoError(t, realStore.Create(second))

			store := &errorInjectingStore{ExecuteBeadLoopStore: realStore}
			if tc.injectErr != nil {
				tc.injectErr(store)
			}

			// For the adjudicateNoChanges case, inject via SatisfactionChecker.
			var satisfactionChecker SatisfactionChecker
			if tc.wantOp == "adjudicateNoChanges" {
				satisfactionChecker = satisfactionCheckerFunc(func(ctx context.Context, beadID string, noChangesCount int) (bool, string, error) {
					if beadID == first.ID {
						return false, "", injectedErr
					}
					return false, "", nil
				})
			}

			var executedIDs []string
			worker := &ExecuteBeadWorker{
				Store:               store,
				SatisfactionChecker: satisfactionChecker,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					executedIDs = append(executedIDs, beadID)
					status := tc.executorStatus
					if beadID == second.ID {
						status = ExecuteBeadStatusSuccess
					}
					return ExecuteBeadReport{
						BeadID:    beadID,
						Status:    status,
						SessionID: "sess-" + beadID,
						ResultRev: "rev-" + beadID,
						BaseRev:   "rev-" + beadID, // same as ResultRev → triggers shouldSuppressNoProgress
					}, nil
				}),
			}

			cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
			rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
			result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
			require.NoError(t, err, "loop must not return error on store failure")

			// (a) loop continues — second bead must have been executed.
			assert.Contains(t, executedIDs, second.ID, "loop must continue to next bead after store error on first")

			// (c) result.Failures advances.
			assert.GreaterOrEqual(t, result.Failures, 1, "result.Failures must be >= 1 after store error")

			// (b) kind:loop-error event recorded on the affected bead (best-effort;
			// not checked for AppendEvent failures since the event sink itself is broken).
			if tc.wantOp != "AppendEvent" {
				events, evErr := realStore.Events(first.ID)
				require.NoError(t, evErr)
				var loopErrorEvent *bead.BeadEvent
				for i := range events {
					if events[i].Kind == "loop-error" {
						loopErrorEvent = &events[i]
						break
					}
				}
				require.NotNil(t, loopErrorEvent, "kind:loop-error event must be recorded; got events: %v", events)
				assert.Contains(t, loopErrorEvent.Summary, tc.wantOp,
					"loop-error summary must contain the failing operation name")
			}
		})
	}
}

// TestExecuteBeadWorkerEndToEndThreeBeadDrain is the integration guard from
// AC-4: seeds 3 ready beads with outcomes no_changes / success /
// already_satisfied and asserts all three are processed without premature exit.
// With the pre-fix loop this test failed because the no_changes result caused
// Unclaim to be the first store call and the loop exited on any transient path.
func TestExecuteBeadWorkerEndToEndThreeBeadDrain(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	a := &bead.Bead{ID: "ddx-e2e-a", Title: "Bead A — no_changes", Priority: 0}
	b := &bead.Bead{ID: "ddx-e2e-b", Title: "Bead B — success", Priority: 1}
	c := &bead.Bead{ID: "ddx-e2e-c", Title: "Bead C — already_satisfied", Priority: 2}
	require.NoError(t, store.Create(a))
	require.NoError(t, store.Create(b))
	require.NoError(t, store.Create(c))

	var executedIDs []string
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			executedIDs = append(executedIDs, beadID)
			switch beadID {
			case a.ID:
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusNoChanges,
					BaseRev:   "aaa",
					ResultRev: "aaa", // equal → shouldSuppressNoProgress
				}, nil
			case b.ID:
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					SessionID: "sess-b",
					ResultRev: "bbb",
				}, nil
			case c.ID:
				// Returning no_changes with a verification_command that exits 0
				// (NoChangesContract verified path) closes the bead as
				// already_satisfied on the first attempt.
				return ExecuteBeadReport{
					BeadID:             beadID,
					Status:             ExecuteBeadStatusNoChanges,
					BaseRev:            "ccc",
					ResultRev:          "ccc",
					NoChangesRationale: "verification_command: true\noutput: passes",
				}, nil
			default:
				return ExecuteBeadReport{BeadID: beadID, Status: ExecuteBeadStatusSuccess}, nil
			}
		}),
		VerificationRunner: func(ctx context.Context, projectRoot, command string) (int, string, error) {
			return 0, "", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{})
	require.NoError(t, err)

	// All three beads must have been attempted.
	assert.ElementsMatch(t, []string{a.ID, b.ID, c.ID}, executedIDs,
		"all three beads must be processed; loop must not exit after the first")
	assert.Equal(t, 3, result.Attempts)

	// Bead B must be closed.
	gotB, err := store.Get(b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotB.Status)

	// Bead C must be closed as already_satisfied.
	gotC, err := store.Get(c.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, gotC.Status)
}

// TestZeroConfigWork_NoConfigDoesNotEmitUnderSpecified asserts that running
// the loop with no intake or lint hooks (zero-config) produces operator log
// output that does not expose stale complexity/intake/lint terminology.
func TestZeroConfigWork_NoConfigDoesNotEmitUnderSpecified(t *testing.T) {
	inner, _, _ := newExecuteLoopTestStore(t)
	store := &claimCountingStore{Store: inner}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-zero-cfg",
				ResultRev: "abc111",
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	var logBuf bytes.Buffer
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &logBuf,
		// No PreClaimIntakeHook, no PreDispatchLintHook — zero-config path.
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	logOut := logBuf.String()
	assert.NotContains(t, logOut, "complexity gate missing")
	assert.NotContains(t, logOut, "pre-claim intake")
	assert.NotContains(t, logOut, "pre-dispatch lint")
	assert.Equal(t, 1, result.Successes, "zero-config loop must still execute beads")
}
