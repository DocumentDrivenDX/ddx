package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

var validBeadID = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (f *CommandFactory) newAgentExecuteBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-bead <bead-id>",
		Short: "Run an agent on one bead in an isolated worktree, then merge or preserve the result",
		Long: `execute-bead is the primitive: it runs a single agent on a single bead in
an isolated git worktree, then merges the result back (fast-forward if clean,
merge commit if not) or preserves it under refs/ddx/iterations/<bead-id>/...
when --no-merge is set. Orphan worktrees from previous crashed runs are
recovered automatically.

Planning and document-only beads are valid execution targets — the agent
produces whatever artifacts (docs, specs, code) the bead's acceptance
criteria call for.

For normal queue-driven work use "ddx agent execute-loop", which claims
ready beads, calls this command, and closes or unclaims each bead based on
the result. Reach for execute-bead directly only to debug or re-run a
specific bead.

Result status is reported on the "status:" line and is one of:
  success                          — merged (or preserved with --no-merge)
  no_changes                       — agent ran but produced no diff
  already_satisfied                — closed by the loop after repeated no_changes
  land_conflict                    — rebase/merge failed; result preserved
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
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().Bool("no-merge", false, "Skip merge; preserve result under refs/ddx/iterations/<bead-id>/<timestamp>-<base-shortsha> instead")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().String("prompt", "", "Prompt file path (auto-generated from bead if omitted)")
	cmd.Flags().Bool("json", false, "Output result as JSON")
	return cmd
}

func (f *CommandFactory) runAgentExecuteBead(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	if !validBeadID.MatchString(beadID) {
		return fmt.Errorf("invalid bead ID %q: must contain only letters, digits, dots, underscores, and hyphens", beadID)
	}
	fromRev, _ := cmd.Flags().GetString("from")
	noMerge, _ := cmd.Flags().GetBool("no-merge")
	harness, _ := cmd.Flags().GetString("harness")
	model, _ := cmd.Flags().GetString("model")
	effort, _ := cmd.Flags().GetString("effort")
	promptFile, _ := cmd.Flags().GetString("prompt")
	asJSON, _ := cmd.Flags().GetBool("json")

	opts := agent.ExecuteBeadOptions{
		FromRev:    fromRev,
		NoMerge:    noMerge,
		Harness:    harness,
		Model:      model,
		Effort:     effort,
		PromptFile: promptFile,
		WorkerID:   os.Getenv("DDX_WORKER_ID"),
	}

	var gitOps agent.GitOps = &agent.RealGitOps{}
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}

	var runner agent.AgentRunner
	if f.AgentRunnerOverride != nil {
		runner = f.AgentRunnerOverride
	} else {
		runner = f.agentRunner()
	}

	res, err := agent.ExecuteBead(f.WorkingDir, beadID, opts, gitOps, runner)
	if err != nil && res == nil {
		return err
	}
	if err != nil {
		// Partial result available — display it, then return the error
		_ = writeExecuteBeadResult(cmd, res, asJSON)
		return err
	}

	return writeExecuteBeadResult(cmd, res, asJSON)
}

func writeExecuteBeadResult(cmd *cobra.Command, res *agent.ExecuteBeadResult, asJSON bool) error {
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
	if res.PreserveRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "ref:     %s\n", res.PreserveRef)
	}
	return nil
}
