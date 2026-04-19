package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
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
	Rationale       string        `json:"rationale,omitempty"`
	PerAC           []ReviewAC    `json:"per_ac,omitempty"`
	RawOutput       string        `json:"raw_output,omitempty"`
	ReviewerHarness string        `json:"reviewer_harness,omitempty"`
	ReviewerModel   string        `json:"reviewer_model,omitempty"`
	SessionID       string        `json:"session_id,omitempty"`
	BaseRev         string        `json:"base_rev,omitempty"`
	ResultRev       string        `json:"result_rev,omitempty"`
	ExecutionDir    string        `json:"execution_dir,omitempty"`
	DurationMS      int           `json:"duration_ms,omitempty"`
	Error           string        `json:"error,omitempty"`
}

type ReviewAC struct {
	Number   int    `json:"number"`
	Item     string `json:"item,omitempty"`
	Grade    string `json:"grade,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type reviewArtifactManifest struct {
	Harness      string `json:"harness,omitempty"`
	Model        string `json:"model,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	BaseRev      string `json:"base_rev,omitempty"`
	ResultRev    string `json:"result_rev,omitempty"`
	Verdict      string `json:"verdict,omitempty"`
	BeadID       string `json:"bead_id,omitempty"`
	ExecutionDir string `json:"execution_dir,omitempty"`
}

type reviewArtifactResult struct {
	Verdict   string     `json:"verdict"`
	PerAC     []ReviewAC `json:"per_ac,omitempty"`
	Rationale string     `json:"rationale,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// reVerdictLine matches "### Verdict: APPROVE|REQUEST_CHANGES|BLOCK"
// anywhere in the output, case-insensitive, allowing 1–4 leading hashes.
var reVerdictLine = regexp.MustCompile(`(?im)^#{1,4}\s+Verdict:\s*(APPROVE|REQUEST_CHANGES|BLOCK)\s*$`)
var reReviewSectionHeading = regexp.MustCompile(`(?im)^#{1,4}\s+(AC Grades|Summary|Findings)\s*:?\s*$`)
var reACReference = regexp.MustCompile(`(?i)\bAC#\s*([0-9]+)\b`)

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

func ParseReviewResult(output string) (ReviewVerdict, []ReviewAC, string) {
	verdict := ParseReviewVerdict(output)
	perAC := parseReviewACTable(output)
	rationale := strings.TrimSpace(extractReviewRationale(output, perAC))
	if len(perAC) == 0 {
		perAC = synthesizeReviewACsFromRationale(verdict, rationale)
	}
	return verdict, perAC, rationale
}

func parseReviewACTable(output string) []ReviewAC {
	lines := strings.Split(output, "\n")
	var rows []ReviewAC
	inTable := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if inTable {
				break
			}
			continue
		}
		if reReviewSectionHeading.MatchString(line) {
			inTable = strings.Contains(strings.ToLower(line), "ac grades")
			continue
		}
		if !inTable || !strings.HasPrefix(line, "|") {
			continue
		}
		cols := splitMarkdownTableRow(line)
		if len(cols) < 4 {
			continue
		}
		if cols[0] == "#" || strings.HasPrefix(cols[0], "---") {
			continue
		}
		n, err := strconv.Atoi(cols[0])
		if err != nil {
			continue
		}
		rows = append(rows, ReviewAC{
			Number:   n,
			Item:     cols[1],
			Grade:    cols[2],
			Evidence: cols[3],
		})
	}
	return rows
}

