package spechonesty

import (
	"path/filepath"
	"testing"
)

// unmetCoverageFailure is the coverage-stage finding this bead
// conditionally suppresses. Coverage resolution is owned by the
// coverage child; tests supply a synthetic unmet-verification failure
// that the coverage child would emit.
func unmetCoverageFailure(path string) CoverageFinding {
	return CoverageFinding{
		Path:     path,
		Line:     1,
		Kind:     FindingUnmetVerification,
		Severity: SeverityError,
		Message:  "unmet verification requirement REQ-001",
	}
}

func TestCompleteRejectsWaiver(t *testing.T) {
	fixtureRoot := filepath.Join("testdata", "docs")
	before := snapshotFixtures(t, fixtureRoot)
	if len(before) == 0 {
		t.Fatalf("expected markdown fixtures under %s", fixtureRoot)
	}

	// Coverage failure that the coverage child would emit for an
	// uncovered / unresolved requirement. This bead must not compute
	// coverage; it only decides whether a reasoned waiver may
	// downgrade that failure.
	baseFindings := []CoverageFinding{unmetCoverageFailure("doc.md")}

	// Complete/Implemented: waiver is ignored; coverage failure remains an error.
	completeStatuses := []struct {
		name   string
		status DocStatus
		file   string
	}{
		{"Complete", StatusComplete, "waiver_complete.md"},
		{"Implemented", StatusImplemented, "waiver_implemented.md"},
	}
	for _, tc := range completeStatuses {
		t.Run("complete_rejects_"+string(tc.status), func(t *testing.T) {
			path := filepath.Join(fixtureRoot, tc.file)
			waiver, err := ParseVerificationWaiverFile(path)
			if err != nil {
				t.Fatalf("ParseVerificationWaiverFile(%s): %v", path, err)
			}
			if !waiver.Present || !waiver.Reasoned {
				t.Fatalf("expected reasoned waiver on %s; got %+v", path, waiver)
			}
			got := ApplyWaiverPolicy(tc.status, waiver, baseFindings)
			if len(got) != 1 {
				t.Fatalf("len(findings) = %d, want 1", len(got))
			}
			if got[0].Severity != SeverityError {
				t.Fatalf("status %s: waiver suppressed coverage failure: severity=%q, want %q",
					tc.status, got[0].Severity, SeverityError)
			}
			if got[0].Kind != FindingUnmetVerification {
				t.Fatalf("kind = %q, want %q", got[0].Kind, FindingUnmetVerification)
			}
		})
	}

	// Non-Complete: reasoned waiver downgrades unmet verification to warning.
	nonCompleteStatuses := []struct {
		status DocStatus
		file   string
	}{
		{StatusProposed, "waiver_proposed.md"},
		{StatusInProgress, "waiver_in_progress.md"},
		{StatusDeferred, "waiver_deferred.md"},
		{StatusAspirational, "waiver_aspirational.md"},
	}
	for _, tc := range nonCompleteStatuses {
		name := string(tc.status)
		if name == "" {
			name = "unknown"
		}
		t.Run("non_complete_downgrades_"+name, func(t *testing.T) {
			path := filepath.Join(fixtureRoot, tc.file)
			waiver, err := ParseVerificationWaiverFile(path)
			if err != nil {
				t.Fatalf("ParseVerificationWaiverFile(%s): %v", path, err)
			}
			if !waiver.Present || !waiver.Reasoned {
				t.Fatalf("expected reasoned waiver on %s; got %+v", path, waiver)
			}
			got := ApplyWaiverPolicy(tc.status, waiver, baseFindings)
			if len(got) != 1 {
				t.Fatalf("len(findings) = %d, want 1", len(got))
			}
			if got[0].Severity != SeverityWarning {
				t.Fatalf("status %s: expected waiver to downgrade unmet requirement to warning; severity=%q",
					tc.status, got[0].Severity)
			}
			if got[0].Kind != FindingUnmetVerification {
				t.Fatalf("kind = %q, want %q", got[0].Kind, FindingUnmetVerification)
			}
		})
	}

	// Non-waivable findings stay errors even with a reasoned non-Complete waiver.
	t.Run("non_waivable_kinds_never_downgraded", func(t *testing.T) {
		path := filepath.Join(fixtureRoot, "waiver_proposed.md")
		waiver, err := ParseVerificationWaiverFile(path)
		if err != nil {
			t.Fatalf("ParseVerificationWaiverFile: %v", err)
		}
		findings := []CoverageFinding{
			{
				Path: path, Line: 1, Kind: FindingMissingStatus,
				Severity: SeverityError, Message: "missing status stamp",
			},
			{
				Path: path, Line: 2, Kind: FindingDuplicateID,
				Severity: SeverityError, Message: "duplicate document id",
			},
			{
				Path: path, Line: 3, Kind: FindingDuplicateUSID,
				Severity: SeverityError, Message: "duplicate US-id",
			},
			unmetCoverageFailure(path),
		}
		got := ApplyWaiverPolicy(StatusProposed, waiver, findings)
		if len(got) != 4 {
			t.Fatalf("len(findings) = %d, want 4", len(got))
		}
		for i, f := range got[:3] {
			if f.Severity != SeverityError {
				t.Fatalf("non-waivable finding[%d] kind=%s severity=%q, want error", i, f.Kind, f.Severity)
			}
		}
		if got[3].Severity != SeverityWarning {
			t.Fatalf("waivable finding severity=%q, want warning", got[3].Severity)
		}
	})

	// Unreasoned (empty) waiver never downgrades, even on non-Complete.
	t.Run("unreasoned_waiver_does_not_downgrade", func(t *testing.T) {
		waiver := &VerificationWaiver{Present: true, Reasoned: false, Reason: ""}
		got := ApplyWaiverPolicy(StatusProposed, waiver, baseFindings)
		if got[0].Severity != SeverityError {
			t.Fatalf("unreasoned waiver downgraded: severity=%q", got[0].Severity)
		}
	})

	// Input slice must not be mutated (pure function).
	t.Run("does_not_mutate_input", func(t *testing.T) {
		in := []CoverageFinding{unmetCoverageFailure("doc.md")}
		_ = ApplyWaiverPolicy(StatusProposed, &VerificationWaiver{
			Present: true, Reasoned: true, Reason: "gap noted",
		}, in)
		if in[0].Severity != SeverityError {
			t.Fatalf("ApplyWaiverPolicy mutated input slice: severity=%q", in[0].Severity)
		}
	})

	// AC2: waiver check is read-only — no fixture file may be modified.
	after := snapshotFixtures(t, fixtureRoot)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("waiver check mutated fixtures:\n%s", joinLines(diffs))
	}
}
