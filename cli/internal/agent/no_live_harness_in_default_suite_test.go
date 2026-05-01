package agent

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLiveHarnessExecInDefaultSuite is the AC #5 static guard for the
// "Remove live harness execution tests from DDx" cleanup.
//
// Harness execution now lives in Fizeau. The default DDx test suite
// (`go test ./...`) must not exec real third-party harness binaries
// (opencode, claude, codex, gemini, pi). The single allowed file that
// references those binaries from a test is
// internal/agent/live_harness_integration_test.go, which is gated by
// `//go:build live_harness`.
//
// This test walks every *_test.go file in the cli/ tree and fails if any
// untagged file passes one of those binary names as a string literal to
// exec.Command, exec.CommandContext, exec.LookPath, or DefaultLookPath.
func TestNoLiveHarnessExecInDefaultSuite(t *testing.T) {
	cliRoot := findCLIRoot(t)
	harnessNames := map[string]struct{}{
		"opencode": {},
		"claude":   {},
		"codex":    {},
		"gemini":   {},
		"pi":       {},
	}
	gatedCallees := map[string]struct{}{
		"Command":         {},
		"CommandContext":  {},
		"LookPath":        {},
		"DefaultLookPath": {},
	}

	allowed := filepath.Join("internal", "agent", "live_harness_integration_test.go")

	var offenders []string
	err := filepath.Walk(cliRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == "build" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(cliRoot, path)
		if rel == allowed {
			return nil
		}

		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return nil
		}
		// Skip files already gated behind a build tag that disables them by
		// default (any //go:build line containing live_harness).
		if hasLiveHarnessBuildTag(f) {
			return nil
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := ""
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				callee = fn.Sel.Name
			case *ast.Ident:
				callee = fn.Name
			}
			if _, ok := gatedCallees[callee]; !ok {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val := strings.Trim(lit.Value, "`\"")
			if _, bad := harnessNames[val]; bad {
				pos := fset.Position(call.Pos())
				offenders = append(offenders,
					fmt.Sprintf("%s:%d %s(%q)", rel, pos.Line, callee, val))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("default test suite exec's real harness binaries; gate behind //go:build live_harness or move to Fizeau:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func hasLiveHarnessBuildTag(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:build") && strings.Contains(c.Text, "live_harness") {
				return true
			}
			if strings.HasPrefix(c.Text, "// +build") && strings.Contains(c.Text, "live_harness") {
				return true
			}
		}
	}
	return false
}

func findCLIRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate cli/ root from %s", wd)
	return ""
}
