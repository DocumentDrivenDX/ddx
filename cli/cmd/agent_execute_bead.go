package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/cobra"
)

// ExecuteBeadResult captures the complete outcome of an execute-bead run.
type ExecuteBeadResult struct {
	BeadID      string    `json:"bead_id"`
	BaseRev     string    `json:"base_rev"`
	ResultRev   string    `json:"result_rev,omitempty"`
	Outcome     string    `json:"outcome"` // merged | preserved | no-changes
	PreserveRef string    `json:"preserve_ref,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Harness     string    `json:"harness,omitempty"`
	Model       string    `json:"model,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	DurationMS  int       `json:"duration_ms"`
	Tokens      int       `json:"tokens,omitempty"`
	CostUSD     float64   `json:"cost_usd,omitempty"`
	ExitCode    int       `json:"exit_code"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

// executeBeadGitOps abstracts git operations to enable dependency injection in tests.
type executeBeadGitOps interface {
	HeadRev(dir string) (string, error)
	ResolveRev(dir, rev string) (string, error)
	IsDirty(dir string) (bool, error)
	Stash(dir string) error
	WorktreeAdd(dir, wtPath, rev string) error
	WorktreeRemove(dir, wtPath string) error
	WorktreeList(dir string) ([]string, error)
	WorktreePrune(dir string) error
	FFMerge(dir, rev string) error
	UpdateRef(dir, ref, sha string) error
}

// realExecuteBeadGit implements executeBeadGitOps via os/exec git commands.
type realExecuteBeadGit struct{}

func (r *realExecuteBeadGit) HeadRev(dir string) (string, error) {
	return r.ResolveRev(dir, "HEAD")
}

func (r *realExecuteBeadGit) ResolveRev(dir, rev string) (string, error) {
	out, err := osexec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *realExecuteBeadGit) IsDirty(dir string) (bool, error) {
	out, err := osexec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (r *realExecuteBeadGit) Stash(dir string) error {
	out, err := osexec.Command("git", "-C", dir, "stash").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *realExecuteBeadGit) WorktreeAdd(dir, wtPath, rev string) error {
	out, err := osexec.Command("git", "-C", dir, "worktree", "add", "--detach", wtPath, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *realExecuteBeadGit) WorktreeRemove(dir, wtPath string) error {
	// best effort; worktree may already be gone
	_ = osexec.Command("git", "-C", dir, "worktree", "remove", "--force", wtPath).Run()
	return nil
}

func (r *realExecuteBeadGit) WorktreeList(dir string) ([]string, error) {
	out, err := osexec.Command("git", "-C", dir, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

func (r *realExecuteBeadGit) WorktreePrune(dir string) error {
	return osexec.Command("git", "-C", dir, "worktree", "prune").Run()
}

func (r *realExecuteBeadGit) FFMerge(dir, rev string) error {
	out, err := osexec.Command("git", "-C", dir, "merge", "--ff-only", rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ff-merge: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *realExecuteBeadGit) UpdateRef(dir, ref, sha string) error {
	out, err := osexec.Command("git", "-C", dir, "update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// executeBeadWtDir is the subdirectory under .ddx used for managed worktrees.
const executeBeadWtDir = ".ddx"

// executeBeadWtPrefix is the filename prefix for managed execute-bead worktrees.
const executeBeadWtPrefix = ".execute-bead-wt-"

// executeBeadAttemptID generates a unique attempt identifier with timestamp and random suffix.
func executeBeadAttemptID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

// executeBeadSessionID generates a short session ID for the agent log.
func executeBeadSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "eb-" + hex.EncodeToString(b)
}

func (f *CommandFactory) newAgentExecuteBeadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute-bead <bead-id>",
		Short: "Run an agent on a bead in an isolated worktree, then merge or preserve the result",
		Long: `execute-bead creates an isolated git worktree at a chosen base revision,
runs an agent within it, then either lands the result via fast-forward merge
or preserves it under a hidden ref (refs/ddx/execute-bead/<bead-id>/<attempt>).

Dirty worktrees are checkpointed via git stash before execution so no local
changes are discarded. Orphan worktrees from previous crashed runs are
recovered automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: f.runAgentExecuteBead,
	}
	cmd.Flags().String("from", "", "Base git revision to start from (default: HEAD)")
	cmd.Flags().Bool("no-merge", false, "Skip merge; preserve result under a hidden ref instead")
	cmd.Flags().String("harness", "", "Agent harness to use")
	cmd.Flags().String("model", "", "Model override")
	cmd.Flags().String("effort", "", "Effort level")
	cmd.Flags().String("prompt", "", "Prompt file path (auto-generated from bead if omitted)")
	cmd.Flags().Bool("json", false, "Output result as JSON")
	return cmd
}

