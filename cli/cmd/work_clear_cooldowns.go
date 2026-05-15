package cmd

import (
	"context"
	"fmt"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newWorkClearCooldownsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear-cooldowns",
		Short: "Bulk-clear queue-drain cooldowns",
		Long: `clear-cooldowns scans beads with an active queue-drain cooldown and clears
them in one pass. Use after a systemic issue (e.g., a push-layer failure) is
resolved so blocked beads re-enter the execution queue without per-bead --unset loops.

Requires --all or --status to prevent accidental bulk clears.`,
		Example: `  # Clear all active cooldowns
  ddx work clear-cooldowns --all

  # Clear only push_failed cooldowns
  ddx work clear-cooldowns --status push_failed

  # Preview without modifying state
  ddx work clear-cooldowns --all --dry-run`,
		Args: cobra.NoArgs,
		RunE: f.runWorkClearCooldowns,
	}
	cmd.Flags().Bool("all", false, "Clear cooldowns on every bead with retry-after set")
	cmd.Flags().String("status", "", "Clear only beads where last-status matches this value")
	cmd.Flags().Bool("dry-run", false, "Print what would be cleared without modifying state")
	return cmd
}

func (f *CommandFactory) runWorkClearCooldowns(cmd *cobra.Command, _ []string) error {
	all, _ := cmd.Flags().GetBool("all")
	status, _ := cmd.Flags().GetString("status")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if !all && status == "" {
		return fmt.Errorf("requires --all or --status <value>")
	}

	ddxDir := ddxroot.Path(context.Background(), f.WorkingDir)
	store := bead.NewStore(ddxDir)

	var filter func(bead.Bead) bool
	if status != "" {
		s := status
		filter = func(b bead.Bead) bool {
			if b.Extra == nil {
				return false
			}
			v, _ := b.Extra[bead.ExtraLastStatus].(string)
			return v == s
		}
	}

	if dryRun {
		allBeads, err := store.ReadAll(context.Background())
		if err != nil {
			return err
		}
		count := 0
		for _, b := range allBeads {
			if b.Extra == nil {
				continue
			}
			v, _ := b.Extra[bead.ExtraRetryAfter].(string)
			if v == "" {
				continue
			}
			if filter != nil && !filter(b) {
				continue
			}
			count++
		}
		fmt.Fprintf(cmd.OutOrStdout(), "would clear %d cooldown(s)\n", count)
		return nil
	}

	count, err := store.ClearCooldowns(filter)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cleared %d cooldown(s)\n", count)
	return nil
}
