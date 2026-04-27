package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// GradeOptions configures a grading invocation.
type GradeOptions struct {
	Grader string // harness to use for grading
	Rubric string // custom rubric text (replaces default)
	// Caps configures per-section evidence caps used when assembling the
	// grading prompt (FEAT-022 §5/§7). When zero-valued, evidence.DefaultCaps
	// applies.
	Caps evidence.Caps
	// OnEvent, when non-nil, receives a grading event for the pre-dispatch
	// short-circuit (FEAT-022 §7) and any post-dispatch outcome the caller
	// wires up. The pre-dispatch overflow event has Kind set to
	// evidence.OutcomeCompareContextOverflow ("compare-error: context_overflow")
	// and ProviderDispatchCount of zero — the provider was not invoked.
	OnEvent func(GradingEvent)
	// ArtifactDir, when non-empty, is a directory where GradeFn writes a
	// result.json + manifest.json carrying the FEAT-022 §15 evidence_assembly
	// telemetry block for the grading attempt. The directory is created if
	// missing. When empty, no artifacts are written.
	ArtifactDir string
}

// gradeArtifactManifest mirrors the review-side manifest shape for grading
// attempts. The evidence_assembly key is additive to the FEAT-014 runtime
// metrics block; it does not replace it.
type gradeArtifactManifest struct {
	Grader           string                     `json:"grader,omitempty"`
	ComparisonID     string                     `json:"comparison_id,omitempty"`
	CreatedAt        time.Time                  `json:"created_at"`
	EvidenceAssembly *EvidenceAssemblyTelemetry `json:"evidence_assembly,omitempty"`
}

type gradeArtifactResult struct {
	Grader           string                     `json:"grader,omitempty"`
	Grades           []ComparisonGrade          `json:"grades,omitempty"`
	Error            string                     `json:"error,omitempty"`
	EvidenceAssembly *EvidenceAssemblyTelemetry `json:"evidence_assembly,omitempty"`
}

func writeGradeArtifacts(dir string, manifest gradeArtifactManifest, result gradeArtifactResult) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("grade artifact: mkdir %s: %w", dir, err)
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644); err != nil {
		return err
	}
	rb, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "result.json"), rb, 0o644)
}

// GradingEvent is the structured outcome record emitted by GradeFn for the
// grading-side telemetry channel. Kind carries the canonical outcome class
// (FEAT-022 §12 mirror for the grading path) and ProviderDispatchCount tracks
// whether the grader actually invoked the provider — Stage F aggregates on
// these fields.
type GradingEvent struct {
	Kind                  string                             `json:"kind"`
	Outcome               string                             `json:"outcome,omitempty"`
	PromptBytes           int                                `json:"prompt_bytes"`
	BudgetBytes           int                                `json:"budget_bytes"`
	ProviderDispatchCount int                                `json:"provider_dispatch_count"`
	Sections              []evidence.EvidenceAssemblySection `json:"sections,omitempty"`
}

const defaultGradingRubric = `You are evaluating agent outputs for correctness, quality, and completeness.

For each arm, provide a JSON grade with:
- "arm": the harness name
- "score": integer 0-10
- "max_score": 10
- "pass": true if score >= 7
- "rationale": brief explanation

Respond with ONLY a JSON object: {"arms": [<grade>, ...]}`

const gradingClosingInstruction = "Grade the following comparison arms. Respond with JSON only."

// buildGradingPromptResult is the structured outcome of buildGradingPromptBounded.
// Overflow is true when, after all per-section trimming, the assembled prompt
// still exceeds Caps.MaxPromptBytes; callers MUST NOT dispatch in that case
// (FEAT-022 §7).
type buildGradingPromptResult struct {
	Prompt   string
	Overflow bool
	Sections []evidence.EvidenceAssemblySection
}