func (f *CommandFactory) runAgentExecuteBead(cmd *cobra.Command, args []string) error {
	gitOps := executeBeadGitOps(&realExecuteBeadGit{})
	if f.executeBeadGitOverride != nil {
		gitOps = f.executeBeadGitOverride
	}
	return f.runAgentExecuteBeadWith(cmd, args, gitOps)
}

// beadAgentRunner is a local interface matching ddxexec.AgentRunner and *agent.Runner.
type beadAgentRunner interface {
	Run(opts agent.RunOptions) (*agent.Result, error)
}

var validBeadID = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (f *CommandFactory) runAgentExecuteBeadWith(cmd *cobra.Command, args []string, gitOps executeBeadGitOps) error {
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

	workDir := f.WorkingDir
	attemptID := executeBeadAttemptID()

	// Recover orphan worktrees from previous crashed runs.
	f.executeBeadRecoverOrphans(gitOps, workDir, beadID)

	// Resolve base revision (default to HEAD).
	baseRev, err := f.executeBeadResolveBase(gitOps, workDir, fromRev)
	if err != nil {
		return err
	}

	// Checkpoint dirty worktree without discarding changes.
	dirty, err := gitOps.IsDirty(workDir)
	if err != nil {
		return fmt.Errorf("checking worktree state: %w", err)
	}
	if dirty {
		if stashErr := gitOps.Stash(workDir); stashErr != nil {
			return fmt.Errorf("checkpointing dirty worktree: %w", stashErr)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "note: dirty worktree stashed before execution")
	}

	// Create isolated managed worktree at base rev.
	wtPath := filepath.Join(workDir, executeBeadWtDir, executeBeadWtPrefix+beadID+"-"+attemptID)
	if addErr := gitOps.WorktreeAdd(workDir, wtPath, baseRev); addErr != nil {
		return fmt.Errorf("creating isolated worktree: %w", addErr)
	}
	defer func() { _ = gitOps.WorktreeRemove(workDir, wtPath) }()

	// Select agent runner; prefer AgentRunnerOverride for testability.
	var runner beadAgentRunner
	if f.AgentRunnerOverride != nil {
		runner = f.AgentRunnerOverride
	} else {
		runner = f.agentRunner()
	}

	// Build prompt; fall back to bead-scoped message if no file provided.
	prompt := ""
	if promptFile == "" {
		prompt = fmt.Sprintf("Work on bead %s.", beadID)
	}

	sessionID := executeBeadSessionID()
	startedAt := time.Now().UTC()

	agentResult, agentErr := runner.Run(agent.RunOptions{
		Harness:    harness,
		Prompt:     prompt,
		PromptFile: promptFile,
		Model:      model,
		Effort:     effort,
		WorkDir:    wtPath,
		Correlation: map[string]string{
			"bead_id":    beadID,
			"base_rev":   baseRev,
			"attempt_id": attemptID,
			"session_id": sessionID,
		},
	})
	finishedAt := time.Now().UTC()

	exitCode := 0
	tokens := 0
	costUSD := 0.0
	resultModel := model
	resultHarness := harness
	if agentResult != nil {
		exitCode = agentResult.ExitCode
		tokens = agentResult.Tokens
		costUSD = agentResult.CostUSD
		if agentResult.Model != "" {
			resultModel = agentResult.Model
		}
		if agentResult.Harness != "" {
			resultHarness = agentResult.Harness
		}
	}
	agentErrMsg := ""
	if agentErr != nil {
		exitCode = 1
		agentErrMsg = agentErr.Error()
	}

	// Get the HEAD of the worktree after the agent ran.
	resultRev, revErr := gitOps.HeadRev(wtPath)
	if revErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to read worktree HEAD: %v\n", revErr)
		res := ExecuteBeadResult{
			BeadID:     beadID,
			BaseRev:    baseRev,
			Harness:    resultHarness,
			Model:      resultModel,
			SessionID:  sessionID,
			DurationMS: int(finishedAt.Sub(startedAt).Milliseconds()),
			Tokens:     tokens,
			CostUSD:    costUSD,
			ExitCode:   1,
			Error:      agentErrMsg,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Outcome:    "error",
			Reason:     fmt.Sprintf("failed to read worktree HEAD: %v", revErr),
		}
		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "bead:    %s\n", res.BeadID)
		fmt.Fprintf(cmd.OutOrStdout(), "base:    %s\n", res.BaseRev)
		fmt.Fprintf(cmd.OutOrStdout(), "outcome: %s\n", res.Outcome)
		fmt.Fprintf(cmd.OutOrStdout(), "reason:  %s\n", res.Reason)
		return fmt.Errorf("failed to read worktree HEAD: %w", revErr)
	}

	res := ExecuteBeadResult{
		BeadID:     beadID,
		BaseRev:    baseRev,
		ResultRev:  resultRev,
		Harness:    resultHarness,
		Model:      resultModel,
		SessionID:  sessionID,
		DurationMS: int(finishedAt.Sub(startedAt).Milliseconds()),
		Tokens:     tokens,
		CostUSD:    costUSD,
		ExitCode:   exitCode,
		Error:      agentErrMsg,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	// Determine outcome: no-changes, merge, or preserve.
	switch {
	case resultRev == baseRev:
		res.Outcome = "no-changes"
		res.Reason = "agent made no commits"

	case noMerge:
		ref := fmt.Sprintf("refs/ddx/execute-bead/%s/%s", beadID, attemptID)
		if updateErr := gitOps.UpdateRef(workDir, ref, resultRev); updateErr != nil {
			return fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "--no-merge specified"

	default:
		if mergeErr := gitOps.FFMerge(workDir, resultRev); mergeErr == nil {
			res.Outcome = "merged"
		} else {
			ref := fmt.Sprintf("refs/ddx/execute-bead/%s/%s", beadID, attemptID)
			if updateErr := gitOps.UpdateRef(workDir, ref, resultRev); updateErr != nil {
				return fmt.Errorf("preserving result ref: %w", updateErr)
			}
			res.Outcome = "preserved"
			res.PreserveRef = ref
			res.Reason = "ff-merge not possible"
		}
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
	if res.Reason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "reason:  %s\n", res.Reason)
	}
	if res.PreserveRef != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "ref:     %s\n", res.PreserveRef)
	}
	return nil
}

