package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) beadMigrator(s *bead.Store) (bead.Migrator, error) {
	if f.beadMigratorOverride != nil {
		return f.beadMigratorOverride(s)
	}
	return bead.NewMigrator(bead.MigratorOptions{Dir: s.Dir})
}

func (f *CommandFactory) newBeadMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate bead storage between JSONL lifecycle and Axon importer flows",
		Long: `Split the existing .ddx/beads.jsonl into the modern layout:

  * Closed beads' inline events are moved to per-bead sidecars under
    .ddx/attachments/<id>/events.jsonl (ADR-004).
  * Eligible closed beads are moved to .ddx/beads-archive.jsonl using a
    permissive archival policy (TD-027).

The command is idempotent — a second run with no further changes is a
no-op. All bead IDs remain addressable: 'ddx bead show', 'ddx bead list',
and 'ddx bead status' transparently consult the archive partner.

With --to-axon, the command imports the JSONL corpus into the Axon
backend using the importer path. Use --dry-run to inspect the counts,
--apply to write the corpus, --verify to read back and validate the
import, and --limit N to cap the number of beads imported.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			mig, err := f.beadMigrator(s)
			if err != nil {
				return err
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			applyFlag, _ := cmd.Flags().GetBool("apply")
			verifyAxon, _ := cmd.Flags().GetBool("verify")
			limit, _ := cmd.Flags().GetInt("limit")
			toAxon, _ := cmd.Flags().GetBool("to-axon")
			asJSON, _ := cmd.Flags().GetBool("json")
			lifecycle, _ := cmd.Flags().GetBool("lifecycle")
			if applyFlag && dryRun {
				return fmt.Errorf("bead: --apply and --dry-run are mutually exclusive")
			}

			if toAxon {
				if lifecycle {
					return fmt.Errorf("bead: --lifecycle cannot be combined with --to-axon")
				}
				if dryRun && verifyAxon {
					return fmt.Errorf("bead: --verify is not supported with --dry-run")
				}
				if limit < 0 {
					return fmt.Errorf("bead: --limit must be >= 0")
				}
				ax, err := mig.MigrateToAxon(cmd.Context(), bead.MigrateAxonOptions{
					DryRun:          dryRun,
					Verify:          verifyAxon,
					Limit:           limit,
					CopyAttachments: true,
				})
				if err != nil {
					return err
				}
				view := migrateAxonStatsView{
					MigrateAxonStats: ax,
					DryRun:           dryRun,
					Verify:           verifyAxon,
					Limit:            limit,
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(view)
				}
				out := cmd.OutOrStdout()
				if dryRun {
					fmt.Fprintln(out, "Dry run — no files were modified.")
				}
				fmt.Fprintf(out, "Imported %d bead(s) (%d inline event(s)) into axon backend\n", view.BeadsMigrated, view.EventsMigrated)
				if view.AttachmentsMigrated > 0 {
					fmt.Fprintf(out, "Copied attachments: %d\n", view.AttachmentsMigrated)
				}
				if verifyAxon {
					fmt.Fprintln(out, "Verification passed.")
				}
				return nil
			}

			if lifecycle {
				var (
					st  bead.LifecycleMigrationStats
					err error
				)
				if applyFlag {
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
				if !applyFlag {
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
				if applyFlag {
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
					st, err := s.Migrate(cmd.Context())
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
	cmd.Flags().Bool("to-axon", false, "Import the JSONL corpus into the Axon backend")
	cmd.Flags().Bool("verify", false, "Verify the imported Axon corpus after writing")
	cmd.Flags().Int("limit", 0, "Limit the number of beads imported into Axon (0 = all)")
	cmd.Flags().Bool("lifecycle", false, "Migrate legacy lifecycle labels and pseudo-statuses to status-owned state")
	cmd.Flags().Bool("apply", false, "Apply the selected migration mode")
	return cmd
}

type migrateStatsView struct {
	EventsExternalized int  `json:"EventsExternalized"`
	Archived           int  `json:"Archived"`
	DryRun             bool `json:"DryRun,omitempty"`
}

type migrateAxonStatsView struct {
	bead.MigrateAxonStats
	DryRun bool `json:"DryRun,omitempty"`
	Verify bool `json:"Verify,omitempty"`
	Limit  int  `json:"Limit,omitempty"`
}
