package agent

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCaps returns small caps suitable for forcing trimming/overflow paths
// in tests without allocating multi-MB fixtures on every run. Production
// caps come from evidence.DefaultCaps; the bounded-assembly contract is
// expressed against the resolved Caps value, not the absolute sizes.
func testCaps() evidence.Caps {
	return evidence.Caps{
		MaxPromptBytes:       64 * 1024, // 64 KiB
		MaxInlinedFileBytes:  4 * 1024,  // 4 KiB
		MaxDiffBytes:         16 * 1024, // 16 KiB
		MaxGoverningDocBytes: 2 * 1024,  // 2 KiB
	}
}

// makeBigBead returns a bead whose floor (Title/Description/Acceptance/
// Notes) names path/to/foo.go so ranking pulls it ahead of unreferenced
// files in the diff.
func makeBigBead() *bead.Bead {
	return &bead.Bead{
		ID:          "ddx-evid-1",
		Title:       "evidence-bounded review prompt",
		Description: "ensure path/to/foo.go is preserved",
		Acceptance:  "AC#1: path/to/foo.go has handler\nAC#2: helpers/big.go also covered",
		Notes:       "internal note: floor must survive cap pressure",
		Labels:      []string{"area:agent"},
	}
}

func TestBuildReviewPrompt_GoverningDocClamped(t *testing.T) {
	caps := testCaps()
	root := t.TempDir()
	docRel := "docs/big.md"
	docAbs := filepath.Join(root, filepath.FromSlash(docRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(docAbs), 0o755))
	huge := strings.Repeat("X", caps.MaxGoverningDocBytes*10)
	require.NoError(t, os.WriteFile(docAbs, []byte(huge), 0o644))

	b := makeBigBead()
	refs := []GoverningRef{{ID: "FEAT-1", Path: docRel, Title: "Big"}}
	res := BuildReviewPromptBounded(b, 1, "rev1", "", root, refs, BuildReviewPromptOptions{Caps: caps})

	// Output must be bounded; governing doc must include the truncation marker.
	assert.LessOrEqual(t, len(res.Prompt), caps.MaxPromptBytes,
		"assembled prompt must respect MaxPromptBytes when only governing-doc is oversize")
	assert.Contains(t, res.Prompt, evidence.TruncationMarker,
		"clamped governing doc must include the canonical truncation marker")
	// The full 10× governing-doc body must NOT survive verbatim.
	assert.NotContains(t, res.Prompt, strings.Repeat("X", caps.MaxGoverningDocBytes+1),
		"governing-doc cap must drop bytes beyond MaxGoverningDocBytes")
}

func TestBuildReviewPrompt_DiffClamped(t *testing.T) {
	caps := testCaps()
	// Synthesize many smaller files so per-file size doesn't trip the
	// MaxInlinedFileBytes degradation; only the total-diff cap kicks in.
	var diff strings.Builder
	perFileBody := strings.Repeat("body line\n", 50)
	for i := 0; i < 800; i++ {
		diff.WriteString("diff --git a/")
		path := filepathSafe(i)
		diff.WriteString(path)
		diff.WriteString(" b/")
		diff.WriteString(path)
		diff.WriteString("\n@@ -1 +1,1 @@\n")
		diff.WriteString(perFileBody)
	}
	require.Greater(t, diff.Len(), caps.MaxDiffBytes*5,
		"fixture diff must be > 5× MaxDiffBytes to exercise the cap")

	b := makeBigBead()
	res := BuildReviewPromptBounded(b, 1, "rev2", diff.String(), t.TempDir(), nil, BuildReviewPromptOptions{Caps: caps})

	// Diff section must be clamped: marker present, output total bounded.
	assert.Contains(t, res.Prompt, evidence.TruncationMarker)
	// The whole prompt may exceed MaxPromptBytes only when MaxDiffBytes alone
	// fills the budget; here MaxDiffBytes < MaxPromptBytes so we expect fit.
	assert.LessOrEqual(t, len(res.Prompt), caps.MaxPromptBytes)
}

func TestBuildReviewPrompt_FloorPreservedUnderPressure(t *testing.T) {
	caps := testCaps()
	// Build a >20× MaxPromptBytes diff with many files.
	var diff strings.Builder
	perFile := strings.Repeat("line\n", 500)
	target := caps.MaxPromptBytes * 20
	i := 0
	for diff.Len() < target {
		diff.WriteString("diff --git a/")
		p := filepathSafe(i)
		diff.WriteString(p)
		diff.WriteString(" b/")
		diff.WriteString(p)
		diff.WriteString("\n@@ -1 +1,1 @@\n")
		diff.WriteString(perFile)
		i++
	}

	b := makeBigBead()
	res := BuildReviewPromptBounded(b, 1, "rev3", diff.String(), t.TempDir(), nil, BuildReviewPromptOptions{Caps: caps})

	// Floor preserved verbatim regardless of pressure.
	assert.Contains(t, res.Prompt, b.Title)
	assert.Contains(t, res.Prompt, b.Description)
	assert.Contains(t, res.Prompt, "AC#1: path/to/foo.go has handler")
	assert.Contains(t, res.Prompt, "AC#2: helpers/big.go also covered")
	assert.Contains(t, res.Prompt, "internal note: floor must survive cap pressure")

	// Full changed-file inventory present verbatim.
	for j := 0; j < i; j++ {
		assert.Contains(t, res.Prompt, "<file>"+filepathSafe(j)+"</file>",
			"file %d must appear in the changed-file inventory", j)
	}
}

