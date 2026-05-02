package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func (f *CommandFactory) newBeadMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Split active beads.jsonl into beads + beads-archive + attachments",
		Long: `Split the existing .ddx/beads.jsonl into the modern layout:

  * Closed beads' inline events are moved to per-bead sidecars under
    .ddx/attachments/<id>/events.jsonl (ADR-004).
  * Eligible closed beads are moved to .ddx/beads-archive.jsonl using a
    permissive archival policy (TD-027).

The command is idempotent — a second run with no further changes is a
no-op. All bead IDs remain addressable: 'ddx bead show', 'ddx bead list',
and 'ddx bead status' transparently consult the archive partner.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			stats, err := s.Migrate()
			if err != nil {
				return err
			}
			if stats.Changed() {
				f.beadAutoCommit(fmt.Sprintf("migrate: externalize=%d archive=%d", stats.EventsExternalized, stats.Archived))
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Externalized events: %d\n", stats.EventsExternalized)
			fmt.Fprintf(cmd.OutOrStdout(), "Archived beads:      %d\n", stats.Archived)
			if !stats.Changed() {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes — already migrated.")
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
