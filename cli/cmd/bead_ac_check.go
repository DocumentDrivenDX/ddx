package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
	"github.com/spf13/cobra"
)

// newBeadAcCheckCommand wires `ddx bead ac-check <id>` — mechanical per-AC
// verification (test-name match, symbol grep, negative-AC absence, mechanical
// file detection). Prose AC items yield needs_judgment, never auto-pass or
// auto-fail. The output is consumed by the reviewer at the gate so it adjudicates
// structured input instead of grepping a raw diff.
func (f *CommandFactory) newBeadAcCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ac-check <bead-id>",
		Short: "Mechanically verify acceptance criteria against the working tree",
		Long: `ac-check classifies each AC into a kind (test-name | build-gate |
negative | symbol | mechanical | prose) and gathers evidence
mechanically. Output is emitted as JSON to stdout (when --json) and to
.ddx/executions/<attempt-id>/ac-check.json when --attempt-id is given.

Examples:
  ddx bead ac-check ddx-abc123
  ddx bead ac-check ddx-abc123 --rev-range main..HEAD --json
  ddx bead ac-check ddx-abc123 --attempt-id 20260510T210000-abcdef00`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadID := args[0]
			s := f.beadStore()
			b, err := s.Get(context.Background(), beadID)
			if err != nil {
				return fmt.Errorf("bead %s: %w", beadID, err)
			}
			if b == nil {
				return fmt.Errorf("bead %s not found", beadID)
			}

			items := accheck.ParseAcceptance(b.Acceptance)
			if len(items) == 0 {
				return fmt.Errorf("bead %s has no acceptance criteria to check", beadID)
			}

			revRange, _ := cmd.Flags().GetString("rev-range")
			attemptID, _ := cmd.Flags().GetString("attempt-id")
			asJSON, _ := cmd.Flags().GetBool("json")
			packages, _ := cmd.Flags().GetStringSlice("package")

			workspace := f.beadWorkspaceRoot()
			if workspace == "" {
				workspace = f.WorkingDir
			}

			ctx := accheck.Context{
				WorkingDir: workspace,
				RevBase:    revRange,
				Packages:   packages,
				RunTest:    accheck.DefaultRunTest(workspace),
				GitGrep:    accheck.DefaultGitGrep(workspace),
				DiffHits:   accheck.DefaultDiffHits(workspace, revRange),
			}

			entries := accheck.Evaluate(items, ctx)
			out := accheck.Aggregate(beadID, attemptID, entries)

			// Persist to .ddx/executions/<attempt-id>/ac-check.json when we have one.
			if attemptID != "" {
				path := filepath.Join(workspace, ".ddx", "executions", attemptID, "ac-check.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
				}
				blob, mErr := json.MarshalIndent(out, "", "  ")
				if mErr != nil {
					return fmt.Errorf("marshal ac-check.json: %w", mErr)
				}
				if err := os.WriteFile(path, blob, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", path, err)
				}
			}

			if asJSON {
				blob, mErr := json.MarshalIndent(out, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(blob))
				return nil
			}
			return accheck.WriteHuman(cmd.OutOrStdout(), out)
		},
	}
	cmd.Flags().String("rev-range", "", "git diff range used to scope DiffHits (e.g., main..HEAD)")
	cmd.Flags().String("attempt-id", "", "if set, persist ac-check.json under .ddx/executions/<attempt-id>/")
	cmd.Flags().Bool("json", false, "emit JSON to stdout instead of the human-readable table")
	cmd.Flags().StringSlice("package", nil, "Go packages to scope test runs to (default ./...)")
	return cmd
}
