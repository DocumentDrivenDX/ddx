package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/spf13/cobra"
)

// newBeadDoctorCommand wires `ddx bead doctor` / `ddx bead doctor --fix`.
//
// Scan mode (no flags): exits non-zero if any field on any bead exceeds the
// per-field cap (ddx-f8a11202), reporting the offending bead id, field, and
// size. Safe to run on any tree — no mutations.
//
// Fix mode (--fix): rewrites oversized fields in place. Before touching the
// tracker the command writes a timestamped backup under .ddx/backups/ so
// the original file is always recoverable. Overflow content persists as
// artifacts under .ddx/executions/<bead-id>/repair-<timestamp>/ and a
// repair event is appended to each rewritten bead. Idempotent — the second
// invocation exits 0 without writing because the scan is clean.
//
// Dangling-success mode (--dangling): detects in_progress beads whose last
// execution produced task_succeeded but CloseWithEvidence never ran. Safe
// to run on any tree — no mutations unless --fix is also passed.
// (ddx-2b2d114e)
func (f *CommandFactory) newBeadDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Detect (and optionally repair) beads.jsonl rows with oversized fields",
		Long: `Scan the bead tracker for fields that exceed the per-field size cap.

Bead fields (description, acceptance, notes, events[].body, events[].summary)
are capped at 65,535 bytes so DDx-authored beads round-trip cleanly through
bd import (upstream's Dolt TEXT column limit). Fields over the cap usually
come from a writer bug that landed before the cap was enforced — for
example a reviewer stream dumped verbatim into an event body.

Without --fix this command only reports offending rows and exits non-zero.
With --fix it:

  1. Writes a timestamped backup to .ddx/backups/ before any mutation.
  2. Truncates each oversized field to the cap using head+tail+marker.
  3. Writes the full original payload to
     .ddx/executions/<bead>/repair-<timestamp>/<field>.log so forensics
     remain possible.
  4. Appends a kind=repair event to every rewritten bead.

Idempotent: once a tracker is clean, running --fix again is a no-op.

With --dangling: scan for in_progress beads whose last execution produced
task_succeeded but whose bead-close step never ran (dangling-success).
For each finding, reports whether result_rev is reachable from HEAD:
  - reachable=true:  merge succeeded, close didn't. Safe to close manually
    with: ddx bead close <bead-id>
  - reachable=false: the merge itself was never applied (dangling commit).
    The result_rev is shown for manual git cherry-pick or re-attempt.
(ddx-2b2d114e)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := f.beadWorkspaceRoot()
			if workspace == "" {
				return fmt.Errorf("bead doctor: no .ddx workspace found")
			}
			path := ddxroot.JoinProject(workspace, "beads.jsonl")

			doFix, _ := cmd.Flags().GetBool("fix")
			asJSON, _ := cmd.Flags().GetBool("json")
			doDangling, _ := cmd.Flags().GetBool("dangling")

			// --dangling mode: detect dangling-success beads (ddx-2b2d114e).
			if doDangling {
				findings, err := agent.DetectDanglingSuccessBeads(workspace)
				if err != nil {
					return fmt.Errorf("bead doctor --dangling: %w", err)
				}
				if asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(struct {
						Clean    bool                           `json:"clean"`
						Findings []agent.DanglingSuccessFinding `json:"findings"`
					}{
						Clean:    len(findings) == 0,
						Findings: findings,
					})
				}
				out := cmd.OutOrStdout()
				if len(findings) == 0 {
					fmt.Fprintf(out, "bead doctor --dangling: clean (no dangling-success beads detected)\n")
					return nil
				}
				fmt.Fprintf(out, "bead doctor --dangling: %d finding(s):\n", len(findings))
				for _, fi := range findings {
					reachableStr := "reachable=false (dangling commit — merge never applied)"
					recovery := "  recovery: git cherry-pick or re-run ddx try " + fi.BeadID
					if fi.Reachable {
						reachableStr = "reachable=true (merge succeeded, close didn't run)"
						recovery = "  recovery: ddx bead close " + fi.BeadID + "  # or re-run ddx try"
					}
					fmt.Fprintf(out, "  %s  attempt=%s  result_rev=%s\n    %s\n%s\n",
						fi.BeadID, fi.AttemptID, fi.ResultRev, reachableStr, recovery)
				}
				return fmt.Errorf("bead doctor --dangling: %d finding(s) detected", len(findings))
			}

			var report bead.DoctorReport
			var err error
			if doFix {
				report, err = bead.BeadDoctorFix(path, nil)
			} else {
				report, err = bead.BeadDoctor(path)
			}
			if err != nil {
				return err
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					Path     string               `json:"path"`
					Fixed    bool                 `json:"fixed"`
					Clean    bool                 `json:"clean"`
					Findings []bead.DoctorFinding `json:"findings"`
				}{
					Path:     report.Path,
					Fixed:    doFix && !report.Clean(),
					Clean:    report.Clean(),
					Findings: report.Findings,
				})
			}

			out := cmd.OutOrStdout()
			if report.Clean() {
				fmt.Fprintf(out, "bead doctor: %s — clean (no fields exceed %d bytes)\n", path, bead.MaxFieldBytes)
				return nil
			}
			fmt.Fprintf(out, "bead doctor: %s — %d finding(s) exceeding %d-byte cap:\n", path, len(report.Findings), bead.MaxFieldBytes)
			for _, f := range report.Findings {
				fmt.Fprintf(out, "  %s  %s  %d bytes  head=%q\n", f.BeadID, f.FieldPath, f.SizeBytes, f.SampleHead)
			}
			if doFix {
				fmt.Fprintf(out, "\nrepair complete. backup written to %s/backups/. artifact sidecars under %s/executions/<bead>/repair-*/\n", filepath.Dir(path), filepath.Dir(path))
				return nil
			}
			// Non-fix scan: non-zero exit via cobra error so CI can catch it.
			return fmt.Errorf("bead doctor: %d finding(s) — run `ddx bead doctor --fix` to repair", len(report.Findings))
		},
	}
	cmd.Flags().Bool("fix", false, "Rewrite oversized fields in place after writing a backup")
	cmd.Flags().Bool("json", false, "Output findings as JSON")
	cmd.Flags().Bool("dangling", false, "Detect in_progress beads with a prior task_succeeded result (dangling-success, ddx-2b2d114e)")
	return cmd
}
