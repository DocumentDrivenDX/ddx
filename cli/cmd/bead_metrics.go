package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// beadMetricsSummary is the aggregate cost/token/attempt record for a single
// bead, derived from .ddx/executions/*/result.json.
type beadMetricsSummary struct {
	AttemptCount  int     `json:"attempt_count"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	AvgDurationMS float64 `json:"avg_duration_ms"`
}

// beadMetricsRow is one row of the ddx bead metrics report.
type beadMetricsRow struct {
	BeadID        string  `json:"bead_id"`
	Title         string  `json:"title,omitempty"`
	AttemptCount  int     `json:"attempt_count"`
	TotalTokens   int     `json:"total_tokens"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	AvgDurationMS float64 `json:"avg_duration_ms"`
}

// scanBeadMetrics reads .ddx/executions/*/result.json under workingDir and
// groups attempts by bead_id. Malformed or bead_id-less results are skipped.
func scanBeadMetrics(workingDir string) (map[string]*beadMetricsSummary, error) {
	execRoot := filepath.Join(workingDir, ".ddx", "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*beadMetricsSummary{}, nil
		}
		return nil, fmt.Errorf("read executions dir: %w", err)
	}

	type agg struct {
		attempts   int
		tokens     int
		cost       float64
		totalDurMS float64
	}
	byBead := map[string]*agg{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		resultPath := filepath.Join(execRoot, entry.Name(), "result.json")
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res agent.ExecuteBeadResult
		if err := json.Unmarshal(raw, &res); err != nil {
			continue
		}
		if res.BeadID == "" {
			continue
		}
		a, ok := byBead[res.BeadID]
		if !ok {
			a = &agg{}
			byBead[res.BeadID] = a
		}
		a.attempts++
		a.tokens += res.Tokens
		a.cost += res.CostUSD
		a.totalDurMS += float64(res.DurationMS)
	}

	out := make(map[string]*beadMetricsSummary, len(byBead))
	for id, a := range byBead {
		s := &beadMetricsSummary{
			AttemptCount: a.attempts,
			TotalTokens:  a.tokens,
			TotalCostUSD: a.cost,
		}
		if a.attempts > 0 {
			s.AvgDurationMS = a.totalDurMS / float64(a.attempts)
		}
		out[id] = s
	}
	return out, nil
}

// beadMetricsFor returns the aggregate metrics for a single bead, or nil when
// no execution evidence exists.
func beadMetricsFor(workingDir, beadID string) (*beadMetricsSummary, error) {
	all, err := scanBeadMetrics(workingDir)
	if err != nil {
		return nil, err
	}
	return all[beadID], nil
}

func (f *CommandFactory) newBeadMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Per-bead cost, token, and attempt summaries from execution evidence",
		Long: `Scan .ddx/executions/*/result.json and aggregate per-bead execution cost,
tokens, attempts, and average duration. One row per bead_id with recorded
execution evidence.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")

			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}

			summaries, err := scanBeadMetrics(workspaceRoot)
			if err != nil {
				return err
			}

			titleByID := map[string]string{}
			if store := f.beadStore(); store != nil {
				if all, err := store.List("", "", nil); err == nil {
					for _, b := range all {
						titleByID[b.ID] = b.Title
					}
				}
			}

			rows := make([]beadMetricsRow, 0, len(summaries))
			for id, s := range summaries {
				rows = append(rows, beadMetricsRow{
					BeadID:        id,
					Title:         titleByID[id],
					AttemptCount:  s.AttemptCount,
					TotalTokens:   s.TotalTokens,
					TotalCostUSD:  s.TotalCostUSD,
					AvgDurationMS: s.AvgDurationMS,
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].BeadID < rows[j].BeadID })

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return renderBeadMetricsTable(cmd, rows)
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func renderBeadMetricsTable(cmd *cobra.Command, rows []beadMetricsRow) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-20s  %8s  %12s  %12s  %14s  %s\n",
		"BEAD_ID", "ATTEMPTS", "TOTAL_TOKENS", "TOTAL_COST_USD", "AVG_DURATION_MS", "TITLE")
	for _, r := range rows {
		fmt.Fprintf(out, "%-20s  %8d  %12d  %12s  %14.1f  %s\n",
			r.BeadID,
			r.AttemptCount,
			r.TotalTokens,
			fmt.Sprintf("%.4f", r.TotalCostUSD),
			r.AvgDurationMS,
			r.Title,
		)
	}
	return nil
}
