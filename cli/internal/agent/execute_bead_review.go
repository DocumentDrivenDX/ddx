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

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docprose"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
)

// beadStrictnessMode returns the strictness mode for review based on bead
// labels. kind:doc and kind:mechanical → mechanical; kind:refactor and
// kind:chore → behavior-light; default → strict.
func beadStrictnessMode(labels []string) string {
	for _, l := range labels {
		switch l {
		case "kind:doc", "kind:mechanical":
			return StrictnessMechanical
		case "kind:refactor", "kind:chore":
			return StrictnessBehavior
		}
	}
	return StrictnessStrict
}

// strictnessModeInstructions returns the per-mode evidence requirement text
// injected into the reviewer prompt.
func strictnessModeInstructions(mode string) string {
	switch mode {
	case StrictnessMechanical:
		return "mechanical — file presence, renames, or symbol evidence only; no test-name or runtime evidence required."
	case StrictnessBehavior:
		return "behavior-light — build green plus file/symbol evidence suffices; test-name match required only when an AC explicitly names a Test* function."
	default:
		return "strict — each AC must be anchored to a named Test* function or a diff-touched symbol; file-only evidence is insufficient."
	}
}

// ReviewRequestClarificationEventKind is the event kind appended to a bead
// when the reviewer returns REQUEST_CLARIFICATION.
const ReviewRequestClarificationEventKind = "review-request-clarification"

// ReviewPairingDegradedEventKind is the kind:review-pairing-degraded event
// kind appended to the bead when the post-merge reviewer was routed to the
// same provider as the implementer. R4 (different reviewer) is best-effort
// emergent: bumping MinPower usually picks a different candidate, but the
// guarantee is not absolute. The event surfaces the degradation so operators
// can see when reviewer diversity collapsed.
const ReviewPairingDegradedEventKind = "review-pairing-degraded"

// ReviewerEscalatedEventKind is the kind:reviewer-escalated event kind
// appended when the reviewer's MinPower is bumped above the baseline
// impl.ActualPower+1 due to prior review-error or review-pairing-degraded
// events for the same result_rev.
const ReviewerEscalatedEventKind = "reviewer-escalated"

// ReviewACOverrideEventKind is appended when the reviewer's per-AC grade
// contradicts the ac-check.json mechanical result for the same AC item.
// The event records the mismatch count and per-AC reasons for accuracy auditing.
const ReviewACOverrideEventKind = "review-ac-override"

// acCheckOutput is the minimal shape of .ddx/executions/<id>/ac-check.json
// needed for AC-grade mismatch detection.
type acCheckOutput struct {
	Items []struct {
		AC     int    `json:"ac"`
		Result string `json:"result"`
	} `json:"items"`
}

// countACGradeMismatches compares the reviewer's per-AC grades against
// the mechanical results from ac-check.json. Returns the number of mismatches
// (reviewer says pass where ac-check says fail, or vice versa) and a list
// of per-AC reasons. Ignores needs_judgment and error mechanical results.
func countACGradeMismatches(acCheckJSONStr string, perAC []ReviewAC) (int, []string) {
	if acCheckJSONStr == "" || len(perAC) == 0 {
		return 0, nil
	}
	var check acCheckOutput
	if err := json.Unmarshal([]byte(acCheckJSONStr), &check); err != nil {
		return 0, nil
	}
	mechByNum := make(map[int]string, len(check.Items))
	for _, item := range check.Items {
		mechByNum[item.AC] = strings.ToLower(item.Result)
	}
	count := 0
	var reasons []string
	for _, ac := range perAC {
		mech, ok := mechByNum[ac.Number]
		if !ok {
			continue
		}
		// Only compare definitive mechanical verdicts; skip needs_judgment/error.
		if mech != "pass" && mech != "fail" {
			continue
		}
		grade := strings.ToLower(ac.Grade)
		if (mech == "fail" && grade == "pass") || (mech == "pass" && grade == "fail") {
			count++
			reasons = append(reasons, fmt.Sprintf("ac%d: ac_check=%s reviewer=%s", ac.Number, mech, grade))
		}
	}
	return count, reasons
}

// ImplementerRouting captures the implementer's resolved routing details so
// the post-merge reviewer can build a paired ExecuteRequest with Role=reviewer,
// the same correlation, and MinPower bumped one above the implementer's
// actual selected power (R4 pairing).
type ImplementerRouting struct {
	Harness     string
	Provider    string
	Model       string
	ActualPower int
	// Correlation is the implementer's correlation map. The reviewer copies
	// its keys into its own dispatch metadata so events / session log /
	// aggregations can join the two calls.
	Correlation map[string]string
}

// Verdict is the outcome of a post-merge bead review.
type Verdict string

const (
	// VerdictApprove means all AC items passed; the bead stays closed.
	VerdictApprove Verdict = "APPROVE"
	// VerdictRequestChanges means some AC items need fixing; the bead stays open
	// and the pre-close gate records the verdict without closing.
	VerdictRequestChanges Verdict = "REQUEST_CHANGES"
	// VerdictBlock means escalation should stop; the bead is flagged for human
	// review while remaining open.
	VerdictBlock Verdict = "BLOCK"
	// VerdictRequestClarification means the reviewer cannot adjudicate one or
	// more needs-judgment AC items without additional operator context. Unlike
	// BLOCK it does not halt automated retry; it parks the bead to
	// status=proposed so the operator can supply the missing context.
	VerdictRequestClarification Verdict = "REQUEST_CLARIFICATION"
)

// Strictness mode constants drive the per-bead evidence requirements injected
// into the reviewer prompt. The mode is derived from bead labels.
const (
	// StrictnessStrict requires named Test* functions and diff-anchored symbols
	// per AC. Applied to kind:fix and kind:feat beads.
	StrictnessStrict = "strict"
	// StrictnessBehavior requires build-green plus file/symbol evidence.
	// Test-name match is required only when an AC explicitly names a Test*.
	// Applied to kind:refactor and kind:chore beads.
	StrictnessBehavior = "behavior-light"
	// StrictnessMechanical requires only file presence, renames, or symbol
	// evidence. Applied to kind:doc and kind:mechanical beads.
	StrictnessMechanical = "mechanical"
)

