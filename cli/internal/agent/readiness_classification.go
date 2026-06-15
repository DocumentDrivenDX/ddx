package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

const (
	ReadinessClassificationReady            = "ready"
	ReadinessClassificationNeedsRefine      = "needs_refine"
	ReadinessClassificationNeedsSplit       = "needs_split"
	ReadinessClassificationOperatorRequired = "operator_required"
	ReadinessClassificationSystemUnready    = "system_unready"

	ReadinessReasonTooLarge                       = "too_large"
	ReadinessReasonAmbiguousScope                 = "ambiguous_scope"
	ReadinessReasonMissingRootCauseOrCurrentState = "missing_root_cause_or_current_state"
	ReadinessReasonMissingVerification            = "missing_verification"
	ReadinessReasonMissingCodePathAssertion       = "missing_code_path_assertion"
	ReadinessReasonMissingDependencyOrParent      = "missing_dependency_or_parent"
	ReadinessReasonHiddenExternalBlocker          = "hidden_external_blocker"
	ReadinessReasonAlreadySatisfiedCandidate      = "already_satisfied_candidate"
	ReadinessReasonSystemUnready                  = "system_unready"

	ReadinessSystemReasonUnavailable       = "unavailable"
	ReadinessSystemReasonRouting           = "routing"
	ReadinessSystemReasonQuota             = "quota"
	ReadinessSystemReasonTransport         = "transport"
	ReadinessSystemReasonResourceExhausted = "resource_exhausted"
	ReadinessSystemReasonRepoConcurrency   = "repo_concurrency"
	ReadinessSystemReasonTimeout           = "timeout"
	// ReadinessSystemReasonSchemaDrift marks readiness output that DDx could not
	// classify because the provider emitted a classification outside the canonical
	// enum. It is a provider/schema-drift signal, not a project-readiness blocker.
	ReadinessSystemReasonSchemaDrift = "schema_drift"
)

// AcceptedReadinessClassifications is the canonical readiness classification
// enum. It is surfaced verbatim in schema-drift diagnostics so an operator can
// see both what the provider emitted and what DDx accepts.
var AcceptedReadinessClassifications = []string{
	ReadinessClassificationReady,
	ReadinessClassificationNeedsRefine,
	ReadinessClassificationNeedsSplit,
	ReadinessClassificationOperatorRequired,
	ReadinessClassificationSystemUnready,
}

// schemaDriftDiagnostic builds an actionable provider/schema-drift message that
// names both the raw (unrecognized) classification and the accepted enum set so
// the failure is not mistaken for the bead itself being system-unready.
func schemaDriftDiagnostic(rawClassification, detail string) string {
	enum := strings.Join(AcceptedReadinessClassifications, ", ")
	if raw := strings.TrimSpace(rawClassification); raw != "" {
		return fmt.Sprintf(
			"provider/schema drift: readiness classification %q is not in the accepted enum (%s)",
			raw, enum,
		)
	}
	if d := strings.TrimSpace(detail); d != "" {
		return fmt.Sprintf(
			"provider/schema drift: %s; accepted classifications are %s",
			d, enum,
		)
	}
	return fmt.Sprintf(
		"provider/schema drift: readiness classification is not in the accepted enum (%s)",
		enum,
	)
}

// ReadinessClassificationResult is the deterministic bridge between the
// bead-lifecycle readiness vocabulary and work scheduling semantics.
type ReadinessClassificationResult struct {
	Classification       string
	Reason               string
	SystemReason         string
	TriageClassification string
	IntakeOutcome        PreClaimIntakeOutcome
	// Diagnostic carries human/operator-facing context that the bare
	// classification cannot. It is set for provider/schema-drift outcomes so an
	// unknown classification is recorded with the raw value and accepted enum set
	// instead of a generic system_unready project failure.
	Diagnostic string
}

// ClassifyReadiness maps bead-lifecycle readiness evidence to a stable result.
// Infrastructure signals always win over bead-quality signals so routing,
// quota, transport, resource, and repository-concurrency failures do not park a
// bead as needs_refine/needs_split.
func ClassifyReadiness(classification string, reasons []string, detail string) ReadinessClassificationResult {
	return ClassifyReadinessWithMode(classification, reasons, detail, config.BeadQualityModeWarnOnly)
}

