package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceExecuteErrorClassifiesProviderConfigFailures (AC1, ddx-3b721804):
// a fake Fizeau service returning representative pre-dispatch Execute errors —
// auth missing, model unavailable, provider unavailable, endpoint timeout —
// must yield a typed ExecuteBeadReport outcome_reason/disruption_reason rather
// than the generic execution_failed bucket.
func TestServiceExecuteErrorClassifiesProviderConfigFailures(t *testing.T) {
	cases := []struct {
		name          string
		execErr       error
		wantReason    string
		wantRetryable bool
	}{
		{
			name:          "auth_missing",
			execErr:       errors.New("missing api key for provider openai"),
			wantReason:    FailureModeProviderAuth,
			wantRetryable: true,
		},
		{
			name:          "model_unavailable",
			execErr:       errors.New("model claude-x is not available on this harness"),
			wantReason:    FailureModeProviderModelUnavailable,
			wantRetryable: true,
		},
		{
			name:          "model_unavailable_typed_fizeau_error",
			execErr:       &agentlib.ErrHarnessModelIncompatible{Harness: "claude", Model: "gpt-foo"},
			wantReason:    FailureModeProviderModelUnavailable,
			wantRetryable: true,
		},
		{
			name:          "provider_unavailable",
			execErr:       errors.New("provider request failed: dial tcp 100.64.0.1:1234: connect: connection refused"),
			wantReason:    FailureModeProviderConnectivity,
			wantRetryable: true,
		},
		{
			name:          "endpoint_timeout",
			execErr:       errors.New("provider request timeout exceeded"),
			wantReason:    FailureModeProviderConnectivity,
			wantRetryable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &passthroughTestService{executeErr: tc.execErr}
			rcfg := resolvedWithPassthrough("claude", "", "", 0, 0)

			_, err := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
				Prompt: "do the work",
			})
			require.Error(t, err)

			var pfErr *ProviderFailureError
			require.ErrorAs(t, err, &pfErr, "pre-dispatch Execute error must carry a typed ProviderFailure")
			assert.Equal(t, tc.wantReason, pfErr.Failure.Reason)
			assert.Equal(t, tc.wantRetryable, pfErr.Failure.Retryable)

			// The typed failure must produce a typed report outcome_reason +
			// disruption_reason, not a generic execution_failed classification.
			report := ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed}
			ApplyProviderFailureToReport(&report, pfErr.Failure)
			assert.Equal(t, tc.wantReason, report.OutcomeReason)
			assert.Equal(t, tc.wantReason, report.DisruptionReason)
			assert.True(t, report.Disrupted)
			assert.NotEqual(t, ExecuteBeadStatusExecutionFailed, report.OutcomeReason,
				"outcome_reason must be a typed provider reason, not the generic status")
		})
	}
}

// TestServiceFinalFailureClassifiesProviderRouteHealth (AC2, ddx-3b721804):
// fake failed final events for provider timeout, quota/rate-limit, model
// unavailable, and harness unavailable must normalize into the provider failure
// taxonomy, preserving retryable (unpinned may fall back) vs hard-pin (pinned
// never widens) semantics.
func TestServiceFinalFailureClassifiesProviderRouteHealth(t *testing.T) {
	cases := []struct {
		name          string
		finalError    string
		wantReason    string
		wantRetryable bool
	}{
		{
			name:          "provider_timeout",
			finalError:    "provider request timeout exceeded",
			wantReason:    FailureModeProviderConnectivity,
			wantRetryable: true,
		},
		{
			name:          "rate_limit",
			finalError:    "429 Too Many Requests: rate limit reached",
			wantReason:    FailureModeProviderRateLimit,
			wantRetryable: true,
		},
		{
			name:          "quota",
			finalError:    "insufficient_quota: you have exceeded your current quota",
			wantReason:    FailureModeProviderQuota,
			wantRetryable: true,
		},
		{
			name:          "model_unavailable",
			finalError:    "model gpt-foo not found",
			wantReason:    FailureModeProviderModelUnavailable,
			wantRetryable: true,
		},
		{
			name:          "harness_unavailable",
			finalError:    "harness not available: claude binary missing",
			wantReason:    FailureModeProviderHarnessUnavailable,
			wantRetryable: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finalPayload, err := json.Marshal(map[string]any{
				"status":    "failed",
				"exit_code": 1,
				"error":     tc.finalError,
			})
			require.NoError(t, err)
			svc := &passthroughTestService{executeEvents: []agentlib.ServiceEvent{
				{Type: "final", Data: finalPayload},
			}}
			rcfg := resolvedWithPassthrough("claude", "", "", 0, 0)

			result, runErr := executeOnService(context.Background(), svc, t.TempDir(), rcfg, AgentRunRuntime{
				Prompt: "do the work",
			})
			require.NoError(t, runErr, "a failed final event is reported on the result, not as a run error")
			require.NotNil(t, result)
			require.NotEqual(t, 0, result.ExitCode)

			pf, ok := ClassifyProviderFailure(result.Error)
			require.True(t, ok, "failed final event %q must classify as a provider failure", result.Error)
			assert.Equal(t, tc.wantReason, pf.Reason)
			assert.Equal(t, tc.wantRetryable, pf.Retryable)

			// Retryable-vs-hard-pin semantics: an unpinned worker may fall back
			// when retryable; a pinned worker never widens (hard-pin exhausted).
			unpinned := DecideProviderFallback(pf, false)
			assert.Equal(t, tc.wantRetryable, unpinned.Continue,
				"unpinned fallback must mirror retryability")
			pinned := DecideProviderFallback(pf, true)
			assert.False(t, pinned.Continue, "pinned worker must never widen its pin")
			assert.Equal(t, FallbackStopHardPinExhausted, pinned.StopReason)
		})
	}
}

