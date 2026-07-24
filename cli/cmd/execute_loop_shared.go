package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
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
	labelFilter, _ := cmd.Flags().GetString("label-filter")
	effort, _ := cmd.Flags().GetString("effort")
	attemptBackend, _ := cmd.Flags().GetString("attempt-backend")
	ignoreCooldown, _ := cmd.Flags().GetBool("ignore-cooldown")
	cooldownReason, _ := cmd.Flags().GetString("reason")
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
	reviewTier, _ := cmd.Flags().GetString("review-tier")
	maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")
	maxBeadCostUSD, _ := cmd.Flags().GetFloat64("max-bead-cost")
	maxRecoveryCostUSD, _ := cmd.Flags().GetFloat64("max-recovery-cost")
	preClaimTimeout, _ := cmd.Flags().GetDuration("preclaim-timeout")
	routeResolutionTimeout, _ := cmd.Flags().GetDuration("route-resolution-timeout")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	rateLimitMaxWait, _ := cmd.Flags().GetDuration("rate-limit-max-wait")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	if noReview && !noReviewAck {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--no-review requires --no-review-i-know-what-im-doing (break-glass acknowledgement)")
	}
	cooldownReason = strings.TrimSpace(cooldownReason)
	if ignoreCooldown && cooldownReason == "" {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, fmt.Errorf("--ignore-cooldown requires --reason \"<text>\"")
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
		ProjectRoot:            projectRoot,
		Harness:                harness,
		Model:                  model,
		Profile:                profile,
		Provider:               provider,
		LabelFilter:            labelFilter,
		Effort:                 effort,
		AttemptBackend:         attemptBackend,
		Mode:                   mode,
		IdleInterval:           idleInterval,
		NoReview:               noReview,
		ReviewTier:             reviewTier,
		IgnoreCooldown:         ignoreCooldown,
		CooldownOverrideReason: cooldownReason,
		OpaquePassthrough:      treatPassthroughAsOpaque,
		MaxCostUSD:             maxCostUSD,
		MaxBeadCostUSD:         maxBeadCostUSD,
		MaxRecoveryCostUSD:     maxRecoveryCostUSD,
		PreClaimTimeout:        executeloop.Duration{Duration: preClaimTimeout},
		RouteResolutionTimeout: executeloop.Duration{Duration: routeResolutionTimeout},
		RequestTimeout:         executeloop.Duration{Duration: requestTimeout},
		RateLimitMaxWait:       executeloop.Duration{Duration: rateLimitMaxWait},
		MinPower:               minPower,
		MinPowerSet:            cmd.Flags().Changed("min-power"),
		MaxPower:               maxPower,
		FromRev:                fromRev,
	}
	spec.ApplyDefaults()
	if err := spec.Validate(); err != nil {
		return executeloop.ExecuteLoopSpec{}, executeloop.DispatchOptions{}, err
	}

	return spec, executeloop.DispatchOptions{Local: local, JSON: dispatchJSON}, nil
}

func workSelfRefreshEnabled(cmd *cobra.Command) bool {
	watch, _ := cmd.Flags().GetBool("watch")
	if !watch {
		return false
	}
	// Explicit opt-out always wins: --no-self-refresh disables the capability
	// even though it is on by default in watch mode.
	if noRefresh, _ := cmd.Flags().GetBool("no-self-refresh"); noRefresh {
		return false
	}
	if cmd.Flags().Changed("self-refresh") {
		enabled, _ := cmd.Flags().GetBool("self-refresh")
		return enabled
	}
	return true
}

func executeLoopAttemptRuntime(spec executeloop.ExecuteLoopSpec, output io.Writer, events agent.BeadEventAppender, runner agent.AgentRunner, checker agent.ExecutionResourceChecker, beadStoreRoot string, primaryConfig *config.ResolvedConfig) agent.ExecuteBeadRuntime {
	var reviewer agent.CandidateReviewer
	if !spec.NoReview {
		reviewer = newCommandReviewer(spec.ProjectRoot, beadStoreRoot, spec.ReviewTier, primaryConfig)
	}
	return agent.ExecuteBeadRuntime{
		FromRev:          spec.FromRev,
		Output:           output,
		BeadStoreRoot:    beadStoreRoot,
		BeadEvents:       events,
		AgentRunner:      runner,
		ResourceChecker:  checker,
		Reviewer:         reviewer,
		NoReview:         spec.NoReview,
		RateLimitMaxWait: spec.RateLimitMaxWait.Duration,
	}
}

