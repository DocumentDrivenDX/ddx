package evidence

import (
	"strings"
	"testing"
)

func TestAssembleRefOnly(t *testing.T) {
	out := AssembleRefOnly([]RefOnlyEntry{
		{Path: "docs/a.md", Title: "A"},
		{Path: "docs/b.md"},
	})
	if !strings.Contains(out, "docs/a.md") || !strings.Contains(out, "A") {
		t.Errorf("ref-only output missing entry: %q", out)
	}
	if !strings.Contains(out, "docs/b.md") {
		t.Errorf("ref-only output missing untitled entry: %q", out)
	}
}

func TestAssembleRefOnlyEmpty(t *testing.T) {
	if got := AssembleRefOnly(nil); got != "" {
		t.Errorf("empty entries should yield empty string, got %q", got)
	}
}

func TestAssembleInlineFits(t *testing.T) {
	res := AssembleInline([]SectionInput{
		{Name: "a", Content: "alpha", MinFloor: true},
		{Name: "b", Content: "beta"},
	}, 1000)
	if res.Overflow {
		t.Errorf("unexpected overflow")
	}
	if !strings.Contains(res.Prompt, "alpha") || !strings.Contains(res.Prompt, "beta") {
		t.Errorf("prompt missing content: %q", res.Prompt)
	}
	if len(res.Sections) != 2 {
		t.Errorf("sections = %d, want 2", len(res.Sections))
	}
}

func TestAssembleInlineEmits_PinnedOutcomeClasses(t *testing.T) {
	// Pin the contract Stage C1/C2/G consumes.
	if OutcomeReviewContextOverflow != "review-error: context_overflow" {
		t.Errorf("OutcomeReviewContextOverflow drift: %q", OutcomeReviewContextOverflow)
	}
	if OutcomeCompareContextOverflow != "compare-error: context_overflow" {
		t.Errorf("OutcomeCompareContextOverflow drift: %q", OutcomeCompareContextOverflow)
	}
}