// ClassifyReadinessWithMode maps bead-lifecycle readiness evidence to a stable
// result using the configured bead-quality mode.
func ClassifyReadinessWithMode(classification string, reasons []string, detail, qualityMode string) ReadinessClassificationResult {
	rawClassification := strings.TrimSpace(classification)
	classification = normalizeReadinessClassification(classification)
	reason := firstReadinessReason(reasons)
	systemReason := classifyReadinessSystemReason(detail, reasons)
	if classification == ReadinessClassificationSystemUnready ||
		reason == ReadinessReasonSystemUnready ||
		systemReason != "" {
		if systemReason == "" {
			systemReason = ReadinessSystemReasonUnavailable
		}
		result := ReadinessClassificationResult{
			Classification:       ReadinessClassificationSystemUnready,
			Reason:               ReadinessReasonSystemUnready,
			SystemReason:         systemReason,
			TriageClassification: triageClassificationForSystemReason(systemReason),
			IntakeOutcome:        PreClaimIntakeError,
		}
		if systemReason == ReadinessSystemReasonSchemaDrift {
			result.Diagnostic = schemaDriftDiagnostic("", detail)
		}
		return result
	}

	if fromReason := readinessClassificationForReason(reason); fromReason != "" {
		classification = fromReason
	}
	if classification == "" {
		classification = ReadinessClassificationReady
	}

	out := ReadinessClassificationResult{
		Classification: classification,
		Reason:         reason,
	}
	switch classification {
	case ReadinessClassificationReady:
		out.IntakeOutcome = PreClaimIntakeActionableAtomic
	case ReadinessClassificationNeedsSplit:
		if out.Reason == "" {
			out.Reason = ReadinessReasonTooLarge
		}
		out.IntakeOutcome = PreClaimIntakeTooLargeDecomposed
	case ReadinessClassificationOperatorRequired:
		if out.Reason == "" {
			out.Reason = ReadinessReasonAmbiguousScope
		}
		out.IntakeOutcome = PreClaimIntakeOperatorRequired
	case ReadinessClassificationNeedsRefine:
		if isReadinessBlockingMode(qualityMode) {
			// In BLOCK/factory mode, a refinement need still parks the bead.
			out.IntakeOutcome = PreClaimIntakeOperatorRequired
		} else {
			// WARN-ONLY mode reports the finding but still proceeds.
			out.IntakeOutcome = PreClaimIntakeActionableAtomic
		}
	default:
		// An unrecognized classification is provider/schema drift, not a project
		// readiness blocker. Record it as such with the raw value and accepted enum
		// set so the diagnostic is actionable instead of a generic system failure.
		out.Classification = ReadinessClassificationSystemUnready
		out.Reason = ReadinessReasonSystemUnready
		out.SystemReason = ReadinessSystemReasonSchemaDrift
		out.TriageClassification = triageClassificationForSystemReason(out.SystemReason)
		out.Diagnostic = schemaDriftDiagnostic(rawClassification, detail)
		out.IntakeOutcome = PreClaimIntakeError
	}
	return out
}

func isReadinessBlockingMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.BeadQualityModeBlock, "factory":
		return true
	default:
		return false
	}
}

func normalizeReadinessClassification(classification string) string {
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case "", "ready", "actionable_atomic", "atomic", "ok", "actionable", "pass":
		return ReadinessClassificationReady
	case "needs_refine", "refinable", "actionable_but_rewritten", "rewritten", "safely_refinable":
		return ReadinessClassificationNeedsRefine
	case "needs_split", "too_large", "too_large_decomposed", "decomposable", "split":
		return ReadinessClassificationNeedsSplit
	case "operator_required", "human_review_required", "ambiguous":
		return ReadinessClassificationOperatorRequired
	case "system_unready", "readiness_error", "intake_error":
		return ReadinessClassificationSystemUnready
	default:
		return strings.ToLower(strings.TrimSpace(classification))
	}
}

func normalizeReadinessReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case ReadinessReasonTooLarge,
		ReadinessReasonAmbiguousScope,
		ReadinessReasonMissingRootCauseOrCurrentState,
		ReadinessReasonMissingVerification,
		ReadinessReasonMissingCodePathAssertion,
		ReadinessReasonMissingDependencyOrParent,
		ReadinessReasonHiddenExternalBlocker,
		ReadinessReasonAlreadySatisfiedCandidate,
		ReadinessReasonSystemUnready:
		return strings.ToLower(strings.TrimSpace(reason))
	default:
		return strings.ToLower(strings.TrimSpace(reason))
	}
}

