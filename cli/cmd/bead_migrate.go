package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) beadMigrator(s *bead.Store) (bead.Migrator, error) {
	return bead.NewMigrator(bead.MigratorOptions{Dir: s.Dir})
}

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
			mig, err := f.beadMigrator(s)
			if err != nil {
				return err
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			applyLifecycle, _ := cmd.Flags().GetBool("apply")
			asJSON, _ := cmd.Flags().GetBool("json")
			lifecycle, _ := cmd.Flags().GetBool("lifecycle")
			target, _ := cmd.Flags().GetString("to")
			if applyLifecycle && !lifecycle {
				return fmt.Errorf("bead: --apply is only supported with --lifecycle")
			}
			if applyLifecycle && dryRun {
				return fmt.Errorf("bead: --apply and --dry-run are mutually exclusive")
			}

			if target != "" {
				if lifecycle {
					return fmt.Errorf("bead: --lifecycle cannot be combined with --to")
				}
				if target != "axon" {
					return fmt.Errorf("bead: unknown migration target %q (supported: axon)", target)
				}
				if dryRun {
					return fmt.Errorf("bead: --dry-run is not supported with --to axon")
				}
				ax, err := mig.MigrateToAxon(cmd.Context())
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

			if lifecycle {
				var (
					st  bead.LifecycleMigrationStats
					err error
				)
				if applyLifecycle {
					err = f.withBeadTrackerWriteLock(func() error {
						st, err = mig.MigrateLifecycle(cmd.Context())
						if err != nil {
							return err
						}
						if st.Changed() {
							paths := []string{s.File}
							if st.MarkerWritten {
								paths = append(paths, s.LifecycleSchemaMarkerPath())
							}
							if _, err := f.beadAutoCommitPaths("migrate lifecycle", paths); err != nil {
								return err
							}
						}
						return nil
					})
				} else {
					st, err = mig.MigrateLifecycleDryRun(cmd.Context())
				}
				if err != nil {
					return err
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(st)
				}
				out := cmd.OutOrStdout()
				if !applyLifecycle {
					fmt.Fprintln(out, "Dry run — no files were modified.")
				}
				fmt.Fprintf(out, "needs_human labels:                  %d\n", st.LegacyNeedsHumanLabels)
				fmt.Fprintf(out, "triage:needs-investigation labels:   %d\n", st.LegacyNeedsInvestigationLabels)
				fmt.Fprintf(out, "needs_investigation pseudo-statuses: %d\n", st.LegacyNeedsInvestigationPseudoStatuses)
				fmt.Fprintf(out, "legacy no_changes metadata rows:     %d\n", st.LegacyNoChangesMetadataRows)
				fmt.Fprintf(out, "to proposed:                         %d\n", st.ToProposed)
				fmt.Fprintf(out, "to open:                             %d\n", st.ToOpen)
				fmt.Fprintf(out, "to blocked:                          %d\n", st.ToBlocked)
				fmt.Fprintf(out, "to closed:                           %d\n", st.ToClosed)
				fmt.Fprintf(out, "to cancelled:                        %d\n", st.ToCancelled)
				fmt.Fprintf(out, "schema marker missing:               %t\n", st.SchemaMarkerMissing)
				if applyLifecycle {
					fmt.Fprintf(out, "rows changed:                        %d\n", st.RowsChanged)
					fmt.Fprintf(out, "marker written:                      %t\n", st.MarkerWritten)
					if !st.Changed() {
						fmt.Fprintln(out, "No changes — lifecycle migration already applied.")
					}
				}
				return nil
			}

			var (
				ext  int
				arch int
			)
			if dryRun {
				st, err := mig.MigrateDryRun(cmd.Context())
				if err != nil {
					return err
				}
				ext, arch = st.EventsExternalized, st.Archived
			} else {
				if err := f.withBeadTrackerWriteLock(func() error {
					st, err := s.Migrate()
					if err != nil {
						return err
					}
					ext, arch = st.EventsExternalized, st.Archived
					if st.Changed() {
						if _, err := f.beadExternalizeArchiveAutoCommit(st); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return err
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
	cmd.Flags().Bool("lifecycle", false, "Migrate legacy lifecycle labels and pseudo-statuses to status-owned state")
	cmd.Flags().Bool("apply", false, "Apply --lifecycle migration (default with --lifecycle is dry-run)")
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
