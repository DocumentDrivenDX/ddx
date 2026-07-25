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

// fixtureCompleteStaticCheck maps a requirement to an exact static check.
const fixtureCompleteStaticCheck = `---
ddx:
  id: FIXTURE-COMPLETE-STATIC-CHECK
---
# Fixture Complete Static Check Evidence

**Status:** Complete

## Requirements

### REQ-010: Static delete guard

The system MUST reject forbidden deletes.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-010 | check:static-delete | go run ./tools/lint/deletecheck |
`

// fixtureCompleteArtifactTarget maps a requirement to an exact artifact target.
const fixtureCompleteArtifactTarget = `---
ddx:
  id: FIXTURE-COMPLETE-ARTIFACT-TARGET
---
# Fixture Complete Artifact Target Evidence

**Status:** Complete

## Requirements

### REQ-020: Observation report

The system MUST emit a machine-readable observation report.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-020 | .ddx/executions/fixture/report.json | test -f .ddx/executions/fixture/report.json |
`

// TestCitationGranularity_StaticCheckRowsCount: a Verification mapping
// row naming an exact static check is classified as covering.
func TestCitationGranularity_StaticCheckRowsCount(t *testing.T) {
	path := "docs/fixtures/complete_static_check.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteStaticCheck)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	row := model.Rows[0]
	if row.EvidenceTarget != "check:static-delete" {
		t.Fatalf("EvidenceTarget = %q, want check:static-delete", row.EvidenceTarget)
	}
	if !isStaticCheckTarget(row.EvidenceTarget) {
		t.Fatalf("isStaticCheckTarget(%q) = false, want true", row.EvidenceTarget)
	}
	// Static checks are out-of-band for the test and artifact resolvers.
	if parsed := parseTestEvidenceTarget(row.EvidenceTarget, row.Command); parsed.kind != evidenceKindIgnore {
		t.Fatalf("parseTestEvidenceTarget kind = %v, want evidenceKindIgnore for static check", parsed.kind)
	}
	if got := ClassifyRuntimeArtifactTarget(row.EvidenceTarget); got.Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("ClassifyRuntimeArtifactTarget Kind = %q, want out_of_band for static check", got.Kind)
	}

	if !IsCoveringCitation(row) {
		t.Fatalf("static check %q must be covering; IsCoveringCitation = false", row.EvidenceTarget)
	}

	// Existence on disk is not required for citation granularity.
	staticCases := []VerificationRow{
		{RequirementRef: "REQ-010", EvidenceTarget: "check:static-delete", Command: "go run ./tools/lint/deletecheck", Line: 1},
		{RequirementRef: "REQ-010", EvidenceTarget: "check:lockreentry", Command: "go run ./tools/lint/lockreentrylint", Line: 2},
		{RequirementRef: "REQ-010", EvidenceTarget: "`check:spechonesty`", Command: "go run ./tools/lint/spechonesty", Line: 3},
		{RequirementRef: "REQ-010", EvidenceTarget: "check:phantom-but-named", Command: "true", Line: 4},
		{RequirementRef: "REQ-010", EvidenceTarget: "CHECK:CaseInsensitive", Command: "true", Line: 5},
	}
	for _, row := range staticCases {
		if !IsCoveringCitation(row) {
			t.Errorf("static check evidence %q must be covering", row.EvidenceTarget)
		}
		if !isStaticCheckTarget(row.EvidenceTarget) {
			t.Errorf("isStaticCheckTarget(%q) = false, want true", row.EvidenceTarget)
		}
	}

	// Prefix alone or empty name is not an exact static check.
	nonChecks := []VerificationRow{
		{RequirementRef: "REQ-010", EvidenceTarget: "check:", Command: "true", Line: 1},
		{RequirementRef: "REQ-010", EvidenceTarget: "check:   ", Command: "true", Line: 2},
		{RequirementRef: "REQ-010", EvidenceTarget: "static-delete", Command: "true", Line: 3},
	}
	for _, row := range nonChecks {
		if isStaticCheckTarget(row.EvidenceTarget) {
			t.Errorf("isStaticCheckTarget(%q) = true, want false", row.EvidenceTarget)
		}
		if IsCoveringCitation(row) {
			t.Errorf("non-static-check evidence %q must be non-covering", row.EvidenceTarget)
		}
	}
}