// executeBeadResolveBase resolves the --from flag to a full git SHA.
// If fromRev is empty or "HEAD", resolves the current HEAD.
func (f *CommandFactory) executeBeadResolveBase(gitOps executeBeadGitOps, workDir, fromRev string) (string, error) {
	if fromRev == "" || fromRev == "HEAD" {
		rev, err := gitOps.HeadRev(workDir)
		if err != nil {
			return "", fmt.Errorf("resolving HEAD: %w", err)
		}
		return rev, nil
	}
	rev, err := gitOps.ResolveRev(workDir, fromRev)
	if err != nil {
		return "", fmt.Errorf("resolving --from %q: %w", fromRev, err)
	}
	return rev, nil
}

// executeBeadRecoverOrphans removes any abandoned execute-bead worktrees for beadID
// left by previous crashed runs, then prunes stale git worktree metadata.
func (f *CommandFactory) executeBeadRecoverOrphans(gitOps executeBeadGitOps, workDir, beadID string) {
	paths, err := gitOps.WorktreeList(workDir)
	if err != nil {
		return
	}
	prefix := filepath.Join(workDir, executeBeadWtDir, executeBeadWtPrefix+beadID+"-")
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			_ = gitOps.WorktreeRemove(workDir, p)
		}
	}
	_ = gitOps.WorktreePrune(workDir)
}
