// Package routinglint implements the FEAT-006 routing-cleanup lint
// rule that mechanically forbids reintroduction of DDx-side
// "compensating routing" logic that was deleted with prejudice in
// ddx-3bd7396a.
//
// The parent epic ddx-fdd3ea36 §AC#7 says:
//
//	"Search DDx for code that re-implements upstream routing
//	concerns: allow-list checks, exact-pin filtering,
//	profile-to-preference mapping, provider cost scoring. Must be
//	absent on the default path. ... Dead-code detector run in CI
//	catches orphans."
//
// Parent-epic AC#7 originally expected the helpers to remain present,
// reachable only via --escalate. The follow-up bead ddx-3bd7396a
// instead deleted them entirely, so this analyzer enforces the
// stricter invariant: the listed compensating-routing identifiers and
// flag literals must not reappear anywhere in cli/ Go source. If they
// do, the cleanup has regressed and CI fails.
//
// Forbidden identifiers (exact name match):
//
//	ResolveProfileLadder, ResolveTierModelRef, AdaptiveMinTier,
//	workersByHarness, ResolveProfileLadderCallCount.
//
// Forbidden string literals (anywhere in cli/ Go source):
//
//	"--escalate", "--override-model",
//	"profile_ladders", "model_overrides",
//	"agent.routing.profile_ladders",
//	"agent.routing.model_overrides".
//
// String-literal occurrences inside this analyzer's own package and
// inside testdata fixtures are exempt.
//
// A specific occurrence may be intentionally exempted (e.g. the
// migration code that rejects a config containing
// agent.routing.profile_ladders has to name the literal key) by
// adding a
//
//	// routinglint:legacy-rejection reason="..."
//
// comment on the same line as the offending node, or on a comment
// line directly above it. The reason= clause must be present and
// non-empty.
package routinglint

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const exemptionAnnotation = "routinglint:legacy-rejection"

var exemptionRE = regexp.MustCompile(`routinglint:legacy-rejection\s+reason="[^"]+"`)

// Analyzer is the FEAT-006 routing-cleanup analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "routinglint",
	Doc:      "flags reintroduction of compensating DDx-side routing helpers and flags retired by FEAT-006 / ddx-3bd7396a",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// forbiddenIdents is the closed list of compensating-routing
// identifiers retired by ddx-3bd7396a. Reintroduction is a CI failure.
var forbiddenIdents = map[string]string{
	"ResolveProfileLadder":          "tier-ladder resolver retired by ddx-3bd7396a — DDx must not re-implement profile-to-preference mapping",
	"ResolveTierModelRef":           "tier-model resolver retired by ddx-3bd7396a — DDx must not re-implement exact-pin filtering",
	"ResolveProfileLadderCallCount": "test seam for the retired tier-ladder resolver — also retired by ddx-3bd7396a",
	"AdaptiveMinTier":               "adaptive-min-tier promotion logic retired by ddx-3bd7396a",
	"workersByHarness":              "escalation worker grouping helper retired by ddx-3bd7396a",
	"queueByHarness":                "using harness as a queue-selection criterion violates the passthrough envelope contract (ddx-20047dd5)",
	"retryByPassthrough":            "using passthrough values in retry policy violates the passthrough envelope contract (ddx-20047dd5)",
	"endpointByPassthrough":         "using passthrough values in endpoint selection violates the passthrough envelope contract (ddx-20047dd5)",
	"catalogThresholdForHarness":    "using harness in catalog threshold evaluation violates the passthrough envelope contract (ddx-20047dd5)",
}

// forbiddenLiterals is the closed list of CLI flag and config-key
// strings retired by ddx-3bd7396a. They must not appear as Go string
// literals anywhere in cli/.
var forbiddenLiterals = map[string]string{
	"--escalate":                    "--escalate flag retired by ddx-3bd7396a — escalation is not a DDx-side concern",
	"--override-model":              "--override-model flag retired by ddx-3bd7396a — model overrides go through --model on the default path",
	"profile_ladders":               "agent.routing.profile_ladders config key retired by ddx-3bd7396a",
	"model_overrides":               "agent.routing.model_overrides config key retired by ddx-3bd7396a",
	"agent.routing.profile_ladders": "agent.routing.profile_ladders config key retired by ddx-3bd7396a",
	"agent.routing.model_overrides": "agent.routing.model_overrides config key retired by ddx-3bd7396a",
}

// selfPkgFragments lists path fragments that mark this analyzer's own
// package or its testdata fixtures. Diagnostics from those packages
// are suppressed because the closed-list constants above legitimately
// embed every forbidden token.
var selfPkgFragments = []string{
	"/tools/lint/routinglint",
	"tools/lint/routinglint",
}

func inSelfPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	p := pass.Pkg.Path()
	for _, f := range selfPkgFragments {
		if strings.Contains(p, f) {
			return true
		}
	}
	return false
}

func run(pass *analysis.Pass) (interface{}, error) {
	if inSelfPkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Build a per-file index of lines that carry an exemption
	// annotation, either as an inline comment on the same line or as
	// a stand-alone comment line directly above. The annotation
	// covers the line it sits on plus the line immediately following.
	exempted := make(map[token.Position]bool)
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if !strings.Contains(c.Text, exemptionAnnotation) {
					continue
				}
				if !exemptionRE.MatchString(c.Text) {
					pass.Report(analysis.Diagnostic{
						Pos:     c.Pos(),
						Message: `routinglint: ` + exemptionAnnotation + ` annotation is missing a non-empty reason="..." clause`,
					})
					continue
				}
				cp := pass.Fset.Position(c.Pos())
				exempted[token.Position{Filename: cp.Filename, Line: cp.Line}] = true
				exempted[token.Position{Filename: cp.Filename, Line: cp.Line + 1}] = true
			}
		}
	}

	isExempt := func(pos token.Pos) bool {
		p := pass.Fset.Position(pos)
		return exempted[token.Position{Filename: p.Filename, Line: p.Line}]
	}

	report := func(pos token.Pos, msg string) {
		if isExempt(pos) {
			return
		}
		pass.Report(analysis.Diagnostic{Pos: pos, Message: msg})
	}

	// Forbidden identifiers — flag at every occurrence, both
	// declarations and references. Matching is exact on Ident.Name so
	// substrings inside larger identifiers (e.g. the test name
	// TestExecuteLoopLocalRejectsProfileLadders) do not trip the lint.
	insp.Preorder([]ast.Node{(*ast.Ident)(nil)}, func(n ast.Node) {
		id := n.(*ast.Ident)
		if reason, ok := forbiddenIdents[id.Name]; ok {
			report(id.Pos(), "routinglint: forbidden identifier "+id.Name+" — "+reason)
		}
	})

	// Forbidden string literals — flag the literal node itself.
	insp.Preorder([]ast.Node{(*ast.BasicLit)(nil)}, func(n ast.Node) {
		lit := n.(*ast.BasicLit)
		if lit.Kind != token.STRING {
			return
		}
		// Trim surrounding quote/backtick so we compare raw value.
		s := lit.Value
		if len(s) >= 2 && (s[0] == '"' || s[0] == '`') {
			s = s[1 : len(s)-1]
		}
		if reason, ok := forbiddenLiterals[s]; ok {
			report(lit.Pos(), "routinglint: forbidden string literal "+lit.Value+" — "+reason)
		}
	})

	return nil, nil
}
