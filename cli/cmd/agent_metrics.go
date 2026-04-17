package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

// tierSuccessRow is one row of the tier-success report.
type tierSuccessRow struct {
	Tier             string         `json:"tier"`
	Harness          string         `json:"harness"`
	Model            string         `json:"model,omitempty"`
	Attempts         int            `json:"attempts"`
	Successes        int            `json:"successes"`
	SuccessRate      float64        `json:"success_rate"`
	AvgCostUSD       float64        `json:"avg_cost_usd"`
	WastedCostUSD    float64        `json:"wasted_cost_usd"`
	EffectiveCostUSD float64        `json:"effective_cost_usd"`
	AvgDurationMS    float64        `json:"avg_duration_ms"`
	FailureModes     map[string]int `json:"failure_modes,omitempty"`
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
	cmd.AddCommand(f.newAgentMetricsReviewOutcomesCommand())
	cmd.AddCommand(f.newAgentMetricsCostEfficiencyCommand())
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
		harness          string
		model            string
		attempts         int
		successes        int
		totalCostUSD     float64
		wastedCostUSD    float64
		effectiveCostUSD float64
		totalDurMS       float64
		failureModes     map[string]int
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
			a.effectiveCostUSD += res.costUSD
		} else {
			a.wastedCostUSD += res.costUSD
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
			Tier:             tier,
			Harness:          a.harness,
			Model:            a.model,
			Attempts:         a.attempts,
			Successes:        a.successes,
			WastedCostUSD:    a.wastedCostUSD,
			EffectiveCostUSD: a.effectiveCostUSD,
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
	fmt.Fprintf(out, "%-40s  %8s  %9s  %12s  %12s  %14s  %14s  %14s  %s\n",
		"TIER", "ATTEMPTS", "SUCCESSES", "SUCCESS_RATE", "AVG_COST_USD", "WASTED_COST", "EFFECTIVE_COST", "AVG_DURATION_MS", "FAILURE_MODES")
	for _, r := range rows {
		fmt.Fprintf(out, "%-40s  %8d  %9d  %12s  %12s  %14s  %14s  %14.1f  %s\n",
			truncateTier(r.Tier, 40),
			r.Attempts,
			r.Successes,
			fmt.Sprintf("%.3f", r.SuccessRate),
			fmt.Sprintf("%.4f", r.AvgCostUSD),
			fmt.Sprintf("%.4f", r.WastedCostUSD),
			fmt.Sprintf("%.4f", r.EffectiveCostUSD),
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

// reviewOutcomesRow is one row of the review-outcomes report, aggregating
// post-merge review verdicts per originating execution tier.
type reviewOutcomesRow struct {
	Tier         string  `json:"tier"`
	Harness      string  `json:"harness"`
	Model        string  `json:"model,omitempty"`
	Reviews      int     `json:"reviews"`
	Approvals    int     `json:"approvals"`
	Rejections   int     `json:"rejections"`
	ApprovalRate float64 `json:"approval_rate"`
}

func (f *CommandFactory) newAgentMetricsReviewOutcomesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review-outcomes",
		Short: "Report post-merge review verdicts per originating tier",
		Long: `Scan kind:review evidence events on beads and aggregate review
verdicts (approve, approve_with_edits, reject) per originating harness/model
tier. The originating tier is derived from the most recent kind:routing
evidence event that precedes each review on the same bead — that is the
provider/model that produced the work being reviewed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")

			rows, err := computeReviewOutcomes(f.WorkingDir)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return renderReviewOutcomesTable(cmd, rows)
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

// reviewVerdictCategory normalises a kind:review event summary into one of
// "approve", "approve_with_edits", "reject", or "" when it cannot be
// classified. Matching is case-insensitive and tolerates the canonical
// verdict names used by the reviewer (APPROVE / REQUEST_CHANGES / BLOCK)
// as well as their generic equivalents.
func reviewVerdictCategory(summary string) string {
	s := strings.ToUpper(strings.TrimSpace(summary))
	switch s {
	case "APPROVE":
		return "approve"
	case "APPROVE_WITH_EDITS", "REQUEST_CHANGES":
		return "approve_with_edits"
	case "REJECT", "BLOCK":
		return "reject"
	}
	return ""
}

// computeReviewOutcomes scans every bead in workingDir/.ddx, attributes each
// kind:review event to the most recent kind:routing event that precedes it on
// the same bead, and aggregates per-tier counts of approvals vs rejections.
// Reviews on beads without any preceding routing event are bucketed under an
// "unknown" tier so they are surfaced rather than silently dropped.
func computeReviewOutcomes(workingDir string) ([]reviewOutcomesRow, error) {
	store := bead.NewStore(filepath.Join(workingDir, ".ddx"))
	beads, err := store.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read beads: %w", err)
	}

	type agg struct {
		harness    string
		model      string
		reviews    int
		approvals  int
		rejections int
	}
	byTier := map[string]*agg{}
	order := []string{}

	for _, b := range beads {
		events := allBeadEventsFromExtra(b.Extra)
		// Walk events in chronological order, tracking the most recent
		// routing decision so each review is attributed to the tier that
		// produced the work under review.
		sort.SliceStable(events, func(i, j int) bool {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		})

		var curHarness, curModel string
		haveRouting := false
		for _, e := range events {
			switch e.Kind {
			case "routing":
				if h, m, ok := parseRoutingHarnessModel(e); ok {
					curHarness = h
					curModel = m
					haveRouting = true
				}
			case "review":
				cat := reviewVerdictCategory(e.Summary)
				if cat == "" {
					continue
				}
				harness, model := curHarness, curModel
				if !haveRouting {
					harness, model = "unknown", ""
				}
				tier := tierKey(harness, model)
				a, ok := byTier[tier]
				if !ok {
					a = &agg{harness: harness, model: model}
					byTier[tier] = a
					order = append(order, tier)
				}
				a.reviews++
				switch cat {
				case "approve":
					a.approvals++
				case "approve_with_edits", "reject":
					a.rejections++
				}
			}
		}
	}

	rows := make([]reviewOutcomesRow, 0, len(order))
	for _, tier := range order {
		a := byTier[tier]
		row := reviewOutcomesRow{
			Tier:       tier,
			Harness:    a.harness,
			Model:      a.model,
			Reviews:    a.reviews,
			Approvals:  a.approvals,
			Rejections: a.rejections,
		}
		if a.reviews > 0 {
			row.ApprovalRate = float64(a.approvals) / float64(a.reviews)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Tier < rows[j].Tier })
	return rows, nil
}

// allBeadEventsFromExtra returns every BeadEvent stored on a bead, regardless
// of kind. The route-status command has a routing-only extractor; here we
// need both routing and review events so we walk Extra["events"] directly.
func allBeadEventsFromExtra(extra map[string]any) []bead.BeadEvent {
	if extra == nil {
		return nil
	}
	raw, ok := extra["events"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]bead.BeadEvent, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		e := bead.BeadEvent{}
		if v, ok := m["kind"].(string); ok {
			e.Kind = v
		}
		if v, ok := m["summary"].(string); ok {
			e.Summary = v
		}
		if v, ok := m["body"].(string); ok {
			e.Body = v
		}
		if v, ok := m["actor"].(string); ok {
			e.Actor = v
		}
		if v, ok := m["source"].(string); ok {
			e.Source = v
		}
		if v, ok := m["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				e.CreatedAt = t
			}
		}
		out = append(out, e)
	}
	return out
}

// parseRoutingHarnessModel extracts (provider, model) from a kind:routing
// event body. Returns ok=false when the body cannot be parsed or carries no
// provider, since a tier without a provider is not a useful attribution.
func parseRoutingHarnessModel(e bead.BeadEvent) (string, string, bool) {
	if e.Body == "" {
		return "", "", false
	}
	var body struct {
		ResolvedProvider string `json:"resolved_provider"`
		ResolvedModel    string `json:"resolved_model"`
	}
	if err := json.Unmarshal([]byte(e.Body), &body); err != nil {
		return "", "", false
	}
	if body.ResolvedProvider == "" {
		return "", "", false
	}
	return body.ResolvedProvider, body.ResolvedModel, true
}

func renderReviewOutcomesTable(cmd *cobra.Command, rows []reviewOutcomesRow) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-40s  %7s  %9s  %10s  %13s\n",
		"TIER", "REVIEWS", "APPROVALS", "REJECTIONS", "APPROVAL_RATE")
	for _, r := range rows {
		fmt.Fprintf(out, "%-40s  %7d  %9d  %10d  %13s\n",
			truncateTier(r.Tier, 40),
			r.Reviews,
			r.Approvals,
			r.Rejections,
			fmt.Sprintf("%.3f", r.ApprovalRate),
		)
	}
	return nil
}

// costEfficiencyRow is one row of the cost-efficiency report — total spend
// to close a single bead, with successful vs wasted (failed-attempt) cost
// broken out so escalation overhead is visible.
type costEfficiencyRow struct {
	BeadID            string  `json:"bead_id"`
	TotalAttempts     int     `json:"total_attempts"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	SuccessfulCostUSD float64 `json:"successful_cost_usd"`
	WastedCostUSD     float64 `json:"wasted_cost_usd"`
	FinalTier         string  `json:"final_tier"`
	FinalHarness      string  `json:"final_harness"`
}

func (f *CommandFactory) newAgentMetricsCostEfficiencyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost-efficiency",
		Short: "Report total cost to close each bead, including failed escalation attempts",
		Long: `Scan .ddx/executions/*/result.json and aggregate cost per bead across
all attempts. The successful_cost_usd column sums attempts where outcome ==
"task_succeeded"; wasted_cost_usd sums attempts where outcome != "task_succeeded"
(failed runs that still consumed budget). final_tier and final_harness reflect
the most recent attempt for each bead, which is the tier that ultimately
closed it (or the last tier tried if it remains open).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lastN, _ := cmd.Flags().GetInt("last")
			jsonOut, _ := cmd.Flags().GetBool("json")

			rows, err := computeCostEfficiency(f.WorkingDir, lastN)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return renderCostEfficiencyTable(cmd, rows)
		},
	}
	cmd.Flags().Int("last", 0, "Limit to beads touched in the most recent N executions (0 = all)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

// computeCostEfficiency scans .ddx/executions/*/result.json under workingDir
// and aggregates per-bead cost totals. When lastN > 0, the set of beads in
// the output is restricted to those with at least one attempt in the most
// recent N executions; for those beads, *all* historical attempts contribute
// to the totals so escalation chains are not artificially truncated.
func computeCostEfficiency(workingDir string, lastN int) ([]costEfficiencyRow, error) {
	execRoot := filepath.Join(workingDir, ".ddx", "executions")
	entries, err := os.ReadDir(execRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []costEfficiencyRow{}, nil
		}
		return nil, fmt.Errorf("read executions dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	type loadedAttempt struct {
		beadID  string
		harness string
		model   string
		outcome string
		costUSD float64
	}
	loaded := make([]loadedAttempt, 0, len(names))
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
		if res.BeadID == "" {
			continue
		}
		loaded = append(loaded, loadedAttempt{
			beadID:  res.BeadID,
			harness: res.Harness,
			model:   res.Model,
			outcome: res.Outcome,
			costUSD: res.CostUSD,
		})
	}

	// --last N: restrict the output set to beads touched by the most recent
	// N usable attempts. All historical attempts for those beads still
	// contribute to totals so escalation cost is visible.
	var includeBead map[string]bool
	if lastN > 0 && len(loaded) > 0 {
		includeBead = map[string]bool{}
		start := len(loaded) - lastN
		if start < 0 {
			start = 0
		}
		for _, a := range loaded[start:] {
			includeBead[a.beadID] = true
		}
	}

	type agg struct {
		beadID            string
		totalAttempts     int
		totalCostUSD      float64
		successfulCostUSD float64
		wastedCostUSD     float64
		finalHarness      string
		finalModel        string
	}
	byBead := map[string]*agg{}
	order := []string{}

	for _, a := range loaded {
		if includeBead != nil && !includeBead[a.beadID] {
			continue
		}
		g, ok := byBead[a.beadID]
		if !ok {
			g = &agg{beadID: a.beadID}
			byBead[a.beadID] = g
			order = append(order, a.beadID)
		}
		g.totalAttempts++
		g.totalCostUSD += a.costUSD
		if a.outcome == "task_succeeded" {
			g.successfulCostUSD += a.costUSD
		} else {
			g.wastedCostUSD += a.costUSD
		}
		// Attempts iterate in chronological order, so the last assignment
		// wins — that is the most recent tier tried on this bead.
		g.finalHarness = a.harness
		g.finalModel = a.model
	}

	rows := make([]costEfficiencyRow, 0, len(order))
	for _, id := range order {
		g := byBead[id]
		rows = append(rows, costEfficiencyRow{
			BeadID:            g.beadID,
			TotalAttempts:     g.totalAttempts,
			TotalCostUSD:      g.totalCostUSD,
			SuccessfulCostUSD: g.successfulCostUSD,
			WastedCostUSD:     g.wastedCostUSD,
			FinalTier:         tierKey(g.finalHarness, g.finalModel),
			FinalHarness:      g.finalHarness,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].BeadID < rows[j].BeadID })
	return rows, nil
}

func renderCostEfficiencyTable(cmd *cobra.Command, rows []costEfficiencyRow) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-24s  %14s  %14s  %18s  %14s  %-30s  %s\n",
		"BEAD_ID", "TOTAL_ATTEMPTS", "TOTAL_COST_USD", "SUCCESSFUL_COST_USD", "WASTED_COST_USD", "FINAL_TIER", "FINAL_HARNESS")
	for _, r := range rows {
		fmt.Fprintf(out, "%-24s  %14d  %14s  %18s  %14s  %-30s  %s\n",
			r.BeadID,
			r.TotalAttempts,
			fmt.Sprintf("%.4f", r.TotalCostUSD),
			fmt.Sprintf("%.4f", r.SuccessfulCostUSD),
			fmt.Sprintf("%.4f", r.WastedCostUSD),
			truncateTier(r.FinalTier, 30),
			r.FinalHarness,
		)
	}
	return nil
}
