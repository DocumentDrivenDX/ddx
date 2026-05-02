package escalation

import (
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestInferTier_NilBead(t *testing.T) {
	if got := InferTier(nil); got != TierCheap {
		t.Fatalf("nil bead: want %q, got %q", TierCheap, got)
	}
}

func TestInferTier_ExplicitTierLabel(t *testing.T) {
	cases := []struct {
		label string
		want  ModelTier
	}{
		{"tier:smart", TierSmart},
		{"tier:standard", TierStandard},
		{"tier:cheap", TierCheap},
		{"TIER:SMART", TierSmart},
	}
	for _, c := range cases {
		b := &bead.Bead{Labels: []string{c.label}}
		if got := InferTier(b); got != c.want {
			t.Errorf("label %q: want %q, got %q", c.label, c.want, got)
		}
	}
}

func TestInferTier_ExplicitTierLabelOverridesPriority(t *testing.T) {
	// An explicit tier wins over priority:critical, which would otherwise
	// route to smart.
	b := &bead.Bead{Labels: []string{"priority:critical", "tier:cheap"}}
	if got := InferTier(b); got != TierCheap {
		t.Fatalf("explicit tier should win: got %q", got)
	}
}

func TestInferTier_PriorityCritical(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:critical", "kind:chore"}}
	if got := InferTier(b); got != TierSmart {
		t.Fatalf("critical priority: want smart, got %q", got)
	}
}

func TestInferTier_PriorityHighBug(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:high", "kind:bug"}}
	if got := InferTier(b); got != TierSmart {
		t.Fatalf("high-priority bug: want smart, got %q", got)
	}
}

func TestInferTier_PriorityHighEnhancement(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:high", "kind:enhancement"}}
	if got := InferTier(b); got != TierStandard {
		t.Fatalf("high-priority enhancement: want standard, got %q", got)
	}
}

func TestInferTier_PriorityLow(t *testing.T) {
	b := &bead.Bead{Labels: []string{"priority:low", "kind:bug"}}
	if got := InferTier(b); got != TierCheap {
		t.Fatalf("low priority: want cheap, got %q", got)
	}
}

func TestInferTier_KindBug(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:bug"}}
	if got := InferTier(b); got != TierStandard {
		t.Fatalf("bug kind: want standard, got %q", got)
	}
}

func TestInferTier_KindChore(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:chore"}}
	if got := InferTier(b); got != TierCheap {
		t.Fatalf("chore kind: want cheap, got %q", got)
	}
}

func TestInferTier_KindEnhancement(t *testing.T) {
	b := &bead.Bead{Labels: []string{"kind:enhancement"}}
	if got := InferTier(b); got != TierStandard {
		t.Fatalf("enhancement kind: want standard, got %q", got)
	}
}

func TestInferTier_IssueTypeFallback(t *testing.T) {
	b := &bead.Bead{IssueType: "bug"}
	if got := InferTier(b); got != TierStandard {
		t.Fatalf("issue_type bug: want standard, got %q", got)
	}
}

func TestInferTier_ScopeShort(t *testing.T) {
	b := &bead.Bead{Description: "fix typo"}
	if got := InferTier(b); got != TierCheap {
		t.Fatalf("short scope: want cheap, got %q", got)
	}
}

func TestInferTier_ScopeMedium(t *testing.T) {
	b := &bead.Bead{Description: strings.Repeat("x", 2000)}
	if got := InferTier(b); got != TierStandard {
		t.Fatalf("medium scope: want standard, got %q", got)
	}
}

func TestInferTier_ScopeLarge(t *testing.T) {
	b := &bead.Bead{Description: strings.Repeat("x", 5000)}
	if got := InferTier(b); got != TierSmart {
		t.Fatalf("large scope: want smart, got %q", got)
	}
}

func TestInferTier_NoMetadataDefaultsCheap(t *testing.T) {
	b := &bead.Bead{Title: "do thing"}
	if got := InferTier(b); got != TierCheap {
		t.Fatalf("no metadata: want cheap, got %q", got)
	}
}

func TestTierToProfile(t *testing.T) {
	cases := map[ModelTier]string{
		TierSmart:     "smart",
		TierStandard:  "standard",
		TierCheap:     "cheap",
		ModelTier(""): "cheap",
	}
	for tier, want := range cases {
		if got := TierToProfile(tier); got != want {
			t.Errorf("TierToProfile(%q): want %q, got %q", tier, want, got)
		}
	}
}