// TestCitationGranularity_ArtifactTargetRowsCount: a Verification mapping
// row naming an exact artifact target is classified as covering.
func TestCitationGranularity_ArtifactTargetRowsCount(t *testing.T) {
	path := "docs/fixtures/complete_artifact_target.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteArtifactTarget)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	row := model.Rows[0]
	if row.EvidenceTarget != ".ddx/executions/fixture/report.json" {
		t.Fatalf("EvidenceTarget = %q, want .ddx/executions/fixture/report.json", row.EvidenceTarget)
	}
	if !isArtifactTarget(row.EvidenceTarget) {
		t.Fatalf("isArtifactTarget(%q) = false, want true", row.EvidenceTarget)
	}
	got := ClassifyRuntimeArtifactTarget(row.EvidenceTarget)
	if got.Kind != RuntimeArtifactClassRuntime {
		t.Fatalf("ClassifyRuntimeArtifactTarget Kind = %q, want runtime_artifact", got.Kind)
	}

	if !IsCoveringCitation(row) {
		t.Fatalf("artifact target %q must be covering; IsCoveringCitation = false", row.EvidenceTarget)
	}

	// Existence on disk is not required for citation granularity.
	artifactCases := []VerificationRow{
		{RequirementRef: "REQ-020", EvidenceTarget: ".ddx/executions/fixture/report.json", Command: "test -f .ddx/executions/fixture/report.json", Line: 1},
		{RequirementRef: "REQ-020", EvidenceTarget: "cli/testdata/observation.json", Command: "test -f cli/testdata/observation.json", Line: 2},
		{RequirementRef: "REQ-020", EvidenceTarget: "`.ddx/executions/run-1/result.json`", Command: "test -f .ddx/executions/run-1/result.json", Line: 3},
		{RequirementRef: "REQ-020", EvidenceTarget: "observation-report.json", Command: "test -f observation-report.json", Line: 4},
		{RequirementRef: "REQ-020", EvidenceTarget: "./artifacts/out.xml", Command: "test -f artifacts/out.xml", Line: 5},
		{RequirementRef: "REQ-020", EvidenceTarget: "artifacts/does-not-need-to-exist.json", Command: "test -f artifacts/does-not-need-to-exist.json", Line: 6},
	}
	for _, row := range artifactCases {
		if !IsCoveringCitation(row) {
			t.Errorf("artifact evidence %q must be covering", row.EvidenceTarget)
		}
		if !isArtifactTarget(row.EvidenceTarget) {
			t.Errorf("isArtifactTarget(%q) = false, want true", row.EvidenceTarget)
		}
	}
}

// TestCitationGranularity_BareFilePathIsNotArtifactTarget: a Verification
// mapping row whose evidence is only a file path and does not name an
// artifact target is classified as non-covering.
func TestCitationGranularity_BareFilePathIsNotArtifactTarget(t *testing.T) {
	// File-only Go paths: the test-symbol branch owns them as non-covering
	// and the runtime-artifact classifier marks them out-of-band (not
	// artifact targets).
	path := "docs/fixtures/complete_file_only_test.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteFileOnlyTest)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	row := model.Rows[0]
	if isArtifactTarget(row.EvidenceTarget) {
		t.Fatalf("bare Go test path %q must not be an artifact target", row.EvidenceTarget)
	}
	if ClassifyRuntimeArtifactTarget(row.EvidenceTarget).Kind != RuntimeArtifactClassOutOfBand {
		t.Fatalf("bare Go test path must be out_of_band for artifact classification")
	}
	if IsCoveringCitation(row) {
		t.Fatalf("bare file path %q must be non-covering", row.EvidenceTarget)
	}

	// Bare paths that point at source or free text without naming an
	// inspectable runtime artifact are not covering.
	bareCases := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 1},
		{RequirementRef: "REQ-001", EvidenceTarget: "cli/internal/bead/store.go", Command: "true", Line: 2},
		{RequirementRef: "REQ-001", EvidenceTarget: "`pkg/foo_test.go`", Command: "go test ./pkg", Line: 3},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go:42", Command: "go test ./pkg", Line: 4},
		{RequirementRef: "REQ-001", EvidenceTarget: "manual inspection of the dashboard", Command: "true", Line: 5},
		{RequirementRef: "REQ-001", EvidenceTarget: "https://example.com/report.json", Command: "true", Line: 6},
		{RequirementRef: "REQ-001", EvidenceTarget: "", Command: "true", Line: 7},
	}
	for _, row := range bareCases {
		if isArtifactTarget(row.EvidenceTarget) {
			t.Errorf("evidence %q must not be classified as an artifact target", row.EvidenceTarget)
		}
		if IsCoveringCitation(row) {
			t.Errorf("bare/non-target evidence %q must be non-covering", row.EvidenceTarget)
		}
	}
}