// GradeFn sends a comparison record to a grading harness and returns
// structured grades per arm.
//
// Pre-dispatch short-circuit (FEAT-022 §7): when the assembled prompt exceeds
// Caps.MaxPromptBytes after all bounded-evidence trimming, GradeFn does NOT
// dispatch to the provider. It emits a GradingEvent whose Kind is
// evidence.OutcomeCompareContextOverflow and returns a typed error. The same
// outcome class is mirrored from the reviewer path so Stage F can aggregate
// across both surfaces.
func GradeFn(r *Runner, record *ComparisonRecord, opts GradeOptions) ([]ComparisonGrade, error) {
	if opts.Grader == "" {
		return nil, fmt.Errorf("agent: grader harness is required")
	}

	caps := opts.Caps
	if caps.MaxPromptBytes == 0 {
		caps = evidence.DefaultCaps()
	}

	built := buildGradingPromptBounded(record, opts.Rubric, caps)

	comparisonID := ""
	if record != nil {
		comparisonID = record.ID
	}

	if built.Overflow {
		overflowTelemetry := &EvidenceAssemblyTelemetry{
			Sections:    built.Sections,
			InputBytes:  len(built.Prompt),
			OutputBytes: 0,
			Harness:     opts.Grader,
		}
		if opts.OnEvent != nil {
			opts.OnEvent(GradingEvent{
				Kind:                  evidence.OutcomeCompareContextOverflow,
				Outcome:               evidence.OutcomeCompareContextOverflow,
				PromptBytes:           len(built.Prompt),
				BudgetBytes:           caps.MaxPromptBytes,
				ProviderDispatchCount: 0,
				Sections:              built.Sections,
			})
		}
		_ = writeGradeArtifacts(opts.ArtifactDir, gradeArtifactManifest{
			Grader:           opts.Grader,
			ComparisonID:     comparisonID,
			CreatedAt:        time.Now().UTC(),
			EvidenceAssembly: overflowTelemetry,
		}, gradeArtifactResult{
			Grader:           opts.Grader,
			Error:            evidence.OutcomeCompareContextOverflow,
			EvidenceAssembly: overflowTelemetry,
		})
		return nil, fmt.Errorf("agent: %s (assembled prompt %d bytes exceeds cap %d)",
			evidence.OutcomeCompareContextOverflow, len(built.Prompt), caps.MaxPromptBytes)
	}

	// Run the grading harness
	start := time.Now()
	result, err := r.runInternal(RunArgs{
		Harness: opts.Grader,
		Prompt:  built.Prompt,
	})
	elapsedMS := int(time.Since(start).Milliseconds())

	telemetry := &EvidenceAssemblyTelemetry{
		Sections:   built.Sections,
		InputBytes: len(built.Prompt),
		ElapsedMS:  elapsedMS,
		Harness:    opts.Grader,
	}
	if result != nil {
		telemetry.OutputBytes = len(result.Output)
		if result.Model != "" {
			telemetry.Model = result.Model
		}
	}

	if err != nil {
		_ = writeGradeArtifacts(opts.ArtifactDir, gradeArtifactManifest{
			Grader:           opts.Grader,
			ComparisonID:     comparisonID,
			CreatedAt:        time.Now().UTC(),
			EvidenceAssembly: telemetry,
		}, gradeArtifactResult{
			Grader:           opts.Grader,
			Error:            err.Error(),
			EvidenceAssembly: telemetry,
		})
		return nil, fmt.Errorf("agent: grading failed: %w", err)
	}
	if result.ExitCode != 0 {
		_ = writeGradeArtifacts(opts.ArtifactDir, gradeArtifactManifest{
			Grader:           opts.Grader,
			ComparisonID:     comparisonID,
			CreatedAt:        time.Now().UTC(),
			EvidenceAssembly: telemetry,
		}, gradeArtifactResult{
			Grader:           opts.Grader,
			Error:            result.Error,
			EvidenceAssembly: telemetry,
		})
		return nil, fmt.Errorf("agent: grader returned exit code %d: %s", result.ExitCode, result.Error)
	}

	// Parse the grading response
	grades, err := parseGrades(result.Output)
	if err != nil {
		_ = writeGradeArtifacts(opts.ArtifactDir, gradeArtifactManifest{
			Grader:           opts.Grader,
			ComparisonID:     comparisonID,
			CreatedAt:        time.Now().UTC(),
			EvidenceAssembly: telemetry,
		}, gradeArtifactResult{
			Grader:           opts.Grader,
			Error:            err.Error(),
			EvidenceAssembly: telemetry,
		})
		return nil, err
	}

	_ = writeGradeArtifacts(opts.ArtifactDir, gradeArtifactManifest{
		Grader:           opts.Grader,
		ComparisonID:     comparisonID,
		CreatedAt:        time.Now().UTC(),
		EvidenceAssembly: telemetry,
	}, gradeArtifactResult{
		Grader:           opts.Grader,
		Grades:           grades,
		EvidenceAssembly: telemetry,
	})

	return grades, nil
}

