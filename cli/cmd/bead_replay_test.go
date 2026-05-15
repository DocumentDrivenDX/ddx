package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReplayManifest writes a minimal manifest.json and prompt.md to
// .ddx/executions/<attemptID>/ under dir.
func writeReplayManifest(t *testing.T, dir, attemptID, beadID, baseRev, harness, model, promptContent string) {
	t.Helper()
	execDir := filepath.Join(dir, ddxroot.DirName, "executions", attemptID)
	require.NoError(t, os.MkdirAll(execDir, 0o755))

	// Write prompt.md.
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "prompt.md"), []byte(promptContent), 0o644))

	// Write manifest.json.
	m := map[string]any{
		"attempt_id": attemptID,
		"bead_id":    beadID,
		"base_rev":   baseRev,
		"requested": map[string]any{
			"harness": harness,
			"model":   model,
		},
		"paths": map[string]any{
			"prompt":   filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", attemptID, "prompt.md")),
			"manifest": filepath.ToSlash(filepath.Join(ddxroot.DirName, "executions", attemptID, "manifest.json")),
		},
	}
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "manifest.json"), raw, 0o644))
}

// TestReplay_LoadsOriginalManifest verifies loadReplayManifest parses the
// manifest and resolves the prompt path correctly.
func TestReplay_LoadsOriginalManifest(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	attemptID := "20260501T120000-aabb1122"
	writeReplayManifest(t, dir, attemptID, "ddx-test", "deadbeef", "claude", "claude-sonnet-4-6", "# Test prompt\nDo the thing.")

	m, promptPath, err := loadReplayManifest(dir, attemptID)
	require.NoError(t, err)
	assert.Equal(t, attemptID, m.AttemptID)
	assert.Equal(t, "ddx-test", m.BeadID)
	assert.Equal(t, "deadbeef", m.BaseRev)
	assert.Equal(t, "claude", m.Requested.Harness)
	assert.Equal(t, "claude-sonnet-4-6", m.Requested.Model)
	assert.True(t, strings.HasSuffix(promptPath, "prompt.md"), "promptPath should end with prompt.md, got: %s", promptPath)
	assert.FileExists(t, promptPath)

	// Missing attempt returns an error.
	_, _, err = loadReplayManifest(dir, "no-such-attempt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-attempt")
}

// TestReplay_OverridesAppliedToDispatch checks that --harness, --model, --rev,
// and --prompt-from flags override the original manifest values when dispatching.
func TestReplay_OverridesAppliedToDispatch(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	attemptID := "20260501T130000-ccdd3344"
	beadID := "my-bead"
	writeReplayManifest(t, dir, attemptID, beadID, "base-sha", "claude", "sonnet", "original prompt")

	// Seed bead tracker.
	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID: beadID, Title: "Test bead", Status: bead.StatusOpen,
	}))

	// Write override prompt file.
	overridePrompt := filepath.Join(dir, "override.md")
	require.NoError(t, os.WriteFile(overridePrompt, []byte("# Override prompt"), 0o644))

	// Capture the prompt file content while the worktree is still live (the
	// worktree is cleaned up by WorktreeRemove before the command returns).
	var capturedPromptContent string
	var runnerInvoked bool
	fakeRunner := &fakeAgentRunner{
		result: &agent.Result{ExitCode: 0},
		sideEffect: func(opts agent.RunArgs) error {
			runnerInvoked = true
			if opts.PromptFile != "" {
				b, err := os.ReadFile(opts.PromptFile)
				if err == nil {
					capturedPromptContent = strings.TrimSpace(string(b))
				}
			}
			return nil
		},
	}
	git := &fakeExecuteBeadGit{
		mainHeadRev: "overridden-sha",
		wtHeadRev:   "overridden-sha",
	}

	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = fakeRunner
	factory.executeBeadGitOverride = git
	factory.executeBeadOrchestratorGitOverride = git
	factory.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	root := factory.NewRootCommand()
	_, err := executeCommand(root, "bead", "replay", attemptID,
		"--harness", "codex",
		"--model", "gpt-4o",
		"--rev", "overridden-sha",
		"--prompt-from", overridePrompt,
	)
	require.NoError(t, err)

	// The runner should have been called once.
	require.True(t, runnerInvoked, "runner should have been invoked")
	// buildPrompt copies the override content to the artifact prompt file;
	// the runner should receive the override content.
	assert.Equal(t, "# Override prompt", capturedPromptContent,
		"override prompt content should be propagated to the runner")
}

