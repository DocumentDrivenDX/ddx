package agent

import (
	"context"

	agenttry "github.com/DocumentDrivenDX/ddx/internal/agent/try"
)

func toTryReport(report ExecuteBeadReport) agenttry.Report {
	cycleTrace := make([]agenttry.ExecutionCycleTrace, 0, len(report.CycleTrace))
	for _, entry := range report.CycleTrace {
		cycleTrace = append(cycleTrace, toTryCycleTrace(entry))
	}
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
		ImplementationRev:           report.ImplementationRev,
		LandedRev:                   report.LandedRev,
		TargetBranch:                report.TargetBranch,
		EvidenceRev:                 report.EvidenceRev,
		ProjectRoot:                 report.ProjectRoot,
		PreserveRef:                 report.PreserveRef,
		RetryAfter:                  report.RetryAfter,
		NoChangesRationale:          report.NoChangesRationale,
		ReviewVerdict:               report.ReviewVerdict,
		ReviewRationale:             report.ReviewRationale,
		CycleTrace:                  cycleTrace,
		PowerClass:                  report.PowerClass,
		ProbeResult:                 report.ProbeResult,
		CostUSD:                     report.CostUSD,
		DurationMS:                  report.DurationMS,
		RequestedProfile:            report.RequestedProfile,
		RoutingIntentSource:         report.RoutingIntentSource,
		EstimatedDifficulty:         report.EstimatedDifficulty,
		InferredPowerClass:          report.InferredPowerClass,
		ResolvedPowerClass:          report.ResolvedPowerClass,
		EscalationCount:             report.EscalationCount,
		FinalPowerClass:             report.FinalPowerClass,
		DecompositionRecommendation: report.DecompositionRecommendation,
		DecompositionRationale:      report.DecompositionRationale,
		Disrupted:                   report.Disrupted,
		DisruptionReason:            report.DisruptionReason,
		OutcomeReason:               report.OutcomeReason,
		ResourceExhausted:           report.ResourceExhausted,
		Error:                       report.Error,
		Stderr:                      report.Stderr,
		RateLimitBudget:             report.RateLimitBudget,
	}
}

func fromTryReport(report agenttry.Report) ExecuteBeadReport {
	cycleTrace := make([]ExecutionCycleTrace, 0, len(report.CycleTrace))
	for _, entry := range report.CycleTrace {
		cycleTrace = append(cycleTrace, fromTryCycleTrace(entry))
	}
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
		ImplementationRev:           report.ImplementationRev,
		LandedRev:                   report.LandedRev,
		TargetBranch:                report.TargetBranch,
		EvidenceRev:                 report.EvidenceRev,
		ProjectRoot:                 report.ProjectRoot,
		PreserveRef:                 report.PreserveRef,
		RetryAfter:                  report.RetryAfter,
		NoChangesRationale:          report.NoChangesRationale,
		ReviewVerdict:               report.ReviewVerdict,
		ReviewRationale:             report.ReviewRationale,
		CycleTrace:                  cycleTrace,
		PowerClass:                  report.PowerClass,
		ProbeResult:                 report.ProbeResult,
		CostUSD:                     report.CostUSD,
		DurationMS:                  report.DurationMS,
		RequestedProfile:            report.RequestedProfile,
		RoutingIntentSource:         report.RoutingIntentSource,
		EstimatedDifficulty:         report.EstimatedDifficulty,
		InferredPowerClass:          report.InferredPowerClass,
		ResolvedPowerClass:          report.ResolvedPowerClass,
		EscalationCount:             report.EscalationCount,
		FinalPowerClass:             report.FinalPowerClass,
		DecompositionRecommendation: report.DecompositionRecommendation,
		DecompositionRationale:      report.DecompositionRationale,
		Disrupted:                   report.Disrupted,
		DisruptionReason:            report.DisruptionReason,
		OutcomeReason:               report.OutcomeReason,
		ResourceExhausted:           report.ResourceExhausted,
		Error:                       report.Error,
		Stderr:                      report.Stderr,
		RateLimitBudget:             report.RateLimitBudget,
	}
}

