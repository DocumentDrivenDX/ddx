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
	"syscall"
	"time"

	agentlib "github.com/easel/fizeau"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	powerladder "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/spf13/cobra"
)

const defaultWorktreeReapMaxAge = 72 * time.Hour

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
	preClaimTimeout, _ := cmd.Flags().GetDuration("preclaim-timeout")
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
		PreClaimTimeout:    executeloop.Duration{Duration: preClaimTimeout},
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
	autoInferPowerClass := noRoutingFlags && !projectHasRoutingConfig(projectRoot)

	if !spec.OpaquePassthrough {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, spec.Harness, spec.Model, spec.Provider); err != nil {
			return fmt.Errorf("work: %w", err)
		}
	}

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	jsonOutput := dispatch.JSON == "true"
	cleanupLog := cmd.ErrOrStderr()
	if jsonOutput {
		cleanupLog = io.Discard
	}
	cleanupRunner := newStartupHousekeepingRunner(projectRoot)
	if tryTargetBeadID == "" && spec.Mode != executeloop.ModeWatch {
		if breakdown, bErr := store.ReadyExecutionBreakdown(); bErr != nil {
			return bErr
		} else if len(breakdown.ExecutionReady) == 0 && len(breakdown.RetryCooldown) == 0 {
			if _, cleanupErr := cleanupRunner.Cleanup(cmd.Context()); cleanupErr != nil {
				return cleanupErr
			}
			return writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, agent.NewNoReadyWorkLoopResult(spec.Mode, breakdown), jsonOutput)
		}
	}

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
			BeadStore:   bead.NewStore(ddxroot.JoinProject(projectRoot)),
			BeadEvents:  bead.NewStore(ddxroot.JoinProject(projectRoot)),
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
	intakeLog := cmd.OutOrStdout()
	if dispatch.JSON == "true" {
		intakeLog = io.Discard
	}
	innerIntakeHook := agent.NewPreClaimIntakeHookWithLogVerbose(projectRoot, store, rcfg, nil, qualityRunner, intakeLog, f.viperInstance.GetBool("verbose"))
	intakeHook := agent.NewACQualityPreClaimGate(store, rcfg.BeadQualityMode(), rcfg.ACQualityMinScore(), innerIntakeHook)
	triageHook := agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	proseHook := f.proseEvidenceHookOverride
	if proseHook == nil {
		proseHook = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
			ProjectRoot: projectRoot,
			Events:      bead.NewStore(ddxroot.JoinProject(projectRoot)),
			Actor:       resolveClaimAssignee(),
			Source:      "ddx work",
		})
	}
	decompositionHook := agent.NewPreClaimDecompositionHook(store, qualityRunner, rcfg, projectRoot)
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
	profileSelector := newImplementationProfileSelector(projectRoot)
	accumulateBilledCost := func(report agent.ExecuteBeadReport) {
		costCap.Add(report.Harness, report.CostUSD)
	}
	costCapTripped := func() (work.StopDecision, agent.ExecuteBeadReport, bool) {
		if _, tripped := costCap.Tripped(); !tripped {
			return work.StopDecision{}, agent.ExecuteBeadReport{}, false
		}
		spent := costCap.Spent()
		decision, _ := work.ClassifyStop(work.StopInput{Budget: true})
		return decision, agent.ExecuteBeadReport{
			Status: agent.ExecuteBeadStatusExecutionFailed,
			Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, spec.MaxCostUSD),
		}, true
	}

	singlePolicyAttempt := func(ctx context.Context, beadID string, requestedMinPower int, requestedProfile string, inferredPolicy escalation.PowerClass, routingNote string, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
		gitOps := &agent.RealGitOps{}
		attemptProvider := spec.Provider
		if resolvedProvider != "" {
			attemptProvider = resolvedProvider
		}
		reportFromResult := func(res *agent.ExecuteBeadResult) agent.ExecuteBeadReport {
			report := agent.ReportFromExecuteBeadResult(res, string(inferredPolicy))
			report.InferredPowerClass = string(inferredPolicy)
			report.RequestedProfile = requestedProfile
			report.RoutingIntentNote = routingNote
			return report
		}
		loopOverrides := config.CLIOverrides{
			Harness:           resolvedHarness,
			Model:             resolvedModel,
			Provider:          attemptProvider,
			Profile:           requestedProfile,
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
			bead.NewStore(ddxroot.JoinProject(projectRoot)),
			f.AgentRunnerOverride,
			resourceChecker,
		), gitOps)
		if execErr != nil && res == nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		if res != nil && agent.IsResourceExhaustedStatus(res.Status) {
			return reportFromResult(res), nil
		}
		if execErr != nil {
			agent.MarkResultExecutionError(res, execErr)
			return reportFromResult(res), nil
		}
		if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
			targetBead, _ := store.Get(context.Background(), beadID)
			landRes, _, landErr := agent.SubmitWithPreMergeChecks(
				ctx, projectRoot, targetBead, res,
				func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
				bead.NewStore(ddxroot.JoinProject(projectRoot)),
				resolveClaimAssignee(), "ddx work",
				nil,
			)
			if landErr == nil {
				agent.ApplyLandResultToExecuteBeadResult(res, landRes)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
			} else {
				agent.MarkResultLandError(projectRoot, res, landErr)
				_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
				return reportFromResult(res), nil
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
		return reportFromResult(res), nil
	}

	var ladderOnce sync.Once
	var ladder escalationFloorFinder
	loadLadder := func() escalationFloorFinder {
		ladderOnce.Do(func() {
			ladder = powerladder.NewLadder(nil)
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
			ladder = powerladder.NewLadder(models)
		})
		return ladder
	}

	executor := f.tryExecutorOverride
	if executor == nil {
		executor = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			targetBead, err := store.Get(context.Background(), beadID)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			inferredPolicy := escalation.PowerClass("")
			if autoInferPowerClass {
				inferredPolicy = escalation.InferPowerClass(targetBead)
			}
			initialMinPower, unavailableReport, unavailable := investigationRetryInitialMinPowerWithInference(targetBead, rcfg.MinPower(), spec.MaxPower, loadLadder(), false)
			if unavailable {
				return unavailableReport, nil
			}
			if spec.Provider == "" && spec.Model == "" {
				if learnedMinPower, learnedUnavailableReport, learned := recentProviderConnectivityMinPower(store, time.Now().UTC(), initialMinPower, spec.MaxPower, loadLadder()); learned {
					if learnedUnavailableReport.Status != "" {
						learnedUnavailableReport.BeadID = beadID
						return learnedUnavailableReport, nil
					}
					if learnedMinPower > initialMinPower {
						initialMinPower = learnedMinPower
					}
				}
				// Pre-dispatch route-exclusion check: if every known route at
				// the requested power class is in the bead's failed-routes list
				// and still within the exclusion window, skip the dispatch and
				// escalate TriagePowerHintKey so the next attempt routes higher.
				if svcForExcl, svcExclErr := agent.ResolveServiceFromWorkDir(projectRoot); svcExclErr == nil {
					if exclusionReport, skip := agent.CheckAndApplyRouteExclusions(
						ctx, store, beadID, resolveClaimAssignee(),
						targetBead.Extra, time.Now().UTC(), initialMinPower,
						svcForExcl.ResolveRoute,
						func(p int) (int, error) { return nextEscalationFloor(loadLadder(), p) },
					); skip {
						return exclusionReport, nil
					}
				}
			}
			initialProfile := spec.Profile
			initialRoutingNote := ""
			if autoInferPowerClass {
				if selection, selectErr := profileSelector.Select(ctx, inferredPolicy, initialMinPower); selectErr == nil && selection.Name != "" {
					initialProfile = selection.Name
					initialRoutingNote = selection.Note
					if spec.MaxPower > 0 && initialMinPower > 0 && initialMinPower >= spec.MaxPower {
						unavailableReport := smartRouteUnavailableReport(targetBead, initialMinPower, spec.MaxPower, nil)
						unavailableReport.InferredPowerClass = string(inferredPolicy)
						unavailableReport.RequestedProfile = initialProfile
						unavailableReport.RoutingIntentNote = initialRoutingNote
						return unavailableReport, nil
					}
				} else {
					initialMinPower, unavailableReport, unavailable = zeroConfigInferredMinPower(targetBead, initialMinPower, spec.MaxPower, loadLadder())
					if unavailable {
						unavailableReport.InferredPowerClass = string(inferredPolicy)
						return unavailableReport, nil
					}
				}
			}
			perBeadBudget := spec.MaxBeadCostUSD
			if override, ok := escalation.ParseBeadBudgetLabel(targetBead.Labels); ok {
				perBeadBudget = override
			}
			perBeadTracker := escalation.NewPerBeadCostTracker(perBeadBudget, harnessBilledLookup)
			report, err := runEscalatingPowerAttempts(
				ctx,
				initialMinPower,
				loadLadder(),
				func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
					requestProfile := initialProfile
					routingNote := initialRoutingNote
					if autoInferPowerClass {
						if selection, selectErr := profileSelector.Select(ctx, inferredPolicy, requestedMinPower); selectErr == nil {
							// Empty selection means no no-extra-requirement policy covers this
							// retry floor. Drop the stale lower policy and send MinPower-only so
							// Fizeau can route or report no eligible candidate from live config.
							requestProfile = selection.Name
							if requestProfile == "" {
								routingNote = ""
							} else if selection.Note != "" {
								routingNote = selection.Note
							}
						}
					}
					return singlePolicyAttempt(ctx, beadID, requestedMinPower, requestProfile, inferredPolicy, routingNote, spec.Harness, spec.Provider, spec.Model)
				},
				nil,
				perBeadTracker,
			)
			if err == nil {
				accumulateBilledCost(report)
			}
			return report, err
		})
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Reviewer: reviewer,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return nextEscalationFloor(loadLadder(), actualPower)
		},
		Executor: executor,
	}

	cliLandingOps := agent.RealLandingGitOps{}
	progressLog := cmd.OutOrStdout()
	if jsonOutput {
		progressLog = io.Discard
	}
	result, err := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                         spec.Mode,
		IdleInterval:                 spec.IdleInterval.Duration,
		Log:                          progressLog,
		CleanupLog:                   cleanupLog,
		EventSink:                    loopSink,
		WorkerID:                     resolveClaimAssignee(),
		ProjectRoot:                  projectRoot,
		CleanupRunner:                cleanupRunner,
		ResourceChecker:              resourceChecker,
		BinaryRefreshCheck:           f.buildWorkBinaryRefreshCheck(cmd, projectRoot, tryTargetBeadID),
		SessionID:                    loopSessionID,
		PreClaimHook:                 buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreClaimIntakeHook:           intakeHook,
		PreClaimTimeout:              spec.PreClaimTimeout.Duration,
		PreDispatchLintHook:          lintHook,
		PostAttemptTriageHook:        triageHook,
		ProseEvidenceHook:            proseHook,
		PostAttemptDecompositionHook: decompositionHook,
		BudgetStop:                   costCapTripped,
		NoReview:                     spec.NoReview,
		TargetBeadID:                 tryTargetBeadID,
		ReviewCostCap:                costCap,
		FinalizeDurableAudit:         f.buildAttemptAuditFinalizer(projectRoot, store),
		PostLadderExhaustionHook:     recoveryHook,
	})
	if err != nil && result != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		_ = writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
	}
	if err != nil {
		return err
	}
	if result != nil && result.ExitReason == "binary_refresh" {
		return nil
	}
	return writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
}

