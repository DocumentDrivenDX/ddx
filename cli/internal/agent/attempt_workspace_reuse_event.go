package agent

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const (
	AttemptWorkspaceReuseOutcomeHit       = "hit"
	AttemptWorkspaceReuseOutcomeMiss      = "miss"
	AttemptWorkspaceReuseOutcomeColdStart = "cold_start"
)

// AttemptWorkspaceReuseTelemetryEventContract is the execution-observability
// contract for reusable workspace allocation outcomes. It keeps the durable
// attempt, project, worker-slot, and backend identity alongside the reuse
// outcome so downstream consumers can read the event body directly.
type AttemptWorkspaceReuseTelemetryEventContract struct {
	Outcome               string `json:"outcome"`
	ProjectRoot           string `json:"project_root"`
	WorkerSlot            string `json:"worker_slot"`
	ResolvedBackendPolicy string `json:"resolved_backend_policy"`
	AttemptID             string `json:"attempt_id"`
	SlotHitCount          int    `json:"slot_hit_count"`
	SlotMissCount         int    `json:"slot_miss_count"`
	TimeSavedMS           int64  `json:"time_saved_ms"`
	BytesSaved            int64  `json:"bytes_saved"`
}

// AttemptWorkspaceReuseExecutionEvent builds the canonical bead event for the
// reusable-workspace observability path.
func AttemptWorkspaceReuseExecutionEvent(contract AttemptWorkspaceReuseTelemetryEventContract) bead.BeadEvent {
	body, err := json.Marshal(contract)
	if err != nil {
		return bead.BeadEvent{}
	}

	summary := fmt.Sprintf(
		"outcome=%s slot_hit_count=%d slot_miss_count=%d time_saved_ms=%d bytes_saved=%d",
		contract.Outcome,
		contract.SlotHitCount,
		contract.SlotMissCount,
		contract.TimeSavedMS,
		contract.BytesSaved,
	)

	return bead.BeadEvent{
		Kind:    "reusable-workspace",
		Summary: summary,
		Body:    string(body),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	}
}

// appendAttemptWorkspaceReuseTelemetry records the reusable-workspace
// observability event on the bead. It is best-effort and ignores empty or
// invalid payloads so telemetry failures never abort attempt execution.
func appendAttemptWorkspaceReuseTelemetry(appender BeadEventAppender, beadID string, contract AttemptWorkspaceReuseTelemetryEventContract) {
	if appender == nil || beadID == "" {
		return
	}
	_ = appender.AppendEvent(beadID, AttemptWorkspaceReuseExecutionEvent(contract))
}

func reusableWorkspaceTelemetryOutcome(ws *AttemptWorkspace) string {
	if ws == nil || ws.ReusableSlot == nil {
		return AttemptWorkspaceReuseOutcomeColdStart
	}
	slot := ws.ReusableSlot
	if !slot.Pooled {
		return AttemptWorkspaceReuseOutcomeColdStart
	}
	if slot.SlotMissCount > 0 && slot.SlotHitCount == 0 {
		return AttemptWorkspaceReuseOutcomeMiss
	}
	return AttemptWorkspaceReuseOutcomeHit
}

func reusableWorkspaceTelemetryEventContract(
	ws *AttemptWorkspace,
	telemetry *ReusableWorkspaceTelemetry,
	projectRoot, workerSlot, resolvedBackendPolicy, attemptID string,
) AttemptWorkspaceReuseTelemetryEventContract {
	contract := AttemptWorkspaceReuseTelemetryEventContract{
		Outcome:               reusableWorkspaceTelemetryOutcome(ws),
		ProjectRoot:           projectRoot,
		WorkerSlot:            workerSlot,
		ResolvedBackendPolicy: resolvedBackendPolicy,
		AttemptID:             attemptID,
	}
	if telemetry == nil {
		return contract
	}
	contract.SlotHitCount = telemetry.SlotHitCount
	contract.SlotMissCount = telemetry.SlotMissCount
	contract.TimeSavedMS = telemetry.TimeSavedMS
	contract.BytesSaved = telemetry.BytesSaved
	return contract
}
