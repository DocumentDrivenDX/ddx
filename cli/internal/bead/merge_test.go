package bead

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeTrackerConflictPreservesUniqueRecords(t *testing.T) {
	base := trackerJSONL(`{"id":"ddx-base","title":"Base","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z"}`)
	ours := trackerJSONL(
		`{"id":"ddx-base","title":"Base","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z"}`,
		`{"id":"ddx-ours","title":"Ours","status":"open","priority":1,"issue_type":"task","created_at":"2026-04-30T01:00:00Z","updated_at":"2026-04-30T01:00:00Z"}`,
	)
	theirs := trackerJSONL(
		`{"id":"ddx-base","title":"Base","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z"}`,
		`{"id":"ddx-theirs","title":"Theirs","status":"open","priority":1,"issue_type":"task","created_at":"2026-04-30T02:00:00Z","updated_at":"2026-04-30T02:00:00Z"}`,
	)

	merged, report, err := MergeTrackerConflictJSONL(base, ours, theirs)
	require.NoError(t, err)
	require.Equal(t, 3, report.TotalRecords)
	require.Equal(t, 1, report.PreservedOurs)
	require.Equal(t, 1, report.PreservedTheirs)

	records := decodeMergedTrackerRecords(t, merged)
	require.Contains(t, records, "ddx-base")
	require.Contains(t, records, "ddx-ours")
	require.Contains(t, records, "ddx-theirs")
	require.NoError(t, ValidateTrackerJSONLUnique(merged))
}

func TestMergeTrackerConflictUnionsEventsAndDependencies(t *testing.T) {
	base := trackerJSONL(`{"id":"ddx-task","title":"Task","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z","dependencies":[{"issue_id":"ddx-task","depends_on_id":"ddx-base","type":"blocks"}],"events":[{"kind":"note","summary":"base","created_at":"2026-04-30T00:00:00Z"}]}`)
	ours := trackerJSONL(`{"id":"ddx-task","title":"Task","status":"in_progress","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T01:00:00Z","dependencies":[{"issue_id":"ddx-task","depends_on_id":"ddx-base","type":"blocks"},{"issue_id":"ddx-task","depends_on_id":"ddx-ours","type":"blocks"}],"events":[{"kind":"note","summary":"base","created_at":"2026-04-30T00:00:00Z"},{"kind":"claim","summary":"ours","created_at":"2026-04-30T01:00:00Z"}]}`)
	theirs := trackerJSONL(`{"id":"ddx-task","title":"Task","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T02:00:00Z","dependencies":[{"issue_id":"ddx-task","depends_on_id":"ddx-base","type":"blocks"},{"issue_id":"ddx-task","depends_on_id":"ddx-theirs","type":"blocks"}],"events":[{"kind":"note","summary":"base","created_at":"2026-04-30T00:00:00Z"},{"kind":"review","summary":"theirs","created_at":"2026-04-30T02:00:00Z"}]}`)

	merged, report, err := MergeTrackerConflictJSONL(base, ours, theirs)
	require.NoError(t, err)
	require.Equal(t, 1, report.MergedRecords)

	rec := decodeMergedTrackerRecords(t, merged)["ddx-task"]
	deps, ok := rec["dependencies"].([]any)
	require.True(t, ok)
	require.Len(t, deps, 3)
	require.Contains(t, dependencyTargets(deps), "ddx-base")
	require.Contains(t, dependencyTargets(deps), "ddx-ours")
	require.Contains(t, dependencyTargets(deps), "ddx-theirs")

	events, ok := rec["events"].([]any)
	require.True(t, ok)
	require.Len(t, events, 3)
	require.Contains(t, eventSummaries(events), "base")
	require.Contains(t, eventSummaries(events), "ours")
	require.Contains(t, eventSummaries(events), "theirs")
}

func TestMergeTrackerConflictReportsScalarConflictChoice(t *testing.T) {
	base := trackerJSONL(`{"id":"ddx-task","title":"Base","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T00:00:00Z"}`)
	ours := trackerJSONL(`{"id":"ddx-task","title":"Ours","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T01:00:00Z"}`)
	theirs := trackerJSONL(`{"id":"ddx-task","title":"Theirs","status":"open","priority":2,"issue_type":"task","created_at":"2026-04-30T00:00:00Z","updated_at":"2026-04-30T02:00:00Z"}`)

	merged, report, err := MergeTrackerConflictJSONL(base, ours, theirs)
	require.NoError(t, err)
	require.NotEmpty(t, report.ScalarConflicts)
	require.Contains(t, report.ScalarConflicts, TrackerScalarConflict{
		ID:     "ddx-task",
		Field:  "title",
		Choice: "theirs",
		Reason: "theirs has newer updated_at",
	})

	rec := decodeMergedTrackerRecords(t, merged)["ddx-task"]
	require.Equal(t, "Theirs", rec["title"])
}

func trackerJSONL(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

func decodeMergedTrackerRecords(t *testing.T, data []byte) map[string]map[string]any {
	t.Helper()
	out := make(map[string]map[string]any)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		id, _ := rec["id"].(string)
		out[id] = rec
	}
	return out
}

func dependencyTargets(deps []any) []string {
	out := make([]string, 0, len(deps))
	for _, dep := range deps {
		m, ok := dep.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := m["depends_on_id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

func eventSummaries(events []any) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		m, ok := event.(map[string]any)
		if !ok {
			continue
		}
		if summary, ok := m["summary"].(string); ok {
			out = append(out, summary)
		}
	}
	return out
}
