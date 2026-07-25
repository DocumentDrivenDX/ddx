package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureCompleteUncovered maps REQ-001 and REQ-002 but leaves REQ-010
// out of the Verification table (inventory still has all three).
const fixtureCompleteUncovered = `---
ddx:
  id: FIXTURE-COMPLETE-UNCOVERED
---
# Fixture Complete With Uncovered Requirement

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-002: List resources

The system MUST list resources.

### REQ-010: Delete resource

The system MUST delete the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-002 | TestListResources | cd cli && go test ./pkg -run TestListResources |
`

// fixtureCompleteDuplicate maps REQ-001 twice.
const fixtureCompleteDuplicate = `---
ddx:
  id: FIXTURE-COMPLETE-DUPLICATE
---
# Fixture Complete With Duplicate Mapping

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-002: List resources

The system MUST list resources.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-001 | TestCreateResourceAlt | cd cli && go test ./pkg -run TestCreateResourceAlt |
| REQ-002 | TestListResources | cd cli && go test ./pkg -run TestListResources |
`

// fixtureCompleteExactlyOnce covers every inventory requirement once.
const fixtureCompleteExactlyOnce = `---
ddx:
  id: FIXTURE-COMPLETE-EXACTLY-ONCE
---
# Fixture Complete Exactly Once

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-002: List resources

The system MUST list resources.

### REQ-010: Delete resource

The system MUST delete the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-002 | TestListResources | cd cli && go test ./pkg -run TestListResources |
| REQ-010 | check:static-delete | go run ./tools/lint/deletecheck |
`

// TestCompleteVerificationCoverageCardinality_Uncovered: a Complete
// fixture with one requirement absent from the mapping rows emits a
// failure naming that requirement.
func TestCompleteVerificationCoverageCardinality_Uncovered(t *testing.T) {
	path := "docs/fixtures/complete_uncovered.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteUncovered)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteUncovered)
	if !containsString(model.Inventory, "REQ-010") {
		t.Fatalf("inventory must include REQ-010; got %#v", model.Inventory)
	}
	for _, row := range model.Rows {
		if row.RequirementRef == "REQ-010" {
			t.Fatalf("fixture must leave REQ-010 unmapped; rows=%+v", model.Rows)
		}
	}

	findings := CheckDocumentCoverageCardinality(path, fixtureCompleteUncovered)
	var uncovered []CoverageFinding
	for _, f := range findings {
		if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-010") {
			uncovered = append(uncovered, f)
		}
	}
	if len(uncovered) != 1 {
		t.Fatalf("expected exactly one uncovered finding naming REQ-010; got findings=%+v", findings)
	}
	f := uncovered[0]
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if f.Path != path {
		t.Fatalf("Path = %q, want %q", f.Path, path)
	}
	if !strings.Contains(f.Message, "REQ-010") {
		t.Fatalf("Message must name REQ-010; got %q", f.Message)
	}
	// No false positives for covered requirements.
	for _, f := range findings {
		if strings.Contains(f.Message, "REQ-001") || strings.Contains(f.Message, "REQ-002") {
			if f.Kind == FindingUnmetVerification {
				t.Fatalf("covered requirement must not emit uncovered diagnostic: %+v", f)
			}
		}
	}

	// Direct CheckCoverageCardinality path (same contract, no re-parse).
	direct := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Inventory:  model.Inventory,
		Rows:       model.Rows,
	})
	var directUncovered int
	for _, f := range direct {
		if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-010") {
			directUncovered++
		}
	}
	if directUncovered != 1 {
		t.Fatalf("direct CheckCoverageCardinality: want 1 uncovered for REQ-010; got %+v", direct)
	}
}

