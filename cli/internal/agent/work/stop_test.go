package work

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStopCondition_ClassifiesDrainExitReasons(t *testing.T) {
	t.Run("stable values", func(t *testing.T) {
		got := []StopCondition{
			StopConditionDrained,
			StopConditionOnce,
			StopConditionSignal,
			StopConditionBudget,
			StopConditionNoProgress,
			StopConditionBlocked,
		}
		want := []string{"Drained", "Once", "Signal", "Budget", "NoProgress", "Blocked"}
		for i := range got {
			if string(got[i]) != want[i] {
				t.Fatalf("StopCondition[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	tests := []struct {
		name       string
		input      StopInput
		wantOK     bool
		wantCond   StopCondition
		wantReason string
	}{
		{
			name:       "drained explicit poll zero",
			input:      StopInput{NoReadyWork: true, PollInterval: 0},
			wantOK:     true,
			wantCond:   StopConditionDrained,
			wantReason: "explicit_poll_zero",
		},
		{
			name:       "once",
			input:      StopInput{Once: true, PollInterval: 30 * time.Second},
			wantOK:     true,
			wantCond:   StopConditionOnce,
			wantReason: "once_complete",
		},
		{
			name:       "signal canceled",
			input:      StopInput{ContextErr: context.Canceled},
			wantOK:     true,
			wantCond:   StopConditionSignal,
			wantReason: "sigint",
		},
		{
			name:       "signal deadline",
			input:      StopInput{ContextErr: context.DeadlineExceeded},
			wantOK:     true,
			wantCond:   StopConditionSignal,
			wantReason: "sigterm",
		},
		{
			name:       "signal other",
			input:      StopInput{ContextErr: errors.New("closed")},
			wantOK:     true,
			wantCond:   StopConditionSignal,
			wantReason: "context_cancelled",
		},
		{
			name:       "budget",
			input:      StopInput{Budget: true},
			wantOK:     true,
			wantCond:   StopConditionBudget,
			wantReason: "budget",
		},
		{
			name:       "no progress",
			input:      StopInput{NoProgress: true},
			wantOK:     true,
			wantCond:   StopConditionNoProgress,
			wantReason: "no_progress",
		},
		{
			name:       "blocked",
			input:      StopInput{Blocked: true},
			wantOK:     true,
			wantCond:   StopConditionBlocked,
			wantReason: "blocked",
		},
		{
			name:   "idle is nonterminal",
			input:  StopInput{NoReadyWork: true, PollInterval: time.Second},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ClassifyStop(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Condition != tc.wantCond {
				t.Fatalf("Condition = %s, want %s", got.Condition, tc.wantCond)
			}
			if got.ExitReason != tc.wantReason {
				t.Fatalf("ExitReason = %q, want %q", got.ExitReason, tc.wantReason)
			}
		})
	}
}
