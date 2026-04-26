// Package runtimelint implements the SD-024 Stage 4 structural lint
// rule that mechanically forbids the durable-knob fields on *Runtime
// structs and the reintroduction of the legacy *Options types.
//
// Three patterns flagged:
//
//  1. Forbidden field name on a *Runtime struct declared in
//     cli/internal/agent/. Forbidden names are a closed list (see
//     forbiddenRuntimeFields below); allowed runtime-intent fields
//     such as NoReview, PollInterval, SessionLogDirOverride are not
//     on the list. Per TD-024 §Lint rule, only struct types whose
//     name ends in "Runtime" are scanned, to avoid false-positives on
//     result/status/event types in cli/internal/agent/types.go.
//
//  2. Composite literal Xxx{...} where Xxx is one of the legacy
//     options types (ExecuteBeadLoopOptions, RunOptions,
//     ExecuteBeadOptions, CompareOptions, QuorumOptions) anywhere in
//     the repository.
//
//  3. Function declaration in cli/internal/agent/ whose signature
//     includes a parameter typed as one of the legacy options types
//     named in pattern 2.
//
// Closed lists are maintained as Go constants — adding a new durable
// knob to *Config requires adding its name to forbiddenRuntimeFields.
package runtimelint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the SD-024 Stage 4 structural analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "runtimelint",
	Doc:      "flags durable-knob fields on *Runtime structs and reintroductions of legacy *Options types per SD-024 §Stage 4 / TD-024 §Lint rule",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// forbiddenRuntimeFields is the closed list of durable-knob field
// names that must not appear on *Runtime structs declared in
// cli/internal/agent/. Maintained as a constant per TD-024 §Lint rule.
var forbiddenRuntimeFields = map[string]bool{
	"Harness":                 true,
	"Model":                   true,
	"Provider":                true,
	"ModelRef":                true,
	"Profile":                 true,
	"Effort":                  true,
	"Permissions":             true,
	"Timeout":                 true,
	"WallClock":               true,
	"ContextBudget":           true,
	"MinTier":                 true,
	"MaxTier":                 true,
	"Assignee":                true,
	"ReviewMaxRetries":        true,
	"NoProgressCooldown":      true,
	"MaxNoChangesBeforeClose": true,
	"HeartbeatInterval":       true,
	"SessionLogDir":           true, // durable variant; SessionLogDirOverride is allowed
	"MirrorCfg":               true,
	"Models":                  true,
	"ReasoningLevels":         true,
	"Endpoints":               true,
	"ProfileLadders":          true,
	"ModelOverrides":          true,
	"PerHarness":              true,
}

// legacyOptionsTypes is the closed list of legacy options type names
// that must not be reintroduced (composite literal) or accepted as a
// function parameter in cli/internal/agent/.
var legacyOptionsTypes = map[string]bool{
	"ExecuteBeadLoopOptions": true,
	"RunOptions":             true,
	"ExecuteBeadOptions":     true,
	"CompareOptions":         true,
	"QuorumOptions":          true,
}

// agentPkgSuffixes lists the package import-path suffixes considered
// the cli/internal/agent/ package for scoping rules 1 and 3.
var agentPkgSuffixes = []string{
	"/internal/agent",
	"internal/agent",
	"agent", // permissive: testdata stub uses bare "agent"
}

func pkgPathHasSuffix(p string, suffixes []string) bool {
	for _, s := range suffixes {
		if p == s || strings.HasSuffix(p, s) {
			return true
		}
	}
	return false
}

// inAgentPkg reports whether the analyzer pass is operating on a
// package whose path is cli/internal/agent/ (or a testdata stub).
func inAgentPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	return pkgPathHasSuffix(pass.Pkg.Path(), agentPkgSuffixes)
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Skip the analyzer's own package.
	if pass.Pkg != nil {
		path := pass.Pkg.Path()
		if strings.HasSuffix(path, "/tools/lint/runtimelint") ||
			strings.Contains(path, "/tools/lint/runtimelint/") {
			return nil, nil
		}
	}

	report := func(pos token.Pos, msg string) {
		pass.Report(analysis.Diagnostic{Pos: pos, Message: msg})
	}

	// Pattern 1: forbidden field on *Runtime struct in agent pkg.
	if inAgentPkg(pass) {
		insp.Preorder([]ast.Node{(*ast.TypeSpec)(nil)}, func(n ast.Node) {
			ts := n.(*ast.TypeSpec)
			checkRuntimeStruct(ts, report)
		})
	}

	// Pattern 2: composite literal of legacy options type (anywhere).
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil)}, func(n ast.Node) {
		cl := n.(*ast.CompositeLit)
		checkLegacyComposite(pass, cl, report)
	})

	// Pattern 3: function param typed as legacy options (agent pkg).
	// Walk only FuncDecl — function-typed values inside the package are
	// not the surface we care about; the rule targets declared
	// signatures.
	if inAgentPkg(pass) {
		insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
			fd := n.(*ast.FuncDecl)
			if fd.Type != nil {
				checkLegacyParam(pass, fd.Type, report)
			}
		})
	}

	return nil, nil
}

// --- Pattern 1: forbidden field on *Runtime struct ------------------------

func checkRuntimeStruct(ts *ast.TypeSpec, report func(token.Pos, string)) {
	if ts.Name == nil || !strings.HasSuffix(ts.Name.Name, "Runtime") {
		return
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return
	}
	for _, f := range st.Fields.List {
		for _, name := range f.Names {
			if forbiddenRuntimeFields[name.Name] {
				report(name.Pos(),
					"runtimelint: forbidden durable-knob field "+name.Name+" on *Runtime struct "+ts.Name.Name+" — durable knobs belong on config.ResolvedConfig, not on the runtime struct (SD-024 §Runtime structs / TD-024 §Lint rule)")
			}
		}
	}
}

// --- Pattern 2: composite literal of legacy options type ------------------

func checkLegacyComposite(pass *analysis.Pass, cl *ast.CompositeLit, report func(token.Pos, string)) {
	t := pass.TypesInfo.TypeOf(cl)
	if t == nil {
		return
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return
	}
	obj := named.Obj()
	if obj == nil {
		return
	}
	if !legacyOptionsTypes[obj.Name()] {
		return
	}
	// Scope to types declared in cli/internal/agent/ — avoids false-
	// positives if some unrelated package happens to use the same name.
	if obj.Pkg() == nil || !pkgPathHasSuffix(obj.Pkg().Path(), agentPkgSuffixes) {
		return
	}
	report(cl.Pos(),
		"runtimelint: composite literal of legacy options type "+obj.Name()+" — these types are retired; use the *Runtime + ResolvedConfig pair instead (SD-024 §Stage 4 / TD-024 §Lint rule)")
}

// --- Pattern 3: function param typed as legacy options --------------------

func checkLegacyParam(pass *analysis.Pass, ft *ast.FuncType, report func(token.Pos, string)) {
	if ft.Params == nil {
		return
	}
	for _, field := range ft.Params.List {
		if isLegacyOptionsType(pass, field.Type) {
			report(field.Pos(),
				"runtimelint: function parameter typed as legacy options — these types are retired; use a *Runtime parameter plus a config.ResolvedConfig (SD-024 §Stage 4 / TD-024 §Lint rule)")
		}
	}
}

func isLegacyOptionsType(pass *analysis.Pass, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil {
		return false
	}
	if !legacyOptionsTypes[obj.Name()] {
		return false
	}
	if obj.Pkg() == nil || !pkgPathHasSuffix(obj.Pkg().Path(), agentPkgSuffixes) {
		return false
	}
	return true
}