// TestCompleteVerificationCoverageCardinality_Duplicate: a Complete
// fixture mapping one requirement twice emits a duplicate-mapping
// failure naming that requirement.
func TestCompleteVerificationCoverageCardinality_Duplicate(t *testing.T) {
	path := "docs/fixtures/complete_duplicate.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteDuplicate)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteDuplicate)
	n001 := 0
	for _, row := range model.Rows {
		if row.RequirementRef == "REQ-001" {
			n001++
		}
	}
	if n001 != 2 {
		t.Fatalf("fixture must map REQ-001 twice; got %d rows for REQ-001; all=%+v", n001, model.Rows)
	}

	findings := CheckDocumentCoverageCardinality(path, fixtureCompleteDuplicate)
	var dups []CoverageFinding
	for _, f := range findings {
		if f.Kind == FindingDuplicateMapping {
			dups = append(dups, f)
		}
	}
	if len(dups) != 1 {
		t.Fatalf("expected exactly one duplicate_mapping finding; got findings=%+v", findings)
	}
	f := dups[0]
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if !strings.Contains(f.Message, "REQ-001") {
		t.Fatalf("duplicate-mapping Message must name REQ-001; got %q", f.Message)
	}
	if !strings.Contains(strings.ToLower(f.Message), "duplicate") {
		t.Fatalf("Message must describe duplicate mapping; got %q", f.Message)
	}
	// REQ-002 is covered exactly once: no finding for it.
	for _, f := range findings {
		if strings.Contains(f.Message, "REQ-002") {
			t.Fatalf("exactly-once REQ-002 must not emit a cardinality diagnostic: %+v", f)
		}
	}

	direct := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Inventory:  model.Inventory,
		Rows:       model.Rows,
	})
	var directDups int
	for _, f := range direct {
		if f.Kind == FindingDuplicateMapping && strings.Contains(f.Message, "REQ-001") {
			directDups++
		}
	}
	if directDups != 1 {
		t.Fatalf("direct CheckCoverageCardinality: want 1 duplicate for REQ-001; got %+v", direct)
	}
}

// TestCompleteVerificationCoverageCardinality_ExactlyOncePasses: a
// Complete fixture whose every inventory requirement has exactly one
// Verification mapping row emits no cardinality diagnostic.
func TestCompleteVerificationCoverageCardinality_ExactlyOncePasses(t *testing.T) {
	path := "docs/fixtures/complete_exactly_once.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteExactlyOnce)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteExactlyOnce)
	if len(model.Inventory) == 0 {
		t.Fatal("fixture inventory must be non-empty")
	}
	if len(model.Rows) != len(model.Inventory) {
		t.Fatalf("rows=%d inventory=%d; fixture must be 1:1", len(model.Rows), len(model.Inventory))
	}
	counts := map[string]int{}
	for _, row := range model.Rows {
		counts[row.RequirementRef]++
	}
	for _, req := range model.Inventory {
		if counts[req] != 1 {
			t.Fatalf("inventory %q coverage count = %d, want 1", req, counts[req])
		}
	}

	findings := CheckDocumentCoverageCardinality(path, fixtureCompleteExactlyOnce)
	if len(findings) != 0 {
		t.Fatalf("exactly-once coverage must emit no cardinality diagnostics; got %+v", findings)
	}

	// On-disk Complete fixture with 1:1 rows also passes.
	diskPath := filepath.Join("testdata", "docs", "with_requirement_ids.md")
	before := snapshotFixtures(t, filepath.Join("testdata", "docs"))
	data, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("read %s: %v", diskPath, err)
	}
	diskFindings := CheckDocumentCoverageCardinality(diskPath, string(data))
	if len(diskFindings) != 0 {
		t.Fatalf("on-disk Complete fixture with 1:1 coverage must pass; got %+v", diskFindings)
	}
	after := snapshotFixtures(t, filepath.Join("testdata", "docs"))
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("coverage-cardinality pass mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}

	// Direct path.
	direct := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Inventory:  model.Inventory,
		Rows:       model.Rows,
	})
	if len(direct) != 0 {
		t.Fatalf("direct CheckCoverageCardinality must be empty; got %+v", direct)
	}
}

