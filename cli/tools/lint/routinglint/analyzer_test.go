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
	analysistest.Run(t, dir, Analyzer, "violations")
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
