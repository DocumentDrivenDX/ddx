package agent

import (
	"context"
	"fmt"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkProviderProcessMissingExecutableClassified (AC1, ddx-ef9df563):
// when the harness binary cannot be found (exec.ErrNotFound equivalent), the
// pre-dispatch Execute error must be wrapped as a typed ProviderFailureError
// with Reason=provider_harness_unavailable. No "routing under-specified"
// message may appear and the bead must be left unclaimed (retryable=true so
// the work loop will unclaim and defer rather than park permanently).
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
	require.ErrorAs(t, err, &pfErr, "missing harness executable must yield a typed ProviderFailureError")
	assert.Equal(t, FailureModeProviderHarnessUnavailable, pfErr.Failure.Reason,
		"typed_failure must be provider_harness_unavailable, not a generic bucket")
	assert.True(t, pfErr.Failure.Retryable,
		"harness_unavailable is retryable: an unpinned worker may fall back to another route")
	assert.NotContains(t, pfErr.Error(), "routing under-specified",
		"must not produce an underspecified-routing message")

	// Verify ApplyProviderFailureToReport stamps the typed reason onto the report
	// so the work loop records provider_harness_unavailable, not execution_failed.
	report := ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed}
	ApplyProviderFailureToReport(&report, pfErr.Failure)
	assert.Equal(t, FailureModeProviderHarnessUnavailable, report.OutcomeReason,
		"outcome_reason must be the typed provider reason, not the generic execution_failed status")
	assert.True(t, report.Disrupted)
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