// TestCompleteVerificationCoverageCardinality_FileOnlyRowsFilteredBeforeJoin:
// duplicate or unrelated file-only mapping rows are excluded by the
// pre-join FilterCoveringRows stage and never enter the cardinality join
// multiset. A Complete requirement whose only rows are file-only citations
// is still reported as uncovered no matter how many such rows exist.
func TestCompleteVerificationCoverageCardinality_FileOnlyRowsFilteredBeforeJoin(t *testing.T) {
	path := "docs/fixtures/complete_file_only_duplicates.md"

	// Constructed case: many duplicate + unrelated file-only rows for one
	// requirement, and an unrelated file-only row for a second requirement.
	// None name a covering target, so both inventory members stay uncovered.
	rawRows := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 10},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 11}, // duplicate file-only
		{RequirementRef: "REQ-001", EvidenceTarget: "cli/internal/bead/store_test.go", Command: "go test ./cli/internal/bead", Line: 12}, // unrelated file-only
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/other_test.go", Command: "go test ./pkg", Line: 13}, // another file-only
		{RequirementRef: "REQ-002", EvidenceTarget: "pkg/list_test.go", Command: "go test ./pkg", Line: 14},
	}
	for _, row := range rawRows {
		if IsCoveringCitation(row) {
			t.Fatalf("test row must be non-covering file-only; got covering EvidenceTarget=%q", row.EvidenceTarget)
		}
	}

	// Explicit pre-join filter stage drops every file-only row.
	filtered := FilterCoveringRows(rawRows)
	if len(filtered) != 0 {
		t.Fatalf("FilterCoveringRows must drop all file-only rows before the join; got %+v", filtered)
	}

	findings := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     StatusComplete,
		StatusLine: 5,
		Inventory:  []string{"REQ-001", "REQ-002"},
		Rows:       rawRows,
	})
	if len(findings) != 2 {
		t.Fatalf("duplicate/unrelated file-only rows must not mask uncovered Complete requirements; want 2 uncovered, got findings=%+v", findings)
	}
	for _, req := range []string{"REQ-001", "REQ-002"} {
		found := false
		for _, f := range findings {
			if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, req) {
				if f.Severity != SeverityError {
					t.Fatalf("%s Severity = %q, want %q", req, f.Severity, SeverityError)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected uncovered diagnostic for %s after pre-join filter; got %+v", req, findings)
		}
	}
	// No duplicate-mapping false positive from multiple file-only rows.
	for _, f := range findings {
		if f.Kind == FindingDuplicateMapping {
			t.Fatalf("file-only duplicates must not enter the join multiset as duplicate covering rows; got %+v", f)
		}
	}

	// Mixed multiset: covering rows pass the filter; file-only duplicates
	// for the same requirement are dropped and do not create a duplicate
	// cardinality finding or mask a second uncovered requirement.
	mixedRaw := []VerificationRow{
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 20},
		{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 21},
		{RequirementRef: "REQ-001", EvidenceTarget: "TestCreateResource", Command: "go test ./pkg -run TestCreateResource", Line: 22},
		{RequirementRef: "REQ-010", EvidenceTarget: "pkg/delete_test.go", Command: "go test ./pkg", Line: 23},
		{RequirementRef: "REQ-010", EvidenceTarget: "pkg/delete_test.go", Command: "go test ./pkg", Line: 24},
	}
	mixedFiltered := FilterCoveringRows(mixedRaw)
	if len(mixedFiltered) != 1 {
		t.Fatalf("FilterCoveringRows must keep only the covering Test* row; got %+v", mixedFiltered)
	}
	if mixedFiltered[0].EvidenceTarget != "TestCreateResource" {
		t.Fatalf("FilterCoveringRows kept wrong row: %+v", mixedFiltered[0])
	}
	mixedFindings := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:      path,
		Status:    StatusComplete,
		Inventory: []string{"REQ-001", "REQ-010"},
		Rows:      mixedRaw,
	})
	// REQ-001 covered exactly once (file-only dups dropped); REQ-010 uncovered.
	var uncovered010, uncovered001, dups int
	for _, f := range mixedFindings {
		switch {
		case f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-010"):
			uncovered010++
		case f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-001"):
			uncovered001++
		case f.Kind == FindingDuplicateMapping:
			dups++
		}
	}
	if uncovered010 != 1 {
		t.Fatalf("REQ-010 with only file-only rows must stay uncovered; findings=%+v", mixedFindings)
	}
	if uncovered001 != 0 {
		t.Fatalf("REQ-001 with one covering row after filter must not be uncovered; findings=%+v", mixedFindings)
	}
	if dups != 0 {
		t.Fatalf("file-only duplicates must not produce duplicate_mapping after pre-join filter; findings=%+v", mixedFindings)
	}

	// Fixture path: single file-only row still filtered before join.
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteFileOnlyTest)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteFileOnlyTest)
	if len(FilterCoveringRows(model.Rows)) != 0 {
		t.Fatalf("fixture file-only rows must be dropped by FilterCoveringRows; rows=%+v", model.Rows)
	}
	docFindings := CheckDocumentCoverageCardinality(path, fixtureCompleteFileOnlyTest)
	var uncovered []CoverageFinding
	for _, f := range docFindings {
		if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-001") {
			uncovered = append(uncovered, f)
		}
	}
	if len(uncovered) != 1 {
		t.Fatalf("file-only fixture must leave REQ-001 uncovered after pre-join filter; got findings=%+v", docFindings)
	}
}

