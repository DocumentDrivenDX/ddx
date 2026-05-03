package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
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

  # Execute without the post-merge review step
  ddx try ddx-abc12345 --no-review`,
		Args: cobra.ExactArgs(1),
		RunE: f.runTry,
	}

	// Routing knobs — identical to "ddx work" passthrough surface.
	cmd.Flags().String("project", "", "Target project root path or name (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().String("harness", "", "Agent harness constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("model", "", "Model constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("profile", agent.DefaultRoutingProfile, "Routing profile: default, cheap, fast, or smart")
	cmd.Flags().String("provider", "", "Provider constraint (passthrough; ddx try does not validate)")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().Bool("no-review", false, "Skip post-merge review")
	cmd.Flags().String("review-harness", "", "Harness for the post-merge reviewer (default: same as implementation harness)")
	cmd.Flags().String("review-model", "", "Model override for the post-merge reviewer (default: smart tier)")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout; overrides project config and model-class defaults")
	cmd.Flags().Int("min-power", 0, "Minimum model power required (0 = unconstrained)")
	cmd.Flags().Int("max-power", 0, "Maximum model power allowed (0 = unconstrained)")

	return cmd
}

// runTry is the RunE for "ddx try <bead-id>".
func (f *CommandFactory) runTry(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	projectFlag, _ := cmd.Flags().GetString("project")
	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)
	fromRev, _ := cmd.Flags().GetString("from")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	profile, _ := cmd.Flags().GetString("profile")
	provider, _ := cmd.Flags().GetString("provider")
	modelRef, _ := cmd.Flags().GetString("model-ref")
	effort, _ := cmd.Flags().GetString("effort")
	noReview, _ := cmd.Flags().GetBool("no-review")
	reviewHarness, _ := cmd.Flags().GetString("review-harness")
	reviewModel, _ := cmd.Flags().GetString("review-model")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	minPower, _ := cmd.Flags().GetInt("min-power")
	maxPower, _ := cmd.Flags().GetInt("max-power")

	store := bead.NewStore(filepath.Join(projectRoot, ".ddx"))

	// Pre-flight: look up the bead.
	target, err := store.Get(beadID)
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

	// Session log tailer so the operator sees progress.
	tailCtx, tailCancel := context.WithCancel(context.Background())
	go agent.TailSessionLogs(tailCtx, projectRoot, cmd.OutOrStdout())

	// Process-local LandCoordinator.
	localCoord := serverpkg.NewLocalLandCoordinator(projectRoot, agent.RealLandingGitOps{})
	defer localCoord.Stop()

	// Post-merge reviewer (on by default, skipped via --no-review).
	var reviewer agent.BeadReviewer
	if !noReview {
		reviewer = &agent.DefaultBeadReviewer{
			ProjectRoot: projectRoot,
			BeadStore:   bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			BeadEvents:  bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			Harness:     reviewHarness,
			Model:       reviewModel,
		}
	}

	profile = agent.NormalizeRoutingProfile(profile)

	// Determine whether zero-config auto-route applies (same logic as runWork).
	noRoutingFlags := harness == "" && model == "" && provider == "" && modelRef == "" &&
		!cmd.Flags().Changed("profile")
	autoInferTier := noRoutingFlags && !projectHasRoutingConfig(projectRoot)

	// Build the executor: either the test override or the real single-tier executor.
	var executor agent.ExecuteBeadExecutor
	if f.tryExecutorOverride != nil {
		executor = f.tryExecutorOverride
	} else {
		executor = agent.ExecuteBeadExecutorFunc(func(ctx context.Context, execBeadID string) (agent.ExecuteBeadReport, error) {
			var inferredTier escalation.ModelTier
			if autoInferTier {
				if b, getErr := store.Get(execBeadID); getErr == nil {
					inferredTier = escalation.InferTier(b)
				} else {
					inferredTier = escalation.TierCheap
				}
			}

			attemptProfile := profile
			if autoInferTier && inferredTier != "" {
				attemptProfile = escalation.TierToProfile(inferredTier)
			}

			loopOverrides := config.CLIOverrides{
				Harness:           harness,
				Model:             model,
				Provider:          provider,
				ModelRef:          modelRef,
				Profile:           attemptProfile,
				Effort:            effort,
				MinPower:          minPower,
				MaxPower:          maxPower,
				OpaquePassthrough: true, // ddx try treats flags as opaque passthrough
			}
			if requestTimeout > 0 {
				loopOverrides.ProviderRequestTimeout = &requestTimeout
			}
			attemptRcfg, _ := config.LoadAndResolve(projectRoot, loopOverrides)
			gitOps := &agent.RealGitOps{}
			res, execErr := agent.ExecuteBeadWithConfig(ctx, projectRoot, execBeadID, attemptRcfg, agent.ExecuteBeadRuntime{
				FromRev:    fromRev,
				BeadEvents: bead.NewStore(filepath.Join(projectRoot, ".ddx")),
			}, gitOps)
			if execErr != nil && res == nil {
				return agent.ExecuteBeadReport{}, execErr
			}
			if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
				targetBead, _ := store.Get(execBeadID)
				landRes, _, landErr := agent.SubmitWithPreMergeChecks(
					ctx, projectRoot, targetBead, res,
					func(req agent.LandRequest) (*agent.LandResult, error) { return localCoord.Submit(req) },
					bead.NewStore(filepath.Join(projectRoot, ".ddx")),
					resolveClaimAssignee(), "ddx try",
					nil,
				)
				if landErr == nil {
					agent.ApplyLandResultToExecuteBeadResult(res, landRes)
				} else if execErr == nil {
					execErr = landErr
				}
			} else if res != nil && res.ResultRev == res.BaseRev {
				res.Outcome = "no-changes"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			} else if res != nil && res.ExitCode != 0 {
				res.Outcome = "preserved"
				res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
			}
			if execErr != nil {
				return agent.ExecuteBeadReport{}, execErr
			}
			tierStr := ""
			if inferredTier != "" {
				tierStr = string(inferredTier)
			}
			return agent.ExecuteBeadReport{
				BeadID:             res.BeadID,
				AttemptID:          res.AttemptID,
				WorkerID:           res.WorkerID,
				Harness:            res.Harness,
				Provider:           res.Provider,
				Model:              res.Model,
				ActualPower:        res.ActualPower,
				Tier:               tierStr,
				Status:             res.Status,
				Detail:             res.Detail,
				SessionID:          res.SessionID,
				BaseRev:            res.BaseRev,
				ResultRev:          res.ResultRev,
				PreserveRef:        res.PreserveRef,
				NoChangesRationale: res.NoChangesRationale,
				CostUSD:            res.CostUSD,
				DurationMS:         int64(res.DurationMS),
			}, nil
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
		ModelRef: modelRef,
		Profile:  profile,
		Effort:   effort,
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		tailCancel()
		return fmt.Errorf("load resolved config: %w", err)
	}

	cliLandingOps := agent.RealLandingGitOps{}
	result, runErr := worker.Run(cmd.Context(), rcfg, agent.ExecuteBeadLoopRuntime{
		Once:         true,
		Log:          cmd.OutOrStdout(),
		EventSink:    loopSink,
		WorkerID:     resolveClaimAssignee(),
		ProjectRoot:  projectRoot,
		SessionID:    loopSessionID,
		PreClaimHook: buildCLIPreClaimHook(projectRoot, cliLandingOps),
		NoReview:     noReview,
	})
	tailCancel()
	if runErr != nil {
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
	fmt.Fprintf(cmd.OutOrStdout(), "\nbead: %s\nstatus: %s\n", report.BeadID, report.Status)
	if report.Detail != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "detail: %s\n", report.Detail)
	}
	if report.ResultRev != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "result_rev: %s\n", report.ResultRev)
	}
	if report.PreserveRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "preserve_ref: %s\n", report.PreserveRef)
	}

	return tryExitCodeForStatus(report.Status)
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
		dep, err := store.Get(depID)
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
