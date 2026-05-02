package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

// DefaultArchiveSizeThreshold is the default size of .ddx/beads.jsonl above
// which `ddx bead archive` will move closed beads into the archive partner.
// Per ADR-004 step 4 the threshold is 4MB.
const DefaultArchiveSizeThreshold int64 = 4 * 1024 * 1024

func (f *CommandFactory) newBeadArchiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive closed beads when active beads.jsonl exceeds size threshold",
		Long: `Move closed beads from .ddx/beads.jsonl into .ddx/beads-archive.jsonl.

By default this command archives closed beads only, and only when the active
beads.jsonl is larger than ` + fmt.Sprintf("%d", DefaultArchiveSizeThreshold) + ` bytes (4MB). Each archived bead's inline
events are externalized into its attachment sidecar at
.ddx/attachments/<id>/events.jsonl.

Flags override the trigger:
  --max-size N      change the size threshold (use 0 to disable the gate)
  --older-than D    only archive beads whose last update is older than D
  --max-count N     cap the number of beads moved in this run

The operation is crash-safe: archive-partner row writes precede active-row
removal, and merged-view reads in 'ddx bead show' / 'ddx bead list' hide any
duplicate that an interrupted run could leave behind.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()

			threshold := DefaultArchiveSizeThreshold
			if cmd.Flags().Changed("max-size") {
				v, _ := cmd.Flags().GetInt64("max-size")
				threshold = v
			}

			info, statErr := os.Stat(s.File)
			activeSize := int64(0)
			if statErr == nil {
				activeSize = info.Size()
			}

			asJSON, _ := cmd.Flags().GetBool("json")

			if threshold > 0 && activeSize <= threshold {
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"EventsExternalized": 0,
						"Archived":           0,
						"ActiveSizeBefore":   activeSize,
						"ActiveSizeAfter":    activeSize,
						"Threshold":          threshold,
						"Skipped":            true,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"Active beads.jsonl is %d bytes (≤ threshold %d); nothing to archive.\n",
					activeSize, threshold)
				return nil
			}

			policy := bead.ArchivePolicy{
				Statuses: []string{bead.StatusClosed},
			}
			if cmd.Flags().Changed("older-than") {
				d, _ := cmd.Flags().GetDuration("older-than")
				policy.MinAge = d
			}
			if cmd.Flags().Changed("max-count") {
				n, _ := cmd.Flags().GetInt("max-count")
				policy.BatchSize = n
			}

			stats, err := s.ArchiveWithEvents(policy)
			if err != nil {
				return err
			}

			afterSize := int64(0)
			if info, err := os.Stat(s.File); err == nil {
				afterSize = info.Size()
			}

			if stats.Changed() {
				f.beadAutoCommit(fmt.Sprintf("archive: externalize=%d archive=%d", stats.EventsExternalized, stats.Archived))
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"EventsExternalized": stats.EventsExternalized,
					"Archived":           stats.Archived,
					"ActiveSizeBefore":   activeSize,
					"ActiveSizeAfter":    afterSize,
					"Threshold":          threshold,
					"Skipped":            false,
				})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Externalized events: %d\n", stats.EventsExternalized)
			fmt.Fprintf(out, "Archived beads:      %d\n", stats.Archived)
			fmt.Fprintf(out, "Active size:         %d -> %d bytes (threshold %d)\n", activeSize, afterSize, threshold)
			if !stats.Changed() {
				fmt.Fprintln(out, "No eligible closed beads to archive.")
			}
			return nil
		},
	}
	cmd.Flags().Int64("max-size", DefaultArchiveSizeThreshold, "Size threshold in bytes for active beads.jsonl; archive only runs when the file exceeds this (0 disables the gate)")
	cmd.Flags().Duration("older-than", 0, "Only archive closed beads whose last update is older than this duration (e.g. 720h)")
	cmd.Flags().Int("max-count", 0, "Maximum number of beads to archive in this run (0 = unlimited)")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
