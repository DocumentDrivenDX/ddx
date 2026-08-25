package spechonesty

import (
	"testing"
)

// fixtureCurrentRevision is the injected current repository revision used
// by observation-freshness unit tests. Tests never fetch live git state.
const fixtureCurrentRevision = "fixture-rev-aaa111"

func completeRow(ref string, line int) VerificationRow {
	return VerificationRow{
		RequirementRef: ref,
		EvidenceTarget: "TestExample",
		Command:        "cd cli && go test ./pkg -run TestExample",
		Line:           line,
	}
}

func structuredObs(ref, rev string, exit int) Observation {
	return Observation{
		RequirementRef:  ref,
		Command:         "cd cli && go test ./pkg -run TestExample",
		Revision:        rev,
		ExitCode:        exit,
		ExitCodePresent: true,
		Evidence:        "ok",
	}
}

func TestStaleObservationFails(t *testing.T) {
	const staleRev = "stale-rev-bbb222"
	if staleRev == fixtureCurrentRevision {
		t.Fatal("fixture stale revision must differ from injected current revision")
	}

	findings := CheckObservationFreshness(FreshnessInput{
		CurrentRevision: fixtureCurrentRevision,
		Status:          StatusComplete,
		Path:            "docs/helix/01-frame/features/FEAT-fixture.md",
		Rows:            []VerificationRow{completeRow("REQ-001", 20)},
		Observations: []Observation{
			structuredObs("REQ-001", staleRev, 0),
		},
	})

	if len(findings) == 0 {
		t.Fatal("expected stale observation to fail; got no findings")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingStaleObservation && f.RequirementRef == "REQ-001" {
			found = true
			if f.Severity != SeverityError {
				t.Fatalf("stale finding severity = %q, want %q", f.Severity, SeverityError)
			}
			if f.Message == "" {
				t.Fatal("stale finding missing message")
			}
		}
	}
	if !found {
		t.Fatalf("expected FindingStaleObservation for REQ-001; findings=%+v", findings)
	}
}

func TestCurrentRevisionExitZeroObservationPasses(t *testing.T) {
	findings := CheckObservationFreshness(FreshnessInput{
		CurrentRevision: fixtureCurrentRevision,
		Status:          StatusComplete,
		Path:            "docs/helix/01-frame/features/FEAT-fixture.md",
		Rows:            []VerificationRow{completeRow("REQ-001", 20)},
		Observations: []Observation{
			structuredObs("REQ-001", fixtureCurrentRevision, 0),
		},
	})

	if len(findings) != 0 {
		t.Fatalf("expected current-revision exit-zero observation to pass; findings=%+v", findings)
	}
}

func TestNonZeroCurrentObservationFails(t *testing.T) {
	// Complete path.
	t.Run("Complete", func(t *testing.T) {
		findings := CheckObservationFreshness(FreshnessInput{
			CurrentRevision: fixtureCurrentRevision,
			Status:          StatusComplete,
			Path:            "docs/helix/01-frame/features/FEAT-fixture.md",
			Rows:            []VerificationRow{completeRow("REQ-001", 20)},
			Observations: []Observation{
				structuredObs("REQ-001", fixtureCurrentRevision, 1),
			},
		})
		assertNonZeroFinding(t, findings, "REQ-001")
	})

	// Implemented is treated identically for freshness.
	t.Run("Implemented", func(t *testing.T) {
		findings := CheckObservationFreshness(FreshnessInput{
			CurrentRevision: fixtureCurrentRevision,
			Status:          StatusImplemented,
			Path:            "docs/helix/02-design/technical-designs/TD-fixture.md",
			Rows:            []VerificationRow{completeRow("REQ-010", 42)},
			Observations: []Observation{
				structuredObs("REQ-010", fixtureCurrentRevision, 2),
			},
		})
		assertNonZeroFinding(t, findings, "REQ-010")
	})
}

func assertNonZeroFinding(t *testing.T, findings []ObservationFinding, ref string) {
	t.Helper()
	if len(findings) == 0 {
		t.Fatal("expected non-zero observation to fail; got no findings")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingNonZeroObservation && f.RequirementRef == ref {
			found = true
			if f.Severity != SeverityError {
				t.Fatalf("non-zero finding severity = %q, want %q", f.Severity, SeverityError)
			}
		}
	}
	if !found {
		t.Fatalf("expected FindingNonZeroObservation for %s; findings=%+v", ref, findings)
	}
}

