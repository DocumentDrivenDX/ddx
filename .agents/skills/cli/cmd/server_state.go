package cmd

import (
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

// newServerStateCommand groups the ddx-server state file utilities. See
// ddx-15f7ee0b Fix C for context — the prune subcommand reclaims state files
// polluted by tests that leaked Go temp dirs into the registered-project list.
func (f *CommandFactory) newServerStateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect and repair the ddx-server persistent state file",
	}
	cmd.AddCommand(f.newServerStatePruneCommand())
	return cmd
}

func (f *CommandFactory) newServerStatePruneCommand() *cobra.Command {
	var dryRun bool
	var statePath string

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove phantom test-directory project entries from server-state.json",
		Long: `Prune drops project entries whose path looks like a Go test temp
directory (any path under /tmp/, /private/tmp/, /var/folders/, or any path
containing a /TestXxx<digits>/ segment). Before overwriting the state file,
the existing content is written to a timestamped ".bak-YYYYMMDDTHHMMSS"
sibling so the operation is recoverable.

Use --dry-run to preview the summary without modifying anything.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := statePath
			if path == "" {
				path = server.DefaultStateFilePath()
			}
			res, err := server.PruneStateFile(path, dryRun)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			prefix := ""
			if res.DryRun {
				prefix = "DRY-RUN: "
			}
			fmt.Fprintf(out, "%sPruned %d of %d entries (%d kept)\n", prefix, res.Dropped, res.Total, res.Kept)
			if !res.DryRun && res.BackupFile != "" {
				fmt.Fprintf(out, "Backup written to %s\n", res.BackupFile)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the summary without modifying the state file")
	cmd.Flags().StringVar(&statePath, "state", "", "Path to server-state.json (default: XDG_DATA_HOME/ddx/server-state.json)")
	return cmd
}
