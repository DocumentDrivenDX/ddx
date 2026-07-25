// Command allowlist validation for Complete/Implemented Verification
// mapping rows (phase2-doc-truth-plan WB-1 steps 3-4).
//
// Consumes status (ParseDocumentStatus*) and well-formed Verification
// rows (ParseVerification*). Emits one non-allowlisted-command
// diagnostic per mapping-row command outside the executable
// verification allowlist. Analyzer remains read-only: pure over the
// supplied input; never executes commands or fetches network state.
//
// Waivers, inventory/mapping parsing, coverage cardinality, Test*
// resolution, static checks, and runtime artifacts are sibling children.
package spechonesty

import (
	"fmt"
	"regexp"
	"strings"
)

// CommandAllowlistInput is the document-level input for the command
// allowlist pass. Callers supply status from the status parser and rows
// from the Verification mapping parser; this pass does not re-parse
// markdown.
type CommandAllowlistInput struct {
	// Path is the document path recorded on diagnostics.
	Path string
	// Status is the normalized base document status.
	Status DocStatus
	// Rows is the well-formed Verification mapping row set.
	Rows []VerificationRow
}

// allowlistedVerificationCommandRes matches the executable verification
// command forms spechonesty accepts for Complete/Implemented mapping
// rows. Matching is applied after optional `cd <dir> &&` prefixes are
// stripped so fixture forms like `cd cli && go test ./pkg -run TestX`
// and bare `go run ./tools/lint/...` both pass.
//
// The allowlist is intentionally narrow: only local, deterministic
// verification tools that CI can re-run without network access.
var allowlistedVerificationCommandRes = []*regexp.Regexp{
	// go test [flags] [packages]
	regexp.MustCompile(`(?i)^go\s+test\b`),
	// go run <package|file> — static lint tools under tools/lint, etc.
	regexp.MustCompile(`(?i)^go\s+run\b`),
	// go vet [packages]
	regexp.MustCompile(`(?i)^go\s+vet\b`),
	// make test / make lint / make test-full (verification make targets)
	regexp.MustCompile(`(?i)^make\s+(test\b|test-full\b|lint\b)`),
	// lefthook run <hook>
	regexp.MustCompile(`(?i)^lefthook\s+run\b`),
}

// cdPrefixRe matches a leading `cd <path> &&` segment (path without spaces
// or a simple quoted path is not required; fixtures use unquoted paths).
var cdPrefixRe = regexp.MustCompile(`(?i)^cd\s+([^\s&;|]+)\s*&&\s*`)

// IsAllowlistedVerificationCommand reports whether command is on the
// executable verification allowlist accepted by spechonesty.
//
// Empty/whitespace-only commands are not allowlisted (the mapping parser
// already emits missing_command for empty cells; this guard is defensive).
// Matching is read-only and does not execute the command.
func IsAllowlistedVerificationCommand(command string) bool {
	cmd := normalizeVerificationCommand(command)
	if cmd == "" {
		return false
	}
	for _, re := range allowlistedVerificationCommandRes {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

// normalizeVerificationCommand trims whitespace/backticks and strips
// leading `cd <dir> &&` prefixes so the allowlist matches the effective
// executable form.
func normalizeVerificationCommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}
	// Strip a single layer of surrounding backticks (markdown inline code).
	if len(cmd) >= 2 && cmd[0] == '`' && cmd[len(cmd)-1] == '`' {
		cmd = strings.TrimSpace(cmd[1 : len(cmd)-1])
	}
	// Strip chained `cd X &&` prefixes (bounded to avoid pathological input).
	for i := 0; i < 4; i++ {
		loc := cdPrefixRe.FindStringIndex(cmd)
		if loc == nil || loc[0] != 0 {
			break
		}
		cmd = strings.TrimSpace(cmd[loc[1]:])
	}
	return cmd
}

// CheckCommandAllowlist validates Verification mapping-row commands for
// Complete/Implemented documents against the executable verification
// allowlist.
//
// Rules:
//   - Non-Complete/Implemented statuses → no findings.
//   - Empty row set → no findings (zero-evidence sibling owns that case).
//   - Each row whose Command is outside the allowlist → one
//     FindingNonAllowlistedCommand naming the rejected command.
//   - Allowlisted commands → no diagnostic for that row.
//
// Pure and read-only: no filesystem, process, or network access.
func CheckCommandAllowlist(in CommandAllowlistInput) []CoverageFinding {
	if !IsCompleteStatus(in.Status) {
		return nil
	}
	if len(in.Rows) == 0 {
		return nil
	}

	var findings []CoverageFinding
	for _, row := range in.Rows {
		cmd := strings.TrimSpace(row.Command)
		if cmd == "" {
			// Parser should have filtered these; skip defensively.
			continue
		}
		if IsAllowlistedVerificationCommand(cmd) {
			continue
		}
		line := row.Line
		if line <= 0 {
			line = 1
		}
		findings = append(findings, CoverageFinding{
			Path:     in.Path,
			Line:     line,
			Kind:     FindingNonAllowlistedCommand,
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s: Verification command %q is not on the executable verification allowlist",
				in.Path, cmd,
			),
		})
	}
	return findings
}

// CheckDocumentCommandAllowlist parses status and Verification rows for
// content and runs the command allowlist pass. Convenience for fixture
// tests; pure over the supplied content (path is diagnostic metadata
// only). Never modifies files.
func CheckDocumentCommandAllowlist(path, content string) []CoverageFinding {
	status := ParseDocumentStatusMarkdown(path, content)
	model := ParseVerificationMarkdown(path, content)
	return CheckCommandAllowlist(CommandAllowlistInput{
		Path:   path,
		Status: status.Status,
		Rows:   model.Rows,
	})
}
