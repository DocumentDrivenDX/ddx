package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/spf13/cobra"
)

// ExecuteBeadResult captures the complete outcome of an execute-bead run.
type ExecuteBeadResult struct {
	BeadID       string    `json:"bead_id"`
	BaseRev      string    `json:"base_rev"`
	ResultRev    string    `json:"result_rev,omitempty"`
	Outcome      string    `json:"outcome"` // merged | preserved | no-changes
	Status       string    `json:"status,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	PreserveRef  string    `json:"preserve_ref,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Harness      string    `json:"harness,omitempty"`
	Model        string    `json:"model,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	DurationMS   int       `json:"duration_ms"`
	Tokens       int       `json:"tokens,omitempty"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	ExitCode     int       `json:"exit_code"`
	Error        string    `json:"error,omitempty"`
	ExecutionDir string    `json:"execution_dir,omitempty"`
	PromptFile   string    `json:"prompt_file,omitempty"`
	ManifestFile string    `json:"manifest_file,omitempty"`
	ResultFile   string    `json:"result_file,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
}

type executeBeadArtifacts struct {
	DirAbs      string
	DirRel      string
	PromptAbs   string
	PromptRel   string
	ManifestAbs string
	ManifestRel string
	ResultAbs   string
	ResultRel   string
}

type executeBeadManifest struct {
	AttemptID string                    `json:"attempt_id"`
	BeadID    string                    `json:"bead_id"`
	BaseRev   string                    `json:"base_rev"`
	CreatedAt time.Time                 `json:"created_at"`
	Requested executeBeadRequested      `json:"requested"`
	Bead      executeBeadManifestBead   `json:"bead"`
	Governing []executeBeadGoverningRef `json:"governing,omitempty"`
	Paths     executeBeadArtifactPaths  `json:"paths"`
}

type executeBeadRequested struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

type executeBeadManifestBead struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Acceptance  string         `json:"acceptance,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type executeBeadGoverningRef struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
}

type executeBeadArtifactPaths struct {
	Dir      string `json:"dir"`
	Prompt   string `json:"prompt"`
	Manifest string `json:"manifest"`
	Result   string `json:"result"`
	Worktree string `json:"worktree"`
}

