package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryRecordsHitsMissesAndSavings(t *testing.T) {
	cases := []struct {
		name string
		in   combinedReusableWorkspaceTelemetry
		want map[string]any
	}{
		{
			name: "reused_attempt",
			in: combinedReusableWorkspaceTelemetry{
				SlotHitCount:  1,
				SlotMissCount: 0,
				TimeSaved:     1500,
				BytesSaved:    4096,
			},
			want: map[string]any{
				"slot_hit_count":  float64(1),
				"slot_miss_count": float64(0),
				"time_saved":      float64(1500),
				"bytes_saved":     float64(4096),
			},
		},
		{
			name: "cold_start_attempt",
			in: combinedReusableWorkspaceTelemetry{
				SlotHitCount:  0,
				SlotMissCount: 1,
				TimeSaved:     0,
				BytesSaved:    0,
			},
			want: map[string]any{
				"slot_hit_count":  float64(0),
				"slot_miss_count": float64(1),
				"time_saved":      float64(0),
				"bytes_saved":     float64(0),
			},
		},
	}

	expectedKeys := []string{"slot_hit_count", "slot_miss_count", "time_saved", "bytes_saved"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &stubBeadEventAppender{}
			appendCombinedReusableWorkspaceTelemetryEvent(app, "ddx-reuse-telemetry", tc.in, "ddx", time.Unix(0, 0).UTC())

			require.Len(t, app.events, 1)
			got := app.events[0]
			require.Equal(t, "attempt-workspace-reuse-telemetry", got.Event.Kind)
			require.Equal(t, "combined reusable workspace telemetry", got.Event.Summary)

			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(got.Event.Body), &body))
			require.Contains(t, body, "slot_hit_count")
			require.Contains(t, body, "slot_miss_count")
			require.Contains(t, body, "time_saved")
			require.Contains(t, body, "bytes_saved")
			require.Equal(t, tc.want, body)

			keys := make([]string, 0, len(body))
			for k := range body {
				keys = append(keys, k)
			}
			require.ElementsMatch(t, expectedKeys, keys)
		})
	}
}
