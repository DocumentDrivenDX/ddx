package agent

const (
	ExecuteBeadStatusStructuralValidationFailed = "structural_validation_failed"
	ExecuteBeadStatusExecutionFailed            = "execution_failed"
	ExecuteBeadStatusPostRunCheckFailed         = "post_run_check_failed"
	ExecuteBeadStatusLandConflict               = "land_conflict"
	ExecuteBeadStatusNoChanges                  = "no_changes"
	ExecuteBeadStatusSuccess                    = "success"
)

// ClassifyExecuteBeadStatus maps the execute-bead transport envelope to the
// supervisor-visible status contract. This keeps worker control flow off of
// free-form reason strings.
func ClassifyExecuteBeadStatus(outcome string, exitCode int, reason string) string {
	if exitCode != 0 {
		return ExecuteBeadStatusExecutionFailed
	}

	switch outcome {
	case "merged":
		return ExecuteBeadStatusSuccess
	case "no-changes":
		return ExecuteBeadStatusNoChanges
	case "preserved":
		switch reason {
		case "rebase failed", "ff-merge not possible", "ff-merge failed after rebase", "merge failed":
			return ExecuteBeadStatusLandConflict
		case "post-run checks failed":
			return ExecuteBeadStatusPostRunCheckFailed
		default:
			// Preserved iterations may still be success when the caller
			// explicitly requested no merge.
			return ExecuteBeadStatusSuccess
		}
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
