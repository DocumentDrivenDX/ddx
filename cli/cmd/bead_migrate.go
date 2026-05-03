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
and 'ddx bead status' transparently consult the archive partner.

With --to axon, the command instead copies the entire JSONL corpus
(.ddx/beads.jsonl + .ddx/beads-archive.jsonl, including externalized
events) losslessly into the axon backend under .ddx/axon/. The source
JSONL files are not modified — the operator removes them after verifying
the migration via 'ddx bead export | diff'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			asJSON, _ := cmd.Flags().GetBool("json")
			target, _ := cmd.Flags().GetString("to")

			if target != "" {
				if target != "axon" {
					return fmt.Errorf("bead: unknown migration target %q (supported: axon)", target)
				}
				if dryRun {
					return fmt.Errorf("bead: --dry-run is not supported with --to axon")
				}
				ax, err := s.MigrateToAxon()
				if err != nil {
					return err
				}
				view := migrateAxonStatsView{
					BeadsMigrated:  ax.BeadsMigrated,
					EventsMigrated: ax.EventsMigrated,
					To:             "axon",
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(view)
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Migrated %d bead(s) (%d inline event(s)) to axon backend at .ddx/axon/\n", view.BeadsMigrated, view.EventsMigrated)
				fmt.Fprintln(out, "Source files left intact — remove .ddx/beads.jsonl and .ddx/beads-archive.jsonl after verifying.")
				return nil
			}

			var (
				ext  int
				arch int
			)
			if dryRun {
				st, err := s.MigrateDryRun()
				if err != nil {
					return err
				}
				ext, arch = st.EventsExternalized, st.Archived
			} else {
				st, err := s.Migrate()
				if err != nil {
					return err
				}
				ext, arch = st.EventsExternalized, st.Archived
				if st.Changed() {
					f.beadAutoCommit(fmt.Sprintf("migrate: externalize=%d archive=%d", ext, arch))
				}
			}
			stats := migrateStatsView{EventsExternalized: ext, Archived: arch, DryRun: dryRun}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			out := cmd.OutOrStdout()
			if dryRun {
				fmt.Fprintln(out, "Dry run — no files were modified.")
			}
			fmt.Fprintf(out, "Externalized events: %d\n", ext)
			fmt.Fprintf(out, "Archived beads:      %d\n", arch)
			if ext == 0 && arch == 0 {
				fmt.Fprintln(out, "No changes — already migrated.")
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	cmd.Flags().Bool("dry-run", false, "Report what would change without writing")
	cmd.Flags().String("to", "", "Migrate corpus to a different backend (supported: axon)")
	return cmd
}

type migrateStatsView struct {
	EventsExternalized int  `json:"EventsExternalized"`
	Archived           int  `json:"Archived"`
	DryRun             bool `json:"DryRun,omitempty"`
}

type migrateAxonStatsView struct {
	BeadsMigrated  int    `json:"BeadsMigrated"`
	EventsMigrated int    `json:"EventsMigrated"`
	To             string `json:"To"`
}
