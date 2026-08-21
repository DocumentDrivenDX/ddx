package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkProviderProcessMissingExecutableClassified (AC1, ddx-ef9df563):
// plain free-text pre-dispatch Execute errors are not scraped into the provider
// taxonomy (ClassifyServiceExecuteError is typed-only). A bare
// "executable file not found" fmt.Errorf therefore surfaces as
// unknown_provider_failure. Typed Fizeau errors and the early-exit path below
// remain the control paths for harness_unavailable.
func TestWorkProviderProcessMissingExecutableClassified(t *testing.T) {
	svc := &passthroughTestService{
		executeErr: fmt.Errorf("executable file not found in $PATH: claude-code"),
	}
	rcfg := resolvedWithPassthrough("claude", "", "", 0, 0)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "do the work",
	})
	require.Error(t, err)

	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr, "pre-dispatch Execute error must yield a typed ProviderFailureError")
	assert.Equal(t, FailureModeUnknownProviderFailure, pfErr.Failure.Reason,
		"free-text missing-executable errors must not be scraped; typed Fizeau errors own harness_unavailable")
	assert.False(t, pfErr.Failure.Retryable)
	assert.NotContains(t, pfErr.Error(), "routing under-specified",
		"must not produce an underspecified-routing message")
}

// TestWorkProviderProcessEarlyExitClassifiedWithEvidence (AC2, ddx-ef9df563):
// when the provider process emits events but exits before sending a final event
// (simulating a child crash or OOM kill), executeOnService must return a typed
// ProviderFailureError with Reason=provider_harness_unavailable and the error
// message must describe the early-exit condition. The caller can then build
// ProviderFailureEvidence with all required fields: requested_harness,
// requested_provider, requested_model, typed_failure, retryable,
// fallback_attempted, and a process-detail equivalent via the error message.
func TestWorkProviderProcessEarlyExitClassifiedWithEvidence(t *testing.T) {
	// Provider process sends a progress event but exits before the final event.
	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{Type: "text_delta", Data: []byte(`{"text":"working..."}`)},
			// Channel closes here without a "final" event — simulates early child exit.
		},
	}
	rcfg := resolvedWithPassthrough("claude", "anthropic", "claude-opus", 5, 90)

	_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "do the work",
	})
	require.Error(t, err, "early child exit must return an error, not a zero-exit success")

	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr, "early child exit must yield a typed ProviderFailureError")
	assert.Equal(t, FailureModeProviderHarnessUnavailable, pfErr.Failure.Reason)
	assert.True(t, pfErr.Failure.Retryable,
		"early exit is retryable: another route may have a healthy harness binary")
	// Process-detail equivalent: the error message names the early-exit condition.
	assert.Contains(t, pfErr.Error(), "final event",
		"error message must describe the early-exit condition as process detail equivalent")

	// Build structured evidence to confirm all required AC2 fields are present.
	decision := DecideProviderFallback(pfErr.Failure, false /* unpinned */)
	evidence := BuildProviderFailureEvidence(
		ProviderFailureRequest{
			Harness:  "claude",
			Provider: "anthropic",
			Model:    "claude-opus",
			MinPower: 5,
			MaxPower: 90,
		},
		nil, // No route resolved before the early exit.
		pfErr.Failure,
		decision,
	)
	assert.Equal(t, "claude", evidence.RequestedHarness)
	assert.Equal(t, "anthropic", evidence.RequestedProvider)
	assert.Equal(t, "claude-opus", evidence.RequestedModel)
	assert.Equal(t, FailureModeProviderHarnessUnavailable, evidence.TypedFailure)
	assert.True(t, evidence.Retryable)
	assert.True(t, evidence.FallbackAttempted, "unpinned worker with retryable failure must record fallback_attempted=true")
	assert.Empty(t, evidence.FallbackStopReason, "fallback was not stopped; the work loop should retry another route")
	// cleanup_result equivalent: the error is returned (not silently swallowed),
	// so the caller can record it as the cleanup/unclaim outcome.
	assert.NotEmpty(t, pfErr.Error())
}

// TestFailedFinalTranscriptIncompleteClassifiesHarnessUnavailable: a Fizeau
// failed final carrying the claude-tui transcript-incomplete diagnostic must
// normalize to provider_harness_unavailable so unpinned workers fall back
// instead of parking as unknown.
func TestFailedFinalTranscriptIncompleteClassifiesHarnessUnavailable(t *testing.T) {
	finalPayload, err := json.Marshal(map[string]any{
		"status":    "failed",
		"exit_code": 1,
		"error":     "Claude transcript contained no assistant final event",
		"outcome":   "failed",
		"cause":     "harness_failed",
		"stage":     "harness",
	})
	require.NoError(t, err)
	svc := &passthroughTestService{executeEvents: []agentlib.ServiceEvent{
		{Type: "final", Data: finalPayload},
	}}
	rcfg := resolvedWithPassthrough("", "", "", 0, 0)

	result, runErr := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
		Prompt: "do the work",
	})
	require.NoError(t, runErr, "a failed final event is reported on the result, not as a run error")
	require.NotNil(t, result)
	require.NotEqual(t, 0, result.ExitCode)
	assert.Contains(t, result.Error, "no assistant final event")

	report := ExecuteBeadReport{
		Status:        ExecuteBeadStatusExecutionFailed,
		Error:         result.Error,
		FizeauOutcome: result.FizeauOutcome,
		FizeauCause:   result.FizeauCause,
		FizeauStage:   result.FizeauStage,
		Harness:       result.Harness,
	}
	if report.FizeauCause == "" {
		report.FizeauCause = "harness_failed"
	}
	if report.FizeauOutcome == "" {
		report.FizeauOutcome = "failed"
	}
	classifyLoopReportFailure(&report)

	assert.Equal(t, FailureModeProviderHarnessUnavailable, report.OutcomeReason)
	assert.True(t, report.Disrupted)

	pf, ok := ProviderFailureFromReason(report.OutcomeReason)
	require.True(t, ok)
	assert.True(t, pf.Retryable)
	unpinned := DecideProviderFallback(pf, false)
	assert.True(t, unpinned.Continue, "unpinned worker must fall back after transcript-incomplete")
	pinned := DecideProviderFallback(pf, true)
	assert.False(t, pinned.Continue)
	assert.Equal(t, FallbackStopHardPinExhausted, pinned.StopReason)
}
