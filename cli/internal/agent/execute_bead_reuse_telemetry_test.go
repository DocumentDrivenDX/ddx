package agent

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryRecordsHitsMissesAndSavings(t *testing.T) {
	app := &stubBeadEventAppender{}

	appendReusableWorkspaceTelemetry(app, "ddx-reuse", reusableWorkspaceTelemetryBody{
		SlotHitCount:  1,
		SlotMissCount: 0,
		TimeSavedMS:   1250,
		BytesSaved:    4096,
	})
	appendReusableWorkspaceTelemetry(app, "ddx-cold", reusableWorkspaceTelemetryBody{
		SlotHitCount:  0,
		SlotMissCount: 1,
		TimeSavedMS:   0,
		BytesSaved:    0,
	})

	require.Len(t, app.events, 2)
	require.Equal(t, "reusable-workspace", app.events[0].Event.Kind)
	require.Equal(t, "reusable-workspace", app.events[1].Event.Kind)

	reusedBody := decodeTelemetryBody(t, app.events[0].Event.Body)
	coldBody := decodeTelemetryBody(t, app.events[1].Event.Body)

	require.ElementsMatch(t, telemetryFieldNames(reusedBody), telemetryFieldNames(coldBody))
	require.ElementsMatch(t, []string{
		"bytes_saved",
		"slot_hit_count",
		"slot_miss_count",
		"time_saved_ms",
	}, telemetryFieldNames(reusedBody))

	require.Equal(t, float64(1), reusedBody["slot_hit_count"])
	require.Equal(t, float64(0), reusedBody["slot_miss_count"])
	require.Equal(t, float64(1250), reusedBody["time_saved_ms"])
	require.Equal(t, float64(4096), reusedBody["bytes_saved"])

	require.Equal(t, float64(0), coldBody["slot_hit_count"])
	require.Equal(t, float64(1), coldBody["slot_miss_count"])
	require.Equal(t, float64(0), coldBody["time_saved_ms"])
	require.Equal(t, float64(0), coldBody["bytes_saved"])
}

func decodeTelemetryBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	return parsed
}

func telemetryFieldNames(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