func TestObservationProseAssertionDoesNotPass(t *testing.T) {
	findings := CheckObservationFreshness(FreshnessInput{
		CurrentRevision: fixtureCurrentRevision,
		Status:          StatusComplete,
		Path:            "docs/helix/01-frame/features/FEAT-fixture.md",
		Rows:            []VerificationRow{completeRow("REQ-001", 20)},
		Observations: []Observation{
			{
				RequirementRef: "REQ-001",
				// Prose claim only — no structured revision or exit code.
				Prose: "Verification command `cd cli && go test ./pkg -run TestExample` passed.",
			},
		},
	})

	if len(findings) == 0 {
		t.Fatal("expected prose-only assertion to fail; got no findings")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingUnstructuredObservation && f.RequirementRef == "REQ-001" {
			found = true
			if f.Severity != SeverityError {
				t.Fatalf("prose finding severity = %q, want %q", f.Severity, SeverityError)
			}
			if f.Message == "" {
				t.Fatal("prose finding missing message")
			}
		}
	}
	if !found {
		t.Fatalf("expected FindingUnstructuredObservation for REQ-001; findings=%+v", findings)
	}

	// A structured observation with only prose should not be treated as
	// structured even if Prose also claims success.
	findings = CheckObservationFreshness(FreshnessInput{
		CurrentRevision: fixtureCurrentRevision,
		Status:          StatusComplete,
		Rows:            []VerificationRow{completeRow("REQ-002", 21)},
		Observations: []Observation{
			{
				RequirementRef:  "REQ-002",
				Revision:        "", // missing
				ExitCodePresent: false,
				Prose:           "all checks passed at HEAD",
			},
		},
	})
	if len(findings) == 0 {
		t.Fatal("expected second prose-only case to fail")
	}
	if findings[0].Kind != FindingUnstructuredObservation {
		t.Fatalf("kind = %q, want %q", findings[0].Kind, FindingUnstructuredObservation)
	}
}

// observationReportFixtureContent is a Complete document with one
// Verification mapping row (REQ-001), used by the report-correlation
// acceptance tests below.
const observationReportFixtureContent = "---\n" +
	"ddx:\n" +
	"  id: FEAT-REP1\n" +
	"---\n" +
	"# Fixture Feature: Report Correlation\n\n" +
	"**Status:** Complete\n\n" +
	"## Verification\n\n" +
	"| Requirement | Evidence | Command |\n" +
	"|-------------|----------|---------|\n" +
	"| REQ-001 | TestExample | cd cli && go test ./pkg -run TestExample |\n"

const observationReportFixturePath = "docs/helix/01-frame/features/FEAT-REP1-fixture.md"

// TestSpecHonestyRejectsMissingObservationReport is the acceptance-named
// proof that a Complete document's Verification row fails when the
// persisted observation report has no row at all for that document (e.g.
// `spechonesty observe` was never run, or was run before this document
// existed) — a checked-in mapping row is never sufficient by itself.
func TestSpecHonestyRejectsMissingObservationReport(t *testing.T) {
	findings := CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		nil, // no observation report supplied
		fixtureCurrentRevision,
	)
	if len(findings) == 0 {
		t.Fatal("expected missing-observation-report finding; got none")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingMissingObservation && f.RequirementRef == "REQ-001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FindingMissingObservation for REQ-001; findings=%+v", findings)
	}

	// A report that exists but names a different document must not be
	// mistaken for evidence covering this one.
	otherDocReport := []ObservationReportRow{{
		DocumentID:     "FEAT-OTHER",
		RequirementRef: "REQ-001",
		Command:        "cd cli && go test ./pkg -run TestExample",
		Revision:       fixtureCurrentRevision,
		ExitCode:       0,
		Evidence:       "PASS",
	}}
	findings = CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		otherDocReport,
		fixtureCurrentRevision,
	)
	if len(findings) == 0 {
		t.Fatal("expected missing-observation finding when report only covers a different document")
	}
}

