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
	"no harness configured",
	"harness not installed",
	"unknown harness",
}

// TestLifecyclePatternTableLintNoProviderOutcomeControl prevents DDx lifecycle
// outcome-control tables from keying on provider CLI names, harness equality,
// or concrete model names. Generic error classification remains allowed, but
// the provider/routing tables that decide fallback or outage policy must stay
// free of those identity-based branches.
func TestLifecyclePatternTableLintNoProviderOutcomeControl(t *testing.T) {
	cliRoot := lifecyclePatternTableLintCLIRoot(t)
	targets := []string{
		"internal/agent/execute_bead_status.go",
		"internal/agent/readiness_classification.go",
		"internal/agent/execute_bead_loop.go",
	}

	var violations []string
	for _, rel := range targets {
		path := filepath.Join(cliRoot, filepath.FromSlash(rel))
		violations = append(violations, lintLifecyclePatternTableFile(t, path)...)
	}

	cmdDir := filepath.Join(cliRoot, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		t.Fatalf("read cmd dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		violations = append(violations, lintLifecyclePatternTableFile(t, filepath.Join(cmdDir, entry.Name()))...)
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
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		name := ""
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			name = fun.Name
		case *ast.SelectorExpr:
			name = fun.Sel.Name
		default:
			return true
		}
		if name != "containsAny" {
			return true
		}

		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value := strings.ToLower(strings.Trim(lit.Value, `"`))
			for _, forbidden := range lifecyclePatternTableLintForbiddenLiterals {
				if strings.Contains(value, forbidden) {
					pos := fset.Position(lit.Pos())
					violations = append(violations, filepath.ToSlash(path)+":"+pos.String()+": forbidden provider outcome-control literal "+lit.Value)
					break
				}
			}
		}

		return true
	})
	return violations
}
