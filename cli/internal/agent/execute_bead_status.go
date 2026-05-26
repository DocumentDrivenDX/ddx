package agent

import "strings"

// OperatorCancelReason marks a landing preserved because the operator
// cancelled the attempt mid-flight via /api/beads/<id>/cancel. ADR-022
// §Cancel SLA: the worker aborts at the next safe point (between LLM turns /
// git ops) and emits a preserved_for_review result with this exact reason.
const OperatorCancelReason = "operator_cancel"

// PushFailedReasonPrefix is the canonical Reason prefix written by the
// landing layer when `git push` rejects the new tip after a local merge or
// fast-forward succeeded. ClassifyExecuteBeadStatus matches on this prefix
// to map a merged-locally-but-not-pushed outcome to push_failed instead of
// success, so the bead stays open.
const PushFailedReasonPrefix = "landed locally; push failed:"

// PushConflictReasonPrefix marks a more specific failure mode than
// PushFailedReasonPrefix: the local push was rejected because the remote
// advanced, the loop attempted an automatic pull/merge to recover, and the
// merge produced a conflict the loop cannot resolve. The bead is parked for
// human review with a short cooldown rather than a year-long park.
const PushConflictReasonPrefix = "landed locally; push conflict:"

// FailureMode constants classify *why* an execution did not land cleanly.
// They are orthogonal to Outcome/Status: a single failed attempt carries one
// outcome (e.g. preserved) and one failure mode (e.g. merge_conflict) so
// measurement surfaces can aggregate failures by cause.
const (
	FailureModeContextOverflow                = "context_overflow"
	FailureModeMergeConflict                  = "merge_conflict"
	FailureModeTestFailure                    = "test_failure"
	FailureModeBuildFailure                   = "build_failure"
	FailureModeTimeout                        = "timeout"
	FailureModeAuthError                      = "auth_error"
	FailureModeNoChanges                      = "no_changes"
	FailureModeNoEvidenceProduced             = "no_evidence_produced"
	FailureModeRatchetMiss                    = "ratchet_miss"
	FailureModeNoViableProvider               = "no_viable_provider"
	FailureModeProviderConnectivity           = "provider_connectivity"
	FailureModeHarnessNotInstalled            = "harness_not_installed"
	FailureModeBlockedByPassthroughConstraint = "blocked_by_passthrough_constraint"
	FailureModeAgentPowerUnsatisfied          = "agent_power_unsatisfied"
	FailureModeLockContention                 = "lock_contention"
	FailureModeWorktreeLost                   = "worktree_lost"
	// FailureModeRouteResolutionTimeout classifies an attempt that was parked
	// because route resolution / routing preflight exceeded the configured
	// route-resolution timeout. The lease is released and the bead is flagged
	// for operator attention rather than auto-retried (ddx-d8970a7b).
	FailureModeRouteResolutionTimeout = "route_resolution_timeout"
	// FailureModeProgressWatchdog classifies an attempt the progress watchdog
	// terminated because phase-empty heartbeats (harness/model/route all empty)
	// persisted past the phase budget. The worker appeared fresh to the
	// liveness TTL while making no forward progress, so the lease is released
	// and the bead is flagged for operator attention (ddx-dc23f001, parent
	// ddx-8f2e0ebf criterion B).
	FailureModeProgressWatchdog = "progress_watchdog"
	// FailureModeConsecutiveWedge classifies a bead the consecutive-wedge guard
	// stopped re-claiming because it wedged (route_resolution_timeout or
	// progress_watchdog) on consecutive claims up to the threshold. The bead is
	// parked to proposed for operator attention so the single-threaded worker
	// keeps draining the rest of the queue instead of re-wedging the same bead
	// indefinitely (ddx-9714eaac, parent ddx-8f2e0ebf criterion E).
	FailureModeConsecutiveWedge = "consecutive_wedge"
	FailureModeUnknown          = "unknown"
)

