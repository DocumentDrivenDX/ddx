package agent

import (
	"strings"
	"testing"
)

// TestExecuteBeadInstructions_ReportsGoUnderAttemptDir covers ddx-d532992f:
// both the claude-variant and agent-variant execute-bead prompts must steer
// investigation/report outputs into the per-attempt evidence directory
// (.ddx/executions/<run-id>/, surfaced as the {{.AttemptDir}} template slot)
// and explicitly forbid writing them to /tmp. A previous attempt (B15a)
// wrote its report to /tmp/feat-011-status.md, which the post-merge reviewer
// could not see and so flagged BLOCK.
func TestExecuteBeadInstructions_ReportsGoUnderAttemptDir(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"claude", executeBeadInstructionsClaudeText},
		{"agent", executeBeadInstructionsAgentText},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(c.text, "{{.AttemptDir}}") {
				t.Fatalf("%s prompt must reference {{.AttemptDir}} for evidence outputs", c.name)
			}
			if !strings.Contains(c.text, "/tmp") {
				t.Fatalf("%s prompt must explicitly forbid writing reports to /tmp", c.name)
			}
			lower := strings.ToLower(c.text)
			if !strings.Contains(lower, "report") {
				t.Fatalf("%s prompt must mention report outputs convention", c.name)
			}
		})
	}
}

// TestExecuteBeadInstructions_AttemptDirSubstitutionForReports verifies that
// the {{.AttemptDir}} placeholder used by the new investigation/report
// guidance is rewritten by executeBeadInstructionsText's caller to the
// concrete .ddx/executions/<run-id>/ path, so agents see a real in-repo
// path, not the literal template token.
func TestExecuteBeadInstructions_AttemptDirSubstitutionForReports(t *testing.T) {
	const fakeAttemptDir = ".ddx/executions/20260502T030443-test/"
	for _, harness := range []string{"claude", "agent"} {
		raw := executeBeadInstructionsText(harness)
		if !strings.Contains(raw, "{{.AttemptDir}}") {
			t.Fatalf("%s prompt missing {{.AttemptDir}} placeholder before substitution", harness)
		}
		substituted := strings.ReplaceAll(raw, "{{.AttemptDir}}", fakeAttemptDir)
		if strings.Contains(substituted, "{{.AttemptDir}}") {
			t.Fatalf("%s prompt still contains {{.AttemptDir}} after substitution", harness)
		}
		if !strings.Contains(substituted, fakeAttemptDir) {
			t.Fatalf("%s prompt does not contain substituted attempt dir %q", harness, fakeAttemptDir)
		}
	}
}
