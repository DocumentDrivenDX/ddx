package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/spf13/cobra"
)

var analyzeValidDims = []string{
	"model", "harness", "profile", "prompt_template_hash",
	"spec_id", "outcome", "bead_id", "day", "week", "month",
	"reviewer_model",
}

var analyzeAggHeaders = []string{
	"count", "success_rate", "avg_cost_usd", "sum_cost_usd",
	"p50_duration_ms", "p95_duration_ms", "avg_input_tokens",
	"avg_output_tokens", "review_block_rate", "escalation_rate",
	"reviewer_fp_rate", "reviewer_fn_rate",
}

// analyzeResultRow is one output row from ddx work analyze.
type analyzeResultRow struct {
	Dimensions      map[string]string `json:"dimensions"`
	Count           int               `json:"count"`
	SuccessRate     float64           `json:"success_rate"`
	AvgCostUSD      float64           `json:"avg_cost_usd"`
	SumCostUSD      float64           `json:"sum_cost_usd"`
	P50DurationMS   int               `json:"p50_duration_ms"`
	P95DurationMS   int               `json:"p95_duration_ms"`
	AvgInputTokens  float64           `json:"avg_input_tokens"`
	AvgOutputTokens float64           `json:"avg_output_tokens"`
	ReviewBlockRate float64           `json:"review_block_rate"`
	EscalationRate  float64           `json:"escalation_rate"`
	// ReviewerFPRate is the fraction of BLOCK verdicts that the operator
	// subsequently contradicted by closing the bead (false-positive rate).
	ReviewerFPRate float64 `json:"reviewer_fp_rate"`
	// ReviewerFNRate is the fraction of APPROVE verdicts that the operator
	// subsequently contradicted by reopening the bead (false-negative rate).
	ReviewerFNRate float64 `json:"reviewer_fn_rate"`
}

type analyzeAggBucket struct {
	durations    []int
	count        int
	success      int
	sumCost      float64
	sumIn        int
	sumOut       int
	blocked      int
	escalated    int
	blockTotal   int // total rows with BLOCK verdict (denominator for FP rate)
	approveTotal int // total rows with APPROVE verdict (denominator for FN rate)
	fpCount      int // BLOCK + accuracy_signal=override (false positives)
	fnCount      int // APPROVE + accuracy_signal=override (false negatives)
}

func (f *CommandFactory) newWorkAnalyzeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Query cross-attempt performance from .ddx/metrics/attempts.jsonl",
		Long: `Aggregate and analyze attempt data from .ddx/metrics/attempts.jsonl.

Group rows by one or more dimensions (--by) and compute aggregated metrics.
Filter with --since (time window) or --where (field equality).

Supported dimensions: model, harness, profile, prompt_template_hash, spec_id,
outcome, bead_id, day, week, month.

Examples:
  ddx work analyze --by model
  ddx work analyze --by model,outcome
  ddx work analyze --by spec_id --since 7d
  ddx work analyze --where model=claude-sonnet-4-6 --by prompt_template_hash`,
		Args: cobra.NoArgs,
		RunE: f.runWorkAnalyze,
	}
	cmd.Flags().StringSlice("by", nil, "Dimensions to group by (comma-separated): "+strings.Join(analyzeValidDims, ", "))
	cmd.Flags().String("since", "", "Time window: e.g. 7d, 2w, 1m (days/weeks/months)")
	cmd.Flags().StringArray("where", nil, "Filter by key=value; may be repeated")
	cmd.Flags().Bool("json", false, "Emit JSONL output")
	cmd.Flags().Bool("csv", false, "Emit CSV output")
	cmd.Flags().String("project", "", "Target project root (default: CWD git root)")
	return cmd
}

