package spechonesty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureCompleteNonAllowlistedCommand is Complete with one allowlisted
// row and one non-allowlisted prose/network command.
const fixtureCompleteNonAllowlistedCommand = `---
ddx:
  id: FIXTURE-COMPLETE-NON-ALLOWLISTED-CMD
---
# Fixture Complete With Non-Allowlisted Command

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-002: Manual claim

The system MUST not accept prose verification.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-002 | TestManualClaim | manual inspection confirmed this works |
`

// fixtureCompleteAllowlistedCommands is Complete with only allowlisted
// executable verification commands (go test with cd prefix, go run).
const fixtureCompleteAllowlistedCommands = `---
ddx:
  id: FIXTURE-COMPLETE-ALLOWLISTED-CMD
---
# Fixture Complete With Allowlisted Commands

**Status:** Complete

## Requirements

### REQ-001: Create resource

The system MUST create the resource.

### REQ-010: Delete resource

The system MUST delete the resource.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestCreateResource | cd cli && go test ./pkg -run TestCreateResource |
| REQ-010 | check:static-delete | go run ./tools/lint/deletecheck |
`

// fixtureImplementedNonAllowlistedCommand is Implemented with a
// non-allowlisted curl command (network-dependent).
const fixtureImplementedNonAllowlistedCommand = `---
ddx:
  id: FIXTURE-IMPLEMENTED-NON-ALLOWLISTED-CMD
---
# Fixture Implemented With Non-Allowlisted Command

**Status:** Implemented

## Requirements

### REQ-001: External probe

The system MUST not rely on network probes for verification.

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestExternalProbe | curl -fsS https://example.com/health |
`

// fixtureProposedNonAllowlistedCommand is not Complete/Implemented.
const fixtureProposedNonAllowlistedCommand = `# Fixture Proposed With Non-Allowlisted Command

**Status:** Proposed

## Verification

| Requirement | Evidence | Command |
|-------------|----------|---------|
| REQ-001 | TestAnything | echo hello from shell |
`

// TestCompleteVerificationRejectsNonAllowlistedCommands: a Complete
// fixture with a Verification row command outside the executable
// verification allowlist emits a non-allowlisted-command diagnostic,
// while a fixture using an allowlisted command passes command validation.
func TestCompleteVerificationRejectsNonAllowlistedCommands(t *testing.T) {
	t.Run("rejects_non_allowlisted", func(t *testing.T) {
		path := "docs/fixtures/complete_non_allowlisted_cmd.md"
		status := ParseDocumentStatusMarkdown(path, fixtureCompleteNonAllowlistedCommand)
		if status.Status != StatusComplete {
			t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
		}
		model := ParseVerificationMarkdown(path, fixtureCompleteNonAllowlistedCommand)
		if len(model.Rows) != 2 {
			t.Fatalf("len(Rows) = %d, want 2; rows=%+v", len(model.Rows), model.Rows)
		}

		findings := CheckDocumentCommandAllowlist(path, fixtureCompleteNonAllowlistedCommand)
		var bad []CoverageFinding
		for _, f := range findings {
			if f.Kind == FindingNonAllowlistedCommand {
				bad = append(bad, f)
			}
		}
		if len(bad) != 1 {
			t.Fatalf("expected exactly one non-allowlisted-command finding; got findings=%+v", findings)
		}
		f := bad[0]
		if f.Severity != SeverityError {
			t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
		}
		if f.Path != path {
			t.Fatalf("Path = %q, want %q", f.Path, path)
		}
		// Diagnostic must name the rejected command.
		if !strings.Contains(f.Message, "manual inspection confirmed this works") {
			t.Fatalf("Message must name the rejected command; got %q", f.Message)
		}
		if !strings.Contains(strings.ToLower(f.Message), "allowlist") {
			t.Fatalf("Message must mention allowlist; got %q", f.Message)
		}
		// Allowlisted row must not produce a diagnostic.
		if strings.Contains(f.Message, "go test") {
			t.Fatalf("allowlisted go test command must not be rejected: %q", f.Message)
		}

		// Direct CheckCommandAllowlist path (same contract, no re-parse).
		direct := CheckCommandAllowlist(CommandAllowlistInput{
			Path:   path,
			Status: status.Status,
			Rows:   model.Rows,
		})
		if len(direct) != 1 || direct[0].Kind != FindingNonAllowlistedCommand {
			t.Fatalf("direct CheckCommandAllowlist = %+v, want one non_allowlisted_command", direct)
		}
	})

	t.Run("allowlisted_commands_pass", func(t *testing.T) {
		path := "docs/fixtures/complete_allowlisted_cmd.md"
		status := ParseDocumentStatusMarkdown(path, fixtureCompleteAllowlistedCommands)
		if status.Status != StatusComplete {
			t.Fatalf("Status = %q, want %q", status.Status, StatusComplete)
		}
		model := ParseVerificationMarkdown(path, fixtureCompleteAllowlistedCommands)
		if len(model.Rows) != 2 {
			t.Fatalf("len(Rows) = %d, want 2; rows=%+v", len(model.Rows), model.Rows)
		}
		for _, row := range model.Rows {
			if !IsAllowlistedVerificationCommand(row.Command) {
				t.Fatalf("fixture command %q must be allowlisted", row.Command)
			}
		}

		findings := CheckDocumentCommandAllowlist(path, fixtureCompleteAllowlistedCommands)
		if len(findings) != 0 {
			t.Fatalf("allowlisted commands must pass validation; got %+v", findings)
		}

		// On-disk Complete fixture with allowlisted go test / go run rows.
		diskPath := filepath.Join("testdata", "docs", "with_requirement_ids.md")
		data, err := os.ReadFile(diskPath)
		if err != nil {
			t.Fatalf("read %s: %v", diskPath, err)
		}
		diskFindings := CheckDocumentCommandAllowlist(diskPath, string(data))
		if len(diskFindings) != 0 {
			t.Fatalf("on-disk Complete fixture commands must be allowlisted; got %+v", diskFindings)
		}
	})

	t.Run("implemented_also_rejects", func(t *testing.T) {
		path := "docs/fixtures/implemented_non_allowlisted_cmd.md"
		status := ParseDocumentStatusMarkdown(path, fixtureImplementedNonAllowlistedCommand)
		if status.Status != StatusImplemented {
			t.Fatalf("Status = %q, want %q", status.Status, StatusImplemented)
		}
		findings := CheckDocumentCommandAllowlist(path, fixtureImplementedNonAllowlistedCommand)
		if len(findings) != 1 {
			t.Fatalf("expected one finding for Implemented non-allowlisted command; got %+v", findings)
		}
		if findings[0].Kind != FindingNonAllowlistedCommand {
			t.Fatalf("Kind = %q, want %q", findings[0].Kind, FindingNonAllowlistedCommand)
		}
		if !strings.Contains(findings[0].Message, "curl") {
			t.Fatalf("Message must name rejected curl command; got %q", findings[0].Message)
		}
	})

	t.Run("ignores_non_complete_status", func(t *testing.T) {
		path := "docs/fixtures/proposed_non_allowlisted_cmd.md"
		status := ParseDocumentStatusMarkdown(path, fixtureProposedNonAllowlistedCommand)
		if status.Status != StatusProposed {
			t.Fatalf("Status = %q, want %q", status.Status, StatusProposed)
		}
		if IsCompleteStatus(status.Status) {
			t.Fatal("Proposed must not be treated as Complete/Implemented")
		}
		findings := CheckDocumentCommandAllowlist(path, fixtureProposedNonAllowlistedCommand)
		if len(findings) != 0 {
			t.Fatalf("non-Complete status must emit no command-allowlist findings; got %+v", findings)
		}
	})
}

