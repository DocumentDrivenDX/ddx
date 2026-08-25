package spechonesty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureExecuteRevision is the injected current repository revision used
// by execution unit tests. Tests never fetch live git state or shell out
// to a real process; a CommandRunner fake stands in for RunShellCommand.
const fixtureExecuteRevision = "exec-fixture-rev-001"

func executeCompleteRow(ref, command string, line int) VerificationRow {
	return VerificationRow{
		RequirementRef: ref,
		EvidenceTarget: "TestExample",
		Command:        command,
		Line:           line,
	}
}

// TestSpecHonestyExecutesAllowlistedCommandWritesObservationReport: an
// allowlisted Verification-row command is executed via the injected
// CommandRunner, and the resulting exit-zero observation is written to
// an on-disk report keyed by document id, requirement, command,
// repository revision, exit code, and observed evidence.
func TestSpecHonestyExecutesAllowlistedCommandWritesObservationReport(t *testing.T) {
	const command = "cd cli && go test ./pkg -run TestCreateResource"
	calls := 0
	runner := func(cmd, workDir string) (int, string, string, error) {
		calls++
		if cmd != command {
			t.Fatalf("unexpected command passed to runner: %q", cmd)
		}
		return 0, "PASS\nok  \tpkg\t0.010s\n", "", nil
	}

	rows := []VerificationRow{executeCompleteRow("REQ-001", command, 30)}
	result := ExecuteVerificationRows(ExecuteVerificationRowsInput{
		DocumentID: "FEAT-777",
		Path:       "docs/helix/01-frame/features/FEAT-777-fixture.md",
		Status:     StatusComplete,
		Rows:       rows,
		Revision:   fixtureExecuteRevision,
		WorkDir:    ".",
		Runner:     runner,
	})

	if calls != 1 {
		t.Fatalf("expected runner to be called exactly once; got %d", calls)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected no findings for a passing allowlisted command; got %+v", result.Findings)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected exactly one report row; got %+v", result.Rows)
	}
	row := result.Rows[0]
	if row.DocumentID != "FEAT-777" {
		t.Fatalf("DocumentID = %q, want FEAT-777", row.DocumentID)
	}
	if row.RequirementRef != "REQ-001" {
		t.Fatalf("RequirementRef = %q, want REQ-001", row.RequirementRef)
	}
	if row.Command != command {
		t.Fatalf("Command = %q, want %q", row.Command, command)
	}
	if row.Revision != fixtureExecuteRevision {
		t.Fatalf("Revision = %q, want %q", row.Revision, fixtureExecuteRevision)
	}
	if row.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", row.ExitCode)
	}
	if !strings.Contains(row.Evidence, "PASS") {
		t.Fatalf("Evidence must capture observed command output; got %q", row.Evidence)
	}

	if len(result.Observations) != 1 {
		t.Fatalf("expected one Observation mirroring the report row; got %+v", result.Observations)
	}
	obs := result.Observations[0]
	if !obs.IsStructured() {
		t.Fatalf("executed observation must be structured: %+v", obs)
	}
	freshness := CheckObservationFreshness(FreshnessInput{
		CurrentRevision: fixtureExecuteRevision,
		Status:          StatusComplete,
		Path:            "docs/helix/01-frame/features/FEAT-777-fixture.md",
		Rows:            rows,
		Observations:    result.Observations,
	})
	if len(freshness) != 0 {
		t.Fatalf("executed observation must satisfy the freshness check; got %+v", freshness)
	}

	// Write and read back the observation report.
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "nested", "observations.json")
	if err := WriteObservationReport(reportPath, result.Rows); err != nil {
		t.Fatalf("WriteObservationReport: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var written []ObservationReportRow
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if len(written) != 1 || written[0] != row {
		t.Fatalf("written report = %+v, want [%+v]", written, row)
	}

	roundTrip, err := ReadObservationReport(reportPath)
	if err != nil {
		t.Fatalf("ReadObservationReport: %v", err)
	}
	if len(roundTrip) != 1 || roundTrip[0] != row {
		t.Fatalf("ReadObservationReport = %+v, want [%+v]", roundTrip, row)
	}
}

// TestSpecHonestyRejectsNonZeroCommand: an allowlisted command that
// exits non-zero is still executed and reported (so the failure itself
// is observable evidence), but it also produces a
// FindingVerificationCommandFailed diagnostic naming the command,
// requirement, and exit code.
func TestSpecHonestyRejectsNonZeroCommand(t *testing.T) {
	const command = "go test ./pkg -run TestFailingCase"
	runner := func(cmd, workDir string) (int, string, string, error) {
		return 1, "", "FAIL\npkg_test.go:10: assertion failed\n", nil
	}

	result := ExecuteVerificationRows(ExecuteVerificationRowsInput{
		DocumentID: "FEAT-778",
		Path:       "docs/helix/01-frame/features/FEAT-778-fixture.md",
		Status:     StatusComplete,
		Rows:       []VerificationRow{executeCompleteRow("REQ-002", command, 40)},
		Revision:   fixtureExecuteRevision,
		WorkDir:    ".",
		Runner:     runner,
	})

	if len(result.Rows) != 1 {
		t.Fatalf("a non-zero exit must still be recorded as an observation row; got %+v", result.Rows)
	}
	if result.Rows[0].ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.Rows[0].ExitCode)
	}
	if !strings.Contains(result.Rows[0].Evidence, "FAIL") {
		t.Fatalf("Evidence must capture the failing command output; got %q", result.Rows[0].Evidence)
	}

	var bad []CoverageFinding
	for _, f := range result.Findings {
		if f.Kind == FindingVerificationCommandFailed {
			bad = append(bad, f)
		}
	}
	if len(bad) != 1 {
		t.Fatalf("expected exactly one verification-command-failed finding; got findings=%+v", result.Findings)
	}
	f := bad[0]
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if !strings.Contains(f.Message, "REQ-002") {
		t.Fatalf("Message must name the failing requirement; got %q", f.Message)
	}
	if !strings.Contains(f.Message, command) {
		t.Fatalf("Message must name the failing command; got %q", f.Message)
	}
	if !strings.Contains(f.Message, "1") {
		t.Fatalf("Message must name the exit code; got %q", f.Message)
	}
}