// ResourceExhaustedStopMessage is the operator-visible message emitted when
// execution must stop because the host cannot safely continue draining the
// queue.
const ResourceExhaustedStopMessage = "resource exhausted after cleanup; stopping work loop"

// ClassifyFailureMode derives a failure_mode for a worker-level result from
// its outcome, exit code, and error message. Returns "" when the outcome is
// task_succeeded (success carries no failure mode). task_no_changes always
// maps to FailureModeNoChanges regardless of error text.
//
// For task_failed (or non-zero exit code), the error message is scanned for
// well-known substrings. The order matters: context/auth/timeout patterns
// are checked before generic test/build patterns so a "test timed out"
// classifies as timeout rather than test_failure. Anything unrecognised
// maps to FailureModeUnknown so aggregates never hide a missed pattern.
func ClassifyFailureMode(outcome string, exitCode int, errMsg string) string {
	switch outcome {
	case ExecuteBeadOutcomeTaskSucceeded:
		if exitCode == 0 {
			return ""
		}
		// Degenerate: success outcome with non-zero exit. Treat as unknown
		// failure rather than success so the aggregate flags the anomaly.
	case ExecuteBeadOutcomeTaskNoChanges:
		return FailureModeNoChanges
	case ExecuteBeadOutcomeTaskNoEvidence:
		return FailureModeNoEvidenceProduced
	}

	lower := strings.ToLower(errMsg)
	switch {
	case IsLockContentionError(errMsg):
		return FailureModeLockContention
	case IsWorktreeLostError(errMsg):
		return FailureModeWorktreeLost
	case containsAny(lower,
		"passthrough constraint unsatisfiable",
		"passthrough constraint:",
		"max_power is less than min_power",
		"max_power exceeds min_power",
		"must be less than max_power",
		"no viable routing candidate for pins",
		"harness cannot satisfy power constraint",
		"harness pin incompatible with power bounds",
		"model pin incompatible with power bounds",
		"provider pin incompatible with power bounds"):
		return FailureModeBlockedByPassthroughConstraint
	case containsAny(lower,
		"agent power unsatisfied",
		"no model meets min_power",
		"no model with power >=",
		"minimum power not achievable",
		"min_power constraint cannot be satisfied"):
		return FailureModeAgentPowerUnsatisfied
	case containsAny(lower,
		"executable file not found in $path",
		"executable file not found",
		"no such file or directory") &&
		containsAny(lower, "exec:", "harness", "claude", "agent", "codex", "gemini"):
		return FailureModeHarnessNotInstalled
	case containsAny(lower,
		"no viable harness",
		"no harness configured",
		"failed to initialize routing service",
		"resolveroute:",
		"no viable routing candidate",
		"no live provider supports",
		"no candidate satisfying local endpoint",
		"no viable provider"):
		return FailureModeNoViableProvider
	case containsAny(lower,
		"context length",
		"context_length_exceeded",
		"context window",
		"maximum context",
		"prompt is too long",
		"prompt too long",
		"token limit",
		"too many tokens",
		"exceeds the context"):
		return FailureModeContextOverflow
	case containsAny(lower,
		"unauthorized",
		"401 unauthorized",
		"invalid api key",
		"invalid_api_key",
		"authentication failed",
		"authentication_error",
		"no api key",
		"missing api key",
		"quota exceeded",
		"insufficient quota",
		"insufficient_quota",
		"rate limit",
		"rate_limit",
		"429"):
		return FailureModeAuthError
	case containsAny(lower,
		"timeout",
		"timed out",
		"context deadline exceeded",
		"deadline exceeded",
		"signal: killed"):
		return FailureModeTimeout
	case containsAny(lower,
		"merge conflict",
		"ff-merge not possible",
		"merge failed",
		"automatic merge failed"):
		return FailureModeMergeConflict
	case containsAny(lower,
		"build failed",
		"compilation failed",
		"compile error",
		"compilation error",
		"cannot find package",
		"undefined:",
		"undefined reference",
		"syntax error"):
		return FailureModeBuildFailure
	case containsAny(lower,
		"test failed",
		"tests failed",
		"--- fail:",
		"assertion failed",
		"test failure"):
		return FailureModeTestFailure
	}
	if outcome == ExecuteBeadOutcomeTaskFailed && errMsg != "" {
		return FailureModeUnknown
	}
	if exitCode != 0 {
		return FailureModeUnknown
	}
	return ""
}