func TestBuildReviewPrompt_RankedDiffInclusion(t *testing.T) {
	caps := testCaps()
	caps.MaxDiffBytes = 1 << 20 // generous; ranking matters, not clamping
	caps.MaxInlinedFileBytes = 1 << 20
	caps.MaxPromptBytes = 1 << 20

	// Three files: unreferenced, AC-referenced, governing-ref-matched.
	mkFile := func(p string) string {
		return "diff --git a/" + p + " b/" + p + "\n@@ -1 +1 @@\n+marker for " + p + "\n"
	}
	diff := mkFile("z/unrelated.go") + mkFile("path/to/foo.go") + mkFile("docs/big.md")

	b := makeBigBead()
	refs := []GoverningRef{{ID: "FEAT-1", Path: "docs/spec.md"}}
	res := BuildReviewPromptBounded(b, 1, "rev4", diff, t.TempDir(), refs, BuildReviewPromptOptions{Caps: caps})

	idxAC := strings.Index(res.Prompt, "marker for path/to/foo.go")
	idxRef := strings.Index(res.Prompt, "marker for docs/big.md")
	idxOther := strings.Index(res.Prompt, "marker for z/unrelated.go")

	require.GreaterOrEqual(t, idxAC, 0)
	require.GreaterOrEqual(t, idxRef, 0)
	require.GreaterOrEqual(t, idxOther, 0)
	assert.Less(t, idxAC, idxRef, "AC-referenced file must appear before governing-ref-matched file")
	assert.Less(t, idxRef, idxOther, "governing-ref-matched file must appear before unreferenced file")
}

func TestBuildReviewPrompt_LargeFileDegradation(t *testing.T) {
	caps := testCaps()
	caps.MaxInlinedFileBytes = 1024
	caps.MaxDiffBytes = 1 << 20
	caps.MaxPromptBytes = 1 << 20

	bigBody := strings.Repeat("+content line in large file\n", 500) // ~14KB
	diff := "diff --git a/huge.go b/huge.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		"@@ -50,2 +51,4 @@\n" +
		bigBody

	b := makeBigBead()
	res := BuildReviewPromptBounded(b, 1, "rev5", diff, t.TempDir(), nil, BuildReviewPromptOptions{Caps: caps})

	assert.Contains(t, res.Prompt, "diff --git a/huge.go b/huge.go", "stat/header line must remain")
	assert.Contains(t, res.Prompt, "@@ -1,2 +1,3 @@", "hunk headers must remain")
	assert.Contains(t, res.Prompt, "@@ -50,2 +51,4 @@", "hunk headers must remain")
	assert.NotContains(t, res.Prompt, "+content line in large file",
		"degraded large-file rendering must omit full body content")
	assert.Contains(t, res.Prompt, "file body omitted", "degradation marker should annotate the omission")
}

func TestBuildReviewPrompt_PreDispatchShortCircuit(t *testing.T) {
	caps := evidence.Caps{
		MaxPromptBytes:       512,
		MaxInlinedFileBytes:  1 << 20,
		MaxDiffBytes:         1 << 20,
		MaxGoverningDocBytes: 1 << 20,
	}
	// Floor alone exceeds MaxPromptBytes (description is huge).
	b := &bead.Bead{
		ID:          "ddx-of",
		Title:       "overflow",
		Description: strings.Repeat("D", 4000),
		Acceptance:  "AC#1: anything",
	}
	res := BuildReviewPromptBounded(b, 1, "revOF", "", t.TempDir(), nil, BuildReviewPromptOptions{Caps: caps})
	assert.True(t, res.Overflow,
		"residual overflow after trimming must surface so callers can short-circuit")
}

