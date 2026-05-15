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
	powerladder "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	workguard "github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
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
	cmd.Flags().String("profile", "", "Routing profile: default, cheap, fast, or smart (empty = unconstrained; let the agent service choose)")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review (break-glass: requires --no-review-i-know-what-im-doing)")
	cmd.Flags().Bool("no-review-i-know-what-im-doing", false, "Break-glass acknowledgement required when using --no-review")
	cmd.Flags().String("review-harness", "", "Harness for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart powerClass)")
	cmd.Flags().Duration("preclaim-timeout", workguard.DefaultPreClaimTimeout, "Pre-claim readiness timeout for preflight/readiness hooks")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout; overrides project config and model-class defaults")
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
	if err := f.failIfInstalledBinaryBehindSource(cmd, projectRoot, tryExitFailed); err != nil {
		return err
	}

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
	noReview, _ := cmd.Flags().GetBool("no-review")
	noReviewAck, _ := cmd.Flags().GetBool("no-review-i-know-what-im-doing")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	if noReview && !noReviewAck {
		return fmt.Errorf("--no-review requires --no-review-i-know-what-im-doing (break-glass acknowledgement)")
	}

	if f.tryExecutorOverride == nil {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, harness, model, provider); err != nil {
			return err
		}
	}

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))

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

	// Wrap the real store so that ReadyExecution() returns only the target bead.
	// All other operations pass through to the underlying store unchanged.
	forcedStore := &singleBeadStore{Store: store, target: *target}

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
		AddrFunc: serverpkg.ReadServerAddr,
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
	var reviewer agent.BeadReviewer
	if !noReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(ddxroot.JoinProject(projectRoot)),
			BeadEvents:  bead.NewStore(ddxroot.JoinProject(projectRoot)),
			Harness:     reviewHarness,
			Model:       reviewModel,
		}
	}

	profile = agent.NormalizeRoutingProfile(profile)

	// Determine whether zero-config auto-route applies (same logic as runWork).
	noRoutingFlags := harness == "" && model == "" && provider == "" &&
		!cmd.Flags().Changed("profile") && !cmd.Flags().Changed("min-power") &&
		!cmd.Flags().Changed("max-power")
	autoInferPowerClass := noRoutingFlags && !projectHasRoutingConfig(projectRoot)
	var ladderOnce sync.Once
	var ladder escalationFloorFinder
	loadLadder := func() escalationFloorFinder {
		ladderOnce.Do(func() {
			ladder = powerladder.NewLadder(nil)
			svc, svcErr := agent.ResolveServiceFromWorkDir(projectRoot)
			if svcErr != nil {
				return
			}
			modelCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
			defer cancel()
			models, listErr := svc.ListModels(modelCtx, agentlib.ModelFilter{})
			if listErr != nil {
				return
			}
			ladder = powerladder.NewLadder(models)
		})
		return ladder
	}
	resourceChecker := buildCLIResourceChecker(projectRoot, f.resourceCheckerOverride)
	profileSelector := newImplementationProfileSelector(projectRoot)

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
			var inferredPolicy escalation.PowerClass
			requestMinPower := minPower
			requestProfile := profile
			var routingNote string
			reportFromResult := func(res *agent.ExecuteBeadResult) agent.ExecuteBeadReport {
				report := agent.ReportFromExecuteBeadResult(res, string(inferredPolicy))
				report.InferredPowerClass = string(inferredPolicy)
				report.RequestedProfile = requestProfile
				report.RoutingIntentNote = routingNote
				return report
			}
			if autoInferPowerClass {
				var targetBead *bead.Bead
				if b, getErr := store.Get(context.Background(), execBeadID); getErr == nil {
					targetBead = b
					inferredPolicy = escalation.InferPowerClass(targetBead)
				} else {
					inferredPolicy = escalation.PowerStandard
				}
				if floor, ok := numericPowerFloorHint(targetBead); ok && floor > requestMinPower {
					requestMinPower = floor
				}
				var unavailableReport agent.ExecuteBeadReport
				var unavailable bool
				if selection, selectErr := profileSelector.Select(ctx, inferredPolicy, requestMinPower); selectErr == nil && selection.Name != "" {
					requestProfile = selection.Name
					routingNote = selection.Note
					if maxPower > 0 && requestMinPower > 0 && requestMinPower >= maxPower {
						unavailableReport := smartRouteUnavailableReport(targetBead, requestMinPower, maxPower, nil)
						unavailableReport.InferredPowerClass = string(inferredPolicy)
						unavailableReport.RequestedProfile = requestProfile
						unavailableReport.RoutingIntentNote = routingNote
						return unavailableReport, nil
					}
				} else {
					requestMinPower, unavailableReport, unavailable = investigationRetryInitialMinPowerWithInference(targetBead, requestMinPower, maxPower, loadLadder(), true)
					if unavailable {
						unavailableReport.InferredPowerClass = string(inferredPolicy)
						return unavailableReport, nil
					}
				}
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
			}
			if requestTimeout > 0 {
				loopOverrides.ProviderRequestTimeout = &requestTimeout
			}
			attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
			res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, execBeadID, attemptRcfg, agent.ExecuteBeadRuntime{
				FromRev:         fromRev,
				Output:          cmd.OutOrStdout(),
				BeadEvents:      bead.NewStore(ddxroot.JoinProject(projectRoot)),
				ResourceChecker: resourceChecker,
			}, gitOps)
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
						policyStr := ""
						if inferredPolicy != "" {
							policyStr = string(inferredPolicy)
						}
						report := agent.ReportFromExecuteBeadResult(res, policyStr)
						report.InferredPowerClass = string(inferredPolicy)
						report.RequestedProfile = requestProfile
						report.RoutingIntentNote = routingNote
						return report, nil
					}
				}
				targetBead := target
				landRes, _, landErr := agent.SubmitWithPreMergeChecks(
					ctx, projectRoot, targetBead, res,
					func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
					bead.NewStore(ddxroot.JoinProject(projectRoot)),
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
			policyStr := ""
			if inferredPolicy != "" {
				policyStr = string(inferredPolicy)
			}
			report := agent.ReportFromExecuteBeadResult(res, policyStr)
			report.InferredPowerClass = string(inferredPolicy)
			report.RequestedProfile = requestProfile
			report.RoutingIntentNote = routingNote
			return report, nil
		})
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    forcedStore,
		Executor: executor,
		Reviewer: reviewer,
	}

	overrides := config.CLIOverrides{
		Assignee: resolveClaimAssignee(),
		Harness:  harness,
		Model:    model,
		Provider: provider,
		Profile:  profile,
		Effort:   effort,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return fmt.Errorf("load resolved config: %w", err)
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
	lintHook := agent.NewPreDispatchLintHook(projectRoot, store, rcfg, nil, qualityRunner)
	triageHook := agent.NewPostAttemptTriageHook(projectRoot, store, rcfg, nil, qualityRunner, nil)
	proseHook := f.proseEvidenceHookOverride
	if proseHook == nil {
		proseHook = agent.NewDefaultProseEvidenceHook(agent.ProseEvidenceConfig{
			ProjectRoot: projectRoot,
			Events:      bead.NewStore(ddxroot.JoinProject(projectRoot)),
			Actor:       resolveClaimAssignee(),
			Source:      "ddx try",
		})
	}
	result, runErr := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Mode:                  executeloop.ModeOnce,
		Log:                   cmd.OutOrStdout(),
		EventSink:             loopSink,
		WorkerID:              resolveClaimAssignee(),
		ProjectRoot:           projectRoot,
		ResourceChecker:       resourceChecker,
		SessionID:             loopSessionID,
		PreClaimHook:          buildCLIPreClaimHook(projectRoot, cliLandingOps),
		PreDispatchLintHook:   lintHook,
		PostAttemptTriageHook: triageHook,
		ProseEvidenceHook:     proseHook,
		NoReview:              noReview,
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
	if intent := escalation.ParseExecutionHint(target); intent.Source == escalation.ExecutionIntentSourceBeadHint && intent.InferredPowerClass != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "routing intent: difficulty=%s powerClass=%s source=%s\n", intent.EstimatedDifficulty, intent.InferredPowerClass, intent.Source)
	}

	return tryExitCodeForStatus(report.Status)
}

func writeTryResult(w io.Writer, report agent.ExecuteBeadReport) {
	fmt.Fprintf(w, "\nbead: %s\nstatus: %s\n", report.BeadID, report.Status)
	if report.Detail != "" {
		fmt.Fprintf(w, "detail: %s\n", report.Detail)
	}
	if report.ResultRev != "" {
		fmt.Fprintf(w, "result_rev: %s\n", report.ResultRev)
	}
	if report.PreserveRef != "" {
		fmt.Fprintf(w, "preserve_ref: %s\n", report.PreserveRef)
	}
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
func unmetDeps(store *bead.Store, b *bead.Bead) []string {
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

// singleBeadStore wraps a *bead.Store and overrides ReadyExecution to return
// only a single target bead. All other operations are delegated unchanged.
// This allows the execute-bead worker to treat the forced bead as if it were
// the only entry in the execution-ready queue.
type singleBeadStore struct {
	*bead.Store
	target bead.Bead
}

func (s *singleBeadStore) ReadyExecution() ([]bead.Bead, error) {
	return []bead.Bead{s.target}, nil
}
