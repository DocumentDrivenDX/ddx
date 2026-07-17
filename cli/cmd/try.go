package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	workguard "github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/spf13/cobra"
)

// Exit codes for ddx try terminal outcomes. These are specific to ddx try
// and intentionally do not reuse the generic ExitCode* constants because
// the bead-execution outcome vocabulary is distinct from generic CLI errors.
const (
	tryExitSuccess         = 0 // success or already_satisfied
	tryExitPreservedOrNone = 1 // preserved_for_review or no_changes
	tryExitFailed          = 2 // execution_failed or any other terminal non-success
)

// newTryCommand creates the top-level "ddx try <bead-id>" command.
// It targets a single specific bead through the full production execution
// machinery (claim → execute → land → review), unlike "ddx work" which drains
// the queue by picker priority and cannot force a specific bead.
func (f *CommandFactory) newTryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "try <bead-id>",
		Short:        "Execute a specific bead through the production machinery",
		SilenceUsage: true,
		Long: `try executes a single named bead through the full production
execution machinery: claim → execute → land → review.

Unlike "ddx work" which drains the queue by picker priority, "ddx try"
targets a specific bead ID. This is useful for:
  - Operator-driven debugging when a bead misbehaves under autonomous drain
  - Deterministic queue ordering during refactor execution (dispatch each child
    in dep order with an explicit ID)
  - Path-comparison studies that need to execute one fixture bead at a time

Pre-flight checks (run before any claim is issued):
  - bead not found → non-zero exit with "bead not found: <id>"
  - bead not claimable (status=closed, cancelled, etc.) → non-zero exit
  - bead has unmet dependencies → non-zero exit listing the blockers

Review is on by default. --no-review is a break-glass override and
requires --no-review-i-know-what-im-doing. A bead label of review:skip
is only honored when it also carries a sibling review:skip-reason:*
label; otherwise the label is ignored.

Flag plumbing is identical to "ddx work": --harness, --model, --provider,
--profile, --effort are forwarded to the agent as opaque passthrough
constraints. DDx does not validate or branch on their string values.

Exit codes:
  0  success or already_satisfied
  1  preserved_for_review or no_changes
  2  execution_failed or any other terminal non-success
`,
		Example: `  # Execute a specific bead
  ddx try ddx-abc12345

  # Execute with an explicit harness and model
  ddx try ddx-abc12345 --harness codex --model gpt-5.4-mini

  # Execute without the post-merge review step (break-glass only)
  ddx try ddx-abc12345 --no-review --no-review-i-know-what-im-doing`,
		Args: cobra.ExactArgs(1),
		RunE: f.runTry,
	}

	// Routing knobs — identical to "ddx work" passthrough surface.
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("model", "", "Model constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("profile", "", "Opaque Fizeau routing profile (empty = unconstrained)")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().String("attempt-backend", "", "Attempt backend: worktree, local-clone, docker-clone, or in-tree (default: executions.attempt_backend)")
	cmd.Flags().Bool("force-claim", false, "Ignore retry cooldown for this targeted attempt only (requires --reason)")
	cmd.Flags().String("reason", "", "Operator reason required by --force-claim")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review (break-glass: requires --no-review-i-know-what-im-doing)")
	cmd.Flags().Bool("no-review-i-know-what-im-doing", false, "Break-glass acknowledgement required when using --no-review")
	cmd.Flags().String("review-tier", "", "Abstract review cardinality tier (empty = risk-proportional; elevated = two reviewers)")
	cmd.Flags().Duration("preclaim-timeout", workguard.DefaultPreClaimTimeout, "Pre-claim readiness timeout for preflight/readiness hooks")
	cmd.Flags().Duration("route-resolution-timeout", agent.DefaultRouteResolutionTimeout, "Timeout from Fizeau Execute dispatch to routing_decision; on expiry the lease is released and the bead is flagged for operator attention")
	cmd.Flags().Duration("request-timeout", 0, "Explicit per-request provider wall-clock timeout passed to Fizeau (default: unset)")
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained)")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained)")

	return cmd
}

