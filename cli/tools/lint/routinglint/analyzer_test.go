package routinglint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestViolations runs the analyzer against fixtures that introduce
// one instance of each forbidden pattern. The "// want" comments in
// the fixtures pin each expected diagnostic to its line.
func TestViolations(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "violations", "cli/internal/agent/legacysurface")
}

// TestClean runs the analyzer against the post-cleanup-shape stub.
// The fixture mirrors AC #1 of ddx-653f6ac9: zero matches in DDx for
// retired compensating-routing tokens means the analyzer must
// produce zero diagnostics on a clean tree.
func TestClean(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "clean")
}

func TestRoutinglint_ForbidsRetiredCatalogIdentifiers(t *testing.T) {
	want := []string{
		"BuiltinCatalog",
		"DefaultModelCatalogPath",
		"LoadModelCatalogYAML",
		"ApplyModelCatalogYAML",
		"DefaultModelCatalogYAML",
		"CheckDeprecatedPin",
	}
	for _, ident := range want {
		if _, ok := forbiddenIdents[ident]; !ok {
			t.Fatalf("missing forbidden identifier %q", ident)
		}
	}
}

func TestRoutinglint_ForbidsRetiredProfileShadowingSymbols(t *testing.T) {
	want := []string{
		"ResolveModelTier",
		"resolveStrongSplitterMinPower",
		"isStrongPowerUnsatisfiedError",
		"isSmartRouteUnavailableError",
	}
	for _, ident := range want {
		if _, ok := forbiddenIdents[ident]; !ok {
			t.Fatalf("missing forbidden identifier %q", ident)
		}
	}
}

func TestRoutinglint_AllowlistedHistoricalTerms(t *testing.T) {
	want := map[string]string{
		"agentskills.io":                 "external skill-site reference retained for historical docs",
		"legacy agent":                   "historical workflow wording retained in migration docs",
		"Agent Service":                  "historical service name retained in docs",
		"cli/internal/agent":             "historical package path retained in docs and tests",
		"agent.routing.default_harness":  "removed config key retained in migration docs",
		"agent.routing.profile_priority": "deprecated config key retained in migration docs",
	}
	for lit, reason := range want {
		if got, ok := allowlistedLiterals[lit]; !ok {
			t.Fatalf("missing allowlisted literal %q", lit)
		} else if got != reason {
			t.Fatalf("allowlisted literal %q reason = %q, want %q", lit, got, reason)
		}
	}
}