// IsWorktreeLostError reports whether errMsg describes the execute-bead
// infrastructure losing the isolated attempt worktree before DDx could inspect
// its HEAD. This deliberately requires both a HEAD/worktree signal and a
// missing-path signal so generic harness "no such file" errors do not get
// bucketed as infrastructure loss.
func IsWorktreeLostError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	hasHeadSignal := containsAny(lower,
		"failed to read worktree head",
		"git rev-parse head",
		"worktree path missing")
	hasMissingPathSignal := containsAny(lower,
		"chdir",
		"no such file or directory",
		"cannot access",
		"does not exist")
	return hasHeadSignal && hasMissingPathSignal
}

// containsAny reports whether s contains any of the given substrings. s is
// assumed to already be lower-cased by the caller.
func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if n == "" {
			continue
		}
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// classifyLandingFailureMode computes the final failure_mode for a result
// after the orchestrator has made its landing decision. It prefers
// landing-specific signals (merge conflict, gate failure) over the worker's
// classification and falls back to workerMode for worker-level failures
// (agent execution failed, context overflow, etc.). Returns "" when the
// bead merged cleanly.
func classifyLandingFailureMode(landingOutcome, landingReason string, gateResults []GateCheckResult, workerMode string) string {
	switch landingOutcome {
	case "merged":
		return ""
	case "no-changes", ExecuteBeadOutcomeTaskNoChanges:
		return FailureModeNoChanges
	case "no-evidence", ExecuteBeadOutcomeTaskNoEvidence:
		return FailureModeNoEvidenceProduced
	case "error":
		if workerMode != "" {
			return workerMode
		}
		return FailureModeUnknown
	case "preserved":
		switch {
		case landingReason == "merge conflict" || landingReason == "merge failed" || landingReason == "ff-merge not possible":
			return FailureModeMergeConflict
		case landingReason == "post-run checks failed", strings.HasPrefix(landingReason, PreMergeChecksReason):
			return classifyGateFailure(gateResults)
		case landingReason == RatchetPreserveReason:
			return FailureModeRatchetMiss
		case landingReason == OperatorCancelReason:
			return ""
		}
		// Other preserved reasons (e.g. "agent execution failed",
		// "--no-merge specified") defer to the worker's classification.
		if workerMode != "" {
			return workerMode
		}
		return FailureModeUnknown
	}
	if workerMode != "" {
		return workerMode
	}
	return FailureModeUnknown
}

// classifyGateFailure inspects failed gate results to distinguish
// build_failure from test_failure. Gate definitions that fail to compile
// before running (go build, tsc, cargo build) typically surface build
// diagnostics in stderr; test runners surface test failure markers. When
// ambiguous, defaults to test_failure since the post-run checks gate is
// test-oriented by convention.
func classifyGateFailure(gateResults []GateCheckResult) string {
	for _, gr := range gateResults {
		if gr.Status != "fail" {
			continue
		}
		combined := strings.ToLower(gr.Stdout + "\n" + gr.Stderr)
		if containsAny(combined,
			"build failed",
			"compilation failed",
			"compile error",
			"compilation error",
			"cannot find package",
			"undefined:",
			"undefined reference",
			"syntax error") {
			return FailureModeBuildFailure
		}
	}
	return FailureModeTestFailure
}

// Worker-level outcome constants — set by ExecuteBead() on the result.
// The parent orchestrator (LandBeadResult + ApplyLandingToResult) then
// overwrites Outcome and Status with the landing decision.
const (
	ExecuteBeadOutcomeTaskSucceeded  = "task_succeeded"
	ExecuteBeadOutcomeTaskFailed     = "task_failed"
	ExecuteBeadOutcomeTaskNoChanges  = "task_no_changes"
	ExecuteBeadOutcomeTaskNoEvidence = "task_no_evidence"
)

