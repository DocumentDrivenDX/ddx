package cmd

import "github.com/DocumentDrivenDX/ddx/internal/agent"

// prepareCandidateCycleLanding returns true only when res may enter a CLI
// landing call site. A pre-land review/repair failure with a changed candidate
// is preserved without replacing its diagnostic status with a landing status.
func prepareCandidateCycleLanding(res *agent.ExecuteBeadResult) bool {
	if res == nil {
		return false
	}

	changedCandidate := res.ResultRev != "" && res.ResultRev != res.BaseRev
	if agent.IsCandidateCycleNonMergeable(res.Status) {
		if changedCandidate {
			res.Outcome = "preserved"
		}
		return false
	}

	return changedCandidate && res.ExitCode == 0
}
