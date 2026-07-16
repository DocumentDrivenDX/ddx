package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandBoundaryOutcome struct {
	name            string
	finalEventJSON  string
	rationale       string
	wantOutcome     string
	wantStatus      string
	wantFailureMode string
	wantFinalReason string
}

func TestTryAndWorkWrappersPreserveTypedNoEvidenceProduced(t *testing.T) {
	testTryAndWorkCommandBoundaryOutcome(t, commandBoundaryOutcome{
		name:            "no_evidence_produced",
		finalEventJSON:  `{"status":"success","final_text":"finished without evidence"}`,
		wantOutcome:     agent.ExecuteBeadOutcomeTaskNoEvidence,
		wantStatus:      agent.ExecuteBeadStatusNoEvidenceProduced,
		wantFailureMode: agent.FailureModeNoEvidenceProduced,
	})
}

func TestTryAndWorkWrappersPreserveTypedNoChanges(t *testing.T) {
	testTryAndWorkCommandBoundaryOutcome(t, commandBoundaryOutcome{
		name:           "no_changes",
		finalEventJSON: `{"status":"success","final_text":"no implementation required"}`,
		rationale: "status: open\n" +
			"reason: command-boundary fixture intentionally makes no implementation commit\n" +
			"suggested_action: retry with smart agent\n",
		wantOutcome:     agent.ExecuteBeadOutcomeTaskNoChanges,
		wantStatus:      agent.ExecuteBeadStatusNoChanges,
		wantFailureMode: agent.FailureModeNoChanges,
	})
}

func TestTryAndWorkWrappersPreserveFailedFizeauFinal(t *testing.T) {
	testTryAndWorkCommandBoundaryOutcome(t, commandBoundaryOutcome{
		name:            "failed_fizeau_final",
		finalEventJSON:  `{"status":"error","exit_code":1,"error":"session closed before Stop hook"}`,
		wantOutcome:     agent.ExecuteBeadOutcomeTaskFailed,
		wantStatus:      agent.ExecuteBeadStatusExecutionFailed,
		wantFailureMode: agent.FailureModeUnknown,
		wantFinalReason: "transport",
	})
}

func testTryAndWorkCommandBoundaryOutcome(t *testing.T, outcome commandBoundaryOutcome) {
	t.Helper()
	for _, entryPoint := range []string{"try", "work"} {
		t.Run(entryPoint, func(t *testing.T) {
			executeFn := func(req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
				if outcome.rationale != "" {
					attemptID := req.Metadata["attempt_id"]
					require.NotEmpty(t, attemptID)
					bundleDir := filepath.Join(req.WorkDir, ddxroot.DirName, "executions", attemptID)
					require.NoError(t, os.MkdirAll(bundleDir, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "no_changes_rationale.txt"), []byte(outcome.rationale), 0o644))
				}
				ch := make(chan agentlib.ServiceEvent, 1)
				ch <- agentlib.ServiceEvent{Type: "final", Data: []byte(outcome.finalEventJSON)}
				close(ch)
				return ch, nil
			}

			result := runProductionRoutingEntryPoint(
				t,
				entryPoint,
				nil,
				[]string{"--harness", "command-boundary-test"},
				&productionRoutingHookRunner{},
				executeFn,
			)
			require.Len(t, result.requests, 1, "output=%q err=%v", result.output, result.err)

			workerResult := readOnlyCommandAttemptResult(t, result.projectRoot)
			assert.Equal(t, outcome.wantOutcome, workerResult.Outcome)
			assert.Equal(t, outcome.wantStatus, workerResult.Status)
			assert.Equal(t, outcome.wantFailureMode, workerResult.FailureMode)

			finalEvent := lastCommandExecuteBeadEvent(t, result.events)
			assert.Equal(t, outcome.wantStatus, finalEvent.Summary)
			wantFinalReason := outcome.wantFinalReason
			if wantFinalReason == "" {
				wantFinalReason = outcome.wantFailureMode
			}
			assert.Contains(t, finalEvent.Body, "outcome_reason="+wantFinalReason)
			if entryPoint == "try" {
				assert.Contains(t, result.output, "status: "+outcome.wantStatus)
			}
		})
	}
}

func readOnlyCommandAttemptResult(t *testing.T, projectRoot string) agent.ExecuteBeadResult {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(projectRoot, ddxroot.DirName, "executions", "*", "result.json"))
	require.NoError(t, err)
	require.Len(t, paths, 1)
	data, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	var result agent.ExecuteBeadResult
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func lastCommandExecuteBeadEvent(t *testing.T, events []bead.BeadEvent) bead.BeadEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "execute-bead" {
			return events[i]
		}
	}
	t.Fatal("execute-bead event not found")
	return bead.BeadEvent{}
}