// ReviewResult is the structured outcome of a post-merge bead review.
type ReviewResult struct {
	Verdict          Verdict    `json:"verdict"`
	Rationale        string     `json:"rationale,omitempty"`
	PerAC            []ReviewAC `json:"per_ac,omitempty"`
	Findings         []Finding  `json:"findings,omitempty"`
	ProseFindings    []Finding  `json:"prose_findings,omitempty"`
	RawOutput        string     `json:"raw_output,omitempty"`
	ReviewerHarness  string     `json:"reviewer_harness,omitempty"`
	ReviewerModel    string     `json:"reviewer_model,omitempty"`
	ReviewerProvider string     `json:"reviewer_provider,omitempty"`
	ReviewerIndex    int        `json:"reviewer_index,omitempty"`
	SessionID        string     `json:"session_id,omitempty"`
	BaseRev          string     `json:"base_rev,omitempty"`
	ResultRev        string     `json:"result_rev,omitempty"`
	ExecutionDir     string     `json:"execution_dir,omitempty"`
	DurationMS       int        `json:"duration_ms,omitempty"`
	Error            string     `json:"error,omitempty"`
	CostUSD          float64    `json:"cost_usd,omitempty"`
	// InputBytes and OutputBytes are the FEAT-022 §16 byte counters used to
	// populate the compact summary appended to review and review-error event
	// bodies. InputBytes is the assembled prompt size; OutputBytes is the
	// raw reviewer output size (zero on pre-dispatch overflow / transport).
	InputBytes  int `json:"input_bytes,omitempty"`
	OutputBytes int `json:"output_bytes,omitempty"`
}

