package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkRetryEscalation_NiflheimEvidence_OnlySemanticFailuresRaiseMinPower(t *testing.T) {
	t.Run("semantic smart retry raises the next min power", func(t *testing.T) {
		out, err := noChangesAttempt(t, "status: open\nreason: rerun with a stronger model\nsuggested_action: retry with smart agent")
		require.NoError(t, err)
		require.NotNil(t, out.NoChanges)
		assert.Equal(t, agenttry.NoChangesActionKeepOpenSmartRetry, out.NoChanges.Action)

		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))
		require.NoError(t, store.Create(context.Background(), &bead.Bead{
			ID:    "ddx-niflheim-semantic-retry",
			Title: "Niflheim semantic retry",
		}))

		require.NoError(t, applyNoChangesSmartRetry(store, "ddx-niflheim-semantic-retry", "worker", out.NoChanges, out.Report.ActualPower, func(actualPower int) (int, error) {
			return actualPower + 1, nil
		}))

		updated, err := store.Get(context.Background(), "ddx-niflheim-semantic-retry")
		require.NoError(t, err)
		require.Equal(t, bead.StatusOpen, updated.Status)
		assert.Equal(t, out.Report.ActualPower+1, noChangesMinPowerOverride(updated, out.Report.ActualPower))
		assert.EqualValues(t, out.Report.ActualPower+1, updated.Extra[executeLoopNoChangesNextMinPowerKey])
		assert.Equal(t, true, updated.Extra[executeLoopSmartRetryKey])
		assert.Equal(t, "rerun with a stronger model", updated.Extra[executeLoopSmartRetryReasonKey])
		assert.Equal(t, "retry with smart agent", updated.Extra[executeLoopSmartRetrySuggestedActionKey])
	})

	t.Run("preclaim system_unready warnings do not bump the power ladder", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))
		beadIDs := []string{
			"ddx-niflheim-preclaim-1",
			"ddx-niflheim-preclaim-2",
		}
		for i, beadID := range beadIDs {
			require.NoError(t, store.Create(context.Background(), &bead.Bead{
				ID:       beadID,
				Title:    "niflheim preclaim warning " + beadID,
				Priority: i,
			}))
		}

		var floorCalls atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:    beadID,
					Status:    ExecuteBeadStatusSuccess,
					ResultRev: "rev-" + beadID,
				}, nil
			}),
			EscalationNextFloor: func(actualPower int) (int, error) {
				floorCalls.Add(1)
				return actualPower + 1, nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		detail := "readiness check timed out after 45s (.ddx/harness-sessions/session-timeout.json)"
		result, err := worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
			Mode:                        executeloop.ModeDrain,
			NoReview:                    true,
			PreClaimWarnRepeatThreshold: 2,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome:      PreClaimIntakeError,
					Reason:       "system_unready",
					SystemReason: "readiness_timeout",
					Detail:       detail,
				}, nil
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, 1, result.Successes)
		assert.Equal(t, 0, result.Failures)
		require.NotNil(t, result.OperatorAttention)
		assert.Equal(t, "preclaim_warn_repeated", result.OperatorAttention.Reason)
		assert.Equal(t, beadIDs[1], result.OperatorAttention.BeadID)
		assert.Equal(t, "operator_attention", result.ExitReason)
		assert.Equal(t, "OperatorAttention", result.StopCondition)
		assert.Zero(t, floorCalls.Load(), "pre-claim system warnings must not request stronger power")

		gotFirst, err := store.Get(context.Background(), beadIDs[0])
		require.NoError(t, err)
		gotSecond, err := store.Get(context.Background(), beadIDs[1])
		require.NoError(t, err)
		assert.Equal(t, bead.StatusClosed, gotFirst.Status)
		assert.Equal(t, bead.StatusOpen, gotSecond.Status)
		assert.NotContains(t, gotFirst.Extra, executeLoopNoChangesNextMinPowerKey)
		assert.NotContains(t, gotSecond.Extra, executeLoopNoChangesNextMinPowerKey)

		lastEvents, err := store.Events(beadIDs[1])
		require.NoError(t, err)
		var operatorAttentionEvent *bead.BeadEvent
		for i := range lastEvents {
			if lastEvents[i].Kind == "operator_attention" && lastEvents[i].Summary == "preclaim_warn_repeated" {
				operatorAttentionEvent = &lastEvents[i]
				break
			}
		}
		require.NotNil(t, operatorAttentionEvent, "a durable operator_attention event must be recorded")
		assert.Contains(t, operatorAttentionEvent.Body, "preclaim_warn_repeated")
		assert.Contains(t, operatorAttentionEvent.Body, detail)
	})

	t.Run("land coordination with staged generated ddx evidence does not bump power", func(t *testing.T) {
		store, first, _ := newExecuteLoopTestStore(t)
		var floorCalls atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return landCoordinationNoPowerBumpReport(t, beadID, errors.New(
					"landing worktree has staged changes after waiting 2s:\n"+
						"M\t.ddx/beads.jsonl\n"+
						"M\t.ddx/executions/20260608T010203-retry/result.json",
				)), nil
			}),
			EscalationNextFloor: func(actualPower int) (int, error) {
				floorCalls.Add(1)
				return actualPower + 10, nil
			},
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Results, 1)

		got := result.Results[0]
		assert.Equal(t, ExecuteBeadStatusLandRetry, got.Status)
		assert.Equal(t, FailureModeLandRetry, got.OutcomeReason)
		assert.Equal(t, "standard", got.RequestedProfile)
		assert.Equal(t, "standard", got.InferredPowerClass)
		assert.Equal(t, "standard", got.PowerClass)
		assert.Zero(t, got.EscalationCount, "land coordination must not request stronger model escalation")
		assert.Zero(t, floorCalls.Load(), "land coordination must not request stronger power")

		gotBead, err := store.Get(context.Background(), first.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, gotBead.Status)
		assert.NotContains(t, gotBead.Extra, executeLoopNoChangesNextMinPowerKey)

		events, err := store.Events(first.ID)
		require.NoError(t, err)
		var executeEvent *bead.BeadEvent
		for i := range events {
			if events[i].Kind == "execute-bead" {
				executeEvent = &events[i]
				break
			}
		}
		require.NotNil(t, executeEvent, "execute-bead event must be recorded")
		assert.Contains(t, executeEvent.Body, ".ddx/beads.jsonl")
		assert.Contains(t, executeEvent.Body, ".ddx/executions/20260608T010203-retry/result.json")

		audit := decisionAuditFromEventBody(t, executeEvent.Body)
		assert.Equal(t, "retry_land", audit["retry_action"])
		assert.Equal(t, float64(0), audit["escalation_count"])
		assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "profile"))
		assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "inferred_power_class"))
		assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "requested_power_class"))
	})

	t.Run("decomposed parents do not request stronger power", func(t *testing.T) {
		store := bead.NewStore(t.TempDir())
		require.NoError(t, store.Init(context.Background()))

		parent := &bead.Bead{ID: "ddx-niflheim-retry-parent", Title: "Niflheim decomposed parent retry"}
		require.NoError(t, store.Create(context.Background(), parent))

		var floorCalls atomic.Int32
		worker := &ExecuteBeadWorker{
			Store: store,
			Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
				return ExecuteBeadReport{
					BeadID:             beadID,
					Status:             ExecuteBeadStatusExecutionFailed,
					Detail:             mixedCommitAndNoChangesRationaleReason,
					BaseRev:            "base-retry",
					ResultRev:          "result-retry",
					ActualPower:        50,
					RequestedProfile:   "standard",
					NoChangesRationale: "status: open\norchestrator_action: decompose\nreason: parent already split into child beads",
				}, nil
			}),
			EscalationNextFloor: func(actualPower int) (int, error) {
				floorCalls.Add(1)
				return actualPower + 10, nil
			},
		}

		cfgOpts := config.TestLoopConfigOpts{Assignee: "worker", MaxDecompositionDepth: 3}
		rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: parent.ID,
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				return niflheimDecomposedParentFixture(), nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Zero(t, floorCalls.Load(), "decomposed parents must not request stronger power")
		require.Len(t, result.Results, 1)

		got := result.Results[0]
		assert.Equal(t, "decomposed", got.OutcomeReason)
		assert.Equal(t, "execution_ineligible", got.ExecutionDecision)
		require.NotEmpty(t, got.CycleTrace)
		assert.Equal(t, "operator_attention", got.CycleTrace[0].RetryAction)

		gotBead, err := store.Get(context.Background(), parent.ID)
		require.NoError(t, err)
		assert.Equal(t, bead.StatusOpen, gotBead.Status)
		assert.NotContains(t, gotBead.Extra, executeLoopNoChangesNextMinPowerKey)

		events, err := store.Events(parent.ID)
		require.NoError(t, err)
		var executeEvent *bead.BeadEvent
		for i := range events {
			if events[i].Kind == "execute-bead" {
				executeEvent = &events[i]
				break
			}
		}
		require.NotNil(t, executeEvent, "execute-bead event must be recorded")
		assert.Contains(t, executeEvent.Body, "orchestrator_action: decompose")

		audit := decisionAuditFromEventBody(t, executeEvent.Body)
		assert.Equal(t, "decomposed", audit["failure_class"])
		assert.Equal(t, "operator_attention", audit["retry_action"])
		assert.Equal(t, float64(0), audit["escalation_count"])
		assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "profile"))
		assert.Equal(t, "execution_ineligible", audit["execution_decision"])
	})
}
