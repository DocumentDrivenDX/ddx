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