func fromTryRateLimitRetryInfo(info agenttry.RateLimitRetryInfo) RateLimitRetryInfo {
	var result *Result
	if info.Report != nil {
		result = &Result{
			Harness:  info.Report.Harness,
			Provider: info.Report.Provider,
			Model:    info.Report.Model,
		}
	}
	return RateLimitRetryInfo{
		Attempt:    info.Attempt,
		Wait:       info.Wait,
		Source:     info.Source,
		Result:     result,
		Elapsed:    info.Elapsed,
		OverBudget: info.OverBudget,
	}
}

func toTryCycleTrace(entry ExecutionCycleTrace) agenttry.ExecutionCycleTrace {
	return agenttry.ExecutionCycleTrace{
		CycleIndex: entry.CycleIndex,
		AttemptID:  entry.AttemptID,
		ResultRev:  entry.ResultRev,
		ImplementerRoute: agenttry.ExecutionCycleRouteFacts{
			Harness:         entry.ImplementerRoute.Harness,
			Provider:        entry.ImplementerRoute.Provider,
			Model:           entry.ImplementerRoute.Model,
			ActualPower:     entry.ImplementerRoute.ActualPower,
			RouteReason:     entry.ImplementerRoute.RouteReason,
			ResolvedBaseURL: entry.ImplementerRoute.ResolvedBaseURL,
		},
		RequestedRoute: agenttry.ExecutionCycleRequestedRouteFacts{
			Harness:             entry.RequestedRoute.Harness,
			Provider:            entry.RequestedRoute.Provider,
			Model:               entry.RequestedRoute.Model,
			Profile:             entry.RequestedRoute.Profile,
			RoutingIntentSource: entry.RequestedRoute.RoutingIntentSource,
			EstimatedDifficulty: entry.RequestedRoute.EstimatedDifficulty,
			InferredPowerClass:  entry.RequestedRoute.InferredPowerClass,
			RequestedPowerClass: entry.RequestedRoute.RequestedPowerClass,
		},
		ActualRoute: agenttry.ExecutionCycleRouteFacts{
			Harness:         entry.ActualRoute.Harness,
			Provider:        entry.ActualRoute.Provider,
			Model:           entry.ActualRoute.Model,
			ActualPower:     entry.ActualRoute.ActualPower,
			RouteReason:     entry.ActualRoute.RouteReason,
			ResolvedBaseURL: entry.ActualRoute.ResolvedBaseURL,
		},
		ReviewGroupID:   entry.ReviewGroupID,
		ReviewerIndices: append([]int(nil), entry.ReviewerIndices...),
		ReviewVerdicts:  append([]string(nil), entry.ReviewVerdicts...),
		ReviewResult: agenttry.ExecutionCycleReviewResult{
			Verdict:        entry.ReviewResult.Verdict,
			Rationale:      entry.ReviewResult.Rationale,
			Classification: entry.ReviewResult.Classification,
			PerAC:          toTryReviewACs(entry.ReviewResult.PerAC),
			Findings:       toTryFindings(entry.ReviewResult.Findings),
		},
		FinalDecision:    entry.FinalDecision,
		FailureClass:     entry.FailureClass,
		RetryAction:      entry.RetryAction,
		EscalationCount:  entry.EscalationCount,
		ReviewStatus:     entry.ReviewStatus,
		ReviewSkipReason: entry.ReviewSkipReason,
		LandStatus:       entry.LandStatus,
		ReconcileStatus:  entry.ReconcileStatus,
	}
}

