package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceExecuteErrorClassifiesProviderConfigFailures (AC1, ddx-3b721804):
// a fake Fizeau service returning typed pre-dispatch Execute errors must yield
// a typed ExecuteBeadReport outcome_reason/disruption_reason rather than the
// generic execution_failed bucket.
func TestServiceExecuteErrorClassifiesProviderConfigFailures(t *testing.T) {
	cases := []struct {
		name          string
		execErr       error
		wantReason    string
		wantRetryable bool
	}{
		{
			name:          "model_unavailable_typed_fizeau_error",
			execErr:       &agentlib.ErrHarnessModelIncompatible{Harness: "claude", Model: "gpt-foo"},
			wantReason:    FailureModeProviderModelUnavailable,
			wantRetryable: true,
		},
		{
			name:          "no_viable_provider_for_now",
			execErr:       &agentlib.NoViableProviderForNow{RetryAfter: time.Unix(1_700_000_000, 0)},
			wantReason:    FailureModeNoViableProvider,
			wantRetryable: false,
		},
		{
			name:          "unsatisfiable_pin",
			execErr:       &agentlib.ErrUnsatisfiablePin{Pin: "--harness claude --provider anthropic"},
			wantReason:    FailureModeProviderConfigInvalid,
			wantRetryable: false,
		},
		{
			name: "rejected_override",
			execErr: &agentlib.ErrRejectedOverride{
				Inner: &agentlib.ErrUnsatisfiablePin{Pin: "--model claude-opus"},
			},
			wantReason:    FailureModeProviderConfigInvalid,
			wantRetryable: false,
		},
		{
			name:          "no_live_provider",
			execErr:       &agentlib.ErrNoLiveProvider{PromptTokens: 1024, RequiresTools: true, StartingPolicy: "smart"},
			wantReason:    FailureModeNoViableProvider,
			wantRetryable: false,
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

			pf, ok := ProviderFailureFromReason(tc.wantReason)
			require.True(t, ok, "typed reason %q must map to a provider failure", tc.wantReason)
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

// TestProviderFailureFromReason_TypedReasonsOnly (ddx-4f4d5a65): typed provider
// reasons must map to the retryability verdict without inspecting free text.
func TestProviderFailureFromReason_TypedReasonsOnly(t *testing.T) {
	for _, reason := range []string{
		FailureModeProviderAuth,
		FailureModeProviderRateLimit,
		FailureModeProviderQuota,
		FailureModeProviderModelUnavailable,
		FailureModeProviderHarnessUnavailable,
		FailureModeProviderConfigInvalid,
		FailureModeProviderConnectivity,
		FailureModeNoViableProvider,
		FailureModeUnknownProviderFailure,
	} {
		pf, ok := ProviderFailureFromReason(reason)
		require.Truef(t, ok, "typed reason %q must map to a provider failure", reason)
		assert.Equal(t, reason, pf.Reason)
		assert.Equal(t, reason, pf.Disruption)
	}
	pf, ok := ProviderFailureFromReason(FailureModeProviderAuth)
	require.True(t, ok)
	assert.True(t, pf.Retryable)
	pf, ok = ProviderFailureFromReason(FailureModeProviderConfigInvalid)
	require.True(t, ok)
	assert.False(t, pf.Retryable)
}

// TestLegacyProviderFailureClassificationPreserved (ddx-45f80b2f) proves that
// non-Fizeau legacy provider failure strings — plain errors carrying no typed
// Fizeau error value — still fall through ClassifyServiceExecuteError's
// free-text compatibility switch (which reuses ClassifyFailureMode) and drive
// the same DDx attempt decisions (retryability, fallback continuation, and
// hard-pin-exhausted stop reason) as before the typed Fizeau adapter existed.
// This guards against the typed-outcome refactor accidentally deleting or
// altering the legacy text tables that non-Fizeau callers still depend on.
func TestLegacyProviderFailureClassificationPreserved(t *testing.T) {
	cases := []struct {
		name          string
		errMsg        string
		wantReason    string
		wantRetryable bool
	}{
		{
			name:          "connection_refused_text",
			errMsg:        "dial tcp 10.0.0.1:443: connection refused",
			wantReason:    FailureModeProviderConnectivity,
			wantRetryable: true,
		},
		{
			name:          "gateway_timeout_text",
			errMsg:        "502 bad gateway",
			wantReason:    FailureModeProviderConnectivity,
			wantRetryable: true,
		},
		{
			name:          "no_viable_routing_candidate_text",
			errMsg:        "resolveRoute: no viable routing candidate in catalog",
			wantReason:    FailureModeNoViableProvider,
			wantRetryable: false,
		},
		{
			name:          "unrecognized_text_falls_back_to_unknown",
			errMsg:        "a completely novel provider hiccup no table recognizes",
			wantReason:    FailureModeUnknownProviderFailure,
			wantRetryable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf := ClassifyServiceExecuteError(errors.New(tc.errMsg))
			assert.Equal(t, tc.wantReason, pf.Reason)
			assert.Equal(t, tc.wantRetryable, pf.Retryable)

			unpinned := DecideProviderFallback(pf, false)
			assert.Equal(t, tc.wantRetryable, unpinned.Continue,
				"unpinned fallback must still mirror legacy retryability")

			pinned := DecideProviderFallback(pf, true)
			assert.False(t, pinned.Continue, "pinned worker must never widen its pin")
			assert.Equal(t, FallbackStopHardPinExhausted, pinned.StopReason)

			report := ExecuteBeadReport{Status: ExecuteBeadStatusExecutionFailed}
			ApplyProviderFailureToReport(&report, pf)
			assert.Equal(t, tc.wantReason, report.OutcomeReason)
			assert.True(t, report.Disrupted)
		})
	}
}

// TestTypedFizeauFailureBypassesLegacyTextTables (ddx-45f80b2f) proves that
// completed, retryable, unavailable-now, cancelled, and permanent typed
// Fizeau immediate outcomes drive AttemptPolicyDecisionForResult's Action and
// Reason solely from the typed FizeauOutcome/FizeauCause/FizeauStage tuple.
// Each case is re-run under several legacy Status/Error text variants that
// would classify to a different (or no) failure mode under
// ClassifyFailureMode if the typed decision ever fell back to scanning free
// text; the decision must stay identical across every variant.
func TestTypedFizeauFailureBypassesLegacyTextTables(t *testing.T) {
	cases := []struct {
		name       string
		outcome    agentlib.SessionOutcome
		cause      agentlib.TerminalCause
		stage      agentlib.SessionStage
		wantAction string
		wantReason string
	}{
		{
			name:       "completed",
			outcome:    agentlib.SessionOutcomeSuccess,
			cause:      agentlib.TerminalCauseCompleted,
			stage:      agentlib.SessionStageHarness,
			wantAction: "close",
			wantReason: "fizeau_outcome_success",
		},
		{
			name:       "retryable",
			outcome:    agentlib.SessionOutcomeFailed,
			cause:      agentlib.TerminalCauseProviderFailed,
			stage:      agentlib.SessionStageProvider,
			wantAction: "park",
			wantReason: "fizeau_terminal_retryable",
		},
		{
			name:       "unavailable_now",
			outcome:    agentlib.SessionOutcomeFailed,
			cause:      agentlib.TerminalCauseRouteUnavailable,
			stage:      agentlib.SessionStageProvider,
			wantAction: "park",
			wantReason: "fizeau_terminal_unavailable_now",
		},
		{
			name:       "cancelled",
			outcome:    agentlib.SessionOutcomeCancelled,
			cause:      agentlib.TerminalCauseCallerDied,
			stage:      agentlib.SessionStageHarness,
			wantAction: "park",
			wantReason: "fizeau_outcome_cancelled",
		},
		{
			name:       "permanent",
			outcome:    agentlib.SessionOutcomeFailed,
			cause:      agentlib.TerminalCauseInternalError,
			stage:      agentlib.SessionStageCleanup,
			wantAction: "park",
			wantReason: "fizeau_terminal_permanent_failure",
		},
	}

	// Status/Error variants that would classify to different (or no) failure
	// modes under ClassifyFailureMode if AttemptPolicyDecisionForResult ever
	// fell back to scanning them instead of the typed lifecycle tuple.
	legacyTextVariants := []struct {
		name   string
		status string
		errMsg string
	}{
		{name: "empty", status: "", errMsg: ""},
		{name: "success_status_no_error", status: ExecuteBeadStatusSuccess, errMsg: ""},
		{name: "timeout_text", status: ExecuteBeadStatusExecutionFailed, errMsg: "context deadline exceeded"},
		{name: "auth_error_text", status: ExecuteBeadStatusExecutionFailed, errMsg: "401 unauthorized: invalid api key"},
		{name: "merge_conflict_text", status: ExecuteBeadStatusLandConflict, errMsg: "merge conflict"},
		{name: "no_changes_status", status: ExecuteBeadStatusNoChanges, errMsg: "unrelated diagnostic"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotAction, gotReason string
			for i, variant := range legacyTextVariants {
				t.Run(variant.name, func(t *testing.T) {
					res := ExecuteBeadResult{
						FizeauOutcome: string(tc.outcome),
						FizeauCause:   string(tc.cause),
						FizeauStage:   string(tc.stage),
						Status:        variant.status,
						Error:         variant.errMsg,
					}

					decision := AttemptPolicyDecisionForResult(&res)
					if string(decision.Action) != tc.wantAction {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s).Action = %q, want %q", tc.name, variant.name, decision.Action, tc.wantAction)
					}
					if decision.Reason != tc.wantReason {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s).Reason = %q, want %q", tc.name, variant.name, decision.Reason, tc.wantReason)
					}
					if i == 0 {
						gotAction, gotReason = string(decision.Action), decision.Reason
						return
					}
					if string(decision.Action) != gotAction || decision.Reason != gotReason {
						t.Fatalf("AttemptPolicyDecisionForResult(%s/%s) diverged across legacy text variants: got %s/%s, want %s/%s",
							tc.name, variant.name, decision.Action, decision.Reason, gotAction, gotReason)
					}
				})
			}
		})
	}
}

func TestProviderFailureTextClassifierRemovedOrAuditOnly(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(file), "provider_failure.go")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	for _, needle := range []string{
		"containsAny(lower,",
		"oauth session expired",
		"429 Too Many Requests",
		"connection refused",
		"provider request timeout",
		"ClassifyProviderFailure(err.Error())",
	} {
		assert.NotContains(t, content, needle, "provider_failure.go must not scrape free-text provider errors")
	}
}
