package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var lifecyclePatternTableLintForbiddenLiterals = []string{
	"claude",
	"codex",
	"gemini",
	"openai",
	"anthropic",
	"sonnet",
	"haiku",
	"opus",
}

// TestLifecyclePatternTableLintNoProviderOutcomeControl prevents DDx lifecycle
// outcome-control tables from keying on provider CLI names, harness equality,
// or concrete model names. Generic error classification remains allowed, but
// the provider/routing tables that decide fallback or outage policy must stay
// free of those identity-based branches.
func TestLifecyclePatternTableLintNoProviderOutcomeControl(t *testing.T) {
	cliRoot := lifecyclePatternTableLintCLIRoot(t)
	var violations []string
	for _, root := range []string{filepath.Join(cliRoot, "internal"), filepath.Join(cliRoot, "cmd")} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			violations = append(violations, lintLifecyclePatternTableFile(t, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("provider outcome-control tables remain:\n%s", strings.Join(violations, "\n"))
	}
}

func lifecyclePatternTableLintCLIRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func lintLifecyclePatternTableFile(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !lifecyclePatternTableLintShouldInspectFunc(fn.Name.Name) || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				if lifecyclePatternTableLintCallViolates(n) {
					pos := fset.Position(n.Pos())
					violations = append(violations, filepath.ToSlash(path)+":"+pos.String()+": forbidden provider outcome-control literal or comparison")
				}
			case *ast.BinaryExpr:
				if lifecyclePatternTableLintBinaryViolates(n) {
					pos := fset.Position(n.Pos())
					violations = append(violations, filepath.ToSlash(path)+":"+pos.String()+": forbidden provider/harness/model comparison")
				}
			}
			return true
		})
	}
	return violations
}

func lifecyclePatternTableLintShouldInspectFunc(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "classify") ||
		strings.Contains(lower, "decide") ||
		strings.Contains(lower, "fallback") ||
		strings.Contains(lower, "routing") ||
		strings.Contains(lower, "readiness") ||
		strings.Contains(lower, "failure") ||
		strings.Contains(lower, "outcome") ||
		strings.Contains(lower, "report")
}

func lifecyclePatternTableLintCallViolates(call *ast.CallExpr) bool {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	default:
		return false
	}
	if name != "containsAny" && name != "Contains" && name != "EqualFold" {
		return false
	}
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value := strings.Trim(lit.Value, `"`)
		for _, forbidden := range lifecyclePatternTableLintForbiddenLiterals {
			if strings.Contains(strings.ToLower(value), forbidden) {
				return true
			}
		}
	}
	return false
}

func lifecyclePatternTableLintBinaryViolates(expr *ast.BinaryExpr) bool {
	switch expr.Op {
	case token.EQL, token.NEQ:
	default:
		return false
	}
	if !lifecyclePatternTableLintMentionsIdentity(expr.X, expr.Y) {
		return false
	}
	return lifecyclePatternTableLintHasNonEmptyStringLiteral(expr.X) || lifecyclePatternTableLintHasNonEmptyStringLiteral(expr.Y)
}

func lifecyclePatternTableLintMentionsIdentity(exprs ...ast.Expr) bool {
	for _, expr := range exprs {
		if expr == nil {
			continue
		}
		found := false
		ast.Inspect(expr, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.Ident:
				lower := strings.ToLower(n.Name)
				if strings.Contains(lower, "provider") || strings.Contains(lower, "harness") || strings.Contains(lower, "model") {
					found = true
					return false
				}
			case *ast.SelectorExpr:
				lower := strings.ToLower(n.Sel.Name)
				if strings.Contains(lower, "provider") || strings.Contains(lower, "harness") || strings.Contains(lower, "model") {
					found = true
					return false
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func lifecyclePatternTableLintHasNonEmptyStringLiteral(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if strings.Trim(lit.Value, `"`) != "" {
			found = true
			return false
		}
		return true
	})
	return found
}
