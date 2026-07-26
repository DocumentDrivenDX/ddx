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

	"github.com/stretchr/testify/require"
)

// agentAndServerCallerPackages are the TD-027 §21 packages owned by
// ddx-a9652d0d (agent + server + escalation caller narrowing).
var agentAndServerCallerPackages = []string{
	"internal/agent",
	"internal/agent/try",
	"internal/agent/coordination",
	"internal/server",
	"internal/server/graphql",
	"internal/escalation",
}

// agentAndServerConcreteStoreAllowlist documents every remaining production
// (non-test) *bead.Store field/param/result/local type exception in the
// agent/server/escalation packages. Construction via bead.NewStore assigned to
// an interface-typed variable is not listed — only explicit *bead.Store types.
//
// Keys are paths relative to the cli/ module root.
var agentAndServerConcreteStoreAllowlist = map[string]string{
	"internal/agent/coordination/local.go": "compile-time ClaimBackend satisfaction proof that *bead.Store implements the production claim backend (TD-027 §21).",
}

// TestCallerMigration_AgentAndServerUseInterfaces AST-scans production sources
// under the agent, agent/try, server, server/graphql, and escalation packages
// for concrete *bead.Store type usage outside the documented
// construction/test-fixture allowlist.
func TestCallerMigration_AgentAndServerUseInterfaces(t *testing.T) {
	t.Parallel()
	cliRoot := agentCallerMigrationCLIRoot(t)
	violations := scanAgentServerConcreteStoreTypes(t, cliRoot, agentAndServerCallerPackages, agentAndServerConcreteStoreAllowlist, false)
	if len(violations) > 0 {
		t.Fatalf("concrete *bead.Store type usage outside allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

// TestCallerMigration_AgentAndServerAllowlistMatchesDocumentedExceptions ensures
// every allowlist entry names an existing production file and still contains at
// least one *bead.Store type (stale allowlist entries fail).
func TestCallerMigration_AgentAndServerAllowlistMatchesDocumentedExceptions(t *testing.T) {
	t.Parallel()
	cliRoot := agentCallerMigrationCLIRoot(t)

	require.NotEmpty(t, agentAndServerConcreteStoreAllowlist,
		"allowlist must document remaining agent/server concrete exceptions")

	for rel, reason := range agentAndServerConcreteStoreAllowlist {
		require.NotEmpty(t, strings.TrimSpace(reason), "allowlist entry %s must document a reason", rel)
		path := filepath.Join(cliRoot, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		require.NoError(t, err, "allowlist path %s must exist", rel)
		require.False(t, info.IsDir(), "allowlist path %s must be a file", rel)
		require.False(t, strings.HasSuffix(rel, "_test.go"),
			"allowlist entry %s is a test file; production exceptions only", rel)

		hits := scanAgentServerFileConcreteStoreTypes(t, path)
		require.NotEmpty(t, hits,
			"allowlist entry %s has no remaining *bead.Store type usage — remove or update it", rel)
	}

	violations := scanAgentServerConcreteStoreTypes(t, cliRoot, agentAndServerCallerPackages, agentAndServerConcreteStoreAllowlist, false)
	require.Empty(t, violations, "allowlist incomplete:\n%s", strings.Join(violations, "\n"))
}

func agentCallerMigrationCLIRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// cli/internal/agent/caller_migration_test.go -> cli/
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func scanAgentServerConcreteStoreTypes(t *testing.T, cliRoot string, relPackages []string, allowlist map[string]string, includeTests bool) []string {
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
			for _, hit := range scanAgentServerFileConcreteStoreTypes(t, path) {
				violations = append(violations, rel+": "+hit)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func scanAgentServerFileConcreteStoreTypes(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, "parse %s", path)

	beadAlias := agentServerBeadImportAlias(file)
	if beadAlias == "" {
		return nil
	}

	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if isAgentServerBeadStorePointer(expr, beadAlias) {
			pos := fset.Position(expr.Pos())
			hits = append(hits, pos.String()+": *"+beadAlias+".Store type")
			return false
		}
		return true
	})
	return hits
}

func agentServerBeadImportAlias(file *ast.File) string {
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

func isAgentServerBeadStorePointer(expr ast.Expr, beadAlias string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == beadAlias && x.Sel.Name == "Store"
	default:
		return false
	}
}
