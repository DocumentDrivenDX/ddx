package evidencelint

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestViolations runs the analyzer against fixtures that introduce one
// unannotated instance of each of the four sink patterns. The "// want"
// comments in the fixtures pin each expected diagnostic to its line.
func TestViolations(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "violations", "server")
}

// TestAllowAnnotation runs the analyzer against fixtures where every
// violation carries a justifying evidence:allow-unbounded annotation.
// No diagnostics should be reported.
func TestAllowAnnotation(t *testing.T) {
	dir := analysistest.TestData()
	analysistest.Run(t, dir, Analyzer, "allowed")
}

// TestEmptyReasonRejected verifies the parser requires a non-empty
// reason="..." clause: an annotation without one does not suppress.
func TestEmptyReasonRejected(t *testing.T) {
	if hasNonEmptyReason(`// evidence:allow-unbounded reason=""`) {
		t.Fatal("empty reason must not suppress")
	}
	if hasNonEmptyReason(`// evidence:allow-unbounded`) {
		t.Fatal("missing reason must not suppress")
	}
	if !hasNonEmptyReason(`// evidence:allow-unbounded reason="legit"`) {
		t.Fatal("non-empty reason must suppress")
	}
}
