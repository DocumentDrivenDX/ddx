package agent

// AttemptWorkspaceReuseAllocationOutcome captures the slot-pool outcome that
// feeds reusable-workspace telemetry. Hit and miss counts stay explicit so the
// combined payload can represent both reused attempts and cold starts with the
// same JSON shape. Conservative savings estimates travel on the same contract
// so callers do not need a parallel structure to preserve them.
type AttemptWorkspaceReuseAllocationOutcome struct {
	SlotHitCount            int
	SlotMissCount           int
	ConservativeTimeSavedMS int64
	ConservativeBytesSaved  int64
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
// allocation outcome into the combined telemetry input. The function is a pure
// projection: it preserves the counts and conservative savings estimates
// already attached to the outcome and does not recompute them.
func AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
	outcome AttemptWorkspaceReuseAllocationOutcome,
) AttemptWorkspaceReuseTelemetryInput {
	return AttemptWorkspaceReuseTelemetryInput{
		SlotHitCount:  outcome.SlotHitCount,
		SlotMissCount: outcome.SlotMissCount,
		TimeSavedMS:   outcome.ConservativeTimeSavedMS,
		BytesSaved:    outcome.ConservativeBytesSaved,
	}
}
