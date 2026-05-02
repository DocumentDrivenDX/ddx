package agent

import (
	"strings"
	"testing"
)

// renderInstructionsForGuardrails returns the post-{{.AttemptDir}}-substitution
// instructions string for the given harness selector. Guardrail tests assert
// on this pre-XML-escape form so substring checks against `<bead-id>`,
// `<specific-paths>`, etc. are not distorted by XML entity encoding (the
// rendered prompt encloses instructions in <instructions>...</instructions>
// and so escapes angle brackets and apostrophes). The bead body itself is
// excluded — invariants must come from the static prompt, not the bead.
func renderInstructionsForGuardrails(t *testing.T, harness, contextBudget string) string {
	t.Helper()
	const attemptDir = ".ddx/executions/20260101T000000-guardrails"
	raw := executeBeadInstructionsText(harness)
	return strings.ReplaceAll(raw, "{{.AttemptDir}}", attemptDir)
}

// renderFullPromptForGuardrails returns the byte-for-byte rendered prompt
// (XML-wrapped, attempt-dir-substituted) for the given harness and
// contextBudget. Used by tests that need to verify buildPrompt-level
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
	content, _, err := buildPrompt(t.TempDir(), b, nil, arts, "deadbeefdeadbeef", "", harness, contextBudget)
	if err != nil {
		t.Fatalf("buildPrompt(%s, %q): %v", harness, contextBudget, err)
	}
	return string(content)
}

// TestExecuteBeadInstructionsLoadBearingGuardrails enforces FEAT-022's
// static-prompt minimum-prompt rule: every guardrail listed in the
// file-level comment block above the constants in execute_bead.go MUST
// appear in the rendered prompt for each (harness, variant). This is the
// regression test for the comment-list contract — adding a new guardrail
// requires adding it to BOTH the comment block AND this list.
func TestExecuteBeadInstructionsLoadBearingGuardrails(t *testing.T) {
	// Each entry: a substring (or substring family) that proves the
	// corresponding guardrail survived the rewrite. Substrings are picked
	// to be robust to wording shuffles within the same guardrail.
	type guardrail struct {
		name     string
		any      []string // pass if ANY of these substrings is present
		variants []string // empty = both; "claude" or "agent" = that variant only
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
		{name: "no_red_code", any: []string{"Do not commit red code"}},
		{name: "no_modify_outside_scope", any: []string{"outside the bead's scope", "outside the bead's named scope"}},
		{name: "never_ddx_init", any: []string{"Never run `ddx init`", "never `ddx init`"}},
		{name: "executions_intact", any: []string{".ddx/executions/"}},
		{name: "no_rewrite_claude_md", any: []string{"CLAUDE.md, AGENTS.md"}},
		{name: "bead_overrides_defaults", any: []string{"override CLAUDE.md", "overrides project defaults"}},
		{name: "reports_attempt_dir", any: []string{".ddx/executions/20260101T000000-guardrails/"}},
		{name: "reports_no_tmp", any: []string{"/tmp"}},
		{name: "no_changes_rationale", any: []string{"no_changes_rationale.txt"}},
		{name: "step0_size_check", any: []string{"## Step 0: size check"}},
		{name: "decompose_bead_create", any: []string{"ddx bead create"}},
		{name: "decompose_bead_dep_add", any: []string{"ddx bead dep add"}},
		{name: "decompose_bead_update", any: []string{"ddx bead update"}},
		{name: "review_is_a_gate", any: []string{"review is a gate", "review is a gate, not an escape hatch"}},
		{name: "blocking_review_findings", any: []string{"BLOCKING `<review-findings>`", "BLOCKING <review-findings>"}},
		{name: "no_no_changes_with_blocking", any: []string{"do not declare `no_changes` with blocking findings open"}},
		{name: "stop_after_commit", variants: []string{"agent"}, any: []string{"Stop immediately after the commit"}},
		{name: "agent_use_tools_not_bash", variants: []string{"agent"}, any: []string{"`bash: cat`", "`bash: rg`"}},
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

// TestExecuteBeadInstructionsHarnessSelector covers the routing at
// executeBeadInstructionsText: agent/fiz/embedded route to the Agent
// variant; claude/codex/opencode/unknown/empty route to the Claude
// variant. AC10 forbids any new per-harness branches beyond Claude/Agent.
func TestExecuteBeadInstructionsHarnessSelector(t *testing.T) {
	cases := []struct {
		harness string
		want    string // "agent" or "claude"
	}{
		{"agent", "agent"},
		{"fiz", "agent"},
		{"embedded", "agent"},
		{"AGENT", "agent"},
		{"  Fiz  ", "agent"},

		{"claude", "claude"},
		{"codex", "claude"},
		{"opencode", "claude"},
		{"unknown", "claude"},
		{"", "claude"},
		{"GPT-4", "claude"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.harness+"_to_"+c.want, func(t *testing.T) {
			got := executeBeadInstructionsText(c.harness)
			switch c.want {
			case "agent":
				if got != executeBeadInstructionsAgentText {
					t.Errorf("harness %q: expected Agent variant, got Claude", c.harness)
				}
			case "claude":
				if got != executeBeadInstructionsClaudeText {
					t.Errorf("harness %q: expected Claude variant, got Agent", c.harness)
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
		"no_changes_rationale.txt",
		".ddx/executions/",
		"ddx bead create",
		"ddx bead dep add",
		"ddx bead update",
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
// cheap-tier "No governing references." note).
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
	fullA := renderFullPromptForGuardrails(t, "agent", "")
	if !strings.Contains(fullA, executeBeadMissingGoverningText) {
		t.Fatalf("agent variant full-budget render with empty refs must include executeBeadMissingGoverningText")
	}
}

// TestExecuteBeadInstructionsSizeFloor is AC2: the rendered prompt for the
// representative bead is at least 30% shorter (word count) than the pre-B2
// baseline for both variants. Hard-coded baselines come from the size
// report at HEAD before this bead landed.
func TestExecuteBeadInstructionsSizeFloor(t *testing.T) {
	// Pre-B2 baselines from TestPromptSizeReport on the parent commit
	// (63bb619 — tracked by metadata.base-rev on this bead).
	const (
		baselineClaude = 1048
		baselineAgent  = 968
		floor          = 0.70 // ≥30% shorter ⇒ words ≤ 0.70 * baseline
	)
	cases := []struct {
		variant, harness string
		baseline         int
	}{
		{"claude", "claude", baselineClaude},
		{"agent", "agent", baselineAgent},
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
