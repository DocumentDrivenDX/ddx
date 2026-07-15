package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestLifecycleRoutinglintNoCatalogOrPreRouteCalls prevents DDx lifecycle
// stages from acquiring enough concrete route information to make a harness,
// provider, model, or policy choice. These stages may send abstract power
// bounds and explicit operator pins to Execute; Fizeau owns route resolution.
func TestLifecycleRoutinglintNoCatalogOrPreRouteCalls(t *testing.T) {
	targets := []string{
		"preclaim_intake_hook.go",
		"recovery_decompose.go",
		"recovery_reframe.go",
		"execute_bead.go",
		"execute_bead_loop.go",
		"dispatch.go",
	}
	forbidden := map[string]struct{}{
		"ResolveRoute":  {},
		"ListHarnesses": {},
		"ListProviders": {},
		"ListModels":    {},
		"ListPolicies":  {},
	}

	for _, path := range targets {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("routinglint: parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, blocked := forbidden[selector.Sel.Name]; blocked {
				t.Errorf("routinglint: %s calls .%s() at %s; lifecycle execution must delegate route selection to Fizeau",
					path, selector.Sel.Name, fset.Position(call.Pos()))
			}
			return true
		})
	}
}
