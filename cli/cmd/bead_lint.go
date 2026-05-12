package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

// newBeadLintCommand wires `ddx bead lint <id>` — ad-hoc AC verifiability
// check. It classifies each AC into a kind (test-name | build-gate | negative
// | symbol | mechanical | prose), computes a verifiability score, and reports
// whether the bead meets the configured threshold. No LLM calls are made; the
// check is purely deterministic. Operators use this to self-check a bead
// before filing or dispatching it.
func (f *CommandFactory) newBeadLintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint <bead-id>",
		Short: "Check acceptance criteria mechanical verifiability",
		Long: `lint classifies each AC into a kind (test-name | build-gate |
negative | symbol | mechanical | prose) and computes a verifiability
score (verifiable_count / total). The score is compared to the
configured threshold (default 0.5; override via intake.ac_quality.min_score
in .ddx/config.yaml).

No LLM calls are made. Use this command to self-check a bead before
filing or dispatching it.

Examples:
  ddx bead lint ddx-abc123
  ddx bead lint ddx-abc123 --json`,
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

			threshold := agent.DefaultACQualityMinScore
			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}
			if cfg, cfgErr := config.LoadWithWorkingDir(workspaceRoot); cfgErr == nil {
				threshold = cfg.ResolveACQualityMinScore()
			}

			result := agent.CheckACQuality(b.Acceptance, threshold)

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				blob, mErr := json.MarshalIndent(result, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(blob))
				return nil
			}
			return writeBeadLintHuman(cmd.OutOrStdout(), beadID, result)
		},
	}
	cmd.Flags().Bool("json", false, "emit JSON to stdout instead of the human-readable table")
	return cmd
}

func writeBeadLintHuman(w io.Writer, beadID string, r agent.ACQualityResult) error {
	fmt.Fprintf(w, "bead lint for %s\n", beadID)
	verdict := "FAILS"
	if r.PassesThreshold {
		verdict = "PASSES"
	}
	fmt.Fprintf(w, "score: %.2f (%d/%d verifiable)  %s threshold %.2f\n\n",
		r.Score, r.VerifiableCount, r.Total, verdict, r.Threshold)
	for _, item := range r.Items {
		verifiableLabel := "prose"
		if item.Verifiable {
			verifiableLabel = "verifiable"
		}
		fmt.Fprintf(w, "AC #%d  [%-13s]  %-10s  %s\n",
			item.AC, item.Kind, verifiableLabel, truncateLintText(item.Text, 80))
	}
	return nil
}

func truncateLintText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
