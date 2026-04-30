package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/spf13/cobra"
)

var validBeadID = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// loadExecutionsMirrorConfig reads .ddx/config.yaml at projectRoot and returns
// the executions.mirror block when configured. Errors return nil silently —
// missing or invalid mirror config simply disables mirroring; it never blocks
// execute-bead.
func loadExecutionsMirrorConfig(projectRoot string) *config.ExecutionsMirrorConfig {
	cfg, err := config.LoadWithWorkingDir(projectRoot)
	if err != nil || cfg == nil || cfg.Executions == nil || cfg.Executions.Mirror == nil {
		return nil
	}
	return cfg.Executions.Mirror
}

// landingGitOpsFromFactory returns the agent.LandingGitOps the CommandFactory
// should use — either the test override or the default RealLandingGitOps.
func landingGitOpsFromFactory(f *CommandFactory) agent.LandingGitOps {
	if f.executeBeadLandingGitOverride != nil {
		return f.executeBeadLandingGitOverride
	}
	return agent.RealLandingGitOps{}
}

func (f *CommandFactory) newAgentExecuteBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-bead <bead-id>",
		Short: "Run an agent on one bead in an isolated worktree, then land the result",
		Long: `execute-bead is the primitive: it runs a single agent on a single bead in
an isolated git worktree (the worker step), then hands the result to the
parent-side orchestrator which merges or preserves the commits.

The worker classifies its outcome as one of:
  task_succeeded  — agent exited 0 and produced commits
  task_no_changes — agent exited 0 but made no commits
  task_failed     — agent exited non-zero

The orchestrator then applies landing logic (merge, gate evaluation, preserve)
and the final result reflects both the worker and landing decisions.

For normal queue-driven work use "ddx agent execute-loop", which claims
ready beads, calls this command, and closes or unclaims each bead based on
the result. Reach for execute-bead directly only to debug or re-run a
specific bead.

Result status is reported on the "status:" line and is one of:
  success                          — merged (or preserved with --no-merge)
  no_changes                       — agent ran but produced no diff
  no_evidence_produced             — agent exited with no commit and no rationale
  already_satisfied                — closed by the loop after repeated no_changes
  land_conflict                    — merge failed; result preserved
  post_run_check_failed            — checks failed; result preserved
  execution_failed                 — agent or harness error
  structural_validation_failed     — bead or prompt inputs invalid

execute-loop closes the bead with session/commit evidence only on success
(and already_satisfied); every other status leaves the bead open and
unclaimed for a later attempt.`,
		Example: `  # Debug one specific bead (prefer "execute-loop" for normal queue work)
  ddx agent execute-bead ddx-7a01ba6c

  # Run against a non-HEAD base revision
  ddx agent execute-bead ddx-7a01ba6c --from main

  # Preserve the result under refs/ddx/iterations/... instead of merging
  ddx agent execute-bead ddx-7a01ba6c --no-merge

  # Pin harness/model for a debugging pass
  ddx agent execute-bead ddx-7a01ba6c --harness codex`,
		Args: cobra.ExactArgs(1),
		RunE: f.runAgentExecuteBead,
	}
	cmd.Flags().String("project", "", "Project root path (default: CWD git root). Env: DDX_PROJECT_ROOT")
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().Bool("no-merge", false, "Skip merge; preserve result under refs/ddx/iterations/<bead-id>/<timestamp>-<base-shortsha> instead")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("provider", "", "Provider name (e.g. vidar, openrouter); selects a named provider from config")
	cmd.Flags().String("model-ref", "", "Model catalog reference (e.g. code-medium); resolved via the model catalog")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().String("context-budget", "", "Context budget for prompt: empty (full), minimal (omit large governing docs for cheap-tier)")
	cmd.Flags().String("prompt", "", "Prompt file path (auto-generated from bead if omitted)")
	cmd.Flags().Duration("request-timeout", 0, "Per-request provider wall-clock timeout (e.g. 60m, 2h); overrides project config and model-class defaults. Use when a thinking model needs more than the default 15 min.")
	cmd.Flags().Bool("json", false, "Output result as JSON")
	return cmd
}

