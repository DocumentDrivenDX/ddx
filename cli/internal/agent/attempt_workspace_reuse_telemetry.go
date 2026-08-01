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
	return AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(outcome)
}

// AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome converts one
// reusable-slot allocation outcome into conservative telemetry values.
//
// Non-zero savings are only exposed when the outcome already proves reuse via
// a hit count and carries explicit positive savings metadata. Cold allocations
// and reused-slot outcomes without savings proof stay zeroed.
func AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(
	outcome AttemptWorkspaceReuseAllocationOutcome,
) AttemptWorkspaceReuseTelemetryInput {
	telemetry := AttemptWorkspaceReuseTelemetryInput{
		SlotHitCount:  outcome.SlotHitCount,
		SlotMissCount: outcome.SlotMissCount,
	}
	if outcome.SlotHitCount <= 0 {
		return telemetry
	}
	if outcome.ConservativeTimeSavedMS <= 0 || outcome.ConservativeBytesSaved <= 0 {
		return telemetry
	}
	return AttemptWorkspaceReuseTelemetryInput{
		SlotHitCount:  outcome.SlotHitCount,
		SlotMissCount: outcome.SlotMissCount,
		TimeSavedMS:   outcome.ConservativeTimeSavedMS,
		BytesSaved:    outcome.ConservativeBytesSaved,
	}
}
