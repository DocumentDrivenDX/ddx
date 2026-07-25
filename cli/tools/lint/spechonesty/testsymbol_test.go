package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGoTestFixture builds a minimal repo tree with a package that defines
// the given Test* symbols under pkg/existing_test.go.
func writeGoTestFixture(t *testing.T, root string, symbols ...string) {
	t.Helper()
	dir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var b strings.Builder
	b.WriteString("package pkg\n\nimport \"testing\"\n\n")
	for _, sym := range symbols {
		b.WriteString("func ")
		b.WriteString(sym)
		b.WriteString("(t *testing.T) {}\n")
	}
	path := filepath.Join(dir, "existing_test.go")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

const fixtureCompleteExistingTest = `---
ddx:
  id: FIXTURE-COMPLETE-EXISTING-TEST
---
# Fixture Complete Existing Test Symbol

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
`

const fixtureCompleteMissingTest = `---
ddx:
  id: FIXTURE-COMPLETE-MISSING-TEST
---
# Fixture Complete Missing Test Symbol

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestDoesNotExistAnywhere | cd cli && go test ./pkg -run TestDoesNotExistAnywhere |
`

const fixtureCompleteFileOnlyTest = `---
ddx:
  id: FIXTURE-COMPLETE-FILE-ONLY-TEST
---
# Fixture Complete File-Only Test Evidence

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | pkg/existing_test.go | cd cli && go test ./pkg |
`

const fixtureCompleteScopedExistingTest = `---
ddx:
  id: FIXTURE-COMPLETE-SCOPED-EXISTING-TEST
---
# Fixture Complete Scoped Existing Test Symbol

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | pkg/existing_test.go:TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
`

// TestCompleteVerificationResolvesExistingTestSymbols: a Complete fixture
// mapping a requirement to an existing Test* symbol in the referenced Go
// package/file scope passes target resolution.
func TestCompleteVerificationResolvesExistingTestSymbols(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, "TestCreateResource", "TestListResources")

	path := "docs/fixtures/complete_existing_test.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteExistingTest)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteExistingTest)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1; %+v", len(model.Rows), model.Rows)
	}
	if model.Rows[0].EvidenceTarget != "TestCreateResource" {
		t.Fatalf("EvidenceTarget = %q", model.Rows[0].EvidenceTarget)
	}

	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 0 {
		t.Fatalf("existing Test* symbol must pass resolution; got %+v", findings)
	}

	// File-scoped form also passes when the symbol lives in that file.
	scopedModel := ParseVerificationMarkdown(path, fixtureCompleteScopedExistingTest)
	scopedFindings := CheckTestSymbolResolution(TestSymbolInput{
		Path:     path,
		Status:   StatusComplete,
		Rows:     scopedModel.Rows,
		RepoRoot: root,
	})
	if len(scopedFindings) != 0 {
		t.Fatalf("file-scoped existing Test* must pass; got %+v", scopedFindings)
	}

	// Convenience path.
	docFindings := CheckDocumentTestSymbols(path, fixtureCompleteExistingTest, root)
	if len(docFindings) != 0 {
		t.Fatalf("CheckDocumentTestSymbols must pass for existing symbol; got %+v", docFindings)
	}
}

// TestCompleteVerificationRejectsMissingTestSymbols: a Complete fixture
// mapping a requirement to a nonexistent Test* symbol emits a missing-test
// diagnostic naming the symbol.
func TestCompleteVerificationRejectsMissingTestSymbols(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, "TestCreateResource")

	path := "docs/fixtures/complete_missing_test.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingTest)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingTest)

	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-test finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingTestSymbol {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingTestSymbol)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if f.Symbol != "TestDoesNotExistAnywhere" {
		t.Fatalf("Symbol = %q, want TestDoesNotExistAnywhere", f.Symbol)
	}
	if !strings.Contains(f.Message, "TestDoesNotExistAnywhere") {
		t.Fatalf("Message must name the missing symbol; got %q", f.Message)
	}
	if !strings.Contains(strings.ToLower(f.Message), "missing") {
		t.Fatalf("Message must describe missing test; got %q", f.Message)
	}
	// Existing symbols in the same tree must not be blamed.
	if strings.Contains(f.Message, "TestCreateResource") {
		t.Fatalf("missing-test diagnostic must not blame existing TestCreateResource: %q", f.Message)
	}

	// A real symbol still present in the repo must not produce findings when mapped alone.
	okFindings := CheckTestSymbolResolution(TestSymbolInput{
		Path:   path,
		Status: StatusComplete,
		Rows: []VerificationRow{{
			RequirementRef: "REQ-001",
			EvidenceTarget: "TestCreateResource",
			Command:        "go test ./pkg -run TestCreateResource",
			Line:           20,
		}},
		RepoRoot: root,
	})
	if len(okFindings) != 0 {
		t.Fatalf("existing symbol must not emit missing-test; got %+v", okFindings)
	}
}

