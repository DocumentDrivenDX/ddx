package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// Verdict is the outcome of a post-merge bead review.
type Verdict string

const (
	// VerdictApprove means all AC items passed; the bead stays closed.
	VerdictApprove Verdict = "APPROVE"
	// VerdictRequestChanges means some AC items need fixing; the bead is reopened.
	VerdictRequestChanges Verdict = "REQUEST_CHANGES"
	// VerdictBlock means escalation should stop; the bead is flagged for human review.
	VerdictBlock Verdict = "BLOCK"
)

// ReviewResult is the structured outcome of a post-merge bead review.
type ReviewResult struct {
	Verdict         Verdict    `json:"verdict"`
	Rationale       string     `json:"rationale,omitempty"`
	PerAC           []ReviewAC `json:"per_ac,omitempty"`
	RawOutput       string     `json:"raw_output,omitempty"`
	ReviewerHarness string     `json:"reviewer_harness,omitempty"`
	ReviewerModel   string     `json:"reviewer_model,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	BaseRev         string     `json:"base_rev,omitempty"`
	ResultRev       string     `json:"result_rev,omitempty"`
	ExecutionDir    string     `json:"execution_dir,omitempty"`
	DurationMS      int        `json:"duration_ms,omitempty"`
	Error           string     `json:"error,omitempty"`
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
	Findings  []Finding  `json:"findings,omitempty"`
	Error     string     `json:"error,omitempty"`
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
// The reviewing agent must produce a single JSON object matching the
// ReviewVerdict schema (schema_version: 1). The markdown contract this
// replaces silently mis-parsed `### Verdict: APPROVE` outputs whenever the
// model echoed the prompt's options header — see review_verdict.go.
const beadReviewInstructions = `You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ` + "`" + `` + "`" + `` + "`" + `json … ` + "`" + `` + "`" + `` + "`" + ` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

` + "`" + `` + "`" + `` + "`" + `json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
` + "`" + `` + "`" + `` + "`" + `

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ` + "`" + `` + "`" + `` + "`" + `json … ` + "`" + `` + "`" + `` + "`" + ` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.`

// BuildReviewPromptOptions configures evidence-bounded prompt assembly.
// FEAT-022 §5/§7: caps drive per-section trimming and the pre-dispatch
// short-circuit on residual oversize.
type BuildReviewPromptOptions struct {
	Caps evidence.Caps
}

// BuildReviewPromptResult is the structured output of BuildReviewPromptBounded.
// Overflow is true when, after all per-section trimming, the assembled
// prompt still exceeds Caps.MaxPromptBytes; callers MUST NOT dispatch in
// that case (FEAT-022 §7).
type BuildReviewPromptResult struct {
	Prompt   string
	Overflow bool
	Sections []evidence.EvidenceAssemblySection
}

// BuildReviewPrompt builds the complete review prompt for a bead implementation
// using default caps. Callers needing overflow detection use
// BuildReviewPromptBounded.
func BuildReviewPrompt(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef) string {
	return BuildReviewPromptBounded(b, iter, rev, diff, projectRoot, refs, BuildReviewPromptOptions{Caps: evidence.DefaultCaps()}).Prompt
}

// BuildReviewPromptBounded assembles the review prompt under byte caps using
// the cli/internal/evidence primitives. Governing documents are read via
// evidence.ReadFileClamped, the diff is decomposed and clamped via
// evidence.ClampDiff, and per-file inlining is bounded by
// Caps.MaxInlinedFileBytes. The minimum evidence floor (bead title,
// description, acceptance, notes, and the full changed-file inventory) is
// preserved verbatim regardless of cap pressure (FEAT-022 §5).
func BuildReviewPromptBounded(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef, opts BuildReviewPromptOptions) BuildReviewPromptResult {
	caps := opts.Caps
	if caps.MaxPromptBytes == 0 {
		caps = evidence.DefaultCaps()
	}

	var sb strings.Builder
	sections := []evidence.EvidenceAssemblySection{}

	sb.WriteString("<bead-review>\n")

	// ── Bead section (floor) ────────────────────────────────────────────────
	beadStart := sb.Len()
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

	if notes := strings.TrimSpace(b.Notes); notes != "" {
		fmt.Fprintf(&sb, "    <notes>\n%s\n    </notes>\n", reviewXMLEscape(notes))
	}

	if len(b.Labels) > 0 {
		fmt.Fprintf(&sb, "    <labels>%s</labels>\n", reviewXMLEscape(strings.Join(b.Labels, ", ")))
	}

	sb.WriteString("  </bead>\n\n")
	sections = append(sections, evidence.EvidenceAssemblySection{
		Name:          "bead",
		BytesIncluded: sb.Len() - beadStart,
		SelectedItems: []string{"bead"},
	})

	// ── Changed-file inventory (floor) ──────────────────────────────────────
	allFiles := evidence.DecomposeDiff(diff)
	if len(allFiles) > 0 {
		invStart := sb.Len()
		sb.WriteString("  <changed-files>\n")
		paths := make([]string, 0, len(allFiles))
		for _, f := range allFiles {
			fmt.Fprintf(&sb, "    <file>%s</file>\n", reviewXMLEscape(f.Path))
			paths = append(paths, f.Path)
		}
		sb.WriteString("  </changed-files>\n\n")
		sections = append(sections, evidence.EvidenceAssemblySection{
			Name:          "changed-files",
			BytesIncluded: sb.Len() - invStart,
			SelectedItems: paths,
		})
	}

	// ── Governing docs section (clamped per-doc) ────────────────────────────
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
			content, truncated, originalBytes, readErr := evidence.ReadFileClamped(docPath, caps.MaxGoverningDocBytes)
			if readErr != nil {
				fmt.Fprintf(&sb, "      <note>Could not read %s: %s</note>\n", ref.Path, readErr)
				sections = append(sections, evidence.EvidenceAssemblySection{
					Name:             "governing:" + ref.ID,
					TruncationReason: "read_error",
					OmittedItems:     []string{ref.Path},
				})
			} else {
				txt := strings.TrimSpace(string(content))
				if truncated {
					txt += evidence.TruncationMarker
				}
				fmt.Fprintf(&sb, "      <content>\n%s\n      </content>\n", txt)
				sec := evidence.EvidenceAssemblySection{
					Name:          "governing:" + ref.ID,
					BytesIncluded: len(content),
					SelectedItems: []string{ref.Path},
				}
				if truncated {
					sec.TruncationReason = "governing_doc_cap"
					sec.BytesOmitted = int(originalBytes) - len(content)
				}
				sections = append(sections, sec)
			}
			sb.WriteString("    </ref>\n")
		}
	}
	sb.WriteString("  </governing>\n\n")

	// ── Diff section (ranked, per-file inlined cap, total clamp) ────────────
	rankedDiff := rankAndDegradeDiffForReview(allFiles, diff, b, refs, caps.MaxInlinedFileBytes)
	clampedDiff, diffSection := evidence.ClampDiff(rankedDiff, caps.MaxDiffBytes)
	diffSection.Name = "diff"
	sections = append(sections, diffSection)
	fmt.Fprintf(&sb, "  <diff rev=%q>\n%s\n  </diff>\n\n", rev, strings.TrimRight(clampedDiff, "\n"))

	// ── Instructions section ────────────────────────────────────────────────
	instructions := strings.ReplaceAll(beadReviewInstructions, "<bead-id>", b.ID)
	instructions = strings.ReplaceAll(instructions, "<N>", fmt.Sprintf("%d", iter))
	fmt.Fprintf(&sb, "  <instructions>\n%s\n  </instructions>\n", reviewXMLEscape(instructions))

	sb.WriteString("</bead-review>\n")

	out := sb.String()
	return BuildReviewPromptResult{
		Prompt:   out,
		Overflow: len(out) > caps.MaxPromptBytes,
		Sections: sections,
	}
}

