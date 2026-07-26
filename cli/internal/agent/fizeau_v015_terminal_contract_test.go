package agent

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

// TestFizeauV015PublicContractCompiles proves the released Fizeau module
// exports the v0.15 typed terminal and portable-runtime public surfaces that
// DDx consumers depend on. Uses only exported package-level APIs.
func TestFizeauV015PublicContractCompiles(t *testing.T) {
	t.Parallel()

	// Typed terminal classification surface (CONTRACT-003 / v0.15).
	var (
		_ agentlib.SessionOutcome = agentlib.SessionOutcomeSuccess
		_ agentlib.SessionOutcome = agentlib.SessionOutcomeFailed
		_ agentlib.SessionOutcome = agentlib.SessionOutcomeCancelled
		_ agentlib.SessionOutcome = agentlib.SessionOutcomeTimedOut

		_ agentlib.TerminalCause = agentlib.TerminalCauseCompleted
		_ agentlib.TerminalCause = agentlib.TerminalCauseRouteUnavailable
		_ agentlib.TerminalCause = agentlib.TerminalCauseProviderFailed
		_ agentlib.TerminalCause = agentlib.TerminalCauseContextCancelled

		_ agentlib.SessionStage = agentlib.SessionStageRouting
		_ agentlib.SessionStage = agentlib.SessionStageHarness
		_ agentlib.SessionStage = agentlib.SessionStageProvider
		_ agentlib.SessionStage = agentlib.SessionStageCleanup

		_ agentlib.CostSource = agentlib.CostSourceReported
		_ agentlib.CostSource = agentlib.CostSourceConfigured
		_ agentlib.CostSource = agentlib.CostSourceUnknown
	)

	cost := 0.0
	final := agentlib.ServiceFinalData{
		Status:     "success",
		Outcome:    agentlib.SessionOutcomeSuccess,
		Cause:      agentlib.TerminalCauseCompleted,
		Stage:      agentlib.SessionStageHarness,
		CostUSD:    &cost,
		CostSource: agentlib.CostSourceReported,
	}
	if final.Outcome == "" || final.Cause == "" || final.Stage == "" {
		t.Fatal("ServiceFinalData typed terminal fields must be assignable")
	}

	// Public event decode path used by service_run drain.
	raw, err := json.Marshal(final)
	if err != nil {
		t.Fatalf("marshal final: %v", err)
	}
	ev := agentlib.ServiceEvent{
		Type:     agentlib.ServiceEventTypeFinal,
		Sequence: 1,
		Time:     time.Unix(1, 0).UTC(),
		Data:     raw,
	}
	decoded, err := agentlib.DecodeServiceEvent(ev)
	if err != nil {
		t.Fatalf("DecodeServiceEvent: %v", err)
	}
	if decoded.Final == nil {
		t.Fatal("DecodeServiceEvent final payload is nil")
	}

	// Portable-runtime public surface (compile + type identity only).
	var (
		_ agentlib.PortableRuntimeRequest
		_ *agentlib.PortableRuntimeBundle
		_ agentlib.PortableRuntimeMount
		_ error = agentlib.ErrPortableRuntimeRequestInvalid
		_ error = agentlib.ErrPortableRuntimeClosureIncomplete
		_ error = agentlib.ErrPortableRuntimeActivationInvalid
		_ error = agentlib.ErrPortableRuntimeCleanupIncomplete
	)
	_ = agentlib.PortableRuntimeGuestRoot()
	_ = agentlib.NewFromPortableRuntime

	// FizeauService is the production constructor/return type for DDx seams.
	var _ agentlib.FizeauService
	_ = agentlib.ServiceExecuteRequest{}
	_ = agentlib.ServiceOptions{}
}

