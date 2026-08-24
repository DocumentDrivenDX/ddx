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

	"github.com/DocumentDrivenDX/ddx/internal/agent/failclass"
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

// ProviderFailureFromReason converts a typed provider-failure reason into the
// corresponding retryability verdict. It does not inspect free-text error
// messages.
func ProviderFailureFromReason(reason string) (ProviderFailure, bool) {
	switch strings.TrimSpace(reason) {
	case FailureModeProviderAuth:
		return providerFailure(FailureModeProviderAuth, true), true
	case FailureModeProviderRateLimit:
		return providerFailure(FailureModeProviderRateLimit, true), true
	case FailureModeProviderQuota:
		return providerFailure(FailureModeProviderQuota, true), true
	case FailureModeProviderModelUnavailable:
		return providerFailure(FailureModeProviderModelUnavailable, true), true
	case FailureModeProviderHarnessUnavailable:
		return providerFailure(FailureModeProviderHarnessUnavailable, true), true
	case FailureModeProviderConfigInvalid:
		return providerFailure(FailureModeProviderConfigInvalid, false), true
	case FailureModeProviderConnectivity:
		return providerFailure(FailureModeProviderConnectivity, true), true
	case FailureModeNoViableProvider:
		return providerFailure(FailureModeNoViableProvider, false), true
	case FailureModeUnknownProviderFailure:
		return providerFailure(FailureModeUnknownProviderFailure, false), true
	default:
		return ProviderFailure{}, false
	}
}

// ClassifyServiceExecuteError classifies a pre-dispatch FizeauService.Execute
// error using typed upstream error values only. A pre-dispatch failure means
// routing never produced a viable dispatch, so typed provider errors are
// surfaced as provider failures and everything else falls back to
// unknown_provider_failure without re-parsing free text.
func ClassifyServiceExecuteError(err error) ProviderFailure {
	if err == nil {
		return providerFailure(FailureModeUnknownProviderFailure, false)
	}
	// The typed attempt-policy adapter (internal/agent/failclass) is the
	// single decision source for the transient (unavailable-now / retryable)
	// typed Fizeau immediate lifecycle classes, so provider_failure.go never
	// re-derives their retryability itself (ddx-47e99eb6, WB-1). Completed,
	// cancelled, and permanent typed immediate errors are unaffected and stay
	// on the local classification below.
	if pf, ok := providerFailureForTransientImmediateError(err); ok {
		return pf
	}
	var modelErr *agentlib.ErrHarnessModelIncompatible
	if errors.As(err, &modelErr) {
		return providerFailure(FailureModeProviderModelUnavailable, true)
	}
	var unsatPin *agentlib.ErrUnsatisfiablePin
	if errors.As(err, &unsatPin) {
		return providerFailure(FailureModeProviderConfigInvalid, false)
	}
	var rejectedOverride *agentlib.ErrRejectedOverride
	if errors.As(err, &rejectedOverride) {
		return providerFailure(FailureModeProviderConfigInvalid, false)
	}
	// Typed Fizeau errors own the taxonomy. Free-text pre-dispatch errors
	// still map the connectivity/transport diagnostics that unpinned
	// workers must retry; other free-text (missing executable, etc.) stays
	// unknown so we do not scrape harness-unavailable from slogans.
	switch ClassifyFailureMode(ExecuteBeadStatusExecutionFailed, 1, err.Error()) {
	case FailureModeProviderConnectivity, FailureModeServerUnavailable:
		return providerFailure(FailureModeProviderConnectivity, true)
	case FailureModeNoViableProvider:
		return providerFailure(FailureModeNoViableProvider, false)
	default:
		return providerFailure(FailureModeUnknownProviderFailure, false)
	}
}

// providerFailureForTransientImmediateError recognizes the two typed Fizeau
// immediate errors the attempt-policy adapter classifies into a transient
// lifecycle — *agentlib.NoViableProviderForNow (unavailable-now) and
// *agentlib.ErrNoLiveProvider (retryable) — and returns the ProviderFailure
// derived exactly from the adapter's decision. The DDx-owned evidence passed
// in is NewAttemptRetryAllowed: a pre-dispatch failure never reached a
// workspace, so "retry a new attempt at a different eligible route" is the
// only DDx-owned evidence this boundary can supply; current-attempt repair,
// land-readiness, and minimum-strength evidence belong to later stages. The
// reused FailureModeNoViableProvider reason is preserved for both so existing
// report/evidence/MET-003 consumers keep recognizing the taxonomy value;
// only Retryable is sourced from the adapter, and it is read once, directly
// off the returned Action, with no secondary policy remapping.
func providerFailureForTransientImmediateError(err error) (ProviderFailure, bool) {
	var noViableForNow *agentlib.NoViableProviderForNow
	var noLiveProvider *agentlib.ErrNoLiveProvider
	if !errors.As(err, &noViableForNow) && !errors.As(err, &noLiveProvider) {
		return ProviderFailure{}, false
	}
	decision := failclass.DecideAttemptPolicy(failclass.AttemptPolicyInput{
		ImmediateErr: err,
		Evidence:     failclass.AttemptPolicyEvidence{NewAttemptRetryAllowed: true},
	})
	retryable := decision.Action != failclass.AttemptPolicyActionPark
	return providerFailure(FailureModeNoViableProvider, retryable), true
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
	_, ok := ProviderFailureFromReason(reason)
	return ok
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
