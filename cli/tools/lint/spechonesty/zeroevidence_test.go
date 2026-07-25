package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureCompleteZeroEvidence is stamped Complete with no Verification rows.
const fixtureCompleteZeroEvidence = `---
ddx:
  id: FIXTURE-COMPLETE-ZERO-EVIDENCE
---
# Fixture Complete With Zero Evidence

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.
`

// fixtureImplementedZeroEvidence is stamped Implemented with no Verification rows.
const fixtureImplementedZeroEvidence = `---
ddx:
  id: FIXTURE-IMPLEMENTED-ZERO-EVIDENCE
---
# Fixture Implemented With Zero Evidence

**Status:** Implemented

## Requirements

### REQ-001: Create resource

The system MUST create the resource.
`

// fixtureCompleteWithRows is stamped Complete and has at least one mapping row.
const fixtureCompleteWithRows = `---
ddx:
  id: FIXTURE-COMPLETE-WITH-ROWS
---
# Fixture Complete With Verification Rows

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
`

// fixtureProposedNoRows is not Complete/Implemented and has no rows.
const fixtureProposedNoRows = `---
ddx:
  id: FIXTURE-PROPOSED-NO-ROWS
---
# Fixture Proposed Without Rows

**Status:** Proposed

## Requirements

### REQ-001: Create resource

The system MUST create the resource.
`

// fixtureInProgressNoRows is In Progress with no verification rows.
const fixtureInProgressNoRows = `# Fixture In Progress Without Rows

**Status:** In Progress

## Overview

Work in flight; no Verification block yet.
`

// fixtureDeferredNoRows is Deferred with no verification rows.
const fixtureDeferredNoRows = `# Fixture Deferred Without Rows

**Status:** Deferred

## Overview

Out of scope for this release.
`

// fixtureAspirationalNoRows is Aspirational with no verification rows.
const fixtureAspirationalNoRows = `# Fixture Aspirational Without Rows

**Status:** Aspirational

## Overview

Metric target, not a claim of current proof.
`

func assertExactlyOneZeroEvidence(t *testing.T, findings []CoverageFinding, path string, status DocStatus) {
	t.Helper()
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1; findings=%+v", len(findings), findings)
	}
	f := findings[0]
	if f.Kind != FindingZeroEvidence {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingZeroEvidence)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if f.Path != path {
		t.Fatalf("Path = %q, want %q", f.Path, path)
	}
	if f.Message == "" {
		t.Fatal("Message must be non-empty")
	}
	if !strings.Contains(f.Message, path) {
		t.Fatalf("Message must name the document path %q; got %q", path, f.Message)
	}
	if !strings.Contains(f.Message, string(status)) {
		t.Fatalf("Message must name status %q; got %q", status, f.Message)
	}
}

// TestCompleteZeroEvidence: Complete with no rows and Implemented with no
// rows each emit exactly one zero-evidence diagnostic naming the document.
func TestCompleteZeroEvidence(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		body   string
		status DocStatus
	}{
		{
			name:   "complete_no_rows",
			path:   "docs/fixtures/complete_zero_evidence.md",
			body:   fixtureCompleteZeroEvidence,
			status: StatusComplete,
		},
		{
			name:   "implemented_no_rows",
			path:   "docs/fixtures/implemented_zero_evidence.md",
			body:   fixtureImplementedZeroEvidence,
			status: StatusImplemented,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse path confirms status + empty rows independently of the pass.
			status := ParseDocumentStatusMarkdown(tc.path, tc.body)
			if status.Status != tc.status {
				t.Fatalf("parsed Status = %q, want %q", status.Status, tc.status)
			}
			model := ParseVerificationMarkdown(tc.path, tc.body)
			if len(model.Rows) != 0 {
				t.Fatalf("fixture must have zero rows; got %d: %+v", len(model.Rows), model.Rows)
			}

			findings := CheckDocumentZeroEvidence(tc.path, tc.body)
			assertExactlyOneZeroEvidence(t, findings, tc.path, tc.status)

			// Direct CheckZeroEvidence path (same contract, no re-parse).
			direct := CheckZeroEvidence(ZeroEvidenceInput{
				Path:       tc.path,
				Status:     status.Status,
				StatusLine: status.Line,
				Rows:       model.Rows,
			})
			assertExactlyOneZeroEvidence(t, direct, tc.path, tc.status)
		})
	}
}

// TestCompleteZeroEvidence_NonEmptyRowsPass: Complete with at least one
// Verification mapping row emits no zero-evidence diagnostic.
func TestCompleteZeroEvidence_NonEmptyRowsPass(t *testing.T) {
	// Inline fixture with a well-formed row.
	path := "docs/fixtures/complete_with_rows.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteWithRows)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteWithRows)
	if len(model.Rows) == 0 {
		t.Fatal("fixture must have at least one Verification mapping row")
	}

	findings := CheckDocumentZeroEvidence(path, fixtureCompleteWithRows)
	if len(findings) != 0 {
		t.Fatalf("expected no zero-evidence findings for non-empty rows; got %+v", findings)
	}

	// On-disk Complete fixture with rows (with_requirement_ids.md) also passes.
	diskPath := filepath.Join("testdata", "docs", "with_requirement_ids.md")
	before := snapshotFixtures(t, filepath.Join("testdata", "docs"))
	data, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("read %s: %v", diskPath, err)
	}
	diskFindings := CheckDocumentZeroEvidence(diskPath, string(data))
	if len(diskFindings) != 0 {
		t.Fatalf("on-disk Complete fixture with rows must pass; got %+v", diskFindings)
	}
	after := snapshotFixtures(t, filepath.Join("testdata", "docs"))
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("zero-evidence pass mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}
}

// TestCompleteZeroEvidence_IgnoresNonCompleteStatus: non-Complete/Implemented
// documents emit no zero-evidence diagnostic even with no verification rows.
func TestCompleteZeroEvidence_IgnoresNonCompleteStatus(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		want DocStatus
	}{
		{"proposed", "docs/fixtures/proposed_no_rows.md", fixtureProposedNoRows, StatusProposed},
		{"in_progress", "docs/fixtures/in_progress_no_rows.md", fixtureInProgressNoRows, StatusInProgress},
		{"deferred", "docs/fixtures/deferred_no_rows.md", fixtureDeferredNoRows, StatusDeferred},
		{"aspirational", "docs/fixtures/aspirational_no_rows.md", fixtureAspirationalNoRows, StatusAspirational},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := ParseDocumentStatusMarkdown(tc.path, tc.body)
			if status.Status != tc.want {
				t.Fatalf("Status = %q, want %q", status.Status, tc.want)
			}
			if IsCompleteStatus(status.Status) {
				t.Fatalf("status %q must not be treated as Complete/Implemented", status.Status)
			}
			model := ParseVerificationMarkdown(tc.path, tc.body)
			if len(model.Rows) != 0 {
				t.Fatalf("fixture must have zero rows; got %d", len(model.Rows))
			}

			findings := CheckDocumentZeroEvidence(tc.path, tc.body)
			if len(findings) != 0 {
				t.Fatalf("non-Complete status %q must emit no zero-evidence diagnostic; got %+v",
					status.Status, findings)
			}
		})
	}
}
