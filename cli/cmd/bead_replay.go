package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

// replayManifest holds the subset of manifest.json fields that replay needs.
type replayManifest struct {
	AttemptID string `json:"attempt_id"`
	BeadID    string `json:"bead_id"`
	BaseRev   string `json:"base_rev"`
	Requested struct {
		Harness  string `json:"harness,omitempty"`
		Model    string `json:"model,omitempty"`
		Provider string `json:"provider,omitempty"`
		Effort   string `json:"effort,omitempty"`
		MinPower int    `json:"min_power,omitempty"`
		MaxPower int    `json:"max_power,omitempty"`
	} `json:"requested"`
	Paths struct {
		Prompt string `json:"prompt"`
	} `json:"paths"`
}

// loadReplayManifest reads manifest.json for the given attempt and returns the
// parsed manifest plus the absolute path to prompt.md for the attempt.
func loadReplayManifest(projectRoot, attemptID string) (*replayManifest, string, error) {
	execDir := filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir, attemptID)
	manifestPath := filepath.Join(execDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, "", fmt.Errorf("load manifest for attempt %s: %w", attemptID, err)
	}
	var m replayManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, "", fmt.Errorf("parse manifest for attempt %s: %w", attemptID, err)
	}
	if m.BeadID == "" {
		return nil, "", fmt.Errorf("manifest for attempt %s has no bead_id", attemptID)
	}
	// Resolve prompt path: prefer manifest's tracked path, fall back to
	// the well-known sibling file in the execution dir.
	promptPath := filepath.Join(execDir, "prompt.md")
	if m.Paths.Prompt != "" {
		abs := filepath.Join(projectRoot, m.Paths.Prompt)
		if _, err := os.Stat(abs); err == nil {
			promptPath = abs
		}
	}
	return &m, promptPath, nil
}

// findLatestAttemptForBead scans .ddx/executions/ and returns the
// lexicographically largest (most recent, timestamp-prefixed) attempt ID
// whose manifest.json records the given bead ID.
func findLatestAttemptForBead(projectRoot, beadID string) (string, error) {
	execDir := filepath.Join(projectRoot, agent.ExecuteBeadArtifactDir)
	entries, err := os.ReadDir(execDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no executions found for bead %s", beadID)
		}
		return "", fmt.Errorf("read executions dir: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(execDir, entry.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var stub struct {
			BeadID string `json:"bead_id"`
		}
		if err := json.Unmarshal(raw, &stub); err != nil || stub.BeadID != beadID {
			continue
		}
		matches = append(matches, entry.Name())
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no executions found for bead %s", beadID)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// patchManifestReplayOf reads the manifest at execDir/manifest.json, injects
// replay_of=<originalAttemptID>, and writes it back. Best-effort: errors are
// silently ignored because the manifest is evidence, not contract.
func patchManifestReplayOf(projectRoot, executionDir, originalAttemptID string) {
	path := filepath.Join(projectRoot, executionDir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return
	}
	obj["replay_of"] = originalAttemptID
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, out, 0o644)
}

// buildReplayMetricsRow constructs an AttemptRow tagged as a replay of
// originalAttemptID.
func buildReplayMetricsRow(beadID, newAttemptID, originalAttemptID string, res *agent.ExecuteBeadResult) attemptmetrics.AttemptRow {
	row := attemptmetrics.AttemptRow{
		SchemaVersion: attemptmetrics.SchemaVersion,
		AttemptID:     newAttemptID,
		BeadID:        beadID,
		ReplayOf:      originalAttemptID,
		Outcome:       res.Outcome,
		ExitCode:      res.ExitCode,
		DurationMS:    res.DurationMS,
		CostUSD:       res.CostUSD,
		Harness:       res.Harness,
		Model:         res.Model,
		Provider:      res.Provider,
		SessionID:     res.SessionID,
		TotalTokens:   res.Tokens,
	}
	if !res.StartedAt.IsZero() {
		row.TSStart = res.StartedAt.UTC().Format(time.RFC3339)
	}
	if !res.FinishedAt.IsZero() {
		row.TSEnd = res.FinishedAt.UTC().Format(time.RFC3339)
	}
	return row
}

// loadOriginalMetricsRow returns the AttemptRow for attemptID from
// attempts.jsonl, or nil if not found.
func loadOriginalMetricsRow(projectRoot, attemptID string) *attemptmetrics.AttemptRow {
	rows, err := attemptmetrics.LoadRows(projectRoot)
	if err != nil {
		return nil
	}
	for i := range rows {
		if rows[i].AttemptID == attemptID {
			return &rows[i]
		}
	}
	return nil
}

