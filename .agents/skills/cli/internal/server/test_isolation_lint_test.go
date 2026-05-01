package server

// Lint test for ddx-15f7ee0b Fix A: every test file in this package (and
// adjacent directories that wire up ServerState) that constructs a server or
// loads a ServerState must point the state directory at an isolated location.
// Tests that skip this leak phantom project entries into the developer's real
// ~/.local/share/ddx/server-state.json.
//
// The check is intentionally conservative: a function that calls any of the
// "constructor" set below must also invoke at least one of the "isolation"
// set. Adding a new helper that internally isolates XDG_DATA_HOME is the
// preferred way to keep this passing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// constructorNames are function calls that build a server or directly load a
// ServerState. Any test that touches one of these must also pick up isolation.
// Entries without a "pkg." prefix match only package-local calls; entries like
// "server.New" match explicit cross-package qualified references.
var constructorNames = map[string]bool{
	"New":               true,
	"server.New":        true,
	"loadServerState":   true,
	"ListenAndServe":    true,
	"ListenAndServeTLS": true,
}

// isolationHelpers are helpers (defined in this package or adjacent test
// helpers) that set XDG_DATA_HOME before any server is created. A call to
// any of them satisfies the lint rule.
var isolationHelpers = map[string]bool{
	"setupTestDir":               true,
	"setupNodeTestDir":           true,
	"setupProcessMetricsTestDir": true,
	"setupDocTestDir":            true,
	"setupExecTestDir":           true,
	"setupGitTestDir":            true,
	"setupProjectWithBeads":      true,
	"setupBeadLookupFixture":     true,
	"BuildBeadFixture":           true,
}

// TestTestsIsolateStateDir walks the Go test files in the server package and
// perf sub-package, and asserts that every test (or benchmark) function that
// wires up a server also isolates XDG_DATA_HOME. See ddx-15f7ee0b Fix A.
func TestTestsIsolateStateDir(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "frontend", "node_modules", ".svelte-kit", "build":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			lintTestFile(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk server tests: %v", err)
	}
}

func lintTestFile(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	// Skip the lint test file itself — it is allowed to reference constructor
	// names inside string maps without wiring up a server.
	if strings.HasSuffix(path, "/test_isolation_lint_test.go") {
		return
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Only audit functions that could contain test code — Test*, Benchmark*,
		// or helpers used by such functions. For simplicity, audit all fns in
		// _test.go files. Helpers that unconditionally call setupTestDir etc.
		// will pass trivially.
		hasCtor := false
		hasIso := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call.Fun)
			if constructorNames[name] {
				hasCtor = true
			}
			if isolationHelpers[name] {
				hasIso = true
			}
			// Detect raw XDG_DATA_HOME sets: any call with a first arg that is
			// the string literal "XDG_DATA_HOME".
			if name == "Setenv" || name == "t.Setenv" {
				// argList: check any arg is "XDG_DATA_HOME"
				for _, a := range call.Args {
					if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if strings.Trim(lit.Value, "`\"") == "XDG_DATA_HOME" {
							hasIso = true
						}
					}
				}
			}
			return true
		})

		if hasCtor && !hasIso {
			t.Errorf("%s: function %s constructs a server without isolating XDG_DATA_HOME — call setupTestDir/etc. or t.Setenv(\"XDG_DATA_HOME\", ...) before building the server (ddx-15f7ee0b Fix A)",
				path, fn.Name.Name)
		}
	}
}

// callName returns one of three things:
//   - For `Foo(...)`:         "Foo"
//   - For `pkg.Foo(...)`:     "pkg.Foo"
//   - For `t.Setenv(...)`:    "Setenv"  (receiver is a local identifier, not
//     a package — we only add the prefix when the
//     receiver looks like a package name, i.e.
//     lowercase and short; this is a heuristic.)
//
// Having qualified names lets the constructor list distinguish server.New
// from handler.New without false positives.
func callName(fn ast.Expr) string {
	switch v := fn.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok {
			// Heuristic: treat short lowercase identifiers as package names.
			// Receivers like `t`, `srv`, `w` fall through — we still want
			// Setenv to match for `t.Setenv(...)`. The Setenv match is
			// special-cased by the lint body; the selector branch here just
			// carries the package-qualified form when it looks right.
			if looksLikePackageName(pkg.Name) {
				return pkg.Name + "." + v.Sel.Name
			}
		}
		return v.Sel.Name
	}
	return ""
}

// looksLikePackageName is a heuristic for callName: longer-than-2 lowercase
// identifiers are treated as package-like (so "server.New" is distinguished
// from "srv.Shutdown"). Receivers like t, s, srv, b, tb return false.
func looksLikePackageName(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	// Reject common local-variable names that happen to be lowercase words.
	switch s {
	case "srv", "tb", "ctx", "err", "req", "res":
		return false
	}
	return true
}

// TestTestIsolationLintCatchesBadCase is a meta-test: it feeds the linter a
// synthetic "bad" file and asserts the linter flags it. Covers the
// "disallowed-pattern" arm of ddx-15f7ee0b Fix A.
func TestTestIsolationLintCatchesBadCase(t *testing.T) {
	badSource := `package server

import "testing"

func TestBadlyIsolatedExample(t *testing.T) {
	srv := New(":0", "/tmp/ignored")
	_ = srv
}
`
	goodSource := `package server

import "testing"

func TestProperlyIsolatedExample(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv := New(":0", "/tmp/ignored")
	_ = srv
}
`
	if got := lintSourceString(badSource); got == nil {
		t.Error("expected bad source to be flagged by lint, got nil")
	}
	if got := lintSourceString(goodSource); got != nil {
		t.Errorf("expected good source to pass lint, got error: %v", got)
	}
}

// lintSourceString runs the same AST walk on an in-memory source string and
// returns a non-nil error if the lint would have failed. Used by the meta-test.
func lintSourceString(src string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		hasCtor := false
		hasIso := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call.Fun)
			if constructorNames[name] {
				hasCtor = true
			}
			if isolationHelpers[name] {
				hasIso = true
			}
			if name == "Setenv" {
				for _, a := range call.Args {
					if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if strings.Trim(lit.Value, "`\"") == "XDG_DATA_HOME" {
							hasIso = true
						}
					}
				}
			}
			return true
		})
		if hasCtor && !hasIso {
			return &lintErr{msg: fn.Name.Name + " constructs server without isolation"}
		}
	}
	return nil
}

type lintErr struct{ msg string }

func (e *lintErr) Error() string { return e.msg }
