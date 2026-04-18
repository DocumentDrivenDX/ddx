package cmd

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
)

// beadReviewTemplate is the review instructions embedded in the generated prompt.
// It defines the required output contract (grade per AC item, overall verdict,
// summary, and findings) that the reviewing agent must follow.
//
// Template sections:
//
//	<bead-review>
//	  <bead>        — bead metadata (title, description, acceptance, labels)
//	  <governing>   — governing doc refs with full file content
//	  <diff>        — git show output for the reviewed commit
//	  <instructions>— this text; tells the agent how to produce the review
//	</bead-review>
//
// Expected output contract:
//
//	## Review: <id> iter <N>
//	### Verdict: APPROVE | REQUEST_CHANGES | BLOCK
//	### AC Grades
//	| # | Item | Grade | Evidence |
//	...
//	### Summary
//	...
//	### Findings (omit if verdict is APPROVE)
//	...
const beadReviewInstructions = `You are reviewing a bead implementation against its acceptance criteria.

## Your task

Examine the diff and each acceptance-criteria (AC) item. For each item assign one grade:

- **APPROVE** — fully and correctly implemented; cite the specific file path and line that proves it.
- **REQUEST_CHANGES** — partially implemented or has fixable minor issues.
- **BLOCK** — not implemented, incorrectly implemented, or the diff is insufficient to evaluate.

Overall verdict rule:
- All items APPROVE → **APPROVE**
- Any item BLOCK → **BLOCK**
- Otherwise → **REQUEST_CHANGES**

## Required output format

Respond with a structured review using exactly this layout (replace placeholder text):

---
## Review: <bead-id> iter <N>

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### AC Grades

| # | Item | Grade | Evidence |
|---|------|-------|----------|
| 1 | <AC item text, max 60 chars> | APPROVE | path/to/file.go:42 — brief note |
| 2 | <AC item text, max 60 chars> | BLOCK   | — not found in diff |

### Summary

<1–3 sentences on overall implementation quality and any recurring theme in findings.>

### Findings

<Bullet list of REQUEST_CHANGES and BLOCK findings. Each finding must name the specific file, function, or test that is missing or wrong — specific enough for the next agent to act on without re-reading the entire diff. Omit this section entirely if verdict is APPROVE.>`

func (f *CommandFactory) newBeadReviewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <id>",
		Short: "Generate a review prompt for a bead's implementation",
		Long: `Generate a review-ready prompt for a bead implementation.

The prompt includes:
  - Bead title, description, and acceptance criteria
  - Full content of governing documents (spec-id refs from the bead)
  - Git diff of the reviewed commit (git show)
  - Review instructions with the expected APPROVE/REQUEST_CHANGES/BLOCK output contract

By default the commit is taken from the bead's closing_commit_sha field.
Use --from-rev to override.

Pipe the output into ddx agent run:

  ddx bead review <id> | ddx agent run --prompt @-
  ddx bead review <id> --output /tmp/review.md && ddx agent run --prompt /tmp/review.md`,
		Args: cobra.ExactArgs(1),
		RunE: f.runBeadReview,
	}
	cmd.Flags().String("from-rev", "", "Commit SHA to review (default: closing_commit_sha from bead)")
	cmd.Flags().Int("iter", 1, "Review iteration number (shown in prompt header and grade table)")
	cmd.Flags().String("output", "", "Write prompt to file instead of stdout")
	return cmd
}

func (f *CommandFactory) runBeadReview(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	fromRev, _ := cmd.Flags().GetString("from-rev")
	iter, _ := cmd.Flags().GetInt("iter")
	outputFile, _ := cmd.Flags().GetString("output")

	s := f.beadStore()
	b, err := s.Get(beadID)
	if err != nil {
		return err
	}

	// Resolve the commit SHA to review.
	// Priority: --from-rev flag > closing_commit_sha on the bead.
	rev := strings.TrimSpace(fromRev)
	if rev == "" {
		if sha, ok := b.Extra["closing_commit_sha"].(string); ok {
			rev = strings.TrimSpace(sha)
		}
	}
	if rev == "" {
		return fmt.Errorf("no commit to review: use --from-rev <sha>, or close the bead with a commit SHA first (ddx bead close --commit <sha>)")
	}

	// Project root is used for git operations and governing doc file reads.
	projectRoot := gitpkg.FindProjectRoot(f.WorkingDir)
	if projectRoot == "" {
		projectRoot = f.WorkingDir
	}

	// Fetch the git diff for the commit.
	diff, err := beadReviewGitShow(projectRoot, rev)
	if err != nil {
		return fmt.Errorf("git show %s: %w", rev, err)
	}

	// Resolve governing document references from the bead's spec-id field.
	refs := agent.ResolveGoverningRefs(projectRoot, b)

	// Build and emit the review prompt.
	var sb strings.Builder
	buildBeadReviewPrompt(&sb, b, iter, rev, diff, projectRoot, refs)

	prompt := sb.String()
	if outputFile != "" {
		return os.WriteFile(outputFile, []byte(prompt), 0o644)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), prompt)
	return err
}

