package cmd

import "github.com/DocumentDrivenDX/ddx/internal/agent"

// normalizeUnlandedAttemptResult translates typed worker outcomes only when
// the command layer has additional information to add. Repository revision
// equality alone is not an outcome: the worker distinguishes a justified
// no-change result from a run that produced no evidence at all.
func normalizeUnlandedAttemptResult(res *agent.ExecuteBeadResult) {
	if res == nil {
		return
	}

	switch {
	case res.Outcome == agent.ExecuteBeadOutcomeTaskFailed || res.ExitCode != 0:
		if res.ResultRev != "" && res.ResultRev != res.BaseRev {
			res.Outcome = "preserved"
		} else {
			res.Outcome = "error"
		}
		res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	case res.Outcome == agent.ExecuteBeadOutcomeTaskNoChanges:
		// Normalize the worker's typed, rationale-backed no-change result to the
		// command-level spelling. Do not infer this state from BaseRev/ResultRev.
		res.Outcome = "no-changes"
		res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	}
}