// TestSpecHonestyRejectsStaleRevisionReport is the acceptance-named proof
// that a persisted observation report row recorded at a revision other
// than the current one does not satisfy a Complete document's Verification
// row.
func TestSpecHonestyRejectsStaleRevisionReport(t *testing.T) {
	const staleRev = "stale-rev-report-999"
	if staleRev == fixtureCurrentRevision {
		t.Fatal("fixture stale revision must differ from injected current revision")
	}
	report := []ObservationReportRow{{
		DocumentID:     "FEAT-REP1",
		RequirementRef: "REQ-001",
		Command:        "cd cli && go test ./pkg -run TestExample",
		Revision:       staleRev,
		ExitCode:       0,
		Evidence:       "PASS",
	}}
	findings := CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		report,
		fixtureCurrentRevision,
	)
	if len(findings) == 0 {
		t.Fatal("expected stale-revision-report finding; got none")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingStaleObservation && f.RequirementRef == "REQ-001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FindingStaleObservation for REQ-001; findings=%+v", findings)
	}
}

// TestSpecHonestyRejectsMappingWithoutCurrentRevisionExitZeroEvidence is
// the acceptance-named proof that a Verification mapping row is rejected
// when its report candidates include structured observations, but none of
// them is simultaneously current-revision AND exit-zero — a stale-but-
// passing rerun and a current-but-failing rerun must not combine into a
// pass.
func TestSpecHonestyRejectsMappingWithoutCurrentRevisionExitZeroEvidence(t *testing.T) {
	const staleRev = "stale-rev-report-888"
	report := []ObservationReportRow{
		{
			DocumentID:     "FEAT-REP1",
			RequirementRef: "REQ-001",
			Command:        "cd cli && go test ./pkg -run TestExample",
			Revision:       staleRev,
			ExitCode:       0,
			Evidence:       "PASS (stale rerun)",
		},
		{
			DocumentID:     "FEAT-REP1",
			RequirementRef: "REQ-001",
			Command:        "cd cli && go test ./pkg -run TestExample-retry",
			Revision:       fixtureCurrentRevision,
			ExitCode:       1,
			Evidence:       "FAIL (current rerun)",
		},
	}
	findings := CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		report,
		fixtureCurrentRevision,
	)
	if len(findings) == 0 {
		t.Fatal("expected a finding when no candidate observation is both current-revision and exit-zero; got none")
	}
	for _, f := range findings {
		if f.RequirementRef != "REQ-001" {
			t.Fatalf("unexpected finding for requirement %q; findings=%+v", f.RequirementRef, findings)
		}
		if f.Kind != FindingNonZeroObservation && f.Kind != FindingStaleObservation {
			t.Fatalf("kind = %q, want %q or %q", f.Kind, FindingNonZeroObservation, FindingStaleObservation)
		}
	}
}

// TestSpecHonestyRejectsMissingObservedEvidence is the acceptance-named
// proof that a current-revision, exit-zero report row with no recorded
// observed-evidence text still fails: WB-1 step 4 requires the report to
// capture what was observed, not just that the command exited 0.
func TestSpecHonestyRejectsMissingObservedEvidence(t *testing.T) {
	report := []ObservationReportRow{{
		DocumentID:     "FEAT-REP1",
		RequirementRef: "REQ-001",
		Command:        "cd cli && go test ./pkg -run TestExample",
		Revision:       fixtureCurrentRevision,
		ExitCode:       0,
		Evidence:       "", // missing observed evidence
	}}
	findings := CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		report,
		fixtureCurrentRevision,
	)
	if len(findings) == 0 {
		t.Fatal("expected missing-observed-evidence finding; got none")
	}
	found := false
	for _, f := range findings {
		if f.Kind == FindingMissingObservedEvidence && f.RequirementRef == "REQ-001" {
			found = true
			if f.Severity != SeverityError {
				t.Fatalf("missing-observed-evidence severity = %q, want %q", f.Severity, SeverityError)
			}
		}
	}
	if !found {
		t.Fatalf("expected FindingMissingObservedEvidence for REQ-001; findings=%+v", findings)
	}

	// A row with the same shape but non-empty evidence must pass cleanly.
	report[0].Evidence = "go test output: PASS"
	findings = CheckDocumentObservationReport(
		observationReportFixturePath,
		observationReportFixtureContent,
		report,
		fixtureCurrentRevision,
	)
	if len(findings) != 0 {
		t.Fatalf("expected no findings once observed evidence is present; findings=%+v", findings)
	}
}
