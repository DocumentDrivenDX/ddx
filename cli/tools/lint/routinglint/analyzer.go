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
//	ResolveProfileLadder, AdaptiveMinPowerHint,
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
	"go/types"
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
	"BuiltinCatalog":                "DDx-side routing catalog retired by ddx-cd641910",
	"DefaultModelCatalogPath":       "DDx-side model catalog path retired by ddx-cd641910",
	"LoadModelCatalogYAML":          "DDx-side model catalog loader retired by ddx-cd641910",
	"ApplyModelCatalogYAML":         "DDx-side model catalog overlay retired by ddx-cd641910",
	"DefaultModelCatalogYAML":       "DDx-side model catalog seed retired by ddx-cd641910",
	"CheckDeprecatedPin":            "DDx-side catalog pin check retired by ddx-cd641910",
	"ResolveProfileLadder":          "powerClass-ladder resolver retired by ddx-3bd7396a — DDx must not re-implement profile-to-preference mapping",
	"ResolveProfileLadderCallCount": "test seam for the retired powerClass-ladder resolver — also retired by ddx-3bd7396a",
	"AdaptiveMinPowerHint":          "adaptive-min-power-class promotion logic retired by ddx-3bd7396a",
	"workersByHarness":              "escalation worker grouping helper retired by ddx-3bd7396a",
	"queueByHarness":                "using harness as a queue-selection criterion violates the passthrough envelope contract (ddx-20047dd5)",
	"retryByPassthrough":            "using passthrough values in retry policy violates the passthrough envelope contract (ddx-20047dd5)",
	"endpointByPassthrough":         "using passthrough values in endpoint selection violates the passthrough envelope contract (ddx-20047dd5)",
	"catalogThresholdForHarness":    "using harness in catalog threshold evaluation violates the passthrough envelope contract (ddx-20047dd5)",
	"ResolvePowerClass":             "DDx-side model powerClass resolution retired by ddx-ef81fa60",
	"resolveStrongSplitterMinPower": "DDx-side strong-power trick retired by ddx-ef81fa60",
	"isStrongPowerUnsatisfiedError": "DDx-side routing error substring retry retired by ddx-ef81fa60",
	"isSmartRouteUnavailableError":  "DDx-side routing error substring retry retired by ddx-ef81fa60",
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

// allowlistedLiterals are historical exact strings that remain
// permitted in docs, tests, or migration notes. They are not forbidden
// tokens, but keeping the allowlist explicit makes the boundary easy to
// audit when the analyzer grows new checks.
var allowlistedLiterals = map[string]string{
	"agentskills.io":                 "external skill-site reference retained for historical docs",
	"legacy agent":                   "historical workflow wording retained in migration docs",
	"Agent Service":                  "historical service name retained in docs",
	"cli/internal/agent":             "historical package path retained in docs and tests",
	"agent.routing.default_harness":  "removed config key retained in migration docs",
	"agent.routing.profile_priority": "deprecated config key retained in migration docs",
}

// allowedAgentSubpkgLeafs are the already-approved subpackages under
// cli/internal/agent/. New leaf packages beneath that tree are rejected.
var allowedAgentSubpkgLeafs = map[string]bool{
	"escalation":  true,
	"executeloop": true,
	"failclass":   true,
	"try":         true,
	"work":        true,
	"workerprobe": true,
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

	if inRetiredAgentSubpkg(pass) {
		if len(pass.Files) > 0 {
			pass.Report(analysis.Diagnostic{
				Pos:     pass.Files[0].Package,
				Message: "routinglint: DDx-owned cli/internal/agent subpackages are retired — keep the execution surface in the root cli/internal/agent package and do not add new agent subpackages",
			})
		}
		return nil, nil
	}

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

	// Structural guardrails for DDx command and request construction
	// surfaces. These catch the new public-command / model-routing
	// regressions without flagging historical prose in docs or tests.
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		lit := n.(*ast.CompositeLit)
		checkRetiredCommandSurface(pass, lit, report)
		checkExecuteRequestRouting(pass, lit, report)
	})

	return nil, nil
}

func inRetiredAgentSubpkg(pass *analysis.Pass) bool {
	if len(pass.Files) == 0 {
		return false
	}
	for _, f := range pass.Files {
		filename := pass.Fset.Position(f.Package).Filename
		leaf, ok := agentSubpkgLeaf(filename)
		if !ok {
			continue
		}
		if !allowedAgentSubpkgLeafs[leaf] {
			return true
		}
	}
	return false
}