// runTry is the RunE for "ddx try <bead-id>".
func (f *CommandFactory) runTry(cmd *cobra.Command, args []string) error {
	if err := f.checkLifecycleMigrationGate(cmd); err != nil {
		return err
	}

	beadID := args[0]

	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	beadStoreRoot := f.commandBeadStoreRoot(projectFlag, projectRoot)
	f.warnIfInstalledBinaryBehindSource(cmd)

	if _, err := newStartupHousekeepingRunner(projectRoot).Cleanup(cmd.Context()); err != nil {
		return err
	}

	// Preflight: warn once per process for degraded project-local skill layout.
	// Runs before claim/worktree setup so the operator sees the warning before
	// any bead state changes.
	preflightResult := checkProjectRuntimePreflight(projectRoot)
	f.preflightWarnOnce.Do(func() {
		emitPreflightWarning(cmd.ErrOrStderr(), preflightResult)
	})

	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	provider, _ := cmd.Flags().GetString("provider")
	effort, _ := cmd.Flags().GetString("effort")
	attemptBackend, _ := cmd.Flags().GetString("attempt-backend")
	forceClaim, _ := cmd.Flags().GetBool("force-claim")
	forceReason, _ := cmd.Flags().GetString("reason")
	noReview, _ := cmd.Flags().GetBool("no-review")
	noReviewAck, _ := cmd.Flags().GetBool("no-review-i-know-what-im-doing")
	reviewTier, _ := cmd.Flags().GetString("review-tier")
	preClaimTimeout, _ := cmd.Flags().GetDuration("preclaim-timeout")
	routeResolutionTimeout, _ := cmd.Flags().GetDuration("route-resolution-timeout")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	forceReason = strings.TrimSpace(forceReason)
	if reviewTier != "" && reviewTier != executeloop.ReviewTierElevated {
		return fmt.Errorf("unknown --review-tier %q (want %q)", reviewTier, executeloop.ReviewTierElevated)
	}
	if noReview && !noReviewAck {
		return fmt.Errorf("--no-review requires --no-review-i-know-what-im-doing (break-glass acknowledgement)")
	}
	if forceClaim && forceReason == "" {
		return fmt.Errorf("--force-claim requires --reason \"<text>\"")
	}
	store := bead.NewStore(beadStoreRoot)

	// Pre-flight: look up the bead.
	target, err := store.Get(context.Background(), beadID)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "bead not found: %s\n", beadID)
		return &ExitError{Code: tryExitFailed, Message: ""}
	}

	// Pre-flight: verify the bead is in a claimable state.
	// Claimable = open, or stale in_progress (the loop handles stale reclaim).
	switch target.Status {
	case bead.StatusOpen, bead.StatusInProgress:
		// allowed — loop's Claim() handles stale-heartbeat reclaim
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "bead is not claimable (status=%s)\n", target.Status)
		return &ExitError{Code: tryExitFailed, Message: ""}
	}

	// Pre-flight: check for unmet dependencies.
	if unmet := unmetDeps(store, target); len(unmet) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "bead has unmet dependencies: %s\n", strings.Join(unmet, ", "))
		return &ExitError{Code: tryExitFailed, Message: ""}
	}

	// Restrict the ready queue view to the targeted bead. Retry cooldown is
	// only bypassed when --force-claim is explicitly present.
	forcedStore := &singleBeadStore{
		Store:         store,
		targetID:      target.ID,
		forceCooldown: forceClaim,
	}
	if !forceClaim {
		standardReady, standardErr := store.ReadyExecution()
		if standardErr == nil && !containsBeadID(standardReady, target.ID) {
			if forcedReady, readyErr := store.ReadyExecutionIgnoringCooldown(); readyErr == nil && containsBeadID(forcedReady, target.ID) {
				fmt.Fprintf(cmd.ErrOrStderr(), "bead is in retry cooldown; re-run with --force-claim --reason \"<text>\": %s\n", beadID)
				return &ExitError{Code: tryExitFailed, Message: ""}
			}
		}
	}

	// Set up progress logging (same as runAgentExecuteLoopImpl).
	loopSessionID := fmt.Sprintf("agent-loop-%d", time.Now().UnixNano())
	loopLogDir := filepath.Join(projectRoot, agent.DefaultLogDir)
	_ = os.MkdirAll(loopLogDir, 0o755)
	loopLogPath := filepath.Join(loopLogDir, loopSessionID+".jsonl")
	var loopSink io.Writer
	if lf, openErr := os.OpenFile(loopLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
		loopSink = lf
		defer lf.Close() //nolint:errcheck
	}

	// Worker-probe (best-effort server mirror; non-blocking).
	probeIdent := workerprobe.Identity{
		ProjectRoot:  projectRoot,
		Harness:      harness,
		Model:        model,
		ExecutorPID:  os.Getpid(),
		ExecutorHost: hostnameOrEmpty(),
		StartedAt:    time.Now().UTC(),
	}
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

	// Process-local LandCoordinator.
	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	// Post-merge reviewer (on by default, skipped via --no-review).
	var reviewer agent.CandidateReviewer
	var postMergeReviewer agent.BeadReviewer

	explicitMinPower := cmd.Flags().Changed("min-power")
	resourceChecker := buildCLIResourceChecker(projectRoot, f.resourceCheckerOverride)

	var gitOps agent.GitOps = &agent.RealGitOps{}
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}
	agent.RecoverOrphans(gitOps, projectRoot, beadID)

	// Build the executor: either the test override or the real single-power executor.
	var executor agent.ExecuteBeadExecutor
	if f.tryExecutorOverride != nil {
		executor = f.tryExecutorOverride
	} else {
		executor = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, execBeadID string) (agent.ExecuteBeadReport, error) {
			routingIntent, err := resolveTryExecutionHint(ctx, execBeadID, store, target, explicitMinPower, profile)
			if err != nil {
				return agent.ExecuteBeadReport{}, err
			}
			initialMinPower := minPower
			if routingIntent.HasInferredMinPower {
				initialMinPower = routingIntent.InferredMinPower
			}
			if maxPower > 0 && initialMinPower > maxPower {
				return agent.ExecuteBeadReport{}, fmt.Errorf("inferred MinPower %d conflicts with requested MaxPower %d", initialMinPower, maxPower)
			}
			singleAttempt := func(ctx context.Context, execBeadID string, requestMinPower int) (agent.ExecuteBeadReport, error) {
				requestProfile := profile
				var routingNote string
				reportFromResult := func(res *agent.ExecuteBeadResult) agent.ExecuteBeadReport {
					report := agent.ReportFromExecuteBeadResult(res, "")
					applyExecutionRoutingIntentReport(&report, routingIntent, requestProfile, requestMinPower, maxPower, routingNote)
					return report
				}
				loopOverrides := config.CLIOverrides{
					Harness:           harness,
					Model:             model,
					Provider:          provider,
					Profile:           requestProfile,
					Effort:            effort,
					MinPower:          requestMinPower,
					MaxPower:          maxPower,
					OpaquePassthrough: true, // ddx try treats flags as opaque passthrough
					AttemptBackend:    attemptBackend,
				}
				if requestTimeout > 0 {
					loopOverrides.ProviderRequestTimeout = &requestTimeout
				}
				attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
				res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, execBeadID, attemptRcfg, agent.ExecuteBeadRuntime{
					FromRev:         fromRev,
					Output:          cmd.OutOrStdout(),
					BeadStoreRoot:   beadStoreRoot,
					BeadEvents:      bead.NewStore(beadStoreRoot),
					ResourceChecker: resourceChecker,
					Reviewer:        reviewer,
					NoReview:        noReview,
				}, gitOps)
				if execErr != nil && res == nil {
					return agent.ExecuteBeadReport{}, execErr
				}
				// Execution evidence is local, per-machine state. Reconcile the
				// repository-local exclude before any outcome-specific return, and
				// fail closed if evidence is already staged or tracked.
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
					if wt, ids, cleanup, ctxErr := agent.BuildLandingGateContext(projectRoot, res, gitOps); ctxErr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "ddx: warning: gate-context setup failed: %v (skipping required-gate eval)\n", ctxErr)
					} else if wt != "" {
						defer cleanup()
						checksAbs := filepath.Join(projectRoot, res.ExecutionDir, "checks.json")
						checksRel := filepath.Join(res.ExecutionDir, "checks.json")
						anyFailed, ratchetFailed, evalErr := agent.EvaluateRequiredGatesForResult(wt, ids, res, projectRoot, checksAbs, checksRel)
						if evalErr != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "ddx: warning: gate evaluation failed: %v (skipping)\n", evalErr)
						} else if anyFailed || ratchetFailed {
							preserveRef := agent.PreserveRef(res.BeadID, res.BaseRev)
							if upErr := gitOps.UpdateRef(projectRoot, preserveRef, res.ResultRev); upErr != nil {
								agent.MarkResultLandError(projectRoot, res, upErr)
							} else {
								res.PreserveRef = preserveRef
								res.Outcome = "preserved"
								if ratchetFailed {
									res.Reason = agent.RatchetPreserveReason
								} else {
									res.Reason = "post-run checks failed"
								}
								res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
								res.Detail = agent.ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
							}
							_ = agent.WriteExecuteBeadResultArtifact(projectRoot, res)
							report := agent.ReportFromExecuteBeadResult(res, "")
							applyExecutionRoutingIntentReport(&report, routingIntent, requestProfile, requestMinPower, maxPower, routingNote)
							return report, nil
						}
					}
					targetBead := target
					if loaded, loadErr := resolveAttemptBead(ctx, execBeadID, store, func() attemptBeadReader {
						if store == nil {
							return nil
						}
						return bead.NewStoreWithCollection(store.Dir, store.Collection)
					}, target); loadErr == nil {
						targetBead = loaded
					}
					landRes, _, landErr := agent.SubmitWithPreMergeChecks(
						ctx, projectRoot, targetBead, res,
						func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
						bead.NewStore(beadStoreRoot),
						resolveClaimAssignee(), "ddx try",
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
				report := agent.ReportFromExecuteBeadResult(res, "")
				applyExecutionRoutingIntentReport(&report, routingIntent, requestProfile, requestMinPower, maxPower, routingNote)
				return report, nil
			}
			return runEscalatingPowerAttempts(ctx, initialMinPower, maxPower, func(ctx context.Context, requestedMinPower int) (agent.ExecuteBeadReport, error) {
				return singleAttempt(ctx, execBeadID, requestedMinPower)
			}, nil, nil, strings.TrimSpace(harness) == "" && strings.TrimSpace(provider) == "" && strings.TrimSpace(model) == "",
				agent.ProviderPin{Harness: harness, Provider: provider, Model: model})
		})
	}

	overrides := config.CLIOverrides{
		Assignee:          resolveClaimAssignee(),
		Harness:           harness,
		Model:             model,
		Provider:          provider,
		Profile:           profile,
		Effort:            effort,
		MinPower:          minPower,
		MaxPower:          maxPower,
		OpaquePassthrough: true,
		AttemptBackend:    attemptBackend,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return fmt.Errorf("load resolved config: %w", err)
	}
	if !noReview {
		postMergeReviewer = newCommandReviewer(projectRoot, beadStoreRoot, reviewTier, &rcfg)
		reviewer = postMergeReviewer.(agent.CandidateReviewer)
	}
	worker := &agent.ExecuteBeadWorker{
		Store:    forcedStore,
		Executor: executor,
		Reviewer: postMergeReviewer,
	}
	if _, err := resourceChecker.Check(cmd.Context()); err != nil {
		if resErr, ok := err.(*agent.ResourceExhaustedError); ok && resErr != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), resErr.Error())
		}
		return &ExitError{Code: tryExitFailed, Message: err.Error()}
	}

	cliLandingOps := agent.RealLandingGitOps{}
	var qualityRunner agent.AgentRunner
	if f.AgentRunnerOverride != nil {
		qualityRunner = f.AgentRunnerOverride
	}
	var lintHook func(context.Context, string) (agent.LintResult, error)
	var intakeHook agent.PreClaimIntakeHook
	var triageHook func(context.Context, string, agent.ExecuteBeadReport) (agent.TriageResult, error)
	// tryExecutorOverride is a hermetic test seam: replacing the implementation
	// executor must not silently dispatch real lifecycle agents. Tests that
	// exercise lifecycle hooks provide AgentRunnerOverride explicitly.
	if f.tryExecutorOverride == nil || qualityRunner != nil {
		lintHook = agent.NewPreDispatchLintHook(projectRoot, store, rcfg, nil, qualityRunner)
		innerIntakeHook := agent.NewPreClaimIntakeHookWithLogVerbose(projectRoot, store, rcfg, nil, qualityRunner, cmd.OutOrStdout(), f.viperInstance.GetBool("verbose"))
		intakeHook = agent.NewACQualityPreClaimGate(store, rcfg.BeadQualityMode(), rcfg.ACQualityMinScore(), innerIntakeHook)
		triageHook = agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	}
	proseHook := f.proseEvidenceHookOverride
	if proseHook == nil {
		proseHook = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
			ProjectRoot: projectRoot,
			Events:      bead.NewStore(beadStoreRoot),
			Actor:       resolveClaimAssignee(),
			Source:      "ddx try",
		})
	}
	result, runErr := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                    executeloop.ModeOnce,
		IgnoreCooldown:          forceClaim,
		CooldownOverrideReason:  forceReason,
		Log:                     cmd.OutOrStdout(),
		EventSink:               loopSink,
		WorkerID:                resolveClaimAssignee(),
		ProjectRoot:             projectRoot,
		ResourceChecker:         resourceChecker,
		ResourcePressureChecker: buildCLIResourcePressureChecker(projectRoot, f.resourcePressureCheckerOverride),
		LoadPressureThreshold:   rcfg.LoadPressureThreshold(),
		LoadPressureSnapshot: func() workerstatus.LoadPressureSnapshot {
			return workerstatus.HostLoadPressureSnapshot(rcfg.LoadPressureThreshold())
		},
		SessionID:              loopSessionID,
		RouteResolutionTimeout: routeResolutionTimeout,
		PreClaimHook:           buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreClaimIntakeHook:     intakeHook,
		PreClaimTimeout:        preClaimTimeout,
		PreDispatchLintHook:    lintHook,
		PostAttemptTriageHook:  triageHook,
		ProseEvidenceHook:      proseHook,
		FinalizeDurableAudit:   f.buildAttemptAuditFinalizer(projectRoot, store),
		NoReview:               noReview,
	})
	if runErr != nil {
		if (errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)) && result != nil && len(result.Results) > 0 {
			writeTryResult(cmd.OutOrStdout(), result.Results[0])
		}
		return runErr
	}

	// Report outcome.
	if result.NoReadyWork {
		// Bead was not in the execution-ready queue even after pre-flight passed.
		// This can happen if a heartbeat re-appeared just before claim, making it
		// not stale-enough for reclaim. Surface a clear message.
		fmt.Fprintf(cmd.ErrOrStderr(), "bead is not execution-ready (it may be actively claimed by another worker): %s\n", beadID)
		return &ExitError{Code: tryExitFailed, Message: ""}
	}

	if len(result.Results) == 0 {
		return &ExitError{Code: tryExitFailed, Message: ""}
	}
	report := result.Results[0]
	writeTryResult(cmd.OutOrStdout(), report)
	if report.EstimatedDifficulty != "" && report.InferredMinPowerPresent {
		source := report.RoutingIntentSource
		if source == "" {
			source = string(escalation.ExecutionIntentSourceDefault)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "routing intent: difficulty=%s minPower=%d source=%s\n", report.EstimatedDifficulty, report.InferredMinPower, source)
	}

	return tryExitCodeForStatus(report.Status)
}

