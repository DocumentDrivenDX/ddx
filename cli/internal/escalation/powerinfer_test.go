package escalation

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestExecutionHintParse_UsesEstimatedDifficulty(t *testing.T) {
	b := &bead.Bead{
		Labels: []string{"priority:critical", "kind:bug", "power:cheap"},
		Extra: map[string]any{
			BeadEstimatedDifficultyKey: string(DifficultyHard),
		},
	}

	hint := ParseExecutionHint(b)
	if hint.EstimatedDifficulty != DifficultyHard {
		t.Fatalf("estimated difficulty: want %q, got %q", DifficultyHard, hint.EstimatedDifficulty)
	}
	if hint.InferredPowerClass != PowerSmart {
		t.Fatalf("mapped power class: want %q, got %q", PowerSmart, hint.InferredPowerClass)
	}
	if hint.Source != ExecutionIntentSourceBeadHint {
		t.Fatalf("source: want bead_hint, got %q", hint.Source)
	}
}

func TestExecutionHintParse_IgnoresLegacyPowerLabels(t *testing.T) {
	b := &bead.Bead{Labels: []string{"power:smart"}}
	hint := ParseExecutionHint(b)

	if hint.InferredPowerClass != PowerStandard {
		t.Fatalf("legacy power label must not affect power inference: got %q", hint.InferredPowerClass)
	}
	if hint.Source != ExecutionIntentSourceDefault {
		t.Fatalf("source: want default, got %q", hint.Source)
	}
	if findings := LintExecutionHints(b); len(findings) != 0 {
		t.Fatalf("legacy power label must not produce lint findings, got %+v", findings)
	}
}

func TestResolveExecutionHint_UsesReadinessDifficultyWhenBeadHintAbsent(t *testing.T) {
	hint := ResolveExecutionHint(ExecutionHintInput{
		Bead:                         &bead.Bead{},
		ReadinessEstimatedDifficulty: string(DifficultyHard),
	})

	if hint.Source != ExecutionIntentSourceReadiness {
		t.Fatalf("source: want readiness, got %q", hint.Source)
	}
	if hint.EstimatedDifficulty != DifficultyHard {
		t.Fatalf("estimated difficulty: want %q, got %q", DifficultyHard, hint.EstimatedDifficulty)
	}
	if hint.InferredPowerClass != PowerSmart {
		t.Fatalf("mapped power class: want %q, got %q", PowerSmart, hint.InferredPowerClass)
	}
}

func TestResolveExecutionHint_BeadHintWinsOverReadinessDifficulty(t *testing.T) {
	hint := ResolveExecutionHint(ExecutionHintInput{
		Bead: &bead.Bead{Extra: map[string]any{
			BeadEstimatedDifficultyKey: string(DifficultyEasy),
		}},
		ReadinessEstimatedDifficulty: string(DifficultyHard),
	})

	if hint.Source != ExecutionIntentSourceBeadHint {
		t.Fatalf("source: want bead_hint, got %q", hint.Source)
	}
	if hint.EstimatedDifficulty != DifficultyEasy {
		t.Fatalf("estimated difficulty: want %q, got %q", DifficultyEasy, hint.EstimatedDifficulty)
	}
	if hint.InferredPowerClass != PowerCheap {
		t.Fatalf("mapped power class: want %q, got %q", PowerCheap, hint.InferredPowerClass)
	}
}

func TestResolveExecutionHint_CLIPassthroughAndProjectConfigTakePrecedence(t *testing.T) {
	b := &bead.Bead{Extra: map[string]any{
		BeadEstimatedDifficultyKey: string(DifficultyHard),
	}}

	cliHint := ResolveExecutionHint(ExecutionHintInput{
		Bead:            b,
		ExplicitRouting: true,
		ProjectRouting:  true,
	})
	if cliHint.Source != ExecutionIntentSourceCLIPassthru {
		t.Fatalf("explicit routing source: want cli, got %q", cliHint.Source)
	}
	if cliHint.EstimatedDifficulty != "" || cliHint.InferredPowerClass != "" {
		t.Fatalf("explicit routing must not infer bead difficulty/power: %+v", cliHint)
	}

	projectHint := ResolveExecutionHint(ExecutionHintInput{
		Bead:           b,
		ProjectRouting: true,
	})
	if projectHint.Source != ExecutionIntentSourceProject {
		t.Fatalf("project routing source: want project_config, got %q", projectHint.Source)
	}
	if projectHint.EstimatedDifficulty != "" || projectHint.InferredPowerClass != "" {
		t.Fatalf("project routing must not infer bead difficulty/power: %+v", projectHint)
	}
}