// printReplayComparison writes a side-by-side original-vs-replay metrics table.
func printReplayComparison(w io.Writer, orig *attemptmetrics.AttemptRow, replayRes *agent.ExecuteBeadResult, origID, replayID string) {
	origOutcome, origCost, origDurMS, origTokens := "", 0.0, 0, 0
	if orig != nil {
		origOutcome = orig.Outcome
		origCost = orig.CostUSD
		origDurMS = orig.DurationMS
		origTokens = orig.TotalTokens
	}
	replayOutcome, replayCost, replayDurMS, replayTokens := "", 0.0, 0, 0
	if replayRes != nil {
		replayOutcome = replayRes.Outcome
		replayCost = replayRes.CostUSD
		replayDurMS = replayRes.DurationMS
		replayTokens = replayRes.Tokens
	}

	fmt.Fprintln(w, "\n--- comparison ---")
	fmt.Fprintf(w, "%-14s  %-30s  %s\n", "", "original", "replay")
	fmt.Fprintf(w, "%-14s  %-30s  %s\n", "attempt_id", truncate(origID, 30), replayID)
	fmt.Fprintf(w, "%-14s  %-30s  %s\n", "outcome", origOutcome, replayOutcome)
	fmt.Fprintf(w, "%-14s  %-30.4f  %.4f\n", "cost_usd", origCost, replayCost)
	fmt.Fprintf(w, "%-14s  %-30d  %d\n", "duration_ms", origDurMS, replayDurMS)
	fmt.Fprintf(w, "%-14s  %-30d  %d\n", "tokens", origTokens, replayTokens)
	if origCost > 0 && replayCost > 0 {
		fmt.Fprintf(w, "%-14s  %+.1f%%\n", "cost_delta", (replayCost-origCost)/origCost*100)
	}
	if origDurMS > 0 && replayDurMS > 0 {
		fmt.Fprintf(w, "%-14s  %+.1f%%\n", "dur_delta", (float64(replayDurMS)-float64(origDurMS))/float64(origDurMS)*100)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// replayVariant encodes one replay-bench variant: a harness name and optional
// model override (format: "harness" or "harness:model").
type replayVariant struct {
	Harness string
	Model   string
}

func parseReplayVariant(s string) replayVariant {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	v := replayVariant{Harness: parts[0]}
	if len(parts) == 2 {
		v.Model = parts[1]
	}
	return v
}

// replayBenchResult is the per-variant outcome for replay-bench.
type replayBenchResult struct {
	Variant    string
	AttemptID  string
	Outcome    string
	ExitCode   int
	CostUSD    float64
	DurationMS int
	Tokens     int
	Err        string
}

// runOneReplay executes a single replay for the given attempt/bead/variant
// and returns a replayBenchResult.
func (f *CommandFactory) runOneReplay(
	projectRoot, sourceAttemptID, beadID, promptFile, fromRev string,
	v replayVariant,
	gitOps agent.GitOps,
) replayBenchResult {
	overrides := config.CLIOverrides{
		Harness: v.Harness,
		Model:   v.Model,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return replayBenchResult{
			Variant: v.Harness + ":" + v.Model,
			Err:     err.Error(),
		}
	}
	runtime := agent.ExecuteBeadRuntime{
		FromRev:    fromRev,
		PromptFile: promptFile,
		WorkerID:   os.Getenv("DDX_WORKER_ID"),
		// BeadEvents nil: replay does not count toward drain-attempt history.
	}
	if f.AgentRunnerOverride != nil {
		runtime.AgentRunner = f.AgentRunnerOverride
	}
	res, runErr := agent.ExecuteBeadWithConfig(nil, projectRoot, beadID, rcfg, runtime, gitOps) //nolint:staticcheck
	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}
	if res == nil {
		return replayBenchResult{
			Variant: v.Harness + ":" + v.Model,
			Err:     errMsg,
		}
	}
	// Append metrics row.
	row := buildReplayMetricsRow(beadID, res.AttemptID, sourceAttemptID, res)
	_ = attemptmetrics.AppendRow(projectRoot, row)
	// Patch manifest with replay_of.
	if res.ExecutionDir != "" {
		patchManifestReplayOf(projectRoot, res.ExecutionDir, sourceAttemptID)
	}
	variantKey := v.Harness
	if v.Model != "" {
		variantKey += ":" + v.Model
	}
	return replayBenchResult{
		Variant:    variantKey,
		AttemptID:  res.AttemptID,
		Outcome:    res.Outcome,
		ExitCode:   res.ExitCode,
		CostUSD:    res.CostUSD,
		DurationMS: res.DurationMS,
		Tokens:     res.Tokens,
		Err:        errMsg,
	}
}

func (f *CommandFactory) newBeadReplayCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <attempt-id>",
		Short: "Re-execute a prior attempt with alternate model/harness/prompt",
		Long: `Re-execute a prior bead attempt with different configuration for A/B testing.

Loads the original attempt's manifest.json and prompt.md, then dispatches a new
agent run with any supplied overrides.  The replay is one-shot: it does NOT count
toward the bead's drain-attempt history (emits a "replay" event instead of
"execute-bead") and appends a metrics row tagged with replay_of=<original-attempt-id>.

After completion a side-by-side comparison of original vs replay metrics is printed.

Examples:
  ddx bead replay 20260501T120000-abcd1234
  ddx bead replay 20260501T120000-abcd1234 --harness claude --model claude-opus-4-7
  ddx bead replay 20260501T120000-abcd1234 --prompt-from my-revised-prompt.md`,
		Args: cobra.ExactArgs(1),
		RunE: f.runBeadReplay,
	}
	cmd.Flags().String("harness", "", "Override original harness")
	cmd.Flags().String("model", "", "Override original model")
	cmd.Flags().String("profile", "", "Override original profile")
	cmd.Flags().Int("min-power", 0, "Override routing floor (minimum model power)")
	cmd.Flags().String("prompt-from", "", "Override prompt with content from this file")
	cmd.Flags().String("rev", "", "Override base git revision (default: original base_rev)")
	return cmd
}