func splitMarkdownTableRow(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func extractReviewRationale(output string, perAC []ReviewAC) string {
	lines := strings.Split(output, "\n")
	var summaryLines []string
	var findingLines []string
	var section string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## Review:") || strings.HasPrefix(line, "### Verdict:") || strings.HasPrefix(line, "#### Verdict:") {
			continue
		}
		if reReviewSectionHeading.MatchString(line) {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimLeft(line, "# "), ":")))
			switch heading {
			case "summary":
				section = "summary"
			case "findings":
				section = "findings"
			case "ac grades":
				section = "ac"
			default:
				section = ""
			}
			continue
		}
		if section == "ac" && strings.HasPrefix(line, "|") {
			continue
		}
		switch section {
		case "findings":
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if line != "" {
				findingLines = append(findingLines, line)
			}
		case "summary":
			summaryLines = append(summaryLines, line)
		}
	}
	if len(findingLines) > 0 {
		return strings.Join(findingLines, "\n")
	}
	if len(summaryLines) > 0 {
		return strings.Join(summaryLines, "\n")
	}
	var unmet []string
	for _, ac := range perAC {
		if strings.EqualFold(ac.Grade, string(VerdictApprove)) {
			continue
		}
		label := fmt.Sprintf("AC#%d", ac.Number)
		if ac.Item != "" {
			label += " " + ac.Item
		}
		if ac.Evidence != "" {
			unmet = append(unmet, label+": "+ac.Evidence)
		} else {
			unmet = append(unmet, label)
		}
	}
	return strings.Join(unmet, "\n")
}

func synthesizeReviewACsFromRationale(verdict ReviewVerdict, rationale string) []ReviewAC {
	if rationale == "" {
		return nil
	}
	matches := reACReference.FindAllStringSubmatch(rationale, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(matches))
	out := make([]ReviewAC, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, ReviewAC{
			Number:   n,
			Grade:    string(verdict),
			Evidence: rationale,
		})
	}
	return out
}

// SelectReviewerTier returns the tier to use for the review agent.
// Rule: max(impl_tier + 1, smart). Since smart is the ceiling, the
// reviewer always runs at smart tier regardless of the implementation tier.
func SelectReviewerTier(_ escalation.ModelTier) escalation.ModelTier {
	return escalation.TierSmart
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

// DefaultBeadReviewer implements BeadReviewer by dispatching the review
// invocation through the agent service (or a test-injected runner).
// It fetches the bead, builds the review prompt, and runs the reviewer agent.
type DefaultBeadReviewer struct {
	ProjectRoot string
	BeadStore   BeadReader
	// Service, when non-nil, is the agentlib.DdxAgent used to dispatch the
	// review invocation. Production callers leave this nil — ReviewBead
	// constructs a fresh service from ProjectRoot via NewServiceFromWorkDir.
	Service agentlib.DdxAgent
	// Runner, when non-nil, replaces the service-based dispatch path. Used by
	// tests to return canned *Result values without spinning up a real
	// service. Takes precedence over Service.
	Runner AgentRunner
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
	attemptID := GenerateAttemptID()
	artifacts, err := createArtifactBundle(r.ProjectRoot, r.ProjectRoot, attemptID)
	if err != nil {
		return nil, fmt.Errorf("reviewer: create artifact bundle: %w", err)
	}
	if err := os.WriteFile(artifacts.PromptAbs, []byte(prompt), 0o644); err != nil {
		return nil, fmt.Errorf("reviewer: write prompt artifact: %w", err)
	}

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
		reviewModel = ResolveModelTier(reviewHarness, SelectReviewerTier(escalation.TierSmart))
	}

	start := time.Now()
	runOpts := RunOptions{
		Context: ctx,
		Harness: reviewHarness,
		Model:   reviewModel,
		Prompt:  prompt,
		WorkDir: r.ProjectRoot,
	}
	result, runErr := r.dispatchReviewRun(ctx, runOpts)

	durationMS := int(time.Since(start).Milliseconds())
	if runErr != nil {
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       runErr.Error(),
			Error:           runErr.Error(),
			ReviewerHarness: reviewHarness,
			ReviewerModel:   reviewModel,
			BaseRev:         resolveReviewBaseRev(r.ProjectRoot, resultRev),
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
			DurationMS:      durationMS,
		}
		_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
			Harness:      reviewHarness,
			Model:        reviewModel,
			BaseRev:      reviewRes.BaseRev,
			ResultRev:    resultRev,
			Verdict:      string(reviewRes.Verdict),
			BeadID:       beadID,
			ExecutionDir: artifacts.DirRel,
		}, reviewArtifactResult{
			Verdict:   string(reviewRes.Verdict),
			Rationale: reviewRes.Rationale,
			Error:     reviewRes.Error,
		})
		return reviewRes, nil
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
	sessionID := ""
	if result != nil {
		output = result.Output
		sessionID = result.AgentSessionID
	}
	verdict, perAC, rationale := ParseReviewResult(output)
	baseRev := resolveReviewBaseRev(r.ProjectRoot, resultRev)
	reviewRes := &ReviewResult{
		Verdict:         verdict,
		Rationale:       rationale,
		PerAC:           perAC,
		RawOutput:       output,
		ReviewerHarness: actualHarness,
		ReviewerModel:   actualModel,
		SessionID:       sessionID,
		BaseRev:         baseRev,
		ResultRev:       resultRev,
		ExecutionDir:    artifacts.DirRel,
		DurationMS:      durationMS,
	}
	_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
		Harness:      actualHarness,
		Model:        actualModel,
		SessionID:    sessionID,
		BaseRev:      baseRev,
		ResultRev:    resultRev,
		Verdict:      string(verdict),
		BeadID:       beadID,
		ExecutionDir: artifacts.DirRel,
	}, reviewArtifactResult{
		Verdict:   string(verdict),
		PerAC:     perAC,
		Rationale: rationale,
		Error:     reviewRes.Error,
	})
	return reviewRes, nil
}

