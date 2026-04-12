package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	ddxexec "github.com/DocumentDrivenDX/ddx/internal/exec"
	"github.com/DocumentDrivenDX/ddx/internal/metric"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newMetricCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metric",
		Short: "Validate and run metric definitions",
		Long:  "Manage DDx metric artifacts and observation history through ddx exec.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newMetricValidateCommand())
	cmd.AddCommand(f.newMetricRunCommand())
	cmd.AddCommand(f.newMetricCompareCommand())
	cmd.AddCommand(f.newMetricHistoryCommand())
	cmd.AddCommand(f.newMetricTrendCommand())

	return cmd
}

func (f *CommandFactory) resolveMetricDefinition(metricID string) (*ddxexec.Definition, error) {
	defs, err := f.execStore().ListDefinitions(metricID)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("metric definition for %q not found", metricID)
	}
	def, doc, err := f.execStore().Validate(defs[0].ID)
	if err != nil {
		return nil, err
	}
	// The doc is only needed to ensure the artifact exists; the command path
	// keeps using the metric artifact ID for projection output.
	_ = doc
	return def, nil
}

func (f *CommandFactory) metricHistory(metricID string) ([]metric.HistoryRecord, error) {
	records, err := f.execStore().History(metricID, "")
	if err != nil {
		return nil, err
	}
	history := make([]metric.HistoryRecord, 0, len(records))
	for _, rec := range records {
		history = append(history, metricRecordFromExec(metricID, rec))
	}
	return history, nil
}

func (f *CommandFactory) newMetricValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <metric-id>",
		Short: "Validate a metric artifact and runtime definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := f.resolveMetricDefinition(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"metric_id":     args[0],
					"definition_id": def.ID,
					"status":        "ok",
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s validated with %s\n", args[0], def.ID)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <metric-id>",
		Short: "Execute a metric definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, err := f.resolveMetricDefinition(args[0])
			if err != nil {
				return err
			}
			rec, err := f.execStore().Run(cmd.Context(), def.ID)
			if err != nil {
				return err
			}
			out := metricRecordFromExec(args[0], rec)
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %.3f%s  %s\n", out.ObservedAt.Format(time.RFC3339), out.Status, out.Value, out.Unit, out.RunID)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricCompareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <metric-id>",
		Short: "Compare the latest metric run to a target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			against, _ := cmd.Flags().GetString("against")
			history, err := f.metricHistory(args[0])
			if err != nil {
				return err
			}
			rec, result, err := compareMetricHistory(history, against)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"record":     rec,
					"comparison": result,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  baseline=%.3f  delta=%.3f  %s\n", rec.RunID, result.Baseline, result.Delta, result.Direction)
			return nil
		},
	}
	cmd.Flags().String("against", "latest", "Compare against baseline, latest, or a run ID")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <metric-id>",
		Short: "Show metric observation history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			records, err := f.metricHistory(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(records)
			}
			for _, rec := range records {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %.3f%s  %s\n", rec.ObservedAt.Format(time.RFC3339), rec.RunID, rec.Value, rec.Unit, rec.Status)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricTrendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trend <metric-id>",
		Short: "Summarize metric observations over time",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			history, err := f.metricHistory(args[0])
			if err != nil {
				return err
			}
			summary := trendMetricHistory(args[0], history)
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(summary)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  count=%d  avg=%.3f%s  min=%.3f  max=%.3f\n", summary.MetricID, summary.Count, summary.Average, summary.Unit, summary.Min, summary.Max)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func metricRecordFromExec(metricID string, rec ddxexec.RunRecord) metric.HistoryRecord {
	out := metric.HistoryRecord{
		RunID:        rec.RunID,
		MetricID:     metricID,
		DefinitionID: rec.DefinitionID,
		ObservedAt:   rec.StartedAt,
		Status:       metric.StatusPass,
		ExitCode:     rec.ExitCode,
		DurationMS:   rec.FinishedAt.Sub(rec.StartedAt).Milliseconds(),
		Stdout:       rec.Result.Stdout,
		Stderr:       rec.Result.Stderr,
		ArtifactID:   metricID,
	}
	if rec.Status != ddxexec.StatusSuccess {
		out.Status = metric.StatusFail
	}
	if rec.Result.Metric != nil {
		out.Value = rec.Result.Metric.Value
		out.Unit = rec.Result.Metric.Unit
		out.Comparison = metric.ComparisonResult{
			Baseline:  rec.Result.Metric.Comparison.Baseline,
			Delta:     rec.Result.Metric.Comparison.Delta,
			Direction: rec.Result.Metric.Comparison.Direction,
		}
	}
	return out
}

func compareMetricHistory(history []metric.HistoryRecord, against string) (metric.HistoryRecord, metric.ComparisonResult, error) {
	if len(history) == 0 {
		return metric.HistoryRecord{}, metric.ComparisonResult{}, fmt.Errorf("no history for metric")
	}
	current := history[len(history)-1]
	target, err := selectMetricComparisonTarget(history, against)
	if err != nil {
		return metric.HistoryRecord{}, metric.ComparisonResult{}, err
	}
	direction := current.Comparison.Direction
	if direction == "" {
		direction = metric.ComparisonLowerIsBetter
	}
	result := metricComparisonFor(current.Value, target.Value, direction)
	current.Comparison = result
	return current, result, nil
}

func selectMetricComparisonTarget(history []metric.HistoryRecord, against string) (metric.HistoryRecord, error) {
	switch against {
	case "", "latest":
		return history[len(history)-1], nil
	case "baseline":
		return history[0], nil
	default:
		for _, rec := range history {
			if rec.RunID == against {
				return rec, nil
			}
		}
		return metric.HistoryRecord{}, fmt.Errorf("comparison target %q not found", against)
	}
}

func trendMetricHistory(metricID string, history []metric.HistoryRecord) metric.TrendSummary {
	if len(history) == 0 {
		return metric.TrendSummary{MetricID: metricID}
	}
	summary := metric.TrendSummary{
		MetricID:  metricID,
		Min:       history[0].Value,
		Max:       history[0].Value,
		Latest:    history[len(history)-1].Value,
		Unit:      history[len(history)-1].Unit,
		UpdatedAt: history[len(history)-1].ObservedAt,
	}
	var sum float64
	for _, rec := range history {
		if rec.Value < summary.Min {
			summary.Min = rec.Value
		}
		if rec.Value > summary.Max {
			summary.Max = rec.Value
		}
		sum += rec.Value
		summary.UpdatedAt = rec.ObservedAt
		if rec.Unit != "" {
			summary.Unit = rec.Unit
		}
	}
	summary.Count = len(history)
	summary.Average = sum / float64(len(history))
	return summary
}

func metricComparisonFor(current, baseline float64, direction string) metric.ComparisonResult {
	delta := current - baseline
	if direction == metric.ComparisonHigherIsBetter {
		delta = baseline - current
	}
	return metric.ComparisonResult{
		Baseline:  baseline,
		Delta:     delta,
		Direction: direction,
	}
}
