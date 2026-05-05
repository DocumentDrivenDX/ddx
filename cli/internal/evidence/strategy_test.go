package evidence

import (
	"strings"
	"testing"
)

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