func (f *CommandFactory) buildAttemptAuditFinalizer(projectRoot string, store *bead.Store) func(agent.ExecuteBeadReport) error {
	if f.durableAuditFinalizeOverride != nil {
		return f.durableAuditFinalizeOverride
	}
	return func(report agent.ExecuteBeadReport) error {
		return agent.FinalizeDurableAttemptAudit(projectRoot, store, report)
	}
}

func worktreeStillLive(meta agent.ExecutionCleanupMetadata, runStates []agent.RunState, now time.Time) bool {
	if meta.Preserved || meta.ActiveCandidateCycle || strings.TrimSpace(meta.CandidateRef) != "" {
		return true
	}
	if worktreeLivenessAlive(meta.Liveness, now) {
		return true
	}
	for _, state := range runStates {
		if !runStateMatchesWorktree(state, meta) {
			continue
		}
		if strings.TrimSpace(state.CandidateCyclePhase) != "" || strings.TrimSpace(state.CandidateRef) != "" {
			return true
		}
		if worktreeRunStateAlive(state, now) {
			return true
		}
	}
	return false
}

func worktreeLivenessAlive(liveness *agent.ExecutionCleanupLiveness, now time.Time) bool {
	if liveness == nil {
		return false
	}
	if processAlive(liveness.PID) {
		return true
	}
	if !liveness.ExpiresAt.IsZero() && now.Before(liveness.ExpiresAt) {
		return true
	}
	return !liveness.RefreshedAt.IsZero() && now.Sub(liveness.RefreshedAt) <= 2*time.Minute
}

