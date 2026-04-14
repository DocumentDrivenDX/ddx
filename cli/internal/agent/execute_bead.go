package agent

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// ExecuteBeadResult captures the outcome of an execute-bead worker run.
// The worker populates the task-level fields (BeadID through UsageFile).
// The parent orchestrator (LandBeadResult) populates the landing fields
// (Outcome, Status, Detail, Reason, PreserveRef, GateResults, RequiredExecSummary,
// ChecksFile) via ApplyLandingToResult before the result is written to disk or
// returned to a caller.
type ExecuteBeadResult struct {
	BeadID    string `json:"bead_id"`
	AttemptID string `json:"attempt_id,omitempty"`
	WorkerID  string `json:"worker_id,omitempty"`
	BaseRev   string `json:"base_rev"`
	ResultRev string `json:"result_rev,omitempty"`

	// Outcome and Status are initially set by the worker to task-level values
	// (task_succeeded / task_failed / task_no_changes), then overwritten by
	// ApplyLandingToResult with the landing decision (merged / preserved /
	// no-changes) so callers see a unified record.
	Outcome string `json:"outcome"`
	Status  string `json:"status,omitempty"`
	Detail  string `json:"detail,omitempty"`

	// Landing fields — populated by ApplyLandingToResult, not by ExecuteBead.
	Reason              string            `json:"reason,omitempty"`
	PreserveRef         string            `json:"preserve_ref,omitempty"`
	GateResults         []GateCheckResult `json:"gate_results,omitempty"`
	RequiredExecSummary string            `json:"required_exec_summary,omitempty"`
	ChecksFile          string            `json:"checks_file,omitempty"`

	Harness    string  `json:"harness,omitempty"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	SessionID  string  `json:"session_id,omitempty"`
	DurationMS int     `json:"duration_ms"`
	Tokens     int     `json:"tokens,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	ExitCode   int     `json:"exit_code"`
	Error      string  `json:"error,omitempty"`

	ExecutionDir string `json:"execution_dir,omitempty"`
	PromptFile   string `json:"prompt_file,omitempty"`
	ManifestFile string `json:"manifest_file,omitempty"`
	ResultFile   string `json:"result_file,omitempty"`
	UsageFile    string `json:"usage_file,omitempty"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// ExecuteBeadOptions holds all parameters for an execute-bead worker run.
type ExecuteBeadOptions struct {
	FromRev    string // base git revision (default: HEAD)
	Harness    string
	Model      string
	Effort     string
	PromptFile string // override prompt file (auto-generated if empty)
	WorkerID   string // from DDX_WORKER_ID env or caller
}

// GitOps abstracts the git operations required by the worker.
// Merge and UpdateRef are intentionally excluded — those belong to the
// parent-side orchestrator (OrchestratorGitOps).
type GitOps interface {
	HeadRev(dir string) (string, error)
	ResolveRev(dir, rev string) (string, error)
	WorktreeAdd(dir, wtPath, rev string) error
	WorktreeRemove(dir, wtPath string) error
	WorktreeList(dir string) ([]string, error)
	WorktreePrune(dir string) error
	IsDirty(dir string) (bool, error)
	// SynthesizeCommit stages real file changes (excluding harness noise paths) and
	// commits them using msg as the commit message. Returns (true, nil) when a
	// commit was made, (false, nil) when there was nothing real to commit (all
	// dirty files were noise), and (false, err) on failure.
	SynthesizeCommit(dir, msg string) (bool, error)
}

// AgentRunner runs an agent with the given options.
type AgentRunner interface {
	Run(opts RunOptions) (*Result, error)
}

// Artifact paths for an execute-bead attempt.
type executeBeadArtifacts struct {
	DirAbs      string
	DirRel      string
	PromptAbs   string
	PromptRel   string
	ManifestAbs string
	ManifestRel string
	ResultAbs   string
	ResultRel   string
	ChecksAbs   string
	ChecksRel   string
	UsageAbs    string
	UsageRel    string
}

type executeBeadManifest struct {
	AttemptID string                    `json:"attempt_id"`
	WorkerID  string                    `json:"worker_id,omitempty"`
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
	Checks   string `json:"checks,omitempty"`
	Usage    string `json:"usage,omitempty"`
	Worktree string `json:"worktree"`
}

// Constants for worktree and artifact paths.
const (
	ExecuteBeadWtDir       = ".ddx" // legacy; kept for compatibility
	ExecuteBeadWtPrefix    = ".execute-bead-wt-"
	ExecuteBeadArtifactDir = ".ddx/executions"

	// ExecuteBeadTmpSubdir is the subdirectory under $TMPDIR in which
	// execute-bead creates its isolated worktrees. Keeping them outside
	// the project tree prevents child processes (tests, hooks) running
	// inside the worktree from mutating the parent repository's
	// .git/config via inherited GIT_DIR.
	ExecuteBeadTmpSubdir = "ddx-exec-wt"
)

// executeBeadWorktreePath returns the absolute path where an execute-bead
// isolated worktree for (beadID, attemptID) should live.
func executeBeadWorktreePath(beadID, attemptID string) string {
	base := os.Getenv("DDX_EXEC_WT_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), ExecuteBeadTmpSubdir)
	}
	return filepath.Join(base, ExecuteBeadWtPrefix+beadID+"-"+attemptID)
}

// RealGitOps implements GitOps via os/exec git commands.
type RealGitOps struct{}

func (r *RealGitOps) HeadRev(dir string) (string, error) {
	return r.ResolveRev(dir, "HEAD")
}

func (r *RealGitOps) ResolveRev(dir, rev string) (string, error) {
	out, err := osexec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *RealGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	out, err := osexec.Command("git", "-C", dir, "worktree", "add", "--detach", wtPath, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *RealGitOps) WorktreeRemove(dir, wtPath string) error {
	_ = osexec.Command("git", "-C", dir, "worktree", "remove", "--force", wtPath).Run()
	return nil
}

func (r *RealGitOps) WorktreeList(dir string) ([]string, error) {
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

func (r *RealGitOps) WorktreePrune(dir string) error {
	return osexec.Command("git", "-C", dir, "worktree", "prune").Run()
}

// IsDirty reports whether dir has any uncommitted changes (tracked modifications or untracked files).
func (r *RealGitOps) IsDirty(dir string) (bool, error) {
	out, _ := osexec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return len(bytes.TrimSpace(out)) > 0, nil
}

// SynthesizeCommit stages real file changes, explicitly excluding harness noise
// paths, and creates a commit with msg as the commit message. Returns (true, nil)
// when a commit was made, (false, nil) when nothing real remained to commit
// after exclusions, and (false, err) on failure.
func (r *RealGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	addArgs := []string{
		"-C", dir, "add", "-A", "--",
		".",
		":(exclude).ddx/agent-logs",
		":(exclude).ddx/workers",
		":(exclude).ddx/executions/*/embedded",
		":(exclude).claude/skills",
		":(exclude).agents/skills",
	}
	if err := osexec.Command("git", addArgs...).Run(); err != nil {
		return false, fmt.Errorf("staging changes: %w", err)
	}
	statusOut, _ := osexec.Command("git", "-C", dir, "diff", "--cached", "--name-only").Output()
	if len(bytes.TrimSpace(statusOut)) == 0 {
		return false, nil
	}
	if msg == "" {
		msg = "chore: execute-bead synthesized result commit"
	}
	out, err := osexec.Command("git", "-C", dir, "commit", "-m", msg).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("synthesize commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

// GenerateAttemptID generates a unique attempt identifier.
func GenerateAttemptID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

// GenerateSessionID generates a short session ID for the agent log.
func GenerateSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "eb-" + hex.EncodeToString(b)
}

// ExecuteBead is the thin worker: it creates an isolated worktree, constructs
// the agent prompt from bead context, runs the agent harness, synthesizes a
// commit if the agent left uncommitted changes, then cleans up the worktree
// and returns the result. It classifies outcomes as exactly one of:
//
//   - task_succeeded: agent exited 0 and produced one or more commits
//   - task_failed:    agent exited non-zero
//   - task_no_changes: agent exited 0 but made no commits
//
// Merge, UpdateRef, gate evaluation, preserve-ref management, and orphan
// recovery are the parent's responsibility (see LandBeadResult, RecoverOrphans).
func ExecuteBead(projectRoot string, beadID string, opts ExecuteBeadOptions, gitOps GitOps, runner AgentRunner) (*ExecuteBeadResult, error) {
	attemptID := GenerateAttemptID()
	if opts.WorkerID == "" {
		opts.WorkerID = os.Getenv("DDX_WORKER_ID")
	}

	wtPath := executeBeadWorktreePath(beadID, attemptID)
	if mkErr := os.MkdirAll(filepath.Dir(wtPath), 0o755); mkErr != nil {
		return nil, fmt.Errorf("creating execute-bead worktree parent dir: %w", mkErr)
	}

	// Commit beads.jsonl before spawning worktree so the worktree snapshot
	// includes any bead metadata updates (e.g. spec-id).
	if err := CommitTracker(projectRoot); err != nil {
		return nil, err
	}

	// Resolve base revision after the tracker commit.
	baseRev, err := resolveBase(gitOps, projectRoot, opts.FromRev)
	if err != nil {
		return nil, err
	}

	// Create the isolated worktree. Orphan recovery is the parent's responsibility
	// (call RecoverOrphans before invoking workers).
	if err := gitOps.WorktreeAdd(projectRoot, wtPath, baseRev); err != nil {
		return nil, fmt.Errorf("creating isolated worktree: %w", err)
	}
	defer func() {
		_ = gitOps.WorktreeRemove(projectRoot, wtPath)
	}()

	// Repair project-local skill symlinks whose targets do not resolve inside
	// the freshly created worktree.
	_ = materializeWorktreeSkills(wtPath)

	// Prepare artifacts (context load, prompt generation).
	artifacts, err := prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev, opts)
	if err != nil {
		res := &ExecuteBeadResult{
			BeadID:    beadID,
			AttemptID: attemptID,
			WorkerID:  opts.WorkerID,
			BaseRev:   baseRev,
			ResultRev: baseRev, // no commits; ResultRev == BaseRev signals no output
			ExitCode:  1,
			Error:     err.Error(),
			Outcome:   ExecuteBeadOutcomeTaskFailed,
		}
		if abInfo, _ := os.Stat(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)); abInfo != nil && abInfo.IsDir() {
			res.ExecutionDir = filepath.Join(ExecuteBeadArtifactDir, attemptID)
		}
		populateWorkerStatus(res)
		_ = writeArtifactJSON(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, "result.json"), res)
		return res, fmt.Errorf("execute-bead context load: %w", err)
	}

	// Redirect per-run session/telemetry output into the DDx-owned execution
	// bundle so the embedded harness does not accumulate state at the worktree root.
	embeddedStateDir := filepath.Join(artifacts.DirAbs, "embedded")
	if err := os.MkdirAll(embeddedStateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating embedded state dir: %w", err)
	}

	sessionID := GenerateSessionID()
	startedAt := time.Now().UTC()

	agentResult, agentErr := runner.Run(RunOptions{
		Harness:       opts.Harness,
		Prompt:        "",
		PromptFile:    artifacts.PromptAbs,
		Model:         opts.Model,
		Effort:        opts.Effort,
		WorkDir:       wtPath,
		Permissions:   "unrestricted", // isolated worktree; writes must not require approval
		SessionLogDir: embeddedStateDir,
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
	inputTokens := 0
	outputTokens := 0
	costUSD := 0.0
	resultModel := opts.Model
	resultHarness := opts.Harness
	resultProvider := ""
	agentErrMsg := ""
	if agentResult != nil {
		exitCode = agentResult.ExitCode
		tokens = agentResult.Tokens
		inputTokens = agentResult.InputTokens
		outputTokens = agentResult.OutputTokens
		costUSD = agentResult.CostUSD
		if agentResult.Error != "" {
			agentErrMsg = agentResult.Error
		}
		if agentResult.Model != "" {
			resultModel = agentResult.Model
		}
		if agentResult.Provider != "" {
			resultProvider = agentResult.Provider
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

	// Get the HEAD of the worktree after the agent ran.
	resultRev, revErr := gitOps.HeadRev(wtPath)
	if revErr != nil {
		res := &ExecuteBeadResult{
			BeadID:       beadID,
			AttemptID:    attemptID,
			WorkerID:     opts.WorkerID,
			BaseRev:      baseRev,
			ResultRev:    baseRev, // no commits readable; treat as no output
			Harness:      resultHarness,
			Provider:     resultProvider,
			Model:        resultModel,
			SessionID:    sessionID,
			DurationMS:   int(finishedAt.Sub(startedAt).Milliseconds()),
			Tokens:       tokens,
			CostUSD:      costUSD,
			ExitCode:     1,
			Error:        agentErrMsg,
			Reason:       revErr.Error(), // HeadRev failure; orchestrator prefers this over Error for Reason
			ExecutionDir: artifacts.DirRel,
			PromptFile:   artifacts.PromptRel,
			ManifestFile: artifacts.ManifestRel,
			ResultFile:   artifacts.ResultRel,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			Outcome:      ExecuteBeadOutcomeTaskFailed,
		}
		populateWorkerStatus(res)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, fmt.Errorf("failed to read worktree HEAD: %w", revErr)
	}

	// Write usage.json when the harness reports token usage or cost.
	// Done before SynthesizeCommit so usage data is available in the
	// preliminary result written for commit-message sourcing.
	var usageFileRel string
	if tokens > 0 || costUSD > 0 {
		usage := executeBeadUsage{
			AttemptID:    attemptID,
			Harness:      resultHarness,
			Provider:     resultProvider,
			Model:        resultModel,
			Tokens:       tokens,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			CostUSD:      costUSD,
		}
		if writeErr := writeArtifactJSON(artifacts.UsageAbs, usage); writeErr == nil {
			usageFileRel = artifacts.UsageRel
		}
	}

	// Synthesize a commit when the agent left tracked edits without committing.
	// Only do so for real changes — harness noise paths are excluded. If nothing
	// real was staged (committed is false), leave resultRev == baseRev so the
	// outcome is classified as task_no_changes.
	if resultRev == baseRev {
		if isDirty, _ := gitOps.IsDirty(wtPath); isDirty {
			// Build a preliminary result and write it to result.json before
			// calling SynthesizeCommit. The commit message is then sourced from
			// the tracked artifact file, satisfying the provenance contract:
			// "commit-message metadata must be projected from tracked artifact
			// files, never from ad hoc runtime state" (FEAT-006).
			// The final result.json is re-written after the commit with the
			// correct ResultRev, Outcome, and Status.
			prelimOutcome := ExecuteBeadOutcomeTaskSucceeded
			if exitCode != 0 {
				prelimOutcome = ExecuteBeadOutcomeTaskFailed
			}
			prelimRes := &ExecuteBeadResult{
				BeadID:       beadID,
				AttemptID:    attemptID,
				WorkerID:     opts.WorkerID,
				BaseRev:      baseRev,
				ResultRev:    "", // unknown until commit is made
				Harness:      resultHarness,
				Provider:     resultProvider,
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
				UsageFile:    usageFileRel,
				StartedAt:    startedAt,
				FinishedAt:   finishedAt,
				Outcome:      prelimOutcome,
			}
			populateWorkerStatus(prelimRes)
			_ = writeArtifactJSON(artifacts.ResultAbs, prelimRes)

			// Render the commit message from the tracked artifact file.
			commitMsg, msgErr := BuildCommitMessageFromResultFile(artifacts.ResultAbs)
			if msgErr != nil {
				commitMsg = "chore: execute-bead iteration " + beadID
			}

			if committed, synthErr := gitOps.SynthesizeCommit(wtPath, commitMsg); synthErr == nil && committed {
				if newRev, _ := gitOps.HeadRev(wtPath); newRev != baseRev {
					resultRev = newRev
				}
			}
		}
	}

	res := &ExecuteBeadResult{
		BeadID:       beadID,
		AttemptID:    attemptID,
		WorkerID:     opts.WorkerID,
		BaseRev:      baseRev,
		ResultRev:    resultRev,
		Harness:      resultHarness,
		Provider:     resultProvider,
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
		UsageFile:    usageFileRel,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}

	// Classify worker outcome: task_succeeded / task_failed / task_no_changes.
	// The parent orchestrator (LandBeadResult + ApplyLandingToResult) will
	// overwrite Outcome and Status with the landing decision before output.
	switch {
	case exitCode != 0:
		res.Outcome = ExecuteBeadOutcomeTaskFailed
	case resultRev == baseRev:
		res.Outcome = ExecuteBeadOutcomeTaskNoChanges
	default:
		res.Outcome = ExecuteBeadOutcomeTaskSucceeded
	}

	populateWorkerStatus(res)
	if err := writeArtifactJSON(artifacts.ResultAbs, res); err != nil {
		return nil, fmt.Errorf("writing execute-bead result artifact: %w", err)
	}

	return res, nil
}

// populateWorkerStatus fills in the Status and Detail fields on a worker result
// based on the task-level Outcome.
func populateWorkerStatus(res *ExecuteBeadResult) {
	switch res.Outcome {
	case ExecuteBeadOutcomeTaskSucceeded:
		res.Status = ExecuteBeadStatusSuccess
	case ExecuteBeadOutcomeTaskNoChanges:
		res.Status = ExecuteBeadStatusNoChanges
	default:
		res.Status = ExecuteBeadStatusExecutionFailed
	}
	res.Detail = ExecuteBeadStatusDetail(res.Status, "", res.Error)
}

// CommitTracker commits beads.jsonl if it has uncommitted changes.
func CommitTracker(projectRoot string) error {
	trackerFile := filepath.Join(projectRoot, ".ddx", "beads.jsonl")
	info, err := os.Stat(trackerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking tracker file: %w", err)
	}
	if info.IsDir() {
		return nil
	}

	out, err := osexec.Command("git", "-C", projectRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	out, err = osexec.Command("git", "-C", projectRoot, "diff", "--", ".ddx/beads.jsonl").Output()
	if err != nil {
		return fmt.Errorf("checking tracker diff: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		out, err = osexec.Command("git", "-C", projectRoot, "ls-files", "--others", "--exclude-standard", ".ddx/beads.jsonl").Output()
		if err != nil {
			return fmt.Errorf("checking tracker untracked: %w", err)
		}
		if strings.TrimSpace(string(out)) == "" {
			return nil
		}
	}

	msg := fmt.Sprintf("chore: update tracker (execute-bead %s)", time.Now().UTC().Format("20060102T150405"))
	commitOut, err := osexec.Command("git", "-C", projectRoot, "add", ".ddx/beads.jsonl").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staging tracker: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	commitOut, err = osexec.Command("git", "-C", projectRoot, "commit", "-m", msg).CombinedOutput()
	if err != nil {
		return fmt.Errorf("committing tracker: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	return nil
}

func resolveBase(gitOps GitOps, workDir, fromRev string) (string, error) {
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

func prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev string, opts ExecuteBeadOptions) (*executeBeadArtifacts, error) {
	b, refs, err := loadBeadContext(wtPath, beadID)
	if err != nil {
		return nil, err
	}
	artifacts, err := createArtifactBundle(projectRoot, wtPath, attemptID)
	if err != nil {
		return nil, err
	}

	promptContent, promptSource, err := buildPrompt(projectRoot, b, refs, artifacts, baseRev, opts.PromptFile)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(artifacts.PromptAbs, promptContent, 0o644); err != nil {
		return nil, fmt.Errorf("writing execute-bead prompt artifact: %w", err)
	}

	manifest := executeBeadManifest{
		AttemptID: attemptID,
		WorkerID:  opts.WorkerID,
		BeadID:    beadID,
		BaseRev:   baseRev,
		CreatedAt: time.Now().UTC(),
		Requested: executeBeadRequested{
			Harness: opts.Harness,
			Model:   opts.Model,
			Effort:  opts.Effort,
			Prompt:  promptSource,
		},
		Bead: executeBeadManifestBead{
			ID:          b.ID,
			Title:       b.Title,
			Description: b.Description,
			Acceptance:  b.Acceptance,
			Parent:      b.Parent,
			Labels:      append([]string{}, b.Labels...),
			Metadata:    beadMetadata(b),
		},
		Governing: refs,
		Paths: executeBeadArtifactPaths{
			Dir:      artifacts.DirRel,
			Prompt:   artifacts.PromptRel,
			Manifest: artifacts.ManifestRel,
			Result:   artifacts.ResultRel,
			Checks:   artifacts.ChecksRel,
			Usage:    artifacts.UsageRel,
			Worktree: filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(wtPath, projectRoot), string(filepath.Separator))),
		},
	}
	if err := writeArtifactJSON(artifacts.ManifestAbs, manifest); err != nil {
		return nil, fmt.Errorf("writing execute-bead manifest artifact: %w", err)
	}
	return artifacts, nil
}

func loadBeadContext(wtPath, beadID string) (*bead.Bead, []executeBeadGoverningRef, error) {
	store := bead.NewStore(filepath.Join(wtPath, ".ddx"))
	b, err := store.Get(beadID)
	if err != nil {
		return nil, nil, fmt.Errorf("loading bead %s from worktree snapshot: %w", beadID, err)
	}
	return b, ResolveGoverningRefs(wtPath, b), nil
}

func ResolveGoverningRefs(root string, b *bead.Bead) []executeBeadGoverningRef {
	specIDRaw, _ := b.Extra["spec-id"].(string)
	specIDRaw = strings.TrimSpace(specIDRaw)
	if specIDRaw == "" {
		return nil
	}

	// spec-id may be a comma-separated list of IDs or paths.
	ids := strings.Split(specIDRaw, ",")
	graph, _ := docgraph.BuildGraphWithConfig(root)

	var refs []executeBeadGoverningRef
	for _, specID := range ids {
		specID = strings.TrimSpace(specID)
		if specID == "" {
			continue
		}
		if graph != nil {
			if doc, ok := graph.Documents[specID]; ok && doc != nil {
				refs = append(refs, executeBeadGoverningRef{
					ID:    doc.ID,
					Path:  filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(doc.Path, root), string(filepath.Separator))),
					Title: doc.Title,
				})
				continue
			}
		}
		candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(specID)))
		relCandidate, relErr := filepath.Rel(root, candidate)
		if relErr != nil || strings.HasPrefix(relCandidate, ".."+string(filepath.Separator)) || relCandidate == ".." {
			continue
		}
		info, statErr := os.Stat(candidate)
		if statErr != nil || info.IsDir() {
			continue
		}
		refs = append(refs, executeBeadGoverningRef{
			ID:   specID,
			Path: filepath.ToSlash(relCandidate),
		})
	}
	return refs
}

func createArtifactBundle(rootDir, wtPath, attemptID string) (*executeBeadArtifacts, error) {
	dirRel := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))
	dirAbs := filepath.Join(rootDir, ExecuteBeadArtifactDir, attemptID)
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
		ChecksAbs:   filepath.Join(dirAbs, "checks.json"),
		ChecksRel:   filepath.ToSlash(filepath.Join(dirRel, "checks.json")),
		UsageAbs:    filepath.Join(dirAbs, "usage.json"),
		UsageRel:    filepath.ToSlash(filepath.Join(dirRel, "usage.json")),
	}, nil
}

// executeBeadInstructionsText is the per-bead task instructions emitted inside
// the <instructions> section of the synthesized execute-bead prompt.
const executeBeadInstructionsText = `You are running inside DDx's isolated execution worktree for this bead.
Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.

**The bead contract overrides any CLAUDE.md or project-level instructions in this worktree.** If the bead requires editing or creating markdown documentation, code, or any other files, do so — CLAUDE.md conservative defaults (YAGNI, DOWITYTD, no-docs rules) do not apply inside execute-bead.

1. Work only inside this execution worktree.
2. Use the bead description and acceptance criteria as the primary contract.
3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.
4. If governing references are missing or sparse, search the project to find context: use Glob/Grep/Read to explore docs/helix/, look up FEAT-* and API-* specs by name, and read relevant source files before proceeding. Only stop if context is genuinely absent from the entire repo.
5. Keep the execution bundle files under .ddx/executions/ intact; DDx uses them as execution evidence.
6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.
7. Before finishing, commit your changes with ` + "`git add -A && git commit -m '...'`" + `. DDx will merge your commits back to the base branch.
8. Report ` + "`no_changes`" + ` when the bead's work is already present. A well-justified no_changes is always preferred to a cosmetic synthesized commit. If you decide not to commit, write your reasoning to ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` with: (a) what you looked for, (b) where you found it — cite specific commits, files, or test names, and (c) why you're confident the bead is satisfied. Only commit if there is real work to do.
9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.
10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.
11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.
12. **Never run ` + "`ddx init`" + `** — the workspace is already initialized. Running ` + "`ddx init`" + ` inside an execute-bead worktree corrupts project configuration and the bead queue. Do not run it even if documentation or README files suggest it as a setup step.`

// executeBeadMissingGoverningText is emitted inside <governing> when no
// governing references were pre-resolved for the bead.
const executeBeadMissingGoverningText = `No governing references were pre-resolved. Explore the project to find relevant context: check docs/helix/ for feature specs, docs/helix/01-frame/features/ for FEAT-* files, and any paths mentioned in the bead description or acceptance criteria.`

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func xmlAttrEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
		"\n", "&#10;",
		"\r", "&#13;",
		"\t", "&#9;",
	)
	return r.Replace(s)
}

func buildPrompt(workDir string, b *bead.Bead, refs []executeBeadGoverningRef, artifacts *executeBeadArtifacts, baseRev, promptOverride string) ([]byte, string, error) {
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
	sb.WriteString("<execute-bead>\n")

	fmt.Fprintf(&sb, "  <bead id=\"%s\">\n", xmlAttrEscape(b.ID))
	fmt.Fprintf(&sb, "    <title>%s</title>\n", xmlEscape(strings.TrimSpace(b.Title)))

	desc := strings.TrimSpace(b.Description)
	if desc == "" {
		sb.WriteString("    <description/>\n")
	} else {
		fmt.Fprintf(&sb, "    <description>\n%s\n    </description>\n", xmlEscape(desc))
	}

	acc := strings.TrimSpace(b.Acceptance)
	if acc == "" {
		sb.WriteString("    <acceptance/>\n")
	} else {
		fmt.Fprintf(&sb, "    <acceptance>\n%s\n    </acceptance>\n", xmlEscape(acc))
	}

	if len(b.Labels) > 0 {
		fmt.Fprintf(&sb, "    <labels>%s</labels>\n", xmlEscape(strings.Join(b.Labels, ", ")))
	} else {
		sb.WriteString("    <labels/>\n")
	}

	metaAttrs := make([]string, 0, 6)
	if b.Parent != "" {
		metaAttrs = append(metaAttrs, fmt.Sprintf("parent=\"%s\"", xmlAttrEscape(b.Parent)))
	}
	if specID, _ := b.Extra["spec-id"].(string); strings.TrimSpace(specID) != "" {
		metaAttrs = append(metaAttrs, fmt.Sprintf("spec-id=\"%s\"", xmlAttrEscape(strings.TrimSpace(specID))))
	}
	metaAttrs = append(metaAttrs, fmt.Sprintf("base-rev=\"%s\"", xmlAttrEscape(baseRev)))
	metaAttrs = append(metaAttrs, fmt.Sprintf("bundle=\"%s\"", xmlAttrEscape(artifacts.DirRel)))
	fmt.Fprintf(&sb, "    <metadata %s/>\n", strings.Join(metaAttrs, " "))
	sb.WriteString("  </bead>\n")

	if len(refs) == 0 {
		fmt.Fprintf(&sb, "  <governing>\n    <note>%s</note>\n  </governing>\n", xmlEscape(executeBeadMissingGoverningText))
	} else {
		sb.WriteString("  <governing>\n")
		for _, ref := range refs {
			attrs := fmt.Sprintf("id=\"%s\" path=\"%s\"", xmlAttrEscape(ref.ID), xmlAttrEscape(ref.Path))
			if strings.TrimSpace(ref.Title) == "" {
				fmt.Fprintf(&sb, "    <ref %s/>\n", attrs)
			} else {
				fmt.Fprintf(&sb, "    <ref %s>%s</ref>\n", attrs, xmlEscape(strings.TrimSpace(ref.Title)))
			}
		}
		sb.WriteString("  </governing>\n")
	}

	instructions := strings.ReplaceAll(executeBeadInstructionsText, "{{.AttemptDir}}", artifacts.DirRel)
	fmt.Fprintf(&sb, "  <instructions>\n%s\n  </instructions>\n", xmlEscape(instructions))

	sb.WriteString("</execute-bead>\n")

	return []byte(sb.String()), "synthesized", nil
}

func beadMetadata(b *bead.Bead) map[string]any {
	if len(b.Extra) == 0 {
		return nil
	}
	meta := make(map[string]any, len(b.Extra))
	for k, v := range b.Extra {
		meta[k] = v
	}
	return meta
}

// executeBeadUsage is the machine-readable schema for usage.json.
// It is written when the harness reports token usage or cost.
type executeBeadUsage struct {
	AttemptID    string  `json:"attempt_id"`
	Harness      string  `json:"harness,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	Model        string  `json:"model,omitempty"`
	Tokens       int     `json:"tokens"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

func writeArtifactJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
