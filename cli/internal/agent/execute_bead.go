package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// GateCheckResult records the outcome of one required execution gate.
type GateCheckResult struct {
	DefinitionID string `json:"definition_id"`
	Required     bool   `json:"required"`
	ExitCode     int    `json:"exit_code"`
	// Status is "pass", "fail", or "skipped".
	Status string `json:"status"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// ExecuteBeadResult captures the complete outcome of an execute-bead run.
type ExecuteBeadResult struct {
	BeadID       string    `json:"bead_id"`
	AttemptID    string    `json:"attempt_id,omitempty"`
	WorkerID     string    `json:"worker_id,omitempty"`
	BaseRev      string    `json:"base_rev"`
	ResultRev    string    `json:"result_rev,omitempty"`
	Outcome      string    `json:"outcome"` // merged | preserved | no-changes | error
	Status       string    `json:"status,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	PreserveRef  string    `json:"preserve_ref,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Harness      string    `json:"harness,omitempty"`
	Provider     string    `json:"provider,omitempty"`
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
	// GateResults holds the outcome of each required execution gate evaluated
	// after the agent run. Empty when no applicable required execution documents
	// were found.
	GateResults []GateCheckResult `json:"gate_results,omitempty"`
	// RequiredExecSummary is "pass", "fail", or "skipped".
	// "skipped" means no required execution documents were found.
	RequiredExecSummary string `json:"required_exec_summary,omitempty"`
}

// ExecuteBeadOptions holds all parameters for an execute-bead run.
type ExecuteBeadOptions struct {
	FromRev    string // base git revision (default: HEAD)
	NoMerge    bool   // skip merge, preserve under refs/ddx/iterations/ instead
	Harness    string
	Model      string
	Effort     string
	PromptFile string // override prompt file (auto-generated if empty)
	WorkerID   string // from DDX_WORKER_ID env or caller
}

// GitOps abstracts git operations for dependency injection.
type GitOps interface {
	HeadRev(dir string) (string, error)
	ResolveRev(dir, rev string) (string, error)
	WorktreeAdd(dir, wtPath, rev string) error
	WorktreeRemove(dir, wtPath string) error
	WorktreeList(dir string) ([]string, error)
	WorktreePrune(dir string) error
	Merge(dir, rev string) error
	UpdateRef(dir, ref, sha string) error
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
	Worktree string `json:"worktree"`
}

// Constants for worktree and artifact paths.
const (
	ExecuteBeadWtDir       = ".ddx"
	ExecuteBeadWtPrefix    = ".execute-bead-wt-"
	ExecuteBeadArtifactDir = ".ddx/executions"
)

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