// executeBeadGitOps abstracts git operations to enable dependency injection in tests.
type executeBeadGitOps interface {
	HeadRev(dir string) (string, error)
	ResolveRev(dir, rev string) (string, error)
	IsDirty(dir string) (bool, error)
	CheckpointCommit(dir, ref, message string) (string, error)
	WorktreeAdd(dir, wtPath, rev string) error
	WorktreeRemove(dir, wtPath string) error
	WorktreeList(dir string) ([]string, error)
	WorktreePrune(dir string) error
	Rebase(wtPath, ontoRev string) error
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

func (r *realExecuteBeadGit) CheckpointCommit(dir, ref, message string) (string, error) {
	ddxDir := filepath.Join(dir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		return "", fmt.Errorf("checkpoint mkdir: %w", err)
	}
	tmpIndex := filepath.Join(ddxDir, fmt.Sprintf(".execute-bead-index-%d-%d", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(tmpIndex)

	env := append(os.Environ(), "GIT_INDEX_FILE="+tmpIndex)

	readTree := osexec.Command("git", "-C", dir, "read-tree", "HEAD")
	readTree.Env = env
	if out, err := readTree.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git read-tree HEAD: %s: %w", strings.TrimSpace(string(out)), err)
	}

	addAll := osexec.Command("git", "-C", dir, "add", "-A")
	addAll.Env = env
	if out, err := addAll.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add -A: %s: %w", strings.TrimSpace(string(out)), err)
	}

	writeTree := osexec.Command("git", "-C", dir, "write-tree")
	writeTree.Env = env
	treeOut, err := writeTree.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git write-tree: %s: %w", strings.TrimSpace(string(treeOut)), err)
	}
	tree := strings.TrimSpace(string(treeOut))

	parent, err := r.HeadRev(dir)
	if err != nil {
		return "", err
	}

	commitTree := osexec.Command("git", "-C", dir, "commit-tree", tree, "-p", parent, "-m", message)
	commitTree.Env = env
	commitOut, err := commitTree.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit-tree: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	commit := strings.TrimSpace(string(commitOut))

	if err := r.UpdateRef(dir, ref, commit); err != nil {
		return "", err
	}
	return commit, nil
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

func (r *realExecuteBeadGit) Rebase(wtPath, ontoRev string) error {
	out, err := osexec.Command("git", "-C", wtPath, "rebase", ontoRev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git rebase: %s: %w", strings.TrimSpace(string(out)), err)
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

// executeBeadArtifactDir is the tracked artifact directory for execute-bead attempts.
const executeBeadArtifactDir = ".ddx/executions"

// executeBeadNow returns the current time for execute-bead ref naming.
// Tests override this for deterministic hidden-ref assertions.
var executeBeadNow = time.Now

// executeBeadAttemptID generates a unique attempt identifier with timestamp and random suffix.
func executeBeadAttemptID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

// executeBeadPreserveRef builds the documented hidden ref for a preserved iteration.
func executeBeadPreserveRef(beadID, baseRev string) string {
	shortSHA := baseRev
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	timestamp := executeBeadNow().UTC().Format("20060102T150405Z")
	return fmt.Sprintf("refs/ddx/iterations/%s/%s-%s", beadID, timestamp, shortSHA)
}

func executeBeadCheckpointRef(beadID, attemptID string) string {
	return fmt.Sprintf("refs/ddx/checkpoints/%s/%s", beadID, attemptID)
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
or preserves it under a hidden ref (refs/ddx/iterations/<bead-id>/<timestamp>-<base-shortsha>).

Dirty worktrees are checkpointed into an immutable hidden commit before
execution so no local changes are discarded. Orphan worktrees from previous crashed runs are
recovered automatically.`,
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

	// Resolve base revision (default to HEAD).
	baseRev, err := f.executeBeadResolveBase(gitOps, workDir, fromRev)
	if err != nil {
		return err
	}

	wtPath := filepath.Join(workDir, executeBeadWtDir, executeBeadWtPrefix+beadID+"-"+attemptID)
	rootLock := bead.NewStoreWithCollection(filepath.Join(workDir, ".ddx"), "execute-bead-root")
	if err := rootLock.WithLock(func() error {
		// Recover orphan worktrees from previous crashed runs while root git state is locked.
		f.executeBeadRecoverOrphans(gitOps, workDir, beadID)

		dirty, err := gitOps.IsDirty(workDir)
		if err != nil {
			return fmt.Errorf("checking worktree state: %w", err)
		}
		if dirty {
			checkpointRef := executeBeadCheckpointRef(beadID, attemptID)
			checkpointRev, checkpointErr := gitOps.CheckpointCommit(workDir, checkpointRef, fmt.Sprintf("ddx execute-bead checkpoint %s %s", beadID, attemptID))
			if checkpointErr != nil {
				return fmt.Errorf("checkpointing dirty worktree: %w", checkpointErr)
			}
			baseRev = checkpointRev
			fmt.Fprintf(cmd.ErrOrStderr(), "note: dirty worktree checkpointed at %s\n", checkpointRef)
		}

		if addErr := gitOps.WorktreeAdd(workDir, wtPath, baseRev); addErr != nil {
			return fmt.Errorf("creating isolated worktree: %w", addErr)
		}
		return nil
	}); err != nil {
		return err
	}
	defer func() {
		_ = gitOps.WorktreeRemove(workDir, wtPath)
	}()

	artifacts, err := f.prepareExecuteBeadArtifacts(wtPath, beadID, attemptID, baseRev, promptFile, harness, model, effort)
	if err != nil {
		return err
	}
	promptFile = artifacts.PromptAbs

	// Select agent runner; prefer AgentRunnerOverride for testability.
	var runner beadAgentRunner
	if f.AgentRunnerOverride != nil {
		runner = f.AgentRunnerOverride
	} else {
		runner = f.agentRunner()
	}

	sessionID := executeBeadSessionID()
	startedAt := time.Now().UTC()

	agentResult, agentErr := runner.Run(agent.RunOptions{
		Harness:    harness,
		Prompt:     "",
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
	agentErrMsg := ""
	if agentResult != nil {
		exitCode = agentResult.ExitCode
		tokens = agentResult.Tokens
		costUSD = agentResult.CostUSD
		if agentResult.Error != "" {
			agentErrMsg = agentResult.Error
		}
		if agentResult.Model != "" {
			resultModel = agentResult.Model
		}
		if agentResult.Harness != "" {
			resultHarness = agentResult.Harness
		}
	}
	if agentErr != nil {
		if exitCode == 0 {
			exitCode = 1
		}
		agentErrMsg = agentErr.Error()
	}
	executionFailed := exitCode != 0

	// Get the HEAD of the worktree after the agent ran.
	resultRev, revErr := gitOps.HeadRev(wtPath)
	if revErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to read worktree HEAD: %v\n", revErr)
		res := ExecuteBeadResult{
			BeadID:       beadID,
			BaseRev:      baseRev,
			Harness:      resultHarness,
			Model:        resultModel,
			SessionID:    sessionID,
			DurationMS:   int(finishedAt.Sub(startedAt).Milliseconds()),
			Tokens:       tokens,
			CostUSD:      costUSD,
			ExitCode:     1,
			Error:        agentErrMsg,
			ExecutionDir: artifacts.DirRel,
			PromptFile:   artifacts.PromptRel,
			ManifestFile: artifacts.ManifestRel,
			ResultFile:   artifacts.ResultRel,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			Outcome:      "error",
			Reason:       fmt.Sprintf("failed to read worktree HEAD: %v", revErr),
		}
		populateExecuteBeadStatus(&res)
		_ = writeExecuteBeadArtifactJSON(artifacts.ResultAbs, res)
		_ = writeExecuteBeadResult(cmd, res, asJSON)
		return fmt.Errorf("failed to read worktree HEAD: %w", revErr)
	}

	res := ExecuteBeadResult{
		BeadID:       beadID,
		BaseRev:      baseRev,
		ResultRev:    resultRev,
		Harness:      resultHarness,
		Model:        resultModel,
		SessionID:    sessionID,
		DurationMS:   int(finishedAt.Sub(startedAt).Milliseconds()),
		Tokens:       tokens,
		CostUSD:      costUSD,
		ExitCode:     exitCode,
		Error:        agentErrMsg,
		ExecutionDir: artifacts.DirRel,
		PromptFile:   artifacts.PromptRel,
		ManifestFile: artifacts.ManifestRel,
		ResultFile:   artifacts.ResultRel,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}

	// Determine outcome: no-changes, merge, or preserve.
	switch {
	case executionFailed && resultRev == baseRev:
		res.Outcome = "error"
		if agentErrMsg != "" {
			res.Reason = agentErrMsg
		} else {
			res.Reason = "agent execution failed"
		}

	case resultRev == baseRev:
		res.Outcome = "no-changes"
		res.Reason = "agent made no commits"

	case executionFailed:
		ref := executeBeadPreserveRef(beadID, baseRev)
		if updateErr := gitOps.UpdateRef(workDir, ref, resultRev); updateErr != nil {
			return fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "agent execution failed"

	case noMerge:
		ref := executeBeadPreserveRef(beadID, baseRev)
		if updateErr := gitOps.UpdateRef(workDir, ref, resultRev); updateErr != nil {
			return fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "--no-merge specified"

	default:
		// Get current HEAD of the target branch to rebase onto.
		currentHead, headErr := gitOps.HeadRev(workDir)
		if headErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to read current HEAD: %v\n", headErr)
			currentHead = baseRev
		}

		// If the target branch has advanced since our base, rebase the worktree commits
		// before attempting a fast-forward land.
		mergeRev := resultRev
		rebased := false
		if currentHead != baseRev {
			if rebaseErr := gitOps.Rebase(wtPath, currentHead); rebaseErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: rebase failed: %v\n", rebaseErr)
				ref := executeBeadPreserveRef(beadID, baseRev)
				if updateErr := gitOps.UpdateRef(workDir, ref, resultRev); updateErr != nil {
					return fmt.Errorf("preserving result ref: %w", updateErr)
				}
				res.Outcome = "preserved"
				res.PreserveRef = ref
				res.Reason = "rebase failed"
				populateExecuteBeadStatus(&res)
				if err := writeExecuteBeadArtifactJSON(artifacts.ResultAbs, res); err != nil {
					return fmt.Errorf("writing execute-bead result artifact: %w", err)
				}
				return writeExecuteBeadResult(cmd, res, asJSON)
			} else {
				if rebasedRev, revErr := gitOps.HeadRev(wtPath); revErr == nil {
					mergeRev = rebasedRev
					res.ResultRev = rebasedRev
					rebased = true
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to read rebased HEAD: %v\n", revErr)
				}
			}
		}

		if mergeErr := gitOps.FFMerge(workDir, mergeRev); mergeErr == nil {
			res.Outcome = "merged"
		} else {
			ref := executeBeadPreserveRef(beadID, baseRev)
			if updateErr := gitOps.UpdateRef(workDir, ref, mergeRev); updateErr != nil {
				return fmt.Errorf("preserving result ref: %w", updateErr)
			}
			res.Outcome = "preserved"
			res.PreserveRef = ref
			if rebased {
				res.Reason = "ff-merge failed after rebase"
			} else {
				res.Reason = "ff-merge not possible"
			}
		}
	}
	populateExecuteBeadStatus(&res)
	if err := writeExecuteBeadArtifactJSON(artifacts.ResultAbs, res); err != nil {
		return fmt.Errorf("writing execute-bead result artifact: %w", err)
	}

	return writeExecuteBeadResult(cmd, res, asJSON)
}

func (f *CommandFactory) prepareExecuteBeadArtifacts(wtPath, beadID, attemptID, baseRev, promptOverride, harness, model, effort string) (*executeBeadArtifacts, error) {
	b, refs, err := executeBeadContext(wtPath, beadID)
	if err != nil {
		return nil, err
	}
	artifacts, err := executeBeadCreateArtifactBundle(f.WorkingDir, wtPath, attemptID)
	if err != nil {
		return nil, err
	}

	promptContent, promptSource, err := executeBeadPromptContent(f.WorkingDir, b, refs, artifacts, baseRev, promptOverride)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(artifacts.PromptAbs, promptContent, 0o644); err != nil {
		return nil, fmt.Errorf("writing execute-bead prompt artifact: %w", err)
	}

	manifest := executeBeadManifest{
		AttemptID: attemptID,
		BeadID:    beadID,
		BaseRev:   baseRev,
		CreatedAt: time.Now().UTC(),
		Requested: executeBeadRequested{
			Harness: harness,
			Model:   model,
			Effort:  effort,
			Prompt:  promptSource,
		},
		Bead: executeBeadManifestBead{
			ID:          b.ID,
			Title:       b.Title,
			Description: b.Description,
			Acceptance:  b.Acceptance,
			Parent:      b.Parent,
			Labels:      append([]string{}, b.Labels...),
			Metadata:    executeBeadMetadata(b),
		},
		Governing: refs,
		Paths: executeBeadArtifactPaths{
			Dir:      artifacts.DirRel,
			Prompt:   artifacts.PromptRel,
			Manifest: artifacts.ManifestRel,
			Result:   artifacts.ResultRel,
			Worktree: filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(wtPath, f.WorkingDir), string(filepath.Separator))),
		},
	}
	if err := writeExecuteBeadArtifactJSON(artifacts.ManifestAbs, manifest); err != nil {
		return nil, fmt.Errorf("writing execute-bead manifest artifact: %w", err)
	}
	return artifacts, nil
}

func executeBeadContext(wtPath, beadID string) (*bead.Bead, []executeBeadGoverningRef, error) {
	store := bead.NewStore(filepath.Join(wtPath, ".ddx"))
	b, err := store.Get(beadID)
	if err != nil {
		return nil, nil, fmt.Errorf("loading bead %s from execution snapshot: %w", beadID, err)
	}
	return b, executeBeadResolveGoverningRefs(wtPath, b), nil
}

func executeBeadResolveGoverningRefs(root string, b *bead.Bead) []executeBeadGoverningRef {
	specID, _ := b.Extra["spec-id"].(string)
	if strings.TrimSpace(specID) == "" {
		return nil
	}
	graph, err := docgraph.BuildGraphWithConfig(root)
	if err != nil || graph == nil {
		return nil
	}
	doc, ok := graph.Documents[specID]
	if !ok || doc == nil {
		return nil
	}
	return []executeBeadGoverningRef{{
		ID:    doc.ID,
		Path:  filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(doc.Path, root), string(filepath.Separator))),
		Title: doc.Title,
	}}
}

func executeBeadCreateArtifactBundle(rootDir, wtPath, attemptID string) (*executeBeadArtifacts, error) {
	dirRel := filepath.ToSlash(filepath.Join(executeBeadArtifactDir, attemptID))
	dirAbs := filepath.Join(rootDir, executeBeadArtifactDir, attemptID)
	if err := os.MkdirAll(dirAbs, 0o755); err != nil {
		return nil, fmt.Errorf("creating execute-bead artifact bundle: %w", err)
	}
	return &executeBeadArtifacts{
		DirAbs:      dirAbs,
		DirRel:      dirRel,
		PromptAbs:   filepath.Join(dirAbs, "prompt.md"),
		PromptRel:   filepath.ToSlash(filepath.Join(dirRel, "prompt.md")),
		ManifestAbs: filepath.Join(dirAbs, "manifest.json"),
		ManifestRel: filepath.ToSlash(filepath.Join(dirRel, "manifest.json")),
		ResultAbs:   filepath.Join(dirAbs, "result.json"),
		ResultRel:   filepath.ToSlash(filepath.Join(dirRel, "result.json")),
	}, nil
}

func executeBeadPromptContent(workDir string, b *bead.Bead, refs []executeBeadGoverningRef, artifacts *executeBeadArtifacts, baseRev, promptOverride string) ([]byte, string, error) {
	if strings.TrimSpace(promptOverride) != "" {
		path := promptOverride
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("reading prompt override %q: %w", promptOverride, err)
		}
		return data, promptOverride, nil
	}

	var sb strings.Builder
	sb.WriteString("# Execute Bead\n\n")
	sb.WriteString("You are running inside DDx's isolated execution worktree for this bead.\n")
	sb.WriteString("Treat the bead contract below as authoritative, then read the listed governing references from this worktree when they are relevant.\n\n")

	sb.WriteString("## Bead\n")
	fmt.Fprintf(&sb, "- ID: `%s`\n", b.ID)
	fmt.Fprintf(&sb, "- Title: %s\n", strings.TrimSpace(b.Title))
	if b.Parent != "" {
		fmt.Fprintf(&sb, "- Parent: `%s`\n", b.Parent)
	}
	if len(b.Labels) > 0 {
		fmt.Fprintf(&sb, "- Labels: %s\n", strings.Join(b.Labels, ", "))
	}
	if specID, _ := b.Extra["spec-id"].(string); specID != "" {
		fmt.Fprintf(&sb, "- spec-id: `%s`\n", specID)
	}
	fmt.Fprintf(&sb, "- Base revision: `%s`\n", baseRev)
	fmt.Fprintf(&sb, "- Execution bundle: `%s`\n\n", artifacts.DirRel)

	sb.WriteString("## Description\n")
	if strings.TrimSpace(b.Description) == "" {
		sb.WriteString("No description was recorded on the bead.\n")
	} else {
		sb.WriteString(strings.TrimSpace(b.Description))
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Acceptance Criteria\n")
	if strings.TrimSpace(b.Acceptance) == "" {
		sb.WriteString("No explicit acceptance criteria were recorded on the bead.\n")
	} else {
		sb.WriteString(strings.TrimSpace(b.Acceptance))
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Governing References\n")
	if len(refs) == 0 {
		sb.WriteString("No governing references were resolved from the current execution snapshot.\n")
	} else {
		for _, ref := range refs {
			if ref.Title != "" {
				fmt.Fprintf(&sb, "- `%s` — `%s` (%s)\n", ref.ID, ref.Path, ref.Title)
			} else {
				fmt.Fprintf(&sb, "- `%s` — `%s`\n", ref.ID, ref.Path)
			}
		}
	}

	sb.WriteString("\n## Execution Rules\n")
	sb.WriteString("1. Work only inside this execution worktree.\n")
	sb.WriteString("2. Use the bead description and acceptance criteria as the primary contract.\n")
	sb.WriteString("3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.\n")
	sb.WriteString("4. If the bead is missing critical context or the governing references conflict, stop and report the gap explicitly instead of improvising hidden policy.\n")
	sb.WriteString("5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.\n")
	sb.WriteString("6. Complete the requested implementation and any local checks the bead contract requires. DDx will classify the final outcome.\n")

	return []byte(sb.String()), "synthesized", nil
}

func executeBeadMetadata(b *bead.Bead) map[string]any {
	if len(b.Extra) == 0 {
		return nil
	}
	meta := make(map[string]any, len(b.Extra))
	for k, v := range b.Extra {
		meta[k] = v
	}
	return meta
}

func writeExecuteBeadArtifactJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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

func populateExecuteBeadStatus(res *ExecuteBeadResult) {
	res.Status = agent.ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	res.Detail = agent.ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
}

func writeExecuteBeadResult(cmd *cobra.Command, res ExecuteBeadResult, asJSON bool) error {
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
