package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadRecheckBlockersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recheck-blockers",
		Short: "Recheck cross-repo blockers and reopen cleared beads",
		Long: `Recheck blocked beads that carry a structured cross-repo blocker ref.

For each blocked bead, the command resolves the referenced repo through the
project's known-repos config, reads the target bead, and reopens the blocked
bead when the target is closed.

Use --id to limit the run to a single blocked bead. Use --json for a machine-
readable result list.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}

			cfg, err := config.LoadWithWorkingDir(workspaceRoot)
			if err != nil {
				return fmt.Errorf("load config for recheck blockers: %w", err)
			}

			beadID, _ := cmd.Flags().GetString("id")
			s := f.beadStoreConcrete()

			var results []bead.RecheckBlockerResult
			var reopened bool
			if err := f.withBeadTrackerWriteLock(func() error {
				var err error
				results, err = bead.RecheckBlockers(cmd.Context(), s, cfg.KnownRepos, beadID)
				if err != nil {
					return err
				}
				for _, row := range results {
					if row.Outcome == bead.RecheckBlockerOutcomeReopened {
						reopened = true
						break
					}
				}
				if reopened {
					if _, err := f.beadAutoCommit("recheck blockers"); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			out := cmd.OutOrStdout()
			if len(results) == 0 {
				fmt.Fprintln(out, "no recheckable blockers matched")
				return nil
			}

			var reopenedCount, blockedCount, manualCount, unresolvableCount int
			for _, row := range results {
				switch row.Outcome {
				case bead.RecheckBlockerOutcomeReopened:
					reopenedCount++
				case bead.RecheckBlockerOutcomeBlocked:
					blockedCount++
				case bead.RecheckBlockerOutcomeManual:
					manualCount++
				case bead.RecheckBlockerOutcomeUnresolvable:
					unresolvableCount++
				}
			}

			fmt.Fprintf(out, "recheck-blockers: %d reopened, %d blocked, %d manual, %d unresolvable\n",
				reopenedCount, blockedCount, manualCount, unresolvableCount)
			for _, row := range results {
				fmt.Fprintf(out, "  %s  status=%s  outcome=%s", row.BeadID, row.Status, row.Outcome)
				if row.Repo != "" && row.TargetBead != "" {
					fmt.Fprintf(out, "  ref=%s#%s", row.Repo, row.TargetBead)
				}
				if row.ObservedStatus != "" {
					fmt.Fprintf(out, "  observed=%s", row.ObservedStatus)
				}
				if row.Reason != "" {
					fmt.Fprintf(out, "  %s", row.Reason)
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}

	cmd.Flags().String("id", "", "Limit recheck to one bead ID")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