// rankAndDegradeDiffForReview reorders diff files so that AC-referenced files
// appear first, then files matching governing-ref paths, then others; and
// degrades any individual file body that exceeds maxInlinedFileBytes to
// "stat + hunk headers only" so a single oversize file cannot dominate the
// diff section. The returned string is a re-serialized unified diff suitable
// for evidence.ClampDiff. FEAT-022 §1.
func rankAndDegradeDiffForReview(files []evidence.DiffFile, originalDiff string, b *bead.Bead, refs []GoverningRef, maxInlinedFileBytes int) string {
	if len(files) == 0 {
		return originalDiff
	}
	rank := func(f evidence.DiffFile) int {
		if f.Path == "" {
			return 3
		}
		if mentionsPathLike(b.Acceptance, f.Path) {
			return 0
		}
		for _, r := range refs {
			if pathsRelated(r.Path, f.Path) {
				return 1
			}
		}
		return 2
	}
	ordered := make([]evidence.DiffFile, len(files))
	copy(ordered, files)
	sort.SliceStable(ordered, func(i, j int) bool { return rank(ordered[i]) < rank(ordered[j]) })

	var b2 strings.Builder
	for _, f := range ordered {
		body := f.Body
		if maxInlinedFileBytes > 0 && len(body) > maxInlinedFileBytes {
			body = degradeDiffFileBody(f)
		}
		b2.WriteString(body)
	}
	return b2.String()
}