// beadReviewGitShow runs `git show <rev>` with pathspec exclusions for
// execution-evidence noise so the review prompt's <diff> section stays
// bounded. See agent.EvidenceReviewExcludePathspecs and ddx-39e27896.
func beadReviewGitShow(projectRoot, rev string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append([]string{"-C", projectRoot, "show", rev, "--", "."}, agent.EvidenceReviewExcludePathspecs()...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// buildBeadReviewPrompt writes the complete review prompt to sb.
func buildBeadReviewPrompt(sb *strings.Builder, b *bead.Bead, iter int, rev, diff, projectRoot string, refs []agent.GoverningRef) {
	sb.WriteString("<bead-review>\n")

	// ── Bead section ────────────────────────────────────────────────────────
	fmt.Fprintf(sb, "  <bead id=%q iter=%d>\n", b.ID, iter)
	fmt.Fprintf(sb, "    <title>%s</title>\n", beadReviewXMLEscape(strings.TrimSpace(b.Title)))

	if desc := strings.TrimSpace(b.Description); desc != "" {
		fmt.Fprintf(sb, "    <description>\n%s\n    </description>\n", beadReviewXMLEscape(desc))
	} else {
		sb.WriteString("    <description/>\n")
	}

	if acc := strings.TrimSpace(b.Acceptance); acc != "" {
		fmt.Fprintf(sb, "    <acceptance>\n%s\n    </acceptance>\n", beadReviewXMLEscape(acc))
	} else {
		sb.WriteString("    <acceptance/>\n")
	}

	if len(b.Labels) > 0 {
		fmt.Fprintf(sb, "    <labels>%s</labels>\n", beadReviewXMLEscape(strings.Join(b.Labels, ", ")))
	}

	sb.WriteString("  </bead>\n\n")

	// ── Governing docs section ───────────────────────────────────────────────
	sb.WriteString("  <governing>\n")
	if len(refs) == 0 {
		sb.WriteString("    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>\n")
	} else {
		for _, ref := range refs {
			if ref.Title != "" {
				fmt.Fprintf(sb, "    <ref id=%q path=%q title=%q>\n", ref.ID, ref.Path, ref.Title)
			} else {
				fmt.Fprintf(sb, "    <ref id=%q path=%q>\n", ref.ID, ref.Path)
			}

			docPath := filepath.Join(projectRoot, filepath.FromSlash(ref.Path))
			content, readErr := os.ReadFile(docPath)
			if readErr != nil {
				fmt.Fprintf(sb, "      <note>Could not read %s: %s</note>\n", ref.Path, readErr)
			} else {
				fmt.Fprintf(sb, "      <content>\n%s\n      </content>\n", strings.TrimSpace(string(content)))
			}
			sb.WriteString("    </ref>\n")
		}
	}
	sb.WriteString("  </governing>\n\n")

	// ── Diff section ─────────────────────────────────────────────────────────
	// Diff content is not XML-escaped; it is raw git show output that the
	// reviewing agent reads as-is. The <diff> tag is a structural delimiter.
	fmt.Fprintf(sb, "  <diff rev=%q>\n%s\n  </diff>\n\n", rev, strings.TrimRight(diff, "\n"))

	// ── Instructions section ─────────────────────────────────────────────────
	instructions := strings.ReplaceAll(beadReviewInstructions, "<bead-id>", b.ID)
	instructions = strings.ReplaceAll(instructions, "<N>", fmt.Sprintf("%d", iter))
	fmt.Fprintf(sb, "  <instructions>\n%s\n  </instructions>\n", beadReviewXMLEscape(instructions))

	sb.WriteString("</bead-review>\n")
}

// beadReviewXMLEscape escapes &, <, and > for inclusion in XML-like attribute
// values and text content in the review prompt.
func beadReviewXMLEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
