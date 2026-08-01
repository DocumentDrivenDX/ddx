package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/require"
)

// reusableWorkspaceTelemetryBody deliberately keeps every counter field
// present so reused and cold-start attempts emit the same JSON key set even
// when the cold-start values are zero.
type reusableWorkspaceTelemetryBodyLegacy struct {
	SlotHitCount  int   `json:"slot_hit_count"`
	SlotMissCount int   `json:"slot_miss_count"`
	TimeSavedMS   int   `json:"time_saved_ms"`
	BytesSaved    int64 `json:"bytes_saved"`
}

func TestAttemptWorkspaceReuseTelemetryRecordsHitsMissesAndSavingsPayload(t *testing.T) {
	app := &stubBeadEventAppender{}

	appendReusableWorkspaceTelemetryForTest(app, "ddx-reuse", reusableWorkspaceTelemetryBodyLegacy{
		SlotHitCount:  1,
		SlotMissCount: 0,
		TimeSavedMS:   1250,
		BytesSaved:    4096,
	})
	appendReusableWorkspaceTelemetryForTest(app, "ddx-cold", reusableWorkspaceTelemetryBodyLegacy{
		SlotHitCount:  0,
		SlotMissCount: 1,
		TimeSavedMS:   0,
		BytesSaved:    0,
	})

	require.Len(t, app.events, 2)
	require.Equal(t, "reusable-workspace", app.events[0].Event.Kind)
	require.Equal(t, "reusable-workspace", app.events[1].Event.Kind)

	reusedBody := decodeTelemetryBodyLegacy(t, app.events[0].Event.Body)
	coldBody := decodeTelemetryBodyLegacy(t, app.events[1].Event.Body)

	require.ElementsMatch(t, telemetryFieldNamesLegacy(reusedBody), telemetryFieldNamesLegacy(coldBody))
	require.ElementsMatch(t, []string{
		"bytes_saved",
		"slot_hit_count",
		"slot_miss_count",
		"time_saved_ms",
	}, telemetryFieldNamesLegacy(reusedBody))

	require.Equal(t, float64(1), reusedBody["slot_hit_count"])
	require.Equal(t, float64(0), reusedBody["slot_miss_count"])
	require.Equal(t, float64(1250), reusedBody["time_saved_ms"])
	require.Equal(t, float64(4096), reusedBody["bytes_saved"])

	require.Equal(t, float64(0), coldBody["slot_hit_count"])
	require.Equal(t, float64(1), coldBody["slot_miss_count"])
	require.Equal(t, float64(0), coldBody["time_saved_ms"])
	require.Equal(t, float64(0), coldBody["bytes_saved"])
}

// appendReusableWorkspaceTelemetryLegacy records reusable-workspace savings evidence
// on the bead. The body shape is intentionally stable across reused and
// cold-start attempts, so zero-valued cold-start counters stay visible.
func appendReusableWorkspaceTelemetryForTest(appender BeadEventAppender, beadID string, body reusableWorkspaceTelemetryBodyLegacy) {
	if appender == nil || beadID == "" {
		return
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := fmt.Sprintf(
		"slot_hit_count=%d slot_miss_count=%d time_saved_ms=%d bytes_saved=%d",
		body.SlotHitCount, body.SlotMissCount, body.TimeSavedMS, body.BytesSaved,
	)
	_ = appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "reusable-workspace",
		Summary: summary,
		Body:    string(data),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	})
}

func decodeTelemetryBodyLegacy(t *testing.T, body string) map[string]any {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	return parsed
}

func telemetryFieldNamesLegacy(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
