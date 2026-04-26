package runtimelint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestViolations runs the analyzer against fixtures that introduce one
// instance of each of the three forbidden patterns. The "// want"
// comments in the fixtures pin each expected diagnostic to its line.
func TestViolations(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "agent", "consumer")
}

// TestClean runs the analyzer against the post-cleanup-shape stub. The
// fixture mirrors the AC: a clean *Runtime + non-Runtime status type
// must produce zero diagnostics.
func TestClean(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "clean")
}
