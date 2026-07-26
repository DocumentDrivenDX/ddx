package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureCompleteMissingStaticCheck = `---
ddx:
  id: FIXTURE-COMPLETE-MISSING-STATIC-CHECK
---
# Fixture Complete Missing Static Check Evidence

**Status:** Complete

## Requirements

### REQ-010: Static delete guard

The system MUST reject forbidden deletes.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-010 | check:phantom-but-named | go run ./tools/lint/deletecheck |
`

func TestCompleteVerificationResolvesRegisteredStaticChecks(t *testing.T) {
	path := "docs/fixtures/complete_static_check.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteStaticCheck)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteStaticCheck)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}
	if model.Rows[0].EvidenceTarget != "check:static-delete" {
		t.Fatalf("EvidenceTarget = %q, want check:static-delete", model.Rows[0].EvidenceTarget)
	}

	findings := CheckStaticCheckResolution(StaticCheckInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
	})
	if len(findings) != 0 {
		t.Fatalf("registered static check must pass resolution; got %+v", findings)
	}

	docFindings := CheckDocumentStaticChecks(path, fixtureCompleteStaticCheck)
	if len(docFindings) != 0 {
		t.Fatalf("CheckDocumentStaticChecks must pass for registered static check; got %+v", docFindings)
	}
}

func TestCompleteVerificationRejectsMissingStaticChecks(t *testing.T) {
	path := "docs/fixtures/complete_missing_static_check.md"
	status := ParseDocumentStatusMarkdown(path, fixtureCompleteMissingStaticCheck)
	if status.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
	}
	model := ParseVerificationMarkdown(path, fixtureCompleteMissingStaticCheck)
	if len(model.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(model.Rows))
	}

	findings := CheckStaticCheckResolution(StaticCheckInput{
		Path:       path,
		Status:     status.Status,
		StatusLine: status.Line,
		Rows:       model.Rows,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing-static-check finding; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingStaticCheck {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingStaticCheck)
	}
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if !strings.Contains(f.Message, "phantom-but-named") {
		t.Fatalf("Message must name the missing static check; got %q", f.Message)
	}
	if !strings.Contains(strings.ToLower(f.Message), "spechonesty verification model") {
		t.Fatalf("Message must mention the verification model; got %q", f.Message)
	}

	docFindings := CheckDocumentStaticChecks(path, fixtureCompleteMissingStaticCheck)
	if len(docFindings) != 1 || docFindings[0].Kind != FindingMissingStaticCheck {
		t.Fatalf("CheckDocumentStaticChecks = %+v, want one missing-static-check finding", docFindings)
	}
}

func TestCompleteVerificationStaticCheckRegistryIsDeterministic(t *testing.T) {
	wantNames := []string{"static-delete", "static-list", "lockreentry", "spechonesty"}
	model := DefaultStaticCheckModel()
	if len(model.Checks) != len(wantNames) {
		t.Fatalf("DefaultStaticCheckModel().Checks length = %d, want %d", len(model.Checks), len(wantNames))
	}
	for i, want := range wantNames {
		if got := model.Checks[i].Name; got != want {
			t.Fatalf("DefaultStaticCheckModel().Checks[%d].Name = %q, want %q", i, got, want)
		}
	}

	model = StaticCheckModel{
		Checks: []StaticCheckDefinition{
			{Name: "static-delete", Analyzer: "deletecheck", Command: "go run ./tools/lint/deletecheck"},
		},
	}
	path := "docs/fixtures/complete_static_registry.md"
	rows := []VerificationRow{
		{
			RequirementRef: "REQ-001",
			EvidenceTarget: "check:static-delete",
			Command:        "manual inspection confirmed this works",
			Line:           1,
		},
		{
			RequirementRef: "REQ-002",
			EvidenceTarget: "check:phantom-but-named",
			Command:        "go run ./tools/lint/deletecheck",
			Line:           2,
		},
		{
			RequirementRef: "REQ-003",
			EvidenceTarget: "manual inspection of the dashboard",
			Command:        "curl -fsS https://example.com/health",
			Line:           3,
		},
		{
			RequirementRef: "REQ-004",
			EvidenceTarget: "pkg/existing_test.go",
			Command:        "go run ./tools/lint/lockreentrylint/cmd/lockreentrylint",
			Line:           4,
		},
	}

	findings := CheckStaticCheckResolution(StaticCheckInput{
		Path:   path,
		Status: StatusComplete,
		Rows:   rows,
		Model:  &model,
	})
	if len(findings) != 1 {
		t.Fatalf("expected exactly one missing static check; got %+v", findings)
	}
	f := findings[0]
	if f.Kind != FindingMissingStaticCheck {
		t.Fatalf("Kind = %q, want %q", f.Kind, FindingMissingStaticCheck)
	}
	if f.Line != 2 {
		t.Fatalf("Line = %d, want 2", f.Line)
	}
	if !strings.Contains(f.Message, "phantom-but-named") {
		t.Fatalf("Message must name the missing check; got %q", f.Message)
	}
	if strings.Contains(f.Message, "deletecheck") {
		t.Fatalf("Message must not infer the registered analyzer from command text; got %q", f.Message)
	}
	if strings.Contains(f.Message, "lockreentry") {
		t.Fatalf("Message must not infer from unrelated filenames or commands; got %q", f.Message)
	}
}

func TestCompleteVerificationStaticChecks_ReadOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "complete_static_check.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(fixtureCompleteStaticCheck), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	missingPath := filepath.Join(root, "docs", "complete_missing_static_check.md")
	if err := os.WriteFile(missingPath, []byte(fixtureCompleteMissingStaticCheck), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	before := snapshotFixtures(t, root)
	if len(before) == 0 {
		t.Fatalf("expected fixture files under %s", root)
	}

	// Exercise both the content parser and the static-check resolver.
	for _, p := range []string{path, missingPath} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		_ = CheckDocumentStaticChecks(p, string(data))
		_ = CheckStaticCheckResolution(StaticCheckInput{
			Path:   p,
			Status: StatusComplete,
			Rows: []VerificationRow{
				{RequirementRef: "REQ-001", EvidenceTarget: "check:static-delete", Command: "manual inspection", Line: 1},
				{RequirementRef: "REQ-002", EvidenceTarget: "check:phantom-but-named", Command: "go run ./tools/lint/deletecheck", Line: 2},
			},
		})
	}

	after := snapshotFixtures(t, root)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("static-check resolver mutated fixtures:\n%s", joinLines(diffs))
	}
}