// TestCompleteVerificationRejectsFileOnlyTestEvidence: a fixture that cites
// only a Go test file path without the specific Test* target emits a
// diagnostic that file-only evidence is not target coverage.
func TestCompleteVerificationRejectsFileOnlyTestEvidence(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, "TestCreateResource")

	path := "docs/fixtures/complete_file_only_test.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteFileOnlyTest)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteFileOnlyTest)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	if model.Rows[0].EvidenceTarget != "pkg/existing_test.go" {
		t.Fatalf("EvidenceTarget = %q, want pkg/existing_test.go", model.Rows[0].EvidenceTarget)
	}

	findings := CheckTestSymbolResolution(TestSymbolInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
		RepoRoot:   root,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one file-only finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingFileOnlyTestEvidence {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingFileOnlyTestEvidence)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	msgLower := strings.ToLower(f.Message)
	if !strings.Contains(msgLower, "file-only") && !strings.Contains(msgLower, "not target coverage") {
		t.Fatalf("Message must state file-only evidence is not target coverage; got %q", f.Message)
	}
	if !strings.Contains(f.Message, "pkg/existing_test.go") {
		t.Fatalf("Message must cite the file path; got %q", f.Message)
	}

	// Static-check and artifact evidence must not be treated as file-only test evidence.
	other := CheckTestSymbolResolution(TestSymbolInput{
		Path:   path,
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-010", EvidenceTarget: "check:static-delete", Command: "go run ./tools/lint/deletecheck", Line: 1},
			{RequirementRef: "REQ-011", EvidenceTarget: ".ddx/executions/fixture/report.json", Command: "test -f report.json", Line: 2},
		},
		RepoRoot: root,
	})
	if len(other) != 0 {
		t.Fatalf("non-test evidence must be ignored by test-symbol resolver; got %+v", other)
	}
}

// TestCompleteVerificationTestSymbolResolution_ReadOnly: running the Go
// test-symbol resolver over fixtures does not modify any fixture file.
func TestCompleteVerificationTestSymbolResolution_ReadOnly(t *testing.T) {
	root := t.TempDir()
	writeGoTestFixture(t, root, "TestCreateResource")

	// Also copy markdown-style content into the fixture tree so snapshot covers both.
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	docPath := filepath.Join(docsDir, "complete.md")
	if err := os.WriteFile(docPath, []byte(fixtureCompleteExistingTest), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	// Missing + file-only docs for full resolver exercise.
	if err := os.WriteFile(filepath.Join(docsDir, "missing.md"), []byte(fixtureCompleteMissingTest), 0o644); err != nil {
		t.Fatalf("write missing doc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "fileonly.md"), []byte(fixtureCompleteFileOnlyTest), 0o644); err != nil {
		t.Fatalf("write fileonly doc: %v", err)
	}

	before := snapshotFixtures(t, root)
	if len(before) == 0 {
		t.Fatal("expected fixture files in snapshot")
	}

	// Resolve across all three fixture documents.
	for _, name := range []string{"complete.md", "missing.md", "fileonly.md"} {
		p := filepath.Join(docsDir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		_ = CheckDocumentTestSymbols(p, string(data), root)
	}

	// Direct path with mixed rows.
	_ = CheckTestSymbolResolution(TestSymbolInput{
		Path:   "docs/mixed.md",
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: "TestCreateResource", Command: "go test ./pkg -run TestCreateResource", Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: "TestMissing", Command: "go test ./pkg -run TestMissing", Line: 2},
			{RequirementRef: "REQ-003", EvidenceTarget: "pkg/existing_test.go", Command: "go test ./pkg", Line: 3},
			{RequirementRef: "REQ-004", EvidenceTarget: "check:static-delete", Command: "go run ./x", Line: 4},
		},
		RepoRoot: root,
	})

	after := snapshotFixtures(t, root)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("Go test-symbol resolver mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}

	// Package testdata fixtures must also stay untouched.
	tdRoot := filepath.Join("testdata")
	beforeTD := snapshotFixtures(t, tdRoot)
	// Index walk may touch testdata only as read when RepoRoot points there.
	_ = CheckTestSymbolResolution(TestSymbolInput{
		Path:   "testdata/docs/with_requirement_ids.md",
		Status: StatusComplete,
		Rows: []VerificationRow{
			{RequirementRef: "REQ-001", EvidenceTarget: "TestCreateResource", Command: "go test ./pkg -run TestCreateResource", Line: 1},
			{RequirementRef: "REQ-002", EvidenceTarget: "testdata/src/clean/clean.go", Command: "true", Line: 2},
		},
		RepoRoot: tdRoot,
	})
	afterTD := snapshotFixtures(t, tdRoot)
	if diffs := diffFixtures(beforeTD, afterTD); len(diffs) > 0 {
		t.Fatalf("resolver mutated package testdata:\n%s", strings.Join(diffs, "\n"))
	}
}
