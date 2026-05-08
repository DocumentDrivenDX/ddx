package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
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

Idempotent: once a tracker is clean, running --fix again is a no-op.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := f.beadWorkspaceRoot()
			if workspace == "" {
				return fmt.Errorf("bead doctor: no .ddx workspace found")
			}
			path := filepath.Join(workspace, ".ddx", "beads.jsonl")

			doFix, _ := cmd.Flags().GetBool("fix")
			asJSON, _ := cmd.Flags().GetBool("json")

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
	return cmd
}
