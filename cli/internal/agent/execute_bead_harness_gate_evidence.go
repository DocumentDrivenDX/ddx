package agent

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// attemptIntegrityInputHook, when non-nil, is invoked with the
// AttemptIntegrityInput just before ValidateAttemptIntegrity runs on the live
// execute-bead path. Tests use this to prove harness-session gate evidence is
// wired through; production leaves it nil.
var attemptIntegrityInputHook func(AttemptIntegrityInput)

// harnessGateEvidenceFromToolCalls adapts the harness-session tool_call /
// tool_result stream into PreCommitGateRun rows for ValidateAttemptIntegrity.
//
// It keeps only foreground gate-relevant commands (lefthook / pre-commit and
// implementation-agent git stage/commit commands), preserves chronological
// order so validators can see stage → gate → commit, and copies exit status
// from the tool result (Error field or an explicit exit_code in the payload).
// Background-only invocations are dropped so they never count as acceptance
// evidence.
func harnessGateEvidenceFromToolCalls(calls []ToolCallEntry) []PreCommitGateRun {
	if len(calls) == 0 {
		return nil
	}
	runs := make([]PreCommitGateRun, 0, len(calls))
	for _, call := range calls {
		if isBackgroundOnlyToolCall(call) {
			continue
		}
		cmd := extractHarnessToolCommand(call)
		if cmd == "" || !isGateRelevantHarnessCommand(cmd) {
			continue
		}
		runs = append(runs, PreCommitGateRun{
			Command:  cmd,
			Output:   strings.TrimSpace(call.Output),
			ExitCode: extractHarnessToolExitCode(call),
		})
	}
	if len(runs) == 0 {
		return nil
	}
	return runs
}

// acceptanceRequiresStagedGateEvidence reports whether bead acceptance asks for
// a staged lefthook / pre-commit gate. Matching is case-insensitive and looks
// for the phrases execute-bead prompts and bead AC use.
func acceptanceRequiresStagedGateEvidence(acceptance string) bool {
	lower := strings.ToLower(acceptance)
	return strings.Contains(lower, "lefthook") ||
		strings.Contains(lower, "pre-commit")
}

// beadRequiresStagedGateEvidence is the bead-level form of
// acceptanceRequiresStagedGateEvidence.
func beadRequiresStagedGateEvidence(b *bead.Bead) bool {
	if b == nil {
		return false
	}
	return acceptanceRequiresStagedGateEvidence(b.Acceptance)
}

func extractHarnessToolCommand(call ToolCallEntry) string {
	input := strings.TrimSpace(call.Input)
	if input == "" {
		return ""
	}
	// Prefer structured tool input payloads used by Claude/Codex-style harnesses.
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err == nil {
		for _, key := range []string{"command", "cmd", "script"} {
			if raw, ok := payload[key]; ok {
				if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
					return normalizeHarnessCommand(s)
				}
			}
		}
	}
	// Fall back to the freeform/unwrapped summary forms used in session logs.
	return normalizeHarnessCommand(unwrapToolCommandSummary(input))
}

func normalizeHarnessCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	command = unwrapToolCommandSummary(command)
	command = stripShellLoginWrapper(command)
	return strings.Join(strings.Fields(command), " ")
}

func isGateRelevantHarnessCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "lefthook") || strings.Contains(lower, "pre-commit") {
		return true
	}
	// Implementation-agent git mutations that define the stage → commit window.
	// Walk argv so "git -C dir add path" and "git commit --no-verify" both match
	// without treating unrelated commands that merely contain the word "add".
	fields := strings.Fields(firstShellCommandSegment(lower))
	for i, field := range fields {
		base := field
		if idx := strings.LastIndex(field, "/"); idx >= 0 {
			base = field[idx+1:]
		}
		if base != "git" {
			continue
		}
		for j := i + 1; j < len(fields); j++ {
			arg := fields[j]
			switch arg {
			case "commit", "add", "stage":
				return true
			case "-C", "-c":
				j++ // skip option argument
				continue
			}
			if strings.HasPrefix(arg, "-") {
				continue
			}
			// First non-option subcommand is neither commit/add/stage.
			break
		}
	}
	return false
}

func isBackgroundOnlyToolCall(call ToolCallEntry) bool {
	// Explicit harness markers.
	combined := strings.ToLower(call.Tool + " " + call.Input + " " + call.Output + " " + call.Error)
	if strings.Contains(combined, "background-only") ||
		strings.Contains(combined, `"background":true`) ||
		strings.Contains(combined, `"run_in_background":true`) ||
		strings.Contains(combined, `"block_until_ms":0`) {
		return true
	}
	// Shell backgrounding via trailing & (not part of && / ||).
	cmd := extractHarnessToolCommand(call)
	trimmed := strings.TrimSpace(cmd)
	if strings.HasSuffix(trimmed, " &") || strings.HasSuffix(trimmed, "\t&") {
		return true
	}
	return false
}

var exitCodeRE = regexp.MustCompile(`(?i)\bexit(?:\s+code)?[=:\s]+(-?\d+)\b`)

func extractHarnessToolExitCode(call ToolCallEntry) int {
	// Prefer structured exit_code in the input/output JSON when present.
	for _, raw := range []string{call.Output, call.Input} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
			continue
		}
		for _, key := range []string{"exit_code", "exitCode", "status"} {
			if v, ok := payload[key]; ok {
				switch n := v.(type) {
				case float64:
					return int(n)
				case string:
					if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
						return parsed
					}
				}
			}
		}
	}
	// Common harness error text: "exit 1", "exit code: 1".
	for _, text := range []string{call.Error, call.Output} {
		if m := exitCodeRE.FindStringSubmatch(text); m != nil {
			if parsed, err := strconv.Atoi(m[1]); err == nil {
				return parsed
			}
		}
	}
	if strings.TrimSpace(call.Error) != "" {
		return 1
	}
	return 0
}
