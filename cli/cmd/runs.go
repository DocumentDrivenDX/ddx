package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func (f *CommandFactory) newRunsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Cross-layer run evidence introspection",
		Long:  "Inspect run evidence across execution layers (log, history, metrics).",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cmd.AddCommand(f.newRunsLogCommand())
	cmd.AddCommand(f.newRunsHistoryCommand())
	return cmd
}

func (f *CommandFactory) newRunsLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show DDX asset history",
		Long:  "Show run and commit history for DDX assets, equivalent to 'ddx log'.",
		RunE:  f.runLog,
	}

	cmd.Flags().IntP("number", "n", 20, "Number of commits to show")
	cmd.Flags().Int("limit", 20, "Limit number of commits to show (same as --number)")
	cmd.Flags().Bool("oneline", false, "Show compact one-line format")
	cmd.Flags().Bool("diff", false, "Show changes in each commit")
	cmd.Flags().String("export", "", "Export history to file (format: .md, .json, .csv, .html)")
	cmd.Flags().String("since", "", "Show commits since date (e.g., '1 week ago')")
	cmd.Flags().String("author", "", "Filter by author")
	cmd.Flags().String("grep", "", "Filter by commit message")
	return cmd
}

func (f *CommandFactory) newRunsHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect historical execution runs",
		Long:  "Show historical runs from the executions store, equivalent to 'ddx exec history'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactID, _ := cmd.Flags().GetString("artifact")
			definitionID, _ := cmd.Flags().GetString("definition")
			records, err := f.execStore().History(artifactID, definitionID)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(records)
			}
			for _, rec := range records {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %d\n", rec.RunID, rec.DefinitionID, rec.Status, rec.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().String("artifact", "", "Filter by artifact ID")
	cmd.Flags().String("definition", "", "Filter by definition ID")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}
