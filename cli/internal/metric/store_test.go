package metric

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMetricArtifact(t *testing.T, wd, id string) {
	t.Helper()
	path := filepath.Join(wd, "docs", "metrics", id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nddx:\n  id: " + id + "\n---\n# " + id + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeMetricDefinition(t *testing.T, wd string, def Definition) {
	t.Helper()
	store := NewStore(wd)
	require.NoError(t, store.SaveDefinition(def))
}

func TestValidateRunAndHistory(t *testing.T) {
	wd := t.TempDir()
	writeMetricArtifact(t, wd, "MET-001")
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '14.6ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	def, doc, err := store.Validate("MET-001")
	require.NoError(t, err)
	require.Equal(t, "metric-startup-time@1", def.DefinitionID)
	require.Equal(t, "MET-001", doc.ID)

	rec, err := store.Run(context.Background(), "MET-001")
	require.NoError(t, err)
	assert.Equal(t, StatusPass, rec.Status)
	assert.InDelta(t, 14.6, rec.Value, 0.01)
	assert.Equal(t, "ms", rec.Unit)
	assert.Equal(t, "MET-001", rec.ArtifactID)

	history, err := store.History("MET-001")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, rec.RunID, history[0].RunID)
}

func TestCompareAndTrend(t *testing.T) {
	wd := t.TempDir()
	writeMetricArtifact(t, wd, "MET-001")
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '10ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	require.NoError(t, store.AppendHistory(HistoryRecord{
		RunID:        "MET-001@1",
		MetricID:     "MET-001",
		DefinitionID: "metric-startup-time@1",
		ObservedAt:   mustTime(t, "2026-04-04T15:00:00Z"),
		Status:       StatusPass,
		Value:        20,
		Unit:         "ms",
		Comparison:   ComparisonResult{Baseline: 20, Delta: 0, Direction: ComparisonLowerIsBetter},
		ArtifactID:   "MET-001",
	}))
	require.NoError(t, store.AppendHistory(HistoryRecord{
		RunID:        "MET-001@2",
		MetricID:     "MET-001",
		DefinitionID: "metric-startup-time@1",
		ObservedAt:   mustTime(t, "2026-04-04T15:01:00Z"),
		Status:       StatusPass,
		Value:        14.6,
		Unit:         "ms",
		Comparison:   ComparisonResult{Baseline: 20, Delta: -5.4, Direction: ComparisonLowerIsBetter},
		ArtifactID:   "MET-001",
	}))

	latest, result, err := store.Compare("MET-001", "baseline")
	require.NoError(t, err)
	assert.Equal(t, "MET-001@2", latest.RunID)
	assert.Equal(t, 20.0, result.Baseline)
	assert.InDelta(t, -5.4, result.Delta, 0.01)

	trend, err := store.Trend("MET-001")
	require.NoError(t, err)
	assert.Equal(t, 2, trend.Count)
	assert.InDelta(t, 14.6, trend.Latest, 0.01)
	assert.InDelta(t, 17.3, trend.Average, 0.01)
}

func TestConcurrentHistoryWrites(t *testing.T) {
	wd := t.TempDir()
	writeMetricArtifact(t, wd, "MET-001")
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '14.6ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	const writers = 12
	observedAt := mustTime(t, "2026-04-04T15:00:00Z")
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- store.AppendHistory(HistoryRecord{
				RunID:        fmt.Sprintf("MET-001@%d", i),
				MetricID:     "MET-001",
				DefinitionID: "metric-startup-time@1",
				ObservedAt:   observedAt,
				Status:       StatusPass,
				Value:        14.6,
				Unit:         "ms",
				Comparison:   ComparisonResult{Baseline: 20, Delta: -5.4, Direction: ComparisonLowerIsBetter},
				ArtifactID:   "MET-001",
			})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	raw, err := os.ReadFile(filepath.Join(wd, ".ddx", "exec-runs.jsonl"))
	require.NoError(t, err)
	lines := 0
	for _, b := range raw {
		if b == '\n' {
			lines++
		}
	}
	assert.Equal(t, writers, lines)

	history, err := store.History("MET-001")
	require.NoError(t, err)
	assert.Len(t, history, writers)
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

func TestSaveDefinitionRoundTrips(t *testing.T) {
	wd := t.TempDir()
	store := NewStore(wd)
	def := Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '14.6ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	}
	require.NoError(t, store.SaveDefinition(def))

	loaded, err := store.LoadDefinition("MET-001")
	require.NoError(t, err)
	raw, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "metric-startup-time@1")
}