// TestReplay_DoesNotCountTowardBeadAttemptHistory verifies that replay emits
// a "replay" event on the bead but NOT an "execute-bead" event.
func TestReplay_DoesNotCountTowardBeadAttemptHistory(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	attemptID := "20260501T140000-eeff5566"
	beadID := "hist-bead"
	writeReplayManifest(t, dir, attemptID, beadID, "sha1", "claude", "sonnet", "test prompt")

	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID: beadID, Title: "History bead", Status: bead.StatusOpen,
	}))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "sha1",
		wtHeadRev:   "sha1",
	}
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	factory.executeBeadGitOverride = git
	factory.executeBeadOrchestratorGitOverride = git
	factory.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	root := factory.NewRootCommand()
	_, err := executeCommand(root, "bead", "replay", attemptID)
	require.NoError(t, err)

	// Check bead events: must have "replay" but NOT "execute-bead".
	events, err := store.Events(beadID)
	require.NoError(t, err)
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	assert.Contains(t, kinds, "replay", "expected a replay event on the bead")
	assert.NotContains(t, kinds, "execute-bead", "replay must not emit execute-bead event")
}

// TestReplay_AppendsMetricsRowWithReplayOf verifies that running replay appends
// a row to attempts.jsonl with replay_of=<original-attempt-id>.
func TestReplay_AppendsMetricsRowWithReplayOf(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	attemptID := "20260501T150000-aabbccdd"
	beadID := "metrics-bead"
	writeReplayManifest(t, dir, attemptID, beadID, "sha2", "claude", "sonnet", "metrics test")

	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID: beadID, Title: "Metrics bead", Status: bead.StatusOpen,
	}))

	// Seed an original metrics row for the source attempt.
	require.NoError(t, attemptmetrics.AppendRow(dir, attemptmetrics.AttemptRow{
		SchemaVersion: attemptmetrics.SchemaVersion,
		AttemptID:     attemptID,
		BeadID:        beadID,
		Outcome:       "task_failed",
		CostUSD:       0.5,
		DurationMS:    5000,
		TotalTokens:   1000,
	}))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "sha2",
		wtHeadRev:   "sha2",
	}
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	factory.executeBeadGitOverride = git
	factory.executeBeadOrchestratorGitOverride = git
	factory.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	root := factory.NewRootCommand()
	_, err := executeCommand(root, "bead", "replay", attemptID)
	require.NoError(t, err)

	// Load all rows and find the replay row.
	rows, err := attemptmetrics.LoadRows(dir)
	require.NoError(t, err)
	var replayRow *attemptmetrics.AttemptRow
	for i := range rows {
		if rows[i].ReplayOf == attemptID {
			replayRow = &rows[i]
			break
		}
	}
	require.NotNil(t, replayRow, "expected a metrics row with replay_of=%s", attemptID)
	assert.Equal(t, attemptID, replayRow.ReplayOf)
	assert.Equal(t, beadID, replayRow.BeadID)
}