func (f *CommandFactory) runAgentExecuteBead(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	if !validBeadID.MatchString(beadID) {
		return fmt.Errorf("invalid bead ID %q: must contain only letters, digits, dots, underscores, and hyphens", beadID)
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	fromRev, _ := cmd.Flags().GetString("from")
	noMerge, _ := cmd.Flags().GetBool("no-merge")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	modelRef, _ := cmd.Flags().GetString("model-ref")
	effort, _ := cmd.Flags().GetString("effort")
	contextBudget, _ := cmd.Flags().GetString("context-budget")
	promptFile, _ := cmd.Flags().GetString("prompt")
	requestTimeout, _ := cmd.Flags().GetDuration("request-timeout")
	asJSON, _ := cmd.Flags().GetBool("json")

	projectRoot := resolveProjectRoot(projectFlag, f.WorkingDir)

	overrides := config.CLIOverrides{
		Harness:       harness,
		Model:         model,
		Provider:      provider,
		ModelRef:      modelRef,
		Effort:        effort,
		ContextBudget: contextBudget,
	}
	if requestTimeout > 0 {
		overrides.ProviderRequestTimeout = &requestTimeout
	}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	if err != nil {
		return fmt.Errorf("load config at %s: %w", projectRoot, err)
	}
	runtime := agent.ExecuteBeadRuntime{
		FromRev:    fromRev,
		PromptFile: promptFile,
		WorkerID:   os.Getenv("DDX_WORKER_ID"),
		BeadEvents: bead.NewStore(filepath.Join(projectRoot, ".ddx")),
	}

	var gitOps agent.GitOps = &agent.RealGitOps{}
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}

	var orchestratorGitOps agent.OrchestratorGitOps = &agent.RealOrchestratorGitOps{}
	if f.executeBeadOrchestratorGitOverride != nil {
		orchestratorGitOps = f.executeBeadOrchestratorGitOverride
	}

	// Test injection seam: AgentRunnerOverride feeds canned *Result values
	// from fakes without spinning up a real provider. Production callers
	// leave this nil so ExecuteBead constructs an agent service from
	// projectRoot internally.
	if f.AgentRunnerOverride != nil {
		runtime.AgentRunner = f.AgentRunnerOverride
	}

	// Preflight the orphan-model check before creating a worktree. Mirrors
	// the execute-loop preflight so `ddx agent execute-bead --model <unroutable>`
	// errors before any git work happens rather than mid-execution. Skip the
	// preflight when a test runner override is in use — overrides may be
	// fakes that bypass routing entirely.
	if f.AgentRunnerOverride == nil {
		if err := agent.ValidateForExecuteLoopViaService(cmd.Context(), f.WorkingDir, harness, model, provider, modelRef); err != nil {
			return err
		}
	}

	// Recover any orphaned worktrees from previous crashed runs before
	// spawning a new worker. This is the parent's responsibility.
	agent.RecoverOrphans(gitOps, projectRoot, beadID)

	// Worker step: run the agent in an isolated worktree.
	res, err := agent.ExecuteBeadWithConfig(cmd.Context(), projectRoot, beadID, rcfg, runtime, gitOps)
	if err != nil && res == nil {
		return err
	}

	// Orchestrator step: land the result (ff or merge → push, gate eval,
	// preserve). Always run when we have a result — the orchestrator handles
	// all cases including no-changes and error (agent failed with no commits).
	//
	// Single-bead CLI is strictly serial (no sibling workers in this
	// process), so we can wire the LandingAdvancer to call Land() directly
	// without going through a per-project coordinator goroutine. Tests may
	// inject a custom advancer via f.executeBeadLandingAdvancerOverride.
	if res != nil {
		var advancer func(r *agent.ExecuteBeadResult) (*agent.LandResult, error)
		if f.executeBeadLandingAdvancerOverride != nil {
			advancer = f.executeBeadLandingAdvancerOverride
		} else {
			landingGitOps := landingGitOpsFromFactory(f)
			advancer = func(r *agent.ExecuteBeadResult) (*agent.LandResult, error) {
				return agent.Land(projectRoot, agent.BuildLandRequestFromResult(projectRoot, r), landingGitOps)
			}
		}
		landingOpts := agent.BeadLandingOptions{
			NoMerge:         noMerge,
			LandingAdvancer: advancer,
		}

		// Wire required execution gates: when the worker manifest declares
		// governing IDs, gate-eval them in an ephemeral worktree at ResultRev
		// before LandBeadResult decides merge vs preserve. The original
		// worker worktree was cleaned up by ExecuteBead, so we pin ResultRev
		// to a transient ref and check it out into a temp worktree for the
		// duration of gate evaluation.
		if res != nil && res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0 {
			wt, ids, cleanup, ctxErr := agent.BuildLandingGateContext(projectRoot, res, gitOps)
			if ctxErr != nil {
				// Soft-fail: log and skip gate eval rather than abort the land.
				fmt.Fprintf(os.Stderr, "ddx: warning: gate-context setup failed: %v (skipping required-gate eval)\n", ctxErr)
			} else if wt != "" {
				defer cleanup()
				landingOpts.WtPath = wt
				landingOpts.GovernIDs = ids
				landingOpts.ChecksArtifactPath = filepath.Join(projectRoot, res.ExecutionDir, "checks.json")
				landingOpts.ChecksArtifactRel = filepath.Join(res.ExecutionDir, "checks.json")
			}
		}

		if landing, landErr := agent.LandBeadResult(projectRoot, res, orchestratorGitOps, landingOpts); landErr == nil {
			agent.ApplyLandingToResult(res, landing)
		} else if err == nil {
			err = landErr
		}
	}

	if err != nil {
		_ = writeExecuteBeadResult(cmd, res, asJSON)
		return err
	}

	return writeExecuteBeadResult(cmd, res, asJSON)
}

func writeExecuteBeadResult(cmd *cobra.Command, res *agent.ExecuteBeadResult, asJSON bool) error {
	if res == nil {
		return nil
	}
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "bead:    %s\n", res.BeadID)
	fmt.Fprintf(cmd.OutOrStdout(), "base:    %s\n", res.BaseRev)
	if res.ResultRev != "" && res.ResultRev != res.BaseRev {
		fmt.Fprintf(cmd.OutOrStdout(), "result:  %s\n", res.ResultRev)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "outcome: %s\n", res.Outcome)
	fmt.Fprintf(cmd.OutOrStdout(), "status:  %s\n", res.Status)
	if res.Detail != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "detail:  %s\n", res.Detail)
	}
	if res.NoChangesRationale != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "rationale: %s\n", res.NoChangesRationale)
	}
	if res.PreserveRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "ref:     %s\n", res.PreserveRef)
	}
	return nil
}
