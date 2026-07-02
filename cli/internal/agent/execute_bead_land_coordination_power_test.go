package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkRetryEscalation_NiflheimEvidence_LandCoordinationDoesNotRaiseMinPower(t *testing.T) {
	tests := []struct {
		name              string
		landErr           error
		wantStatus        string
		wantOutcomeReason string
		wantRetryAction   string
		wantBeadStatus    string
	}{
		{
			name: "staged generated evidence retries land without power bump",
			landErr: errors.New(
				"landing worktree has staged changes after waiting 2s:\n" +
					"M\t.ddx/beads.jsonl\n" +
					"M\t.ddx/executions/20260608T010203-retry/result.json",
			),
			wantStatus:        ExecuteBeadStatusLandRetry,
			wantOutcomeReason: FailureModeLandRetry,
			wantRetryAction:   "retry_land",
			wantBeadStatus:    bead.StatusOpen,
		},
		{
			name: "staged source work parks operator attention without power bump",
			landErr: errors.New(
				"landing worktree has staged changes after waiting 2s:\n" +
					"M\t.ddx/beads.jsonl\n" +
					"M\tcli/internal/agent/foo.go",
			),
			wantStatus:        ExecuteBeadStatusLandOperatorAttention,
			wantOutcomeReason: FailureModeLandOperatorAttention,
			wantRetryAction:   "operator_attention",
			wantBeadStatus:    bead.StatusProposed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, first, _ := newExecuteLoopTestStore(t)
			var calls atomic.Int32
			var escalationCalls atomic.Int32
			worker := &ExecuteBeadWorker{
				Store: store,
				Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
					calls.Add(1)
					report := landCoordinationNoPowerBumpReport(t, beadID, tt.landErr)
					return report, nil
				}),
				EscalationNextFloor: func(actualPower int) (int, error) {
					escalationCalls.Add(1)
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
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantOutcomeReason, got.OutcomeReason)
			assert.Equal(t, "standard", got.RequestedProfile)
			assert.Equal(t, "standard", got.InferredPowerClass)
			assert.Equal(t, "standard", got.PowerClass)
			assert.Zero(t, got.EscalationCount, "land coordination must not request stronger model escalation")

			assert.Equal(t, int32(1), calls.Load(), "land coordination must not rerun implementation")
			assert.Zero(t, escalationCalls.Load(), "land coordination must not request a stronger model")

			gotBead, err := store.Get(context.Background(), first.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBeadStatus, gotBead.Status)

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
			audit := decisionAuditFromEventBody(t, executeEvent.Body)
			assert.Equal(t, tt.wantRetryAction, audit["retry_action"])
			assert.Equal(t, float64(0), audit["escalation_count"])
			assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "profile"))
			assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "inferred_power_class"))
			assert.Equal(t, "standard", nestedString(t, audit, "requested_route", "requested_power_class"))
		})
	}
}

func landCoordinationNoPowerBumpReport(t *testing.T, beadID string, landErr error) ExecuteBeadReport {
	t.Helper()

	res := &ExecuteBeadResult{
		BeadID:    beadID,
		AttemptID: "20260608T010203-power",
		BaseRev:   "base-power",
		ResultRev: "base-power",
		ExitCode:  0,
		Outcome:   ExecuteBeadOutcomeTaskSucceeded,
		SessionID: "sess-power",
	}
	MarkResultLandError(t.TempDir(), res, landErr)

	report := ReportFromExecuteBeadResult(res, "standard")
	report.RequestedProfile = "standard"
	report.InferredPowerClass = "standard"
	report.PowerClass = "standard"
	return report
}