// TestProviderFailureEvidenceNamesFallbackDecision (AC5, ddx-3b721804): the
// durable evidence must name the requested constraints, the resolved route (if
// any), the typed failure, retryability, fallback_attempted, and
// fallback_stop_reason.
func TestProviderFailureEvidenceNamesFallbackDecision(t *testing.T) {
	pf := providerFailure(FailureModeProviderRateLimit, true)
	req := ProviderFailureRequest{
		Harness:  "claude",
		Provider: "anthropic",
		Model:    "claude-opus",
		Profile:  "smart",
		MinPower: 40,
		MaxPower: 90,
	}
	resolved := &ResolvedRoute{Harness: "claude", Provider: "anthropic", Model: "claude-opus"}

	t.Run("unpinned_retryable_records_fallback_attempted", func(t *testing.T) {
		decision := DecideProviderFallback(pf, false)
		ev := BuildProviderFailureEvidence(req, resolved, pf, decision)

		assert.Equal(t, "claude", ev.RequestedHarness)
		assert.Equal(t, "anthropic", ev.RequestedProvider)
		assert.Equal(t, "claude-opus", ev.RequestedModel)
		assert.Equal(t, "smart", ev.RequestedProfile)
		assert.Equal(t, 40, ev.RequestedMinPower)
		assert.Equal(t, 90, ev.RequestedMaxPower)
		assert.Equal(t, "claude", ev.ResolvedHarness)
		assert.Equal(t, "anthropic", ev.ResolvedProvider)
		assert.Equal(t, "claude-opus", ev.ResolvedModel)
		assert.Equal(t, FailureModeProviderRateLimit, ev.TypedFailure)
		assert.True(t, ev.Retryable)
		assert.True(t, ev.FallbackAttempted)
		assert.Empty(t, ev.FallbackStopReason)

		// Every required field must survive serialization to durable evidence.
		raw, err := json.Marshal(ev)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(raw, &decoded))
		for _, key := range []string{
			"requested_harness", "requested_provider", "requested_model",
			"requested_profile", "requested_min_power", "requested_max_power",
			"resolved_harness", "resolved_provider", "resolved_model",
			"typed_failure", "retryable", "fallback_attempted",
		} {
			_, present := decoded[key]
			assert.Truef(t, present, "evidence must include %q", key)
		}
	})

	t.Run("pinned_records_hard_pin_stop_reason", func(t *testing.T) {
		decision := DecideProviderFallback(pf, true)
		ev := BuildProviderFailureEvidence(req, resolved, pf, decision)

		assert.False(t, ev.FallbackAttempted, "pinned worker does not attempt fallback")
		assert.Equal(t, FallbackStopHardPinExhausted, ev.FallbackStopReason)
	})

	t.Run("non_retryable_records_typed_stop_reason", func(t *testing.T) {
		nvp := providerFailure(FailureModeNoViableProvider, false)
		decision := DecideProviderFallback(nvp, false)
		ev := BuildProviderFailureEvidence(req, nil, nvp, decision)

		assert.False(t, ev.FallbackAttempted)
		assert.Equal(t, FailureModeNoViableProvider, ev.FallbackStopReason)
		assert.Empty(t, ev.ResolvedProvider, "no route resolved when there is no viable provider")
	})
}