func (f *CommandFactory) runBeadReplay(cmd *cobra.Command, args []string) error {
	attemptID := args[0]
	projectRoot := resolveProjectRoot("", f.WorkingDir)

	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	minPower, _ := cmd.Flags().GetInt("min-power")
	promptFrom, _ := cmd.Flags().GetString("prompt-from")
	rev, _ := cmd.Flags().GetString("rev")

	m, defaultPromptPath, err := loadReplayManifest(projectRoot, attemptID)
	if err != nil {
		return err
	}

	origRow := loadOriginalMetricsRow(projectRoot, attemptID)

	// Apply flag overrides on top of original requested values.
	if harness == "" {
		harness = m.Requested.Harness
	}
	if model == "" {
		model = m.Requested.Model
	}
	if minPower == 0 {
		minPower = m.Requested.MinPower
	}
	promptFile := defaultPromptPath
	if promptFrom != "" {
		promptFile = promptFrom
	}
	fromRev := rev
	if fromRev == "" {
		fromRev = m.BaseRev
	}

	overrides := config.CLIOverrides{
		Harness:  harness,
		Model:    model,
		Profile:  profile,
		MinPower: minPower,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var gitOps agent.GitOps = &agent.RealGitOps{}
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}

	// BeadEvents is nil: replay does not count toward drain-attempt history.
	// Cost and routing events are NOT appended to the bead's event stream.
	runtime := agent.ExecuteBeadRuntime{
		FromRev:    fromRev,
		PromptFile: promptFile,
		WorkerID:   os.Getenv("DDX_WORKER_ID"),
	}
	if f.AgentRunnerOverride != nil {
		runtime.AgentRunner = f.AgentRunnerOverride
	}

	fmt.Fprintf(cmd.OutOrStdout(), "replaying: %s\n", attemptID)
	if harness != "" || model != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "config:    harness=%s model=%s\n", harness, model)
	}

	res, runErr := agent.ExecuteBeadWithConfig(cmd.Context(), projectRoot, m.BeadID, rcfg, runtime, gitOps)
	if runErr != nil && res == nil {
		return runErr
	}

	newAttemptID := ""
	if res != nil {
		newAttemptID = res.AttemptID
		// Append metrics row with replay_of tag.
		row := buildReplayMetricsRow(m.BeadID, newAttemptID, attemptID, res)
		_ = attemptmetrics.AppendRow(projectRoot, row)
		// Patch the new execution's manifest to record replay_of.
		if res.ExecutionDir != "" {
			patchManifestReplayOf(projectRoot, res.ExecutionDir, attemptID)
		}
	}

	// Emit a "replay" event (not "execute-bead") so the replay does not
	// count toward the bead's drain-attempt history.
	outcome := ""
	if res != nil {
		outcome = res.Outcome
	}
	store := bead.NewStore(resolveBeadStoreRoot(projectRoot))
	_ = store.AppendEvent(m.BeadID, bead.BeadEvent{
		Kind:    "replay",
		Summary: fmt.Sprintf("replay_of=%s outcome=%s run_id=%s", attemptID, outcome, newAttemptID),
		Body:    fmt.Sprintf(`{"replay_of":%q,"run_id":%q,"outcome":%q}`, attemptID, newAttemptID, outcome),
		Actor:   "ddx",
		Source:  "ddx bead replay",
	})

	fmt.Fprintf(cmd.OutOrStdout(), "run_id:    %s\n", newAttemptID)
	printReplayComparison(cmd.OutOrStdout(), origRow, res, attemptID, newAttemptID)

	return runErr
}