func firstReadinessReason(reasons []string) string {
	for _, reason := range reasons {
		reason = normalizeReadinessReason(reason)
		if reason != "" {
			return reason
		}
	}
	return ""
}

func readinessClassificationForReason(reason string) string {
	switch normalizeReadinessReason(reason) {
	case ReadinessReasonTooLarge:
		return ReadinessClassificationNeedsSplit
	case ReadinessReasonAmbiguousScope, ReadinessReasonHiddenExternalBlocker:
		return ReadinessClassificationOperatorRequired
	case ReadinessReasonMissingRootCauseOrCurrentState,
		ReadinessReasonMissingVerification,
		ReadinessReasonMissingCodePathAssertion,
		ReadinessReasonMissingDependencyOrParent,
		ReadinessReasonAlreadySatisfiedCandidate:
		return ReadinessClassificationNeedsRefine
	case ReadinessReasonSystemUnready:
		return ReadinessClassificationSystemUnready
	default:
		return ""
	}
}

func classifyReadinessSystemReason(detail string, reasons []string) string {
	combined := strings.ToLower(strings.Join(append([]string{detail}, reasons...), "\n"))
	combined = strings.TrimSpace(combined)
	if combined == "" {
		return ""
	}
	switch {
	case containsAny(combined,
		"unknown classification",
		"unknown readiness classification",
		"unexpected classification",
		"unrecognized classification"):
		return ReadinessSystemReasonSchemaDrift
	case IsLockContentionError(combined):
		return ReadinessSystemReasonRepoConcurrency
	case containsAny(combined,
		"resource_exhausted",
		"resource exhausted",
		"enospc",
		"no space left on device",
		"out of space",
		"disk quota exceeded",
		"temp root is full",
		"cannot allocate memory"):
		return ReadinessSystemReasonResourceExhausted
	case containsAny(combined, "worktree") && containsAny(combined,
		"failed",
		"lost",
		"missing",
		"cannot",
		"no such file",
		"not found",
		"no space"):
		return ReadinessSystemReasonResourceExhausted
	case containsAny(combined, "evidence") && containsAny(combined,
		"write",
		"persist",
		"commit",
		"bundle") && containsAny(combined,
		"failed",
		"no space",
		"permission denied",
		"i/o error"):
		return ReadinessSystemReasonResourceExhausted
	case containsAny(combined,
		"429",
		"too many requests",
		"rate limit",
		"rate_limit",
		"quota",
		"usage ceiling",
		"spend cap"):
		return ReadinessSystemReasonQuota
	case containsAny(combined,
		"timeout after",
		"timed out",
		"context deadline exceeded"):
		return ReadinessSystemReasonTimeout
	case isTransportError(errors.New(combined)):
		return ReadinessSystemReasonTransport
	case containsAny(combined,
		"resolveroute:",
		"no viable routing candidate",
		"no viable provider",
		"no live provider",
		"no candidate satisfying local endpoint",
		"no harness configured",
		"missing harness",
		"harness not installed",
		"unknown harness",
		"executable file not found"):
		return ReadinessSystemReasonRouting
	default:
		return ""
	}
}

func triageClassificationForSystemReason(systemReason string) string {
	switch systemReason {
	case ReadinessSystemReasonRouting:
		return "routing"
	case ReadinessSystemReasonQuota:
		return "quota"
	case ReadinessSystemReasonTransport:
		return "transport"
	case ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency,
		ReadinessSystemReasonTimeout,
		ReadinessSystemReasonSchemaDrift,
		ReadinessSystemReasonUnavailable:
		return "recoverable"
	default:
		return ""
	}
}

func isDeterministicSystemOutcomeReason(reason string) bool {
	switch reason {
	case "routing",
		"quota",
		"transport",
		"timeout",
		"recoverable",
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency,
		FailureModeNoViableProvider,
		FailureModeHarnessNotInstalled,
		FailureModeLockContention,
		FailureModeWorktreeLost:
		return true
	default:
		return false
	}
}