// TestReviewContextOverflow asserts the executor honors the pre-dispatch
// short-circuit: when bounded assembly cannot fit the prompt, no provider
// run happens and the surfaced error carries the literal substring
// 'context_overflow'.
func TestReviewContextOverflow(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := exec.Command("git", "init", projectRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))
	require.NoError(t, store.Init())
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# x\n"), 0o644))
	require.NoError(t, store.Create(&bead.Bead{
		ID:          "ddx-overflow",
		Title:       "context overflow",
		Description: strings.Repeat("D", 4000),
		Acceptance:  "AC#1",
	}))
	out, err = exec.Command("git", "-C", projectRoot, "add", "README.md", ".ddx/beads.jsonl").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", projectRoot, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init").CombinedOutput()
	require.NoError(t, err, string(out))
	headRaw, err := exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headRaw))

	counter := &countingRunner{result: &Result{Output: "### Verdict: APPROVE\n"}}
	reviewer := &DefaultBeadReviewer{
		ProjectRoot: projectRoot,
		BeadStore:   store,
		Runner:      counter,
		Caps: evidence.Caps{
			MaxPromptBytes:       512,
			MaxInlinedFileBytes:  1 << 20,
			MaxDiffBytes:         1 << 20,
			MaxGoverningDocBytes: 1 << 20,
		},
	}

	res, reviewErr := reviewer.ReviewBead(context.Background(), "ddx-overflow", head, "claude", "claude-opus")
	require.Error(t, reviewErr)
	require.NotNil(t, res)
	assert.Contains(t, reviewErr.Error(), "context_overflow",
		"short-circuit error body must carry the literal context_overflow class")
	assert.Equal(t, 0, counter.calls,
		"pre-dispatch short-circuit must skip the provider call entirely")

	// Drive the worker loop with this reviewer so we can assert the
	// review-error event lands on the bead and the bead is not closed.
	store2 := store
	worker := &ExecuteBeadWorker{
		Store: store2,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				SessionID: "sess-overflow",
				ResultRev: head,
			}, nil
		}),
		Reviewer: reviewer,
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	events, err := store2.Events("ddx-overflow")
	require.NoError(t, err)
	foundOverflow := false
	for _, ev := range events {
		if ev.Kind == "review-error" && strings.Contains(ev.Body, "context_overflow") {
			foundOverflow = true
		}
	}
	assert.True(t, foundOverflow,
		"executor must emit a review-error event whose body contains 'context_overflow'")
	assert.Equal(t, 0, counter.calls,
		"loop-driven short-circuit must still avoid the provider call")
}

// TestReviewFailureNotCloseBead is the named anchor for the
// non-close invariant: any reviewer terminal failure (including the
// pre-dispatch context_overflow short-circuit) must not close the
// bead. It defers to the existing failure-modes coverage in
// execute_bead_review_failure_modes_test.go.
func TestReviewFailureNotCloseBead(t *testing.T) {
	TestExecuteBeadWorker_ReviewerFailureModesKeepBeadOpen(t)
}

// TestEvidencePrimitiveUsage is the AST gate that prevents future drift
// in execute_bead_review.go: any new direct os.ReadFile or os/exec
// "git show" call must be marked //evidence:allow-unbounded.
func TestEvidencePrimitiveUsage(t *testing.T) {
	src := "execute_bead_review.go"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, nil, parser.ParseComments)
	require.NoError(t, err)

	// Import check.
	importsEvidence := false
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == `"github.com/DocumentDrivenDX/ddx/internal/evidence"` {
			importsEvidence = true
		}
	}
	assert.True(t, importsEvidence,
		"execute_bead_review.go must import cli/internal/evidence (FEAT-022)")

	// Walk the AST. For every os.ReadFile or osexec.Command("git","show",...)
	// call, climb to the enclosing statement and look for an adjacent
	// //evidence:allow-unbounded directive in the file's comment map.
	cmap := ast.NewCommentMap(fset, file, file.Comments)

	hasAllow := func(node ast.Node) bool {
		// Walk parents via ast.Inspect — simplest is to scan all comment
		// groups whose end position is just before the node's start, on a
		// preceding line.
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
		// Also check the comment map: if any associated comment carries the
		// directive on this node, treat as allowed.
		for n, groups := range cmap {
			if n.Pos() <= node.Pos() && node.End() <= n.End() {
				for _, cg := range groups {
					for _, c := range cg.List {
						if strings.Contains(c.Text, "evidence:allow-unbounded") {
							return true
						}
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
		// os.ReadFile
		if ident.Name == "os" && sel.Sel.Name == "ReadFile" {
			if !hasAllow(call) {
				p := fset.Position(call.Pos())
				violations = append(violations, "os.ReadFile at line "+evidItoa(p.Line))
			}
			return true
		}
		// osexec.Command("git","show",...)
		if (ident.Name == "osexec" || ident.Name == "exec") && sel.Sel.Name == "Command" {
			if len(call.Args) >= 2 {
				if isStringLit(call.Args[0], "git") && isStringLit(call.Args[1], "show") {
					if !hasAllow(call) {
						p := fset.Position(call.Pos())
						violations = append(violations, "exec.Command(\"git\",\"show\",...) at line "+evidItoa(p.Line))
					}
				}
			}
		}
		return true
	})

	assert.Empty(t, violations,
		"execute_bead_review.go must route file reads and git show through cli/internal/evidence (or annotate with //evidence:allow-unbounded)")
}

// countingRunner is an AgentRunner stub that increments calls on every
// Run invocation, used to assert the pre-dispatch short-circuit avoids
// the provider call entirely.
type countingRunner struct {
	calls  int
	result *Result
	err    error
}

func (c *countingRunner) Run(_ RunArgs) (*Result, error) {
	c.calls++
	return c.result, c.err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func filepathSafe(i int) string {
	return "pkg/file_" + evidItoa(i) + ".go"
}

func evidItoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	out := string(buf[pos:])
	if neg {
		return "-" + out
	}
	return out
}

func isStringLit(e ast.Expr, want string) bool {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return false
	}
	return bl.Value == "\""+want+"\""
}
