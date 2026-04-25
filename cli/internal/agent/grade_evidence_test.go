package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// TestBuildGradingPrompt_ArmOutputTrimming exercises the per-arm Output cap:
// when arm.Output is much larger than the per-arm cap, the assembled prompt
// must contain a clamped slice with the canonical truncation marker.
func TestBuildGradingPrompt_ArmOutputTrimming(t *testing.T) {
	caps := evidence.Caps{
		MaxPromptBytes:      4 * 1024 * 1024,
		MaxInlinedFileBytes: 1024, // 1 KiB per-arm cap
		MaxDiffBytes:        1024,
	}
	bigOutput := strings.Repeat("A", caps.MaxInlinedFileBytes*10)
	record := &ComparisonRecord{
		ID:     "cmp-trim",
		Prompt: "do the thing",
		Arms: []ComparisonArm{
			{Harness: "agent", Model: "m1", Output: bigOutput},
		},
	}
	res := buildGradingPromptBounded(record, "", caps)
	require.False(t, res.Overflow, "with a generous prompt cap, no overflow expected")
	assert.Contains(t, res.Prompt, evidence.TruncationMarker, "clamped output must carry truncation marker")
	// The full bigOutput should NOT be present (it's clamped to per-arm cap).
	assert.NotContains(t, res.Prompt, bigOutput, "unbounded original output must not appear verbatim")
}

// TestBuildGradingPrompt_MinimumFloorPreserved verifies the floor invariant
// from FEAT-022 §5: regardless of cap pressure, the rubric, task prompt, and
// each arm's identity (harness, model) appear verbatim in the assembled
// prompt.
func TestBuildGradingPrompt_MinimumFloorPreserved(t *testing.T) {
	caps := evidence.Caps{
		MaxPromptBytes:      8 * 1024,
		MaxInlinedFileBytes: 256,
		MaxDiffBytes:        256,
	}
	// Total fixture content ≫ MaxPromptBytes: stuff each arm with a 2 MiB
	// output so the body plus all arms is roughly 20× the prompt cap.
	huge := strings.Repeat("Z", 2*1024*1024)
	record := &ComparisonRecord{
		ID:     "cmp-floor",
		Prompt: "rubric-floor task identity",
		Arms: []ComparisonArm{
			{Harness: "agent", Model: "modelA", Output: huge},
			{Harness: "claude", Model: "modelB", Output: huge},
			{Harness: "codex", Model: "modelC", Output: huge},
		},
	}
	res := buildGradingPromptBounded(record, "", caps)

	// Floor must always be present: rubric, task prompt, and each arm's identity.
	assert.Contains(t, res.Prompt, defaultGradingRubric, "rubric must be preserved verbatim")
	assert.Contains(t, res.Prompt, "rubric-floor task identity", "task prompt must be preserved verbatim")
	for _, arm := range record.Arms {
		assert.Contains(t, res.Prompt, "## Arm")
		assert.Contains(t, res.Prompt, arm.Harness, "arm harness identity must be preserved")
		assert.Contains(t, res.Prompt, arm.Model, "arm model identity must be preserved")
	}
	// The huge body must NOT appear verbatim — body got trimmed.
	assert.NotContains(t, res.Prompt, huge, "huge per-arm body must not appear verbatim")
}

// TestBuildGradingPrompt_PostRunAndToolCallsIncluded covers FEAT-022 §6 for
// the grader path: PostRunOut, PostRunOK, and ToolCalls are routed through
// evidence.ClampOutput and appear in the assembled prompt with the canonical
// truncation marker when the per-arm cap clamps them.
func TestBuildGradingPrompt_PostRunAndToolCallsIncluded(t *testing.T) {
	caps := evidence.Caps{
		MaxPromptBytes:      4 * 1024 * 1024,
		MaxInlinedFileBytes: 256, // tight per-arm cap so all three sections clamp
		MaxDiffBytes:        1024,
	}
	postRun := strings.Repeat("P", caps.MaxInlinedFileBytes*5)
	bigToolOutput := strings.Repeat("T", caps.MaxInlinedFileBytes*5)
	ok := true
	record := &ComparisonRecord{
		ID:     "cmp-postrun",
		Prompt: "task",
		Arms: []ComparisonArm{
			{
				Harness:    "agent",
				Output:     "small output",
				PostRunOut: postRun,
				PostRunOK:  &ok,
				ToolCalls: []ToolCallEntry{
					{Tool: "bash", Input: "ls", Output: bigToolOutput},
				},
			},
		},
	}
	res := buildGradingPromptBounded(record, "", caps)
	require.False(t, res.Overflow, "no overflow expected")
	assert.Contains(t, res.Prompt, "PostRunOK", "PostRunOK header must appear")
	assert.Contains(t, res.Prompt, "true", "PostRunOK boolean value must appear")
	assert.Contains(t, res.Prompt, "PostRunOut", "PostRunOut header must appear")
	assert.Contains(t, res.Prompt, "ToolCalls", "ToolCalls header must appear")
	// Both PostRunOut and ToolCalls must be clamped (truncation marker present).
	assert.Contains(t, res.Prompt, evidence.TruncationMarker, "clamped sections must carry truncation marker")
	// Originals must not appear verbatim.
	assert.NotContains(t, res.Prompt, postRun, "unbounded PostRunOut must not appear verbatim")
	assert.NotContains(t, res.Prompt, bigToolOutput, "unbounded ToolCall output must not appear verbatim")
}

