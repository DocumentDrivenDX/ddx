package agent

// provider_failure.go defines the typed provider-failure taxonomy at the
// DDx/Fizeau boundary (ddx-3b721804). A provider failure is any pre-dispatch
// Execute error or failed final event whose root cause is provider/model
// availability, credentials, quota/rate limiting, endpoint reachability, or
// harness/configuration validity — as opposed to a model giving up on the task.
//
// DDx is the abstraction that lets unattended workers continue through provider
// variance across machines. A generic, untyped provider failure that names no
// cause, no fallback action, and no route-health evidence is a product bug, so
// this file gives every such failure a typed reason, a retryability verdict, a
// fallback decision, and durable evidence.
//
// Routing itself stays in Fizeau (aliases, model discovery, provider catalog);
// DDx only classifies the outcome and decides whether an unpinned worker should
// fall back to another eligible route or a pinned worker should fail loudly.

import (
	"errors"
	"strings"

	agentlib "github.com/easel/fizeau"
)

// Provider-failure taxonomy. provider_connectivity and no_viable_provider reuse
// the existing FailureMode* constants (execute_bead_status.go) so the rest of
// the loop's route-health handling continues to recognize them.
const (
	FailureModeProviderAuth               = "provider_auth"
	FailureModeProviderRateLimit          = "provider_rate_limit"
	FailureModeProviderQuota              = "provider_quota"
	FailureModeProviderModelUnavailable   = "provider_model_unavailable"
	FailureModeProviderHarnessUnavailable = "provider_harness_unavailable"
	FailureModeProviderConfigInvalid      = "provider_config_invalid"
	FailureModeUnknownProviderFailure     = "unknown_provider_failure"
)

// ProviderFailure is the typed classification of a provider/route failure.
type ProviderFailure struct {
	// Reason is one of the taxonomy constants above (or the reused
	// FailureModeProviderConnectivity / FailureModeNoViableProvider).
	Reason string
	// Retryable reports whether a *different* eligible route could succeed.
	// True for transient/availability failures an unpinned worker should fall
	// back from; false for whole-fleet conditions (no viable provider) or
	// configuration bugs that another route cannot fix.
	Retryable bool
	// Disruption is the disruption_reason stamped on the report. A provider
	// failure is always a worker-side disruption (the model never got to give
	// up), so it mirrors Reason.
	Disruption string
}

// ProviderFailureError wraps a pre-dispatch Execute error with its typed
// classification so callers can recover the taxonomy via errors.As without
// re-parsing free text. The wrapped error's message is preserved verbatim.
type ProviderFailureError struct {
	Failure ProviderFailure
	Err     error
}

