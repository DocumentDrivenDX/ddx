package agent

import (
	"fmt"
)

// MarkResultExecutionError preserves a populated worker result as a structured
// attempt failure. Callers use this when ExecuteBeadWithConfig returned both a
// result and an error; the presence of the result means DDx has enough
// execution context to report the attempt instead of aborting the process.
func MarkResultExecutionError(res *ExecuteBeadResult, err error) {
	if res == nil || err == nil {
		return
	}
	if res.Outcome == "" {
		res.Outcome = ExecuteBeadOutcomeTaskFailed
	}
	if res.Reason == "" {
		res.Reason = err.Error()
	}
	if res.Error == "" {
		res.Error = err.Error()
	}
	if res.Status == "" || res.Status == ExecuteBeadStatusNoChanges {
		res.Status = ClassifyExecuteBeadStatus("error", res.ExitCode, res.Reason)
	}
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
	res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
	if res.FailureMode == "" {
		res.FailureMode = FailureModeUnknown
	}
}

// MarkResultLandError records a coordinator/landing failure on a completed
// worker result. When possible it first pins ResultRev under refs/ddx/iterations
// so the worker's commit remains reachable even though the target branch did
// not advance cleanly.
func MarkResultLandError(projectRoot string, res *ExecuteBeadResult, err error) {
	if res == nil || err == nil {
		return
	}
	reason := "land coordination failed: " + err.Error()
	if res.ResultRev != "" && res.ResultRev != res.BaseRev {
		preserveRef := landIterationRef(res.BeadID, res.AttemptID, res.BaseRev)
		if upErr := (RealLandingGitOps{}).UpdateRefTo(projectRoot, preserveRef, res.ResultRev, ""); upErr == nil {
			res.PreserveRef = preserveRef
		} else {
			reason = fmt.Sprintf("%s; preserving result failed: %v", reason, upErr)
		}
	}
	res.Outcome = "error"
	res.Reason = reason
	res.Error = err.Error()
	res.Status = ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	res.OrchestratorStatus = res.Status
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
	res.FailureMode = classifyLandingFailureMode(res.Outcome, res.Reason, res.GateResults, res.FailureMode)
}

// ReportFromExecuteBeadResult maps the detailed worker/orchestrator result
// into the loop-facing report shape used by ddx work and ddx try.
func ReportFromExecuteBeadResult(res *ExecuteBeadResult, powerClass string) ExecuteBeadReport {
	if res == nil {
		return ExecuteBeadReport{}
	}
	return ExecuteBeadReport{
		BeadID:                      res.BeadID,
		AttemptID:                   res.AttemptID,
		WorkerID:                    res.WorkerID,
		Harness:                     res.Harness,
		Provider:                    res.Provider,
		Model:                       res.Model,
		ActualPower:                 res.ActualPower,
		PredictedPower:              res.PredictedPower,
		PredictedSpeedTPS:           res.PredictedSpeedTPS,
		PredictedCostUSDPer1kTokens: res.PredictedCostUSDPer1kTokens,
		PredictedCostSource:         res.PredictedCostSource,
		Status:                      res.Status,
		Detail:                      res.Detail,
		Error:                       res.Error,
		Stderr:                      res.Stderr,
		RateLimitBudget:             res.RateLimitBudget,
		SessionID:                   res.SessionID,
		BaseRev:                     res.BaseRev,
		ResultRev:                   res.ResultRev,
		ImplementationRev:           res.ImplementationRev,
		LandedRev:                   res.LandedRev,
		TargetBranch:                res.TargetBranch,
		EvidenceRev:                 res.EvidenceRev,
		ProjectRoot:                 res.ProjectRoot,
		PreserveRef:                 res.PreserveRef,
		CandidateRef:                res.CandidateRef,
		CycleIndex:                  res.CycleIndex,
		NoChangesRationale:          res.NoChangesRationale,
		CycleTrace:                  append([]ExecutionCycleTrace(nil), res.CycleTrace...),
		PowerClass:                  powerClass,
		CostUSD:                     res.CostUSD,
		DurationMS:                  int64(res.DurationMS),
		ResourceExhausted:           res.ResourceExhausted,
		OutcomeReason:               res.FailureMode,
	}
}
