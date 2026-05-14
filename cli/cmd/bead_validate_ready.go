package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

// validateReadyEntry is one row in the validate-ready report. It identifies
// the bead and reports whether its acceptance criteria meet the configured
// machine-measurability threshold.
type validateReadyEntry struct {
	ID              string                `json:"id"`
	Title           string                `json:"title"`
	Priority        int                   `json:"priority"`
	Score           float64               `json:"score"`
	VerifiableCount int                   `json:"verifiable_count"`
	ProseCount      int                   `json:"prose_count"`
	Total           int                   `json:"total"`
	Threshold       float64               `json:"threshold"`
	PassesThreshold bool                  `json:"passes_threshold"`
	Items           []agent.ACQualityItem `json:"items,omitempty"`
}

// validateReadyReport bundles the per-bead findings together with the
// summary counts the operator and CI need to decide pass/fail.
type validateReadyReport struct {
	Threshold         float64              `json:"threshold"`
	TotalReady        int                  `json:"total_ready"`
	FailingCount      int                  `json:"failing_count"`
	Entries           []validateReadyEntry `json:"entries"`
	CloseWithEvidence closeWithEvidenceDoc `json:"close_with_evidence"`
}

// closeWithEvidenceDoc embeds the close-with-evidence contract in the report
// output so callers (HELIX, CI) have the expectations in-band rather than
// having to cross-reference docs.
type closeWithEvidenceDoc struct {
	Summary      string   `json:"summary"`
	Requirements []string `json:"requirements"`
	GateBehavior string   `json:"gate_behavior"`
}

// closeWithEvidenceContract is the canonical description of what DDx requires
// to close a bead via the automated work path. Mirrors the
// ClosureGate implementation in internal/bead/store.go.
var closeWithEvidenceContract = closeWithEvidenceDoc{
	Summary: "Automated closures must carry both a terminal verdict and execution evidence.",
	Requirements: []string{
		"closing_commit_sha is non-empty OR at least one BeadEvent exists in the events history (inline or externalized).",
		"Any review event with summary=APPROVE carries a non-empty body (rationale). Empty-rationale APPROVE is rejected as a malformed reviewer verdict.",
		"Beads that skip review (--no-review or no Reviewer configured) still need execution evidence to close.",
	},
	GateBehavior: "ClosureGate enforces these on every CloseWithEvidence call; the manual `ddx bead close` path is an administration escape hatch.",
}

func (f *CommandFactory) newBeadValidateReadyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "validate-ready",
		Short:         "Validate execution-ready beads have machine-measurable acceptance",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `validate-ready selects beads using the same semantics as ` + "`ddx bead ready --execution`" + `
and rejects any whose acceptance criteria are not machine-measurable.

Selection: status=open, deps closed, not an epic, execution-eligible, not
superseded, not on retry cooldown. Identical to ` + "`ddx bead ready --execution`" + `.

Measurability: each bead's acceptance is classified by accheck into
test-name | build-gate | symbol | negative | mechanical | prose. The
verifiability score = (non-prose count) / total. A bead passes when score
>= intake.ac_quality.min_score (default 0.5).

Exit code: 0 when every execution-ready bead passes; 1 when any fail (or
on store errors). Operators run this in CI to keep the queue free of
prose-only ACs, replacing HELIX-side duplicate validators that mirrored
DDx's execution-ready logic out-of-band.

Close-with-evidence: the JSON report includes the contract DDx enforces
when automated closures happen. Adopting projects (HELIX, etc.) can rely
on this surface instead of maintaining their own validator.

Examples:
  ddx bead validate-ready
  ddx bead validate-ready --json
  ddx bead validate-ready --threshold 0.75`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := f.beadStore()
			beads, err := s.ReadyExecution()
			if err != nil {
				return fmt.Errorf("ready-execution: %w", err)
			}

			threshold := agent.DefaultACQualityMinScore
			workspaceRoot := f.beadWorkspaceRoot()
			if workspaceRoot == "" {
				workspaceRoot = f.WorkingDir
			}
			if cfg, cfgErr := config.LoadWithWorkingDir(workspaceRoot); cfgErr == nil {
				threshold = cfg.ResolveACQualityMinScore()
			}
			if explicit, _ := cmd.Flags().GetFloat64("threshold"); cmd.Flags().Changed("threshold") {
				threshold = explicit
			}

			report := buildValidateReadyReport(beads, threshold)

			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				blob, mErr := json.MarshalIndent(report, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(blob))
			} else {
				writeValidateReadyHuman(cmd.OutOrStdout(), report)
			}

			if report.FailingCount > 0 {
				return fmt.Errorf("%d of %d execution-ready beads have non-measurable acceptance (threshold %.2f)",
					report.FailingCount, report.TotalReady, report.Threshold)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit JSON to stdout (includes close-with-evidence contract)")
	cmd.Flags().Float64("threshold", 0, "Override intake.ac_quality.min_score for this run (0..1)")
	return cmd
}

func buildValidateReadyReport(beads []bead.Bead, threshold float64) validateReadyReport {
	entries := make([]validateReadyEntry, 0, len(beads))
	failing := 0
	for _, b := range beads {
		result := agent.CheckACQuality(b.Acceptance, threshold)
		entry := validateReadyEntry{
			ID:              b.ID,
			Title:           b.Title,
			Priority:        b.Priority,
			Score:           result.Score,
			VerifiableCount: result.VerifiableCount,
			ProseCount:      result.ProseCount,
			Total:           result.Total,
			Threshold:       result.Threshold,
			PassesThreshold: result.PassesThreshold,
			Items:           result.Items,
		}
		if !result.PassesThreshold {
			failing++
		}
		entries = append(entries, entry)
	}
	return validateReadyReport{
		Threshold:         threshold,
		TotalReady:        len(beads),
		FailingCount:      failing,
		Entries:           entries,
		CloseWithEvidence: closeWithEvidenceContract,
	}
}

func writeValidateReadyHuman(w io.Writer, r validateReadyReport) {
	fmt.Fprintf(w, "validate-ready: %d execution-ready beads (threshold %.2f)\n",
		r.TotalReady, r.Threshold)
	if r.TotalReady == 0 {
		fmt.Fprintln(w, "No execution-ready beads.")
		fmt.Fprintln(w)
		writeCloseWithEvidenceHuman(w)
		return
	}
	for _, e := range r.Entries {
		verdict := "PASS"
		if !e.PassesThreshold {
			verdict = "FAIL"
		}
		fmt.Fprintf(w, "  %s  %s  score=%.2f (%d/%d verifiable)  P%d  %s\n",
			verdict, e.ID, e.Score, e.VerifiableCount, e.Total, e.Priority, e.Title)
		if !e.PassesThreshold {
			for _, item := range e.Items {
				if !item.Verifiable {
					fmt.Fprintf(w, "      AC #%d  [%s]  %s\n", item.AC, item.Kind, truncateLintText(item.Text, 80))
				}
			}
		}
	}
	fmt.Fprintf(w, "\nSummary: %d failing / %d total\n", r.FailingCount, r.TotalReady)
	fmt.Fprintln(w)
	writeCloseWithEvidenceHuman(w)
}

func writeCloseWithEvidenceHuman(w io.Writer) {
	fmt.Fprintln(w, "Close-with-evidence contract:")
	fmt.Fprintf(w, "  %s\n", closeWithEvidenceContract.Summary)
	for _, req := range closeWithEvidenceContract.Requirements {
		fmt.Fprintf(w, "    - %s\n", req)
	}
	fmt.Fprintf(w, "  %s\n", closeWithEvidenceContract.GateBehavior)
}