func newCommandReviewer(projectRoot, beadStoreRoot, reviewTier string, primaryConfig *config.ResolvedConfig) *agent.DefaultBeadReviewer {
	reviewer := &agent.DefaultBeadReviewer{
		ProjectRoot:           projectRoot,
		BeadStore:             bead.NewStore(beadStoreRoot),
		BeadEvents:            bead.NewStore(beadStoreRoot),
		ReviewTier:            reviewTier,
		PrimaryConfigSnapshot: primaryConfig,
	}
	if primaryConfig != nil {
		reviewer.Caps = primaryConfig.EvidenceCapsForRole(config.EvidenceRoleReviewer)
		reviewer.CapsConfigured = true
	}
	return reviewer
}

func optionalIntFlag(cmd *cobra.Command, name string, defaultValue int) int {
	if cmd.Flags().Lookup(name) == nil {
		return defaultValue
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil {
		return defaultValue
	}
	return value
}

func optionalFloat64Flag(cmd *cobra.Command, name string, defaultValue float64) float64 {
	if cmd.Flags().Lookup(name) == nil {
		return defaultValue
	}
	value, err := cmd.Flags().GetFloat64(name)
	if err != nil {
		return defaultValue
	}
	return value
}

func (f *CommandFactory) runAgentExecuteLoopImpl(cmd *cobra.Command, treatPassthroughAsOpaque bool, tryTargetBeadID string) error {
	spec, dispatch, err := parseExecuteLoopSpec(cmd, treatPassthroughAsOpaque)
	if err != nil {
		return err
	}
	projectFlag := spec.ProjectRoot
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	spec.ProjectRoot = projectRoot
	beadStoreRoot := f.commandBeadStoreRoot(projectFlag, projectRoot)
	if tryTargetBeadID != "" {
		spec.Mode = executeloop.ModeOnce
		spec.IdleInterval = executeloop.Duration{}
	}
	explicitMinPower := cmd.Flags().Changed("min-power")
	store := bead.NewStore(beadStoreRoot)
	workerStore := agent.ExecuteBeadLoopStore(store)
	if spec.IgnoreCooldown {
		workerStore = newIgnoreCooldownStore(store)
	}
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
	// The DDx server serves a self-signed cert; inject a skip-verify client so
	// worker registration/event mirroring succeeds instead of failing every
	// POST with "tls: bad certificate".
	probe := workerprobe.New(probeIdent, workerprobe.Config{
		AddrFunc:   serverpkg.ReadServerAddr,
		HTTPClient: newLocalServerClientTimeout(5 * time.Second),
	})
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
	var reviewer agent.BeadReviewer
	if !spec.NoReview {
		reviewer = newCommandReviewer(projectRoot, beadStoreRoot, spec.ReviewTier, &rcfg)
	}
	serverHealthProbe := func(ctx context.Context) (bool, error) {
		addr := serverpkg.ReadServerAddr()
		if strings.TrimSpace(addr) == "" {
			return false, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(addr, "/")+"/api/health", nil)
		if err != nil {
			return false, err
		}
		// The DDx server serves a self-signed cert, so this loopback probe must
		// skip verification; a bare http.Client fails the handshake with
		// "tls: bad certificate" and spams the server log.
		client := newLocalServerClientTimeout(5 * time.Second)
		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("server health status %s", resp.Status)
		}
		smokeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		smokeResult, smokeErr := agent.RunWithConfigViaService(smokeCtx, projectRoot, rcfg, agent.AgentRunRuntime{
			Prompt:  "server health smoke test",
			WorkDir: projectRoot,
		})
		if smokeErr != nil {
			return false, smokeErr
		}
		return smokeResult != nil && smokeResult.ExitCode == 0, nil
	}

	resourceChecker := buildCLIResourceChecker(projectRoot, f.resourceCheckerOverride)
	resourcePressureChecker := buildCLIResourcePressureChecker(projectRoot, f.resourcePressureCheckerOverride)
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
			Events:      bead.NewStore(beadStoreRoot),
			Actor:       resolveClaimAssignee(),
			Source:      "ddx work",
		})
	}
	decompositionHook := agent.NewPreClaimDecompositionHook(store, qualityRunner, rcfg, projectRoot)
	recoveryHook := agent.NewAutoRecoveryPostLadderExhaustionHook(store, qualityRunner, rcfg, projectRoot, agent.AutoRecoveryConfig{
		MaxRecoveryCostUSD: spec.MaxRecoveryCostUSD,
		MaxBeadCostUSD:     spec.MaxBeadCostUSD,
	})

	// DDx consumes the cost reported by Fizeau. It does not inspect Fizeau's
	// harness inventory or infer billing properties during attempt control.
	costCap := escalation.NewCostCapTracker(spec.MaxCostUSD, nil)
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
			Detail: fmt.Sprintf("cost cap reached: $%.2f reported >= $%.2f cap; raise the cap or set 0 to disable.", spent, spec.MaxCostUSD),
		}, true
	}

	singlePolicyAttempt := func(ctx context.Context, beadID string, requestedMinPower int, requestedProfile string, routingIntent escalation.ExecutionHint, routingNote string, resolvedHarness, resolvedProvider, resolvedModel string) (agent.ExecuteBeadReport, error) {
		gitOps := &agent.RealGitOps{}
		attemptProvider := spec.Provider
		if resolvedProvider != "" {
			attemptProvider = resolvedProvider
		}
		reportFromResult := func(res *agent.ExecuteBeadResult) agent.ExecuteBeadReport {
			report := agent.ReportFromExecuteBeadResult(res, "")
			applyExecutionRoutingIntentReport(&report, routingIntent, requestedProfile, requestedMinPower, spec.MaxPower, routingNote)
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
			AttemptBackend:    spec.AttemptBackend,
		}
		requestTimeout := spec.RequestTimeout.Duration
		if requestTimeout > 0 {
			loopOverrides.ProviderRequestTimeout = &requestTimeout
		}
		attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
		res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, beadID, attemptRcfg, executeLoopAttemptRuntime(
			spec,
			cmd.OutOrStdout(),
			bead.NewStore(beadStoreRoot),
			f.AgentRunnerOverride,
			resourceChecker,
			beadStoreRoot,
			&attemptRcfg,
		), gitOps)
		if execErr != nil && res == nil {
			return agent.ExecuteBeadReport{}, execErr
		}
		// Execution evidence is local, per-machine state. Reconcile the
		// repository-local exclude before any outcome-specific return, and fail
		// closed if evidence has already reached the index or history.
		if res != nil && res.AttemptID != "" {
			if candidateErr := agent.VerifyCandidateHasNoExecutionEvidence(projectRoot, res.BaseRev, res.ResultRev); candidateErr != nil {
				candidateErr = fmt.Errorf("validating local execution evidence boundary: %w", candidateErr)
				if execErr != nil {
					agent.MarkResultExecutionError(res, execErr)
					candidateErr = errors.Join(execErr, candidateErr)
				}
				return reportFromResult(res), candidateErr
			}
			if retentionErr := agent.VerifyCleanWorktree(projectRoot, res.AttemptID); retentionErr != nil {
				retentionErr = fmt.Errorf("retaining local execution evidence: %w", retentionErr)
				if execErr != nil {
					agent.MarkResultExecutionError(res, execErr)
					retentionErr = errors.Join(execErr, retentionErr)
				}
				return reportFromResult(res), retentionErr
			}
		}
		if res != nil && agent.IsResourceExhaustedStatus(res.Status) {
			return reportFromResult(res), nil
		}
		if execErr != nil {
			agent.MarkResultExecutionError(res, execErr)
			return reportFromResult(res), nil
		}
		if prepareCandidateCycleLanding(res) {
			targetBead, err := resolveAttemptBead(ctx, beadID, store, func() attemptBeadReader {
				if store == nil {
					return nil
				}
				return bead.NewStoreWithCollection(store.Dir, store.Collection)
			}, nil)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			landRes, _, landErr := agent.SubmitWithPreMergeChecks(
				ctx, projectRoot, targetBead, res,
				func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
				bead.NewStore(beadStoreRoot),
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
		} else {
			normalizeUnlandedAttemptResult(res)
		}
		return reportFromResult(res), nil
	}

	executor := f.tryExecutorOverride
	if executor == nil {
		executor = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (agent.ExecuteBeadReport, error) {
			targetBead, err := resolveAttemptBead(ctx, beadID, store, func() attemptBeadReader {
				if store == nil {
					return nil
				}
				return bead.NewStoreWithCollection(store.Dir, store.Collection)
			}, nil)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			routingIntent := resolveCommandExecutionHint(ctx, targetBead, explicitMinPower, spec.Profile)
			initialMinPower := rcfg.MinPower()
			if routingIntent.HasInferredMinPower {
				initialMinPower = routingIntent.InferredMinPower
			}
			if spec.MaxPower > 0 && initialMinPower > spec.MaxPower {
				return agent.ExecuteBeadReport{}, fmt.Errorf("inferred MinPower %d conflicts with requested MaxPower %d", initialMinPower, spec.MaxPower)
			}
			initialProfile := spec.Profile
			initialRoutingNote := ""
			perBeadBudget := spec.MaxBeadCostUSD
			if override, ok := escalation.ParseBeadBudgetLabel(targetBead.Labels); ok {
				perBeadBudget = override
			}
			perBeadTracker := escalation.NewPerBeadCostTracker(perBeadBudget, nil)
			report, err := runEscalatingPowerAttempts(
				ctx,
				initialMinPower,
				spec.MaxPower,
				func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
					return singlePolicyAttempt(ctx, beadID, requestedMinPower, initialProfile, routingIntent, initialRoutingNote, spec.Harness, spec.Provider, spec.Model)
				},
				nil,
				perBeadTracker,
				strings.TrimSpace(spec.Harness) == "" && strings.TrimSpace(spec.Provider) == "" && strings.TrimSpace(spec.Model) == "",
				agent.ProviderPin{Harness: spec.Harness, Provider: spec.Provider, Model: spec.Model},
			)
			if err == nil {
				accumulateBilledCost(report)
			}
			return report, err
		})
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    workerStore,
		Reviewer: reviewer,
		EscalationNextFloor: func(actualPower int) (int, error) {
			return executeloop.NextAbstractMinPower(actualPower, actualPower)
		},
		Executor: executor,
	}

	cliLandingOps := agent.RealLandingGitOps{}
	progressLog := cmd.OutOrStdout()
	if jsonOutput {
		progressLog = io.Discard
	}
	finalizeDurableAudit, flushDurableAudit := f.buildAttemptAuditFinalizers(projectRoot, store)
	result, err := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                    spec.Mode,
		IdleInterval:            spec.IdleInterval.Duration,
		IgnoreCooldown:          spec.IgnoreCooldown,
		CooldownOverrideReason:  spec.CooldownOverrideReason,
		Log:                     progressLog,
		CleanupLog:              cleanupLog,
		EventSink:               loopSink,
		WorkerID:                resolveClaimAssignee(),
		ProjectRoot:             projectRoot,
		TrackerSyncEnabled:      workTrackerSyncEnabled(cmd),
		CleanupRunner:           cleanupRunner,
		ResourceChecker:         resourceChecker,
		ResourcePressureChecker: resourcePressureChecker,
		LoadPressureThreshold:   rcfg.LoadPressureThreshold(),
		LoadPressureSnapshot: func() workerstatus.LoadPressureSnapshot {
			return workerstatus.HostLoadPressureSnapshot(rcfg.LoadPressureThreshold())
		},
		ServerHealthProbe:            serverHealthProbe,
		BinaryRefreshCheck:           f.buildWorkBinaryRefreshCheck(cmd, projectRoot, tryTargetBeadID, workSelfRefreshEnabled(cmd)),
		ProjectRootDirtyCheck:        agent.CanonicalRootDirtyPaths,
		SessionID:                    loopSessionID,
		PreClaimHook:                 buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreClaimIntakeHook:           intakeHook,
		PreClaimTimeout:              spec.PreClaimTimeout.Duration,
		RouteResolutionTimeout:       spec.RouteResolutionTimeout.Duration,
		ClaimSuccessRateWindow:       optionalIntFlag(cmd, "claim-rate-window", agent.DefaultClaimSuccessRateWindow),
		ClaimSuccessRateThreshold:    optionalFloat64Flag(cmd, "claim-rate-threshold", agent.DefaultClaimSuccessRateThreshold),
		PreClaimWarnRepeatThreshold:  optionalIntFlag(cmd, "preclaim-warn-threshold", agent.DefaultPreClaimWarnRepeatThreshold),
		PreDispatchLintHook:          lintHook,
		PostAttemptTriageHook:        triageHook,
		ProseEvidenceHook:            proseHook,
		PostAttemptDecompositionHook: decompositionHook,
		BudgetStop:                   costCapTripped,
		NoReview:                     spec.NoReview,
		TargetBeadID:                 tryTargetBeadID,
		ReviewCostCap:                costCap,
		FinalizeDurableAudit:         finalizeDurableAudit,
		FlushDurableAudit:            flushDurableAudit,
		PostLadderExhaustionHook:     recoveryHook,
	})
	if err != nil && result != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		_ = writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
		if errors.Is(err, context.Canceled) {
			return NewExitError(130, "")
		}
	}
	if err != nil {
		return err
	}
	writeServerManagedResult(cmd, projectRoot, result)
	if result != nil && result.ExitReason == "binary_refresh" {
		return nil
	}
	return writeExecuteLoopResult(cmd.OutOrStdout(), projectRoot, result, jsonOutput)
}

