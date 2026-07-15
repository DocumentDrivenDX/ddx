package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProductionAgentHasNoConcreteHarnessRunner guards the DDx/Fizeau
// ownership boundary. Test-only fixtures may model a concrete harness, but a
// production agent package file must not reintroduce DDx's retired runner or
// detailed harness registry.
func TestProductionAgentHasNoConcreteHarnessRunner(t *testing.T) {
	forbidden := map[string]struct{}{
		"runAgentViaService": {}, "recordRoutingOutcome": {}, "harnessConfig": {},
		"builtinHarnessConfigs": {}, "newHarnessRegistry": {}, "runScriptFn": {},
		"ExtractOutput": {}, "ExtractUsage": {}, "extractOutputCodex": {},
		"extractOutputClaude": {}, "extractOutputPiGemini": {}, "extractUsageClaude": {},
		"VirtualDictionaryDir": {}, "VirtualEntry": {}, "InlineResponse": {},
		"LookupInline": {}, "NormalizePrompt": {}, "PromptHash": {},
		"RecordEntry": {}, "LookupEntry": {},
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch declaration := node.(type) {
			case *ast.TypeSpec:
				switch declaration.Name.Name {
				case "Runner", "ExecResult", "Executor", "OSExecutor":
					t.Errorf("%s:%d reintroduces concrete runner/executor type %s", path, fset.Position(declaration.Pos()).Line, declaration.Name.Name)
				}
			case *ast.FuncDecl:
				if declaration.Name.Name == "NewRunner" {
					t.Errorf("%s:%d reintroduces concrete NewRunner constructor", path, fset.Position(declaration.Pos()).Line)
				}
			}
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, blocked := forbidden[ident.Name]; blocked {
				t.Errorf("%s:%d reintroduces concrete route symbol %s", path, fset.Position(ident.Pos()).Line, ident.Name)
			}
			return true
		})
	}

	runPath := filepath.Join("..", "..", "cmd", "run.go")
	fset := token.NewFileSet()
	runFile, err := parser.ParseFile(fset, runPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", runPath, err)
	}
	ast.Inspect(runFile, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "ExtractOutput" {
			t.Errorf("%s:%d reparses Fizeau FinalText with provider-specific ExtractOutput", runPath, fset.Position(selector.Pos()).Line)
		}
		return true
	})
}
