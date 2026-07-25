package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TD-027-style mixed evidence fixture: two real Test* symbols and two
// phantom Test* symbols. Existing coverage must not satisfy missing rows.
const fixtureCompleteMixedEvidence = `---
ddx:
  id: FIXTURE-COMPLETE-MIXED-EVIDENCE
---
# Fixture Complete Mixed Evidence Targets (TD-027 style)

**Status:** Complete

## Requirements

### REQ-001: Bead data model invariants

The bead data model MUST hold its invariants.

### REQ-002: Module boundary

The module MUST not import internal packages outside the bead boundary.

### REQ-003: Operation catalog axon store switch

The operation catalog MUST cover the axon store switch.

### REQ-004: Watcher hub factory

The watcher hub MUST use the provided factory.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestBeadDataModel_InvariantsHold | cd cli && go test ./pkg -run TestBeadDataModel_InvariantsHold |
| REQ-002 | TestModuleBoundary_NoInternalImportsOutsideBead | cd cli && go test ./pkg -run TestModuleBoundary_NoInternalImportsOutsideBead |
| REQ-003 | TestOperationCatalog_AxonStoreSwitchCoverage | cd cli && go test ./pkg -run TestOperationCatalog_AxonStoreSwitchCoverage |
| REQ-004 | TestWatcherHub_UsesProvidedFactory | cd cli && go test ./pkg -run TestWatcherHub_UsesProvidedFactory |
`

// Multi-missing fixture: every Test* target is absent.
const fixtureCompleteAllMissingTests = `---
ddx:
  id: FIXTURE-COMPLETE-ALL-MISSING-TESTS
---
# Fixture Complete All Missing Test Symbols

**Status:** Complete

## Requirements

### REQ-001: Alpha

Alpha MUST work.

### REQ-002: Beta

Beta MUST work.

### REQ-003: Gamma

Gamma MUST work.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestPhantomAlpha | cd cli && go test ./pkg -run TestPhantomAlpha |
| REQ-002 | TestPhantomBeta | cd cli && go test ./pkg -run TestPhantomBeta |
| REQ-003 | TestPhantomGamma | cd cli && go test ./pkg -run TestPhantomGamma |
`

// existingTD027Symbols are the two real tests from the TD-027 note.
var existingTD027Symbols = []string{
	"TestBeadDataModel_InvariantsHold",
	"TestModuleBoundary_NoInternalImportsOutsideBead",
}

// missingTD027Symbols are the two phantom tests from the TD-027 note.
var missingTD027Symbols = []string{
	"TestOperationCatalog_AxonStoreSwitchCoverage",
	"TestWatcherHub_UsesProvidedFactory",
}

// TestCompleteVerificationResolvesMixedEvidenceRows: a TD-027-style fixture
// with two existing and two missing Test* symbols emits failures for the two
// missing symbols only; existing symbols do not suppress or satisfy missing rows.
func TestCompleteVerificationResolvesMixedEvidenceRows(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, existingTD027Symbols...)

	path := "docs/fixtures/complete_mixed_evidence.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMixedEvidence)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMixedEvidence)
	if len(model.Rows) != 4 {
		t.Fatalf("rows = %d, want 4; %+v", len(model.Rows), model.Rows)
	}

	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})

	// Exactly two missing-test findings — one per phantom symbol.
	if len(findings) != 2 {
		t.Fatalf("expected exactly 2 missing-test findings for mixed fixture; got %d: %+v", len(findings), findings)
	}

	foundMissing := map[string]bool{}
	for _, f := range findings {
		if f.Kind != FindingMissingTestSymbol {
			t.Fatalf("Kind = %q, want %q; finding=%+v", f.Kind, FindingMissingTestSymbol, f)
		}
		if f.Severity != SeverityError {
			t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
		}
		if f.Symbol == "" {
			t.Fatalf("missing-test finding must name Symbol: %+v", f)
		}
		foundMissing[f.Symbol] = true
		if !strings.Contains(f.Message, f.Symbol) {
			t.Fatalf("Message must name symbol %q; got %q", f.Symbol, f.Message)
		}
		// Existing symbols must not appear as the blamed missing target.
		for _, existing := range existingTD027Symbols {
			if f.Symbol == existing {
				t.Fatalf("existing symbol %q must not emit missing-test; finding=%+v", existing, f)
			}
			// Message may mention requirement context but must not claim
			// the existing symbol is missing.
			if strings.Contains(f.Message, "missing test symbol \""+existing+"\"") ||
				strings.Contains(f.Message, "missing test symbol '"+existing+"'") {
				t.Fatalf("diagnostic must not claim existing %q is missing: %q", existing, f.Message)
			}
		}
	}
	for _, missing := range missingTD027Symbols {
		if !foundMissing[missing] {
			t.Fatalf("expected missing-test diagnostic for %q; got findings for %v", missing, foundMissing)
		}
	}

	// Existing rows must not be "satisfied by association": re-check that
	// mapping only the existing symbols still produces zero findings while
	// the full mixed set still fails the missing ones.
	existingOnly := CheckTestSymbolResolution(TestSymbolInput{
		Path:   path,
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: existingTD027Symbols[0], Command: "go test ./pkg -run " + existingTD027Symbols[0], Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: existingTD027Symbols[1], Command: "go test ./pkg -run " + existingTD027Symbols[1], Line: 2},
		},
		RepoRoot: root,
	})
	if len(existingOnly) != 0 {
		t.Fatalf("existing-only rows must stay clean; got %+v", existingOnly)
	}

	// Convenience path over the same mixed document.
	docFindings := CheckDocumentTestSymbols(path, fixtureCompleteMixedEvidence, root)
	if len(docFindings) != 2 {
		t.Fatalf("CheckDocumentTestSymbols mixed: want 2 findings, got %d: %+v", len(docFindings), docFindings)
	}
}

