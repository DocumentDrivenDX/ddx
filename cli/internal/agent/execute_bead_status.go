package agent

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
	ExecuteBeadStatusLandConflict               = "land_conflict"
	ExecuteBeadStatusNoChanges                  = "no_changes"
	ExecuteBeadStatusAlreadySatisfied           = "already_satisfied"
	ExecuteBeadStatusSuccess                    = "success"

	// Post-merge review outcomes. The bead was merged, then reviewed;
	// the review returned a non-APPROVE verdict and the bead was reopened.
	ExecuteBeadStatusReviewRequestChanges = "review_request_changes"
	ExecuteBeadStatusReviewBlock          = "review_block"
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