func TestExecutionHintLint_DoesNotRequireSmartJustification(t *testing.T) {
	b := &bead.Bead{
		Extra: map[string]any{
			BeadEstimatedDifficultyKey: string(DifficultyHard),
		},
	}
	if findings := LintExecutionHints(b); len(findings) != 0 {
		t.Fatalf("difficulty=hard must not require SMART JUSTIFICATION, got %+v", findings)
	}
}

func TestExecutionHintLint_RejectsDurableRoutePins(t *testing.T) {
	b := &bead.Bead{
		Labels: []string{"harness:claude", "provider:openai"},
		Extra: map[string]any{
			"execution-model": "gpt-5.5",
			"model-ref":       "claude-opus",
		},
	}

	findings := LintExecutionHints(b)
	if len(findings) != 4 {
		t.Fatalf("expected 4 findings, got %d: %+v", len(findings), findings)
	}

	want := map[string]bool{
		"harness":         false,
		"provider":        false,
		"execution-model": false,
		"model-ref":       false,
	}
	for _, finding := range findings {
		switch finding.Field {
		case "harness", "provider", "execution-model", "model-ref":
			want[finding.Field] = true
		default:
			t.Fatalf("unexpected finding field: %q", finding.Field)
		}
		if finding.Message == "" {
			t.Fatalf("finding must carry a message: %+v", finding)
		}
	}
	for field, seen := range want {
		if !seen {
			t.Fatalf("missing finding for %s", field)
		}
	}
}

func TestInferPowerClass_DefaultsStandard(t *testing.T) {
	cases := []struct {
		name string
		bead *bead.Bead
	}{
		{name: "nil", bead: nil},
		{name: "empty", bead: &bead.Bead{Title: "do thing"}},
		{name: "labels ignored", bead: &bead.Bead{Labels: []string{"priority:critical", "kind:chore", "power:smart"}}},
		{name: "length ignored", bead: &bead.Bead{Description: "long text is not a routing signal"}},
		{name: "legacy power hint ignored", bead: &bead.Bead{Extra: map[string]any{"triage." + "power_hint": "smart"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferPowerClass(tc.bead); got != PowerStandard {
				t.Fatalf("default: want %q, got %q", PowerStandard, got)
			}
		})
	}
}

func TestInferPowerClass_IgnoresLegacyPowerHintsAndHeuristics(t *testing.T) {
	b := &bead.Bead{
		IssueType:   "bug",
		Labels:      []string{"priority:critical", "kind:incident", "power:smart"},
		Description: "large and important",
		Extra: map[string]any{
			"triage." + "power_hint": "smart",
		},
	}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("legacy hints and heuristics must be ignored: want %q, got %q", PowerStandard, got)
	}
}

func TestInferPowerClass_UsesEstimatedDifficulty(t *testing.T) {
	cases := []struct {
		difficulty EstimatedDifficulty
		want       PowerClass
	}{
		{difficulty: DifficultyEasy, want: PowerCheap},
		{difficulty: DifficultyMedium, want: PowerStandard},
		{difficulty: DifficultyHard, want: PowerSmart},
	}
	for _, tc := range cases {
		t.Run(string(tc.difficulty), func(t *testing.T) {
			b := &bead.Bead{
				Labels: []string{"priority:critical", "kind:chore", "power:smart"},
				Extra: map[string]any{
					BeadEstimatedDifficultyKey: string(tc.difficulty),
				},
			}
			if got := InferPowerClass(b); got != tc.want {
				t.Fatalf("estimated difficulty %q: want %q, got %q", tc.difficulty, tc.want, got)
			}
		})
	}
}

func TestInferPowerClass_InvalidEstimatedDifficultyDefaultsStandard(t *testing.T) {
	b := &bead.Bead{
		Extra: map[string]any{
			BeadEstimatedDifficultyKey: "expensive",
		},
	}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("invalid difficulty: want %q, got %q", PowerStandard, got)
	}
}
