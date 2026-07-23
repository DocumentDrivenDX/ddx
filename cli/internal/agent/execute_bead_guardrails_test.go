package agent

import (
	"strings"
	"testing"
)

// renderInstructionsForGuardrails returns the pre-XML-escape, harness-neutral
// instructions string. Guardrail tests assert on this form
// so substring checks against `<bead-id>`, `<specific-paths>`, etc. are not
// distorted by XML entity encoding (the rendered prompt encloses instructions
// in <instructions>...</instructions> and so escapes angle brackets and
// apostrophes). The bead body itself is excluded — invariants must come from
// the static prompt, not the bead.
func renderInstructionsForGuardrails(t *testing.T, _ string, _ string) string {
	t.Helper()
	return executeBeadInstructionsText
}

// renderFullPromptForGuardrails returns the byte-for-byte rendered prompt
// (XML-wrapped, attempt-dir-substituted) for the given contextBudget. The
// harness label is retained only for readable test failures. Used by tests
// that need to verify buildPrompt-level
// behaviour (governing fallback, minimal-budget gating).
func renderFullPromptForGuardrails(t *testing.T, harness, contextBudget string) string {
	t.Helper()
	const attemptID = "20260101T000000-guardrails"
	arts := &executeBeadArtifacts{
		DirRel:      ".ddx/executions/" + attemptID,
		PromptRel:   ".ddx/executions/" + attemptID + "/prompt.md",
		ManifestRel: ".ddx/executions/" + attemptID + "/manifest.json",
		ResultRel:   ".ddx/executions/" + attemptID + "/result.json",
		ChecksRel:   ".ddx/executions/" + attemptID + "/checks.json",
		UsageRel:    ".ddx/executions/" + attemptID + "/usage.json",
	}
	b := representativePromptBead()
	content, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeefdeadbeef", "", contextBudget)
	if err != nil {
		t.Fatalf("buildPrompt(%s, %q): %v", harness, contextBudget, err)
	}
	return string(content)
}

// TestExecuteBeadInstructionsLoadBearingGuardrails enforces FEAT-022's
// static-prompt minimum-prompt rule: every guardrail listed in the
// file-level comment block above the constants in execute_bead.go MUST
// appear in the rendered harness-neutral prompt. This is the
// regression test for the comment-list contract — adding a new guardrail
// requires adding it to BOTH the comment block AND this list.
func TestExecuteBeadInstructionsLoadBearingGuardrails(t *testing.T) {
	// Each entry: a substring (or substring family) that proves the
	// corresponding guardrail survived the rewrite. Substrings are picked
	// to be robust to wording shuffles within the same guardrail.
	type guardrail struct {
		name     string
		any      []string // pass if ANY of these substrings is present
		variants []string // retained for compatibility with the guardrail table
	}
	guardrails := []guardrail{
		{name: "ac_anti_handwave", any: []string{
			"every AC must be provably satisfied",
			"Every AC must be satisfied by code",
		}},
		{name: "read_first", any: []string{"Read first"}},
		{name: "commit_subject_bead_id", any: []string{"[<bead-id>]"}},
		{name: "commit_exactly_once", any: []string{"Commit exactly once", "commit exactly once"}},
		{name: "git_add_specific_paths", any: []string{"git add <specific-paths>"}},
		{name: "no_git_add_dash_a", any: []string{"never `git add -A`", "never git add -A"}},
		{name: "sequential_git_operations", any: []string{"Run git/index mutations sequentially", "Do not use parallel tool calls for"}},
		{name: "no_red_code", any: []string{"Do not commit red code"}},
		{name: "implementation_commit_single_gate", any: []string{"run `git commit` normally", "hook's output/exit status", "single authoritative staged gate"}},
		{name: "no_modify_outside_scope", any: []string{"outside the bead's scope", "outside the bead's named scope"}},
		{name: "never_ddx_init", any: []string{"Never run `ddx init`", "never `ddx init`"}},
		{name: "executions_intact", any: []string{".ddx/executions/"}},
		{name: "no_rewrite_claude_md", any: []string{"CLAUDE.md, AGENTS.md"}},
		{name: "bead_overrides_defaults", any: []string{"override CLAUDE.md", "overrides project defaults"}},
		{name: "reports_bundle_path", any: []string{"metadata `bundle` path"}},
		{name: "reports_no_tmp", any: []string{"/tmp"}},
		{name: "no_changes_rationale", any: []string{"no_changes_rationale.txt"}},
		{name: "step0_size_check", any: []string{"## Step 0: size check"}},
		{name: "step0_large_description_hint", any: []string{"exceeds 8000 bytes", "split-first pass"}},
		{name: "step0_depth_cap", any: []string{"Auto-decomposition is capped at depth 2", "third-level split"}},
		{name: "decompose_bead_create", any: []string{"ddx bead create"}},
		{name: "decompose_bead_dep_add", any: []string{"ddx bead dep add"}},
		{name: "decompose_dep_edges_are_child_specific", any: []string{"legitimate child-to-child or sibling/replacement edges", "never the parent"}},
		{name: "decompose_bead_update", any: []string{"ddx bead update"}},
		{name: "current_bead_lifecycle_orchestrator_owned", any: []string{"Current-bead lifecycle is orchestrator-owned"}},
		{name: "review_is_a_gate", any: []string{"review is a gate", "review is a gate, not an escape hatch"}},
		{name: "blocking_review_findings", any: []string{"BLOCKING `<review-findings>`", "BLOCKING <review-findings>"}},
		{name: "no_no_changes_with_blocking", any: []string{"do not declare `no_changes` with blocking findings open"}},
		{name: "long_running_matrix_plan", any: []string{"Long-running matrix/benchmark beads", "matrix plan", "output paths", "completion criteria"}},
		{name: "long_running_rerun_justification", any: []string{"Do not re-run the same long-running command", "document why prior output is invalid"}},
	}

	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, g := range guardrails {
				if len(g.variants) > 0 && !contains(g.variants, c.variant) {
					continue
				}
				if !anyContains(rendered, g.any) {
					t.Errorf("guardrail %q missing from %s variant; expected one of %v",
						g.name, c.variant, g.any)
				}
			}
		})
	}
}