func resolveTryExecutionHint(ctx context.Context, beadID string, store *bead.Store, fallback *bead.Bead, explicitMinPower bool, publicPolicy string) (escalation.ExecutionHint, error) {
	currentTarget, err := resolveAttemptBead(ctx, beadID, store, func() attemptBeadReader {
		if store == nil {
			return nil
		}
		return bead.NewStoreWithCollection(store.Dir, store.Collection)
	}, fallback)
	if err != nil {
		return escalation.ExecutionHint{}, err
	}
	return resolveCommandExecutionHint(ctx, currentTarget, explicitMinPower, publicPolicy), nil
}

func writeTryResult(w io.Writer, report agent.ExecuteBeadReport) {
	fmt.Fprintf(w, "\nbead: %s\nstatus: %s\n", report.BeadID, report.Status)
	if report.Detail != "" {
		fmt.Fprintf(w, "detail: %s\n", report.Detail)
	}
	if report.ResultRev != "" {
		fmt.Fprintf(w, "result_rev: %s\n", report.ResultRev)
	}
	if report.ImplementationRev != "" && report.ImplementationRev != report.LandedRev {
		fmt.Fprintf(w, "implementation_rev: %s\n", report.ImplementationRev)
	}
	if report.LandedRev != "" && report.LandedRev != report.ImplementationRev {
		fmt.Fprintf(w, "landed_rev: %s\n", report.LandedRev)
	}
	if isSuccessfulTryResult(report) && report.TargetBranch != "" {
		fmt.Fprintf(w, "landed_branch: %s\n", report.TargetBranch)
	}
	if isSuccessfulTryResult(report) && report.ProjectRoot != "" {
		fmt.Fprintf(w, "project_root: %s\n", report.ProjectRoot)
	}
	if report.PreserveRef != "" {
		fmt.Fprintf(w, "preserve_ref: %s\n", report.PreserveRef)
	}
	if needsNoRewriteLandingRescue(report) {
		fmt.Fprintf(w, "landing_scope: branch-local\n")
		fmt.Fprintf(w, "rescue_command: %s\n", noRewriteLandingRescueCommand(report.TargetBranch))
		fmt.Fprintf(w, "dirty_worktree_rescue: %s\n", dirtyWorktreeRescueCommand(report.TargetBranch))
	}
}

