package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// ReviewVerdict is the outcome of a post-merge bead review.
type ReviewVerdict string

const (
	// VerdictApprove means all AC items passed; the bead stays closed.
	VerdictApprove ReviewVerdict = "APPROVE"
	// VerdictRequestChanges means some AC items need fixing; the bead is reopened.
	VerdictRequestChanges ReviewVerdict = "REQUEST_CHANGES"
	// VerdictBlock means escalation should stop; the bead is flagged for human review.
	VerdictBlock ReviewVerdict = "BLOCK"
)

// ReviewResult is the structured outcome of a post-merge bead review.
type ReviewResult struct {
	Verdict         ReviewVerdict `json:"verdict"`
	RawOutput       string        `json:"raw_output,omitempty"`
	ReviewerHarness string        `json:"reviewer_harness,omitempty"`
	ReviewerModel   string        `json:"reviewer_model,omitempty"`
	DurationMS      int           `json:"duration_ms,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// reVerdictLine matches "### Verdict: APPROVE|REQUEST_CHANGES|BLOCK"
// anywhere in the output, case-insensitive, allowing 1–4 leading hashes.
var reVerdictLine = regexp.MustCompile(`(?im)^#{1,4}\s+Verdict:\s*(APPROVE|REQUEST_CHANGES|BLOCK)\s*$`)

// ParseReviewVerdict extracts the verdict from a review agent's output.
// The expected format includes a line: ### Verdict: APPROVE | REQUEST_CHANGES | BLOCK
// Returns VerdictBlock when the output cannot be parsed (safe default: requires human attention).
func ParseReviewVerdict(output string) ReviewVerdict {
	m := reVerdictLine.FindStringSubmatch(output)
	if m == nil {
		return VerdictBlock
	}
	switch strings.ToUpper(strings.TrimSpace(m[1])) {
	case "APPROVE":
		return VerdictApprove
	case "REQUEST_CHANGES":
		return VerdictRequestChanges
	default:
		return VerdictBlock
	}
}

// SelectReviewerTier returns the tier to use for the review agent.
// Rule: max(impl_tier + 1, smart). Since smart is the ceiling, the
// reviewer always runs at smart tier regardless of the implementation tier.
func SelectReviewerTier(_ ModelTier) ModelTier {
	return TierSmart
}

// HasBeadLabel reports whether label is present in labels.
func HasBeadLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

// BeadReader can fetch a bead by ID. Implemented by *bead.Store.
type BeadReader interface {
	Get(id string) (*bead.Bead, error)
}

// BeadReviewer runs a post-merge review for a completed bead.
type BeadReviewer interface {
	ReviewBead(ctx context.Context, beadID, resultRev, implHarness, implModel string) (*ReviewResult, error)
}

// BeadReviewerFunc is a functional adapter implementing BeadReviewer.
type BeadReviewerFunc func(ctx context.Context, beadID, resultRev, implHarness, implModel string) (*ReviewResult, error)

func (f BeadReviewerFunc) ReviewBead(ctx context.Context, beadID, resultRev, implHarness, implModel string) (*ReviewResult, error) {
	return f(ctx, beadID, resultRev, implHarness, implModel)
}

// beadReviewInstructions is the review contract embedded in the prompt.
// The reviewing agent must produce APPROVE / REQUEST_CHANGES / BLOCK with
// the exact markdown format described below.
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

// BuildReviewPrompt builds the complete review prompt for a bead implementation.
// It renders the bead metadata, governing document contents, git diff, and
// review instructions into an XML-structured prompt string.
func BuildReviewPrompt(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef) string {
	var sb strings.Builder

	sb.WriteString("<bead-review>\n")

	// ── Bead section ────────────────────────────────────────────────────────
	fmt.Fprintf(&sb, "  <bead id=%q iter=%d>\n", b.ID, iter)
	fmt.Fprintf(&sb, "    <title>%s</title>\n", reviewXMLEscape(strings.TrimSpace(b.Title)))

	if desc := strings.TrimSpace(b.Description); desc != "" {
		fmt.Fprintf(&sb, "    <description>\n%s\n    </description>\n", reviewXMLEscape(desc))
	} else {
		sb.WriteString("    <description/>\n")
	}

	if acc := strings.TrimSpace(b.Acceptance); acc != "" {
		fmt.Fprintf(&sb, "    <acceptance>\n%s\n    </acceptance>\n", reviewXMLEscape(acc))
	} else {
		sb.WriteString("    <acceptance/>\n")
	}

	if len(b.Labels) > 0 {
		fmt.Fprintf(&sb, "    <labels>%s</labels>\n", reviewXMLEscape(strings.Join(b.Labels, ", ")))
	}

	sb.WriteString("  </bead>\n\n")

	// ── Governing docs section ───────────────────────────────────────────────
	sb.WriteString("  <governing>\n")
	if len(refs) == 0 {
		sb.WriteString("    <note>No governing documents found. Evaluate the diff against the acceptance criteria alone.</note>\n")
	} else {
		for _, ref := range refs {
			if ref.Title != "" {
				fmt.Fprintf(&sb, "    <ref id=%q path=%q title=%q>\n", ref.ID, ref.Path, ref.Title)
			} else {
				fmt.Fprintf(&sb, "    <ref id=%q path=%q>\n", ref.ID, ref.Path)
			}
			docPath := filepath.Join(projectRoot, filepath.FromSlash(ref.Path))
			content, readErr := os.ReadFile(docPath)
			if readErr != nil {
				fmt.Fprintf(&sb, "      <note>Could not read %s: %s</note>\n", ref.Path, readErr)
			} else {
				fmt.Fprintf(&sb, "      <content>\n%s\n      </content>\n", strings.TrimSpace(string(content)))
			}
			sb.WriteString("    </ref>\n")
		}
	}
	sb.WriteString("  </governing>\n\n")

	// ── Diff section ─────────────────────────────────────────────────────────
	fmt.Fprintf(&sb, "  <diff rev=%q>\n%s\n  </diff>\n\n", rev, strings.TrimRight(diff, "\n"))

	// ── Instructions section ─────────────────────────────────────────────────
	instructions := strings.ReplaceAll(beadReviewInstructions, "<bead-id>", b.ID)
	instructions = strings.ReplaceAll(instructions, "<N>", fmt.Sprintf("%d", iter))
	fmt.Fprintf(&sb, "  <instructions>\n%s\n  </instructions>\n", reviewXMLEscape(instructions))

	sb.WriteString("</bead-review>\n")

	return sb.String()
}

// reviewXMLEscape escapes &, <, and > for inclusion in XML text content.
func reviewXMLEscape(s string) string {
	var buf bytes.Buffer
	buf.WriteString(strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s))
	return buf.String()
}