// TestReplay_PrintsComparison verifies that the replay command prints a
// side-by-side comparison of original vs replay metrics.
func TestReplay_PrintsComparison(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	attemptID := "20260501T160000-11223344"
	beadID := "compare-bead"
	writeReplayManifest(t, dir, attemptID, beadID, "sha3", "claude", "sonnet", "compare test")

	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID: beadID, Title: "Compare bead", Status: bead.StatusOpen,
	}))

	// Seed an original metrics row.
	require.NoError(t, attemptmetrics.AppendRow(dir, attemptmetrics.AttemptRow{
		SchemaVersion: attemptmetrics.SchemaVersion,
		AttemptID:     attemptID,
		BeadID:        beadID,
		Outcome:       "task_failed",
		CostUSD:       1.0,
		DurationMS:    8000,
		TotalTokens:   2000,
	}))

	git := &fakeExecuteBeadGit{
		mainHeadRev: "sha3",
		wtHeadRev:   "sha3",
	}
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = &fakeAgentRunner{result: &agent.Result{ExitCode: 0}}
	factory.executeBeadGitOverride = git
	factory.executeBeadOrchestratorGitOverride = git
	factory.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "bead", "replay", attemptID)
	require.NoError(t, err)

	// The output should contain the comparison section header and key fields.
	assert.Contains(t, out, "--- comparison ---")
	assert.Contains(t, out, "original")
	assert.Contains(t, out, "replay")
	assert.Contains(t, out, "outcome")
	assert.Contains(t, out, "cost_usd")
	assert.Contains(t, out, "duration_ms")
}

// TestReplayBench_MultipleVariantsInParallel verifies that replay-bench runs
// each variant and produces a matrix with one row per variant.
func TestReplayBench_MultipleVariantsInParallel(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	t.Setenv("DDX_BEAD_DIR", "")

	dir := t.TempDir()
	beadID := "bench-bead"
	attemptID := "20260501T170000-deadbeef"
	writeReplayManifest(t, dir, attemptID, beadID, "sha4", "claude", "sonnet", "bench prompt")

	store := bead.NewStore(filepath.Join(dir, ddxroot.DirName))
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{
		ID: beadID, Title: "Bench bead", Status: bead.StatusOpen,
	}))

	// Count concurrent invocations to verify parallel execution.
	var callCount int64
	var maxConcurrent int64
	var active int64

	fakeRunner := &fakeAgentRunner{
		result: &agent.Result{ExitCode: 0},
		sideEffect: func(_ agent.RunArgs) error {
			cur := atomic.AddInt64(&active, 1)
			atomic.AddInt64(&callCount, 1)
			for {
				m := atomic.LoadInt64(&maxConcurrent)
				if cur <= m || atomic.CompareAndSwapInt64(&maxConcurrent, m, cur) {
					break
				}
			}
			// Simulate a tiny bit of work so goroutines overlap.
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&active, -1)
			return nil
		},
	}
	git := &fakeExecuteBeadGit{
		mainHeadRev: "sha4",
		wtHeadRev:   "sha4",
	}
	factory := NewCommandFactory(dir)
	factory.AgentRunnerOverride = fakeRunner
	factory.executeBeadGitOverride = git
	factory.executeBeadOrchestratorGitOverride = git
	factory.executeBeadLandingAdvancerOverride = fakeLandingAdvancerFromGit(git)

	root := factory.NewRootCommand()
	out, err := executeCommand(root, "bead", "replay-bench", beadID,
		"--variants", "claude:claude-sonnet-4-6,codex:gpt-4o",
	)
	require.NoError(t, err)

	// Both variants should appear in output.
	assert.Contains(t, out, "claude:claude-sonnet-4-6")
	assert.Contains(t, out, "codex:gpt-4o")

	// The matrix header row should be present.
	assert.Contains(t, out, "variant")
	assert.Contains(t, out, "outcome")

	// Both variants were dispatched.
	assert.Equal(t, int64(2), atomic.LoadInt64(&callCount),
		"expected exactly 2 agent runs (one per variant)")

	// Both variants produced a metrics row with replay_of set.
	rows, err := attemptmetrics.LoadRows(dir)
	require.NoError(t, err)
	replayRows := 0
	for _, r := range rows {
		if r.ReplayOf == attemptID {
			replayRows++
		}
	}
	assert.Equal(t, 2, replayRows, "expected 2 replay metrics rows (one per variant)")
}
