package docprose

import (
	"strings"
	"testing"
)

// initialValeChecks lists every Vale check id shipped in the initial rule
// pack. The normalization layer must translate each of these to a DDx rule
// id.
var initialValeChecks = []struct {
	Check          string
	ExpectedRuleID string
}{
	{"DDx.UnsupportedClaim", "prose.claim.unsupported"},
	{"DDx.AISlop", "prose.ai_slop.polish"},
	{"DDx.FillerTransition", "prose.filler.transition"},
	{"DDx.MissingActorAction", "prose.specificity.actor_action"},
	{"DDx.TokenCost", "prose.cost.filler"},
	{"DDx.RepeatedOpening", "prose.structure.repeated_opening"},
	{"DDx.Vocabulary", "prose.vocabulary.generic_substitute"},
}

func TestDocProseNormalize_MapsValeChecksToDDxRuleIDs(t *testing.T) {
	for _, tc := range initialValeChecks {
		tc := tc
		t.Run(tc.Check, func(t *testing.T) {
			alerts := []ValeAlert{{
				File:     "docs/example.md",
				Check:    tc.Check,
				Line:     3,
				Span:     [2]int{1, 5},
				Severity: "warning",
				Message:  "vale-specific message",
				Match:    "match",
			}}
			findings := NormalizeValeAlerts(alerts)
			if len(findings) != 1 {
				t.Fatalf("expected 1 finding for %s, got %+v", tc.Check, findings)
			}
			if findings[0].RuleID != tc.ExpectedRuleID {
				t.Fatalf("check %s: rule id = %q, want %q", tc.Check, findings[0].RuleID, tc.ExpectedRuleID)
			}
		})
	}
}

func TestDocProseNormalize_AddsRationaleAndSuggestedEdit(t *testing.T) {
	for _, tc := range initialValeChecks {
		tc := tc
		t.Run(tc.Check, func(t *testing.T) {
			alerts := []ValeAlert{{
				File:     "docs/example.md",
				Check:    tc.Check,
				Line:     5,
				Span:     [2]int{1, 5},
				Severity: "warning",
				Message:  "vale message that must not leak",
				Match:    "match",
			}}
			findings := NormalizeValeAlerts(alerts)
			if len(findings) != 1 {
				t.Fatalf("expected 1 finding, got %+v", findings)
			}
			f := findings[0]
			if strings.TrimSpace(f.Rationale) == "" {
				t.Fatalf("missing DDx rationale for %s: %+v", tc.Check, f)
			}
			if strings.TrimSpace(f.SuggestedEdit) == "" {
				t.Fatalf("missing DDx suggested edit for %s: %+v", tc.Check, f)
			}
			if f.Rationale == "vale message that must not leak" {
				t.Fatalf("rationale is the raw Vale message for %s: %+v", tc.Check, f)
			}
			if f.SuggestedEdit == "vale message that must not leak" {
				t.Fatalf("suggested edit is the raw Vale message for %s: %+v", tc.Check, f)
			}
		})
	}
}

