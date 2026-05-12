package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

func hostnameOrEmpty() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

func parseExecuteLoopSpec(cmd *cobra.Command, treatPassthroughAsOpaque bool) (executeloop.ExecuteLoopSpec, executeloop.DispatchOptions, error) {
	projectRoot, _ := cmd.Flags().GetString("project")
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	provider, _ := cmd.Flags().GetString("provider")
	effort, _ := cmd.Flags().GetString("effort")
	once, _ := cmd.Flags().GetBool("once")
	watch, _ := cmd.Flags().GetBool("watch")
	rawIdleInterval, _ := cmd.Flags().GetDuration("idle-interval")
	asJSON, _ := cmd.Flags().GetBool("json")
	dispatchJSON := ""
	if asJSON {
		dispatchJSON = "true"
	}
	local, _ := cmd.Flags().GetBool("local")
	noReview, _ := cmd.Flags().GetBool("no-review")
	noReviewAck, _ := cmd.Flags().GetBool("no-review-i-know-what-im-doing")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")
	maxBeadCostUSD, _ := cmd.Flags().GetFloat64("max-bead-cost")
	maxRecoveryCostUSD, _ := cmd.Flags().GetFloat64("max-recovery-cost")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	rateLimitMaxWait, _ := cmd.Flags().GetDuration("rate-limit-max-wait")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	if noReview && !noReviewAck {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--no-review requires --no-review-i-know-what-im-doing (break-glass acknowledgement)")
	}
	if once && watch {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--once and --watch are mutually exclusive")
	}
	if !watch && cmd.Flags().Changed("idle-interval") {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--idle-interval only applies in watch mode; pass --watch")
	}

	mode := executeloop.ModeDrain
	var idleInterval executeloop.Duration
	if once {
		mode = executeloop.ModeOnce
	} else if watch {
		mode = executeloop.ModeWatch
		idleInterval = executeloop.Duration{Duration: rawIdleInterval}
	}

	spec := executeloop.ExecuteLoopSpec{
		ProjectRoot:        projectRoot,
		Harness:            harness,
		Model:              model,
		Profile:            profile,
		Provider:           provider,
		Effort:             effort,
		Mode:               mode,
		IdleInterval:       idleInterval,
		NoReview:           noReview,
		ReviewHarness:      reviewHarness,
		ReviewModel:        reviewModel,
		OpaquePassthrough:  treatPassthroughAsOpaque,
		MaxCostUSD:         maxCostUSD,
		MaxBeadCostUSD:     maxBeadCostUSD,
		MaxRecoveryCostUSD: maxRecoveryCostUSD,
		RequestTimeout:     executeloop.Duration{Duration: requestTimeout},
		RateLimitMaxWait:   executeloop.Duration{Duration: rateLimitMaxWait},
		MinPower:           minPower,
		MaxPower:           maxPower,
		FromRev:            fromRev,
	}
	spec.ApplyDefaults()
	if err := spec.Validate(); err != nil {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, err
	}

	return spec, executeloop.DispatchOptions{Local: local, JSON: dispatchJSON}, nil
}

func executeLoopAttemptRuntime(spec executeloop.ExecuteLoopSpec, output io.Writer, events agent.BeadEventAppender, runner agent.AgentRunner, checker agent.ExecutionResourceChecker) agent.ExecuteBeadRuntime {
	return agent.ExecuteBeadRuntime{
		FromRev:          spec.FromRev,
		Output:           output,
		BeadEvents:       events,
		AgentRunner:      runner,
		ResourceChecker:  checker,
		RateLimitMaxWait: spec.RateLimitMaxWait.Duration,
	}
}

