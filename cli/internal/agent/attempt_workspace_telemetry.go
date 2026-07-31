package agent

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// AttemptWorkspaceReuseSavingsContract carries the savings values computed by
// the reusable-workspace allocation/execution outcome path.
//
// The telemetry builder treats these as already-derived inputs and only
// serializes them into the final event body.
type AttemptWorkspaceReuseSavingsContract struct {
	TimeSavedMS int64 `json:"time_saved_ms"`
	BytesSaved  int64 `json:"bytes_saved"`
}

// AttemptWorkspaceReuseTelemetryPayload is the combined attempt-workspace
// telemetry record emitted for reusable workspace outcomes.
//
// The payload keeps the allocation counts and savings values together in one
// JSON object so downstream consumers do not need to correlate separate events.
type AttemptWorkspaceReuseTelemetryPayload struct {
	SlotHitCount  int64 `json:"slot_hit_count"`
	SlotMissCount int64 `json:"slot_miss_count"`
	AttemptWorkspaceReuseSavingsContract
}

// AttemptWorkspaceReuseTelemetryEvent builds the canonical bead event for a
// reusable workspace outcome.
//
// The function is deliberately a thin serializer: it does not recompute time or
// bytes saved, it only packages the supplied payload into the event envelope.
func AttemptWorkspaceReuseTelemetryEvent(payload AttemptWorkspaceReuseTelemetryPayload) bead.BeadEvent {
	body, err := json.Marshal(payload)
	if err != nil {
		return bead.BeadEvent{}
	}

	summary := fmt.Sprintf(
		"hits=%d misses=%d time_saved_ms=%d bytes_saved=%d",
		payload.SlotHitCount,
		payload.SlotMissCount,
		payload.TimeSavedMS,
		payload.BytesSaved,
	)

	return bead.BeadEvent{
		Kind:    "attempt-workspace-reuse",
		Summary: summary,
		Body:    string(body),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	}
}
