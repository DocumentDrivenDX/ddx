package agent

import (
	"strings"
	"testing"
)

// TestExecuteBeadInstructionsReportsStayLocalAndUncommitted covers ddx-d532992f:
// the harness-neutral execute-bead prompt must steer
// investigation/report outputs into the per-attempt evidence directory
// via the bead metadata bundle path under .ddx/executions/
// and explicitly forbid writing them to /tmp. A previous attempt (B15a)
// wrote its report to /tmp/feat-011-status.md, which the post-merge reviewer
// could not see and so flagged BLOCK.
func TestExecuteBeadInstructionsReportsStayLocalAndUncommitted(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"route-neutral", executeBeadInstructionsText},
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
			if !strings.Contains(c.text, "Keep bundle files local and untracked") && !strings.Contains(c.text, "Bundle files are local-only: never stage or commit them") {
				t.Fatalf("%s prompt must forbid staging or committing bundle reports", c.name)
			}
			if !strings.Contains(c.text, "Commit only requested deliverables outside the bundle") {
				t.Fatalf("%s prompt must limit commits to named deliverables outside the evidence tree", c.name)
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
	prompt, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeefdeadbeef", "", "")
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	rendered := string(prompt)
	if strings.Contains(rendered, "{{.AttemptDir}}") {
		t.Fatal("prompt must not contain the old {{.AttemptDir}} placeholder")
	}
	metadataIdx := strings.Index(rendered, "<metadata ")
	bundleIdx := strings.Index(rendered, arts.DirRel)
	if metadataIdx == -1 || bundleIdx == -1 {
		t.Fatal("prompt missing metadata bundle path")
	}
	if bundleIdx < metadataIdx {
		t.Fatal("prompt leaked bundle path before metadata")
	}
}
