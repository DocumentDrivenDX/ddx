package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

var validBeadID = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (f *CommandFactory) newAgentExecuteBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-bead <bead-id>",
		Short: "Run an agent on a bead in an isolated worktree, then merge or preserve the result",
		Long: `execute-bead creates an isolated git worktree from HEAD,
runs an agent within it, then merges the result back (ff if clean, merge commit if not).
Orphan worktrees from previous crashed runs are recovered automatically.`,
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

// ExecuteBeadResultFromReport converts a loop report into the CLI display type.
// Kept for backward compatibility with execute-loop's old inline path.
func ExecuteBeadResultFromReport(report agent.ExecuteBeadReport) agent.ExecuteBeadResult {
	return agent.ExecuteBeadResult{
		BeadID:      report.BeadID,
		AttemptID:   report.AttemptID,
		WorkerID:    report.WorkerID,
		Harness:     report.Harness,
		Provider:    report.Provider,
		Model:       report.Model,
		Status:      report.Status,
		Detail:      report.Detail,
		SessionID:   report.SessionID,
		BaseRev:     report.BaseRev,
		ResultRev:   report.ResultRev,
		PreserveRef: report.PreserveRef,
		Outcome:     report.Status,
	}
}

// Legacy helpers kept for test compatibility

type executeBeadGitOps = agent.GitOps

type realExecuteBeadGit = agent.RealGitOps

// beadAgentRunner is a local interface matching *agent.Runner.
type beadAgentRunner interface {
	Run(opts agent.RunOptions) (*agent.Result, error)
}

// executeBeadAttemptID is kept for test compatibility.
func executeBeadAttemptID() string {
	return agent.GenerateAttemptID()
}

// executeBeadSessionID is kept for test compatibility.
func executeBeadSessionID() string {
	return agent.GenerateSessionID()
}

// executeBeadCommitTracker is kept for test compatibility.
func executeBeadCommitTracker(workDir string, w fmt.Stringer) error {
	return agent.CommitTracker(workDir)
}

// Old type aliases for test compatibility
type ExecuteBeadResult = agent.ExecuteBeadResult

// These are needed by agent_cmd.go's invokeExecuteBeadFromLoop
// which we'll also refactor, but keep them compiling for now.
const (
	executeBeadWtDir       = agent.ExecuteBeadWtDir
	executeBeadWtPrefix    = agent.ExecuteBeadWtPrefix
	executeBeadArtifactDir = agent.ExecuteBeadArtifactDir
)

// executeBeadNow is kept for test compatibility.
var executeBeadNow = time.Now

func init() {
	// Ensure old references still compile
	_ = strings.TrimSpace
	_ = time.Now
}
