package agent

import "strings"

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
	FailureModeContextOverflow = "context_overflow"
	FailureModeMergeConflict   = "merge_conflict"
	FailureModeTestFailure     = "test_failure"
	FailureModeBuildFailure    = "build_failure"
	FailureModeTimeout         = "timeout"
	FailureModeAuthError       = "auth_error"
	FailureModeNoChanges       = "no_changes"
	FailureModeRatchetMiss     = "ratchet_miss"
	FailureModeUnknown         = "unknown"
)

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
	}

	lower := strings.ToLower(errMsg)
	switch {
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
	if exitCode != 0 {
		return FailureModeUnknown
	}
	return ""
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
	case "error":
		if workerMode != "" {
			return workerMode
		}
		return FailureModeUnknown
	case "preserved":
		switch landingReason {
		case "merge conflict", "merge failed", "ff-merge not possible":
			return FailureModeMergeConflict
		case "post-run checks failed":
			return classifyGateFailure(gateResults)
		case RatchetPreserveReason:
			return FailureModeRatchetMiss
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
	ExecuteBeadOutcomeTaskSucceeded = "task_succeeded"
	ExecuteBeadOutcomeTaskFailed    = "task_failed"
	ExecuteBeadOutcomeTaskNoChanges = "task_no_changes"
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
	ExecuteBeadStatusNoChanges                  = "no_changes"
	ExecuteBeadStatusAlreadySatisfied           = "already_satisfied"
	ExecuteBeadStatusSuccess                    = "success"

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
	case "error":
		return ExecuteBeadStatusExecutionFailed
	case "preserved":
		switch reason {
		case "merge conflict", "merge failed", "ff-merge not possible":
			return ExecuteBeadStatusLandConflict
		case "post-run checks failed":
			return ExecuteBeadStatusPostRunCheckFailed
		case RatchetPreserveReason:
			return ExecuteBeadStatusRatchetFailed
		default:
			// Preserved iterations may still be success when the caller
			// explicitly requested no merge.
			return ExecuteBeadStatusSuccess
		}
	case ExecuteBeadOutcomeTaskSucceeded:
		return ExecuteBeadStatusSuccess
	case ExecuteBeadOutcomeTaskFailed:
		return ExecuteBeadStatusExecutionFailed
	default:
		return ExecuteBeadStatusExecutionFailed
	}
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