func (f *CommandFactory) newBeadReplayBenchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay-bench <bead-id>",
		Short: "Run multiple replays in parallel against the same source attempt",
		Long: `Run multiple replays in parallel for A/B model/harness comparison.

Finds the most recent execution for <bead-id>, then dispatches one replay per
variant in parallel.  Each variant is "harness" or "harness:model".  The result
is a matrix of outcomes, cost, duration, and token counts across variants.

Examples:
  ddx bead replay-bench ddx-abc123 --variants claude,codex
  ddx bead replay-bench ddx-abc123 --variants claude:claude-opus-4-7,claude:claude-sonnet-4-6`,
		Args: cobra.ExactArgs(1),
		RunE: f.runBeadReplayBench,
	}
	cmd.Flags().StringSlice("variants", nil, "Comma-separated list of variants (harness or harness:model)")
	_ = cmd.MarkFlagRequired("variants")
	cmd.Flags().String("attempt", "", "Source attempt ID (default: most recent for the bead)")
	return cmd
}

func (f *CommandFactory) runBeadReplayBench(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	projectRoot := resolveProjectRoot("", f.WorkingDir)

	variantStrs, _ := cmd.Flags().GetStringSlice("variants")
	sourceAttemptID, _ := cmd.Flags().GetString("attempt")

	if sourceAttemptID == "" {
		var err error
		sourceAttemptID, err = findLatestAttemptForBead(projectRoot, beadID)
		if err != nil {
			return err
		}
	}

	m, defaultPromptPath, err := loadReplayManifest(projectRoot, sourceAttemptID)
	if err != nil {
		return err
	}

	variants := make([]replayVariant, 0, len(variantStrs))
	for _, s := range variantStrs {
		// StringSlice already splits on comma; also accept semicolon-separated.
		for _, part := range strings.Split(s, ";") {
			part = strings.TrimSpace(part)
			if part != "" {
				variants = append(variants, parseReplayVariant(part))
			}
		}
	}
	if len(variants) == 0 {
		return fmt.Errorf("--variants: no variants specified")
	}

	var gitOps agent.GitOps = &agent.RealGitOps{}
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}

	results := make([]replayBenchResult, len(variants))
	var wg sync.WaitGroup
	for i, v := range variants {
		wg.Add(1)
		go func(idx int, variant replayVariant) {
			defer wg.Done()
			results[idx] = f.runOneReplay(projectRoot, sourceAttemptID, m.BeadID, defaultPromptPath, m.BaseRev, variant, gitOps)
		}(i, v)
	}
	wg.Wait()

	// Print matrix.
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nbench source: %s (bead: %s)\n", sourceAttemptID, beadID)
	fmt.Fprintf(out, "%-30s  %-20s  %-10s  %-10s  %-10s  %s\n",
		"variant", "outcome", "cost_usd", "dur_ms", "tokens", "attempt_id")
	fmt.Fprintln(out, strings.Repeat("-", 100))
	for _, r := range results {
		if r.Err != "" {
			fmt.Fprintf(out, "%-30s  ERROR: %s\n", r.Variant, r.Err)
			continue
		}
		fmt.Fprintf(out, "%-30s  %-20s  %-10.4f  %-10d  %-10d  %s\n",
			r.Variant, r.Outcome, r.CostUSD, r.DurationMS, r.Tokens, r.AttemptID)
	}
	return nil
}
