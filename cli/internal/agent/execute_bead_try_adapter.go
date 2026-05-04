package agent

import (
	"context"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
)

func toTryReport(report ExecuteBeadReport) agenttry.Report {
	return agenttry.Report{
		BeadID:                      report.BeadID,
		AttemptID:                   report.AttemptID,
		WorkerID:                    report.WorkerID,
		Harness:                     report.Harness,
		Provider:                    report.Provider,
		Model:                       report.Model,
		ActualPower:                 report.ActualPower,
		Status:                      report.Status,
		Detail:                      report.Detail,
		SessionID:                   report.SessionID,
		BaseRev:                     report.BaseRev,
		ResultRev:                   report.ResultRev,
		PreserveRef:                 report.PreserveRef,
		RetryAfter:                  report.RetryAfter,
		NoChangesRationale:          report.NoChangesRationale,
		ReviewVerdict:               report.ReviewVerdict,
		ReviewRationale:             report.ReviewRationale,
		Tier:                        report.Tier,
		ProbeResult:                 report.ProbeResult,
		CostUSD:                     report.CostUSD,
		DurationMS:                  report.DurationMS,
		RequestedProfile:            report.RequestedProfile,
		RequestedTier:               report.RequestedTier,
		ResolvedTier:                report.ResolvedTier,
		EscalationCount:             report.EscalationCount,
		FinalTier:                   report.FinalTier,
		DecompositionRecommendation: report.DecompositionRecommendation,
		DecompositionRationale:      report.DecompositionRationale,
		Disrupted:                   report.Disrupted,
		DisruptionReason:            report.DisruptionReason,
		OutcomeReason:               report.OutcomeReason,
	}
}

func fromTryReport(report agenttry.Report) ExecuteBeadReport {
	return ExecuteBeadReport{
		BeadID:                      report.BeadID,
		AttemptID:                   report.AttemptID,
		WorkerID:                    report.WorkerID,
		Harness:                     report.Harness,
		Provider:                    report.Provider,
		Model:                       report.Model,
		ActualPower:                 report.ActualPower,
		Status:                      report.Status,
		Detail:                      report.Detail,
		SessionID:                   report.SessionID,
		BaseRev:                     report.BaseRev,
		ResultRev:                   report.ResultRev,
		PreserveRef:                 report.PreserveRef,
		RetryAfter:                  report.RetryAfter,
		NoChangesRationale:          report.NoChangesRationale,
		ReviewVerdict:               report.ReviewVerdict,
		ReviewRationale:             report.ReviewRationale,
		Tier:                        report.Tier,
		ProbeResult:                 report.ProbeResult,
		CostUSD:                     report.CostUSD,
		DurationMS:                  report.DurationMS,
		RequestedProfile:            report.RequestedProfile,
		RequestedTier:               report.RequestedTier,
		ResolvedTier:                report.ResolvedTier,
		EscalationCount:             report.EscalationCount,
		FinalTier:                   report.FinalTier,
		DecompositionRecommendation: report.DecompositionRecommendation,
		DecompositionRationale:      report.DecompositionRationale,
		Disrupted:                   report.Disrupted,
		DisruptionReason:            report.DisruptionReason,
		OutcomeReason:               report.OutcomeReason,
	}
}

func tryAutoRecover(fn ConflictAutoRecoverFn) agenttry.ConflictAutoRecoverFn {
	return func(wd, preserveRef string) (string, error) {
		if fn == nil {
			fn = LandConflictAutoRecover
		}
		return fn(wd, preserveRef, RealLandingGitOps{})
	}
}

func tryExecutor(executor ExecuteBeadExecutor) agenttry.Executor {
	return agenttry.ExecutorFunc(func(ctx context.Context, beadID string) (agenttry.Report, error) {
		report, err := executor.Execute(ctx, beadID)
		return toTryReport(report), err
	})
}