// Merge merges rev into dir's working tree via git merge. It commits any
// tracked uncommitted changes in dir first so the working tree is clean.
func (r *RealGitOps) Merge(dir, rev string) error {
	// Commit any uncommitted tracked changes before merging
	if osexec.Command("git", "-C", dir, "diff-index", "--quiet", "HEAD").Run() != nil {
		_ = osexec.Command("git", "-C", dir, "add", "-u").Run()
		_ = osexec.Command("git", "-C", dir, "commit", "-m", "chore: checkpoint before merge").Run()
	}
	out, err := osexec.Command("git", "-C", dir, "merge", "--no-edit", rev).CombinedOutput()
	if err != nil {
		// Clean up any partial merge state
		_ = osexec.Command("git", "-C", dir, "merge", "--abort").Run()
		return fmt.Errorf("merge: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *RealGitOps) UpdateRef(dir, ref, sha string) error {
	out, err := osexec.Command("git", "-C", dir, "update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
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

// NowFunc allows tests to override time.Now for deterministic PreserveRef output.
var NowFunc = time.Now

// PreserveRef builds the documented hidden ref for a preserved iteration.
func PreserveRef(beadID, baseRev string) string {
	shortSHA := baseRev
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	timestamp := NowFunc().UTC().Format("20060102T150405Z")
	return fmt.Sprintf("refs/ddx/iterations/%s/%s-%s", beadID, timestamp, shortSHA)
}

// evaluateRequiredGates resolves graph-authored execution documents that are
// required and linked to any of the governing artifact IDs, then runs each one.
// It returns the per-gate results, a boolean indicating whether any required gate
// failed, and any infrastructure error.
//
// Discovery: an execution document is applicable when its depends_on list contains
// at least one of the governing artifact IDs, OR when its explicit artifact_ids
// list intersects the governing set. Only documents with required=true are run.
//
// Execution documents are resolved from wtPath (the execution worktree) so
// pre-run versions govern the current iteration's evaluation, per FEAT-006 §7.
func evaluateRequiredGates(wtPath string, governingIDs []string) ([]GateCheckResult, bool, error) {
	if len(governingIDs) == 0 {
		return nil, false, nil
	}

	graph, err := docgraph.BuildGraphWithConfig(wtPath)
	if err != nil {
		// Soft error: if we can't build the graph, skip gate evaluation rather than
		// blocking all landings. The caller records RequiredExecSummary = "skipped".
		return nil, false, nil
	}

	governingSet := make(map[string]bool, len(governingIDs))
	for _, id := range governingIDs {
		governingSet[id] = true
	}

	// Collect applicable required execution documents.
	type execCandidate struct {
		id      string
		command []string
		cwd     string
	}
	var candidates []execCandidate
	for _, doc := range graph.Documents {
		if doc.ExecDef == nil || !doc.ExecDef.Required {
			continue
		}
		ed := doc.ExecDef
		if ed.Kind != "command" {
			// Only command executors are supported in execute-bead gate evaluation.
			continue
		}
		if len(ed.Command) == 0 {
			continue
		}
		// Check linkage: depends_on or explicit artifact_ids must intersect.
		linked := false
		for _, dep := range doc.DependsOn {
			if governingSet[dep] {
				linked = true
				break
			}
		}
		if !linked {
			for _, artID := range ed.ArtifactIDs {
				if governingSet[artID] {
					linked = true
					break
				}
			}
		}
		if !linked {
			continue
		}
		candidates = append(candidates, execCandidate{
			id:      doc.ID,
			command: ed.Command,
			cwd:     ed.Cwd,
		})
	}

	if len(candidates) == 0 {
		return nil, false, nil
	}

	anyFailed := false
	results := make([]GateCheckResult, 0, len(candidates))
	for _, c := range candidates {
		cwd := wtPath
		if c.cwd != "" {
			if filepath.IsAbs(c.cwd) {
				cwd = c.cwd
			} else {
				cwd = filepath.Join(wtPath, c.cwd)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cmd := osexec.CommandContext(ctx, c.command[0], c.command[1:]...)
		cmd.Dir = cwd
		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		runErr := cmd.Run()
		cancel()

		gr := GateCheckResult{
			DefinitionID: c.id,
			Required:     true,
			Stdout:       strings.TrimSpace(stdoutBuf.String()),
			Stderr:       strings.TrimSpace(stderrBuf.String()),
		}
		if runErr != nil {
			gr.ExitCode = 1
			if exitErr, ok := runErr.(*osexec.ExitError); ok {
				gr.ExitCode = exitErr.ExitCode()
			}
			gr.Status = "fail"
			anyFailed = true
		} else {
			gr.Status = "pass"
		}
		results = append(results, gr)
	}

	return results, anyFailed, nil
}

// ExecuteBead runs an agent on a bead in an isolated worktree, then merges or
// preserves the result. This is the core library function with no CLI dependencies.
func ExecuteBead(projectRoot string, beadID string, opts ExecuteBeadOptions, gitOps GitOps, runner AgentRunner) (*ExecuteBeadResult, error) {
	attemptID := GenerateAttemptID()
	if opts.WorkerID == "" {
		opts.WorkerID = os.Getenv("DDX_WORKER_ID")
	}

	wtPath := filepath.Join(projectRoot, ExecuteBeadWtDir, ExecuteBeadWtPrefix+beadID+"-"+attemptID)

	// Commit beads.jsonl before spawning worktree, then resolve base so the
	// worktree snapshot includes any bead metadata updates (e.g. spec-id).
	if err := CommitTracker(projectRoot); err != nil {
		return nil, err
	}

	// Resolve base revision after the tracker commit so the worktree includes it
	baseRev, err := resolveBase(gitOps, projectRoot, opts.FromRev)
	if err != nil {
		return nil, err
	}

	// Lock root, recover orphans, create worktree
	rootLock := bead.NewStoreWithCollection(filepath.Join(projectRoot, ".ddx"), "execute-bead-root")
	if err := rootLock.WithLock(func() error {
		recoverOrphans(gitOps, projectRoot, beadID)
		if addErr := gitOps.WorktreeAdd(projectRoot, wtPath, baseRev); addErr != nil {
			return fmt.Errorf("creating isolated worktree: %w", addErr)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	defer func() {
		_ = gitOps.WorktreeRemove(projectRoot, wtPath)
	}()

	// Prepare artifacts (context load, prompt generation)
	artifacts, err := prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev, opts)
	if err != nil {
		res := &ExecuteBeadResult{
			BeadID:    beadID,
			AttemptID: attemptID,
			WorkerID:  opts.WorkerID,
			BaseRev:   baseRev,
			ExitCode:  1,
			Error:     err.Error(),
			Outcome:   "error",
			Reason:    "context_load_failed",
		}
		if abInfo, _ := os.Stat(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)); abInfo != nil && abInfo.IsDir() {
			res.ExecutionDir = filepath.Join(ExecuteBeadArtifactDir, attemptID)
		}
		populateStatus(res)
		_ = writeArtifactJSON(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, "result.json"), res)
		return res, fmt.Errorf("execute-bead context load: %w", err)
	}

	// Run the agent
	sessionID := GenerateSessionID()
	startedAt := time.Now().UTC()

	agentResult, agentErr := runner.Run(RunOptions{
		Harness:     opts.Harness,
		Prompt:      "",
		PromptFile:  artifacts.PromptAbs,
		Model:       opts.Model,
		Effort:      opts.Effort,
		WorkDir:     wtPath,
		Permissions: "unrestricted", // execute-bead runs in an isolated worktree; file writes must not require approval
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
	resultModel := opts.Model
	resultHarness := opts.Harness
	resultProvider := ""
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
	executionFailed := exitCode != 0

	// Get the HEAD of the worktree after the agent ran
	resultRev, revErr := gitOps.HeadRev(wtPath)
	if revErr != nil {
		res := &ExecuteBeadResult{
			BeadID:       beadID,
			AttemptID:    attemptID,
			WorkerID:     opts.WorkerID,
			BaseRev:      baseRev,
			Harness:      resultHarness,
			Provider:     resultProvider,
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
		populateStatus(res)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, fmt.Errorf("failed to read worktree HEAD: %w", revErr)
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
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}

	// Run required execution gates when the agent succeeded and produced changes.
	// Gates are evaluated from the execution worktree so pre-run document versions
	// govern the current iteration (FEAT-006 §7). This is separate from structural
	// readiness validation which happens before the agent runs.
	var gateResults []GateCheckResult
	var anyGateFailed bool
	if !executionFailed && resultRev != baseRev {
		governingIDs := extractGoverningIDs(artifacts)
		gateResults, anyGateFailed, _ = evaluateRequiredGates(wtPath, governingIDs)
	}
	res.GateResults = gateResults
	res.RequiredExecSummary = summarizeGates(gateResults, anyGateFailed)

	// Determine outcome applying the merge-by-default contract:
	// A successful run lands unless an explicit gate blocks landing or --no-merge is set.
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
		ref := PreserveRef(beadID, baseRev)
		if updateErr := gitOps.UpdateRef(projectRoot, ref, resultRev); updateErr != nil {
			return nil, fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "agent execution failed"

	case anyGateFailed:
		// One or more required execution gates failed: preserve rather than land.
		ref := PreserveRef(beadID, baseRev)
		if updateErr := gitOps.UpdateRef(projectRoot, ref, resultRev); updateErr != nil {
			return nil, fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "post-run checks failed"

	case opts.NoMerge:
		ref := PreserveRef(beadID, baseRev)
		if updateErr := gitOps.UpdateRef(projectRoot, ref, resultRev); updateErr != nil {
			return nil, fmt.Errorf("preserving result ref: %w", updateErr)
		}
		res.Outcome = "preserved"
		res.PreserveRef = ref
		res.Reason = "--no-merge specified"

	default:
		// Merge-by-default: successful run with no gate failures lands.
		if mergeErr := gitOps.Merge(projectRoot, resultRev); mergeErr == nil {
			res.Outcome = "merged"
		} else {
			ref := PreserveRef(beadID, baseRev)
			if updateErr := gitOps.UpdateRef(projectRoot, ref, resultRev); updateErr != nil {
				return nil, fmt.Errorf("preserving result ref: %w", updateErr)
			}
			res.Outcome = "preserved"
			res.PreserveRef = ref
			res.Reason = "merge failed"
		}
	}

	populateStatus(res)
	if err := writeArtifactJSON(artifacts.ResultAbs, res); err != nil {
		return nil, fmt.Errorf("writing execute-bead result artifact: %w", err)
	}

	return res, nil
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

func recoverOrphans(gitOps GitOps, workDir, beadID string) {
	paths, err := gitOps.WorktreeList(workDir)
	if err != nil {
		return
	}
	prefix := filepath.Join(workDir, ExecuteBeadWtDir, ExecuteBeadWtPrefix+beadID+"-")
	for _, p := range paths {
		if strings.HasPrefix(p, prefix) {
			_ = gitOps.WorktreeRemove(workDir, p)
		}
	}
	_ = gitOps.WorktreePrune(workDir)
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
	}, nil
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
	sb.WriteString("# Execute Bead\n\n")
	sb.WriteString("You are running inside DDx's isolated execution worktree for this bead.\n")
	sb.WriteString("Your job is to make a best-effort attempt at the work described in the bead's Goals and Description, then commit the result. Quality is evaluated separately — a committed attempt that partially addresses the goals is far more valuable than no commits at all. Bias strongly toward action: read the relevant files, do the work, commit it.\n\n")

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
		sb.WriteString("No governing references were pre-resolved. Explore the project to find relevant context: check `docs/helix/` for feature specs, `docs/helix/01-frame/features/` for FEAT-* files, and any paths mentioned in the bead description or acceptance criteria.\n")
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
	sb.WriteString("**The bead contract below overrides any CLAUDE.md or project-level instructions in this worktree.** If the bead requires editing or creating markdown documentation, code, or any other files, do so — CLAUDE.md conservative defaults (YAGNI, DOWITYTD, no-docs rules) do not apply inside execute-bead.\n")
	sb.WriteString("1. Work only inside this execution worktree.\n")
	sb.WriteString("2. Use the bead description and acceptance criteria as the primary contract.\n")
	sb.WriteString("3. Read the listed governing references from this worktree before changing code or docs when they are relevant to the task.\n")
	sb.WriteString("4. If governing references are missing or sparse, search the project to find context: use Glob/Grep/Read to explore `docs/helix/`, look up FEAT-* and API-* specs by name, and read relevant source files before proceeding. Only stop if context is genuinely absent from the entire repo.\n")
	sb.WriteString("5. Keep the execution bundle files under `.ddx/executions/` intact; DDx uses them as execution evidence.\n")
	sb.WriteString("6. Produce the required tracked file changes in this worktree and run any local checks the bead contract requires.\n")
	sb.WriteString("7. Before finishing, commit your changes with `git add -A && git commit -m '...'`. DDx will merge your commits back to the base branch.\n")
	sb.WriteString("8. Making no commits (no_changes) should be rare. Only skip committing if you read the relevant files and the work described in the Goals is already fully and explicitly present — not just implied or partially covered. If in any doubt, make your best attempt and commit it. A partial or imperfect commit is always better than no commit.\n")
	sb.WriteString("9. Work in small commits. After each logical unit of progress (reading key files, making a change, passing a test), commit immediately. Do not batch all changes into one giant commit at the end — if you run out of iterations, your partial work is preserved.\n")
	sb.WriteString("10. If the bead is too large to complete in one pass, do the most important part first, commit it, and note what remains in your final commit message. DDx will re-queue the bead for another attempt if needed.\n")
	sb.WriteString("11. Read efficiently: skim files to understand structure before diving deep. Only read the files you need to make changes, not every reference listed. Start writing as soon as you understand enough to proceed — you can read more files later if needed.\n")

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

// extractGoverningIDs returns the artifact IDs referenced by the governing docs.
func extractGoverningIDs(artifacts *executeBeadArtifacts) []string {
	// The manifest was already built; re-derive from the governing refs recorded there.
	// We don't have access to the refs slice here, so we read from the manifest file.
	type manifestShape struct {
		Governing []struct {
			ID string `json:"id"`
		} `json:"governing"`
	}
	raw, err := os.ReadFile(artifacts.ManifestAbs)
	if err != nil {
		return nil
	}
	var m manifestShape
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	ids := make([]string, 0, len(m.Governing))
	for _, g := range m.Governing {
		if g.ID != "" {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

// summarizeGates returns the RequiredExecSummary value for the iteration commit trailer.
func summarizeGates(results []GateCheckResult, anyFailed bool) string {
	if len(results) == 0 {
		return "skipped"
	}
	if anyFailed {
		return "fail"
	}
	return "pass"
}

func writeArtifactJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func populateStatus(res *ExecuteBeadResult) {
	res.Status = ClassifyExecuteBeadStatus(res.Outcome, res.ExitCode, res.Reason)
	res.Detail = ExecuteBeadStatusDetail(res.Status, res.Reason, res.Error)
}