// Status constants — used by the loop and other consumers of ExecuteBeadReport.
// These map to specific close/unclaim behaviors in ExecuteBeadWorker.
const (
	ExecuteBeadStatusStructuralValidationFailed = "structural_validation_failed"
	ExecuteBeadStatusExecutionFailed            = "execution_failed"
	ExecuteBeadStatusPostRunCheckFailed         = "post_run_check_failed"
	ExecuteBeadStatusRatchetFailed              = "ratchet_failed"
	ExecuteBeadStatusLandConflict               = "land_conflict"
	ExecuteBeadStatusPushFailed                 = "push_failed"
	ExecuteBeadStatusPushConflict               = "push_conflict"
	ExecuteBeadStatusPreservedNeedsReview       = "preserved_needs_review"
	ExecuteBeadStatusNoEvidenceProduced         = "no_evidence_produced"
	ExecuteBeadStatusNoChanges                  = "no_changes"
	ExecuteBeadStatusAlreadySatisfied           = "already_satisfied"
	ExecuteBeadStatusSuccess                    = "success"
	ExecuteBeadStatusResourceExhausted          = "resource_exhausted"

	// ExecuteBeadStatusDeclinedNeedsDecomposition is a structured outcome
	// signalling that the executor inspected the bead and concluded it cannot
	// be delivered in a single pass — typically because the scope is
	// epic-sized and must first be broken into sub-beads. It is distinct
	// from no_changes (which means "I tried and there was nothing to change")
	// and from execution_failed (which means the agent or harness errored).
	//
	// The loop treats this status as terminal-for-loop: it parks the bead
	// with a long cooldown and records the recommended sub-beads as a
	// structured `decomposition-recommendation` event. No further attempts
	// happen until an operator clears the cooldown (e.g. via
	// `ddx bead cooldown clear`) or splits the bead.
	ExecuteBeadStatusDeclinedNeedsDecomposition = "declined_needs_decomposition"

	// Post-merge review outcomes. The bead was merged, then reviewed;
	// the review returned a non-APPROVE verdict and the bead was reopened.
	ExecuteBeadStatusReviewRequestChanges = "review_request_changes"
	ExecuteBeadStatusReviewBlock          = "review_block"
	ExecuteBeadStatusReviewMalfunction    = "review_malfunction"

	// ExecuteBeadStatusReviewRequestClarification is set when the reviewer
	// returns REQUEST_CLARIFICATION — it cannot adjudicate one or more
	// needs-judgment AC items without operator input. The bead moves to
	// status=proposed (operator lane) without blocking the drain queue.
	ExecuteBeadStatusReviewRequestClarification = "review_request_clarification"

	// ExecuteBeadStatusLandConflictUnresolvable is set when work
	// attempted 3-way ort auto-resolution and (if configured) a focused
	// conflict-resolve agent run for a preserved iteration, and both failed.
	// The bead is parked under LandConflictCooldown for operator review.
	ExecuteBeadStatusLandConflictUnresolvable = "land_conflict_unresolvable"

	// ExecuteBeadStatusLandConflictOperatorRequired is set when the focused
	// conflict-resolve agent escalated to BLOCKING — the conflict requires
	// operator judgment, not automated retry. The bead moves to status=proposed
	// with a kind:land-conflict-operator-required event.
	ExecuteBeadStatusLandConflictOperatorRequired = "land_conflict_operator_required"

	// ExecuteBeadStatusReviewTerminalBlock is set when a BLOCK review verdict
	// is classified as a terminal operator-required condition —
	// review_spec_gap, review_missing_acceptance, review_too_large at depth
	// cap, or review_unsafe_or_out_of_scope. These classes do not re-enter
	// the automated retry cycle and do not consume the no-progress retry
	// budget; the bead moves to status=proposed until an operator
	// intervenes.
	ExecuteBeadStatusReviewTerminalBlock = "review_terminal_block"

	// ExecuteBeadStatusReviewFixableGap is set when a BLOCK review verdict is
	// classified as a fixable gap — the issues are addressable by a repair
	// attempt at higher MinPower with the reviewer's rationale as context.
	// Exactly one repair cycle is scheduled per result_rev; a second
	// review_fixable_gap on the same result_rev falls through to the regular
	// BLOCK triage path instead.
	ExecuteBeadStatusReviewFixableGap = "review_fixable_gap"

	// ExecuteBeadStatusRepairCycleExhausted is set when the candidate-cycle
	// repair loop reaches its per-attempt repair_max_cycles limit before a
	// reviewer approves the candidate. The latest candidate ref is retained for
	// operator inspection.
	ExecuteBeadStatusRepairCycleExhausted = "repair-cycle-exhausted"
)

