package cmd

import (
	"context"
	"reflect"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
)

func TestResolveReadinessDifficultyWithoutBeadMutation(t *testing.T) {
	target := &bead.Bead{
		ID:     "ddx-readiness-difficulty",
		Labels: []string{"priority:high"},
		Extra:  map[string]any{"unrelated": "preserved"},
	}
	wantLabels := append([]string(nil), target.Labels...)
	wantExtra := map[string]any{"unrelated": "preserved"}
	ctx := agent.ContextWithReadinessEstimatedDifficulty(context.Background(), string(escalation.DifficultyHard))

	hint := resolveCommandExecutionHint(ctx, target, false, "")
	initialMinPower := 0
	if hint.HasInferredMinPower {
		initialMinPower = hint.InferredMinPower
	}

	if hint.Source != escalation.ExecutionIntentSourceReadiness || initialMinPower != 9 {
		t.Fatalf("readiness fallback did not affect current dispatch: %+v", hint)
	}
	if !reflect.DeepEqual(target.Labels, wantLabels) || !reflect.DeepEqual(target.Extra, wantExtra) {
		t.Fatalf("readiness inference mutated bead: labels=%v extra=%v", target.Labels, target.Extra)
	}
}

func TestTryDifficultyInferenceUsesCurrentTarget(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	current := &bead.Bead{
		ID:    "ddx-current-target",
		Title: "current target",
		Extra: map[string]any{escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyHard)},
	}
	if err := store.Create(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	stale := &bead.Bead{
		ID:    current.ID,
		Title: current.Title,
		Extra: map[string]any{escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyEasy)},
	}

	hint, err := resolveTryExecutionHint(context.Background(), current.ID, store, stale, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if hint.EstimatedDifficulty != escalation.DifficultyHard || !hint.HasInferredMinPower || hint.InferredMinPower != 9 {
		t.Fatalf("try used stale preflight target instead of current bead: %+v", hint)
	}
}
