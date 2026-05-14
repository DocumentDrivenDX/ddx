package escalation

import (
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestExecutionHintParse_ValidPowerLabels(t *testing.T) {
	cases := []struct {
		label string
		want  PowerClass
	}{
		{"power:cheap", PowerCheap},
		{"power:standard", PowerStandard},
		{"power:smart", PowerSmart},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			b := &bead.Bead{Labels: []string{tc.label}}
			hint := ParseExecutionHint(b)
			if hint.InferredPowerClass != tc.want {
				t.Fatalf("requested power: want %q, got %q", tc.want, hint.InferredPowerClass)
			}
			if hint.Source != ExecutionIntentSourceBeadHint {
				t.Fatalf("source: want bead_hint, got %q", hint.Source)
			}
		})
	}
}

func TestExecutionHintParse_SmartRequiresJustification(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		b := &bead.Bead{Labels: []string{"power:smart"}}
		hint := ParseExecutionHint(b)
		if hint.InferredPowerClass != PowerSmart {
			t.Fatalf("requested power: want smart, got %q", hint.InferredPowerClass)
		}
		if hint.SmartJustification != "" {
			t.Fatalf("smart justification must be empty when absent, got %q", hint.SmartJustification)
		}
		findings := LintExecutionHints(b)
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Message != "bead uses power:smart but has no SMART JUSTIFICATION section" {
			t.Fatalf("unexpected finding: %+v", findings[0])
		}
	})

	t.Run("present", func(t *testing.T) {
		b := &bead.Bead{
			Labels:      []string{"power:smart"},
			Description: "PROBLEM\nhard decision\n\nSMART JUSTIFICATION:\nThis bead decides the durable execution-hint contract.\n",
		}
		hint := ParseExecutionHint(b)
		if hint.SmartJustification == "" {
			t.Fatal("smart justification must be extracted when present")
		}
		if !strings.Contains(hint.SmartJustification, "durable execution-hint contract") {
			t.Fatalf("unexpected smart justification: %q", hint.SmartJustification)
		}
		if findings := LintExecutionHints(b); len(findings) != 0 {
			t.Fatalf("expected no lint findings, got %+v", findings)
		}
	})
}

func TestExecutionHintLint_RejectsDurableRoutePins(t *testing.T) {
	b := &bead.Bead{
		Labels: []string{"harness:claude", "provider:openai"},
		Extra: map[string]any{
			"execution-model": "gpt-5.5",
		},
	}

	findings := LintExecutionHints(b)
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d: %+v", len(findings), findings)
	}

	want := map[string]bool{
		"harness":         false,
		"provider":        false,
		"execution-model": false,
	}
	for _, finding := range findings {
		switch finding.Field {
		case "harness", "provider", "execution-model":
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

func TestInferPowerClass_NilBead(t *testing.T) {
	if got := InferPowerClass(nil); got != PowerCheap {
		t.Fatalf("nil bead: want %q, got %q", PowerCheap, got)
	}
}

func TestInferPowerClass_ExplicitPowerLabel(t *testing.T) {
	cases := []struct {
		label string
		want  PowerClass
	}{
		{"power:smart", PowerSmart},
		{"power:standard", PowerStandard},
		{"power:cheap", PowerCheap},
		{"POWER:SMART", PowerSmart},
	}
	for _, c := range cases {
		b := &bead.Bead{Labels: []string{c.label}}
		if got := InferPowerClass(b); got != c.want {
			t.Errorf("label %q: want %q, got %q", c.label, c.want, got)
		}
	}
}

func TestInferPowerClass_ExplicitPowerLabelOverridesPriority(t *testing.T) {
	// An explicit powerClass wins over priority:critical, which would otherwise
	// route to smart.
	b := &bead.Bead{Labels: []string{"priority:critical", "power:cheap"}}
	if got := InferPowerClass(b); got != PowerCheap {
		t.Fatalf("explicit powerClass should win: got %q", got)
	}
}

func TestInferPowerClass_UsesTriagePowerHint(t *testing.T) {
	b := &bead.Bead{
		Labels: []string{"priority:low", "kind:chore"},
		Extra: map[string]any{
			triagePowerHintKey: string(PowerSmart),
		},
	}
	if got := InferPowerClass(b); got != PowerSmart {
		t.Fatalf("triage powerClass hint: want %q, got %q", PowerSmart, got)
	}
}

func TestInferPowerClass_PriorityCritical(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:critical", "kind:chore"}}
	if got := InferPowerClass(b); got != PowerSmart {
		t.Fatalf("critical priority: want smart, got %q", got)
	}
}

func TestInferPowerClass_PriorityHighBug(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:high", "kind:bug"}}
	if got := InferPowerClass(b); got != PowerSmart {
		t.Fatalf("high-priority bug: want smart, got %q", got)
	}
}

func TestInferPowerClass_PriorityHighEnhancement(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:high", "kind:enhancement"}}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("high-priority enhancement: want standard, got %q", got)
	}
}

func TestInferPowerClass_PriorityLow(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:low", "kind:bug"}}
	if got := InferPowerClass(b); got != PowerCheap {
		t.Fatalf("low priority: want cheap, got %q", got)
	}
}

func TestInferPowerClass_KindBug(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:bug"}}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("bug kind: want standard, got %q", got)
	}
}

func TestInferPowerClass_KindChore(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:chore"}}
	if got := InferPowerClass(b); got != PowerCheap {
		t.Fatalf("chore kind: want cheap, got %q", got)
	}
}

func TestInferPowerClass_KindEnhancement(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:enhancement"}}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("enhancement kind: want standard, got %q", got)
	}
}

func TestInferPowerClass_IssueTypeFallback(t *testing.T) {
	b := &bead.Bead{IssueType: "bug"}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("issue_type bug: want standard, got %q", got)
	}
}

func TestInferPowerClass_ScopeShort(t *testing.T) {
	b := &bead.Bead{Description: "fix typo"}
	if got := InferPowerClass(b); got != PowerCheap {
		t.Fatalf("short scope: want cheap, got %q", got)
	}
}

func TestInferPowerClass_ScopeMedium(t *testing.T) {
	b := &bead.Bead{Description: strings.Repeat("x", 2000)}
	if got := InferPowerClass(b); got != PowerStandard {
		t.Fatalf("medium scope: want standard, got %q", got)
	}
}

func TestInferPowerClass_ScopeLarge(t *testing.T) {
	b := &bead.Bead{Description: strings.Repeat("x", 5000)}
	if got := InferPowerClass(b); got != PowerSmart {
		t.Fatalf("large scope: want smart, got %q", got)
	}
}

func TestInferPowerClass_NoMetadataDefaultsCheap(t *testing.T) {
	b := &bead.Bead{Title: "do thing"}
	if got := InferPowerClass(b); got != PowerCheap {
		t.Fatalf("no metadata: want cheap, got %q", got)
	}
}