func (f *CommandFactory) runWorkAnalyze(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

	byDims, _ := cmd.Flags().GetStringSlice("by")
	sinceStr, _ := cmd.Flags().GetString("since")
	whereFilters, _ := cmd.Flags().GetStringArray("where")
	asJSON, _ := cmd.Flags().GetBool("json")
	asCSV, _ := cmd.Flags().GetBool("csv")

	for _, d := range byDims {
		if !analyzeDimValid(d) {
			return fmt.Errorf("unsupported dimension %q; valid: %s", d, strings.Join(analyzeValidDims, ", "))
		}
	}

	var cutoff time.Time
	if sinceStr != "" {
		dur, err := analyzeParseSince(sinceStr)
		if err != nil {
			return err
		}
		cutoff = time.Now().UTC().Add(-dur)
	}

	filters := make(map[string]string)
	for _, wf := range whereFilters {
		parts := strings.SplitN(wf, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid --where %q: expected key=value", wf)
		}
		if !analyzeDimValid(parts[0]) {
			return fmt.Errorf("unsupported --where key %q; valid: %s", parts[0], strings.Join(analyzeValidDims, ", "))
		}
		filters[parts[0]] = parts[1]
	}

	rows, err := attemptmetrics.LoadRows(projectRoot)
	if err != nil {
		return fmt.Errorf("load attempts: %w", err)
	}

	aggs := make(map[string]*analyzeAggBucket)
	var keyOrder []string
	keyDimVals := make(map[string][]string)

	for _, r := range rows {
		if !cutoff.IsZero() && r.TSStart != "" {
			t, parseErr := time.Parse(time.RFC3339, r.TSStart)
			if parseErr == nil && t.Before(cutoff) {
				continue
			}
		}
		match := true
		for k, v := range filters {
			if analyzeExtractDim(r, k) != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}

		dimVals := make([]string, len(byDims))
		for i, d := range byDims {
			dimVals[i] = analyzeExtractDim(r, d)
		}
		key := strings.Join(dimVals, "\x00")
		if _, ok := aggs[key]; !ok {
			aggs[key] = &analyzeAggBucket{}
			keyOrder = append(keyOrder, key)
			keyDimVals[key] = dimVals
		}
		agg := aggs[key]
		agg.count++
		if r.Outcome == "task_succeeded" {
			agg.success++
		}
		agg.sumCost += r.CostUSD
		agg.durations = append(agg.durations, r.DurationMS)
		agg.sumIn += r.InputTokens
		agg.sumOut += r.OutputTokens
		if r.Outcome == "review_block" || r.ReviewVerdict == "BLOCK" {
			agg.blocked++
		}
		if r.LadderStepsTaken > 0 {
			agg.escalated++
		}
		// Reviewer accuracy signal aggregation.
		if r.ReviewVerdict == "BLOCK" {
			agg.blockTotal++
			if r.ReviewerAccuracySignal == "override" {
				agg.fpCount++
			}
		}
		if r.ReviewVerdict == "APPROVE" {
			agg.approveTotal++
			if r.ReviewerAccuracySignal == "override" {
				agg.fnCount++
			}
		}
	}

	results := make([]analyzeResultRow, 0, len(keyOrder))
	for _, key := range keyOrder {
		agg := aggs[key]
		dimVals := keyDimVals[key]
		dims := make(map[string]string, len(byDims))
		for i, d := range byDims {
			dims[d] = dimVals[i]
		}
		results = append(results, analyzeResultRow{
			Dimensions:      dims,
			Count:           agg.count,
			SuccessRate:     analyzeRatio(agg.success, agg.count),
			AvgCostUSD:      analyzeDiv(agg.sumCost, float64(agg.count)),
			SumCostUSD:      agg.sumCost,
			P50DurationMS:   analyzePercentile(agg.durations, 50),
			P95DurationMS:   analyzePercentile(agg.durations, 95),
			AvgInputTokens:  analyzeDiv(float64(agg.sumIn), float64(agg.count)),
			AvgOutputTokens: analyzeDiv(float64(agg.sumOut), float64(agg.count)),
			ReviewBlockRate: analyzeRatio(agg.blocked, agg.count),
			EscalationRate:  analyzeRatio(agg.escalated, agg.count),
			ReviewerFPRate:  analyzeRatio(agg.fpCount, agg.blockTotal),
			ReviewerFNRate:  analyzeRatio(agg.fnCount, agg.approveTotal),
		})
	}

	out := cmd.OutOrStdout()
	switch {
	case asJSON:
		return analyzeWriteJSONL(out, results)
	case asCSV:
		return analyzeWriteCSV(out, byDims, results)
	default:
		return analyzeWriteTable(out, byDims, results)
	}
}

