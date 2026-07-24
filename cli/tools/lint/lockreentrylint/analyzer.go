// Package lockreentrylint flags non-reentrant collection-lock re-entry.
//
// Store.WriteAll acquires the bead collection directory lock. Callers
// already inside Store.WithLock must use Store.WriteAllLocked instead.
// Nested WriteAll deadlocks until LockWait expires (ddx-2a319f04 /
// ddx-79148c01 regression that broke upstream CI with cascading
// "bead: lock timeout" failures across exec/metric/server packages).
//
// The analyzer reports CallExpr nodes whose selector is WriteAll when the
// call is lexically nested inside a FuncLit argument to WithLock, and the
// WriteAll receiver type is *bead.Store. Interface-typed receivers are not
// flagged; the exec package's collectionBackend.WriteAllLocked contract
// covers that path at the type level.
package lockreentrylint

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the collection-lock re-entry structural analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "lockreentrylint",
	Doc:      "flags Store.WriteAll calls nested inside WithLock callbacks (non-reentrant collection lock; use WriteAllLocked)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	if pass.Pkg != nil {
		path := pass.Pkg.Path()
		if strings.Contains(path, "/tools/lint/lockreentrylint") {
			return nil, nil
		}
	}

	// Map each FuncLit node that is an argument to *.WithLock(...) so we can
	// test membership of nested WriteAll calls.
	withLockBodies := map[*ast.FuncLit]bool{}

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "WithLock" {
			return
		}
		for _, arg := range call.Args {
			if fl, ok := arg.(*ast.FuncLit); ok {
				withLockBodies[fl] = true
			}
		}
	})

	if len(withLockBodies) == 0 {
		return nil, nil
	}

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "WriteAll" {
			return
		}
		// Runtime negative tests intentionally nest WriteAll under WithLock to
		// pin the deadlock; production non-test code is the enforcement surface.
		if file := pass.Fset.File(call.Pos()); file != nil && strings.HasSuffix(file.Name(), "_test.go") {
			return
		}
		if !insideWithLockBody(call, withLockBodies) {
			return
		}
		if !isBeadStoreReceiver(pass, sel.X) {
			return
		}
		pass.Reportf(sel.Sel.Pos(), "Store.WriteAll nested inside WithLock re-enters the non-reentrant collection lock; use WriteAllLocked (ddx-2a319f04)")
	})

	return nil, nil
}

func insideWithLockBody(call *ast.CallExpr, bodies map[*ast.FuncLit]bool) bool {
	pos := call.Pos()
	for fl := range bodies {
		if fl.Pos() <= pos && pos < fl.End() {
			return true
		}
	}
	return false
}

func isBeadStoreReceiver(pass *analysis.Pass, recv ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[recv]
	if !ok {
		return false
	}
	t := tv.Type
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
	if obj == nil || obj.Name() != "Store" {
		return false
	}
	pkg := obj.Pkg()
	if pkg == nil {
		return false
	}
	path := pkg.Path()
	return path == "github.com/DocumentDrivenDX/ddx/internal/bead" ||
		strings.HasSuffix(path, "/internal/bead") ||
		path == "bead" // analysistest stubs
}
