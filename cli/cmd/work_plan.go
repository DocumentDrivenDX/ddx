package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/spf13/cobra"
)

// newWorkPlanCommand creates the "ddx work plan" subcommand: a read-only
// dry-run preview of the queue showing the exact ordering "ddx work" would
// pick, without claiming or executing any bead.
//
// Relationship to siblings:
//   - "ddx work"       — executes the drain (claims and runs beads)
//   - "ddx work plan"  — previews what the drain would do (read-only)
//   - "ddx bead ready" — lists dependency-ready beads; does NOT apply the
//     picker's label-filter or cooldown/superseded eligibility filters
func (f *CommandFactory) newWorkPlanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview what 'ddx work' would pick (dry-run)",
		Long: `plan shows the execution queue in picker order — the same order
'ddx work' would claim beads — without claiming or executing anything.

Unlike 'ddx bead ready', plan applies the picker's filters:
  - Execution eligibility (cooldown, superseded, execution-eligible flag)
  - Optional label-filter intersection (--label-filter)
  - Optional capabilities constraint (--capabilities; currently a no-op)

The per-Run attempted map is intentionally excluded: it is non-deterministic
across runs and is not part of the stable picker decision surface.

Columns:
  POS      — 1-based pick order
  ID       — bead ID
  PRI      — priority (0 = highest)
  UPDATED  — last updated timestamp (RFC3339)
  STATUS   — bead status
  DECISION — "next claim", "eligible (rank N)", or "skipped: <reason>"

Use --json for machine-readable output suitable for piping to jq:
  ddx work plan --json | jq '.[].id'
`,
		Example: `  # Show top 10 beads in picker order (default)
  ddx work plan

  # Show the full queue
  ddx work plan --limit=0

  # Filter identically to a label-constrained worker
  ddx work plan --label-filter=phase:2,area:agent

  # Machine-readable output
  ddx work plan --json | jq '.[].id'`,
		Args: cobra.NoArgs,
		RunE: f.runWorkPlan,
	}

	cmd.Flags().String("label-filter", "", "Filter by label (same intersection logic as 'ddx work --label-filter')")
	cmd.Flags().String("capabilities", "", "Capabilities constraint (reserved; currently no-op pass-through)")
	cmd.Flags().Int("limit", 10, "Maximum number of entries to show; 0 = full queue")
	cmd.Flags().Bool("json", false, "Output as JSON array (suitable for piping to jq)")

	return cmd
}

// runWorkPlan is the RunE for "ddx work plan".
func (f *CommandFactory) runWorkPlan(cmd *cobra.Command, _ []string) error {
	labelFilter, _ := cmd.Flags().GetString("label-filter")
	capabilities, _ := cmd.Flags().GetString("capabilities")
	limit, _ := cmd.Flags().GetInt("limit")
	asJSON, _ := cmd.Flags().GetBool("json")

	// Resolve the project root (same logic work.go uses).
	projectRoot := f.WorkingDir

	// Open the bead store directly — same path ReadyExecution uses.
	ddxDir := projectRoot + "/.ddx"
	store := bead.NewStore(ddxDir)

	filters := agent.PickerFilters{
		LabelFilter:  labelFilter,
		Capabilities: capabilities,
	}

	entries, err := agent.PreviewQueue(store, filters, limit)
	if err != nil {
		return fmt.Errorf("work plan: %w", err)
	}

	if asJSON {
		return printWorkPlanJSON(cmd, entries)
	}
	return printWorkPlanText(cmd, entries, limit)
}

func printWorkPlanJSON(cmd *cobra.Command, entries []agent.QueueEntry) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func printWorkPlanText(cmd *cobra.Command, entries []agent.QueueEntry, limit int) error {
	out := cmd.OutOrStdout()
	if len(entries) == 0 {
		fmt.Fprintln(out, "No execution-eligible beads in the queue.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "POS\tID\tPRI\tUPDATED\tSTATUS\tDECISION\tWHY")
	fmt.Fprintln(w, "---\t--\t---\t-------\t------\t--------\t---")
	for _, e := range entries {
		updated := e.UpdatedAt.UTC().Format(time.RFC3339)
		if e.UpdatedAt.IsZero() {
			updated = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\t%s\n",
			e.Position, e.BeadID, e.Priority, updated, e.Status, e.FilterDecision, e.Why)
	}
	_ = w.Flush()

	if limit > 0 {
		fmt.Fprintf(out, "\n(showing up to %d entries; use --limit=0 for full queue)\n", limit)
	}
	return nil
}