func isSuccessfulTryResult(report agent.ExecuteBeadReport) bool {
	return report.Status == agent.ExecuteBeadStatusSuccess || report.Status == agent.ExecuteBeadStatusAlreadySatisfied
}

func needsNoRewriteLandingRescue(report agent.ExecuteBeadReport) bool {
	if !isSuccessfulTryResult(report) || report.TargetBranch == "" {
		return false
	}
	switch report.TargetBranch {
	case "main", "master", "trunk":
		return false
	default:
		return true
	}
}

func noRewriteLandingRescueCommand(branch string) string {
	return "git merge --no-ff " + branch
}

func dirtyWorktreeRescueCommand(branch string) string {
	safeBranch := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(branch)
	return fmt.Sprintf("git worktree add ../ddx-rescue-%s <target-branch> && cd ../ddx-rescue-%s && git merge --no-ff %s", safeBranch, safeBranch, branch)
}

// tryExitCodeForStatus maps a bead execution status string to the ddx try exit code contract.
func tryExitCodeForStatus(status string) error {
	switch status {
	case agent.ExecuteBeadStatusSuccess, agent.ExecuteBeadStatusAlreadySatisfied:
		return nil // exit 0
	case agent.ExecuteBeadStatusPreservedNeedsReview, agent.ExecuteBeadStatusNoChanges,
		agent.ExecuteBeadStatusNoEvidenceProduced:
		return &ExitError{Code: tryExitPreservedOrNone, Message: ""}
	default:
		// execution_failed and all other terminal non-success statuses
		return &ExitError{Code: tryExitFailed, Message: ""}
	}
}

// unmetDeps returns the IDs of deps that are not yet closed, in the order
// they appear in the bead's dependency list.
func unmetDeps(store bead.Backend, b *bead.Bead) []string {
	depIDs := b.DepIDs()
	if len(depIDs) == 0 {
		return nil
	}
	var unmet []string
	for _, depID := range depIDs {
		dep, err := store.Get(context.Background(), depID)
		if err != nil || dep.Status != bead.StatusClosed {
			unmet = append(unmet, depID)
		}
	}
	return unmet
}
