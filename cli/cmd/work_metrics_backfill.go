package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

// beadSpecID extracts the spec-id from a bead's Extra map, returning "" when
// absent or not a string.
func beadSpecID(b bead.Bead) string {
	raw, _ := b.Extra["spec-id"].(string)
	return strings.TrimSpace(raw)
}

// newWorkMetricsCommand returns the "ddx work metrics" parent command with
// backfill as its only subcommand.
func (f *CommandFactory) newWorkMetricsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Metrics over attempt evidence",
		Long:  "Manage .ddx/metrics/attempts.jsonl — the cold-powerClass per-attempt metrics store.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(f.newWorkMetricsBackfillCommand())
	return cmd
}

// newWorkMetricsBackfillCommand returns "ddx work metrics backfill". It reads
// all beads' events.jsonl attachments and emits one row per attempt found,
// deduplicating against any rows already in .ddx/metrics/attempts.jsonl.
func (f *CommandFactory) newWorkMetricsBackfillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill .ddx/metrics/attempts.jsonl from existing bead events",
		Long: `Scan all beads' events and produce per-attempt rows in .ddx/metrics/attempts.jsonl.

Each kind:cost event in a bead's event stream corresponds to one agent execution
attempt. backfill pairs each cost event with the immediately following
kind:execute-bead event (the finalization outcome) to produce a complete row.

The command is idempotent: rows already present in attempts.jsonl (identified by
attempt_id) are skipped. Re-running backfill after a partial run or after new
beads are closed is safe.

Note: backfill reads .ddx/attachments/<bead-id>/events.jsonl and the inline
events in beads.jsonl. It does NOT read .ddx/executions/ directories.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectFlag, _ := cmd.Flags().GetString("project")
			projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

			store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
			allBeads, err := store.ReadAll(context.Background())
			if err != nil {
				return fmt.Errorf("read beads: %w", err)
			}

			inputs := make([]attemptmetrics.BeadAttemptEvents, 0, len(allBeads))
			for _, b := range allBeads {
				events, err := store.Events(b.ID)
				if err != nil {
					// Best-effort: skip beads whose events can't be read.
					if f.WorkingDir != "" {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
							"warning: skipping %s events: %v\n", b.ID, err)
					}
					continue
				}
				inputs = append(inputs, attemptmetrics.BeadAttemptEvents{
					BeadID: b.ID,
					SpecID: beadSpecID(b),
					Events: events,
				})
			}

			added, err := attemptmetrics.BackfillFromEvents(projectRoot, inputs)
			if err != nil {
				return fmt.Errorf("backfill: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "backfill complete: %d new rows added\n", added)
			return nil
		},
	}
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root)")
	return cmd
}
