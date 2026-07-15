package escalation

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestInferInitialMinPowerMapsOnlyEstimatedDifficulty(t *testing.T) {
	cases := []struct {
		name string
		bead *bead.Bead
		want int
	}{
		{name: "nil", bead: nil, want: 7},
		{name: "absent", bead: &bead.Bead{}, want: 7},
		{name: "invalid", bead: &bead.Bead{Extra: map[string]any{BeadEstimatedDifficultyKey: "expensive"}}, want: 7},
		{name: "easy", bead: difficultyBead(DifficultyEasy), want: 0},
		{name: "medium", bead: difficultyBead(DifficultyMedium), want: 7},
		{name: "hard", bead: difficultyBead(DifficultyHard), want: 9},
		{
			name: "other hints ignored",
			bead: &bead.Bead{
				IssueType:   "bug",
				Labels:      []string{"priority:critical", "kind:incident", "power:smart"},
				Description: "large and important",
				Extra:       map[string]any{"triage." + "power_hint": "smart"},
			},
			want: 7,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InferInitialMinPower(tc.bead); got != tc.want {
				t.Fatalf("InferInitialMinPower() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestResolveEstimatedDifficultyMapsOnlyMinPower(t *testing.T) {
	for _, tc := range []struct {
		difficulty EstimatedDifficulty
		want       int
	}{
		{DifficultyEasy, 0},
		{DifficultyMedium, 7},
		{DifficultyHard, 9},
		{"invalid", 7},
	} {
		t.Run(string(tc.difficulty), func(t *testing.T) {
			hint := ParseExecutionHint(difficultyBead(tc.difficulty))
			if !hint.HasInferredMinPower || hint.InferredMinPower != tc.want {
				t.Fatalf("hint = %+v, want present MinPower=%d", hint, tc.want)
			}
			if hint.Source != ExecutionIntentSourceBeadHint && tc.difficulty != "invalid" {
				t.Fatalf("source = %q, want bead_hint", hint.Source)
			}
		})
	}
}

func TestResolveExplicitOperatorMinPowerSuppressesDifficultyInference(t *testing.T) {
	hint := ResolveExecutionHint(ExecutionHintInput{
		Bead:             difficultyBead(DifficultyHard),
		ExplicitMinPower: true,
	})
	if hint.HasInferredMinPower || hint.EstimatedDifficulty != "" || hint.Source != ExecutionIntentSourceCLIPassthru {
		t.Fatalf("explicit MinPower must suppress inference: %+v", hint)
	}
}

func TestResolvePublicPolicySuppressesDifficultyInference(t *testing.T) {
	hint := ResolveExecutionHint(ExecutionHintInput{
		Bead:         difficultyBead(DifficultyHard),
		PublicPolicy: " opaque:fizeau-policy ",
	})
	if hint.HasInferredMinPower || hint.EstimatedDifficulty != "" || hint.Source != ExecutionIntentSourceCLIPassthru {
		t.Fatalf("non-empty public policy must suppress inference: %+v", hint)
	}
}

func TestResolveOperatorRouteConstraintsDoNotSuppressDifficultyInference(t *testing.T) {
	// Harness, provider, model, and MaxPower are deliberately absent from the
	// inference input. They remain opaque sticky request constraints.
	hint := ResolveExecutionHint(ExecutionHintInput{Bead: difficultyBead(DifficultyHard)})
	if !hint.HasInferredMinPower || hint.InferredMinPower != 9 {
		t.Fatalf("sticky route constraints must leave inference active: %+v", hint)
	}
}

func TestResolveExecutionHintUsesReadinessAndAuthoredDifficultyWins(t *testing.T) {
	hint := ResolveExecutionHint(ExecutionHintInput{
		Bead:                         difficultyBead(DifficultyEasy),
		ReadinessEstimatedDifficulty: string(DifficultyHard),
	})
	if hint.Source != ExecutionIntentSourceBeadHint || hint.EstimatedDifficulty != DifficultyEasy || hint.InferredMinPower != 0 || !hint.HasInferredMinPower {
		t.Fatalf("authored difficulty must win: %+v", hint)
	}

	hint = ResolveExecutionHint(ExecutionHintInput{
		Bead:                         &bead.Bead{},
		ReadinessEstimatedDifficulty: string(DifficultyHard),
	})
	if hint.Source != ExecutionIntentSourceReadiness || hint.EstimatedDifficulty != DifficultyHard || hint.InferredMinPower != 9 || !hint.HasInferredMinPower {
		t.Fatalf("readiness difficulty must be transient fallback: %+v", hint)
	}
}

func TestExecutionHintParseIgnoresLegacyPowerLabels(t *testing.T) {
	b := &bead.Bead{Labels: []string{"power:smart"}}
	hint := ParseExecutionHint(b)
	if !hint.HasInferredMinPower || hint.InferredMinPower != 7 || hint.Source != ExecutionIntentSourceDefault {
		t.Fatalf("legacy power label affected inference: %+v", hint)
	}
	if findings := LintExecutionHints(b); len(findings) != 0 {
		t.Fatalf("legacy power label must not produce lint findings, got %+v", findings)
	}
}

func TestExecutionHintLintDoesNotRequireSmartJustification(t *testing.T) {
	b := difficultyBead(DifficultyHard)
	if findings := LintExecutionHints(b); len(findings) != 0 {
		t.Fatalf("difficulty=hard must not create concrete-route lint findings: %+v", findings)
	}
}

func TestExecutionHintLintRejectsDurableRoutePins(t *testing.T) {
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
	seen := map[string]bool{}
	for _, finding := range findings {
		seen[finding.Field] = true
		if finding.Message == "" {
			t.Fatalf("finding must carry a message: %+v", finding)
		}
	}
	for _, field := range []string{"harness", "provider", "execution-model", "model-ref"} {
		if !seen[field] {
			t.Fatalf("missing finding for %s", field)
		}
	}
}

func difficultyBead(difficulty EstimatedDifficulty) *bead.Bead {
	return &bead.Bead{Extra: map[string]any{BeadEstimatedDifficultyKey: string(difficulty)}}
}
