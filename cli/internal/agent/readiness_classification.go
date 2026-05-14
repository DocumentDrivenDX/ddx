package agent

import (
	"errors"
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
)

// ReadinessClassificationResult is the deterministic bridge between the
// bead-lifecycle readiness vocabulary and work scheduling semantics.
type ReadinessClassificationResult struct {
	Classification       string
	Reason               string
	SystemReason         string
	TriageClassification string
	IntakeOutcome        PreClaimIntakeOutcome
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
	classification = normalizeReadinessClassification(classification)
	reason := firstReadinessReason(reasons)
	systemReason := classifyReadinessSystemReason(detail, reasons)
	if classification == ReadinessClassificationSystemUnready ||
		reason == ReadinessReasonSystemUnready ||
		systemReason != "" {
		if systemReason == "" {
			systemReason = ReadinessSystemReasonUnavailable
		}
		return ReadinessClassificationResult{
			Classification:       ReadinessClassificationSystemUnready,
			Reason:               ReadinessReasonSystemUnready,
			SystemReason:         systemReason,
			TriageClassification: triageClassificationForSystemReason(systemReason),
			IntakeOutcome:        PreClaimIntakeError,
		}
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
		out.Classification = ReadinessClassificationSystemUnready
		out.Reason = ReadinessReasonSystemUnready
		out.SystemReason = ReadinessSystemReasonUnavailable
		out.TriageClassification = triageClassificationForSystemReason(out.SystemReason)
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
	case "needs_refine", "actionable_but_rewritten", "rewritten":
		return ReadinessClassificationNeedsRefine
	case "needs_split", "too_large", "too_large_decomposed", "decomposable":
		return ReadinessClassificationNeedsSplit
	case "operator_required", "human_review_required":
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