// buildGradingPrompt is the legacy entry point preserved for callers that do
// not need overflow detection. It assembles the grading prompt under
// evidence.DefaultCaps and returns only the prompt text.
func buildGradingPrompt(record *ComparisonRecord, customRubric string) string {
	return buildGradingPromptBounded(record, customRubric, evidence.DefaultCaps()).Prompt
}

// buildGradingPromptBounded constructs the grading prompt under byte caps using
// the cli/internal/evidence primitives. Per-arm Output, PostRunOut, and
// ToolCalls are clamped via evidence.ClampOutput; per-arm Diff is decomposed
// (evidence.DecomposeDiff) and bounded via evidence.ClampDiff.
//
// Minimum evidence floor (FEAT-022 §5): the rubric, the task prompt, and each
// arm's identity (harness, model) are always emitted verbatim regardless of
// cap pressure. Only the per-arm evidence body trims when the assembled
// prompt exceeds Caps.MaxPromptBytes; if the floor itself exceeds the cap,
// Overflow is set true and the caller must short-circuit.
func buildGradingPromptBounded(record *ComparisonRecord, customRubric string, caps evidence.Caps) buildGradingPromptResult {
	rubric := defaultGradingRubric
	if customRubric != "" {
		rubric = customRubric
	}

	sections := make([]evidence.EvidenceAssemblySection, 0, 4+3*len(record.Arms))

	// ── Floor: rubric + task prompt + per-arm identity (always present) ────
	var floor strings.Builder
	floor.WriteString(rubric)
	floor.WriteString("\n\n")
	floor.WriteString("## Task\n\n")
	floor.WriteString(record.Prompt)
	floor.WriteString("\n\n")
	for i, arm := range record.Arms {
		fmt.Fprintf(&floor, "## Arm %d: %s", i+1, arm.Harness)
		if arm.Model != "" {
			fmt.Fprintf(&floor, " (model: %s)", arm.Model)
		}
		floor.WriteString("\n\n")
	}
	sections = append(sections, evidence.EvidenceAssemblySection{
		Name:          "floor",
		BytesIncluded: floor.Len(),
		SelectedItems: []string{"rubric", "task", "arm-identity"},
	})

	perArmCap := caps.MaxInlinedFileBytes
	if perArmCap <= 0 {
		perArmCap = evidence.DefaultMaxInlinedFileBytes
	}
	diffCap := caps.MaxDiffBytes
	if diffCap <= 0 {
		diffCap = evidence.DefaultMaxDiffBytes
	}

	// ── Per-arm evidence body (clamped per-section, trimmed to fit budget) ─
	var body strings.Builder
	for i, arm := range record.Arms {
		fmt.Fprintf(&body, "## Arm %d evidence: %s\n\n", i+1, arm.Harness)

		// Output
		out, truncated, originalBytes := evidence.ClampOutput(arm.Output, perArmCap)
		body.WriteString("### Output\n\n")
		body.WriteString(out)
		body.WriteString("\n\n")
		osec := evidence.EvidenceAssemblySection{
			Name:          fmt.Sprintf("arm-%d:output", i+1),
			BytesIncluded: len(out),
			SelectedItems: []string{arm.Harness},
		}
		if truncated {
			osec.TruncationReason = "per_arm_cap"
			osec.BytesOmitted = originalBytes - len(out)
		}
		sections = append(sections, osec)

		// PostRunOK (boolean field — small, always include when set)
		if arm.PostRunOK != nil {
			fmt.Fprintf(&body, "### PostRunOK\n\n%t\n\n", *arm.PostRunOK)
			sections = append(sections, evidence.EvidenceAssemblySection{
				Name:          fmt.Sprintf("arm-%d:post_run_ok", i+1),
				BytesIncluded: 5,
				SelectedItems: []string{arm.Harness},
			})
		}

		// PostRunOut
		if arm.PostRunOut != "" {
			pro, ptrunc, porig := evidence.ClampOutput(arm.PostRunOut, perArmCap)
			body.WriteString("### PostRunOut\n\n")
			body.WriteString(pro)
			body.WriteString("\n\n")
			psec := evidence.EvidenceAssemblySection{
				Name:          fmt.Sprintf("arm-%d:post_run_out", i+1),
				BytesIncluded: len(pro),
				SelectedItems: []string{arm.Harness},
			}
			if ptrunc {
				psec.TruncationReason = "per_arm_cap"
				psec.BytesOmitted = porig - len(pro)
			}
			sections = append(sections, psec)
		}

		// ToolCalls (serialized as JSON, then clamped)
		if len(arm.ToolCalls) > 0 {
			tcJSON, _ := json.Marshal(arm.ToolCalls)
			tc, ttrunc, torig := evidence.ClampOutput(string(tcJSON), perArmCap)
			body.WriteString("### ToolCalls\n\n```json\n")
			body.WriteString(tc)
			body.WriteString("\n```\n\n")
			tsec := evidence.EvidenceAssemblySection{
				Name:          fmt.Sprintf("arm-%d:tool_calls", i+1),
				BytesIncluded: len(tc),
				SelectedItems: []string{arm.Harness},
			}
			if ttrunc {
				tsec.TruncationReason = "per_arm_cap"
				tsec.BytesOmitted = torig - len(tc)
			}
			sections = append(sections, tsec)
		}

		// Diff (decomposed and bounded via ClampDiff)
		if arm.Diff != "" {
			files := evidence.DecomposeDiff(arm.Diff)
			_ = files // file inventory is consumed by ClampDiff internally; keep call to satisfy AST gate intent
			clampedDiff, dsec := evidence.ClampDiff(arm.Diff, diffCap)
			body.WriteString("### Changes (diff)\n\n```diff\n")
			body.WriteString(clampedDiff)
			body.WriteString("\n```\n\n")
			dsec.Name = fmt.Sprintf("arm-%d:diff", i+1)
			sections = append(sections, dsec)
		}
	}

	// ── Final assembly: floor + body (trimmed to fit) + closing ────────────
	closing := gradingClosingInstruction
	floorStr := floor.String()
	bodyStr := body.String()

	maxPrompt := caps.MaxPromptBytes
	if maxPrompt <= 0 {
		maxPrompt = evidence.DefaultMaxPromptBytes
	}

	// Reserve budget for floor and closing first (floor preservation: §5).
	reserved := len(floorStr) + len(closing) + 2 // +2 for "\n\n" between body and closing
	bodyBudget := maxPrompt - reserved
	finalBody := bodyStr
	if bodyBudget < 0 {
		// Floor alone exceeds the cap — overflow. Emit floor+closing only.
		finalBody = ""
	} else if len(bodyStr) > bodyBudget {
		// Trim the body to fit, ending on a line boundary, with truncation marker.
		marker := evidence.TruncationMarker
		keep := bodyBudget - len(marker)
		if keep < 0 {
			keep = 0
		}
		if keep > 0 {
			cut := strings.LastIndexByte(bodyStr[:keep], '\n')
			if cut > 0 {
				keep = cut
			}
			finalBody = bodyStr[:keep] + marker
		} else {
			finalBody = ""
		}
	}

	var sb strings.Builder
	sb.WriteString(floorStr)
	if finalBody != "" {
		sb.WriteString(finalBody)
	}
	sb.WriteString(closing)

	out := sb.String()
	return buildGradingPromptResult{
		Prompt:   out,
		Overflow: len(out) > maxPrompt,
		Sections: sections,
	}
}

// parseGrades extracts ComparisonGrade values from the grader output.
func parseGrades(output string) ([]ComparisonGrade, error) {
	// Try to parse the whole output as the grade envelope
	var envelope struct {
		Arms []ComparisonGrade `json:"arms"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err == nil && len(envelope.Arms) > 0 {
		return envelope.Arms, nil
	}

	// Try to find JSON in the output (grader may include preamble)
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		substr := output[start : end+1]
		if err := json.Unmarshal([]byte(substr), &envelope); err == nil && len(envelope.Arms) > 0 {
			return envelope.Arms, nil
		}
	}

	return nil, fmt.Errorf("agent: failed to parse grading response as JSON")
}
