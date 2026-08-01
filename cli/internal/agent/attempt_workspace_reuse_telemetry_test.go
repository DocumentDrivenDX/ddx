package agent

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttemptWorkspaceReuseTelemetryInputCarriesReusedAllocationSavings(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1834,
			BytesSaved:  987654321,
		},
	)

	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.Equal(t, int64(1834), got.TimeSavedMS)
	require.Equal(t, int64(987654321), got.BytesSaved)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)
}

func TestAttemptWorkspaceReuseTelemetryReusedAttemptCarriesAllocationSavings(t *testing.T) {
	allocationOutcome := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  1,
			SlotMissCount: 0,
		},
		AttemptWorkspaceReuseSavings{
			TimeSavedMS: 1834,
			BytesSaved:  987654321,
		},
	)

	got := reusableWorkspaceTelemetryForWorkspace(
		&AttemptWorkspace{
			ReusableSlot: &AttemptWorkspaceSlot{
				SlotHitCount:            1,
				SlotMissCount:           0,
				ConservativeTimeSavedMS: 0,
				ConservativeBytesSaved:  0,
			},
		},
		&ReusableWorkspaceTelemetry{
			TimeSavedMS: allocationOutcome.TimeSavedMS,
			BytesSaved:  allocationOutcome.BytesSaved,
		},
	)

	require.NotNil(t, got)
	require.Equal(t, 1, got.SlotHitCount)
	require.Equal(t, 0, got.SlotMissCount)
	require.Equal(t, int64(1834), got.TimeSavedMS)
	require.Equal(t, int64(987654321), got.BytesSaved)

	res := &ExecuteBeadResult{
		BeadID:    "ddx-int-0001",
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "base-rev",
		ResultRev: "result-rev",
	}
	applyReusableWorkspaceTelemetry(res, got)
	report := ReportFromExecuteBeadResult(res, "")
	event := executeBeadLoopEvent(report, "worker", time.Unix(0, 0).UTC())
	require.Equal(t, "execute-bead", event.Kind)
	require.Contains(t, event.Body, "reusable_workspace_slot_hits=1")
	require.Contains(t, event.Body, "reusable_workspace_slot_misses=0")
	require.Contains(t, event.Body, "reusable_workspace_time_saved_ms=1834")
	require.Contains(t, event.Body, "reusable_workspace_bytes_saved=987654321")

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":1`)
	require.Contains(t, string(raw), `"slot_miss_count":0`)
	require.Contains(t, string(raw), `"time_saved_ms":1834`)
	require.Contains(t, string(raw), `"bytes_saved":987654321`)
}

func TestAttemptWorkspaceReuseTelemetryInputPreservesColdStartZeroSavings(t *testing.T) {
	got := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotHitCount:  0,
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{},
	)

	require.Equal(t, 0, got.SlotHitCount)
	require.Equal(t, 1, got.SlotMissCount)
	require.Zero(t, got.TimeSavedMS)
	require.Zero(t, got.BytesSaved)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"slot_hit_count":0`)
	require.Contains(t, string(raw), `"slot_miss_count":1`)
	require.Contains(t, string(raw), `"time_saved_ms":0`)
	require.Contains(t, string(raw), `"bytes_saved":0`)
}

func TestAttemptWorkspaceReuseTelemetryColdStartRecordsSlotMiss(t *testing.T) {
	telemetry := AttemptWorkspaceReuseTelemetryInputFromAllocationOutcome(
		AttemptWorkspaceReuseAllocationOutcome{
			SlotMissCount: 1,
		},
		AttemptWorkspaceReuseSavings{},
	)

	require.Zero(t, telemetry.SlotHitCount)
	require.Equal(t, 1, telemetry.SlotMissCount)
	require.Zero(t, telemetry.TimeSavedMS)
	require.Zero(t, telemetry.BytesSaved)

	app := &stubBeadEventAppender{}
	appendReusableWorkspaceTelemetry(app, "ddx-int-0001", ReusableWorkspaceTelemetry{
		AttemptID:     "20260801T010203-cold",
		SlotHitCount:  telemetry.SlotHitCount,
		SlotMissCount: telemetry.SlotMissCount,
		TimeSavedMS:   telemetry.TimeSavedMS,
		BytesSaved:    telemetry.BytesSaved,
	})

	require.Len(t, app.events, 1)
	evt := app.events[0].Event
	require.Equal(t, "reusable-workspace", evt.Kind)
	require.Contains(t, evt.Summary, "slot_hit_count=0")
	require.Contains(t, evt.Summary, "slot_miss_count=1")
	require.Contains(t, evt.Summary, "time_saved_ms=0")
	require.Contains(t, evt.Summary, "bytes_saved=0")

	var parsed ReusableWorkspaceTelemetry
	require.NoError(t, json.Unmarshal([]byte(evt.Body), &parsed))
	require.Equal(t, "20260801T010203-cold", parsed.AttemptID)
	require.Zero(t, parsed.SlotHitCount)
	require.Equal(t, 1, parsed.SlotMissCount)
	require.Zero(t, parsed.TimeSavedMS)
	require.Zero(t, parsed.BytesSaved)
}

func TestAttemptWorkspaceReuseCombinedTelemetryPayloadSchemaIsStable(t *testing.T) {
	reused := executeBeadLoopEvent(ExecuteBeadReport{
		BeadID:                       "ddx-reuse",
		Status:                       ExecuteBeadStatusNoChanges,
		ReusableWorkspaceSlotHits:    1,
		ReusableWorkspaceSlotMisses:  0,
		ReusableWorkspaceTimeSavedMS: 1834,
		ReusableWorkspaceBytesSaved:  987654321,
	}, "worker", time.Unix(0, 0).UTC())

	coldStart := executeBeadLoopEvent(ExecuteBeadReport{
		BeadID:                       "ddx-cold",
		Status:                       ExecuteBeadStatusNoChanges,
		ReusableWorkspaceSlotHits:    0,
		ReusableWorkspaceSlotMisses:  1,
		ReusableWorkspaceTimeSavedMS: 0,
		ReusableWorkspaceBytesSaved:  0,
	}, "worker", time.Unix(0, 0).UTC())

	require.Equal(t, "execute-bead", reused.Kind)
	require.Equal(t, "execute-bead", coldStart.Kind)

	reusedKeys := executeBeadTelemetryFieldNames(reused.Body)
	coldStartKeys := executeBeadTelemetryFieldNames(coldStart.Body)
	require.ElementsMatch(t, reusedKeys, coldStartKeys)
	require.Equal(t, []string{
		"decision_audit",
		"reusable_workspace_bytes_saved",
		"reusable_workspace_slot_hits",
		"reusable_workspace_slot_misses",
		"reusable_workspace_time_saved_ms",
	}, reusedKeys)
}

func executeBeadTelemetryFieldNames(body string) []string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	keys := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