func TestDocProseNormalize_MergesSameLineWordHits(t *testing.T) {
	// Two Vale word-level alerts on the same line, same DDx rule, must
	// collapse into one DDx finding because the sentence has one underlying
	// problem.
	alerts := []ValeAlert{
		{
			File:     "docs/example.md",
			Check:    "DDx.UnsupportedClaim",
			Line:     7,
			Span:     [2]int{1, 7},
			Severity: "warning",
			Message:  "raw message",
			Match:    "robust",
		},
		{
			File:     "docs/example.md",
			Check:    "DDx.UnsupportedClaim",
			Line:     7,
			Span:     [2]int{10, 16},
			Severity: "warning",
			Message:  "raw message",
			Match:    "seamless",
		},
	}
	findings := NormalizeValeAlerts(alerts)
	if len(findings) != 1 {
		t.Fatalf("expected 1 merged finding, got %d: %+v", len(findings), findings)
	}
	if findings[0].Line != 7 {
		t.Fatalf("merged finding line = %d, want 7", findings[0].Line)
	}
	if findings[0].RuleID != "prose.claim.unsupported" {
		t.Fatalf("merged finding rule id = %q, want prose.claim.unsupported", findings[0].RuleID)
	}

	// Different DDx rules on the same line must stay separate.
	mixed := []ValeAlert{
		{
			File:     "docs/example.md",
			Check:    "DDx.UnsupportedClaim",
			Line:     9,
			Span:     [2]int{1, 7},
			Severity: "warning",
			Message:  "raw",
			Match:    "robust",
		},
		{
			File:     "docs/example.md",
			Check:    "DDx.TokenCost",
			Line:     9,
			Span:     [2]int{8, 22},
			Severity: "warning",
			Message:  "raw",
			Match:    "very important",
		},
	}
	mixedFindings := NormalizeValeAlerts(mixed)
	if len(mixedFindings) != 2 {
		t.Fatalf("expected 2 findings for distinct rules on same line, got %d: %+v", len(mixedFindings), mixedFindings)
	}

	// Different lines, same rule must stay separate.
	multiLine := []ValeAlert{
		{
			File:     "docs/example.md",
			Check:    "DDx.UnsupportedClaim",
			Line:     11,
			Span:     [2]int{1, 7},
			Severity: "warning",
			Message:  "raw",
			Match:    "robust",
		},
		{
			File:     "docs/example.md",
			Check:    "DDx.UnsupportedClaim",
			Line:     12,
			Span:     [2]int{1, 7},
			Severity: "warning",
			Message:  "raw",
			Match:    "robust",
		},
	}
	multiLineFindings := NormalizeValeAlerts(multiLine)
	if len(multiLineFindings) != 2 {
		t.Fatalf("expected 2 findings across distinct lines, got %d: %+v", len(multiLineFindings), multiLineFindings)
	}
}

func TestDocProseNormalize_DoesNotExposeRawValeNames(t *testing.T) {
	var alerts []ValeAlert
	for _, tc := range initialValeChecks {
		alerts = append(alerts, ValeAlert{
			File:     "docs/example.md",
			Check:    tc.Check,
			Line:     3,
			Span:     [2]int{1, 5},
			Severity: "warning",
			Message:  "DDx." + tc.Check + " raw vale message",
			Match:    "match",
		})
	}
	findings := NormalizeValeAlerts(alerts)
	if len(findings) != len(initialValeChecks) {
		t.Fatalf("expected %d findings, got %d", len(initialValeChecks), len(findings))
	}
	for _, f := range findings {
		if strings.HasPrefix(f.RuleID, "DDx.") {
			t.Errorf("rule id %q exposes raw Vale check name", f.RuleID)
		}
		if strings.Contains(f.Rationale, "DDx.") {
			t.Errorf("rationale exposes raw Vale check name: %q", f.Rationale)
		}
		if strings.Contains(f.SuggestedEdit, "DDx.") {
			t.Errorf("suggested edit exposes raw Vale check name: %q", f.SuggestedEdit)
		}
		if strings.Contains(f.Rationale, "raw vale message") {
			t.Errorf("rationale leaks raw Vale message: %q", f.Rationale)
		}
		if strings.Contains(f.SuggestedEdit, "raw vale message") {
			t.Errorf("suggested edit leaks raw Vale message: %q", f.SuggestedEdit)
		}
	}

	// An unknown Vale check id must be dropped rather than passed through.
	unknown := []ValeAlert{{
		File:     "docs/example.md",
		Check:    "OtherStyle.MadeUpRule",
		Line:     1,
		Span:     [2]int{1, 5},
		Severity: "warning",
		Message:  "raw",
		Match:    "x",
	}}
	if got := NormalizeValeAlerts(unknown); len(got) != 0 {
		t.Fatalf("expected unknown Vale check to produce no findings, got %+v", got)
	}
}
