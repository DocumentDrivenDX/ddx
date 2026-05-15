package metric

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMetricTestWorkingDir(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return t.TempDir()
}

func writeMetricArtifact(t *testing.T, wd, id string) {
	t.Helper()
	path := filepath.Join(wd, "docs", "metrics", id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nddx:\n  id: " + id + "\n---\n# " + id + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeArtifact(t *testing.T, wd, dir, id string) {
	t.Helper()
	path := filepath.Join(wd, "docs", dir, id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nddx:\n  id: " + id + "\n---\n# " + id + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeMetricDefinition(t *testing.T, wd string, def Definition) {
	t.Helper()
	store := ddxexec.NewStore(wd)
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          def.DefinitionID,
		ArtifactIDs: []string{def.MetricID},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: append([]string{}, def.Command...),
			Cwd:     def.Cwd,
			Env:     cloneStringMap(def.Env),
		},
		Result: ddxexec.ResultSpec{
			Metric: &ddxexec.MetricResultSpec{
				Unit: def.Thresholds.Unit,
			},
		},
		Evaluation: ddxexec.Evaluation{
			Comparison: def.Comparison,
			Thresholds: ddxexec.Thresholds{
				WarnMS:    def.Thresholds.Warn,
				RatchetMS: def.Thresholds.Ratchet,
			},
		},
		Active:    def.Active,
		CreatedAt: def.CreatedAt,
	}))
}

func TestValidateRunAndHistory(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
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

func TestDefinitionAndHistoryPreferMetricArtifactID(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
	writeMetricArtifact(t, wd, "MET-001")
	writeArtifact(t, wd, "misc", "DOC-001")
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
	def, err := store.LoadDefinition("MET-001")
	require.NoError(t, err)
	assert.Equal(t, "MET-001", def.MetricID)

	rec, err := store.Run(context.Background(), "MET-001")
	require.NoError(t, err)
	assert.Equal(t, "MET-001", rec.ArtifactID)
	assert.Equal(t, "MET-001", rec.MetricID)

	history, err := store.History("MET-001")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "MET-001", history[0].MetricID)
	assert.Equal(t, "MET-001", history[0].ArtifactID)
}

func TestCompareAndTrend(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
	writeMetricArtifact(t, wd, "MET-001")
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '20ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	firstRec, err := store.Run(context.Background(), "MET-001")
	require.NoError(t, err)
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '14.6ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:01:00Z"),
	})
	secondRec, err := store.Run(context.Background(), "MET-001")
	require.NoError(t, err)

	latest, result, err := store.Compare("MET-001", "baseline")
	require.NoError(t, err)
	assert.Equal(t, secondRec.RunID, latest.RunID)
	assert.Equal(t, 20.0, result.Baseline)
	assert.InDelta(t, -5.4, result.Delta, 0.01)

	trend, err := store.Trend("MET-001")
	require.NoError(t, err)
	assert.Equal(t, 2, trend.Count)
	assert.InDelta(t, 14.6, trend.Latest, 0.01)
	assert.InDelta(t, 17.3, trend.Average, 0.01)
	assert.NotEmpty(t, firstRec.RunID)
}

func TestCompareAndTrendRejectMixedUnitsAndGroupHistoryByUnit(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
	writeMetricArtifact(t, wd, "MET-001")
	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '20ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	_, err := store.Run(context.Background(), "MET-001")
	require.NoError(t, err)

	writeMetricDefinition(t, wd, Definition{
		DefinitionID: "metric-startup-time@2",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '0.01USD\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "USD"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:01:00Z"),
	})
	_, err = store.Run(context.Background(), "MET-001")
	require.NoError(t, err)

	_, _, err = store.Compare("MET-001", "baseline")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mixed units")

	_, err = store.Trend("MET-001")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mixed units")

	groups, err := store.GroupedHistory("MET-001")
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "ms", groups[0].Unit)
	assert.Equal(t, "USD", groups[1].Unit)
	require.Len(t, groups[0].Records, 1)
	require.Len(t, groups[1].Records, 1)
}

func TestConcurrentHistoryWrites(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
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
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.Run(context.Background(), "MET-001")
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	raw, err := os.ReadFile(ddxroot.JoinProject(wd, "exec-runs.jsonl"))
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

// TestLegacyMetricsDirIgnored verifies that a pre-exec .ddx/metrics/ directory
// left over from a pre-release build does not cause DDx to crash. The current
// metric store delegates to the exec substrate and never reads .ddx/metrics/.
func TestLegacyMetricsDirIgnored(t *testing.T) {
	wd := newMetricTestWorkingDir(t)

	// Simulate old-format .ddx/metrics/ data written by the pre-exec metric store.
	metricsDir := filepath.Join(wd, ddxroot.DirName, "metrics")
	defsDir := filepath.Join(metricsDir, "definitions")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))

	oldDef := `{"definition_id":"metric-startup-time@1","metric_id":"MET-001","command":["sh","-c","echo 10ms"],"active":true,"created_at":"2026-04-04T10:00:00Z"}`
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "metric-startup-time@1.json"), []byte(oldDef), 0o644))

	oldHistory := `{"run_id":"MET-001@1","metric_id":"MET-001","definition_id":"metric-startup-time@1","observed_at":"2026-04-04T10:00:01Z","status":"pass","value":10,"unit":"ms"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(metricsDir, "history.jsonl"), []byte(oldHistory), 0o644))

	// The current store must not crash when .ddx/metrics/ exists.
	store := NewStore(wd)

	// History returns empty — legacy metrics/ data is not read.
	history, err := store.History("MET-001")
	require.NoError(t, err)
	assert.Empty(t, history)
}

func TestSaveDefinitionRoundTrips(t *testing.T) {
	wd := newMetricTestWorkingDir(t)
	store := ddxexec.NewStore(wd)
	def := Definition{
		DefinitionID: "metric-startup-time@1",
		MetricID:     "MET-001",
		Command:      []string{"sh", "-c", "printf '14.6ms\\n'"},
		Thresholds:   Thresholds{Warn: 20, Ratchet: 30, Unit: "ms"},
		Comparison:   ComparisonLowerIsBetter,
		Active:       true,
		CreatedAt:    mustTime(t, "2026-04-04T15:00:00Z"),
	}
	require.NoError(t, store.SaveDefinition(ddxexec.Definition{
		ID:          def.DefinitionID,
		ArtifactIDs: []string{def.MetricID},
		Executor: ddxexec.ExecutorSpec{
			Kind:    ddxexec.ExecutorKindCommand,
			Command: append([]string{}, def.Command...),
			Cwd:     def.Cwd,
			Env:     cloneStringMap(def.Env),
		},
		Result: ddxexec.ResultSpec{
			Metric: &ddxexec.MetricResultSpec{
				Unit: def.Thresholds.Unit,
			},
		},
		Evaluation: ddxexec.Evaluation{
			Comparison: def.Comparison,
			Thresholds: ddxexec.Thresholds{
				WarnMS:    def.Thresholds.Warn,
				RatchetMS: def.Thresholds.Ratchet,
			},
		},
		Active:    def.Active,
		CreatedAt: def.CreatedAt,
	}))

	loaded, err := NewStore(wd).LoadDefinition("MET-001")
	require.NoError(t, err)
	raw, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "metric-startup-time@1")
}