func (f *CommandFactory) runAgentExecuteLoopImpl(cmd *cobra.Command, treatPassthroughAsOpaque bool, tryTargetBeadID string) error {
	spec, dispatch, err := parseExecuteLoopSpec(cmd, treatPassthroughAsOpaque)
	if err != nil {
		return err
	}
	projectRoot := resolveProjectRoot(spec.ProjectRoot, f.WorkingDir)
	spec.ProjectRoot = projectRoot
	if tryTargetBeadID != "" {
		spec.Mode = executeloop.ModeOnce
		spec.IdleInterval = executeloop.Duration{}
	}

	noRoutingFlags := spec.Harness == "" && spec.Model == "" && spec.Provider == "" &&
		spec.Profile == "" && spec.MinPower == 0 &&
		spec.MaxPower == 0 && !cmd.Flags().Changed("harness") &&
		!cmd.Flags().Changed("model") && !cmd.Flags().Changed("provider") &&
		!cmd.Flags().Changed("profile") &&
		!cmd.Flags().Changed("min-power") && !cmd.Flags().Changed("max-power")
	autoInferTier := noRoutingFlags && !projectHasRoutingConfig(projectRoot)

	if !spec.OpaquePassthrough {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, spec.Harness, spec.Model, spec.Provider); err != nil {
			return fmt.Errorf("execute-loop: %w", err)
		}
	}

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))

	loopSessionID := fmt.Sprintf("agent-loop-%d", time.Now().UnixNano())
	loopLogDir := filepath.Join(projectRoot, agent.DefaultLogDir)
	_ = os.MkdirAll(loopLogDir, 0o755)
	loopLogPath := filepath.Join(loopLogDir, loopSessionID+".jsonl")
	var loopSink io.Writer
	if lf, err := os.OpenFile(loopLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		loopSink = lf
		defer lf.Close() //nolint:errcheck
	}

	probeIdent := workerprobe.Identity{
		ProjectRoot:  projectRoot,
		Harness:      spec.Harness,
		Model:        spec.Model,
		ExecutorPID:  os.Getpid(),
		ExecutorHost: hostnameOrEmpty(),
		StartedAt:    time.Now().UTC(),
	}
	probe := workerprobe.New(probeIdent, workerprobe.Config{AddrFunc: serverpkg.ReadServerAddr})
	probeCtx, probeCancel := context.WithCancel(context.Background())
	probe.Start(probeCtx)
	defer func() {
		probeCancel()
		probe.Stop()
	}()
	if loopSink != nil {
		loopSink = workerprobe.TeeJSONL(loopSink, probe)
	} else {
		loopSink = workerprobe.TeeJSONL(io.Discard, probe)
	}

	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	var reviewer agent.BeadReviewer
	if !spec.NoReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			BeadEvents:  bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			Harness:     spec.ReviewHarness,
			Model:       spec.ReviewModel,
		}
	}

	spec.Profile = agent.NormalizeRoutingProfile(spec.Profile)

	overrides := config.CLIOverrides{
		Assignee:          resolveClaimAssignee(),
		Harness:           spec.Harness,
		Model:             spec.Model,
		Provider:          spec.Provider,
		Profile:           spec.Profile,
		Effort:            spec.Effort,
		MinPower:          spec.MinPower,
		MaxPower:          spec.MaxPower,
		OpaquePassthrough: spec.OpaquePassthrough,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return fmt.Errorf("load resolved config: %w", err)
	}

	resourceChecker := buildCLIResourceChecker(projectRoot, f.resourceCheckerOverride)
	if _, err := resourceChecker.Check(cmd.Context()); err != nil {
		var resourceErr *agent.ResourceExhaustedError
		if errors.As(err, &resourceErr) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), agent.ResourceExhaustedStopMessage)
		}
		return err
	}

	var qualityRunner agent.AgentRunner
	if f.AgentRunnerOverride != nil {
		qualityRunner = f.AgentRunnerOverride
	}
	lintHook := agent.NewPreDispatchLintHook(projectRoot, store, rcfg, nil, qualityRunner)
	innerIntakeHook := agent.NewPreClaimIntakeHook(projectRoot, store, rcfg, nil, qualityRunner)
	intakeHook := agent.NewACQualityPreClaimGate(store, rcfg.BeadQualityMode(), rcfg.ACQualityMinScore(), innerIntakeHook)
	triageHook := agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	recoveryHook := agent.NewAutoRecoveryPostLadderExhaustionHook(store, qualityRunner, rcfg, projectRoot, agent.AutoRecoveryConfig{
		MaxRecoveryCostUSD: spec.MaxRecoveryCostUSD,
		MaxBeadCostUSD:     spec.MaxBeadCostUSD,
	})

	harnessBilledLookup := func(harnessName string) bool {
		svc, svcErr := agent.NewServiceFromWorkDir(f.WorkingDir)
		if svcErr != nil {
			return true
		}
		infos, err := svc.ListHarnesses(context.Background())
		if err != nil {
			return true
		}
		for _, h := range infos {
			if h.Name == harnessName {
				return escalation.CountsTowardCostCap(h.CostClass == "local", h.Billing == agentlib.BillingModelSubscription, h.CostClass)
			}
		}
		return true
	}

	costCap := escalation.NewCostCapTracker(spec.MaxCostUSD, harnessBilledLookup)
	accumulateBilledCost := func(report agent.ExecuteBeadReport) {
		costCap.Add(report.Harness, report.CostUSD)
	}
	costCapTripped := func() (agent.ExecuteBeadReport, bool) {
		if _, tripped := costCap.Tripped(); !tripped {
			return agent.ExecuteBeadReport{}, false
		}
		spent := costCap.Spent()
		return agent.ExecuteBeadReport{
			Status: agent.ExecuteBeadStatusExecutionFailed,
			Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, spec.MaxCostUSD),
		}, true
	}

	singleTierAttempt := func(ctx context.Context, beadID string, requestedMinPower int, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
		gitOps := &agent.RealGitOps{}
		attemptProvider := spec.Provider
		if resolvedProvider != "" {
			attemptProvider = resolvedProvider
		}
		loopOverrides := config.CLIOverrides{
			Harness:           resolvedHarness,
			Model:             resolvedModel,
			Provider:          attemptProvider,
			Profile:           spec.Profile,
			Effort:            spec.Effort,
			MinPower:          requestedMinPower,
			MaxPower:          spec.MaxPower,
			OpaquePassthrough: spec.OpaquePassthrough,
		}
		requestTimeout := spec.RequestTimeout.Duration
		if requestTimeout > 0 {
			loopOverrides.ProviderRequestTimeout = &requestTimeout
		}
		attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
		res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, executeLoopAttemptRuntime(
			spec,
			cmd.OutOrStdout(),
			bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			f.AgentRunnerOverride,
			resourceChecker,
		), gitOps)
		if execErr != nil && res == nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		if res != nil && agent.IsResourceExhaustedStatus(res.Status) {
			return agent.ReportFromExecuteBeadResult(res, ""), nil
		}
		if execErr != nil {
			agent.MarkResultExecutionError(res, execErr)
			return agent.ReportFromExecuteBeadResult(res, ""), nil
		}
		if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
			targetBead, _ := store.Get(beadID)
			landRes, _, landErr := agent.SubmitWithPreMergeChecks(
				ctx, projectRoot, targetBead, res,
				func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
				bead.NewStore(filepath.Join(projectRoot, ".ddx")),
				resolveClaimAssignee(), "ddx work",
				nil,
			)
			if landErr == nil {
				agent.ApplyLandResultToExecuteBeadResult(res, landRes)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
			} else {
				agent.MarkResultLandError(projectRoot, res, landErr)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
				return agent.ReportFromExecuteBeadResult(res, ""), nil
			}
		} else if res != nil && (res.Outcome == agent.ExecuteBeadOutcomeTaskFailed || res.ExitCode != 0) {
			if res.ResultRev != "" && res.ResultRev != res.BaseRev {
				res.Outcome = "preserved"
			} else {
				res.Outcome = "error"
			}
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		} else if res != nil && res.ResultRev == res.BaseRev {
			res.Outcome = "no-changes"
			res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
		}
		return agent.ReportFromExecuteBeadResult(res, ""), nil
	}

	var ladderOnce sync.Once
	var ladder escalationFloorFinder
	loadLadder := func() escalationFloorFinder {
		ladderOnce.Do(func() {
			ladder = tierescalation.NewLadder(nil)
			svc, svcErr := agent.ResolveServiceFromWorkDir(projectRoot)
			if svcErr != nil {
				return
			}
			modelFilter := agentlib.ModelFilter{}
			if spec.Harness != "" {
				modelFilter.Harness = spec.Harness
			}
			modelCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
			defer cancel()
			models, listErr := svc.ListModels(modelCtx, modelFilter)
			if listErr != nil {
				return
			}
			ladder = tierescalation.NewLadder(models)
		})
		return ladder
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Reviewer: reviewer,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return nextEscalationFloor(loadLadder(), actualPower)
		},
		Executor: agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			targetBead, err := store.Get(beadID)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			initialMinPower, unavailableReport, unavailable := investigationRetryInitialMinPowerWithInference(targetBead, rcfg.MinPower(), spec.MaxPower, loadLadder(), autoInferTier)
			if unavailable {
				return unavailableReport, nil
			}
			perBeadBudget := spec.MaxBeadCostUSD
			if override, ok := escalation.ParseBeadBudgetLabel(targetBead.Labels); ok {
				perBeadBudget = override
			}
			perBeadTracker := escalation.NewPerBeadCostTracker(perBeadBudget, harnessBilledLookup)
			report, err := runEscalatingSingleTierAttempts(
				ctx,
				initialMinPower,
				loadLadder(),
				func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
					return singleTierAttempt(ctx, beadID, requestedMinPower, spec.Harness, spec.Provider, spec.Model)
				},
				nil,
				perBeadTracker,
			)
			if err == nil {
				accumulateBilledCost(report)
			}
			return report, err
		}),
	}

	cliLandingOps := agent.RealLandingGitOps{}
	progressLog := cmd.OutOrStdout()
	cleanupLog := cmd.ErrOrStderr()
	jsonOutput := dispatch.JSON == "true"
	if jsonOutput {
		progressLog = io.Discard
		cleanupLog = io.Discard
	}
	result, err := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                     spec.Mode,
		IdleInterval:             spec.IdleInterval.Duration,
		Log:                      progressLog,
		CleanupLog:               cleanupLog,
		EventSink:                loopSink,
		WorkerID:                 resolveClaimAssignee(),
		ProjectRoot:              projectRoot,
		CleanupRunner:            agent.NewExecutionCleanupManager(projectRoot, &agent.RealGitOps{}),
		ResourceChecker:          resourceChecker,
		SessionID:                loopSessionID,
		PreClaimHook:             buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreClaimIntakeHook:       intakeHook,
		PreDispatchLintHook:      lintHook,
		PostAttemptTriageHook:    triageHook,
		BudgetStop:               costCapTripped,
		NoReview:                 spec.NoReview,
		TargetBeadID:             tryTargetBeadID,
		ReviewCostCap:            costCap,
		OnAttemptFinalized:       buildAttemptMetricsHook(projectRoot, store, spec.Profile),
		PostLadderExhaustionHook: recoveryHook,
	})
	if err != nil && result != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		_ = writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
	}
	if err != nil {
		return err
	}
	return writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
}

