package cmd

// routinglint_test.go: AST guard ensuring ResolveRoute is not called from
// execution-path functions in agent_cmd.go. Fails if someone re-introduces
// a ResolveRoute call inside newAgentRunCommand or newAgentExecuteLoopCommand
// (CONTRACT-003 / ddx-da19756a).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// routinglintExecutionFunctions are the CLI entry points that must NOT call
// ResolveRoute. These are the "run/try/work" execution paths described in
// the bead contract.
var routinglintExecutionFunctions = []string{
	"newAgentRunCommand",
	"newAgentExecuteLoopCommand",
}

// TestRoutinglintNoResolveRouteInExecutionPaths verifies that execution-path
// functions in agent_cmd.go do not call .ResolveRoute(). Status/debug
// surfaces (newAgentCatalogCommand, newAgentRouteStatusCommand, etc.) are
// allowed to call ResolveRoute and are not scanned by this lint.
func TestRoutinglintNoResolveRouteInExecutionPaths(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "agent_cmd.go", nil, 0)
	if err != nil {
		t.Fatalf("routinglint: parse agent_cmd.go: %v", err)
	}

	bannedSet := make(map[string]bool, len(routinglintExecutionFunctions))
	for _, name := range routinglintExecutionFunctions {
		bannedSet[name] = true
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !bannedSet[fn.Name.Name] {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "ResolveRoute" {
				t.Errorf("routinglint: %s calls .ResolveRoute() at %s — execution paths must pass operator constraints to Execute, not pre-resolve routes (CONTRACT-003 / ddx-da19756a)",
					fn.Name.Name, fset.Position(call.Pos()))
			}
			return true
		})
	}
}

// TestRoutinglintNonStatusFilesDoNotCallResolveRoute verifies that non-test
// cmd/ source files other than agent_cmd.go (which hosts the allowed catalog
// health-check) do not call .ResolveRoute().
func TestRoutinglintNonStatusFilesDoNotCallResolveRoute(t *testing.T) {
	// agent_cmd.go hosts the catalog-show health check, a status surface.
	// All other non-test .go files in cmd/ must not call ResolveRoute.
	allowedFiles := map[string]bool{
		"agent_cmd.go": true,
	}

	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("routinglint: glob cmd dir: %v", err)
	}
	for _, path := range paths {
		base := filepath.Base(path)
		if len(base) > 8 && base[len(base)-8:] == "_test.go" {
			continue
		}
		if allowedFiles[base] {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("routinglint: parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "ResolveRoute" {
				t.Errorf("routinglint: %s calls .ResolveRoute() at %s — only status/debug surfaces may call ResolveRoute; add to allowedFiles if intentional",
					base, fset.Position(call.Pos()))
			}
			return true
		})
	}
}