// TestCompleteVerificationCommandAllowlistChecks_ReadOnly: running
// command allowlist validation over fixtures does not modify any
// fixture file.
func TestCompleteVerificationCommandAllowlistChecks_ReadOnly(t *testing.T) {
	fixtureRoot := filepath.Join("testdata", "docs")
	before := snapshotFixtures(t, fixtureRoot)
	if len(before) == 0 {
		t.Fatalf("expected markdown fixtures under %s", fixtureRoot)
	}

	// Exercise the pass over every on-disk fixture.
	err := filepath.Walk(fixtureRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		_ = CheckDocumentCommandAllowlist(path, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}

	// Also exercise inline fixtures used by the reject/pass tests.
	_ = CheckDocumentCommandAllowlist(
		"docs/fixtures/complete_non_allowlisted_cmd.md",
		fixtureCompleteNonAllowlistedCommand,
	)
	_ = CheckDocumentCommandAllowlist(
		"docs/fixtures/complete_allowlisted_cmd.md",
		fixtureCompleteAllowlistedCommands,
	)
	_ = CheckDocumentCommandAllowlist(
		"docs/fixtures/implemented_non_allowlisted_cmd.md",
		fixtureImplementedNonAllowlistedCommand,
	)

	// Direct allowlist helper on representative commands (no I/O).
	_ = IsAllowlistedVerificationCommand("cd cli && go test ./pkg -run TestCreateResource")
	_ = IsAllowlistedVerificationCommand("go run ./tools/lint/deletecheck")
	_ = IsAllowlistedVerificationCommand("manual inspection confirmed this works")

	after := snapshotFixtures(t, fixtureRoot)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("command allowlist validation mutated fixtures:\n%s", strings.Join(diffs, "\n"))
	}
}

func TestIsAllowlistedVerificationCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"cd cli && go test ./pkg -run TestCreateResource", true},
		{"go test ./pkg -run TestListResources", true},
		{"go run ./tools/lint/deletecheck", true},
		{"go vet ./internal/bead/...", true},
		{"make test", true},
		{"make test-full", true},
		{"make lint", true},
		{"lefthook run pre-commit", true},
		{"`go test ./...`", true},
		{"manual inspection confirmed this works", false},
		{"curl -fsS https://example.com/health", false},
		{"echo hello", false},
		{"rm -rf /", false},
		{"make clean", false},
		{"", false},
		{"   ", false},
	}
	for _, tc := range cases {
		if got := IsAllowlistedVerificationCommand(tc.cmd); got != tc.want {
			t.Errorf("IsAllowlistedVerificationCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}
