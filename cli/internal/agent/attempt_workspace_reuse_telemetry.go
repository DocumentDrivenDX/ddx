package agent

import "time"

// combinedReusableWorkspaceTelemetry is the combined event payload used to
// report reusable workspace allocation counts and savings in one record.
// The zero values are intentionally preserved so cold-start attempts still
// emit explicit miss and zero-savings fields.
type combinedReusableWorkspaceTelemetry struct {
	SlotHitCount  int   `json:"slot_hit_count"`
	SlotMissCount int   `json:"slot_miss_count"`
	TimeSaved     int64 `json:"time_saved"`
	BytesSaved    int64 `json:"bytes_saved"`
}

// appendCombinedReusableWorkspaceTelemetryEvent records the reusable-workspace
// telemetry event on the bead stream. Callers supply the allocation/savings
// snapshot; this helper keeps the event shape stable across reused and
// cold-start attempts.
func appendCombinedReusableWorkspaceTelemetryEvent(
	appender BeadEventAppender,
	beadID string,
	telemetry combinedReusableWorkspaceTelemetry,
	actor string,
	createdAt time.Time,
) {
	if appender == nil || beadID == "" {
		return
	}

	appendWorkEvent(appender, beadID, "attempt-workspace-reuse-telemetry", "combined reusable workspace telemetry", map[string]any{
		"slot_hit_count":  telemetry.SlotHitCount,
		"slot_miss_count": telemetry.SlotMissCount,
		"time_saved":      telemetry.TimeSaved,
		"bytes_saved":     telemetry.BytesSaved,
	}, actor, createdAt)
}
