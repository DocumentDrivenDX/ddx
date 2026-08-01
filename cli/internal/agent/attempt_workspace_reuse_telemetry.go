package agent

// AttemptWorkspaceReuseAllocationOutcome captures the slot-pool outcome that
// feeds reusable-workspace telemetry. Hit and miss counts stay explicit so the
// combined payload can represent both reused attempts and cold starts with the
// same JSON shape.
type AttemptWorkspaceReuseAllocationOutcome struct {
	SlotHitCount  int
	SlotMissCount int
}

// AttemptWorkspaceReuseSavings carries the savings contract consumed by the
// reusable-workspace telemetry path.
type AttemptWorkspaceReuseSavings struct {
	TimeSavedMS int64
	BytesSaved  int64
}

// AttemptWorkspaceReuseTelemetryInput is the combined reusable-workspace
// telemetry payload. It preserves the same shape for reused attempts and cold
// starts: counts identify the allocation outcome and savings are zeroed when
// nothing was reused.
type AttemptWorkspaceReuseTelemetryInput struct {
	SlotHitCount  int   `json:"slot_hit_count"`
	SlotMissCount int   `json:"slot_miss_count"`
	TimeSavedMS   int64 `json:"time_saved_ms"`
	BytesSaved    int64 `json:"bytes_saved"`
}

// AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome converts the
// allocation outcome plus savings contract into the combined telemetry input.
// Callers provide the observed hit/miss counts and the savings estimate; cold
// starts should pass zero savings and an expected miss count.
func AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
	outcome AttemptWorkspaceReuseAllocationOutcome,
	savings AttemptWorkspaceReuseSavings,
) AttemptWorkspaceReuseTelemetryInput {
	return AttemptWorkspaceReuseTelemetryInput{
		SlotHitCount:  outcome.SlotHitCount,
		SlotMissCount: outcome.SlotMissCount,
		TimeSavedMS:   savings.TimeSavedMS,
		BytesSaved:    savings.BytesSaved,
	}
}
