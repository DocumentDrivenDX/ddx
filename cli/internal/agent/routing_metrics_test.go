package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quotaTestHarness() Harness {
	return Harness{
		Name:         "quota-test",
		Binary:       "quota-test",
		PromptMode:   "arg",
		QuotaCommand: "usage",
	}
}

func TestRoutingMetricsStoreRoundTrip(t *testing.T) {
	store := NewRoutingMetricsStore(t.TempDir())

	outcome := RoutingOutcome{
		Harness:         "codex",
		Surface:         "codex",
		CanonicalTarget: "gpt-5.4",
		ObservedAt:      time.Date(2026, 4, 9, 18, 0, 0, 0, time.UTC),
		Success:         true,
		LatencyMS:       1234,
		InputTokens:     100,
		OutputTokens:    50,
		CostUSD:         0.75,
		NativeSessionID: "native-1",
		TraceID:         "trace-1",
	}
	require.NoError(t, store.AppendOutcome(outcome))

	snapshot := QuotaSnapshot{
		Harness:         "codex",
		Surface:         "codex",
		CanonicalTarget: "gpt-5.4",
		Source:          "codex usage",
		ObservedAt:      time.Date(2026, 4, 9, 18, 5, 0, 0, time.UTC),
		QuotaState:      "ok",
		UsedPercent:     42,
		WindowMinutes:   300,
		ResetsAt:        "April 12",
		SampleKind:      "async-probe",
	}
	require.NoError(t, store.AppendQuotaSnapshot(snapshot))
	snapshot2 := snapshot
	snapshot2.ObservedAt = snapshot.ObservedAt.Add(10 * time.Minute)
	snapshot2.UsedPercent = 55
	require.NoError(t, store.AppendQuotaSnapshot(snapshot2))

	outcomes, err := store.ReadOutcomes()
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, outcome, outcomes[0])

	snapshots, err := store.ReadQuotaSnapshots()
	require.NoError(t, err)
	require.Len(t, snapshots, 2)
	assert.Equal(t, snapshot, snapshots[0])
	assert.Equal(t, snapshot2, snapshots[1])

	summaries := BuildBurnSummaries(snapshots)
	require.Len(t, summaries, 1)
	assert.Equal(t, "codex", summaries[0].Harness)
	assert.Equal(t, "rising", summaries[0].Trend)
	assert.InDelta(t, 0.55, summaries[0].BurnIndex, 0.0001)
}

func TestProbeHarnessStatePersistsQuotaSnapshot(t *testing.T) {
	logDir := t.TempDir()
	r := NewRunner(Config{SessionLogDir: logDir})
	r.LookPath = mockLookPath
	r.Executor = &mockExecutor{output: "83% of 5h limit, resets April 12"}
	r.Registry.harnesses["quota-test"] = quotaTestHarness()

	state := r.ProbeHarnessState("quota-test", 5*time.Second)
	require.True(t, state.QuotaOK)
	require.NotNil(t, state.Quota)

	snapshots, err := NewRoutingMetricsStore(logDir).ReadQuotaSnapshots()
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	assert.Equal(t, "quota-test", snapshots[0].Harness)
	assert.Equal(t, "async-probe", snapshots[0].SampleKind)
	assert.Equal(t, 83, snapshots[0].UsedPercent)
	assert.Equal(t, 300, snapshots[0].WindowMinutes)
	assert.Equal(t, "ok", snapshots[0].QuotaState)
}