// ClassifyExecuteBeadStatus maps a landing outcome to the supervisor-visible
// status contract. This is called by ApplyLandingToResult and by callers who
// build an ExecuteBeadReport from a landing result.
func ClassifyExecuteBeadStatus(outcome string, exitCode int, reason string) string {
	if exitCode != 0 {
		return ExecuteBeadStatusExecutionFailed
	}

	switch outcome {
	case "merged":
		// A merged outcome whose reason carries the push-failure marker
		// means the local target branch advanced but `git push` rejected the
		// new tip (large blob, branch protection, auth, divergence, etc.).
		// Classify these as push_failed so the loop refuses to close the
		// bead and operators are forced to investigate.
		if strings.HasPrefix(reason, PushConflictReasonPrefix) {
			return ExecuteBeadStatusPushConflict
		}
		if strings.HasPrefix(reason, PushFailedReasonPrefix) {
			return ExecuteBeadStatusPushFailed
		}
		return ExecuteBeadStatusSuccess
	case "no-changes", ExecuteBeadOutcomeTaskNoChanges:
		return ExecuteBeadStatusNoChanges
	case "no-evidence", ExecuteBeadOutcomeTaskNoEvidence:
		return ExecuteBeadStatusNoEvidenceProduced
	case "error":
		return ExecuteBeadStatusExecutionFailed
	case "preserved":
		switch {
		case reason == "merge conflict" || reason == "merge failed" || reason == "ff-merge not possible":
			return ExecuteBeadStatusLandConflict
		case reason == "post-run checks failed", strings.HasPrefix(reason, PreMergeChecksReason):
			return ExecuteBeadStatusPostRunCheckFailed
		case reason == RatchetPreserveReason:
			return ExecuteBeadStatusRatchetFailed
		case reason == OperatorCancelReason:
			return ExecuteBeadStatusPreservedNeedsReview
		}
		if isPreservedNeedsReviewReason(reason) {
			return ExecuteBeadStatusPreservedNeedsReview
		}
		// Preserved iterations may still be success when the caller
		// explicitly requested no merge.
		return ExecuteBeadStatusSuccess
	case ExecuteBeadOutcomeTaskSucceeded:
		return ExecuteBeadStatusSuccess
	case ExecuteBeadOutcomeTaskFailed:
		return ExecuteBeadStatusExecutionFailed
	default:
		return ExecuteBeadStatusExecutionFailed
	}
}

// IsResourceExhaustedStatus reports whether the status is the stable
// host-resource failure classification.
func IsResourceExhaustedStatus(status string) bool {
	return status == ExecuteBeadStatusResourceExhausted
}

func isPreservedNeedsReviewReason(reason string) bool {
	for _, prefix := range []string{
		"evidence commit failed:",
		"large-deletion gate:",
		"syntax sanity gate:",
		"post-land gate failed:",
	} {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

func ExecuteBeadStatusDetail(status, reason, errMsg string) string {
	switch {
	case reason != "" && errMsg != "" && reason != errMsg:
		return reason + ": " + errMsg
	case reason != "":
		return reason
	case errMsg != "":
		return errMsg
	case status != "":
		return status
	default:
		return "unknown"
	}
}