// dispatchReviewRun resolves how the review invocation should be executed.
// Resolution order matches dispatchAgentRun:
//  1. r.Runner (test injection seam) — used directly via Runner.Run.
//  2. r.Service (pre-built service) — used via RunViaServiceWith.
//  3. Fallback: construct a fresh service via NewServiceFromWorkDir(ProjectRoot)
//     and dispatch via RunViaServiceWith.
func (r *DefaultBeadReviewer) dispatchReviewRun(ctx context.Context, runOpts RunOptions) (*Result, error) {
	if r.Runner != nil {
		return r.Runner.Run(runOpts)
	}
	svc := r.Service
	if svc == nil {
		built, err := NewServiceFromWorkDir(r.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("reviewer: build agent service: %w", err)
		}
		svc = built
	}
	return RunViaServiceWith(ctx, svc, r.ProjectRoot, runOpts)
}

// gitShow runs `git show <rev>` with pathspec exclusions for execution-
// evidence noise so the review prompt's <diff> section stays bounded even
// when an old commit tracked a multi-thousand-line session log. See
// EvidenceReviewExcludePathspecs and ddx-39e27896 for the regression.
func (r *DefaultBeadReviewer) gitShow(rev string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"-C", r.ProjectRoot, "show", rev, "--", "."}, EvidenceReviewExcludePathspecs()...)
	out, err := osexec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func resolveReviewBaseRev(projectRoot, resultRev string) string {
	if resultRev == "" {
		return ""
	}
	out, err := osexec.Command("git", "-C", projectRoot, "rev-parse", resultRev+"^").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writeReviewArtifacts(artifacts *executeBeadArtifacts, manifest reviewArtifactManifest, result reviewArtifactResult) error {
	if artifacts == nil {
		return nil
	}
	if err := writeArtifactJSON(artifacts.ManifestAbs, manifest); err != nil {
		return err
	}
	return writeArtifactJSON(artifacts.ResultAbs, result)
}

func ReadReviewArtifactResult(path string) (*ReviewResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact reviewArtifactResult
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, err
	}
	return &ReviewResult{
		Verdict:   ReviewVerdict(artifact.Verdict),
		Rationale: artifact.Rationale,
		PerAC:     artifact.PerAC,
		Error:     artifact.Error,
	}, nil
}
