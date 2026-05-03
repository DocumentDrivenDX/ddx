package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/checks"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newAcCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ac",
		Short: "Run pre-merge checks (declared in .ddx/checks/*.yaml)",
	}
	cmd.AddCommand(f.newAcRunCommand())
	return cmd
}

func (f *CommandFactory) newAcRunCommand() *cobra.Command {
	var only string
	var fromRev string
	cmd := &cobra.Command{
		Use:   "run <bead-id>",
		Short: "Run all applicable checks against the current worktree",
		Long: `Run every check in .ddx/checks/*.yaml whose applies_to filter matches the
named bead and the changed paths between DIFF_BASE and HEAD. Each check
runs in parallel and writes its result file under .ddx/executions/ac-<ts>/.

Exits 1 if any check returns block or error.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadID := args[0]
			projectRoot := f.WorkingDir

			b, err := loadBead(projectRoot, beadID)
			if err != nil {
				return fmt.Errorf("load bead: %w", err)
			}

			diffHead, err := gitRevParse(projectRoot, "HEAD")
			if err != nil {
				return fmt.Errorf("resolve HEAD: %w", err)
			}
			diffBase := fromRev
			if diffBase == "" {
				diffBase = beadBaseRev(b)
			}
			if diffBase == "" {
				diffBase, _ = gitMergeBase(projectRoot, "HEAD", "main")
			}
			if diffBase == "" {
				diffBase = diffHead // empty diff fallback
			}

			changed, err := gitChangedPaths(projectRoot, diffBase, diffHead)
			if err != nil {
				return fmt.Errorf("diff paths: %w", err)
			}

			all, err := checks.LoadDir(filepath.Join(projectRoot, ".ddx", "checks"))
			if err != nil {
				return err
			}
			if only != "" {
				all = filterByName(all, only)
				if len(all) == 0 {
					return fmt.Errorf("no check named %q", only)
				}
			}

			runID := newRunID()
			evidenceDir := filepath.Join(projectRoot, ".ddx", "executions", "ac-"+runID)
			ictx := checks.InvocationContext{
				BeadID:       beadID,
				DiffBase:     diffBase,
				DiffHead:     diffHead,
				ProjectRoot:  projectRoot,
				EvidenceDir:  evidenceDir,
				RunID:        runID,
				BeadLabels:   b.Labels,
				ChangedPaths: changed,
			}

			results, err := checks.Run(cmd.Context(), all, ictx)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "ac run %s (bead=%s base=%s head=%s checks=%d evidence=%s)\n",
				runID, beadID, shortSHA(diffBase), shortSHA(diffHead), len(results), evidenceDir)
			anyBad := false
			for _, r := range results {
				marker := "PASS"
				switch r.Status {
				case checks.StatusBlock:
					marker = "BLOCK"
					anyBad = true
				case checks.StatusError:
					marker = "ERROR"
					anyBad = true
				}
				fmt.Fprintf(out, "  [%s] %s — %s\n", marker, r.Name, r.Message)
				for _, v := range r.Violations {
					loc := v.File
					if v.Line > 0 {
						loc = fmt.Sprintf("%s:%d", v.File, v.Line)
					}
					fmt.Fprintf(out, "      %s %s %s\n", loc, v.Symbol, v.Detail)
				}
			}
			if anyBad {
				return fmt.Errorf("one or more checks reported block/error")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "check", "", "Run only the named check")
	cmd.Flags().StringVar(&fromRev, "from", "", "Override DIFF_BASE (defaults to bead base_rev or merge-base with main)")
	return cmd
}

func loadBead(projectRoot, id string) (*bead.Bead, error) {
	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	return store.Get(id)
}

func beadBaseRev(b *bead.Bead) string {
	if b == nil || b.Extra == nil {
		return ""
	}
	for _, k := range []string{"base_rev", "base-rev", "baseRev"} {
		if v, ok := b.Extra[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func filterByName(all []checks.Check, name string) []checks.Check {
	out := make([]checks.Check, 0, 1)
	for _, c := range all {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func gitRevParse(dir, ref string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitMergeBase(dir, a, b string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "merge-base", a, b).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitChangedPaths(dir, base, head string) ([]string, error) {
	if base == "" || base == head {
		return nil, nil
	}
	out, err := exec.Command("git", "-C", dir, "diff", "--name-only", base, head).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	paths := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}

func newRunID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:])
}

func shortSHA(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}
