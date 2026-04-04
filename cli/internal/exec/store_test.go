package exec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeExecArtifact(t *testing.T, wd, id string) {
	t.Helper()
	path := filepath.Join(wd, "docs", "metrics", id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nddx:\n  id: " + id + "\n---\n# " + id + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeExecDefinition(t *testing.T, wd string, def Definition) {
	t.Helper()
	store := NewStore(wd)
	require.NoError(t, store.SaveDefinition(def))
}

func TestValidateRunHistoryAndBundle(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeExecDefinition(t, wd, Definition{
		ID:          "exec-metric-startup-time@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '14.6ms\\n'"},
			Cwd:     ".",
		},
		Result: ResultSpec{
			Metric: &MetricResultSpec{Unit: "ms"},
		},
		Evaluation: Evaluation{
			Comparison: "lower-is-better",
			Thresholds: Thresholds{WarnMS: 20, RatchetMS: 30},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	def, doc, err := store.Validate("exec-metric-startup-time@1")
	require.NoError(t, err)
	require.Equal(t, "exec-metric-startup-time@1", def.ID)
	require.Equal(t, "MET-001", doc.ID)

	rec, err := store.Run(context.Background(), "exec-metric-startup-time@1")
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, rec.Status)
	require.NotNil(t, rec.Result.Metric)
	assert.InDelta(t, 14.6, rec.Result.Metric.Value, 0.01)
	assert.Equal(t, "ms", rec.Result.Metric.Unit)
	assert.Equal(t, "MET-001", rec.Result.Metric.ArtifactID)

	manifestPath := filepath.Join(store.RunsDir, rec.RunID, "manifest.json")
	resultPath := filepath.Join(store.RunsDir, rec.RunID, "result.json")
	stdoutPath := filepath.Join(store.RunsDir, rec.RunID, "stdout.log")
	stderrPath := filepath.Join(store.RunsDir, rec.RunID, "stderr.log")
	for _, path := range []string{manifestPath, resultPath, stdoutPath, stderrPath} {
		_, err := os.Stat(path)
		require.NoError(t, err)
	}

	history, err := store.History("MET-001", "")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, rec.RunID, history[0].RunID)

	stdout, stderr, err := store.Log(rec.RunID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "14.6")
	assert.Empty(t, stderr)

	result, err := store.Result(rec.RunID)
	require.NoError(t, err)
	require.NotNil(t, result.Metric)
	assert.Equal(t, rec.Result.Metric.Value, result.Metric.Value)
}

func TestConcurrentRunBundleWrites(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeExecDefinition(t, wd, Definition{
		ID:          "exec-metric-startup-time@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '14.6ms\\n'"},
		},
		Result:    ResultSpec{Metric: &MetricResultSpec{Unit: "ms"}},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	const writers = 12
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Run(context.Background(), "exec-metric-startup-time@1")
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	history, err := store.History("MET-001", "")
	require.NoError(t, err)
	assert.Len(t, history, writers)

	manifestCount := 0
	err = filepath.WalkDir(store.RunsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && d.Name() == "manifest.json" {
			manifestCount++
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, writers, manifestCount)
}

func mustExecTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

func TestDefinitionRoundTrips(t *testing.T) {
	wd := t.TempDir()
	store := NewStore(wd)
	def := Definition{
		ID:          "exec-metric-startup-time@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '14.6ms\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	}
	require.NoError(t, store.SaveDefinition(def))

	loaded, err := store.ShowDefinition(def.ID)
	require.NoError(t, err)
	raw, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "exec-metric-startup-time@1")
}
