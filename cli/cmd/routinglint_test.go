package cmd

// routinglint_test.go: AST guard ensuring ResolveRoute is not called from
// execution-path functions in run.go, try.go, or work.go. Fails if someone
// re-introduces a ResolveRoute call inside one of the top-level execution
// entry points (CONTRACT-003 / ddx-da19756a).

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
var routinglintExecutionFunctions = []struct {
	file string
	fn   string
}{
	{file: "run.go", fn: "runRun"},
	{file: "try.go", fn: "runTry"},
	{file: "work.go", fn: "runWork"},
}

// TestRoutinglintNoResolveRouteInExecutionPaths verifies that execution-path
// functions do not call .ResolveRoute(). Status/debug surfaces are allowed to
// call ResolveRoute and are not scanned by this lint.
func TestRoutinglintNoResolveRouteInExecutionPaths(t *testing.T) {
	for _, target := range routinglintExecutionFunctions {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, target.file, nil, 0)
		if err != nil {
			t.Fatalf("routinglint: parse %s: %v", target.file, err)
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != target.fn {
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
					t.Errorf("routinglint: %s in %s calls .ResolveRoute() at %s — execution paths must pass operator constraints to Execute, not pre-resolve routes (CONTRACT-003 / ddx-da19756a)",
						target.fn, target.file, fset.Position(call.Pos()))
				}
				return true
			})
		}
	}
}

// TestRoutinglintNonStatusFilesDoNotCallResolveRoute verifies that non-test
// cmd/ source files other than status/introspection surfaces do not call
// .ResolveRoute().
func TestRoutinglintNonStatusFilesDoNotCallResolveRoute(t *testing.T) {
	// agent_route_status.go hosts the status/introspection surface.
	// All other non-test .go files in cmd/ must not call ResolveRoute.
	allowedFiles := map[string]bool{
		"agent_route_status.go": true,
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