func buildAttemptMetricsHook(projectRoot string, store *bead.Store, profile string) func(agent.ExecuteBeadReport) {
	return func(report agent.ExecuteBeadReport) {
		if report.AttemptID == "" {
			return
		}
		specID := ""
		if report.BeadID != "" && store != nil {
			if b, err := store.Get(report.BeadID); err == nil {
				specID, _ = b.Extra["spec-id"].(string)
			}
		}
		tsEnd := attemptmetrics.Rfc3339(time.Now().UTC())
		tsStart := ""
		if report.DurationMS > 0 {
			start := time.Now().UTC().Add(-time.Duration(report.DurationMS) * time.Millisecond)
			tsStart = attemptmetrics.Rfc3339(start)
		}
		row := attemptmetrics.AttemptRow{
			SchemaVersion: attemptmetrics.SchemaVersion,
			AttemptID:     report.AttemptID,
			BeadID:        report.BeadID,
			SessionID:     report.SessionID,
			TSStart:       tsStart,
			TSEnd:         tsEnd,
			Model:         report.Model,
			Harness:       report.Harness,
			Profile:       profile,
			Provider:      report.Provider,
			SpecID:        specID,
			Outcome:       report.Status,
			CostUSD:       report.CostUSD,
			DurationMS:    int(report.DurationMS),
			ReviewVerdict: report.ReviewVerdict,
		}
		_ = attemptmetrics.AppendRow(projectRoot, row)
	}
}

func projectHasRoutingConfig(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	cfgPath := filepath.Join(projectRoot, ".ddx", "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		return false
	}
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Agent == nil {
		return false
	}
	a := cfg.Agent
	if strings.TrimSpace(a.Model) != "" {
		return true
	}
	if len(a.Endpoints) > 0 {
		return true
	}
	return false
}