// TestCompleteVerificationReportsEachMissingTestRow: a fixture with multiple
// missing Test* targets emits a separate missing-test diagnostic for each
// missing target, naming each symbol.
func TestCompleteVerificationReportsEachMissingTestRow(t *testing.T) {
	root := t.TempDir()
	// Repo has unrelated real symbols — they must not coalesce or suppress.
	writeGoTestFixture(t, root, "TestUnrelatedReal")

	path := "docs/fixtures/complete_all_missing_tests.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteAllMissingTests)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteAllMissingTests)
	if len(model.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(model.Rows))
	}

	wantSymbols := []string{"TestPhantomAlpha", "TestPhantomBeta", "TestPhantomGamma"}
	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != len(wantSymbols) {
		t.Fatalf("expected one diagnostic per missing target (%d); got %d: %+v",
			len(wantSymbols), len(findings), findings)
	}

	bySymbol := map[string]TestSymbolFinding{}
	for _, f := range findings {
		if f.Kind != FindingMissingTestSymbol {
			t.Fatalf("Kind = %q, want %q; finding=%+v", f.Kind, FindingMissingTestSymbol, f)
		}
		if f.Symbol == "" {
			t.Fatalf("each finding must name Symbol: %+v", f)
		}
		if _, dup := bySymbol[f.Symbol]; dup {
			t.Fatalf("duplicate diagnostic for symbol %q (must be independent per row): %+v", f.Symbol, findings)
		}
		bySymbol[f.Symbol] = f
		if !strings.Contains(f.Message, f.Symbol) {
			t.Fatalf("Message must name %q; got %q", f.Symbol, f.Message)
		}
		// Row context: requirement ref should be present when available.
		if f.RequirementRef == "" {
			t.Fatalf("finding for %q should carry RequirementRef for row context", f.Symbol)
		}
		if f.Line <= 0 {
			t.Fatalf("finding for %q should carry row Line > 0", f.Symbol)
		}
	}
	for _, sym := range wantSymbols {
		if _, ok := bySymbol[sym]; !ok {
			t.Fatalf("missing independent diagnostic for %q; have %v", sym, keysOf(bySymbol))
		}
	}
	// Unrelated real symbol must not absorb or replace any missing diagnostic.
	if _, ok := bySymbol["TestUnrelatedReal"]; ok {
		t.Fatalf("must not emit missing-test for existing TestUnrelatedReal")
	}
}