// TestCompleteVerificationCoverageCardinality_IgnoresFileOnlyCitations:
// a Complete fixture that cites only a test file path without a specific
// mapped target does not count that citation as requirement coverage, and
// the affected requirement is reported as uncovered.
func TestCompleteVerificationCoverageCardinality_IgnoresFileOnlyCitations(t *testing.T) {
	path := "docs/fixtures/complete_file_only_test.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteFileOnlyTest)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteFileOnlyTest)
	if len(model.Inventory) == 0 {
		t.Fatal("fixture inventory must be non-empty")
	}
	if len(model.Rows) == 0 {
		t.Fatal("fixture must have at least one mapping row (file-only citation)")
	}
	for _, row := range model.Rows {
		if IsCoveringCitation(row) {
			t.Fatalf("fixture row must be non-covering file-only; got covering EvidenceTarget=%q", row.EvidenceTarget)
		}
	}

	findings := CheckDocumentCoverageCardinality(path, fixtureCompleteFileOnlyTest)
	var uncovered []CoverageFinding
	for _, f := range findings {
		if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-001") {
			uncovered = append(uncovered, f)
		}
	}
	if len(uncovered) != 1 {
		t.Fatalf("file-only citation must leave REQ-001 uncovered; got findings=%+v", findings)
	}
	f := uncovered[0]
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if f.Path != path {
		t.Fatalf("Path = %q, want %q", f.Path, path)
	}
	if !strings.Contains(f.Message, "REQ-001") {
		t.Fatalf("Message must name REQ-001; got %q", f.Message)
	}
	if !strings.Contains(strings.ToLower(f.Message), "uncovered") {
		t.Fatalf("Message must report uncovered; got %q", f.Message)
	}

	// Direct CheckCoverageCardinality path (same contract, no re-parse).
	direct := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Inventory:  model.Inventory,
		Rows:       model.Rows,
	})
	var directUncovered int
	for _, f := range direct {
		if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, "REQ-001") {
			directUncovered++
		}
	}
	if directUncovered != 1 {
		t.Fatalf("direct CheckCoverageCardinality: want 1 uncovered for REQ-001; got %+v", direct)
	}

	// Constructed multi-req case: only file-only rows → all uncovered;
	// no false "covered" from non-covering citations.
	multi := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:   path,
		Status: StatusComplete,
		Inventory: []string{"REQ-A", "REQ-B"},
		Rows: []VerificationRow{
			{RequirementRef: "REQ-A", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 10},
			{RequirementRef: "REQ-B", EvidenceTarget: "cli/internal/bead/store_test.go", Command: "go test ./cli/internal/bead", Line: 11},
		},
	})
	if len(multi) != 2 {
		t.Fatalf("two file-only-only requirements must both be uncovered; got %+v", multi)
	}
	for _, req := range []string{"REQ-A", "REQ-B"} {
		found := false
		for _, f := range multi {
			if f.Kind == FindingUnmetVerification && strings.Contains(f.Message, req) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected uncovered diagnostic for %s; got %+v", req, multi)
		}
	}
}