// DefaultBeadReviewer implements BeadReviewer using a local AgentRunner.
// It fetches the bead, builds the review prompt, and runs the reviewer agent.
type DefaultBeadReviewer struct {
	ProjectRoot string
	BeadStore   BeadReader
	Runner      AgentRunner
	// Harness and Model override the reviewer harness/model.
	// When empty, Harness defaults to "claude" and Model is resolved
	// from TierSmart for the chosen harness.
	Harness string
	Model   string
}

// ReviewBead implements BeadReviewer.
func (r *DefaultBeadReviewer) ReviewBead(ctx context.Context, beadID, resultRev, implHarness, _ string) (*ReviewResult, error) {
	b, err := r.BeadStore.Get(beadID)
	if err != nil {
		return nil, fmt.Errorf("reviewer: get bead %s: %w", beadID, err)
	}

	// Fetch the git diff for the commit being reviewed.
	diff, err := r.gitShow(resultRev)
	if err != nil {
		return nil, fmt.Errorf("reviewer: git show %s: %w", resultRev, err)
	}

	// Resolve governing document references.
	refs := ResolveGoverningRefs(r.ProjectRoot, b)

	// Determine iteration number from bead events.
	iter := 1

	// Build the review prompt.
	prompt := BuildReviewPrompt(b, iter, resultRev, diff, r.ProjectRoot, refs)

	// Resolve reviewer harness and model.
	reviewHarness := r.Harness
	if reviewHarness == "" {
		if implHarness != "" {
			reviewHarness = implHarness
		} else {
			reviewHarness = "claude" // default reviewer harness
		}
	}
	reviewModel := r.Model
	if reviewModel == "" {
		reviewModel = ResolveModelTier(reviewHarness, SelectReviewerTier(TierSmart))
	}

	start := time.Now()
	result, runErr := r.Runner.Run(RunOptions{
		Harness: reviewHarness,
		Model:   reviewModel,
		Prompt:  prompt,
		WorkDir: r.ProjectRoot,
	})

	durationMS := int(time.Since(start).Milliseconds())
	if runErr != nil {
		return &ReviewResult{
			Error:           runErr.Error(),
			ReviewerHarness: reviewHarness,
			ReviewerModel:   reviewModel,
			DurationMS:      durationMS,
		}, nil
	}

	actualHarness := reviewHarness
	actualModel := reviewModel
	if result != nil {
		if result.Harness != "" {
			actualHarness = result.Harness
		}
		if result.Model != "" {
			actualModel = result.Model
		}
		durationMS = result.DurationMS
	}

	output := ""
	if result != nil {
		output = result.Output
	}
	verdict := ParseReviewVerdict(output)

	return &ReviewResult{
		Verdict:         verdict,
		RawOutput:       output,
		ReviewerHarness: actualHarness,
		ReviewerModel:   actualModel,
		DurationMS:      durationMS,
	}, nil
}

// gitShow runs `git show <rev>` and returns the output.
func (r *DefaultBeadReviewer) gitShow(rev string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := osexec.CommandContext(ctx, "git", "-C", r.ProjectRoot, "show", rev).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
