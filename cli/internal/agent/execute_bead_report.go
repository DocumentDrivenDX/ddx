package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/trackerpaths"
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
	// A pre-dispatch provider-boundary failure carries its typed classification
	// on the error (ddx-3b721804). Prefer it so the report's outcome_reason is
	// the precise provider taxonomy (provider_auth, provider_model_unavailable,
	// ...) rather than the coarse worker-level bucket.
	var pfErr *ProviderFailureError
	if errors.As(err, &pfErr) && pfErr.Failure.Reason != "" {
		res.FailureMode = pfErr.Failure.Reason
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
	if markReconciledAlreadyLanded(projectRoot, res, err) {
		return
	}
	reason := "land coordination failed: " + err.Error()
	if action, ok := classifyLandCoordinationAction(err.Error()); ok {
		switch action {
		case landCoordinationActionRetryLand:
			reason = "land coordination retry: " + err.Error()
			preserveResultRefForLandError(projectRoot, res, &reason)
			res.Outcome = "land_coordination_retry"
			res.Reason = reason
			res.Error = err.Error()
			res.Status = ExecuteBeadStatusLandRetry
			res.OrchestratorStatus = res.Status
			res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
			res.FailureMode = FailureModeLandRetry
			return
		case landCoordinationActionOperatorAttention:
			reason = "land coordination operator attention: " + err.Error()
			preserveResultRefForLandError(projectRoot, res, &reason)
			res.Outcome = "land_coordination_operator_attention"
			res.Reason = reason
			res.Error = err.Error()
			res.Status = ExecuteBeadStatusLandOperatorAttention
			res.OrchestratorStatus = res.Status
			res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
			res.FailureMode = FailureModeLandOperatorAttention
			return
		}
	}
	preserveResultRefForLandError(projectRoot, res, &reason)
	res.Outcome = "error"
	res.Reason = reason
	res.Error = err.Error()
	res.Status = ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	res.OrchestratorStatus = res.Status
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
	res.FailureMode = classifyLandingFailureMode(res.Outcome, res.Reason, res.GateResults, res.FailureMode)
}

func preserveResultRefForLandError(projectRoot string, res *ExecuteBeadResult, reason *string) {
	if res == nil || res.ResultRev == "" || res.ResultRev == res.BaseRev {
		return
	}
	preserveRef := landIterationRef(res.BeadID, res.AttemptID, res.BaseRev)
	if upErr := (RealLandingGitOps{}).UpdateRefTo(projectRoot, preserveRef, res.ResultRev, ""); upErr == nil {
		res.PreserveRef = preserveRef
	} else if reason != nil {
		*reason = fmt.Sprintf("%s; preserving result failed: %v", *reason, upErr)
	}
}

type landCoordinationAction string

const (
	landCoordinationActionRetryLand         landCoordinationAction = "retry-land"
	landCoordinationActionOperatorAttention landCoordinationAction = "operator-attention"
)

func markReconciledAlreadyLanded(projectRoot string, res *ExecuteBeadResult, err error) bool {
	if projectRoot == "" || res == nil || res.ResultRev == "" || res.ResultRev == res.BaseRev || err == nil {
		return false
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cannot lock ref") {
		return false
	}
	targets := landCoordinationTargetRefs(err.Error())
	targets = append(targets, "HEAD")
	for _, target := range targets {
		tip, ok := resultReachableFromRef(projectRoot, res.ResultRev, target)
		if !ok {
			continue
		}
		if res.ImplementationRev == "" {
			res.ImplementationRev = res.ResultRev
		}
		res.ResultRev = tip
		res.LandedRev = tip
		res.Outcome = "merged"
		res.Reason = "land coordination reconciled: result already landed"
		res.Error = err.Error()
		res.Status = ExecuteBeadStatusSuccess
		res.OrchestratorStatus = res.Status
		res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
		res.FailureMode = ""
		return true
	}
	return false
}

func resultReachableFromRef(projectRoot, resultRev, ref string) (string, bool) {
	tipOut, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--verify", ref).Output()
	if err != nil {
		return "", false
	}
	tip := strings.TrimSpace(string(tipOut))
	if tip == "" {
		return "", false
	}
	err = internalgit.Command(context.Background(), projectRoot, "merge-base", "--is-ancestor", resultRev, tip).Run()
	return tip, err == nil
}

func landCoordinationTargetRefs(msg string) []string {
	var refs []string
	for _, token := range strings.FieldsFunc(msg, func(r rune) bool {
		return r == '\'' || r == '"' || r == ':' || r == ' ' || r == '\n' || r == '\t'
	}) {
		token = strings.TrimSpace(token)
		if strings.HasPrefix(token, "refs/heads/") {
			refs = append(refs, token)
		}
	}
	return refs
}

func classifyLandCoordinationAction(msg string) (landCoordinationAction, bool) {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "staged changes after waiting") {
		paths := stagedPathsFromLandError(msg)
		if len(paths) > 0 && allGeneratedLandCoordinationPaths(paths) {
			return landCoordinationActionRetryLand, true
		}
		return landCoordinationActionOperatorAttention, true
	}
	if strings.Contains(lower, "cannot lock ref") {
		return landCoordinationActionRetryLand, true
	}
	return "", false
}

func stagedPathsFromLandError(msg string) []string {
	lines := strings.Split(msg, "\n")
	if len(lines) <= 1 {
		return nil
	}
	var paths []string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if tab := strings.IndexByte(line, '\t'); tab >= 0 {
			line = line[tab+1:]
		}
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, filepath.ToSlash(p))
		}
	}
	return paths
}

func allGeneratedLandCoordinationPaths(paths []string) bool {
	for _, p := range paths {
		if trackerpaths.IsManagedTrackerPath(p) ||
			strings.HasPrefix(p, ".ddx/executions/") ||
			strings.HasPrefix(p, ".ddx/runs/") ||
			strings.HasPrefix(p, ".ddx/metrics/") ||
			strings.HasPrefix(p, ".ddx/attachments/") {
			continue
		}
		return false
	}
	return true
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