// TestCompleteVerificationCoverageCardinality_TargetedRowsCount: a
// Complete fixture whose mapping row names an exact Test* symbol, static
// check, or artifact target counts as coverage and yields no uncovered
// diagnostic for that requirement.
func TestCompleteVerificationCoverageCardinality_TargetedRowsCount(t *testing.T) {
	// Exact Test* symbol.
	testPath := "docs/fixtures/complete_existing_test.md"
	testStatus := ParseDocumentStatusMarkdown(testPath, fixtureCompleteExistingTest)
	if testStatus.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", testStatus.Status, StatusComplete)
	}
	testModel := ParseVerificationMarkdown(testPath, fixtureCompleteExistingTest)
	if len(testModel.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(testModel.Rows))
	}
	if !IsCoveringCitation(testModel.Rows[0]) {
		t.Fatalf("exact Test* row must be covering; EvidenceTarget=%q", testModel.Rows[0].EvidenceTarget)
	}
	testFindings := CheckDocumentCoverageCardinality(testPath, fixtureCompleteExistingTest)
	for _, f := range testFindings {
		if f.Kind == FindingUnmetVerification {
			t.Fatalf("targeted Test* coverage must not emit uncovered diagnostic; got %+v", testFindings)
		}
	}
	if len(testFindings) != 0 {
		t.Fatalf("exactly-once targeted Test* must emit no cardinality diagnostics; got %+v", testFindings)
	}

	// Exact static check (check:<name>).
	checkPath := "docs/fixtures/complete_static_check.md"
	checkModel := ParseVerificationMarkdown(checkPath, fixtureCompleteStaticCheck)
	if len(checkModel.Rows) != 1 {
		t.Fatalf("static-check rows = %d, want 1", len(checkModel.Rows))
	}
	if !IsCoveringCitation(checkModel.Rows[0]) {
		t.Fatalf("static check row must be covering; EvidenceTarget=%q", checkModel.Rows[0].EvidenceTarget)
	}
	checkFindings := CheckDocumentCoverageCardinality(checkPath, fixtureCompleteStaticCheck)
	for _, f := range checkFindings {
		if f.Kind == FindingUnmetVerification {
			t.Fatalf("targeted static-check coverage must not emit uncovered; got %+v", checkFindings)
		}
	}
	if len(checkFindings) != 0 {
		t.Fatalf("exactly-once static check must emit no cardinality diagnostics; got %+v", checkFindings)
	}

	// Exact artifact target.
	artPath := "docs/fixtures/complete_artifact_target.md"
	artModel := ParseVerificationMarkdown(artPath, fixtureCompleteArtifactTarget)
	if len(artModel.Rows) != 1 {
		t.Fatalf("artifact rows = %d, want 1", len(artModel.Rows))
	}
	if !IsCoveringCitation(artModel.Rows[0]) {
		t.Fatalf("artifact row must be covering; EvidenceTarget=%q", artModel.Rows[0].EvidenceTarget)
	}
	artFindings := CheckDocumentCoverageCardinality(artPath, fixtureCompleteArtifactTarget)
	for _, f := range artFindings {
		if f.Kind == FindingUnmetVerification {
			t.Fatalf("targeted artifact coverage must not emit uncovered; got %+v", artFindings)
		}
	}
	if len(artFindings) != 0 {
		t.Fatalf("exactly-once artifact must emit no cardinality diagnostics; got %+v", artFindings)
	}

	// Mixed document: covering rows count; file-only does not mask
	// (sibling ordering/masking is out of scope — here only assert that a
	// requirement with one targeted row is not uncovered).
	mixed := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:      "docs/fixtures/mixed.md",
		Status:    StatusComplete,
		Inventory: []string{"REQ-001", "REQ-002", "REQ-010"},
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: "TestCreateResource", Command: "go test ./pkg -run TestCreateResource", Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: "check:static-list", Command: "go run ./tools/lint/listcheck", Line: 2},
			{RequirementRef: "REQ-010", EvidenceTarget: ".ddx/executions/fixture/report.json", Command: "test -f .ddx/executions/fixture/report.json", Line: 3},
		},
	})
	if len(mixed) != 0 {
		t.Fatalf("targeted Test*/check/artifact rows must cover all inventory; got %+v", mixed)
	}

	// Direct construction: scoped Test* still counts.
	scoped := CheckCoverageCardinality(CoverageCardinalityInput{
		Path:      "docs/fixtures/scoped.md",
		Status:    StatusComplete,
		Inventory: []string{"REQ-001"},
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: "pkg/existing_test.go:TestCreateResource", Command: "go test ./pkg", Line: 1},
		},
	})
	if len(scoped) != 0 {
		t.Fatalf("scoped Test* must count as coverage; got %+v", scoped)
	}
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