func worktreeRunStateAlive(state agent.RunState, now time.Time) bool {
	if processAlive(state.PID) {
		return true
	}
	if !state.ExpiresAt.IsZero() && now.Before(state.ExpiresAt) {
		return true
	}
	return !state.RefreshedAt.IsZero() && now.Sub(state.RefreshedAt) <= agent.RunStateLivenessTTL
}

func runStateMatchesWorktree(state agent.RunState, meta agent.ExecutionCleanupMetadata) bool {
	if meta.AttemptID != "" && state.AttemptID == meta.AttemptID {
		return true
	}
	return meta.WorktreePath != "" &&
		state.WorktreePath != "" &&
		filepath.Clean(state.WorktreePath) == filepath.Clean(meta.WorktreePath)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func worktreeReapMaxAgeFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DDX_WORKTREE_REAP_MAX_AGE"))
	if raw == "" {
		return defaultWorktreeReapMaxAge
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultWorktreeReapMaxAge
	}
	return d
}

// projectHasRoutingConfig reports whether the project's .ddx/config.yaml pins a
// routing decision that should suppress DDx's zero-config powerClass inference.
// agent.endpoints alone is transport configuration (where providers live) and
// does NOT pin routing — leaving it on the suppression list caused no-flag work
// in projects with local endpoints to send an empty Policy and inherit
// Fizeau's default policy (which, with no DDx-supplied hint, scored Opus on
// ordinary implementation work; see ddx-e0b95b4a).
func projectHasRoutingConfig(projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	cfgPath := ddxroot.JoinProject(projectRoot, "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		return false
	}
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Agent == nil {
		return false
	}
	a := cfg.Agent
	return strings.TrimSpace(a.Model) != ""
}