func fromTryCycleTrace(entry agenttry.ExecutionCycleTrace) ExecutionCycleTrace {
	return ExecutionCycleTrace{
		CycleIndex: entry.CycleIndex,
		AttemptID:  entry.AttemptID,
		ResultRev:  entry.ResultRev,
		ImplementerRoute: ExecutionCycleRouteFacts{
			Harness:         entry.ImplementerRoute.Harness,
			Provider:        entry.ImplementerRoute.Provider,
			Model:           entry.ImplementerRoute.Model,
			ActualPower:     entry.ImplementerRoute.ActualPower,
			RouteReason:     entry.ImplementerRoute.RouteReason,
			ResolvedBaseURL: entry.ImplementerRoute.ResolvedBaseURL,
		},
		RequestedRoute: ExecutionCycleRequestedRouteFacts{
			Harness:             entry.RequestedRoute.Harness,
			Provider:            entry.RequestedRoute.Provider,
			Model:               entry.RequestedRoute.Model,
			Profile:             entry.RequestedRoute.Profile,
			RoutingIntentSource: entry.RequestedRoute.RoutingIntentSource,
			EstimatedDifficulty: entry.RequestedRoute.EstimatedDifficulty,
			InferredPowerClass:  entry.RequestedRoute.InferredPowerClass,
			RequestedPowerClass: entry.RequestedRoute.RequestedPowerClass,
		},
		ActualRoute: ExecutionCycleRouteFacts{
			Harness:         entry.ActualRoute.Harness,
			Provider:        entry.ActualRoute.Provider,
			Model:           entry.ActualRoute.Model,
			ActualPower:     entry.ActualRoute.ActualPower,
			RouteReason:     entry.ActualRoute.RouteReason,
			ResolvedBaseURL: entry.ActualRoute.ResolvedBaseURL,
		},
		ReviewGroupID:   entry.ReviewGroupID,
		ReviewerIndices: append([]int(nil), entry.ReviewerIndices...),
		ReviewVerdicts:  append([]string(nil), entry.ReviewVerdicts...),
		ReviewResult: ExecutionCycleReviewResult{
			Verdict:        entry.ReviewResult.Verdict,
			Rationale:      entry.ReviewResult.Rationale,
			Classification: entry.ReviewResult.Classification,
			PerAC:          fromTryReviewACs(entry.ReviewResult.PerAC),
			Findings:       fromTryFindings(entry.ReviewResult.Findings),
		},
		FinalDecision:    entry.FinalDecision,
		FailureClass:     entry.FailureClass,
		RetryAction:      entry.RetryAction,
		EscalationCount:  entry.EscalationCount,
		ReviewStatus:     entry.ReviewStatus,
		ReviewSkipReason: entry.ReviewSkipReason,
		LandStatus:       entry.LandStatus,
		ReconcileStatus:  entry.ReconcileStatus,
	}
}

func toTryReviewACs(items []ReviewAC) []agenttry.ReviewAC {
	if len(items) == 0 {
		return nil
	}
	out := make([]agenttry.ReviewAC, 0, len(items))
	for _, item := range items {
		out = append(out, agenttry.ReviewAC{
			Number:   item.Number,
			Item:     item.Item,
			Grade:    item.Grade,
			Evidence: item.Evidence,
		})
	}
	return out
}

func fromTryReviewACs(items []agenttry.ReviewAC) []ReviewAC {
	if len(items) == 0 {
		return nil
	}
	out := make([]ReviewAC, 0, len(items))
	for _, item := range items {
		out = append(out, ReviewAC{
			Number:   item.Number,
			Item:     item.Item,
			Grade:    item.Grade,
			Evidence: item.Evidence,
		})
	}
	return out
}

func toTryFindings(items []Finding) []agenttry.Finding {
	if len(items) == 0 {
		return nil
	}
	out := make([]agenttry.Finding, 0, len(items))
	for _, item := range items {
		out = append(out, agenttry.Finding{
			Severity: item.Severity,
			Summary:  item.Summary,
			Location: item.Location,
		})
	}
	return out
}

func fromTryFindings(items []agenttry.Finding) []Finding {
	if len(items) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(items))
	for _, item := range items {
		out = append(out, Finding{
			Severity: item.Severity,
			Summary:  item.Summary,
			Location: item.Location,
		})
	}
	return out
}

func tryAutoRecover(fn func(wd, preserveRef string, gitOps LandingGitOps) (string, error)) agenttry.ConflictAutoRecoverFn {
	return func(wd, preserveRef string) (string, error) {
		if fn == nil {
			fn = LandConflictAutoRecover
		}
		return fn(wd, preserveRef, RealLandingGitOps{})
	}
}

func tryExecutor(executor ExecuteBeadExecutor, onRouteResolved func(harness, provider, model string)) agenttry.Executor {
	return agenttry.ExecutorFunc(func(ctx context.Context, beadID string) (agenttry.Report, error) {
		if onRouteResolved != nil {
			ctx = contextWithOnRouteResolved(ctx, onRouteResolved)
		}
		report, err := executor.Execute(ctx, beadID)
		return toTryReport(report), err
	})
}