type ReviewAC struct {
	Number   int    `json:"number"`
	Item     string `json:"item,omitempty"`
	Grade    string `json:"grade,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// ReviewGroupBundle identifies the shared evidence bundle used by a
// two-slot review-group dispatch.
type ReviewGroupBundle struct {
	GroupID   string `json:"group_id"`
	DirAbs    string `json:"dir_abs"`
	DirRel    string `json:"dir_rel"`
	PromptAbs string `json:"prompt_abs"`
	PromptRel string `json:"prompt_rel"`
}

// ReviewGroupSlotResult captures one slot's runtime and structured review
// result for a review-group dispatch.
type ReviewGroupSlotResult struct {
	ReviewerIndex int             `json:"reviewer_index"`
	Runtime       AgentRunRuntime `json:"runtime"`
	Result        *ReviewResult   `json:"result,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// ReviewGroupResult is the structured output of a two-slot review-group
// dispatch.
type ReviewGroupResult struct {
	BeadID    string                  `json:"bead_id"`
	ResultRev string                  `json:"result_rev"`
	Bundle    ReviewGroupBundle       `json:"bundle"`
	Slots     []ReviewGroupSlotResult `json:"slots"`
}

// ReviewGroupDispatchMeta identifies a single slot within a review group.
type ReviewGroupDispatchMeta struct {
	GroupID       string
	ReviewerIndex int
}

type reviewerDispatchProfile struct {
	Name     string
	MinPower int
}

type reviewArtifactManifest struct {
	Harness          string                     `json:"harness,omitempty"`
	Model            string                     `json:"model,omitempty"`
	SessionID        string                     `json:"session_id,omitempty"`
	BaseRev          string                     `json:"base_rev,omitempty"`
	ResultRev        string                     `json:"result_rev,omitempty"`
	Verdict          string                     `json:"verdict,omitempty"`
	BeadID           string                     `json:"bead_id,omitempty"`
	ExecutionDir     string                     `json:"execution_dir,omitempty"`
	EvidenceAssembly *EvidenceAssemblyTelemetry `json:"evidence_assembly,omitempty"`
}

type reviewArtifactResult struct {
	Verdict          string                     `json:"verdict"`
	PerAC            []ReviewAC                 `json:"per_ac,omitempty"`
	ProseFindings    []Finding                  `json:"prose_findings,omitempty"`
	Rationale        string                     `json:"rationale,omitempty"`
	Findings         []Finding                  `json:"findings,omitempty"`
	Error            string                     `json:"error,omitempty"`
	EvidenceAssembly *EvidenceAssemblyTelemetry `json:"evidence_assembly,omitempty"`
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

// HasBeadLabelPrefix reports whether any label starts with the provided prefix.
func HasBeadLabelPrefix(labels []string, prefix string) bool {
	for _, l := range labels {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// BeadReader can fetch a bead by ID. Implemented by *bead.Store.
type BeadReader interface {
	Get(args ...any) (*bead.Bead, error)
}

// BeadEventReader reads the event history recorded against a bead.
// Implemented by *bead.Store. Used by DefaultBeadReviewer to count prior
// review escalation triggers for the same result_rev.
type BeadEventReader interface {
	Events(id string) ([]bead.BeadEvent, error)
}

// BeadReviewer runs a post-merge review for a completed bead.
type BeadReviewer interface {
	ReviewBead(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error)
}

// reviewGroupReviewer is the optional pre-close gate used by work.
// DefaultBeadReviewer implements it; test doubles can opt in when they need
// to exercise the unanimous-two-slot path explicitly.
type reviewGroupReviewer interface {
	ReviewGroup(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewGroupResult, error)
}

// beadReviewInstructions is the review contract embedded in the prompt.
// The reviewing agent must produce a single JSON object matching the
// ReviewVerdict schema (schema_version: 1). The markdown contract this
// replaces silently mis-parsed `### Verdict: APPROVE` outputs whenever the
// model echoed the prompt's options header — see review_verdict.go.
const beadReviewInstructions = `You are reviewing a bead implementation against its acceptance criteria.

## AC-Check Ratification

When an <ac-check> section is present, ratify the mechanical results rather
than re-verifying them independently from the diff:

- result="pass": confirm the evidence is credible. Override to fail only if
  the evidence is fabricated — include judgment_override_reason and a diff
  citation (file:line) in the per_ac evidence string.
- result="fail": mechanically verified failure. Grade as fail and BLOCK unless
  the commit message contains an explicit AC-Waive trailer for this AC.
- result="needs_judgment": adjudicate from the diff. If you cannot determine
  pass/fail without additional bead context from the operator, use
  REQUEST_CLARIFICATION for that AC item.
- result="error": treat as needs_judgment.

Overriding a mechanical grade (pass→fail or fail→pass) requires an explicit
judgment_override_reason note and a concrete diff citation in the evidence.

## Strictness Mode

The <strictness-mode> tag specifies per-bead evidence requirements:

- strict (kind:fix, kind:feat): each AC must be anchored to a named Test*
  function or a diff-touched symbol; file-only evidence is insufficient.
- behavior-light (kind:refactor, kind:chore): build green plus file/symbol
  evidence suffices; test-name match required only when an AC explicitly
  names a Test* function.
- mechanical (kind:doc, kind:mechanical): file presence, renames, or symbol
  evidence only; no test-name or runtime evidence required.

## Verdicts

For each acceptance-criteria (AC) item, decide whether it is implemented
correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented;
  or the diff is insufficient to evaluate.
- REQUEST_CLARIFICATION — you cannot adjudicate one or more needs_judgment AC
  items without operator clarification. Use this ONLY when the item is
  ambiguous even given the full diff. This verdict does NOT block the queue;
  it routes to the operator lane for input.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ` + "`" + `` + "`" + `` + "`" + `json … ` + "`" + `` + "`" + `` + "`" + ` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

` + "`" + `` + "`" + `` + "`" + `json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "per_ac": [
    { "number": 1, "item": "acceptance criterion text", "grade": "pass", "evidence": "file:line or test evidence" }
  ],
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
` + "`" + `` + "`" + `` + "`" + `

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK", "REQUEST_CLARIFICATION".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ` + "`" + `` + "`" + `` + "`" + `json … ` + "`" + `` + "`" + `` + "`" + ` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the verdict value anywhere except as the JSON value of the verdict field.`

// BuildReviewPromptOptions configures evidence-bounded prompt assembly.
// FEAT-022 §5/§7: caps drive per-section trimming and the pre-dispatch
// short-circuit on residual oversize.
type BuildReviewPromptOptions struct {
	Caps evidence.Caps
	// ACCheckJSON, when non-empty, is the raw JSON content of the
	// .ddx/executions/<attempt-id>/ac-check.json file produced by
	// `ddx bead ac-check`. Injected as a structured <ac-check> section so
	// the reviewer ratifies mechanical results rather than re-grepping the diff.
	ACCheckJSON string
}

// ReviewPromptOptions controls optional prompt sections.
type ReviewPromptOptions struct {
	IncludeProse bool
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

// BuildReviewPromptWithProse builds the review prompt and includes an
// advisory prose-quality section derived from the bead text using the same
// docprose checker and config path as `ddx doc prose`.
func BuildReviewPromptWithProse(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef) string {
	return BuildReviewPromptBoundedWithProse(b, iter, rev, diff, projectRoot, refs, BuildReviewPromptOptions{Caps: evidence.DefaultCaps()}).Prompt
}

// BuildReviewPromptBounded assembles the review prompt under byte caps using
// the cli/internal/evidence primitives. Governing documents are read via
// evidence.ReadFileClamped, the diff is decomposed and clamped via
// evidence.ClampDiff, and per-file inlining is bounded by
// Caps.MaxInlinedFileBytes. The minimum evidence floor (bead title,
// description, acceptance, notes, and the full changed-file inventory) is
// preserved verbatim regardless of cap pressure (FEAT-022 §5).
func BuildReviewPromptBounded(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef, opts BuildReviewPromptOptions) BuildReviewPromptResult {
	return buildReviewPromptBounded(b, iter, rev, diff, projectRoot, refs, opts, false)
}

// BuildReviewPromptBoundedWithProse assembles the review prompt with the
// optional prose-quality section enabled.
func BuildReviewPromptBoundedWithProse(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef, opts BuildReviewPromptOptions) BuildReviewPromptResult {
	return buildReviewPromptBounded(b, iter, rev, diff, projectRoot, refs, opts, true)
}

func buildReviewPromptBounded(b *bead.Bead, iter int, rev, diff, projectRoot string, refs []GoverningRef, opts BuildReviewPromptOptions, includeProse bool) BuildReviewPromptResult {
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

	// ── AC-Check section (structured mechanical results) ────────────────────
	if opts.ACCheckJSON != "" {
		acStart := sb.Len()
		sb.WriteString("  <ac-check>\n")
		sb.WriteString(opts.ACCheckJSON)
		sb.WriteString("\n  </ac-check>\n\n")
		sections = append(sections, evidence.EvidenceAssemblySection{
			Name:          "ac-check",
			BytesIncluded: sb.Len() - acStart,
			SelectedItems: []string{"ac-check.json"},
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
				fmt.Fprintf(&sb, "      <content>\n%s\n      </content>\n", evidence.DelimitUntrustedData(txt))
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
	fmt.Fprintf(&sb, "  <diff rev=%q>\n%s\n  </diff>\n\n", rev, evidence.DelimitUntrustedData(strings.TrimRight(clampedDiff, "\n")))

	if includeProse {
		proseSection, proseSectionEvidence, proseErr := buildProseReviewSection(projectRoot, b)
		if proseErr != nil {
			fmt.Fprintf(&sb, "  <prose-review advisory=%q>\n", "true")
			fmt.Fprintf(&sb, "    <note>Could not compute prose findings: %s</note>\n", reviewXMLEscape(proseErr.Error()))
			sb.WriteString("  </prose-review>\n\n")
			sections = append(sections, evidence.EvidenceAssemblySection{
				Name:          "prose-review",
				SelectedItems: []string{"bead"},
			})
		} else {
			sb.WriteString(proseSection)
			sb.WriteString("\n")
			sections = append(sections, proseSectionEvidence)
		}
	}

	// ── Strictness mode section ──────────────────────────────────────────────
	mode := beadStrictnessMode(b.Labels)
	modeStart := sb.Len()
	fmt.Fprintf(&sb, "  <strictness-mode mode=%q>%s</strictness-mode>\n\n", mode, reviewXMLEscape(strictnessModeInstructions(mode)))
	sections = append(sections, evidence.EvidenceAssemblySection{
		Name:          "strictness-mode",
		BytesIncluded: sb.Len() - modeStart,
		SelectedItems: []string{mode},
	})

	// ── Instructions section ────────────────────────────────────────────────
	instructions := strings.ReplaceAll(beadReviewInstructions, "<bead-id>", b.ID)
	instructions = strings.ReplaceAll(instructions, "<N>", fmt.Sprintf("%d", iter))
	if includeProse {
		instructions += "\n\nIf a <prose-review> section is present, add a prose_findings array to the JSON output. Keep prose findings separate from correctness findings. Prose findings are advisory by default and must not change the overall verdict."
	}
	fmt.Fprintf(&sb, "  <instructions>\n%s\n  </instructions>\n", reviewXMLEscape(instructions))

	sb.WriteString("</bead-review>\n")

	out := sb.String()
	assembled := evidence.AssembleInline([]evidence.SectionInput{{
		Name:    "review-prompt",
		Content: out,
	}}, len(out))
	return BuildReviewPromptResult{
		Prompt:   assembled.Prompt,
		Overflow: len(out) > caps.MaxPromptBytes,
		Sections: sections,
	}
}

func buildProseReviewSection(projectRoot string, b *bead.Bead) (string, evidence.EvidenceAssemblySection, error) {
	cfg, err := ddxconfig.LoadWithWorkingDir(projectRoot)
	if err != nil {
		return "", evidence.EvidenceAssemblySection{}, err
	}

	settings, err := docprose.ResolveSettings(cfg)
	if err != nil {
		return "", evidence.EvidenceAssemblySection{}, err
	}
	checker, err := docprose.NewChecker(settings.Mode, settings.Vocabulary)
	if err != nil {
		return "", evidence.EvidenceAssemblySection{}, err
	}

	var src strings.Builder
	if title := strings.TrimSpace(b.Title); title != "" {
		fmt.Fprintf(&src, "# %s\n\n", title)
	}
	if desc := strings.TrimSpace(b.Description); desc != "" {
		src.WriteString(desc)
		src.WriteString("\n\n")
	}
	if acc := strings.TrimSpace(b.Acceptance); acc != "" {
		src.WriteString("## Acceptance\n\n")
		src.WriteString(acc)
		src.WriteString("\n\n")
	}
	if notes := strings.TrimSpace(b.Notes); notes != "" {
		src.WriteString("## Notes\n\n")
		src.WriteString(notes)
		src.WriteString("\n")
	}

	findings := checker.Findings("bead.md", src.String())
	var sb strings.Builder
	sb.WriteString("  <prose-review advisory=\"true\">\n")
	fmt.Fprintf(&sb, "    <mode>%s</mode>\n", settings.Mode)
	fmt.Fprintf(&sb, "    <policy>%s</policy>\n", settings.Policy)
	if len(findings) == 0 {
		sb.WriteString("    <findings/>\n")
	} else {
		sb.WriteString("    <findings>\n")
		for _, finding := range findings {
			fmt.Fprintf(&sb, "      <finding file=%q line=%d rule_id=%q severity=%q>\n", finding.File, finding.Line, finding.RuleID, finding.Severity)
			fmt.Fprintf(&sb, "        <rationale>%s</rationale>\n", reviewXMLEscape(finding.Rationale))
			fmt.Fprintf(&sb, "        <suggested_edit>%s</suggested_edit>\n", reviewXMLEscape(finding.SuggestedEdit))
			sb.WriteString("      </finding>\n")
		}
		sb.WriteString("    </findings>\n")
	}
	sb.WriteString("  </prose-review>")

	section := evidence.EvidenceAssemblySection{
		Name:          "prose-review",
		SelectedItems: []string{"bead"},
	}
	section.BytesIncluded = len(src.String())
	return sb.String(), section, nil
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
	// Service, when non-nil, is the agentlib.FizeauService used to dispatch the
	// review invocation. Production callers leave this nil — ReviewBead
	// constructs a fresh service from ProjectRoot via NewServiceFromWorkDir.
	Service agentlib.FizeauService
	// Runner, when non-nil, replaces the service-based dispatch path. Used by
	// tests to return canned *Result values without spinning up a real
	// service. Takes precedence over Service.
	Runner AgentRunner
	// Harness and Model override the reviewer harness/model.
	// When Harness is empty, the reviewer follows the implementation harness;
	// when both are empty, dispatch is left to the agent service.
	Harness string
	Model   string
	// Caps configures the per-section evidence caps used when assembling
	// the review prompt (FEAT-022). When zero-valued, evidence.DefaultCaps
	// applies.
	Caps evidence.Caps
	// BeadEvents, when non-nil, is used to append the
	// kind:review-pairing-degraded event when the reviewer's resolved
	// provider matches the implementer's. Optional: nil disables the
	// telemetry emission while preserving the rest of the review flow.
	BeadEvents BeadEventAppender
	// EventReader, when non-nil, is used to count prior review escalation
	// triggers (review-error and review-pairing-degraded events) for the
	// same result_rev so the reviewer MinPower is bumped on retry.
	EventReader BeadEventReader
}

// BuildReviewExecuteRequest constructs the AgentRunRuntime used for the
// post-merge reviewer dispatch. It sets the reviewer harness/profile,
// attaches the implementer's correlation map (with role=reviewer overlaid),
// and derives MinPower as impl.ActualPower + 1 so routing is biased toward a
// stronger candidate than the implementer's actual selection. The returned
// runtime is missing per-call plumbing (Prompt/PromptFile, WorkDir,
// SessionLogDirOverride) — the caller fills those in before dispatching.
func BuildReviewExecuteRequest(impl ImplementerRouting, reviewerHarness, reviewerProfile string) AgentRunRuntime {
	return BuildReviewGroupExecuteRequest(impl, reviewerHarness, reviewerProfile, ReviewGroupDispatchMeta{})
}

// BuildReviewGroupExecuteRequest constructs the reviewer runtime for one
// slot in a review group. It threads the implementer's correlation facts,
// overlays reviewer role metadata, and stamps the review_group_id and
// reviewer_index when provided so downstream routing / tracing can join the
// two reviewer slots back to the same evidence bundle.
func BuildReviewGroupExecuteRequest(impl ImplementerRouting, reviewerHarness, reviewerProfile string, meta ReviewGroupDispatchMeta) AgentRunRuntime {
	correlation := map[string]string{}
	for k, v := range impl.Correlation {
		correlation[k] = v
	}
	correlation["role"] = "reviewer"
	if meta.GroupID != "" {
		correlation["review_group_id"] = meta.GroupID
		correlation["reviewer_index"] = fmt.Sprintf("%d", meta.ReviewerIndex)
	}
	if impl.Harness != "" {
		correlation["impl_harness"] = impl.Harness
	}
	if impl.Provider != "" {
		correlation["impl_provider"] = impl.Provider
	}
	if impl.Model != "" {
		correlation["impl_model"] = impl.Model
	}
	if impl.ActualPower > 0 {
		correlation["impl_actual_power"] = fmt.Sprintf("%d", impl.ActualPower)
	}
	minPower := 0
	if impl.ActualPower > 0 {
		minPower = impl.ActualPower + 1
	}
	correlationID := reviewCorrelationID(correlation)
	return AgentRunRuntime{
		HarnessOverride:     reviewerHarness,
		ProfileOverride:     reviewerProfile,
		MinPowerOverride:    minPower,
		Correlation:         correlation,
		Role:                "reviewer",
		CorrelationID:       correlationID,
		PermissionsOverride: PermissionsReadOnlyReviewer,
		ClearRoutingPins:    true,
		ClearProfile:        true,
		ClearMaxPower:       true,
	}
}

// reviewerDispatchProfile returns the fizeau profile name and MinPower to use
// for the reviewer dispatch. priorErrors is the number of prior review-error
// and review-pairing-degraded events for the same result_rev; each adds one
// ladder step to MinPower:
//   - 0 prior errors: impl.ActualPower+1 (baseline R4 pairing)
//   - 1 prior error:  impl.ActualPower+2 (one step above baseline)
//   - 2+ prior errors: top-of-ladder (SelectStrongestProfile, no floor)
func (r *DefaultBeadReviewer) reviewerDispatchProfile(ctx context.Context, impl ImplementerRouting, priorErrors int) reviewerDispatchProfile {
	floor := 0
	if impl.ActualPower > 0 {
		floor = impl.ActualPower + 1
	}
	// Escalated floor: each prior error adds one ladder step.
	escalatedFloor := floor
	if priorErrors > 0 && floor > 0 {
		escalatedFloor = floor + priorErrors
	}
	if r.Runner != nil {
		return reviewerDispatchProfile{MinPower: escalatedFloor}
	}
	selectedSvc := r.Service
	if selectedSvc == nil {
		factory := serviceRunFactory
		if factory == nil {
			factory = NewServiceFromWorkDir
		}
		built, err := factory(r.ProjectRoot)
		if err != nil {
			return reviewerDispatchProfile{MinPower: escalatedFloor}
		}
		selectedSvc = built
	}
	snap, err := LoadProfileSnapshot(ctx, selectedSvc)
	if err != nil {
		return reviewerDispatchProfile{MinPower: escalatedFloor}
	}
	// On 2+ prior errors (or 1+ error with unknown impl power), use the
	// strongest available profile without a floor restriction (top-of-ladder).
	topOfLadder := priorErrors >= 2 || (priorErrors >= 1 && floor == 0)
	name := ""
	if topOfLadder {
		name = SelectStrongestProfile(snap)
	} else if escalatedFloor > 0 {
		name = SelectStrongestProfileAbove(snap, escalatedFloor)
	} else {
		name = SelectReviewerProfile(snap)
	}
	if name == "" {
		return reviewerDispatchProfile{MinPower: escalatedFloor}
	}
	for _, profile := range snap.Profiles {
		if profile.Name == name {
			// Use the higher of the profile's own MinPower and the escalated floor
			// so top-of-ladder selection never yields a MinPower below the floor
			// we computed from the error count.
			mp := profile.MinPower
			if escalatedFloor > mp {
				mp = escalatedFloor
			}
			return reviewerDispatchProfile{Name: name, MinPower: mp}
		}
	}
	return reviewerDispatchProfile{Name: name, MinPower: escalatedFloor}
}

// SelectReviewerProfile chooses the strongest satisfiable Fizeau policy for
// reviewer work when the implementer's actual power is unknown. The policy name
// is intentionally taken from Fizeau metadata rather than assumed by DDx.
func SelectReviewerProfile(snap ProfileSnapshot) string {
	return SelectStrongestProfile(snap)
}

func (r *DefaultBeadReviewer) applyExplicitReviewerPins(runtime *AgentRunRuntime) string {
	if runtime == nil {
		return ""
	}
	if r.Model != "" {
		runtime.ModelOverride = r.Model
		return r.Model
	}
	return runtime.ProfileOverride
}

func reviewCorrelationID(correlation map[string]string) string {
	if len(correlation) == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if beadID := correlation["bead_id"]; beadID != "" {
		parts = append(parts, beadID)
	}
	if groupID := correlation["review_group_id"]; groupID != "" {
		parts = append(parts, groupID)
	} else if resultRev := correlation["result_rev"]; resultRev != "" {
		parts = append(parts, resultRev)
	}
	if reviewerIndex := correlation["reviewer_index"]; reviewerIndex != "" {
		parts = append(parts, reviewerIndex)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ":")
}

// reviewPairingDegradedBody renders the kind:review-pairing-degraded event
// body. The body carries both implementer and reviewer routing details so
// operators can see exactly which routes converged. resultRev is included so
// the event can be scoped to a specific commit when counting escalation
// triggers.
func reviewPairingDegradedBody(impl ImplementerRouting, reviewerHarness, reviewerProvider, reviewerModel string, reviewerPower int, resultRev string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "impl_harness=%s\n", impl.Harness)
	fmt.Fprintf(&b, "impl_provider=%s\n", impl.Provider)
	fmt.Fprintf(&b, "impl_model=%s\n", impl.Model)
	fmt.Fprintf(&b, "impl_actual_power=%d\n", impl.ActualPower)
	fmt.Fprintf(&b, "reviewer_harness=%s\n", reviewerHarness)
	fmt.Fprintf(&b, "reviewer_provider=%s\n", reviewerProvider)
	fmt.Fprintf(&b, "reviewer_model=%s\n", reviewerModel)
	fmt.Fprintf(&b, "reviewer_actual_power=%d\n", reviewerPower)
	fmt.Fprintf(&b, "min_power_requested=%d\n", impl.ActualPower+1)
	if resultRev != "" {
		fmt.Fprintf(&b, "result_rev=%s\n", resultRev)
	}
	return b.String()
}

// countPriorEscalationTriggers counts review-error and review-pairing-degraded
// events recorded against the bead whose body cites the given result_rev. Each
// counts as one escalation step — review-error events from transport/overflow/
// parse failures and pairing-degraded events from routing collisions both
// signal that the current reviewer powerClass is insufficient.
func countPriorEscalationTriggers(reader BeadEventReader, beadID, resultRev string) int {
	if reader == nil || resultRev == "" {
		return 0
	}
	events, err := reader.Events(beadID)
	if err != nil {
		return 0
	}
	n := 0
	for _, ev := range events {
		if ev.Kind != "review-error" && ev.Kind != ReviewPairingDegradedEventKind {
			continue
		}
		m := reResultRevField.FindStringSubmatch(ev.Body)
		if m == nil || m[1] != resultRev {
			continue
		}
		n++
	}
	return n
}

// reviewerEscalatedEventBody renders the kind:reviewer-escalated event body.
func reviewerEscalatedEventBody(newMinPower, errorCount int, resultRev string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "new_min_power=%d\n", newMinPower)
	fmt.Fprintf(&b, "error_count=%d\n", errorCount)
	fmt.Fprintf(&b, "result_rev=%s\n", resultRev)
	return b.String()
}

// ReviewBead implements BeadReviewer.
func (r *DefaultBeadReviewer) ReviewBead(ctx context.Context, beadID, resultRev string, impl ImplementerRouting) (*ReviewResult, error) {
	// Fetch the git diff for the commit being reviewed.
	diff, err := r.gitShow(resultRev)
	if err != nil {
		return nil, fmt.Errorf("reviewer: git show %s: %w", resultRev, err)
	}
	return r.reviewBeadWithDiff(ctx, beadID, resultRev, impl, diff, r.ProjectRoot)
}

// Review implements CandidateReviewer for the pre-land candidate-cycle path.
// It reviews the full base_rev..candidate_rev range and dispatches the reviewer
// inside the still-live attempt worktree.
func (r *DefaultBeadReviewer) Review(ctx context.Context, projectRoot string, candidate CandidateResult) (CandidateReviewResult, error) {
	reviewer := *r
	if reviewer.ProjectRoot == "" {
		reviewer.ProjectRoot = projectRoot
	}
	if reviewer.ProjectRoot == "" {
		return CandidateReviewResult{}, fmt.Errorf("candidate reviewer: project_root required")
	}
	beadID := candidate.Report.BeadID
	if beadID == "" {
		return CandidateReviewResult{}, fmt.Errorf("candidate reviewer: bead_id required")
	}
	diff, err := reviewer.gitDiff(candidate.Report.BaseRev, candidate.Report.ResultRev)
	if err != nil {
		return CandidateReviewResult{}, fmt.Errorf("candidate reviewer: git diff %s..%s: %w", candidate.Report.BaseRev, candidate.Report.ResultRev, err)
	}
	workDir := candidate.WorktreePath
	if workDir == "" {
		workDir = reviewer.ProjectRoot
	}
	impl := ImplementerRouting{
		Harness:     candidate.Report.Harness,
		Provider:    candidate.Report.Provider,
		Model:       candidate.Report.Model,
		ActualPower: candidate.Report.ActualPower,
		Correlation: map[string]string{
			"bead_id":       beadID,
			"attempt_id":    candidate.Report.AttemptID,
			"session_id":    candidate.Report.SessionID,
			"result_rev":    candidate.Report.ResultRev,
			"candidate_ref": candidate.Report.CandidateRef,
			"cycle_index":   fmt.Sprintf("%d", candidate.CycleIndex),
		},
	}
	// Read ac-check.json from the still-live worktree so the reviewer can
	// ratify mechanical results rather than re-grepping the diff.
	acCheckJSON := ""
	if candidate.Report.AttemptID != "" && candidate.WorktreePath != "" {
		acCheckPath := filepath.Join(candidate.WorktreePath, ExecuteBeadArtifactDir, candidate.Report.AttemptID, "ac-check.json")
		// evidence:allow-unbounded — ac-check.json is a small structured file (~1-2 kB)
		if data, readErr := os.ReadFile(acCheckPath); readErr == nil {
			acCheckJSON = string(data)
		}
	}
	group, groupErr := reviewer.reviewGroupWithDiff(ctx, beadID, candidate.Report.ResultRev, impl, diff, workDir, acCheckJSON)
	review, reduceErr := reducePreCloseReviewGroup(group)
	if review == nil {
		if reduceErr != nil {
			return CandidateReviewResult{}, reduceErr
		}
		return CandidateReviewResult{}, groupErr
	}
	out := CandidateReviewResult{
		Verdict:          string(review.Verdict),
		Rationale:        review.Rationale,
		PerAC:            append([]ReviewAC(nil), review.PerAC...),
		Findings:         append([]Finding(nil), review.Findings...),
		Classification:   ClassifyReviewFindings(review).Class,
		ReviewGroupID:    group.Bundle.GroupID,
		ReviewerIndices:  make([]int, 0, len(group.Slots)),
		ReviewerVerdicts: make([]string, 0, len(group.Slots)),
	}
	for _, slot := range group.Slots {
		out.ReviewerIndices = append(out.ReviewerIndices, slot.ReviewerIndex)
		if slot.Result == nil {
			out.ReviewerVerdicts = append(out.ReviewerVerdicts, "")
			continue
		}
		out.ReviewerVerdicts = append(out.ReviewerVerdicts, strings.TrimSpace(string(slot.Result.Verdict)))
	}
	if reduceErr != nil {
		return out, reduceErr
	}
	return out, groupErr
}

func (r *DefaultBeadReviewer) reviewBeadWithDiff(ctx context.Context, beadID, resultRev string, impl ImplementerRouting, diff, reviewWorkDir string) (*ReviewResult, error) {
	b, err := r.BeadStore.Get(ctx, beadID)
	if err != nil {
		return nil, fmt.Errorf("reviewer: get bead %s: %w", beadID, err)
	}

	// Resolve governing document references.
	refs := ResolveGoverningRefs(r.ProjectRoot, b)

	// Determine iteration number from bead events.
	iter := 1

	// Count prior escalation triggers so reviewerDispatchProfile can bump
	// MinPower on retries.
	priorErrors := countPriorEscalationTriggers(r.EventReader, beadID, resultRev)

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
		reviewProfile := r.reviewerDispatchProfile(ctx, impl, priorErrors)
		reviewRouteLabel := reviewProfile.Name
		if r.Model != "" {
			reviewRouteLabel = r.Model
		}
		overflowTelemetry := &EvidenceAssemblyTelemetry{
			Sections:    built.Sections,
			InputBytes:  len(prompt),
			OutputBytes: 0,
			Harness:     r.Harness,
			Model:       reviewRouteLabel,
		}
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Error:           evidence.OutcomeReviewContextOverflow,
			Rationale:       evidence.OutcomeReviewContextOverflow,
			ReviewerHarness: r.Harness,
			ReviewerModel:   reviewRouteLabel,
			BaseRev:         baseRev,
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
			CostUSD:         0,
			InputBytes:      overflowTelemetry.InputBytes,
			OutputBytes:     overflowTelemetry.OutputBytes,
		}
		_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
			Harness:          r.Harness,
			Model:            reviewRouteLabel,
			BaseRev:          baseRev,
			ResultRev:        resultRev,
			Verdict:          string(VerdictBlock),
			BeadID:           beadID,
			ExecutionDir:     artifacts.DirRel,
			EvidenceAssembly: overflowTelemetry,
		}, reviewArtifactResult{
			Verdict:          string(VerdictBlock),
			Error:            evidence.OutcomeReviewContextOverflow,
			EvidenceAssembly: overflowTelemetry,
		})
		return reviewRes, fmt.Errorf("reviewer: PROMPT_BUDGET_EXCEEDED/context_overflow (assembled prompt %d bytes exceeds cap %d; see %s)",
			len(prompt), caps.MaxPromptBytes, artifacts.DirRel)
	}

	// Resolve reviewer routing. An explicit reviewer harness/model wins as an
	// operator override; otherwise DDx sends only a fizeau profile hint plus
	// the reviewer MinPower floor.
	reviewHarness := r.Harness
	reviewProfile := r.reviewerDispatchProfile(ctx, impl, priorErrors)
	// Emit reviewer-escalated event when MinPower is bumped above baseline.
	if priorErrors > 0 && r.BeadEvents != nil {
		_ = r.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
			Kind:      ReviewerEscalatedEventKind,
			Summary:   fmt.Sprintf("reviewer escalated to min_power=%d after %d prior error(s)", reviewProfile.MinPower, priorErrors),
			Body:      reviewerEscalatedEventBody(reviewProfile.MinPower, priorErrors, resultRev),
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
	}

	start := time.Now()
	runRuntime := BuildReviewExecuteRequest(impl, reviewHarness, reviewProfile.Name)
	// Apply escalated MinPower: use the higher of the base R4 floor and the
	// escalated profile floor so retries reach a stronger reviewer powerClass.
	if reviewProfile.MinPower > runRuntime.MinPowerOverride {
		runRuntime.MinPowerOverride = reviewProfile.MinPower
	}
	reviewRouteLabel := r.applyExplicitReviewerPins(&runRuntime)
	runRuntime.Prompt = prompt
	runRuntime.WorkDir = reviewWorkDir
	result, runErr := r.dispatchReviewRun(ctx, runRuntime)

	durationMS := int(time.Since(start).Milliseconds())
	if runErr != nil {
		// Transport-class failure (FEAT-022 §12): network or provider-side
		// error. Surface as a typed review-error so the loop classifies and
		// counts it correctly, rather than masquerading as a BLOCK verdict.
		transportTelemetry := &EvidenceAssemblyTelemetry{
			Sections:    built.Sections,
			InputBytes:  len(prompt),
			OutputBytes: 0,
			ElapsedMS:   durationMS,
			Harness:     reviewHarness,
			Model:       reviewRouteLabel,
		}
		reviewRes := &ReviewResult{
			Verdict:         VerdictBlock,
			Rationale:       runErr.Error(),
			Error:           evidence.OutcomeReviewTransport,
			ReviewerHarness: reviewHarness,
			ReviewerModel:   reviewRouteLabel,
			BaseRev:         resolveReviewBaseRev(r.ProjectRoot, resultRev),
			ResultRev:       resultRev,
			ExecutionDir:    artifacts.DirRel,
			DurationMS:      durationMS,
			CostUSD:         resultCost(result),
			InputBytes:      transportTelemetry.InputBytes,
			OutputBytes:     transportTelemetry.OutputBytes,
		}
		_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
			Harness:          reviewHarness,
			Model:            reviewRouteLabel,
			BaseRev:          reviewRes.BaseRev,
			ResultRev:        resultRev,
			Verdict:          string(reviewRes.Verdict),
			BeadID:           beadID,
			ExecutionDir:     artifacts.DirRel,
			EvidenceAssembly: transportTelemetry,
		}, reviewArtifactResult{
			Verdict:          string(reviewRes.Verdict),
			Rationale:        reviewRes.Rationale,
			Error:            reviewRes.Error,
			EvidenceAssembly: transportTelemetry,
		})
		return reviewRes, fmt.Errorf("reviewer: %s: %w", evidence.OutcomeReviewTransport, runErr)
	}

	actualHarness := reviewHarness
	actualModel := reviewRouteLabel
	actualProvider := ""
	actualPower := 0
	if result != nil {
		if result.Harness != "" {
			actualHarness = result.Harness
		}
		if result.Model != "" {
			actualModel = result.Model
		}
		actualProvider = result.Provider
		actualPower = result.ActualPower
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
	// a typed review-error class — the work leaves the bead open for
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
	telemetry := &EvidenceAssemblyTelemetry{
		Sections:    built.Sections,
		InputBytes:  len(prompt),
		OutputBytes: len(output),
		ElapsedMS:   durationMS,
		Harness:     actualHarness,
		Model:       actualModel,
	}
	reviewRes := &ReviewResult{
		Verdict:          strictVerdict,
		Rationale:        rationale,
		PerAC:            parsed.PerAC,
		Findings:         findings,
		ProseFindings:    parsed.ProseFindings,
		RawOutput:        output,
		ReviewerHarness:  actualHarness,
		ReviewerModel:    actualModel,
		ReviewerProvider: actualProvider,
		SessionID:        sessionID,
		BaseRev:          baseRev,
		ResultRev:        resultRev,
		ExecutionDir:     artifacts.DirRel,
		DurationMS:       durationMS,
		CostUSD:          resultCost(result),
		InputBytes:       telemetry.InputBytes,
		OutputBytes:      telemetry.OutputBytes,
	}

	// R4 pairing-degradation telemetry: when MinPower=actualPower+1 fails
	// to push the reviewer onto a different provider than the implementer,
	// surface a kind:review-pairing-degraded event so operators can see the
	// regression. The result_rev is included in the body so the event can be
	// counted as an escalation trigger for subsequent dispatches.
	// Best-effort only — emission failures never affect the review outcome.
	if r.BeadEvents != nil && actualProvider != "" && impl.Provider != "" && actualProvider == impl.Provider {
		_ = r.BeadEvents.AppendEvent(beadID, bead.BeadEvent{
			Kind:      ReviewPairingDegradedEventKind,
			Summary:   fmt.Sprintf("reviewer pinned to same provider as implementer (%s)", impl.Provider),
			Body:      reviewPairingDegradedBody(impl, actualHarness, actualProvider, actualModel, actualPower, resultRev),
			Source:    "ddx work",
			CreatedAt: time.Now().UTC(),
		})
	}
	_ = writeReviewArtifacts(artifacts, reviewArtifactManifest{
		Harness:          actualHarness,
		Model:            actualModel,
		SessionID:        sessionID,
		BaseRev:          baseRev,
		ResultRev:        resultRev,
		Verdict:          string(strictVerdict),
		BeadID:           beadID,
		ExecutionDir:     artifacts.DirRel,
		EvidenceAssembly: telemetry,
	}, reviewArtifactResult{
		Verdict:          string(strictVerdict),
		Rationale:        rationale,
		PerAC:            parsed.PerAC,
		Findings:         findings,
		ProseFindings:    parsed.ProseFindings,
		Error:            reviewRes.Error,
		EvidenceAssembly: telemetry,
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

// dispatchReviewRun is a thin SD-024 wrapper around dispatchViaResolvedConfig
// for the post-merge reviewer. The reviewer carries no persistent
// ResolvedConfig of its own — the durable knobs that affect a review
// invocation (timeout, provider, evidence caps) are read from the project's
// .ddx/config.yaml via LoadAndResolve, while reviewer-powerClass harness/model
// are pinned via the runtime override fields. Resolution order matches the
// execute-bead worker (runner > pre-built service > fresh service).
func (r *DefaultBeadReviewer) dispatchReviewRun(ctx context.Context, runtime AgentRunRuntime) (*Result, error) {
	rcfg, _ := ddxconfig.LoadAndResolve(r.ProjectRoot, ddxconfig.CLIOverrides{})
	return dispatchViaResolvedConfig(ctx, r.ProjectRoot, r.Service, r.Runner, rcfg, runtime)
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

func (r *DefaultBeadReviewer) gitDiff(baseRev, resultRev string) (string, error) {
	if strings.TrimSpace(baseRev) == "" || strings.TrimSpace(resultRev) == "" {
		return "", fmt.Errorf("base_rev and result_rev are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"diff", baseRev, resultRev, "--", "."}, EvidenceReviewExcludePathspecs()...)
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

func resultCost(result *Result) float64 {
	if result == nil {
		return 0
	}
	return result.CostUSD
}
