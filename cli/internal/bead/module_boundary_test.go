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

	// Transitional analyzers retired by TD-027 §21 / §23.4 once the
	// compiler-enforced internal/ packages and this AST boundary test carry
	// the boundary. They must not reappear as active primary guards.
	retiredAnalyzerConcreteStoreMethods        = "concrete-store-methods"
	retiredAnalyzerNoInternalStoreConstruction = "no-internal-store-construction"
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

// TestBoundaryLockdownDocsRetireTransitionalAnalyzers verifies TD-027 no longer
// presents concrete-store-methods / no-internal-store-construction as active
// primary guards, and that no bead-lints analyzer package under cli/tools
// reintroduces those retired transitional rules (TD-027 §21 / §23.4).
func TestBoundaryLockdownDocsRetireTransitionalAnalyzers(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// thisFile → cli/internal/bead → repo root is three levels up from bead, then one more from cli.
	cliRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	repoRoot := filepath.Dir(cliRoot)

	td027 := filepath.Join(repoRoot, "docs", "helix", "02-design", "technical-designs", "TD-027-bead-collection-abstraction.md")
	body, err := os.ReadFile(td027)
	if err != nil {
		t.Fatalf("read TD-027: %v", err)
	}
	text := string(body)

	for _, name := range []string{retiredAnalyzerConcreteStoreMethods, retiredAnalyzerNoInternalStoreConstruction} {
		if !strings.Contains(text, name) {
			// Mentions are optional once fully scrubbed, but when present they
			// must be marked retired/removed rather than active requirements.
			continue
		}
		// Require the analyzer name to appear with a retirement marker nearby.
		// TD-027 documents both as "**Removed.**" after BL landed.
		idx := 0
		for {
			pos := strings.Index(text[idx:], name)
			if pos < 0 {
				break
			}
			abs := idx + pos
			// Window after the name must include retirement language.
			windowEnd := abs + len(name) + 200
			if windowEnd > len(text) {
				windowEnd = len(text)
			}
			window := text[abs:windowEnd]
			retired := strings.Contains(window, "**Removed.**") ||
				strings.Contains(window, "Removed") ||
				strings.Contains(window, "drops the two transitional") ||
				strings.Contains(window, "dropped")
			if !retired {
				// Also accept "Lint suite drops the two transitional analyzers (`name`" style.
				preStart := abs - 80
				if preStart < 0 {
					preStart = 0
				}
				pre := text[preStart:abs]
				if strings.Contains(pre, "drops the two transitional") || strings.Contains(pre, "transitional analyzers") {
					// §23.4 bullet listing them as dropped.
					retired = true
				}
			}
			if !retired {
				t.Errorf("TD-027 mentions %q without a retirement marker (Removed/dropped) nearby; transitional analyzers must not be active primary guards", name)
			}
			idx = abs + len(name)
		}
	}

	// No analyzer implementation package may still ship under these names.
	lintRoot := filepath.Join(cliRoot, "tools", "lint")
	if _, err := os.Stat(lintRoot); err != nil {
		// Alternate historical path used in TD-027 wording.
		lintRoot = filepath.Join(cliRoot, "tools", "lints")
	}
	if info, err := os.Stat(lintRoot); err == nil && info.IsDir() {
		entries, err := os.ReadDir(lintRoot)
		if err != nil {
			t.Fatalf("read lint tools: %v", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			if n == retiredAnalyzerConcreteStoreMethods ||
				n == retiredAnalyzerNoInternalStoreConstruction ||
				n == "concretestoremethods" ||
				n == "nointernalstoreconstruction" ||
				strings.Contains(n, "concrete-store") ||
				strings.Contains(n, "no-internal-store") {
				t.Errorf("retired transitional analyzer package still present at %s/%s", lintRoot, n)
			}
		}
	}

	// Scan lint tool sources for analyzer name registrations that would
	// re-activate the retired rules as live CI guards.
	if info, err := os.Stat(filepath.Join(cliRoot, "tools")); err == nil && info.IsDir() {
		err := filepath.Walk(filepath.Join(cliRoot, "tools"), func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(src)
			// Only flag if the analyzer is registered as a live name constant
			// or Analyzer.Name assignment — not historical comments.
			for _, name := range []string{retiredAnalyzerConcreteStoreMethods, retiredAnalyzerNoInternalStoreConstruction} {
				if !strings.Contains(content, `"`+name+`"`) && !strings.Contains(content, "`"+name+"`") {
					continue
				}
				// Allow pure documentation comments; fail on Name: "..." style.
				if strings.Contains(content, `Name: "`+name+`"`) ||
					strings.Contains(content, `name: "`+name+`"`) ||
					strings.Contains(content, `Name = "`+name+`"`) {
					rel, _ := filepath.Rel(cliRoot, path)
					t.Errorf("%s still registers retired transitional analyzer %q as an active lint", rel, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