// writeServerManagedResult persists the loop's structured terminal outcome to
// the worker dir when running as a server-managed subprocess, so the
// supervising ddx-server can classify the exit (notably operator-attention
// stops) instead of treating every clean exit as a successful drain. See
// ddx-3d57bc30.
func writeServerManagedResult(cmd *cobra.Command, projectRoot string, result *agent.ExecuteBeadLoopResult) {
	if result == nil {
		return
	}
	f := cmd.Flags().Lookup("server-managed")
	if f == nil {
		return
	}
	workerID := f.Value.String()
	if workerID == "" {
		return
	}
	lastFailureDetail := ""
	diagnosis := ""
	restartable := false
	if n := len(result.Results); n > 0 {
		last := result.Results[n-1]
		lastFailureDetail = last.Detail
		// Preserve structured resource-exhaustion diagnosis from the agent path
		// so the supervisor status helper can surface fd_exhaustion without
		// free-text matching (ddx-029c1dde).
		diagnosis = last.ResourceExhaustionDiagnosis
		restartable = last.ResourceExhaustionRestartable
	}
	_ = serverpkg.WriteManagedWorkerResult(projectRoot, workerID, serverpkg.ManagedWorkerResult{
		StopCondition:                 result.StopCondition,
		OperatorAttention:             result.OperatorAttention != nil,
		LastFailureStatus:             result.LastFailureStatus,
		LastFailureDetail:             lastFailureDetail,
		ResourceExhaustionDiagnosis:   diagnosis,
		ResourceExhaustionRestartable: restartable,
	})
}

