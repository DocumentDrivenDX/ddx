package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// tierSuccessRow is one row of the tier-success report.
type tierSuccessRow struct {
	Tier          string         `json:"tier"`
	Harness       string         `json:"harness"`
	Model         string         `json:"model,omitempty"`
	Attempts      int            `json:"attempts"`
	Successes     int            `json:"successes"`
	SuccessRate   float64        `json:"success_rate"`
	AvgCostUSD    float64        `json:"avg_cost_usd"`
	AvgDurationMS float64        `json:"avg_duration_ms"`
	FailureModes  map[string]int `json:"failure_modes,omitempty"`
}

func (f *CommandFactory) newAgentMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Analytics over agent execution evidence",
		Long:  "Aggregate execution evidence from .ddx/executions/*/result.json into routing analytics.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(f.newAgentMetricsTierSuccessCommand())
	return cmd
}

func (f *CommandFactory) newAgentMetricsTierSuccessCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tier-success",
		Short: "Report success rate per harness/model tier from execution evidence",
		Long: `Scan .ddx/executions/*/result.json and aggregate execution outcomes
into per-tier success rates. A tier is identified by harness alone when the
result has no model field, or by harness/model when a concrete model is
recorded. Success is defined as outcome == "task_succeeded".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lastN, _ := cmd.Flags().GetInt("last")
			jsonOut, _ := cmd.Flags().GetBool("json")

			rows, err := computeTierSuccess(f.WorkingDir, lastN)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return renderTierSuccessTable(cmd, rows)
		},
	}
	cmd.Flags().Int("last", 0, "Limit to most recent N attempts (0 = all)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

// computeTierSuccess scans .ddx/executions/*/result.json under workingDir and
// returns per-tier aggregates. When lastN > 0, only the most recent lastN
// attempts (sorted by attempt directory name, which is a sortable timestamp)
// are considered.
func computeTierSuccess(workingDir string, lastN int) ([]tierSuccessRow, error) {
	execRoot := filepath.Join(workingDir, ".ddx", "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []tierSuccessRow{}, nil
		}
		return nil, fmt.Errorf("read executions dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	// Directory names are sortable timestamps (YYYYMMDDTHHMMSS-<hash>), so
	// lexicographic sort is chronological.
	sort.Strings(names)

	// First pass: read and keep only usable results, preserving chronological
	// order. --last N then picks the most recent N usable attempts so that
	// malformed or missing files never hide a valid recent attempt.
	type loadedResult struct {
		harness     string
		model       string
		outcome     string
		failureMode string
		costUSD     float64
		durMS       int
	}
	loaded := make([]loadedResult, 0, len(names))
	for _, name := range names {
		resultPath := filepath.Join(execRoot, name, "result.json")
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res agent.ExecuteBeadResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		if res.Harness == "" {
			continue
		}
		loaded = append(loaded, loadedResult{
			harness:     res.Harness,
			model:       res.Model,
			outcome:     res.Outcome,
			failureMode: res.FailureMode,
			costUSD:     res.CostUSD,
			durMS:       res.DurationMS,
		})
	}
	if lastN > 0 && len(loaded) > lastN {
		loaded = loaded[len(loaded)-lastN:]
	}

	type agg struct {
		harness      string
		model        string
		attempts     int
		successes    int
		totalCostUSD float64
		totalDurMS   float64
		failureModes map[string]int
	}
	byTier := map[string]*agg{}
	order := []string{}

	for _, res := range loaded {
		tier := tierKey(res.harness, res.model)
		a, ok := byTier[tier]
		if !ok {
			a = &agg{harness: res.harness, model: res.model, failureModes: map[string]int{}}
			byTier[tier] = a
			order = append(order, tier)
		}
		a.attempts++
		if res.outcome == "task_succeeded" {
			a.successes++
		}
		if res.failureMode != "" {
			a.failureModes[res.failureMode]++
		}
		a.totalCostUSD += res.costUSD
		a.totalDurMS += float64(res.durMS)
	}

	rows := make([]tierSuccessRow, 0, len(order))
	for _, tier := range order {
		a := byTier[tier]
		row := tierSuccessRow{
			Tier:      tier,
			Harness:   a.harness,
			Model:     a.model,
			Attempts:  a.attempts,
			Successes: a.successes,
		}
		if a.attempts > 0 {
			row.SuccessRate = float64(a.successes) / float64(a.attempts)
			row.AvgCostUSD = a.totalCostUSD / float64(a.attempts)
			row.AvgDurationMS = a.totalDurMS / float64(a.attempts)
		}
		if len(a.failureModes) > 0 {
			row.FailureModes = a.failureModes
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Tier < rows[j].Tier })
	return rows, nil
}

func tierKey(harness, model string) string {
	if model == "" {
		return harness
	}
	return harness + "/" + model
}

func renderTierSuccessTable(cmd *cobra.Command, rows []tierSuccessRow) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-40s  %8s  %9s  %12s  %12s  %14s  %s\n",
		"TIER", "ATTEMPTS", "SUCCESSES", "SUCCESS_RATE", "AVG_COST_USD", "AVG_DURATION_MS", "FAILURE_MODES")
	for _, r := range rows {
		fmt.Fprintf(out, "%-40s  %8d  %9d  %12s  %12s  %14.1f  %s\n",
			truncateTier(r.Tier, 40),
			r.Attempts,
			r.Successes,
			fmt.Sprintf("%.3f", r.SuccessRate),
			fmt.Sprintf("%.4f", r.AvgCostUSD),
			r.AvgDurationMS,
			formatFailureModes(r.FailureModes),
		)
	}
	return nil
}

// formatFailureModes renders a failure-mode breakdown as a stable, sorted
// "mode=count,mode=count" string. Returns "-" when no failure modes were
// recorded so the column is never blank.
func formatFailureModes(modes map[string]int) string {
	if len(modes) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(modes))
	for k := range modes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, modes[k]))
	}
	return strings.Join(parts, ",")
}

func truncateTier(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const ellipsis = "…"
	if max <= len(ellipsis) {
		return strings.Repeat(".", max)
	}
	return s[:max-len(ellipsis)] + ellipsis
}
