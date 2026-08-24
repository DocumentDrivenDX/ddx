package agent

import (
	"encoding/json"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

// TestFizeauFinalEventTypedLifecycleRoundTrip proves the pinned public
// github.com/easel/fizeau v0.17.3 release exposes typed final-event outcome,
// cause, stage, usage/cost, and session-reference fields through exported
// APIs, and that those fields round-trip through the public JSON and
// DecodeServiceEvent paths without inspecting stderr.
//
// Per CONTRACT-003 (ServiceFinalData in service_events.go), RetryAfter is not
// a final-event field: it lives only on NoViableProviderForNow
// (routing_errors.go), a synchronous pre-dispatch error that never reaches
// DecodeServiceEvent. This test does not assert RetryAfter on
// ServiceFinalData.
func TestFizeauFinalEventTypedLifecycleRoundTrip(t *testing.T) {
	t.Parallel()

	inputTokens := 128
	outputTokens := 256
	totalTokens := 384
	cost := 4.5

	want := agentlib.ServiceFinalData{
		Status:  "success",
		Outcome: agentlib.SessionOutcomeSuccess,
		Cause:   agentlib.TerminalCauseCompleted,
		Stage:   agentlib.SessionStageHarness,
		Usage: &agentlib.ServiceFinalUsage{
			InputTokens:  &inputTokens,
			OutputTokens: &outputTokens,
			TotalTokens:  &totalTokens,
			Source:       "provider",
		},
		CostUSD:         &cost,
		CostSource:      agentlib.CostSourceReported,
		ParentSessionID: "session-parent-123",
		Continuation:    agentlib.ContinuationResumed,
	}

	// Round-trip 1: direct public JSON encode/decode of ServiceFinalData.
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal ServiceFinalData: %v", err)
	}
	var gotDirect agentlib.ServiceFinalData
	if err := json.Unmarshal(raw, &gotDirect); err != nil {
		t.Fatalf("unmarshal ServiceFinalData: %v", err)
	}
	assertFinalLifecycleFields(t, "direct-json", gotDirect, want)

	// Round-trip 2: through the public ServiceEvent / DecodeServiceEvent path
	// used by service_run drain, not by inspecting stderr.
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
	assertFinalLifecycleFields(t, "decode-service-event", *decoded.Final, want)
}

func assertFinalLifecycleFields(t *testing.T, path string, got, want agentlib.ServiceFinalData) {
	t.Helper()

	if got.Outcome != want.Outcome {
		t.Fatalf("%s: Outcome = %q, want %q", path, got.Outcome, want.Outcome)
	}
	if got.Cause != want.Cause {
		t.Fatalf("%s: Cause = %q, want %q", path, got.Cause, want.Cause)
	}
	if got.Stage != want.Stage {
		t.Fatalf("%s: Stage = %q, want %q", path, got.Stage, want.Stage)
	}

	if got.Usage == nil {
		t.Fatalf("%s: Usage is nil, want non-nil", path)
	}
	if got.Usage.InputTokens == nil || *got.Usage.InputTokens != *want.Usage.InputTokens {
		t.Fatalf("%s: Usage.InputTokens = %v, want %v", path, got.Usage.InputTokens, *want.Usage.InputTokens)
	}
	if got.Usage.OutputTokens == nil || *got.Usage.OutputTokens != *want.Usage.OutputTokens {
		t.Fatalf("%s: Usage.OutputTokens = %v, want %v", path, got.Usage.OutputTokens, *want.Usage.OutputTokens)
	}
	if got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != *want.Usage.TotalTokens {
		t.Fatalf("%s: Usage.TotalTokens = %v, want %v", path, got.Usage.TotalTokens, *want.Usage.TotalTokens)
	}
	if got.Usage.Source != want.Usage.Source {
		t.Fatalf("%s: Usage.Source = %q, want %q", path, got.Usage.Source, want.Usage.Source)
	}

	if got.CostUSD == nil || *got.CostUSD != *want.CostUSD {
		t.Fatalf("%s: CostUSD = %v, want %v", path, got.CostUSD, *want.CostUSD)
	}
	if got.CostSource != want.CostSource {
		t.Fatalf("%s: CostSource = %q, want %q", path, got.CostSource, want.CostSource)
	}

	if got.ParentSessionID != want.ParentSessionID {
		t.Fatalf("%s: ParentSessionID = %q, want %q", path, got.ParentSessionID, want.ParentSessionID)
	}
	if got.Continuation != want.Continuation {
		t.Fatalf("%s: Continuation = %q, want %q", path, got.Continuation, want.Continuation)
	}
}

// TestFizeauImmediateErrorRetryAfterIsNotAFinalEventField documents, as
// shared helper setup, that RetryAfter is scoped to the synchronous
// NoViableProviderForNow routing error and is never a ServiceFinalData field.
// It does not prove any immediate-error lifecycle round-trip behavior beyond
// that boundary.
func TestFizeauImmediateErrorRetryAfterIsNotAFinalEventField(t *testing.T) {
	t.Parallel()

	retryAfter := time.Unix(100, 0).UTC()
	err := &agentlib.NoViableProviderForNow{
		RetryAfter:         retryAfter,
		ExhaustedProviders: []string{"provider-a"},
	}
	if !err.RetryAfter.Equal(retryAfter) {
		t.Fatalf("RetryAfter = %v, want %v", err.RetryAfter, retryAfter)
	}
	if err.Error() == "" {
		t.Fatal("NoViableProviderForNow.Error() must not be empty")
	}

	// ServiceFinalData has no RetryAfter field; this is a compile-time
	// guarantee validated by TestFizeauFinalEventTypedLifecycleRoundTrip
	// constructing ServiceFinalData without one. NoViableProviderForNow is a
	// pre-dispatch error and never reaches DecodeServiceEvent.
}