func analyzeDimValid(d string) bool {
	for _, v := range analyzeValidDims {
		if d == v {
			return true
		}
	}
	return false
}

func analyzeExtractDim(r attemptmetrics.AttemptRow, dim string) string {
	switch dim {
	case "model":
		return r.Model
	case "harness":
		return r.Harness
	case "profile":
		return r.Profile
	case "prompt_template_hash":
		return r.PromptTemplateHash
	case "spec_id":
		return r.SpecID
	case "outcome":
		return r.Outcome
	case "bead_id":
		return r.BeadID
	case "reviewer_model":
		return r.ReviewerModel
	case "day":
		if r.TSStart == "" {
			return ""
		}
		t, err := time.Parse(time.RFC3339, r.TSStart)
		if err != nil {
			return ""
		}
		return t.UTC().Format("2006-01-02")
	case "week":
		if r.TSStart == "" {
			return ""
		}
		t, err := time.Parse(time.RFC3339, r.TSStart)
		if err != nil {
			return ""
		}
		y, w := t.UTC().ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	case "month":
		if r.TSStart == "" {
			return ""
		}
		t, err := time.Parse(time.RFC3339, r.TSStart)
		if err != nil {
			return ""
		}
		return t.UTC().Format("2006-01")
	}
	return ""
}

func analyzeParseSince(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid --since %q: use e.g. 7d, 2w, 1m", s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid --since %q: expected positive integer followed by d/w/m", s)
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid --since suffix %q: use d (days), w (weeks), or m (months)", string(s[len(s)-1]))
	}
}

func analyzeRatio(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}

func analyzeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func analyzePercentile(vals []int, p int) int {
	if len(vals) == 0 {
		return 0
	}
	s := make([]int, len(vals))
	copy(s, vals)
	sort.Ints(s)
	idx := p * (len(s) - 1) / 100
	return s[idx]
}

func analyzeAggValues(r analyzeResultRow) []string {
	return []string{
		strconv.Itoa(r.Count),
		fmt.Sprintf("%.4f", r.SuccessRate),
		fmt.Sprintf("%.6f", r.AvgCostUSD),
		fmt.Sprintf("%.6f", r.SumCostUSD),
		strconv.Itoa(r.P50DurationMS),
		strconv.Itoa(r.P95DurationMS),
		fmt.Sprintf("%.1f", r.AvgInputTokens),
		fmt.Sprintf("%.1f", r.AvgOutputTokens),
		fmt.Sprintf("%.4f", r.ReviewBlockRate),
		fmt.Sprintf("%.4f", r.EscalationRate),
		fmt.Sprintf("%.4f", r.ReviewerFPRate),
		fmt.Sprintf("%.4f", r.ReviewerFNRate),
	}
}

func analyzeWriteJSONL(w io.Writer, rows []analyzeResultRow) error {
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func analyzeWriteCSV(w io.Writer, dims []string, rows []analyzeResultRow) error {
	cw := csv.NewWriter(w)
	headers := make([]string, 0, len(dims)+len(analyzeAggHeaders))
	headers = append(headers, dims...)
	headers = append(headers, analyzeAggHeaders...)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, r := range rows {
		row := make([]string, 0, len(headers))
		for _, d := range dims {
			row = append(row, r.Dimensions[d])
		}
		row = append(row, analyzeAggValues(r)...)
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func analyzeWriteTable(w io.Writer, dims []string, rows []analyzeResultRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	headers := make([]string, 0, len(dims)+len(analyzeAggHeaders))
	headers = append(headers, dims...)
	headers = append(headers, analyzeAggHeaders...)
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		vals := make([]string, 0, len(headers))
		for _, d := range dims {
			v := r.Dimensions[d]
			if v == "" {
				v = "(none)"
			}
			vals = append(vals, v)
		}
		vals = append(vals, analyzeAggValues(r)...)
		_, _ = fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
}
