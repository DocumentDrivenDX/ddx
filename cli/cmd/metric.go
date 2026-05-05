package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/metric"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newMetricCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metric",
		Short: "Inspect and run metric artifacts",
		Long:  "Manage DDx metric artifacts, run definitions, and observation history through ddx exec.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newMetricValidateCommand())
	cmd.AddCommand(f.newMetricListCommand())
	cmd.AddCommand(f.newMetricShowCommand())
	cmd.AddCommand(f.newMetricRunCommand())
	cmd.AddCommand(f.newMetricCompareCommand())
	cmd.AddCommand(f.newMetricHistoryCommand())
	cmd.AddCommand(f.newMetricTrendCommand())

	return cmd
}

func (f *CommandFactory) metricStore() *metric.Store {
	return metric.NewStore(f.WorkingDir)
}

func (f *CommandFactory) metricHistory(metricID string) ([]metric.HistoryRecord, error) {
	return f.metricStore().History(metricID)
}

func (f *CommandFactory) newMetricValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <metric-id>",
		Short: "Validate a metric artifact and runtime definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, _, err := f.metricStore().Validate(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"metric_id":     args[0],
					"definition_id": def.DefinitionID,
					"status":        "ok",
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s validated with %s\n", args[0], def.DefinitionID)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List metric artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			docs, err := f.metricStore().ListArtifacts()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(docs)
			}
			for _, doc := range docs {
				if doc.Title != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", doc.ID, doc.Title)
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), doc.ID)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <metric-id>",
		Short: "Show one metric artifact and recent history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			def, doc, err := f.metricStore().Validate(args[0])
			if err != nil {
				return err
			}
			history, err := f.metricHistory(args[0])
			if err != nil {
				return err
			}
			recent := history
			if len(recent) > 5 {
				recent = recent[len(recent)-5:]
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"artifact":       doc,
					"definition":     def,
					"recent_history": recent,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ID:        %s\n", doc.ID)
			if doc.Title != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Title:     %s\n", doc.Title)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Path:      %s\n", doc.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "Definition: %s\n", def.DefinitionID)
			fmt.Fprintf(cmd.OutOrStdout(), "Command:   %s\n", joinCommand(def.Command))
			fmt.Fprintf(cmd.OutOrStdout(), "Created:   %s\n", def.CreatedAt.Format(time.RFC3339))
			fmt.Fprintln(cmd.OutOrStdout(), "Recent history:")
			if len(recent) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				return nil
			}
			for _, rec := range recent {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  %.3f%s  %s\n", rec.ObservedAt.Format(time.RFC3339), rec.RunID, rec.Value, rec.Unit, rec.Status)
			}
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
			rec, err := f.metricStore().Run(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rec)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %.3f%s  %s\n", rec.ObservedAt.Format(time.RFC3339), rec.Status, rec.Value, rec.Unit, rec.RunID)
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
			rec, result, err := f.metricStore().Compare(args[0], against)
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
			records, err := f.metricStore().History(args[0])
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
			summary, err := f.metricStore().Trend(args[0])
			if err != nil {
				return err
			}
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

func joinCommand(parts []string) string {
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " ")
}