// TestExecuteBeadInstructionsHarnessNeutral guards against reintroducing
// concrete-harness prompt selection in DDx.
func TestExecuteBeadInstructionsHarnessNeutral(t *testing.T) {
	for _, harness := range []string{"", "claude", "codex", "agent", "fiz", " opaque-harness "} {
		if got := renderInstructionsForGuardrails(t, harness, ""); got != executeBeadInstructionsText {
			t.Errorf("harness %q changed execute-bead instructions", harness)
		}
	}
}

func TestExecuteBeadInstructionsContainsBeadExecutionMode(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, sub := range []string{
				"DDX_MODE=bead_execution",
				"edit code/docs for bead AC",
				"Only broad queue-steward default is overridden",
				"tracker, merge-policy, verification, safety stay active",
			} {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing execution-mode substring %q", c.variant, sub)
				}
			}
		})
	}
}

func TestExecuteBeadInstructionsForbidCurrentBeadLifecycleMutation(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	forbidden := []string{
		"Current-bead lifecycle is orchestrator-owned",
		"ddx bead update <bead-id> --claim",
		"ddx bead update <bead-id> --status <status>",
		"ddx bead update <bead-id> --unclaim",
		"ddx bead close <bead-id>",
	}
	allowed := []string{
		"ddx bead create",
		"parent=<parent-id>",
		"parent -> child",
		"ddx bead dep add",
		"child-to-child or sibling/replacement edges",
		"ddx bead update <parent-id> --notes 'decomposed into <child-ids>'",
	}
	mustNotContain := []string{
		"ddx bead dep add <parent-id> <child-id>",
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, sub := range forbidden {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing lifecycle guardrail substring %q", c.variant, sub)
				}
			}
			for _, sub := range allowed {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing decomposition allowance substring %q", c.variant, sub)
				}
			}
			for _, sub := range mustNotContain {
				if strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt unexpectedly still contains deprecated lifecycle substring %q", c.variant, sub)
				}
			}
		})
	}
}

func TestExecuteBeadPromptSnapshotsUseParentToChildDecompositionEdges(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			if strings.Contains(rendered, "ddx bead dep add <child-id> <parent-id>") {
				t.Fatalf("rendered %s prompt still contains deprecated child-to-parent dep-add instruction", c.variant)
			}
			for _, sub := range []string{
				"parent=<parent-id>",
				"parent -> child",
				"ddx bead update <parent-id> --notes 'decomposed into <child-ids>'",
			} {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing parent-to-child decomposition substring %q", c.variant, sub)
				}
			}
		})
	}
}

func TestExecuteBeadInstructions_SerializesGitOperations(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	required := []string{
		"Run git/index mutations sequentially",
		"don't parallelize `git add`, `git commit`, or staging/commit commands",
		"git add <specific-paths>",
		"Commit exactly once when green",
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, sub := range required {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing git-serialization substring %q", c.variant, sub)
				}
			}
		})
	}
}