// TestBuildGradingPrompt_DiffDecomposed covers the diff path: large diffs are
// decomposed and clamped via ClampDiff (per-file degradation) so they fit the
// cap.
func TestBuildGradingPrompt_DiffDecomposed(t *testing.T) {
	caps := evidence.Caps{
		MaxPromptBytes:      4 * 1024 * 1024,
		MaxInlinedFileBytes: 1024,
		MaxDiffBytes:        512,
	}
	diff := "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-old\n+new\n" + strings.Repeat("Z", 4096)
	record := &ComparisonRecord{
		ID:     "cmp-diff",
		Prompt: "task",
		Arms: []ComparisonArm{
			{Harness: "agent", Diff: diff},
		},
	}
	res := buildGradingPromptBounded(record, "", caps)
	assert.Contains(t, res.Prompt, "diff --git a/a.go", "diff header must appear after clamping")
	// The original 4096-byte run must not appear verbatim.
	assert.NotContains(t, res.Prompt, strings.Repeat("Z", 4096), "ClampDiff must bound the diff")
}

// TestGradingContextOverflow exercises the pre-dispatch short-circuit: when
// even after all per-section trimming the assembled prompt exceeds
// Caps.MaxPromptBytes, GradeFn must NOT dispatch and must emit a
// GradingEvent whose body contains the literal substring
// "compare-error: context_overflow" (FEAT-022 §7, §12 — pinned event name).
// The provider-dispatch count must remain 0.
func TestGradingContextOverflow(t *testing.T) {
	mock := &mockExecutor{output: "should not be called"}
	r := newTestRunner(mock)

	// Cap is tiny (200 bytes) but the rubric+task+identity floor alone
	// exceeds it — forces residual overflow after all body trimming.
	tinyCaps := evidence.Caps{
		MaxPromptBytes:      200,
		MaxInlinedFileBytes: 50,
		MaxDiffBytes:        50,
	}
	record := &ComparisonRecord{
		ID:     "cmp-overflow",
		Prompt: strings.Repeat("X", 1024), // task prompt is part of the floor
		Arms: []ComparisonArm{
			{Harness: "agent", Model: "modelA", Output: "ok"},
		},
	}

	var events []GradingEvent
	_, err := GradeFn(r, record, GradeOptions{
		Grader: "codex",
		Caps:   tinyCaps,
		OnEvent: func(e GradingEvent) {
			events = append(events, e)
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compare-error: context_overflow",
		"GradeFn error must carry the pinned outcome class")

	require.Len(t, events, 1, "exactly one short-circuit event expected")
	body := events[0].Kind + " " + events[0].Outcome
	assert.Contains(t, body, "compare-error: context_overflow",
		"event body must contain the pinned outcome class")
	assert.Equal(t, 0, events[0].ProviderDispatchCount,
		"provider must not have been dispatched")

	// Verify the mock executor was never invoked — definitive proof of
	// pre-dispatch short-circuit.
	assert.Empty(t, mock.lastBinary, "no provider dispatch should have occurred")
	assert.Nil(t, mock.lastArgs, "no provider dispatch arguments should have been recorded")
}

// TestEvidencePrimitiveUsage_Grade is the AST gate that prevents future drift
// in grade.go: the file must import cli/internal/evidence and any direct
// os.ReadFile call must carry an //evidence:allow-unbounded annotation.
// The base test name TestEvidencePrimitiveUsage is shared with the reviewer
// path so `go test -run TestEvidencePrimitiveUsage` matches both gates.
func TestEvidencePrimitiveUsage_Grade(t *testing.T) {
	src := "grade.go"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, nil, parser.ParseComments)
	require.NoError(t, err)

	// Import check: grade.go must import cli/internal/evidence.
	importsEvidence := false
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == `"github.com/DocumentDrivenDX/ddx/internal/evidence"` {
			importsEvidence = true
		}
	}
	assert.True(t, importsEvidence,
		"grade.go must import cli/internal/evidence (FEAT-022)")

	// AST walk: any os.ReadFile call must be marked //evidence:allow-unbounded.
	hasAllow := func(node ast.Node) bool {
		nodePos := fset.Position(node.Pos())
		for _, cg := range file.Comments {
			cgEnd := fset.Position(cg.End())
			if cgEnd.Filename != nodePos.Filename {
				continue
			}
			if cgEnd.Line >= nodePos.Line-3 && cgEnd.Line < nodePos.Line {
				for _, c := range cg.List {
					if strings.Contains(c.Text, "evidence:allow-unbounded") {
						return true
					}
				}
			}
		}
		return false
	}

	var violations []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "os" && sel.Sel.Name == "ReadFile" {
			if !hasAllow(call) {
				p := fset.Position(call.Pos())
				violations = append(violations, "os.ReadFile at line "+evidItoa(p.Line))
			}
		}
		return true
	})

	assert.Empty(t, violations,
		"grade.go must route file reads through cli/internal/evidence (or annotate with //evidence:allow-unbounded)")
}
