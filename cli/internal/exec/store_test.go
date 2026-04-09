package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAgentRunner struct {
	result *agent.Result
	err    error
}

func (m *mockAgentRunner) Run(opts agent.RunOptions) (*agent.Result, error) {
	return m.result, m.err
}

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

	manifestPath := filepath.Join(wd, ".ddx", execRunAttachmentDir, rec.RunID, "manifest.json")
	resultPath := filepath.Join(wd, ".ddx", execRunAttachmentDir, rec.RunID, "result.json")
	stdoutPath := filepath.Join(wd, ".ddx", execRunAttachmentDir, rec.RunID, "stdout.log")
	stderrPath := filepath.Join(wd, ".ddx", execRunAttachmentDir, rec.RunID, "stderr.log")
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
	runRoot := filepath.Join(wd, ".ddx", execRunAttachmentDir)
	err = filepath.WalkDir(runRoot, func(path string, d os.DirEntry, walkErr error) error {
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

func writeAgentExecDefinition(t *testing.T, wd string, def Definition) {
	t.Helper()
	store := NewStore(wd)
	require.NoError(t, store.SaveDefinition(def))
}

func TestAgentExecutorDelegation(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeAgentExecDefinition(t, wd, Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind: ExecutorKindAgent,
			Env: map[string]string{
				"DDX_AGENT_HARNESS": "codex",
				"DDX_AGENT_PROMPT":  "run the task",
			},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	store.AgentRunner = &mockAgentRunner{
		result: &agent.Result{
			Harness:  "codex",
			ExitCode: 0,
			Output:   "task complete",
			Stderr:   "",
		},
	}

	rec, err := store.Run(context.Background(), "exec-agent-task@1")
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, rec.Status)
	assert.Equal(t, 0, rec.ExitCode)
	assert.NotEmpty(t, rec.AgentSessionID)
	assert.Equal(t, "task complete", rec.Result.Stdout)

	history, err := store.History("MET-001", "")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, rec.RunID, history[0].RunID)
	assert.NotEmpty(t, history[0].AgentSessionID)
}

func TestAgentExecutorDelegationFailure(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeAgentExecDefinition(t, wd, Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind: ExecutorKindAgent,
			Env:  map[string]string{"DDX_AGENT_PROMPT": "run the task"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	store.AgentRunner = &mockAgentRunner{
		result: &agent.Result{
			Harness:  "codex",
			ExitCode: 1,
			Output:   "",
			Stderr:   "something went wrong",
			Error:    "something went wrong",
		},
	}

	rec, err := store.Run(context.Background(), "exec-agent-task@1")
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, rec.Status)
	assert.Equal(t, 1, rec.ExitCode)
	assert.Equal(t, "something went wrong", rec.Result.Stderr)
}

func TestAgentExecutorDelegationTimeout(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeAgentExecDefinition(t, wd, Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:      ExecutorKindAgent,
			Env:       map[string]string{"DDX_AGENT_PROMPT": "run the task"},
			TimeoutMS: 1000,
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	store.AgentRunner = &mockAgentRunner{
		result: &agent.Result{
			Harness:  "codex",
			ExitCode: -1,
			Error:    "timeout after 1s",
		},
	}

	rec, err := store.Run(context.Background(), "exec-agent-task@1")
	require.NoError(t, err)
	assert.Equal(t, StatusTimedOut, rec.Status)
	assert.Equal(t, -1, rec.ExitCode)
}

func TestAgentExecutorDelegationRunnerError(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeAgentExecDefinition(t, wd, Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind: ExecutorKindAgent,
			Env:  map[string]string{"DDX_AGENT_PROMPT": "run the task"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	store.AgentRunner = &mockAgentRunner{
		err: fmt.Errorf("harness not found: fake"),
	}

	rec, err := store.Run(context.Background(), "exec-agent-task@1")
	require.NoError(t, err)
	assert.Equal(t, StatusErrored, rec.Status)
	assert.Equal(t, 1, rec.ExitCode)
	assert.Contains(t, rec.Result.Stderr, "harness not found")

	// Verify the run is persisted in history
	history, err := store.History("MET-001", "")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, StatusErrored, history[0].Status)
}

func TestAgentExecutorNilRunner(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-001")
	writeAgentExecDefinition(t, wd, Definition{
		ID:          "exec-agent-task@1",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind: ExecutorKindAgent,
			Env:  map[string]string{"DDX_AGENT_PROMPT": "run the task"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	// AgentRunner is nil (not set)

	_, err := store.Run(context.Background(), "exec-agent-task@1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent runner not configured")
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

func TestListDefinitionsFallsBackToLegacyExecDirectory(t *testing.T) {
	wd := t.TempDir()
	legacyDir := filepath.Join(wd, ".ddx", "exec", "definitions")
	require.NoError(t, os.MkdirAll(legacyDir, 0o755))
	legacyDef := Definition{
		ID:          "exec-metric-startup-time@legacy",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf 'legacy\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-03T15:00:00Z"),
	}
	raw, err := json.MarshalIndent(legacyDef, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, legacyDef.ID+".json"), raw, 0o644))

	store := NewStore(wd)
	defs, err := store.ListDefinitions("MET-001")
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, legacyDef.ID, defs[0].ID)
}

func TestHistoryFallsBackToLegacyExecBundle(t *testing.T) {
	wd := t.TempDir()
	legacyRunDir := filepath.Join(wd, ".ddx", "exec", "runs", "exec-metric-startup-time@legacy")
	require.NoError(t, os.MkdirAll(legacyRunDir, 0o755))
	manifest := RunManifest{
		RunID:        "exec-metric-startup-time@legacy",
		DefinitionID: "exec-metric-startup-time@legacy",
		ArtifactIDs:  []string{"MET-001"},
		StartedAt:    mustExecTime(t, "2026-04-03T15:00:00Z"),
		FinishedAt:   mustExecTime(t, "2026-04-03T15:00:01Z"),
		Status:       StatusSuccess,
		ExitCode:     0,
		Attachments: map[string]string{
			"stdout": "stdout.log",
			"stderr": "stderr.log",
			"result": "result.json",
		},
	}
	result := RunResult{Stdout: "legacy stdout", Stderr: "", Parsed: true, Value: 12.3, Unit: "ms"}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	resultRaw, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(legacyRunDir, "manifest.json"), manifestRaw, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(legacyRunDir, "result.json"), resultRaw, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(legacyRunDir, "stdout.log"), []byte(result.Stdout), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(legacyRunDir, "stderr.log"), []byte(result.Stderr), 0o644))

	store := NewStore(wd)
	history, err := store.History("MET-001", "")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, manifest.RunID, history[0].RunID)
	stdout, stderr, err := store.Log(manifest.RunID)
	require.NoError(t, err)
	assert.Equal(t, result.Stdout, stdout)
	assert.Equal(t, result.Stderr, stderr)
}

// writeGraphExecDoc writes a markdown file with an exec: block in its ddx: frontmatter.
func writeGraphExecDoc(t *testing.T, wd, id, artifactID, kind string, command []string, required, active bool) {
	t.Helper()
	path := filepath.Join(wd, "docs", "exec", id+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	cmdYAML := ""
	for _, c := range command {
		cmdYAML += "        - " + c + "\n"
	}
	requiredStr := "false"
	if required {
		requiredStr = "true"
	}
	activeStr := "false"
	if active {
		activeStr = "true"
	}
	content := "---\nddx:\n  id: " + id + "\n  exec:\n    artifact_ids:\n      - " + artifactID +
		"\n    executor:\n      kind: " + kind + "\n      command:\n" + cmdYAML +
		"    required: " + requiredStr + "\n    active: " + activeStr + "\n---\n# " + id + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestGraphAuthoredDefinitionPrecedence(t *testing.T) {
	wd := t.TempDir()
	// Write the artifact document
	writeExecArtifact(t, wd, "MET-001")
	// Write a graph-authored exec doc with a different command than the runtime-managed one
	writeGraphExecDoc(t, wd, "graph-exec-def", "MET-001", ExecutorKindCommand,
		[]string{"sh", "-c", "printf 'graph-result\\n'"}, false, true)
	// Write a runtime-managed definition for the same document ID
	writeExecDefinition(t, wd, Definition{
		ID:          "graph-exec-def",
		ArtifactIDs: []string{"MET-001"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf 'runtime-result\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	def, err := store.ShowDefinition("graph-exec-def")
	require.NoError(t, err)
	// Graph-authored definition must win: GraphSource must be true
	assert.True(t, def.GraphSource, "graph-authored definition should be preferred")

	// Run it and verify the graph-authored command was used
	rec, err := store.Run(context.Background(), "graph-exec-def")
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, rec.Status)
	assert.Contains(t, rec.Result.Stdout, "graph-result")
}

func TestGraphAuthoredDefinitionFallbackToRuntime(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-002")
	// No graph-authored exec doc; only runtime-managed
	writeExecDefinition(t, wd, Definition{
		ID:          "runtime-only-exec",
		ArtifactIDs: []string{"MET-002"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf '99ms\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-04T15:00:00Z"),
	})

	store := NewStore(wd)
	def, err := store.ShowDefinition("runtime-only-exec")
	require.NoError(t, err)
	assert.False(t, def.GraphSource, "runtime-managed definition should not report GraphSource")
	assert.Equal(t, "runtime-only-exec", def.ID)
}

func TestRequiredFlagMergeBlockingOnFailure(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-003")
	writeGraphExecDoc(t, wd, "required-exec", "MET-003", ExecutorKindCommand,
		[]string{"sh", "-c", "exit 1"}, true, true)

	store := NewStore(wd)
	rec, err := store.Run(context.Background(), "required-exec")
	require.NoError(t, err)
	assert.NotEqual(t, StatusSuccess, rec.Status)
	assert.True(t, rec.MergeBlocking, "failed required definition must be merge-blocking")
}

func TestRequiredFlagNoMergeBlockingOnSuccess(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-004")
	writeGraphExecDoc(t, wd, "required-pass-exec", "MET-004", ExecutorKindCommand,
		[]string{"sh", "-c", "printf 'ok\\n'"}, true, true)

	store := NewStore(wd)
	rec, err := store.Run(context.Background(), "required-pass-exec")
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, rec.Status)
	assert.False(t, rec.MergeBlocking, "successful required definition must not be merge-blocking")
}

func TestSelfModifyingContractPrevention(t *testing.T) {
	// Graph-authored definitions are read from git-tracked files before any agent run.
	// Even if a runtime-managed definition with the same ID is saved later,
	// the graph-authored one should still take precedence on subsequent loads.
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-005")
	writeGraphExecDoc(t, wd, "self-mod-exec", "MET-005", ExecutorKindCommand,
		[]string{"sh", "-c", "printf 'original\\n'"}, false, true)

	// Simulate an agent saving a runtime definition with the same ID
	writeExecDefinition(t, wd, Definition{
		ID:          "self-mod-exec",
		ArtifactIDs: []string{"MET-005"},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf 'modified\\n'"},
		},
		Active:    true,
		CreatedAt: mustExecTime(t, "2026-04-05T15:00:00Z"),
	})

	store := NewStore(wd)
	def, err := store.ShowDefinition("self-mod-exec")
	require.NoError(t, err)
	// Must use the graph-authored version, not the runtime-managed modified one
	assert.True(t, def.GraphSource, "graph-authored definition must take precedence even after runtime save")

	rec, err := store.Run(context.Background(), "self-mod-exec")
	require.NoError(t, err)
	assert.Contains(t, rec.Result.Stdout, "original", "graph-authored command must be used, not the runtime-modified one")
}

func TestGraphAuthoredDefinitionInspection(t *testing.T) {
	wd := t.TempDir()
	writeExecArtifact(t, wd, "MET-006")
	writeGraphExecDoc(t, wd, "inspect-exec", "MET-006", ExecutorKindCommand,
		[]string{"sh", "-c", "printf '42ms\\n'"}, false, true)

	store := NewStore(wd)

	// Validate
	def, doc, err := store.Validate("inspect-exec")
	require.NoError(t, err)
	assert.Equal(t, "inspect-exec", def.ID)
	assert.Equal(t, "MET-006", doc.ID)
	assert.True(t, def.GraphSource)

	// Run
	rec, err := store.Run(context.Background(), "inspect-exec")
	require.NoError(t, err)
	assert.Equal(t, StatusSuccess, rec.Status)

	// History
	history, err := store.History("MET-006", "")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "inspect-exec", history[0].DefinitionID)

	// Result
	result, err := store.Result(rec.RunID)
	require.NoError(t, err)
	assert.True(t, result.Parsed)

	// Log
	stdout, _, err := store.Log(rec.RunID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "42ms")
}
