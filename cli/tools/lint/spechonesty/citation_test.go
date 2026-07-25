package spechonesty

import (
	"strings"
	"testing"
)

// TestCitationGranularity_FileOnlyRowsDoNotCount: a Verification mapping
// row whose evidence is only a test file path is classified as
// non-covering when it has no exact mapped Test* symbol.
func TestCitationGranularity_FileOnlyRowsDoNotCount(t *testing.T) {
	path := "docs/fixtures/complete_file_only_test.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteFileOnlyTest)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	row := model.Rows[0]
	if row.EvidenceTarget != "pkg/existing_test.go" {
		t.Fatalf("EvidenceTarget = %q, want pkg/existing_test.go", row.EvidenceTarget)
	}
	// Sanity: parser classifies this as file-only test evidence.
	parsed := parseTestEvidenceTarget(row.EvidenceTarget, row.Command)
	if parsed.kind != evidenceKindFileOnly {
		t.Fatalf("parseTestEvidenceTarget kind = %v, want evidenceKindFileOnly", parsed.kind)
	}

	if IsCoveringCitation(row) {
		t.Fatalf("file-only test path %q must be non-covering; IsCoveringCitation = true", row.EvidenceTarget)
	}

	// Direct construction of common file-only shapes.
	fileOnlyCases := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 1},
		{RequirementRef: "REQ-001", EvidenceTarget: "cli/internal/bead/store_test.go", Command: "go test ./cli/internal/bead", Line: 2},
		{RequirementRef: "REQ-001", EvidenceTarget: "`pkg/foo_test.go`", Command: "go test ./pkg", Line: 3},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go:42", Command: "go test ./pkg", Line: 4},
	}
	for _, row := range fileOnlyCases {
		if IsCoveringCitation(row) {
			t.Errorf("file-only evidence %q must be non-covering", row.EvidenceTarget)
		}
	}
}

// TestCitationGranularity_ExactTestSymbolRowsCount: a Verification mapping
// row naming an exact Test* symbol is classified as covering.
func TestCitationGranularity_ExactTestSymbolRowsCount(t *testing.T) {
	path := "docs/fixtures/complete_existing_test.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteExistingTest)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	row := model.Rows[0]
	if row.EvidenceTarget != "TestCreateResource" {
		t.Fatalf("EvidenceTarget = %q, want TestCreateResource", row.EvidenceTarget)
	}
	// Sanity: parser classifies this as an exact Test* symbol.
	parsed := parseTestEvidenceTarget(row.EvidenceTarget, row.Command)
	if parsed.kind != evidenceKindTestSymbol {
		t.Fatalf("parseTestEvidenceTarget kind = %v, want evidenceKindTestSymbol", parsed.kind)
	}
	if parsed.symbol != "TestCreateResource" {
		t.Fatalf("symbol = %q, want TestCreateResource", parsed.symbol)
	}

	if !IsCoveringCitation(row) {
		t.Fatalf("exact Test* %q must be covering; IsCoveringCitation = false", row.EvidenceTarget)
	}

	// Bare, scoped, and markdown-wrapped exact Test* forms all count.
	// Existence on disk is not required for citation granularity.
	exactCases := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "TestCreateResource", Command: "go test ./pkg -run TestCreateResource", Line: 1},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go:TestCreateResource", Command: "go test ./pkg", Line: 2},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go#TestCreateResource", Command: "go test ./pkg", Line: 3},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg.TestCreateResource", Command: "go test ./pkg", Line: 4},
		{RequirementRef: "REQ-001", EvidenceTarget: "`TestDoesNotNeedToExistOnDisk`", Command: "go test ./pkg -run TestDoesNotNeedToExistOnDisk", Line: 5},
		{RequirementRef: "REQ-001", EvidenceTarget: "TestPhantomButNamed", Command: "go test ./nowhere -run TestPhantomButNamed", Line: 6},
	}
	for _, row := range exactCases {
		if !IsCoveringCitation(row) {
			t.Errorf("exact Test* evidence %q must be covering", row.EvidenceTarget)
		}
		// File-only and exact-Test* classifications must remain distinct.
		if strings.Contains(row.EvidenceTarget, "_test.go") && !strings.Contains(row.EvidenceTarget, "Test") {
			t.Fatalf("test case misconfigured as file-only: %q", row.EvidenceTarget)
		}
	}
}