// TestFizeauV015TerminalAndCostPresenceRoundTrip proves Outcome, Cause, and
// Stage survive the public terminal contract and that absent, explicit 0.0,
// and positive CostUSD remain distinguishable through public encode/decode.
func TestFizeauV015TerminalAndCostPresenceRoundTrip(t *testing.T) {
	t.Parallel()

	// Additive / future-safe typed values must round-trip unchanged.
	want := agentlib.ServiceFinalData{
		Status:         "failed",
		Outcome:        agentlib.SessionOutcome("future_outcome"),
		Cause:          agentlib.TerminalCause("future_cause"),
		Stage:          agentlib.SessionStage("future_stage"),
		PrimaryOutcome: agentlib.SessionOutcomeSuccess,
		PrimaryCause:   agentlib.TerminalCauseCompleted,
		PrimaryStage:   agentlib.SessionStageHarness,
		Error:          "diagnostic only",
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range [][]byte{[]byte(`"outcome"`), []byte(`"cause"`), []byte(`"stage"`)} {
		if !bytes.Contains(raw, key) {
			t.Fatalf("required key %s missing from %s", key, raw)
		}
	}

	var got agentlib.ServiceFinalData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Outcome != want.Outcome || got.Cause != want.Cause || got.Stage != want.Stage {
		t.Fatalf("typed terminal tuple changed: got %q/%q/%q want %q/%q/%q",
			got.Outcome, got.Cause, got.Stage, want.Outcome, want.Cause, want.Stage)
	}
	if got.PrimaryOutcome != want.PrimaryOutcome || got.PrimaryCause != want.PrimaryCause || got.PrimaryStage != want.PrimaryStage {
		t.Fatalf("primary tuple changed: got %q/%q/%q", got.PrimaryOutcome, got.PrimaryCause, got.PrimaryStage)
	}

	// Public service-event decode path preserves the same tuple.
	ev := agentlib.ServiceEvent{
		Type:     agentlib.ServiceEventTypeFinal,
		Sequence: 2,
		Time:     time.Unix(2, 0).UTC(),
		Data:     raw,
	}
	decoded, err := agentlib.DecodeServiceEvent(ev)
	if err != nil {
		t.Fatalf("DecodeServiceEvent: %v", err)
	}
	if decoded.Final == nil {
		t.Fatal("DecodeServiceEvent final is nil")
	}
	if decoded.Final.Outcome != want.Outcome || decoded.Final.Cause != want.Cause || decoded.Final.Stage != want.Stage {
		t.Fatalf("decoded terminal tuple changed: got %q/%q/%q",
			decoded.Final.Outcome, decoded.Final.Cause, decoded.Final.Stage)
	}

	// CostUSD presence: absent (nil), explicit 0.0, and positive must remain
	// distinguishable through public JSON and DecodeServiceEvent.
	zero := 0.0
	positive := 1.25
	cases := []struct {
		name       string
		cost       *float64
		source     agentlib.CostSource
		wantNil    bool
		wantValue  float64
		wantSource agentlib.CostSource
	}{
		{
			name:       "absent",
			cost:       nil,
			source:     agentlib.CostSourceUnknown,
			wantNil:    true,
			wantSource: agentlib.CostSourceUnknown,
		},
		{
			name:       "explicit_zero",
			cost:       &zero,
			source:     agentlib.CostSourceReported,
			wantNil:    false,
			wantValue:  0.0,
			wantSource: agentlib.CostSourceReported,
		},
		{
			name:       "positive",
			cost:       &positive,
			source:     agentlib.CostSourceConfigured,
			wantNil:    false,
			wantValue:  1.25,
			wantSource: agentlib.CostSourceConfigured,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := agentlib.ServiceFinalData{
				Status:     "success",
				Outcome:    agentlib.SessionOutcomeSuccess,
				Cause:      agentlib.TerminalCauseCompleted,
				Stage:      agentlib.SessionStageHarness,
				CostUSD:    tc.cost,
				CostSource: tc.source,
			}
			payload, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var out agentlib.ServiceFinalData
			if err := json.Unmarshal(payload, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			assertCostPresence(t, "json", out.CostUSD, out.CostSource, tc.wantNil, tc.wantValue, tc.wantSource)

			decoded, err := agentlib.DecodeServiceEvent(agentlib.ServiceEvent{
				Type:     agentlib.ServiceEventTypeFinal,
				Sequence: 3,
				Time:     time.Unix(3, 0).UTC(),
				Data:     payload,
			})
			if err != nil {
				t.Fatalf("DecodeServiceEvent: %v", err)
			}
			if decoded.Final == nil {
				t.Fatal("DecodeServiceEvent final is nil")
			}
			assertCostPresence(t, "decode", decoded.Final.CostUSD, decoded.Final.CostSource, tc.wantNil, tc.wantValue, tc.wantSource)
		})
	}

	// Cross-case distinguishability: the three presence states are not equal.
	absent := agentlib.ServiceFinalData{CostUSD: nil, CostSource: agentlib.CostSourceUnknown}
	explicitZero := agentlib.ServiceFinalData{CostUSD: &zero, CostSource: agentlib.CostSourceReported}
	positiveCost := agentlib.ServiceFinalData{CostUSD: &positive, CostSource: agentlib.CostSourceReported}
	if absent.CostUSD != nil {
		t.Fatal("absent cost must be nil pointer")
	}
	if explicitZero.CostUSD == nil || *explicitZero.CostUSD != 0 {
		t.Fatalf("explicit zero = %v, want non-nil 0.0", explicitZero.CostUSD)
	}
	if positiveCost.CostUSD == nil || *positiveCost.CostUSD <= 0 {
		t.Fatalf("positive cost = %v, want > 0", positiveCost.CostUSD)
	}
	if (absent.CostUSD == nil) == (explicitZero.CostUSD == nil) {
		t.Fatal("absent and explicit zero must differ by pointer presence")
	}
	if explicitZero.CostUSD != nil && positiveCost.CostUSD != nil && *explicitZero.CostUSD == *positiveCost.CostUSD {
		t.Fatal("explicit zero and positive must differ by value")
	}
}

func assertCostPresence(
	t *testing.T,
	path string,
	cost *float64,
	source agentlib.CostSource,
	wantNil bool,
	wantValue float64,
	wantSource agentlib.CostSource,
) {
	t.Helper()
	if wantNil {
		if cost != nil {
			t.Fatalf("%s: CostUSD = %v, want nil (absent)", path, *cost)
		}
	} else {
		if cost == nil {
			t.Fatalf("%s: CostUSD is nil, want %v", path, wantValue)
		}
		if *cost != wantValue {
			t.Fatalf("%s: CostUSD = %v, want %v", path, *cost, wantValue)
		}
	}
	if source != wantSource {
		t.Fatalf("%s: CostSource = %q, want %q", path, source, wantSource)
	}
}