func (e *ProviderFailureError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ProviderFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ClassifyProviderFailure normalizes a free-text provider/route error into the
// typed taxonomy. It returns ok=false when the text does not name a recognized
// provider/route/config cause, so callers can leave non-provider failures
// (test_failure, merge_conflict, ...) classified by ClassifyFailureMode.
//
// The order is significant: config-invalid and auth/quota/rate-limit markers
// are checked before the broad connectivity bucket so a "429" or "unauthorized"
// is not swallowed as a generic timeout.
func ClassifyProviderFailure(errMsg string) (ProviderFailure, bool) {
	lower := strings.ToLower(strings.TrimSpace(errMsg))
	if lower == "" {
		return ProviderFailure{}, false
	}
	switch {
	case containsAny(lower,
		"passthrough constraint unsatisfiable",
		"passthrough constraint:",
		"invalid configuration",
		"config invalid",
		"invalid endpoint",
		"misconfigured",
		"unknown provider",
		"harness model incompatible",
		"model incompatible with",
		"incompatible with power bounds"):
		return providerFailure(FailureModeProviderConfigInvalid, false), true
	case containsAny(lower,
		"unauthorized",
		"401",
		"invalid api key",
		"invalid_api_key",
		"authentication failed",
		"authentication_error",
		"no api key",
		"missing api key",
		"forbidden",
		"403"):
		return providerFailure(FailureModeProviderAuth, true), true
	case containsAny(lower,
		"quota exceeded",
		"insufficient quota",
		"insufficient_quota",
		"out of quota",
		"quota"):
		return providerFailure(FailureModeProviderQuota, true), true
	case containsAny(lower,
		"rate limit",
		"rate_limit",
		"ratelimit",
		"too many requests",
		"429"):
		return providerFailure(FailureModeProviderRateLimit, true), true
	case containsAny(lower, "is not routable", "not routable", "model_not_found"),
		strings.Contains(lower, "model") && containsAny(lower,
			"not available",
			"unavailable",
			"not found",
			"no such",
			"unknown",
			"incompatible"):
		return providerFailure(FailureModeProviderModelUnavailable, true), true
	case containsAny(lower,
		"harness unavailable",
		"harness not available",
		"harness not installed",
		"unknown harness",
		"no harness configured",
		"executable file not found",
		"command not found"):
		return providerFailure(FailureModeProviderHarnessUnavailable, true), true
	case containsAny(lower,
		"no viable provider",
		"no viable harness",
		"no viable routing candidate",
		"no live provider supports",
		"no candidate satisfying local endpoint"):
		return providerFailure(FailureModeNoViableProvider, false), true
	case containsAny(lower,
		"connection refused",
		"connection reset",
		"no route to host",
		"network is unreachable",
		"no such host",
		"i/o timeout",
		"dial tcp",
		"provider_connectivity",
		"provider request timeout",
		"provider timeout",
		"endpoint timeout",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"502", "503", "504"):
		return providerFailure(FailureModeProviderConnectivity, true), true
	}
	return ProviderFailure{}, false
}

// ClassifyServiceExecuteError classifies a pre-dispatch FizeauService.Execute
// error. Because a pre-dispatch failure means routing never produced a viable
// dispatch, every such error is a provider-boundary failure: when no specific
// marker matches it falls back to unknown_provider_failure (non-retryable) so
// the outcome is always typed rather than generic execution_failed.
func ClassifyServiceExecuteError(err error) ProviderFailure {
	if err == nil {
		return providerFailure(FailureModeUnknownProviderFailure, false)
	}
	var modelErr *agentlib.ErrHarnessModelIncompatible
	if errors.As(err, &modelErr) {
		return providerFailure(FailureModeProviderModelUnavailable, true)
	}
	if pf, ok := ClassifyProviderFailure(err.Error()); ok {
		return pf
	}
	return providerFailure(FailureModeUnknownProviderFailure, false)
}

// ApplyProviderFailureToReport stamps a typed provider failure onto a report:
// the typed reason as outcome_reason and the worker-disruption markers so the
// retry/fallback layer treats it as route-health evidence, not a model give-up.
func ApplyProviderFailureToReport(report *ExecuteBeadReport, pf ProviderFailure) {
	if report == nil || pf.Reason == "" {
		return
	}
	report.OutcomeReason = pf.Reason
	report.Disrupted = true
	report.DisruptionReason = pf.Disruption
}

// IsProviderFailureReason reports whether reason is one of the typed
// provider-failure taxonomy values (including the reused connectivity /
// no-viable-provider reasons).
func IsProviderFailureReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case FailureModeProviderAuth,
		FailureModeProviderRateLimit,
		FailureModeProviderQuota,
		FailureModeProviderModelUnavailable,
		FailureModeProviderHarnessUnavailable,
		FailureModeProviderConfigInvalid,
		FailureModeUnknownProviderFailure,
		FailureModeProviderConnectivity,
		FailureModeNoViableProvider:
		return true
	default:
		return false
	}
}

// ProviderPin captures an operator's explicit routing pins. A pinned worker
// must never have its pin silently widened: a typed provider failure on a
// pinned route is reported as hard-pin-exhausted with operator remediation.
type ProviderPin struct {
	Harness  string
	Provider string
	Model    string
}

// Any reports whether the operator pinned any routing dimension.
func (p ProviderPin) Any() bool {
	return strings.TrimSpace(p.Harness) != "" ||
		strings.TrimSpace(p.Provider) != "" ||
		strings.TrimSpace(p.Model) != ""
}

func (p ProviderPin) describe() string {
	var parts []string
	if h := strings.TrimSpace(p.Harness); h != "" {
		parts = append(parts, "--harness "+h)
	}
	if pr := strings.TrimSpace(p.Provider); pr != "" {
		parts = append(parts, "--provider "+pr)
	}
	if m := strings.TrimSpace(p.Model); m != "" {
		parts = append(parts, "--model "+m)
	}
	return strings.Join(parts, " ")
}

// FallbackDecision records whether the worker should attempt another route and,
// when it should not, why fallback stopped. The StopReason feeds durable
// evidence (fallback_stop_reason).
type FallbackDecision struct {
	Continue   bool
	StopReason string
}

// Fallback stop reasons used in evidence.
const (
	FallbackStopHardPinExhausted = "hard_pin_exhausted"
)