// TestSpecHonestyRejectsDisallowedCommand: a Verification-row command
// outside the executable verification allowlist is rejected before
// execution — the injected runner is never invoked, and no observation-
// report row is produced — and yields a FindingNonAllowlistedCommand
// diagnostic naming the rejected command.
func TestSpecHonestyRejectsDisallowedCommand(t *testing.T) {
	const command = "curl -fsS https://example.com/health"
	calls := 0
	runner := func(cmd, workDir string) (int, string, string, error) {
		calls++
		return 0, "", "", nil
	}

	result := ExecuteVerificationRows(ExecuteVerificationRowsInput{
		DocumentID: "FEAT-779",
		Path:       "docs/helix/01-frame/features/FEAT-779-fixture.md",
		Status:     StatusComplete,
		Rows:       []VerificationRow{executeCompleteRow("REQ-003", command, 50)},
		Revision:   fixtureExecuteRevision,
		WorkDir:    ".",
		Runner:     runner,
	})

	if calls != 0 {
		t.Fatalf("disallowed command must never be executed; runner called %d times", calls)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("disallowed command must not produce an observation-report row; got %+v", result.Rows)
	}

	var bad []CoverageFinding
	for _, f := range result.Findings {
		if f.Kind == FindingNonAllowlistedCommand {
			bad = append(bad, f)
		}
	}
	if len(bad) != 1 {
		t.Fatalf("expected exactly one non-allowlisted-command finding; got findings=%+v", result.Findings)
	}
	f := bad[0]
	if f.Severity != SeverityError {
		t.Fatalf("Severity = %q, want %q", f.Severity, SeverityError)
	}
	if !strings.Contains(f.Message, command) {
		t.Fatalf("Message must name the rejected command; got %q", f.Message)
	}
}

// TestExecuteVerificationRowsSkipsNonCompleteStatus: non-Complete/
// Implemented statuses never execute anything, matching the sibling
// coverage/observation passes.
func TestExecuteVerificationRowsSkipsNonCompleteStatus(t *testing.T) {
	calls := 0
	runner := func(cmd, workDir string) (int, string, string, error) {
		calls++
		return 0, "", "", nil
	}

	result := ExecuteVerificationRows(ExecuteVerificationRowsInput{
		DocumentID: "FEAT-780",
		Path:       "docs/helix/01-frame/features/FEAT-780-fixture.md",
		Status:     StatusProposed,
		Rows:       []VerificationRow{executeCompleteRow("REQ-004", "go test ./pkg", 10)},
		Revision:   fixtureExecuteRevision,
		WorkDir:    ".",
		Runner:     runner,
	})

	if calls != 0 {
		t.Fatalf("non-Complete status must never execute commands; runner called %d times", calls)
	}
	if len(result.Rows) != 0 || len(result.Findings) != 0 || len(result.Observations) != 0 {
		t.Fatalf("non-Complete status must produce zero-value result; got %+v", result)
	}
}
