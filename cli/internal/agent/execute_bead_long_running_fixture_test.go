package agent

import (
	"strings"
	"testing"
)

// TestExecuteBeadLongRunningMatrixFixture_NiflheimScenario validates the
// prompt-level guardrails (FEAT-010) with a fixture based on the Niflheim
// incident where a worker repeatedly ran the same writeamp benchmark for
// arch-c-default without advancing through the required matrix.
//
// This test demonstrates that the prompt guardrails require:
// 1. An explicit matrix plan before expensive commands
// 2. Justification for rerunning identical long-running commands
// 3. Early exit via no_changes when a metric is missing after one bounded run
//
// The prompt-level enforcement is sufficient for this phase; orchestrator-level
// duplicate detection is a follow-up optimization. The agent's adherence to the
// guardrails is validated by snapshot tests verifying the prompt includes all
// required guidance.
func TestExecuteBeadLongRunningMatrixFixture_NiflheimScenario(t *testing.T) {
	// Fixture: a bead that requires running writeamp benchmarks with three
	// configurations (default, aggressive, legacy), each with 600-second runtime.
	_ = []string{
		"1. Run benchmarks for arch-c with [default, aggressive, legacy] configs",
		"2. Each config outputs a latency_report.json with p99_latency_ms >= value",
		"3. Compare all three reports and document throughput ranking",
	}

	// Scenario: without the guardrails, an agent would:
	// - Start running arch-c-default (600s)
	// - Get incomplete results and retry (another 600s)
	// - Get incomplete results and retry again (another 600s)
	// - Consume 40+ minutes on duplicates with no matrix progress
	//
	// With the guardrails, an agent must:
	// - Document a matrix plan first (what output, completion criterion per config)
	// - Run arch-c-default once, document output path
	// - If results are incomplete, exit with status:open reason:retryable
	//   rather than silently retrying
	// - Let the orchestrator decide on budget extension

	cases := []struct {
		name     string
		harness  string
		validate func(t *testing.T, prompt string)
	}{
		{
			name:    "claude_variant",
			harness: "claude",
			validate: func(t *testing.T, prompt string) {
				// Verify the Claude variant includes the matrix-plan guardrail
				if !strings.Contains(prompt, "matrix plan") {
					t.Error("Claude variant missing matrix plan requirement")
				}
				if !strings.Contains(prompt, "output paths") {
					t.Error("Claude variant missing output paths guidance")
				}
				// Verify rerun justification requirement
				if !strings.Contains(prompt, "Do not re-run the same long-running command") {
					t.Error("Claude variant missing rerun justification requirement")
				}
				// Verify early exit guidance
				if !strings.Contains(prompt, "status: open") {
					t.Error("Claude variant missing no_changes early exit guidance")
				}
			},
		},
		{
			name:    "agent_variant",
			harness: "agent",
			validate: func(t *testing.T, prompt string) {
				// Verify the Agent variant includes the same guardrails
				if !strings.Contains(prompt, "matrix plan") {
					t.Error("Agent variant missing matrix plan requirement")
				}
				if !strings.Contains(prompt, "Do not re-run the same long-running command") {
					t.Error("Agent variant missing rerun justification requirement")
				}
				if !strings.Contains(prompt, "status: open") {
					t.Error("Agent variant missing early exit guidance")
				}
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			instructions := executeBeadInstructionsText(c.harness)
			c.validate(t, instructions)

			// Additional validation: verify the guardrail is in the load-bearing list
			if !strings.Contains(instructions, "Long-running matrix/benchmark beads") {
				t.Fatalf("Long-running matrix guardrail not found in %s instructions", c.harness)
			}
		})
	}
}

// TestExecuteBeadLongRunningMatrixPromptSnapshotValidation ensures that the
// prompt snapshot for long-running beads includes sufficient guidance to prevent
// the Niflheim duplicate-command scenario. This validates the choice of
// prompt-only enforcement for this phase.
func TestExecuteBeadLongRunningMatrixPromptSnapshotValidation(t *testing.T) {
	const attemptID = "20260520T225406-niflheim-fixture"
	arts := &executeBeadArtifacts{
		DirRel:      ".ddx/executions/" + attemptID,
		PromptRel:   ".ddx/executions/" + attemptID + "/prompt.md",
		ManifestRel: ".ddx/executions/" + attemptID + "/manifest.json",
		ResultRel:   ".ddx/executions/" + attemptID + "/result.json",
		ChecksRel:   ".ddx/executions/" + attemptID + "/checks.json",
		UsageRel:    ".ddx/executions/" + attemptID + "/usage.json",
	}

	// Create a representative benchmark bead
	b := representativePromptBead()
	b.Description = "Benchmark arch-c with default/aggressive/legacy configs. Each variant requires 600s validation."
	b.Acceptance = "1. Document matrix plan: 3 configs, output paths, completion criteria\n" +
		"2. Run arch-c-default; if latency_report.json is incomplete, exit with status:open + retryable reason\n" +
		"3. Only continue to arch-c-aggressive if arch-c-default completed successfully"

	prompt, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeef", "", "claude", "")
	if err != nil {
		t.Fatalf("buildPrompt failed: %v", err)
	}

	promptStr := string(prompt)

	// Validate the prompt includes guardrails for this scenario
	requiredGuidance := []string{
		"Long-running matrix/benchmark beads",
		"matrix plan",
		"output paths",
		"completion criteria",
		"Do not re-run the same long-running command",
		"document why prior output is invalid",
		"status: open",
		"retryable",
	}
	for _, guidance := range requiredGuidance {
		if !strings.Contains(promptStr, guidance) {
			t.Errorf("prompt missing guidance: %q", guidance)
		}
	}
}
