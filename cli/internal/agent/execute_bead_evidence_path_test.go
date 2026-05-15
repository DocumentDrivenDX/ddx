package agent

import (
	"strings"
	"testing"
)

// TestExecuteBeadInstructions_ReportsGoUnderBundlePath covers ddx-d532992f:
// both the claude-variant and agent-variant execute-bead prompts must steer
// investigation/report outputs into the per-attempt evidence directory
// via the bead metadata bundle path under .ddx/executions/
// and explicitly forbid writing them to /tmp. A previous attempt (B15a)
// wrote its report to /tmp/feat-011-status.md, which the post-merge reviewer
// could not see and so flagged BLOCK.
func TestExecuteBeadInstructions_ReportsGoUnderBundlePath(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"claude", executeBeadInstructionsClaudeText},
		{"agent", executeBeadInstructionsAgentText},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(c.text, "metadata `bundle` path") {
				t.Fatalf("%s prompt must reference the bead metadata bundle path for evidence outputs", c.name)
			}
			if !strings.Contains(c.text, ".ddx/executions/") {
				t.Fatalf("%s prompt must name .ddx/executions/ as the evidence root", c.name)
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

// TestBuildPrompt_BundlePathAppearsOnlyInMetadata verifies the prompt keeps
// the initial instructions prefix static and only introduces the concrete
// per-attempt bundle path on the later metadata bundle attribute.
func TestBuildPrompt_BundlePathAppearsOnlyInMetadata(t *testing.T) {
	const attemptID = "20260502T030443-test"
	arts := &executeBeadArtifacts{
		DirRel:      ".ddx/executions/" + attemptID,
		PromptRel:   ".ddx/executions/" + attemptID + "/prompt.md",
		ManifestRel: ".ddx/executions/" + attemptID + "/manifest.json",
		ResultRel:   ".ddx/executions/" + attemptID + "/result.json",
		ChecksRel:   ".ddx/executions/" + attemptID + "/checks.json",
		UsageRel:    ".ddx/executions/" + attemptID + "/usage.json",
	}
	b := representativePromptBead()
	for _, harness := range []string{"claude", "agent"} {
		prompt, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeefdeadbeef", "", harness, "")
		if err != nil {
			t.Fatalf("%s buildPrompt: %v", harness, err)
		}
		rendered := string(prompt)
		if strings.Contains(rendered, "{{.AttemptDir}}") {
			t.Fatalf("%s prompt must not contain the old {{.AttemptDir}} placeholder", harness)
		}
		metadataIdx := strings.Index(rendered, "<metadata ")
		bundleIdx := strings.Index(rendered, arts.DirRel)
		if metadataIdx == -1 || bundleIdx == -1 {
			t.Fatalf("%s prompt missing metadata bundle path", harness)
		}
		if bundleIdx < metadataIdx {
			t.Fatalf("%s prompt leaked bundle path before metadata", harness)
		}
	}
}
