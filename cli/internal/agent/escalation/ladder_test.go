package escalation

import (
	"errors"
	"testing"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

func mkModel(power int, available, autoRoutable bool) agentlib.ModelInfo {
	return agentlib.ModelInfo{
		Power:        power,
		Available:    available,
		AutoRoutable: autoRoutable,
	}
}

func viable(power int) agentlib.ModelInfo    { return mkModel(power, true, true) }
func nonviable(power int) agentlib.ModelInfo { return mkModel(power, false, true) }

func TestLadder_Tiers_DistinctAscending(t *testing.T) {
	l := NewLadder([]agentlib.ModelInfo{
		viable(70), viable(50), viable(70), viable(90), viable(50),
	})
	got := l.Tiers()
	want := []int{50, 70, 90}
	if len(got) != len(want) {
		t.Fatalf("Tiers len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Tiers[%d]: got %d want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestLadder_IgnoresUnratedModels(t *testing.T) {
	l := NewLadder([]agentlib.ModelInfo{
		viable(50),
		mkModel(0, true, true), // unrated
		mkModel(-10, true, true),
		viable(80),
	})
	got := l.Tiers()
	if len(got) != 2 || got[0] != 50 || got[1] != 80 {
		t.Fatalf("Tiers: got %v want [50 80]", got)
	}
}

func TestLadder_Next_AllViable_StepsThroughTiers(t *testing.T) {
	l := NewLadder([]agentlib.ModelInfo{viable(50), viable(70), viable(90)})

	cases := []struct {
		in, want int
	}{
		{0, 50},
		{50, 70},
		{60, 70}, // strictly greater
		{70, 90},
		{89, 90},
	}
	for _, tc := range cases {
		got, err := l.Next(tc.in)
		if err != nil {
			t.Fatalf("Next(%d): unexpected err %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Next(%d): got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestLadder_Next_ExhaustedReturnsTypedError(t *testing.T) {
	l := NewLadder([]agentlib.ModelInfo{viable(50), viable(90)})

	if _, err := l.Next(90); !errors.Is(err, ErrLadderExhausted) {
		t.Fatalf("Next(90): err = %v; want ErrLadderExhausted", err)
	}
	if _, err := l.Next(95); !errors.Is(err, ErrLadderExhausted) {
		t.Fatalf("Next(95): err = %v; want ErrLadderExhausted", err)
	}
}

func TestLadder_Next_EmptyCatalogExhausted(t *testing.T) {
	l := NewLadder(nil)
	if _, err := l.Next(0); !errors.Is(err, ErrLadderExhausted) {
		t.Fatalf("empty catalog Next: err = %v; want ErrLadderExhausted", err)
	}
}

func TestLadder_Next_NilReceiverExhausted(t *testing.T) {
	var l *Ladder
	if _, err := l.Next(0); !errors.Is(err, ErrLadderExhausted) {
		t.Fatalf("nil ladder Next: err = %v; want ErrLadderExhausted", err)
	}
}

func TestLadder_Next_SkipTierReturnsNoViableProviderError(t *testing.T) {
	// 50 viable, 70 has only non-viable models, 90 viable.
	l := NewLadder([]agentlib.ModelInfo{
		viable(50),
		nonviable(70),
		viable(90),
	})

	floor, err := l.Next(50)
	if err == nil {
		t.Fatalf("Next(50): expected NoViableProviderError, got floor=%d nil err", floor)
	}
	var nvp *NoViableProviderError
	if !errors.As(err, &nvp) {
		t.Fatalf("Next(50): err = %v; want *NoViableProviderError", err)
	}
	if nvp.Floor != 70 {
		t.Fatalf("NoViableProviderError.Floor: got %d want 70", nvp.Floor)
	}

	// Loop bumps further by calling Next(floor=70).
	got, err := l.Next(nvp.Floor)
	if err != nil {
		t.Fatalf("Next(70) after skip: unexpected err %v", err)
	}
	if got != 90 {
		t.Fatalf("Next(70) after skip: got %d want 90", got)
	}
}

func TestLadder_Next_AllNonViableExhaustsViaSkip(t *testing.T) {
	l := NewLadder([]agentlib.ModelInfo{
		nonviable(50), nonviable(70), nonviable(90),
	})

	// First call returns NoViableProvider for tier 50.
	_, err := l.Next(0)
	var nvp *NoViableProviderError
	if !errors.As(err, &nvp) || nvp.Floor != 50 {
		t.Fatalf("Next(0): want NoViable@50, got %v", err)
	}
	// Bump past 50 → NoViable@70.
	_, err = l.Next(nvp.Floor)
	if !errors.As(err, &nvp) || nvp.Floor != 70 {
		t.Fatalf("Next(50): want NoViable@70, got %v", err)
	}
	// Bump past 70 → NoViable@90.
	_, err = l.Next(nvp.Floor)
	if !errors.As(err, &nvp) || nvp.Floor != 90 {
		t.Fatalf("Next(70): want NoViable@90, got %v", err)
	}
	// Bump past 90 → exhausted.
	_, err = l.Next(nvp.Floor)
	if !errors.Is(err, ErrLadderExhausted) {
		t.Fatalf("Next(90): want ErrLadderExhausted, got %v", err)
	}
}

func TestLadder_Next_ViabilityDropsAtPartiallyAvailableTier(t *testing.T) {
	// Tier 70 has one Available but non-AutoRoutable model and one
	// non-Available AutoRoutable model — neither counts as viable.
	l := NewLadder([]agentlib.ModelInfo{
		viable(50),
		mkModel(70, true, false),
		mkModel(70, false, true),
		viable(90),
	})
	_, err := l.Next(50)
	var nvp *NoViableProviderError
	if !errors.As(err, &nvp) || nvp.Floor != 70 {
		t.Fatalf("Next(50): want NoViable@70, got %v", err)
	}
}

// TestLadder_Next_UsesRoutingActualPower asserts that the ladder consumes
// RoutingActual.Power as the input for next-floor computation. The bead
// contract guarantees this field lands as the previous attempt's actual
// routed power on RoutingActual; the ladder API takes that integer
// directly.
func TestLadder_Next_UsesRoutingActualPower(t *testing.T) {
	// Stand-in for the RoutingActual record carried between attempts.
	// The field name and type mirror the contract: RoutingActual.Power int.
	type routingActual struct {
		Harness  string
		Provider string
		Model    string
		Power    int
	}

	l := NewLadder([]agentlib.ModelInfo{viable(50), viable(70), viable(90)})

	prev := routingActual{
		Harness:  "claude",
		Provider: "anthropic",
		Model:    "some-model",
		Power:    70,
	}

	// The escalation loop sources actualPower from the previous attempt's
	// RoutingActual.Power and passes it directly to Next.
	got, err := l.Next(prev.Power)
	if err != nil {
		t.Fatalf("Next(prev.Power=%d): unexpected err %v", prev.Power, err)
	}
	if got != 90 {
		t.Fatalf("Next(prev.Power=70): got %d want 90", got)
	}

	// And confirm the input value is the only relevant signal — provider,
	// model, and harness on the routing record never enter the decision.
	prev.Provider = "different-vendor"
	prev.Model = "different-model"
	prev.Harness = "different-harness"
	got2, err := l.Next(prev.Power)
	if err != nil {
		t.Fatalf("Next(prev.Power=70) after vendor swap: unexpected err %v", err)
	}
	if got2 != got {
		t.Fatalf("Next must depend on Power only; got %d then %d", got, got2)
	}
}

func TestNoViableProviderError_ErrorMessageIncludesFloor(t *testing.T) {
	err := &NoViableProviderError{Floor: 80}
	want := "escalation: no viable provider at floor 80"
	if err.Error() != want {
		t.Fatalf("Error(): got %q want %q", err.Error(), want)
	}
}