func agentSubpkgLeaf(filename string) (string, bool) {
	const marker = "/cli/internal/agent/"
	idx := strings.Index(filename, marker)
	if idx < 0 {
		return "", false
	}
	rest := filename[idx+len(marker):]
	if rest == "" || !strings.Contains(rest, "/") {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 {
		return "", false
	}
	leaf := parts[0]
	leaf = strings.TrimSuffix(leaf, "_test")
	if leaf == "" {
		return "", false
	}
	return leaf, true
}

func isNamedType(named *types.Named, pkgPath, typeName string) bool {
	if named == nil {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == pkgPath && obj.Name() == typeName
}

func namedUnderlyingType(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

func stringLiteralValue(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s := lit.Value
	if len(s) >= 2 && (s[0] == '"' || s[0] == '`') {
		s = s[1 : len(s)-1]
	}
	return s, true
}

func checkRetiredCommandSurface(pass *analysis.Pass, lit *ast.CompositeLit, report func(token.Pos, string)) {
	t := pass.TypesInfo.TypeOf(lit)
	named := namedUnderlyingType(t)
	if !isNamedType(named, "github.com/spf13/cobra", "Command") {
		return
	}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Use" {
			continue
		}
		if val, ok := stringLiteralValue(kv.Value); ok && val == "agent" {
			report(kv.Pos(), `routinglint: forbidden public command surface Use:"agent" — DDx must not reintroduce a ddx agent command; keep the workflow surface on ddx run/try/work`)
		}
	}
}

func checkExecuteRequestRouting(pass *analysis.Pass, lit *ast.CompositeLit, report func(token.Pos, string)) {
	t := pass.TypesInfo.TypeOf(lit)
	named := namedUnderlyingType(t)
	if !isNamedType(named, "github.com/easel/fizeau", "ServiceExecuteRequest") {
		return
	}

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Harness", "Provider", "Model":
			if reason, bad := routedFromConfigOrNormalization(key.Name, kv.Value); bad {
				report(kv.Pos(), "routinglint: forbidden "+key.Name+" source for ServiceExecuteRequest — "+reason)
			}
		}
	}
}

func routedFromConfigOrNormalization(field string, e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "model" || x.Name == "provider" || x.Name == "harness" {
			return "", false
		}
		return "", false
	case *ast.SelectorExpr:
		if reason, bad := selectorRoutingReason(field, x); bad {
			return reason, true
		}
		return routedFromConfigOrNormalization(field, x.X)
	case *ast.CallExpr:
		if reason, bad := callRoutingReason(field, x); bad {
			return reason, true
		}
		if reason, bad := routedFromConfigOrNormalization(field, x.Fun); bad {
			return reason, true
		}
		for _, arg := range x.Args {
			if reason, bad := routedFromConfigOrNormalization(field, arg); bad {
				return reason, true
			}
		}
	case *ast.ParenExpr:
		return routedFromConfigOrNormalization(field, x.X)
	case *ast.UnaryExpr:
		return routedFromConfigOrNormalization(field, x.X)
	}
	return "", false
}

func selectorRoutingReason(field string, sel *ast.SelectorExpr) (string, bool) {
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	switch sel.Sel.Name {
	case "Model", "Provider", "Harness":
		switch x.Name {
		case "cfg", "rcfg", "resolved", "resolvedCfg", "resolvedConfig", "config":
			return "config-derived " + field + " access via " + x.Name + "." + sel.Sel.Name + "() bypasses explicit operator passthrough", true
		}
	}
	return "", false
}

func callRoutingReason(field string, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok {
		if reason, bad := selectorRoutingReason(field, sel); bad {
			return reason, true
		}
		if suspiciousFieldSource(sel.Sel.Name) {
			return "normalization or fuzzy matching helper " + sel.Sel.Name + " used to build " + field + " violates the explicit-passthrough boundary", true
		}
		return "", false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", false
	}
	if suspiciousFieldSource(ident.Name) {
		return "normalization or fuzzy matching helper " + ident.Name + " used to build " + field + " violates the explicit-passthrough boundary", true
	}
	return "", false
}

func suspiciousFieldSource(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "normalize") ||
		strings.Contains(lower, "fuzzy") ||
		strings.Contains(lower, "alias") ||
		strings.Contains(lower, "match") ||
		strings.Contains(lower, "resolvemodel") ||
		strings.Contains(lower, "resolveprovider") ||
		strings.Contains(lower, "resolveharness")
}