// degradeDiffFileBody returns a stat + hunk-headers only rendering of a diff
// file (FEAT-022 §1: large-file degradation). Used when a single file's body
// exceeds Caps.MaxInlinedFileBytes — preserves enough structure for the
// reviewer to know what changed without inlining the full content.
func degradeDiffFileBody(f evidence.DiffFile) string {
	var b strings.Builder
	b.WriteString("diff --git a/")
	b.WriteString(f.Path)
	b.WriteString(" b/")
	b.WriteString(f.Path)
	b.WriteString("\n")
	if f.Stat != "" {
		b.WriteString(f.Stat)
		b.WriteString("\n")
	}
	for _, h := range f.HunkHeaders {
		b.WriteString(h)
		b.WriteString("\n")
	}
	b.WriteString("[…file body omitted by ddx evidence cap…]\n")
	return b.String()
}

// mentionsPathLike returns true when text mentions the file path or its
// basename (a coarse heuristic — the reviewer only needs ranking, not
// exact symbolic resolution).
func mentionsPathLike(text, p string) bool {
	if text == "" || p == "" {
		return false
	}
	if strings.Contains(text, p) {
		return true
	}
	base := path.Base(p)
	return base != "" && base != "." && strings.Contains(text, base)
}

// pathsRelated returns true when refPath and filePath share a directory
// prefix or basename, used to associate diff files with governing
// documents.
func pathsRelated(refPath, filePath string) bool {
	if refPath == "" || filePath == "" {
		return false
	}
	refDir := path.Dir(refPath)
	if refDir != "" && refDir != "." && strings.HasPrefix(filePath, refDir+"/") {
		return true
	}
	if path.Base(refPath) == path.Base(filePath) {
		return true
	}
	return false
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
	// Caps configures the per-section evidence caps used when assembling
	// the review prompt (FEAT-022). When zero-valued, evidence.DefaultCaps
	// applies.
	Caps evidence.Caps
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

	// Build the review prompt under evidence caps.
	caps := r.Caps
	if caps.MaxPromptBytes == 0 {
		caps = evidence.DefaultCaps()
	}
	built := BuildReviewPromptBounded(b, iter, resultRev, diff, r.ProjectRoot, refs, BuildReviewPromptOptions{Caps: caps})
	prompt := built.Prompt
	attemptID := GenerateAttemptID()
	artifacts, err := createArtifactBundle(r.ProjectRoot, r.ProjectRoot, attemptID)
	if err != nil {
		return nil, fmt.Errorf("reviewer: create artifact bundle: %w", err)
	}
	if err := os.WriteFile(artifacts.PromptAbs, []byte(prompt), 0o644); err != nil {
		return nil, fmt.Errorf("reviewer: write prompt artifact: %w", err)
	}

	// Pre-dispatch short-circuit (FEAT-022 §7): if the assembled prompt
	// exceeds MaxPromptBytes after all per-section trimming, skip the
	// provider dispatch and return a typed review-error: context_overflow.
	// The bead is NOT closed — the loop's review-error event-append path
	// records the failure and leaves the bead open for retry.
	if built.Overflow {
		baseRev := resolveReviewBaseRev(r.ProjectRoot, resultRev)
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Error:           evidence.OutcomeReviewContextOverflow,
			Rationale:       evidence.OutcomeReviewContextOverflow,
			ReviewerHarness: r.Harness,
			ReviewerModel:   r.Model,
			BaseRev:         baseRev,
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
		}
		_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
			Harness:      r.Harness,
			Model:        r.Model,
			BaseRev:      baseRev,
			ResultRev:    resultRev,
			Verdict:      string(VerdictBlock),
			BeadID:       beadID,
			ExecutionDir: artifacts.DirRel,
		}, reviewArtifactResult{
			Verdict: string(VerdictBlock),
			Error:   evidence.OutcomeReviewContextOverflow,
		})
		return reviewRes, fmt.Errorf("reviewer: %s (assembled prompt %d bytes exceeds cap %d; see %s)",
			evidence.OutcomeReviewContextOverflow, len(prompt), caps.MaxPromptBytes, artifacts.DirRel)
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
		// Transport-class failure (FEAT-022 §12): network or provider-side
		// error. Surface as a typed review-error so the loop classifies and
		// counts it correctly, rather than masquerading as a BLOCK verdict.
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       runErr.Error(),
			Error:           evidence.OutcomeReviewTransport,
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
		return reviewRes, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewTransport, runErr)
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
	// Strict JSON parse: replaces the legacy markdown extractor that silently
	// pulled "BLOCK" from the prompt's options-header line whenever the model
	// echoed it back (the upstream-report regression). On parse error we emit
	// a typed review-error class — the execute-loop reopens the bead for
	// retry rather than mis-recording a BLOCK verdict.
	parsed, parseErr := ParseReviewVerdict([]byte(output))
	var strictVerdict Verdict
	var rationale string
	var findings []Finding
	if parseErr == nil {
		strictVerdict = parsed.Verdict
		rationale = strings.TrimSpace(parsed.Summary)
		findings = parsed.Findings
		if rationale == "" && len(findings) > 0 {
			parts := make([]string, 0, len(findings))
			for _, f := range findings {
				line := strings.TrimSpace(f.Summary)
				if line == "" {
					continue
				}
				if f.Location != "" {
					line = f.Location + ": " + line
				}
				parts = append(parts, line)
			}
			rationale = strings.Join(parts, "\n")
		}
	}
	baseRev := resolveReviewBaseRev(r.ProjectRoot, resultRev)
	reviewRes := &ReviewResult{
		Verdict:         strictVerdict,
		Rationale:       rationale,
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
		Verdict:      string(strictVerdict),
		BeadID:       beadID,
		ExecutionDir: artifacts.DirRel,
	}, reviewArtifactResult{
		Verdict:   string(strictVerdict),
		Rationale: rationale,
		Findings:  findings,
		Error:     reviewRes.Error,
	})
	if parseErr != nil {
		// FEAT-022 §12: distinguish provider_empty (zero bytes) from
		// unparseable (text without a recognizable JSON contract). Operators
		// triage these differently — provider_empty often signals a context-
		// window or backend availability issue, while unparseable signals a
		// reviewer-prompt or output-format drift.
		class := evidence.OutcomeReviewUnparseable
		if strings.TrimSpace(output) == "" {
			class = evidence.OutcomeReviewProviderEmpty
		}
		reviewRes.Error = class
		return reviewRes, fmt.Errorf("reviewer: %s: %w (raw output %d bytes; see %s)", class, parseErr, len(output), artifacts.DirRel)
	}
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
	args := append([]string{"show", rev, "--", "."}, EvidenceReviewExcludePathspecs()...)
	out, err := internalgit.Command(ctx, r.ProjectRoot, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func resolveReviewBaseRev(projectRoot, resultRev string) string {
	if resultRev == "" {
		return ""
	}
	out, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", resultRev+"^").Output()
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
	//evidence:allow-unbounded — review result artifacts are small JSON
	// documents written by writeReviewArtifacts; bounded by construction.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact reviewArtifactResult
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, err
	}
	return &ReviewResult{
		Verdict:   Verdict(artifact.Verdict),
		Rationale: artifact.Rationale,
		PerAC:     artifact.PerAC,
		Error:     artifact.Error,
	}, nil
}