func TestExecuteBeadPrompt_UsesImplementationCommitAsSinglePreCommitGate(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	required := []string{
		"run `git commit` normally",
		"hook's output/exit status",
		"single authoritative staged gate",
		"Stage the exact commit set",
		"hook inputs",
		"lefthook run pre-commit",
		"no-staged-files",
	}
	forbidden := []string{
		"rerun it after staging the exact commit set",
		"Rerun lefthook run pre-commit after staging when hooks depend on staged files",
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, sub := range required {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing staged pre-commit substring %q", c.variant, sub)
				}
			}
			for _, sub := range forbidden {
				if strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt unexpectedly still contains deprecated pre-commit substring %q", c.variant, sub)
				}
			}
		})
	}
}

// TestExecuteBeadInstructionsRenderedInvariants is the AC4 substring-invariant
// test: rendered prompts MUST contain the named substrings in every (variant,
// non-minimal contextBudget) pair. These are the operational primitives the
// agent has to know about; if a tightening pass drops one, the bead loop
// silently degrades.
func TestExecuteBeadInstructionsRenderedInvariants(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	required := []string{
		"[<bead-id>]",
		"git add <specific-paths>",
		"git add -A", // appears as part of "never `git add -A`"
		"Run git/index mutations sequentially",
		"run `git commit` normally",
		"hook's output/exit status",
		"lefthook run pre-commit",
		"no_changes_rationale.txt",
		".ddx/executions/",
		"ddx bead create",
		"ddx bead dep add",
		"ddx bead update",
		"Auto-decomposition is capped at depth 2",
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			for _, sub := range required {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing required substring %q", c.variant, sub)
				}
			}
		})
	}
}

// TestExecuteBeadInstructionsMissingGoverningGate covers AC5: non-minimal
// renders include executeBeadMissingGoverningText when refs are empty;
// contextBudget=="minimal" omits governing refs entirely (replaced by the
// cheap-powerClass "No governing references." note).
func TestExecuteBeadInstructionsMissingGoverningGate(t *testing.T) {
	full := renderFullPromptForGuardrails(t, "claude", "")
	if !strings.Contains(full, executeBeadMissingGoverningText) {
		t.Fatalf("full-budget render with empty refs must include executeBeadMissingGoverningText")
	}
	minimal := renderFullPromptForGuardrails(t, "claude", "minimal")
	if strings.Contains(minimal, executeBeadMissingGoverningText) {
		t.Errorf("minimal-budget render must NOT include executeBeadMissingGoverningText")
	}
	if !strings.Contains(minimal, "No governing references") {
		t.Errorf("minimal-budget render must contain its short fallback note")
	}
}

// TestExecuteBeadInstructionsSizeFloor is AC1 for ddx-fcdbc731: the
// harness-neutral prompt must be at least 30% shorter (word count) than the
// pre-tightening baseline. Hard-coded baselines from TestPromptSizeReport
// at HEAD before the shared-block extraction dep landed.
func TestExecuteBeadInstructionsSizeFloor(t *testing.T) {
	// Pre-tightening baselines from TestPromptSizeReport before ddx-fcdbc731.
	// Updated for FEAT-010 (long-running matrix guardrails).
	const (
		baselineClaude = 1105
		floor          = 0.70 // ≥30% reduction; words ≤ 0.70 * baseline
	)
	cases := []struct {
		variant, harness string
		baseline         int
	}{
		{"route-neutral", "", baselineClaude},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderFullPromptForGuardrails(t, c.harness, "")
			words := len(strings.Fields(rendered))
			limit := int(float64(c.baseline) * floor)
			if words > limit {
				t.Errorf("AC2 size floor: %s variant rendered %d words; must be ≤ %d (%.0f%% of baseline %d)",
					c.variant, words, limit, floor*100, c.baseline)
			}
			t.Logf("%s variant rendered %d words (baseline %d, limit %d)",
				c.variant, words, c.baseline, limit)
		})
	}
}

