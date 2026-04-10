package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/processmetrics"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show process metrics derived from beads and agent sessions",
		Long:  "Read-only process metrics over bead lifecycle facts and agent session usage.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newMetricsSummaryCommand())
	cmd.AddCommand(f.newMetricsCostCommand())
	cmd.AddCommand(f.newMetricsCycleTimeCommand())
	cmd.AddCommand(f.newMetricsReworkCommand())

	return cmd
}

func (f *CommandFactory) newMetricsSummaryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show a dashboard of process metrics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := metricsQueryFromFlags(cmd)
			if err != nil {
				return err
			}
			report, err := f.metricsService().Summary(query)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return encodeMetricsJSON(cmd, report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "beads=%d closed=%d reopened=%d sessions=%d cost=%.3f\n",
				report.Beads.Total,
				report.Beads.Closed,
				report.Beads.Reopened,
				report.Sessions.Total,
				report.Cost.KnownCostUSD+report.Cost.EstimatedCostUSD,
			)
			return nil
		},
	}
	cmd.Flags().String("since", "", "Optional cutoff window (today, Nd, RFC3339, or YYYY-MM-DD)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricsCostCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Show bead and feature cost attribution",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := metricsQueryFromFlags(cmd)
			if err != nil {
				return err
			}
			beadID, _ := cmd.Flags().GetString("bead")
			featureID, _ := cmd.Flags().GetString("feature")
			if beadID != "" && featureID != "" {
				return fmt.Errorf("use either --bead or --feature, not both")
			}
			query.BeadID = beadID
			query.FeatureID = featureID
			report, err := f.metricsService().Cost(query)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return encodeMetricsJSON(cmd, report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "scope=%s beads=%d features=%d cost=%.3f\n",
				report.Scope,
				len(report.Beads),
				len(report.Features),
				report.Summary.KnownCostUSD+report.Summary.EstimatedCostUSD,
			)
			for _, row := range report.Beads {
				if row.CostUSD == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  unknown\n", row.BeadID)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %.3f  tokens=%d\n", row.BeadID, row.CostState, *row.CostUSD, row.TotalTokens)
			}
			return nil
		},
	}
	cmd.Flags().String("bead", "", "Limit to one bead")
	cmd.Flags().String("feature", "", "Limit to one spec-id")
	cmd.Flags().String("since", "", "Optional cutoff window (today, Nd, RFC3339, or YYYY-MM-DD)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricsCycleTimeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cycle-time",
		Short: "Show bead cycle time facts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := metricsQueryFromFlags(cmd)
			if err != nil {
				return err
			}
			report, err := f.metricsService().CycleTime(query)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return encodeMetricsJSON(cmd, report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "known=%d unknown=%d\n", report.Summary.KnownCount, report.Summary.UnknownCount)
			for _, row := range report.Beads {
				if row.CycleTimeMS == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  unknown\n", row.BeadID)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %dms\n", row.BeadID, *row.CycleTimeMS)
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "Optional cutoff window (today, Nd, RFC3339, or YYYY-MM-DD)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) newMetricsReworkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rework",
		Short: "Show reopen and revision facts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query, err := metricsQueryFromFlags(cmd)
			if err != nil {
				return err
			}
			report, err := f.metricsService().Rework(query)
			if err != nil {
				return err
			}
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				return encodeMetricsJSON(cmd, report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "closed=%d reopened=%d rate=%.3f\n", report.Summary.KnownClosed, report.Summary.KnownReopened, report.Summary.ReopenRate)
			for _, row := range report.Beads {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  reopened=%t\n", row.BeadID, row.Reopened)
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "Optional cutoff window (today, Nd, RFC3339, or YYYY-MM-DD)")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func (f *CommandFactory) metricsService() *processmetrics.Service {
	return processmetrics.New(f.WorkingDir)
}

func metricsQueryFromFlags(cmd *cobra.Command) (processmetrics.Query, error) {
	rawSince, _ := cmd.Flags().GetString("since")
	since, err := processmetrics.ParseSince(rawSince)
	if err != nil {
		return processmetrics.Query{}, err
	}
	return processmetrics.Query{
		Since:    since,
		HasSince: rawSince != "",
	}, nil
}

func encodeMetricsJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
