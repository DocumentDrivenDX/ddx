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
	// ProvenPreservedProjectLocalState is true when the allocation path has
	// explicit evidence that project-local state was preserved and can safely
	// back the savings estimate. When false, savings remain conservative zero.
	ProvenPreservedProjectLocalState bool
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
// projection: it preserves the counts and only exposes conservative savings
// when the outcome proves preserved project-local state and reuse.
func AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
	outcome AttemptWorkspaceReuseAllocationOutcome,
) AttemptWorkspaceReuseTelemetryInput {
	return AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(outcome)
}

// AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome converts one
// reusable-slot allocation outcome into conservative telemetry values.
//
// Non-zero savings are only exposed when:
//  1. ProvenPreservedProjectLocalState is true (explicit proof flag),
//  2. SlotHitCount > 0 (reuse occurred), and
//  3. both conservative savings fields are positive.
//
// Cold allocations and reused-slot outcomes without proof stay zeroed.
func AttemptWorkspaceReuseSavingsEstimateFromReusableSlotOutcome(
	outcome AttemptWorkspaceReuseAllocationOutcome,
) AttemptWorkspaceReuseTelemetryInput {
	telemetry := AttemptWorkspaceReuseTelemetryInput{
		SlotHitCount:  outcome.SlotHitCount,
		SlotMissCount: outcome.SlotMissCount,
	}
	if !outcome.ProvenPreservedProjectLocalState {
		return telemetry
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
