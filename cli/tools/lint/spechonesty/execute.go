// Verification-command execution and observation-report writing
// (phase2-doc-truth-plan WB-1 step 4).
//
// Runs each allowlisted Verification mapping-row command for
// Complete/Implemented documents and emits one machine-readable
// observation-report row per executed command, keyed by document id,
// requirement, command, repository revision, exit code, and observed
// evidence. Commands outside the executable verification allowlist
// (command.go) are rejected without being executed. A non-zero exit is
// still recorded as an observation (the failure must be observable) and
// additionally propagated into spechonesty diagnostics as
// FindingVerificationCommandFailed.
//
// This is the only spechonesty file that shells out; every sibling pass
// (allowlist matching, coverage cardinality, observation-freshness
// validation over already-captured Observation values) remains pure and
// read-only.
package spechonesty

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CommandRunner executes one verification command in workDir and reports
// its exit code and captured stdout/stderr. Production code defaults to
// RunShellCommand; tests inject a fake runner so execution stays
// hermetic and fast.
type CommandRunner func(command, workDir string) (exitCode int, stdout string, stderr string, err error)

// RunShellCommand runs command through `sh -c` in workDir and captures
// stdout/stderr separately. A command that starts and exits non-zero
// reports that exit code with err == nil; err is non-nil only when the
// command could not be started at all (e.g. shell not found).
func RunShellCommand(command, workDir string) (exitCode int, stdout string, stderr string, err error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr == nil {
		return 0, stdout, stderr, nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), stdout, stderr, nil
	}
	return -1, stdout, stderr, runErr
}

// ObservationReportRow is one machine-readable observation-report row
// (phase2-doc-truth-plan WB-1 step 4): document id, requirement,
// command, repository revision, exit code, and observed evidence.
type ObservationReportRow struct {
	DocumentID     string `json:"document_id"`
	RequirementRef string `json:"requirement"`
	Command        string `json:"command"`
	Revision       string `json:"revision"`
	ExitCode       int    `json:"exit_code"`
	Evidence       string `json:"evidence"`
}

// ExecuteVerificationRowsInput is the input to ExecuteVerificationRows.
type ExecuteVerificationRowsInput struct {
	// DocumentID is the canonical document id recorded on each report row.
	DocumentID string
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
	// Revision is the repository revision under evaluation, recorded on
	// every report row and observation.
	Revision string
	// WorkDir is the directory verification commands execute in
	// (typically the repository root).
	WorkDir string
	// Runner executes one command; defaults to RunShellCommand when nil.
	Runner CommandRunner
}

// ExecuteVerificationRowsResult is the output of ExecuteVerificationRows.
type ExecuteVerificationRowsResult struct {
	// Rows are observation-report rows for every executed command
	// (allowlisted rows only; one per Verification mapping row).
	Rows []ObservationReportRow
	// Observations mirrors Rows as Observation values, ready for
	// CheckObservationFreshness without re-executing anything.
	Observations []Observation
	// Findings carries one FindingNonAllowlistedCommand per rejected
	// command (never executed) and one FindingVerificationCommandFailed
	// per executed command with a non-zero exit code.
	Findings []CoverageFinding
}

// ExecuteVerificationRows runs every allowlisted Verification mapping-row
// command for a Complete/Implemented document and returns the resulting
// observation-report rows plus diagnostics.
//
// Rules:
//   - Non-Complete/Implemented statuses → zero-value result; nothing runs.
//   - A row whose command is outside the executable verification
//     allowlist is rejected: one FindingNonAllowlistedCommand, and the
//     command is never executed or reported.
//   - An allowlisted row is executed via Runner (RunShellCommand by
//     default) in WorkDir. Its exit code and captured output become one
//     ObservationReportRow/Observation keyed by DocumentID, requirement,
//     command, and Revision — including a non-zero exit, so the failure
//     itself is observable evidence.
//   - A non-zero exit code additionally produces one
//     FindingVerificationCommandFailed diagnostic naming the command,
//     requirement, and exit code.
func ExecuteVerificationRows(in ExecuteVerificationRowsInput) ExecuteVerificationRowsResult {
	if !IsCompleteStatus(in.Status) {
		return ExecuteVerificationRowsResult{}
	}
	runner := in.Runner
	if runner == nil {
		runner = RunShellCommand
	}

	var result ExecuteVerificationRowsResult
	for _, row := range in.Rows {
		cmd := strings.TrimSpace(row.Command)
		line := row.Line
		if line <= 0 {
			line = 1
		}
		if cmd == "" || !IsAllowlistedVerificationCommand(cmd) {
			result.Findings = append(result.Findings, CoverageFinding{
				Path:     in.Path,
				Line:     line,
				Kind:     FindingNonAllowlistedCommand,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s: Verification command %q is not on the executable verification allowlist",
					in.Path, row.Command,
				),
			})
			continue
		}

		exitCode, stdout, stderr, runErr := runner(cmd, in.WorkDir)
		if runErr != nil {
			exitCode = -1
		}
		evidence := strings.TrimSpace(stdout)
		if evidence == "" {
			evidence = strings.TrimSpace(stderr)
		}
		if runErr != nil && evidence == "" {
			evidence = runErr.Error()
		}

		ref := strings.TrimSpace(row.RequirementRef)
		result.Rows = append(result.Rows, ObservationReportRow{
			DocumentID:     in.DocumentID,
			RequirementRef: ref,
			Command:        cmd,
			Revision:       in.Revision,
			ExitCode:       exitCode,
			Evidence:       evidence,
		})
		result.Observations = append(result.Observations, Observation{
			RequirementRef:  ref,
			Command:         cmd,
			Revision:        in.Revision,
			ExitCode:        exitCode,
			ExitCodePresent: true,
			Evidence:        evidence,
			Path:            in.Path,
			Line:            line,
		})

		if exitCode != 0 {
			result.Findings = append(result.Findings, CoverageFinding{
				Path:     in.Path,
				Line:     line,
				Kind:     FindingVerificationCommandFailed,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s: Verification command %q for requirement %q exited %d",
					in.Path, cmd, ref, exitCode,
				),
			})
		}
	}
	return result
}

// WriteObservationReport serializes rows as indented JSON to path,
// creating parent directories as needed. Rows are written in the order
// supplied; callers control ordering.
func WriteObservationReport(path string, rows []ObservationReportRow) error {
	if rows == nil {
		rows = []ObservationReportRow{}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadObservationReport reads and parses a report previously written by
// WriteObservationReport.
func ReadObservationReport(path string) ([]ObservationReportRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []ObservationReportRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