// TestPromptGuardrails_AllPresent is AC2 for ddx-fcdbc731: enumerates the
// 22 load-bearing guardrails from the FEAT-022 comment block above the
// instr* constants and asserts each is present in the rendered full prompt
// for the harness-neutral prompt. Adding or removing a guardrail must update the
// comment block and this test.
func TestPromptGuardrails_AllPresent(t *testing.T) {
	// 21 instruction-level guardrails. Probes are XML-safe (no angle brackets)
	// so they can be checked against the full XML-wrapped rendered prompt.
	instrGuardrails := []struct{ name, probe string }{
		{"ac_checkbox_anti_handwave", "AC must be"},
		{"read_first", "Read first"},
		{"commit_subject_bead_id", "bead-id"},
		{"commit_exactly_once", "Commit exactly once"},
		{"git_add_specific_paths", "specific-paths"},
		{"never_git_add_dash_a", "git add -A"},
		{"sequential_git_operations", "Run git/index mutations sequentially"},
		{"no_red_code", "Do not commit red code"},
		{"implementation_commit_single_gate", "hook&#39;s output/exit status"},
		{"no_modify_outside_scope", "outside the bead"},
		{"never_ddx_init", "ddx init"},
		{"executions_intact", ".ddx/executions/"},
		{"no_rewrite_claude_md", "CLAUDE.md, AGENTS.md"},
		{"bead_overrides_defaults", "override CLAUDE.md"},
		{"reports_not_tmp", "/tmp"},
		{"no_changes_rationale", "no_changes_rationale.txt"},
		{"step0_size_check", "Step 0: size check"},
		{"decompose_recipe", "ddx bead create"},
		{"current_bead_lifecycle_orchestrator_owned", "Current-bead lifecycle is orchestrator-owned"},
		{"review_is_a_gate", "review is a gate"},
		{"no_no_changes_with_blocking", "blocking findings open"},
	}
	// Guardrail 3 (missing-governing fallback) lives in the governing section,
	// not the instructions block; it is checked via the full rendered prompt.
	const missingGoverningProbe = "No governing references"

	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			full := renderFullPromptForGuardrails(t, c.harness, "")
			for _, g := range instrGuardrails {
				if !strings.Contains(full, g.probe) {
					t.Errorf("guardrail %q missing from %s variant (probe: %q)",
						g.name, c.variant, g.probe)
				}
			}
			// Guardrail 3: missing-governing fallback
			if !strings.Contains(full, missingGoverningProbe) {
				t.Errorf("guardrail \"missing_governing_fallback\" missing from %s full-budget prompt",
					c.variant)
			}
		})
	}
}

// TestPromptSelector_CorrectBlock is retained as a regression name for the old
// selector: every former selector input now produces the single neutral block.
func TestPromptSelector_CorrectBlock(t *testing.T) {
	TestExecuteBeadInstructionsHarnessNeutral(t)
}

// TestPromptSelector_MissingGoverning is AC5 for ddx-fcdbc731: when no
// governing refs are provided (refs == nil), buildPrompt must include
// executeBeadMissingGoverningText in the full-budget render and must NOT
// include it in the minimal-budget render.
func TestPromptSelector_MissingGoverning(t *testing.T) {
	for _, harness := range []string{"claude", "agent"} {
		harness := harness
		t.Run(harness, func(t *testing.T) {
			full := renderFullPromptForGuardrails(t, harness, "")
			if !strings.Contains(full, executeBeadMissingGoverningText) {
				t.Errorf("%s full-budget with nil refs must include executeBeadMissingGoverningText", harness)
			}
			minimal := renderFullPromptForGuardrails(t, harness, "minimal")
			if strings.Contains(minimal, executeBeadMissingGoverningText) {
				t.Errorf("%s minimal-budget must NOT include executeBeadMissingGoverningText", harness)
			}
			if !strings.Contains(minimal, "No governing references") {
				t.Errorf("%s minimal-budget must include short fallback note", harness)
			}
		})
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func anyContains(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// TestExecuteBeadPromptRequiresMatrixPlanForLongRunningWork verifies FEAT-010
// guardrail 23: agents must document a matrix plan before running expensive
// long-duration commands (> 60s per variant).
func TestExecuteBeadPromptRequiresMatrixPlanForLongRunningWork(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			requiredSubstrings := []string{
				"Long-running matrix/benchmark beads",
				"matrix plan",
				"output paths",
				"completion criteria",
			}
			for _, sub := range requiredSubstrings {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing matrix-plan substring %q", c.variant, sub)
				}
			}
		})
	}
}

// TestExecuteBeadPromptRequiresRerunJustificationForDuplicateLongCommands
// verifies FEAT-010 guardrail 24: agents must document why a prior long-running
// command's output is invalid before rerunning the same command.
func TestExecuteBeadPromptRequiresRerunJustificationForDuplicateLongCommands(t *testing.T) {
	cases := []struct{ variant, harness string }{
		{"claude", "claude"},
		{"agent", "agent"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.variant, func(t *testing.T) {
			rendered := renderInstructionsForGuardrails(t, c.harness, "")
			requiredSubstrings := []string{
				"Do not re-run the same long-running command",
				"document why prior output is invalid",
				"what changed",
			}
			for _, sub := range requiredSubstrings {
				if !strings.Contains(rendered, sub) {
					t.Errorf("rendered %s prompt missing rerun-justification substring %q", c.variant, sub)
				}
			}
		})
	}
}