func (f *CommandFactory) buildAttemptAuditFinalizers(projectRoot string, store *bead.Store) (func(agent.ExecuteBeadReport) error, func() error) {
	if f.durableAuditFinalizeOverride != nil {
		return f.durableAuditFinalizeOverride, nil
	}
	accumulator := agent.NewDurableAuditAccumulator(projectRoot, store)
	return accumulator.Finalize, accumulator.Flush
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

func resolveCommandExecutionHint(ctx context.Context, b *bead.Bead, explicitMinPower bool, publicPolicy string) escalation.ExecutionHint {
	return escalation.ResolveExecutionHint(escalation.ExecutionHintInput{
		Bead:                         b,
		ReadinessEstimatedDifficulty: agent.ReadinessEstimatedDifficultyFromContext(ctx),
		ExplicitMinPower:             explicitMinPower,
		PublicPolicy:                 publicPolicy,
	})
}

func applyExecutionRoutingIntentReport(report *agent.ExecuteBeadReport, intent escalation.ExecutionHint, requestedPolicy string, requestedMinPower, requestedMaxPower int, routingNote string) {
	if report == nil {
		return
	}
	report.RoutingIntentSource = string(intent.Source)
	report.EstimatedDifficulty = string(intent.EstimatedDifficulty)
	report.InferredMinPower = intent.InferredMinPower
	report.InferredMinPowerPresent = intent.HasInferredMinPower
	report.RequestedPolicy = requestedPolicy
	report.RequestedMinPower = requestedMinPower
	report.RequestedMaxPower = requestedMaxPower
	report.RoutingIntentNote = routingNote
}
