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

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