// DecideProviderFallback decides what an worker does after a typed provider
// failure. A pinned worker never widens its pin, so any provider failure stops
// with hard_pin_exhausted. An unpinned worker continues when the failure is
// retryable (a different eligible route may succeed) and otherwise stops naming
// the typed reason (e.g. no_viable_provider — nothing left to try).
func DecideProviderFallback(pf ProviderFailure, pinned bool) FallbackDecision {
	if pinned {
		return FallbackDecision{Continue: false, StopReason: FallbackStopHardPinExhausted}
	}
	if pf.Retryable {
		return FallbackDecision{Continue: true}
	}
	return FallbackDecision{Continue: false, StopReason: pf.Reason}
}

// ProviderFailureEvidence is the durable record persisted on the bead/run when
// a provider failure occurs. It proves what was requested, what (if anything)
// resolved, the typed failure, its retryability, whether fallback was
// attempted, and why fallback stopped — so an operator reading the bead can see
// the full route-health decision without re-running the worker.
type ProviderFailureEvidence struct {
	RequestedHarness   string `json:"requested_harness,omitempty"`
	RequestedProvider  string `json:"requested_provider,omitempty"`
	RequestedModel     string `json:"requested_model,omitempty"`
	RequestedProfile   string `json:"requested_profile,omitempty"`
	RequestedMinPower  int    `json:"requested_min_power,omitempty"`
	RequestedMaxPower  int    `json:"requested_max_power,omitempty"`
	ResolvedHarness    string `json:"resolved_harness,omitempty"`
	ResolvedProvider   string `json:"resolved_provider,omitempty"`
	ResolvedModel      string `json:"resolved_model,omitempty"`
	TypedFailure       string `json:"typed_failure"`
	Retryable          bool   `json:"retryable"`
	FallbackAttempted  bool   `json:"fallback_attempted"`
	FallbackStopReason string `json:"fallback_stop_reason,omitempty"`
}

// ProviderFailureRequest captures the constraints the worker requested for the
// failed dispatch.
type ProviderFailureRequest struct {
	Harness  string
	Provider string
	Model    string
	Profile  string
	MinPower int
	MaxPower int
}

// ResolvedRoute captures the route Fizeau resolved, when any route resolved
// before the failure. Nil/zero means routing never produced a candidate.
type ResolvedRoute struct {
	Harness  string
	Provider string
	Model    string
}

// BuildProviderFailureEvidence assembles the durable evidence for a provider
// failure from the requested constraints, the resolved route (if any), the
// typed failure, and the fallback decision.
func BuildProviderFailureEvidence(req ProviderFailureRequest, resolved *ResolvedRoute, pf ProviderFailure, decision FallbackDecision) ProviderFailureEvidence {
	ev := ProviderFailureEvidence{
		RequestedHarness:   req.Harness,
		RequestedProvider:  req.Provider,
		RequestedModel:     req.Model,
		RequestedProfile:   req.Profile,
		RequestedMinPower:  req.MinPower,
		RequestedMaxPower:  req.MaxPower,
		TypedFailure:       pf.Reason,
		Retryable:          pf.Retryable,
		FallbackAttempted:  decision.Continue,
		FallbackStopReason: decision.StopReason,
	}
	if resolved != nil {
		ev.ResolvedHarness = resolved.Harness
		ev.ResolvedProvider = resolved.Provider
		ev.ResolvedModel = resolved.Model
	}
	return ev
}

// MarkHardPinExhausted refines a report for a pinned worker whose pinned route
// hit a typed provider failure. It preserves the pin (it never clears the pin
// or widens routing), records the typed failure as the outcome reason, and
// writes operator remediation naming the exact pin so the operator can act.
func MarkHardPinExhausted(report *ExecuteBeadReport, pin ProviderPin, pf ProviderFailure) {
	if report == nil || pf.Reason == "" {
		return
	}
	report.OutcomeReason = pf.Reason
	report.Disrupted = true
	report.DisruptionReason = pf.Disruption
	remediation := "hard pin exhausted: typed provider failure " + pf.Reason +
		" on pinned route (" + pin.describe() + "); the pin was not widened. " +
		"Remediation: fix the pinned provider/model/harness, or rerun without the pin to let DDx fall back to another eligible route."
	if strings.TrimSpace(report.Detail) == "" {
		report.Detail = remediation
	} else {
		report.Detail = report.Detail + "; " + remediation
	}
}

func providerFailure(reason string, retryable bool) ProviderFailure {
	return ProviderFailure{Reason: reason, Retryable: retryable, Disruption: reason}
}
