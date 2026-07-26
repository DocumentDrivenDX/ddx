package cmd

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

	"github.com/stretchr/testify/require"
)

// cmdAndMetricsCallerPackages are the TD-027 §21 packages owned by
// ddx-cdc521b5 (command + metrics/exec caller narrowing).
var cmdAndMetricsCallerPackages = []string{
	"cmd",
	"internal/exec",
	"internal/processmetrics",
	"internal/agentmetrics",
	"internal/attemptmetrics",
}

// cmdAndMetricsConcreteStoreAllowlist documents every remaining production
// (non-test) *bead.Store field/param/result/local type exception in the
// command and metrics packages. Construction via bead.NewStore assigned to an
// interface-typed variable is not listed — only explicit *bead.Store types.
//
// Keys are paths relative to the cli/ module root (e.g. "cmd/bead.go").
var cmdAndMetricsConcreteStoreAllowlist = map[string]string{
	"cmd/bead.go": "beadStoreConcrete is the documented construction escape hatch for path-level migrate/archive/git helpers not yet on bead.Backend (TD-027 §21).",
}

// TestCallerMigration_CmdAndMetricsUseInterfaces AST-scans production sources
// under the command and metrics packages for concrete *bead.Store type usage
// outside the documented construction allowlist.
func TestCallerMigration_CmdAndMetricsUseInterfaces(t *testing.T) {
	t.Parallel()
	cliRoot := callerMigrationCLIRoot(t)
	violations := scanConcreteStoreTypes(t, cliRoot, cmdAndMetricsCallerPackages, cmdAndMetricsConcreteStoreAllowlist, false)
	if len(violations) > 0 {
		t.Fatalf("concrete *bead.Store type usage outside allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

// TestCallerMigration_AllowlistMatchesDocumentedExceptions ensures every
// allowlist entry names an existing production file and carries a non-empty
// reason, and that each allowlisted file still contains at least one
// *bead.Store type (stale allowlist entries fail).
func TestCallerMigration_AllowlistMatchesDocumentedExceptions(t *testing.T) {
	t.Parallel()
	cliRoot := callerMigrationCLIRoot(t)

	require.NotEmpty(t, cmdAndMetricsConcreteStoreAllowlist,
		"allowlist must document remaining command/metrics concrete exceptions")

	for rel, reason := range cmdAndMetricsConcreteStoreAllowlist {
		require.NotEmpty(t, strings.TrimSpace(reason), "allowlist entry %s must document a reason", rel)
		path := filepath.Join(cliRoot, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		require.NoError(t, err, "allowlist path %s must exist", rel)
		require.False(t, info.IsDir(), "allowlist path %s must be a file", rel)
		require.False(t, strings.HasSuffix(rel, "_test.go"),
			"allowlist entry %s is a test file; production exceptions only", rel)

		hits := scanFileConcreteStoreTypes(t, path)
		require.NotEmpty(t, hits,
			"allowlist entry %s has no remaining *bead.Store type usage — remove or update it", rel)
	}

	// Every production hit must be covered by the allowlist (same property as
	// TestCallerMigration_CmdAndMetricsUseInterfaces, re-checked here so the
	// allowlist test alone documents the exception set).
	violations := scanConcreteStoreTypes(t, cliRoot, cmdAndMetricsCallerPackages, cmdAndMetricsConcreteStoreAllowlist, false)
	require.Empty(t, violations, "allowlist incomplete:\n%s", strings.Join(violations, "\n"))
}

// TestCallerMigration_NoConcreteStore is the package-level gate for the
// command/metrics packages covered by this child bead. It shares the AST
// allowlist with TestCallerMigration_CmdAndMetricsUseInterfaces.
func TestCallerMigration_NoConcreteStore(t *testing.T) {
	t.Parallel()
	cliRoot := callerMigrationCLIRoot(t)
	violations := scanConcreteStoreTypes(t, cliRoot, cmdAndMetricsCallerPackages, cmdAndMetricsConcreteStoreAllowlist, false)
	if len(violations) > 0 {
		t.Fatalf("TestCallerMigration_NoConcreteStore: concrete *bead.Store usage outside allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

func callerMigrationCLIRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// cli/cmd/caller_migration_test.go -> cli/
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func scanConcreteStoreTypes(t *testing.T, cliRoot string, relPackages []string, allowlist map[string]string, includeTests bool) []string {
	t.Helper()
	var violations []string
	for _, relPkg := range relPackages {
		dir := filepath.Join(cliRoot, filepath.FromSlash(relPkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			if !includeTests && strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			rel := filepath.ToSlash(filepath.Join(relPkg, name))
			if _, allowed := allowlist[rel]; allowed {
				continue
			}
			for _, hit := range scanFileConcreteStoreTypes(t, path) {
				violations = append(violations, rel+": "+hit)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func scanFileConcreteStoreTypes(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, "parse %s", path)

	beadAlias := beadImportAlias(file)
	if beadAlias == "" {
		return nil
	}

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if isBeadStorePointer(expr, beadAlias) {
			pos := fset.Position(expr.Pos())
			hits = append(hits, pos.String()+": *"+beadAlias+".Store type")
			return false
		}
		return true
	})
	return hits
}

func beadImportAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != "github.com/DocumentDrivenDX/ddx/internal/bead" {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				return ""
			}
			return imp.Name.Name
		}
		return "bead"
	}
	return ""
}

func isBeadStorePointer(expr ast.Expr, beadAlias string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == beadAlias && x.Sel.Name == "Store"
	case *ast.Ident:
		// Same-package *Store would only appear inside package bead.
		return false
	default:
		return false
	}
}
