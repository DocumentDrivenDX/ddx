package bead

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	forbiddenBeadStorageImport   = "github.com/DocumentDrivenDX/ddx/internal/bead/internal/storage"
	forbiddenBeadLifecycleImport = "github.com/DocumentDrivenDX/ddx/internal/bead/internal/lifecycle"
)

// TestModuleBoundary_NoInternalImportsOutsideBead asserts that no Go package
// outside cli/internal/bead imports the relocated concrete storage/lifecycle
// packages (TD-027 §21/§23.4). Go's internal/ visibility rule already makes
// this physically impossible to compile; this test exists so a violation
// reports as a clear, named failure rather than a bare compiler error buried
// in an unrelated build.
func TestModuleBoundary_NoInternalImportsOutsideBead(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// thisFile is .../cli/internal/bead/module_boundary_test.go; walk up to cli/.
	cliRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	excludeDir := filepath.Join(cliRoot, "internal", "bead")
	skipDirs := map[string]bool{".git": true, "node_modules": true}

	fset := token.NewFileSet()
	err := filepath.Walk(cliRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			if path == excludeDir || strings.HasPrefix(path, excludeDir+string(filepath.Separator)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == forbiddenBeadStorageImport || importPath == forbiddenBeadLifecycleImport {
				rel, relErr := filepath.Rel(cliRoot, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s imports %s; concrete bead storage/lifecycle packages are only reachable via cli/internal/bead's public factory", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