// TestCompleteVerificationKeepsResolvedRowsClean: in a mixed fixture, rows
// mapped to existing Test* symbols do not emit missing-target diagnostics.
func TestCompleteVerificationKeepsResolvedRowsClean(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, existingTD027Symbols...)

	path := "docs/fixtures/complete_mixed_evidence.md"
	model := ParseVerificationMarkdown(path, fixtureCompleteMixedEvidence)

	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:     path,
		Status:   StatusComplete,
		Rows:     model.Rows,
		RepoRoot: root,
	})

	// Collect findings by symbol; none may be an existing (resolved) symbol.
	for _, f := range findings {
		for _, existing := range existingTD027Symbols {
			if f.Symbol == existing {
				t.Fatalf("resolved row for %q must not emit missing-target diagnostic: %+v", existing, f)
			}
			if f.Kind == FindingMissingTestSymbol && f.EvidenceTarget == existing {
				t.Fatalf("resolved EvidenceTarget %q must not appear on missing findings: %+v", existing, f)
			}
		}
	}

	// Explicitly resolve each existing row alone — zero findings each.
	for i, sym := range existingTD027Symbols {
		rowFindings := CheckTestSymbolResolution(TestSymbolInput{
			Path:   path,
			Status: StatusComplete,
			Rows: []VerificationRow{{
				RequirementRef: model.Rows[i].RequirementRef,
				EvidenceTarget: sym,
				Command:        model.Rows[i].Command,
				Line:           model.Rows[i].Line,
			}},
			RepoRoot: root,
		})
		if len(rowFindings) != 0 {
			t.Fatalf("resolved row %q must stay clean; got %+v", sym, rowFindings)
		}
	}

	// Mixed set still reports only missing symbols (cross-check independence).
	missingOnly := 0
	for _, f := range findings {
		if f.Kind == FindingMissingTestSymbol {
			missingOnly++
			ok := false
			for _, m := range missingTD027Symbols {
				if f.Symbol == m {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("unexpected missing symbol %q in mixed findings", f.Symbol)
			}
		}
	}
	if missingOnly != len(missingTD027Symbols) {
		t.Fatalf("want %d missing-symbol findings, got %d (total findings %d)",
			len(missingTD027Symbols), missingOnly, len(findings))
	}
}

// TestCompleteVerificationMixedEvidenceRows_ReadOnly: running mixed-row
// target resolution over fixtures does not modify any fixture file.
func TestCompleteVerificationMixedEvidenceRows_ReadOnly(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, existingTD027Symbols...)

	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	mixedPath := filepath.Join(docsDir, "mixed.md")
	allMissingPath := filepath.Join(docsDir, "all_missing.md")
	if err := os.WriteFile(mixedPath, []byte(fixtureCompleteMixedEvidence), 0o644); err != nil {
		t.Fatalf("write mixed: %v", err)
	}
	if err := os.WriteFile(allMissingPath, []byte(fixtureCompleteAllMissingTests), 0o644); err != nil {
		t.Fatalf("write all_missing: %v", err)
	}

	before := snapshotFixtures(t, root)
	if len(before) == 0 {
		t.Fatal("expected fixture files in snapshot")
	}

	// Exercise mixed-row resolution via document path and direct input.
	for _, p := range []string{mixedPath, allMissingPath} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		_ = CheckDocumentTestSymbols(p, string(data), root)
	}

	model := ParseVerificationMarkdown(mixedPath, fixtureCompleteMixedEvidence)
	_ = CheckTestSymbolResolution(TestSymbolInput{
		Path:     mixedPath,
		Status:   StatusComplete,
		Rows:     model.Rows,
		RepoRoot: root,
	})

	// Also run with rows built in-memory (no document write path).
	_ = CheckTestSymbolResolution(TestSymbolInput{
		Path:   "docs/in_memory_mixed.md",
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: existingTD027Symbols[0], Command: "go test ./pkg -run " + existingTD027Symbols[0], Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: existingTD027Symbols[1], Command: "go test ./pkg -run " + existingTD027Symbols[1], Line: 2},
			{RequirementRef: "REQ-003", EvidenceTarget: missingTD027Symbols[0], Command: "go test ./pkg -run " + missingTD027Symbols[0], Line: 3},
			{RequirementRef: "REQ-004", EvidenceTarget: missingTD027Symbols[1], Command: "go test ./pkg -run " + missingTD027Symbols[1], Line: 4},
		},
		RepoRoot: root,
	})

	after := snapshotFixtures(t, root)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("mixed-row target resolution mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}

	// Package testdata must remain untouched when used as RepoRoot.
	tdRoot := filepath.Join("testdata")
	beforeTD := snapshotFixtures(t, tdRoot)
	_ = CheckTestSymbolResolution(TestSymbolInput{
		Path:   "testdata/docs/with_requirement_ids.md",
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: "TestBeadDataModel_InvariantsHold", Command: "go test ./pkg -run TestBeadDataModel_InvariantsHold", Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: "TestOperationCatalog_AxonStoreSwitchCoverage", Command: "go test ./pkg -run TestOperationCatalog_AxonStoreSwitchCoverage", Line: 2},
		},
		RepoRoot: tdRoot,
	})
	afterTD := snapshotFixtures(t, tdRoot)
	if diffs := diffFixtures(beforeTD, afterTD); len(diffs) > 0 {
		t.Fatalf("mixed-row resolution mutated package testdata:\n%s", strings.Join(diffs, "\n"))
	}
}

func keysOf(m map[string]TestSymbolFinding) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
