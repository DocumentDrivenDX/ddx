package triage

import "testing"

func TestDefaultPolicy_BlockLadderProgression(t *testing.T) {
	p := DefaultPolicy()
	bead := "ddx-test"

	cases := []struct {
		name    string
		history []FailureMode
		want    Action
	}{
		{"first BLOCK", nil, ActionReAttemptWithContext},
		{"second BLOCK", []FailureMode{FailureModeReviewBlock}, ActionEscalateTier},
		{"third BLOCK", []FailureMode{FailureModeReviewBlock, FailureModeReviewBlock}, ActionNeedsHuman},
		{"fourth BLOCK clamps to last rung",
			[]FailureMode{FailureModeReviewBlock, FailureModeReviewBlock, FailureModeReviewBlock},
			ActionNeedsHuman},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Decide(bead, FailureModeReviewBlock, tc.history)
			if got != tc.want {
				t.Fatalf("Decide(...)=%q want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultPolicy_NonBlockModesHaveDefaults(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		mode FailureMode
		want Action
	}{
		{FailureModeLockContention, ActionRetryWithBackoff},
		{FailureModeExecutionFailed, ActionReAttemptWithContext},
		{FailureModeNoChanges, ActionReAttemptWithContext},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			got := p.Decide("ddx-test", tc.mode, nil)
			if got != tc.want {
				t.Fatalf("Decide(%q, nil)=%q want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestDefaultPolicy_OnlyMatchingModeAdvancesLadder(t *testing.T) {
	p := DefaultPolicy()
	// Mixed history: a prior execution_failed should NOT advance the
	// review_block ladder.
	hist := []FailureMode{FailureModeExecutionFailed, FailureModeNoChanges}
	got := p.Decide("ddx-test", FailureModeReviewBlock, hist)
	if got != ActionReAttemptWithContext {
		t.Fatalf("expected first-rung action, got %q", got)
	}
}

func TestPolicy_UnknownModeFallsBackToNeedsHuman(t *testing.T) {
	p := DefaultPolicy()
	got := p.Decide("ddx-test", FailureMode("not_a_mode"), nil)
	if got != ActionNeedsHuman {
		t.Fatalf("unknown mode: got %q want %q", got, ActionNeedsHuman)
	}
}

func TestPolicy_ConfigOverride(t *testing.T) {
	// Caller-supplied policy overrides the default ladder for review_block.
	custom := TriagePolicy{Ladders: map[FailureMode][]Action{
		FailureModeReviewBlock: {ActionEscalateTier, ActionNeedsHuman},
	}}
	got := custom.Decide("ddx-test", FailureModeReviewBlock, nil)
	if got != ActionEscalateTier {
		t.Fatalf("override first rung: got %q want %q", got, ActionEscalateTier)
	}
	got = custom.Decide("ddx-test", FailureModeReviewBlock, []FailureMode{FailureModeReviewBlock})
	if got != ActionNeedsHuman {
		t.Fatalf("override second rung: got %q want %q", got, ActionNeedsHuman)
	}
	// A mode absent from the override map yields ActionNeedsHuman.
	got = custom.Decide("ddx-test", FailureModeNoChanges, nil)
	if got != ActionNeedsHuman {
		t.Fatalf("absent mode: got %q want %q", got, ActionNeedsHuman)
	}
}

func TestParseAction(t *testing.T) {
	if _, err := ParseAction("re_attempt_with_context"); err != nil {
		t.Fatalf("valid action rejected: %v", err)
	}
	if _, err := ParseAction("nope"); err == nil {
		t.Fatalf("invalid action accepted")
	}
}

func TestParseFailureMode(t *testing.T) {
	if _, err := ParseFailureMode("review_block"); err != nil {
		t.Fatalf("valid mode rejected: %v", err)
	}
	if _, err := ParseFailureMode("nope"); err == nil {
		t.Fatalf("invalid mode accepted")
	}
}
