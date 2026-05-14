package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"errors"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
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
	// ImplementationRev is the worker's own commit SHA — the candidate revision
	// produced by the agent before landing. Set from ResultRev by
	// ApplyLandResultToExecuteBeadResult before ResultRev is rewritten to the
	// branch tip. Never overwritten after first assignment.
	ImplementationRev string `json:"implementation_rev,omitempty"`
	// LandedRev is the target branch tip after the coordinator fast-forwards or
	// merges the implementation commit. Populated by ApplyLandResultToExecuteBeadResult.
	LandedRev string `json:"landed_rev,omitempty"`
	// EvidenceRev is the SHA of the trailing evidence commit when it differs from
	// ImplementationRev. Populated by the evidence committer; empty otherwise.
	EvidenceRev string `json:"evidence_rev,omitempty"`

	// Outcome and Status are initially set by the worker to task-level values
	// (task_succeeded / task_failed / task_no_changes), then overwritten by
	// ApplyLandingToResult with the landing decision (merged / preserved /
	// no-changes) so callers see a unified record.
	Outcome string `json:"outcome"`
	Status  string `json:"status,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// OrchestratorStatus is populated after the landing/pre-merge orchestration
	// decision so result.json can distinguish worker success from final
	// non-landing outcomes.
	OrchestratorStatus string `json:"orchestrator_status,omitempty"`

	// Landing fields — populated by ApplyLandingToResult, not by ExecuteBead.
	Reason      string `json:"reason,omitempty"`
	PreserveRef string `json:"preserve_ref,omitempty"`
	// CandidateRef is the project-root git ref pinned before checks and review.
	// Format: refs/ddx/iterations/<attempt-id>/<cycle-index>.
	CandidateRef string `json:"candidate_ref,omitempty"`
	// CycleIndex is the zero-based repair-cycle index for this candidate.
	CycleIndex          int               `json:"cycle_index,omitempty"`
	GateResults         []GateCheckResult `json:"gate_results,omitempty"`
	RequiredExecSummary string            `json:"required_exec_summary,omitempty"`
	ChecksFile          string            `json:"checks_file,omitempty"`
	// Ratchet fields — populated by ApplyLandingToResult when declarative
	// ratchet thresholds were evaluated during landing. HELIX and other
	// consumers use these to distinguish ratchet-preserved attempts from
	// generic execution failures.
	RatchetEvidence []RatchetEvidence `json:"ratchet_evidence,omitempty"`
	RatchetSummary  string            `json:"ratchet_summary,omitempty"`

	// NoChangesRationale is populated when the agent wrote a rationale file to
	// the execution bundle dir inside the worktree. It carries the agent's
	// explanation of why no commits were made, and is preserved even when a
	// mixed commit + no_changes rationale is rejected.
	NoChangesRationale string `json:"no_changes_rationale,omitempty"`

	// NoEvidencePaths names worktree paths that remained dirty when the agent
	// exited without creating a commit or no_changes_rationale.txt. It helps
	// operators diagnose silent commit failures before the worktree is cleaned up.
	NoEvidencePaths []string `json:"no_evidence_paths,omitempty"`

	// CycleTrace records the append-only execution cycle trace in order.
	// Each entry captures one implementation or repair cycle.
	CycleTrace []ExecutionCycleTrace `json:"cycle_trace,omitempty"`

	// ResourceExhausted captures the root and cleanup summary when execution
	// stopped before a bead attempt because the host could not safely continue.
	ResourceExhausted any `json:"resource_exhausted,omitempty"`

	Harness                     string  `json:"harness,omitempty"`
	Provider                    string  `json:"provider,omitempty"`
	Model                       string  `json:"model,omitempty"`
	ActualPower                 int     `json:"actual_power,omitempty"`
	PredictedPower              int     `json:"predicted_power,omitempty"`
	PredictedSpeedTPS           float64 `json:"predicted_speed_tps,omitempty"`
	PredictedCostUSDPer1kTokens float64 `json:"predicted_cost_usd_per_1k_tokens,omitempty"`
	PredictedCostSource         string  `json:"predicted_cost_source,omitempty"`
	SessionID                   string  `json:"session_id,omitempty"`
	DurationMS                  int     `json:"duration_ms"`
	Tokens                      int     `json:"tokens,omitempty"`
	CostUSD                     float64 `json:"cost_usd,omitempty"`
	ExitCode                    int     `json:"exit_code"`
	Error                       string  `json:"error,omitempty"`

	// FailureMode classifies why an execution did not land cleanly. Empty
	// when the bead was merged (task_succeeded landing outcome). Populated
	// by the orchestrator from known patterns; see ClassifyFailureMode and
	// the FailureMode* constants in execute_bead_status.go.
	FailureMode string `json:"failure_mode,omitempty"`

	ExecutionDir       string             `json:"execution_dir,omitempty"`
	PromptFile         string             `json:"prompt_file,omitempty"`
	ManifestFile       string             `json:"manifest_file,omitempty"`
	ResultFile         string             `json:"result_file,omitempty"`
	UsageFile          string             `json:"usage_file,omitempty"`
	WorktreePath       string             `json:"worktree_path,omitempty"`
	AttemptDiagnostics *AttemptDiagnostic `json:"attempt_diagnostics,omitempty"`
	Stderr             string             `json:"stderr,omitempty"`
	RateLimitBudget    time.Duration      `json:"rate_limit_budget,omitempty"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// ExecutionCycleRouteFacts captures the implementer-side routing facts for
// one execution cycle.
type ExecutionCycleRouteFacts struct {
	Harness         string `json:"harness,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ActualPower     int    `json:"actual_power,omitempty"`
	RouteReason     string `json:"route_reason,omitempty"`
	ResolvedBaseURL string `json:"resolved_base_url,omitempty"`
}

// ExecutionCycleReviewResult captures the reduced review outcome for one
// execution cycle.
type ExecutionCycleReviewResult struct {
	Verdict        string     `json:"verdict,omitempty"`
	Rationale      string     `json:"rationale,omitempty"`
	Classification string     `json:"classification,omitempty"`
	PerAC          []ReviewAC `json:"per_ac,omitempty"`
	Findings       []Finding  `json:"findings,omitempty"`
}

// ExecutionCycleTrace records one implementation or repair cycle and its
// durable review/final-decision metadata.
type ExecutionCycleTrace struct {
	CycleIndex       int                        `json:"cycle_index"`
	AttemptID        string                     `json:"attempt_id,omitempty"`
	ResultRev        string                     `json:"result_rev,omitempty"`
	ImplementerRoute ExecutionCycleRouteFacts   `json:"implementer_route"`
	ReviewGroupID    string                     `json:"review_group_id,omitempty"`
	ReviewerIndices  []int                      `json:"reviewer_indices,omitempty"`
	ReviewVerdicts   []string                   `json:"review_verdicts,omitempty"`
	ReviewResult     ExecutionCycleReviewResult `json:"review_result,omitempty"`
	FinalDecision    string                     `json:"final_decision,omitempty"`
}

// AttemptDiagnostic captures infrastructure state when an attempt cannot be
// inspected normally. It is intentionally small and JSON-native so result.json
// remains useful even when the worker worktree has already vanished.
type AttemptDiagnostic struct {
	BeadID                 string    `json:"bead_id"`
	AttemptID              string    `json:"attempt_id"`
	WorktreePath           string    `json:"worktree_path"`
	HeadRevError           string    `json:"head_rev_error,omitempty"`
	WorktreePathExists     bool      `json:"worktree_path_exists"`
	CleanupMetadataPresent bool      `json:"cleanup_metadata_present"`
	RunStatePresent        bool      `json:"run_state_present"`
	RunState               *RunState `json:"run_state,omitempty"`
	GitWorktreeRegistered  bool      `json:"git_worktree_registered"`
}

// BeadEventAppender records append-only evidence events on a bead.
// Implemented by *bead.Store — kept as a minimal interface so the agent
// package does not need to import a concrete store type in tests.
type BeadEventAppender interface {
	AppendEvent(id string, event bead.BeadEvent) error
}

// BeadCancelStore reads/writes the cancel markers on a bead's Extra map.
// Implemented by *bead.Store. ADR-022 §Cancel SLA: the worker polls
// IsCancelRequested mid-attempt and on a positive read writes
// MarkCancelHonored before aborting at the next safe point.
type BeadCancelStore interface {
	IsCancelRequested(id string) (bool, error)
	MarkCancelHonored(id string) error
}

// CancelPollInterval is how often the worker checks for an operator cancel
// while an agent attempt is in flight. ADR-022 §Cancel SLA names 10s as the
// default. Exposed as a variable so tests can override it.
var CancelPollInterval = 10 * time.Second

// RunStateRefreshInterval controls how often execute-bead refreshes its
// per-attempt liveness record while the agent harness is running. Exposed as a
// variable so tests can shorten it without slowing production attempts.
var RunStateRefreshInterval = 10 * time.Second

// ExecuteBeadRuntime carries the non-durable plumbing for an execute-bead
// run: per-invocation intent (FromRev, PromptFile, WorkerID) and
// non-serializable injection seams (BeadEvents, Service, AgentRunner).
// Durable knobs (Harness, Model, Provider, Effort,
// ContextBudget, MirrorCfg) live on config.ResolvedConfig and are passed
// via ExecuteBeadWithConfig's rcfg argument.
//
// See SD-024 / TD-024 §Runtime structs and §Stage 3.
type ExecuteBeadRuntime struct {
	FromRev         string // base git revision (default: HEAD)
	PromptFile      string // override prompt file (auto-generated if empty)
	Output          io.Writer
	WorkerID        string // from DDX_WORKER_ID env or caller
	BeadEvents      BeadEventAppender
	BeadCancel      BeadCancelStore // optional: enables operator-cancel mid-attempt poll
	ResourceChecker ExecutionResourceChecker
	Service         agentlib.FizeauService
	AgentRunner     AgentRunner
	// RateLimitMaxWait bounds the per-bead total wait spent on rate-limit
	// retries (ddx-c6e3db02 RateLimitRetryContract / TD-031 §8.4). Zero uses
	// RateLimitRetryDefaultBudget (5 min). Negative disables the wrapper —
	// rate-limit responses fall through to the standard execution_failed path.
	RateLimitMaxWait time.Duration
	// ACCheckRunner, when non-nil, runs ddx bead ac-check for the given bead
	// and attempt after the agent commits, writing ac-check.json to the
	// attempt dir under wtPath. When nil, ac-check is skipped.
	ACCheckRunner func(ctx context.Context, beadID, attemptID, wtPath string) (*accheck.Output, error)
}

// GitOps abstracts the git operations required by the worker.
// Merge is intentionally excluded — that belongs to the parent-side
// orchestrator (OrchestratorGitOps). UpdateRef/DeleteRef are exposed here
// so landing-side helpers (e.g. BuildLandingGateContext) can pin a
// transient ref while running gate evaluation against an ephemeral
// worktree.
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
	// UpdateRef updates ref in dir to sha. Used by landing helpers to pin a
	// commit so a transient worktree can check it out without racing with
	// other work that might prune it.
	UpdateRef(dir, ref, sha string) error
	// DeleteRef removes ref from dir. Used to unpin a transient ref after
	// the consumer (e.g. an ephemeral worktree) is done with it.
	DeleteRef(dir, ref string) error
}

// AgentRunner runs an agent with the given options.
type AgentRunner interface {
	Run(opts RunArgs) (*Result, error)
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
	PromptSHA   string
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
	// PromptSHA is the sha256 (hex) of the rendered prompt bytes written to
	// prompt.md. Stable across attempts that render the same prompt, so
	// analytics can group attempts by prompt_sha to compare outcomes across
	// prompt rewrites (Story 12 / FEAT-022 measurement).
	PromptSHA string `json:"prompt_sha,omitempty"`
}

type executeBeadRequested struct {
	// Passthrough constraints:
	Harness  string `json:"harness,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	// Power bounds: separate from passthrough constraints.
	MinPower int `json:"min_power,omitempty"`
	MaxPower int `json:"max_power,omitempty"`
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

// GoverningRef is the exported alias for executeBeadGoverningRef, for use
// outside the agent package (e.g. cmd/bead_review.go).
type GoverningRef = executeBeadGoverningRef

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
func executeBeadWorktreePath(projectRoot, beadID, attemptID string) string {
	base := config.ExecutionWorktreeRoot(projectRoot)
	if base == "" {
		base = filepath.Join(os.TempDir(), ExecuteBeadTmpSubdir)
	}
	return filepath.Join(base, ExecuteBeadWtPrefix+beadID+"-"+attemptID)
}

const mixedCommitAndNoChangesRationaleReason = "mixed_commit_and_no_changes_rationale"

// RealGitOps implements GitOps via os/exec git commands.
type RealGitOps struct{}

func (r *RealGitOps) HeadRev(dir string) (string, error) {
	return r.ResolveRev(dir, "HEAD")
}

func (r *RealGitOps) ResolveRev(dir, rev string) (string, error) {
	out, err := internalgit.Command(context.Background(), dir, "rev-parse", rev).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *RealGitOps) WorktreeAdd(dir, wtPath, rev string) error {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "add", "--detach", wtPath, rev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (r *RealGitOps) WorktreeRemove(dir, wtPath string) error {
	_ = internalgit.Command(context.Background(), dir, "worktree", "remove", "--force", wtPath).Run()
	return nil
}

func (r *RealGitOps) WorktreeList(dir string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), dir, "worktree", "list", "--porcelain").Output()
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
	return internalgit.Command(context.Background(), dir, "worktree", "prune").Run()
}

// UpdateRef updates ref in dir to sha via `git update-ref`.
func (r *RealGitOps) UpdateRef(dir, ref, sha string) error {
	out, err := internalgit.Command(context.Background(), dir, "update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DeleteRef removes ref from dir via `git update-ref -d`.
func (r *RealGitOps) DeleteRef(dir, ref string) error {
	out, err := internalgit.Command(context.Background(), dir, "update-ref", "-d", ref).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref -d %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// IsDirty reports whether dir has any uncommitted changes (tracked modifications or untracked files).
func (r *RealGitOps) IsDirty(dir string) (bool, error) {
	out, _ := internalgit.Command(context.Background(), dir, "status", "--porcelain", "--", ".", ":(exclude)"+ExecutionCleanupMetadataFileName).Output()
	return len(bytes.TrimSpace(out)) > 0, nil
}

func dirtyWorktreePaths(dir string) []string {
	out, err := internalgit.Command(context.Background(), dir, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

// EvidenceLandExcludePathspecs returns git pathspec-exclusion fragments
// applied when landEvidence / VerifyCleanWorktree stage the evidence
// directory. Only excludes the multi-thousand-line embedded session log
// (the single file that actually explodes past provider context
// limits). prompt.md, usage.json, manifest.json, and result.json remain
// tracked — they are small and serve the audit-trail contract. The
// gitignore written by `ddx init` already treats .ddx/executions/*/
// embedded as ignored; this pathspec is defence-in-depth for force-add
// paths.
//
// Regression anchor: ddx-39e27896 — session logs landed as tracked
// evidence caused retry prompts to balloon past 2M+ tokens and crash
// every provider with n_keep > n_ctx.
func EvidenceLandExcludePathspecs() []string {
	return []string{
		":(exclude,glob).ddx/executions/*/embedded/**",
	}
}

// EvidenceReviewExcludePathspecs returns git pathspec-exclusion fragments
// applied when review-prompt synthesis runs `git show <rev>` over the
// evidence commit. Broader than EvidenceLandExcludePathspecs: excludes
// prompt.md and usage.json too, because even though they're tracked for
// audit, they're execution-artifact noise from the reviewer's
// perspective — the reviewer wants to see the implementation diff, not
// the prior attempt's prompt or token counters. This also protects
// against old commits (pre-fix) that committed the session log
// directly.
//
// Regression anchor: ddx-39e27896.
func EvidenceReviewExcludePathspecs() []string {
	return []string{
		":(exclude,glob).ddx/executions/*/embedded/**",
		":(exclude,glob).ddx/executions/*/prompt.md",
		":(exclude,glob).ddx/executions/*/usage.json",
	}
}

// SynthesizeCommit stages real file changes, explicitly excluding harness noise
// paths, and creates a commit with msg as the commit message. Returns (true, nil)
// when a commit was made, (false, nil) when nothing real remained to commit
// after exclusions, and (false, err) on failure.
func (r *RealGitOps) SynthesizeCommit(dir, msg string) (bool, error) {
	// Do NOT list already-gitignored paths (.ddx/agent-logs, .ddx/workers) as
	// :(exclude) pathspecs. Git treats a path named by :(exclude) as explicitly
	// referenced, so when the path is also .gitignored git emits "The following
	// paths are ignored by one of your .gitignore files" AND exits 1 — even
	// though the pathspec is trying to SKIP it. Paths already in .gitignore are
	// excluded by default; excludes here are only for paths that would
	// otherwise be tracked.
	addArgs := []string{
		"add", "-A", "--",
		".",
	}
	addArgs = append(addArgs, synthesizeCommitExcludePathspecs(dir)...)
	if err := internalgit.Command(context.Background(), dir, addArgs...).Run(); err != nil {
		return false, fmt.Errorf("staging changes: %w", err)
	}
	statusOut, _ := internalgit.Command(context.Background(), dir, "diff", "--cached", "--name-only").Output()
	if len(bytes.TrimSpace(statusOut)) == 0 {
		return false, nil
	}
	if msg == "" {
		msg = "chore: execute-bead synthesized result commit"
	}
	out, err := internalgit.Command(context.Background(), dir, "commit", "-m", msg).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("synthesize commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

// checkpointPreDispatchDirt captures DDx bookkeeping changes as a commit on
// the current branch without using the caller checkout's real index or commit
// hooks. The checkpoint is intentionally narrow: if the parent checkout also
// contains ordinary implementation files, the attempt fails with an actionable
// error so those changes can be committed in the bead's substantive
// [ddx-<id>] commit instead of being folded into the checkpoint.
func checkpointPreDispatchDirt(projectRoot, attemptID string) (bool, error) {
	if err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return false, nil
	}
	headOut, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return false, nil
	}
	head := strings.TrimSpace(string(headOut))
	if head == "" {
		return false, nil
	}

	dirtyPaths, err := preDispatchCheckpointDirtyPaths(projectRoot)
	if err != nil {
		return false, err
	}
	if len(dirtyPaths) == 0 {
		return false, nil
	}

	allowedPaths := make([]string, 0, len(dirtyPaths))
	blockedPaths := make([]string, 0)
	for _, path := range dirtyPaths {
		if preDispatchCheckpointAllowedPath(path) {
			allowedPaths = append(allowedPaths, path)
			continue
		}
		blockedPaths = append(blockedPaths, path)
	}
	sort.Strings(allowedPaths)
	sort.Strings(blockedPaths)
	if len(blockedPaths) > 0 {
		return false, fmt.Errorf(
			"checkpoint refused to absorb implementation changes outside DDx bookkeeping: %s; commit those files in the bead's [ddx-<id>] substantive commit before rerunning",
			strings.Join(blockedPaths, ", "),
		)
	}
	if len(allowedPaths) == 0 {
		return false, nil
	}

	indexFile, err := os.CreateTemp("", "ddx-pre-dispatch-index-*")
	if err != nil {
		return false, fmt.Errorf("creating temp checkpoint index: %w", err)
	}
	indexPath := indexFile.Name()
	_ = indexFile.Close()
	_ = os.Remove(indexPath)
	defer func() { _ = os.Remove(indexPath) }()

	gitWithIndex := func(args ...string) ([]byte, error) {
		cmd := internalgit.Command(context.Background(), projectRoot, args...)
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+indexPath)
		return cmd.CombinedOutput()
	}

	if out, err := gitWithIndex("read-tree", "HEAD"); err != nil {
		return false, fmt.Errorf("initializing checkpoint index: %s: %w", strings.TrimSpace(string(out)), err)
	}

	addArgs := []string{"add", "-A", "--force", "--"}
	addArgs = append(addArgs, allowedPaths...)
	if out, err := gitWithIndex(addArgs...); err != nil {
		return false, fmt.Errorf("staging checkpoint changes: %s: %w", strings.TrimSpace(string(out)), err)
	}

	changedOut, err := gitWithIndex("diff", "--cached", "--name-only")
	if err != nil {
		return false, fmt.Errorf("checking checkpoint diff: %w", err)
	}
	if len(bytes.TrimSpace(changedOut)) == 0 {
		return false, nil
	}

	treeOut, err := gitWithIndex("write-tree")
	if err != nil {
		return false, fmt.Errorf("writing checkpoint tree: %s: %w", strings.TrimSpace(string(treeOut)), err)
	}
	tree := strings.TrimSpace(string(treeOut))
	msg := "chore: checkpoint pre-execute-bead " + attemptID
	commitOut, err := internalgit.Command(context.Background(), projectRoot,
		"-c", "user.name=ddx-checkpoint",
		"-c", "user.email=checkpoint@ddx.local",
		"commit-tree", tree, "-p", head, "-m", msg,
	).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("creating checkpoint commit: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	commit := strings.TrimSpace(string(commitOut))

	refOut, _ := internalgit.Command(context.Background(), projectRoot, "symbolic-ref", "-q", "HEAD").Output()
	ref := strings.TrimSpace(string(refOut))
	if ref == "" {
		ref = "HEAD"
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "update-ref", ref, commit, head).CombinedOutput(); err != nil {
		return false, fmt.Errorf("advancing checkpoint ref: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := internalgit.Command(context.Background(), projectRoot, "read-tree", "HEAD").CombinedOutput(); err != nil {
		return false, fmt.Errorf("syncing checkpoint index: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

func preDispatchCheckpointDirtyPaths(projectRoot string) ([]string, error) {
	out, err := internalgit.Command(context.Background(), projectRoot,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching", "--", ".").Output()
	if err != nil {
		return nil, fmt.Errorf("listing checkpoint dirt: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	paths := make([]string, 0, 8)
	for len(out) > 0 {
		recordEnd := bytes.IndexByte(out, 0)
		if recordEnd == -1 {
			recordEnd = len(out)
		}
		record := out[:recordEnd]
		if recordEnd < len(out) {
			out = out[recordEnd+1:]
		} else {
			out = nil
		}
		if len(record) < 3 {
			continue
		}
		status := string(record[:2])
		path := filepath.ToSlash(string(record[3:]))
		if status == "!!" && !preDispatchCheckpointAllowedPath(path) {
			continue
		}
		if preDispatchCheckpointIgnoredPath(path) {
			continue
		}
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
		if record[0] == 'R' || record[0] == 'C' {
			recordEnd = bytes.IndexByte(out, 0)
			if recordEnd == -1 {
				recordEnd = len(out)
			}
			record = out[:recordEnd]
			if recordEnd < len(out) {
				out = out[recordEnd+1:]
			} else {
				out = nil
			}
			path := filepath.ToSlash(string(record))
			if status == "!!" && !preDispatchCheckpointAllowedPath(path) {
				continue
			}
			if preDispatchCheckpointIgnoredPath(path) {
				continue
			}
			if path != "" && !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

func preDispatchCheckpointAllowedPath(path string) bool {
	switch {
	case path == ".ddx/beads.jsonl":
		return true
	case path == ".ddx/beads-archive.jsonl":
		return true
	case strings.HasPrefix(path, ".ddx/executions/"):
		return true
	case strings.HasPrefix(path, ".ddx/metrics/"):
		return true
	case strings.HasPrefix(path, ".ddx/runs/"):
		return true
	case strings.HasPrefix(path, ".ddx/run-state/"):
		return true
	case path == ".ddx/run-state.json":
		return true
	case path == ExecutionCleanupMetadataFileName:
		return true
	default:
		return false
	}
}

func preDispatchCheckpointIgnoredPath(path string) bool {
	switch {
	case path == ".ddx/.git-tracker.lock":
		return true
	case strings.HasPrefix(path, ".ddx/.git-tracker.lock/"):
		return true
	case path == ".ddx/beads.lock":
		return true
	case strings.HasPrefix(path, ".ddx/beads.lock/"):
		return true
	case strings.HasPrefix(path, ".ddx/attachments/"):
		return true
	case strings.HasPrefix(path, ".ddx/beads.jsonl.tmp-"):
		return true
	default:
		return false
	}
}

func synthesizeCommitExcludePathspecs(dir string) []string {
	candidates := []struct {
		pathspec    string
		ignoreProbe string
	}{
		{
			pathspec:    ":(exclude).ddx/executions/*/embedded",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/embedded",
		},
		{
			pathspec:    ":(exclude).ddx/executions/*/no_changes_rationale.txt",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/no_changes_rationale.txt",
		},
		// Exclude DDx-managed evidence bundle files written to the attempt
		// worktree by execute_bead.go itself. These are committed separately by
		// commitEvidenceBundleInWorktree and must not appear in the agent's code
		// commit (which SynthesizeCommit creates). Prior to this worktree-evidence
		// design, these files lived in projectRoot and were invisible to
		// SynthesizeCommit (which runs in wtPath).
		{
			pathspec:    ":(exclude).ddx/executions/*/prompt.md",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/prompt.md",
		},
		{
			pathspec:    ":(exclude).ddx/executions/*/manifest.json",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/manifest.json",
		},
		{
			pathspec:    ":(exclude).ddx/executions/*/result.json",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/result.json",
		},
		{
			pathspec:    ":(exclude).ddx/executions/*/usage.json",
			ignoreProbe: ".ddx/executions/.ddx-check-ignore/usage.json",
		},
		{
			pathspec:    ":(exclude).claude/skills",
			ignoreProbe: ".claude/skills",
		},
		{
			pathspec:    ":(exclude).agents/skills",
			ignoreProbe: ".agents/skills",
		},
		{
			pathspec:    ":(exclude)" + ExecutionCleanupMetadataFileName,
			ignoreProbe: ExecutionCleanupMetadataFileName,
		},
		{
			// Tracker-lock coordination dir (.ddx/.git-tracker.lock/{pid,
			// acquired_at}). Present while withTrackerLock is held — must
			// not be staged by a SynthesizeCommit running inside the lock
			// (regression: HEAD-race fix folded SynthesizeCommit into the
			// locked critical section, exposing this directory to
			// `git add -A`).
			pathspec:    ":(exclude).ddx/.git-tracker.lock",
			ignoreProbe: ".ddx/.git-tracker.lock/pid",
		},
	}

	pathspecs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if isGitIgnored(dir, c.ignoreProbe) {
			continue
		}
		pathspecs = append(pathspecs, c.pathspec)
	}
	return pathspecs
}

func isGitIgnored(dir, path string) bool {
	err := internalgit.Command(context.Background(), dir, "check-ignore", "-q", "--", path).Run()
	return err == nil
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

// appendBeadRoutingEvidence records a kind:routing evidence entry on the bead
// after an agent run. It is a best-effort operation — errors are silently
// ignored so a store failure never aborts the main execute-bead flow.
//
// The body separates requested passthrough constraints (harness/provider/model
// from the CLI envelope) and requested power bounds from the actual resolved
// values so analytics can distinguish intent from outcome (AC5 of ddx-20047dd5).
func appendBeadRoutingEvidence(appender BeadEventAppender, beadID, harness, provider, model, routeReason, baseURL string, passthrough config.AgentPassthrough, minPower, maxPower, actualPower int) {
	if appender == nil || beadID == "" {
		return
	}
	resolvedProvider := provider
	if resolvedProvider == "" {
		resolvedProvider = harness
	}
	type routingBody struct {
		ResolvedProvider  string   `json:"resolved_provider"`
		ResolvedModel     string   `json:"resolved_model,omitempty"`
		RouteReason       string   `json:"route_reason,omitempty"`
		FallbackChain     []string `json:"fallback_chain"`
		BaseURL           string   `json:"base_url,omitempty"`
		RequestedHarness  string   `json:"requested_harness,omitempty"`
		RequestedProvider string   `json:"requested_provider,omitempty"`
		RequestedModel    string   `json:"requested_model,omitempty"`
		RequestedMinPower int      `json:"requested_min_power,omitempty"`
		RequestedMaxPower int      `json:"requested_max_power,omitempty"`
		ActualPower       int      `json:"actual_power,omitempty"`
	}
	body := routingBody{
		ResolvedProvider:  resolvedProvider,
		ResolvedModel:     model,
		RouteReason:       routeReason,
		FallbackChain:     []string{},
		BaseURL:           baseURL,
		RequestedHarness:  passthrough.Harness,
		RequestedProvider: passthrough.Provider,
		RequestedModel:    passthrough.Model,
		RequestedMinPower: minPower,
		RequestedMaxPower: maxPower,
		ActualPower:       actualPower,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := fmt.Sprintf("provider=%s", resolvedProvider)
	if model != "" {
		summary += fmt.Sprintf(" model=%s", model)
	}
	if routeReason != "" {
		summary += fmt.Sprintf(" reason=%s", routeReason)
	}
	_ = appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "routing",
		Summary: summary,
		Body:    string(data),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	})
}

// costEventBody is the JSON shape persisted in a kind:cost evidence event.
// `ddx bead metrics aggregate` reads these directly so cost rollup never
// has to join against the session index.
type costEventBody struct {
	AttemptID    string  `json:"attempt_id"`
	Harness      string  `json:"harness,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	Model        string  `json:"model,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMS   int     `json:"duration_ms"`
	ExitCode     int     `json:"exit_code"`
}

// appendBeadCostEvidence records a kind:cost evidence entry on the bead with
// per-attempt token and dollar usage. Best-effort: errors are discarded so a
// store failure never aborts the main execute-bead flow. Emits nothing when
// the appender is nil, the beadID is empty, or every cost field is zero
// (e.g., dry-run, no-changes with no provider call).
func appendBeadCostEvidence(appender BeadEventAppender, beadID, attemptID string, body costEventBody) {
	if appender == nil || beadID == "" {
		return
	}
	if body.InputTokens == 0 && body.OutputTokens == 0 && body.TotalTokens == 0 && body.CostUSD == 0 {
		return
	}
	body.AttemptID = attemptID
	if body.TotalTokens == 0 {
		body.TotalTokens = body.InputTokens + body.OutputTokens
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	var summary string
	if body.CostUSD > 0 {
		summary = fmt.Sprintf("tokens=%d cost_usd=%.4f", body.TotalTokens, body.CostUSD)
	} else {
		summary = fmt.Sprintf("tokens=%d", body.TotalTokens)
	}
	if body.Model != "" {
		summary += fmt.Sprintf(" model=%s", body.Model)
	}
	_ = appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:    "cost",
		Summary: summary,
		Body:    string(data),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	})
}

func startRunStateRefresh(ctx context.Context, projectRoot string, state RunState) func() {
	interval := RunStateRefreshInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = WriteRunState(projectRoot, mergeLatestCandidateCycleRunState(projectRoot, state))
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func mergeLatestCandidateCycleRunState(projectRoot string, state RunState) RunState {
	states, err := ReadRunStates(projectRoot)
	if err != nil {
		return state
	}
	for i := range states {
		current := states[i]
		if state.AttemptID != "" && current.AttemptID != state.AttemptID {
			continue
		}
		if state.AttemptID == "" && state.WorktreePath != "" && current.WorktreePath != "" &&
			filepath.Clean(current.WorktreePath) != filepath.Clean(state.WorktreePath) {
			continue
		}
		state.CandidateCyclePhase = current.CandidateCyclePhase
		state.CandidateRef = current.CandidateRef
		state.CandidateRev = current.CandidateRev
		state.CycleIndex = current.CycleIndex
		state.ReviewActive = current.ReviewActive
		state.RepairActive = current.RepairActive
		return state
	}
	return state
}

func buildAttemptDiagnostic(projectRoot, wtPath, beadID, attemptID, headRevErr string, gitOps GitOps) *AttemptDiagnostic {
	diag := &AttemptDiagnostic{
		BeadID:       beadID,
		AttemptID:    attemptID,
		WorktreePath: wtPath,
		HeadRevError: headRevErr,
	}
	if _, err := os.Stat(wtPath); err == nil {
		diag.WorktreePathExists = true
	}
	if _, err := ReadExecutionCleanupMetadata(wtPath); err == nil {
		diag.CleanupMetadataPresent = true
	}
	if states, err := ReadRunStates(projectRoot); err == nil {
		if state := matchingRunStateForAttempt(states, attemptID, wtPath); state != nil {
			diag.RunStatePresent = true
			copied := *state
			diag.RunState = &copied
		}
	}
	if gitOps != nil {
		if paths, err := gitOps.WorktreeList(projectRoot); err == nil {
			for _, path := range paths {
				if filepath.Clean(path) == filepath.Clean(wtPath) {
					diag.GitWorktreeRegistered = true
					break
				}
			}
		}
	}
	return diag
}

func matchingRunStateForAttempt(states []RunState, attemptID, wtPath string) *RunState {
	for i := range states {
		if attemptID != "" && states[i].AttemptID == attemptID {
			return &states[i]
		}
		if wtPath != "" && states[i].WorktreePath != "" && filepath.Clean(states[i].WorktreePath) == filepath.Clean(wtPath) {
			return &states[i]
		}
	}
	return nil
}

// appendRateLimitRetryEvent records one rate-limit retry on the bead's
// event stream per TD-031 §4 / §8.4 RateLimitRetryContract. The event body
// carries the retry count and wait duration so an audit can reconstruct the
// pause without consulting routing logs.
func appendRateLimitRetryEvent(appender BeadEventAppender, beadID string, info RateLimitRetryInfo) {
	if appender == nil || beadID == "" {
		return
	}
	body := map[string]any{
		"attempt":     info.Attempt,
		"wait_ms":     info.Wait.Milliseconds(),
		"source":      info.Source,
		"elapsed_ms":  info.Elapsed.Milliseconds(),
		"over_budget": info.OverBudget,
	}
	if info.Result != nil {
		if info.Result.Harness != "" {
			body["harness"] = info.Result.Harness
		}
		if info.Result.Provider != "" {
			body["provider"] = info.Result.Provider
		}
		if info.Result.Model != "" {
			body["model"] = info.Result.Model
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	summary := fmt.Sprintf("attempt=%d wait=%s source=%s", info.Attempt, info.Wait.Round(time.Millisecond), info.Source)
	if info.OverBudget {
		summary = "budget exhausted: " + RateLimitBudgetExhaustedReason
	}
	_ = appender.AppendEvent(beadID, bead.BeadEvent{
		Kind:    RateLimitRetryEventKind,
		Summary: summary,
		Body:    string(data),
		Actor:   "ddx",
		Source:  "legacy agent execute-bead",
	})
}

// ExecuteBeadWithConfig is the thin worker: it creates an isolated worktree,
// constructs the agent prompt from bead context, runs the agent harness,
// synthesizes a commit if the agent left uncommitted changes, then cleans up
// the worktree and returns the result. It classifies outcomes as exactly one
// of:
//
//   - task_succeeded: agent exited 0 and produced one or more commits
//   - task_failed:    agent exited non-zero
//   - task_no_changes: agent exited 0, made no commits, and wrote a rationale
//   - task_no_evidence: agent exited 0 but made no commits and wrote no rationale
//
// Merge, UpdateRef, gate evaluation, preserve-ref management, and orphan
// recovery are the parent's responsibility (see LandBeadResult, RecoverOrphans).
//
// Agent dispatch: production callers leave runtime.Service and runtime.AgentRunner
// nil. ExecuteBeadWithConfig constructs a fresh agentlib.FizeauService from
// projectRoot via NewServiceFromWorkDir and dispatches via RunViaServiceWith.
// Tests may set runtime.AgentRunner to inject a fake that returns canned
// Result values; when set, it takes precedence over the service path.
func ExecuteBeadWithConfig(ctx context.Context, projectRoot string, beadID string, rcfg config.ResolvedConfig, runtime ExecuteBeadRuntime, gitOps GitOps) (*ExecuteBeadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = rcfg.Harness()
	if runtime.WorkerID == "" {
		runtime.WorkerID = os.Getenv("DDX_WORKER_ID")
	}

	resourceChecker := runtime.ResourceChecker
	if resourceChecker == nil {
		resourceChecker = NewExecutionResourceChecker(projectRoot, gitOps)
	}
	if _, err := resourceChecker.Check(ctx); err != nil {
		var resourceErr *ResourceExhaustedError
		if errors.As(err, &resourceErr) {
			res := &ExecuteBeadResult{
				BeadID:            beadID,
				WorkerID:          runtime.WorkerID,
				ExitCode:          1,
				Error:             resourceErr.Error(),
				Reason:            resourceErr.Error(),
				Outcome:           ExecuteBeadOutcomeTaskFailed,
				Status:            ExecuteBeadStatusResourceExhausted,
				ResourceExhausted: &resourceErr.Result,
			}
			res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
			return res, nil
		}
		return nil, err
	}

	attemptID := GenerateAttemptID()
	wtPath := executeBeadWorktreePath(projectRoot, beadID, attemptID)

	// Serialize the parent-repo mutating pre-dispatch sequence (tracker
	// commit, caller-dirt checkpoint, base resolution, worktree creation)
	// under the tracker lock. Without this, concurrent workers race on the
	// parent's HEAD ref between unlocked CommitTracker and SynthesizeCommit
	// and surface as "cannot lock ref 'HEAD': is at X but expected Y" CAS
	// failures (regression: concurrent `ddx work` against the same project).
	// The lock is held only across the brief pre-dispatch sequence (~1-2s);
	// agent execution that follows is fully parallel.
	var baseRev string
	if err := withTrackerLock(projectRoot, func() error {
		if mkErr := os.MkdirAll(filepath.Dir(wtPath), 0o755); mkErr != nil {
			return fmt.Errorf("creating execute-bead worktree parent dir: %w", mkErr)
		}
		// Commit beads.jsonl before spawning worktree so the worktree snapshot
		// includes any bead metadata updates (e.g. spec-id).
		if err := commitTrackerLocked(projectRoot); err != nil {
			return err
		}
		// Checkpoint any remaining caller dirt as a real commit on the current
		// branch (FEAT-012 §22, US-126 AC#1). Use a temp-index commit-tree path
		// here so parent checkout hooks and runtime artifacts cannot fail the
		// dispatch before the isolated worker worktree exists.
		if _, err := checkpointPreDispatchDirt(projectRoot, attemptID); err != nil {
			return fmt.Errorf("pre-execute-bead checkpoint: %w", err)
		}
		// Resolve base revision after the tracker + checkpoint commits.
		rev, err := resolveBase(gitOps, projectRoot, runtime.FromRev)
		if err != nil {
			return err
		}
		baseRev = rev
		// Create the isolated worktree. Orphan recovery is the parent's responsibility
		// (call RecoverOrphans before invoking workers).
		if err := gitOps.WorktreeAdd(projectRoot, wtPath, baseRev); err != nil {
			_ = os.RemoveAll(wtPath)
			return fmt.Errorf("creating isolated worktree: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := excludeCleanupMetadataFromWorktreeGit(wtPath); err != nil {
		_ = gitOps.WorktreeRemove(projectRoot, wtPath)
		return nil, fmt.Errorf("excluding execute-bead cleanup metadata: %w", err)
	}
	if err := WriteExecutionCleanupMetadata(wtPath, ExecutionCleanupMetadata{
		ProjectRoot:  projectRoot,
		BeadID:       beadID,
		AttemptID:    attemptID,
		WorktreePath: wtPath,
		Registered:   true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		_ = gitOps.WorktreeRemove(projectRoot, wtPath)
		return nil, fmt.Errorf("writing execute-bead cleanup metadata: %w", err)
	}
	defer func() {
		_ = gitOps.WorktreeRemove(projectRoot, wtPath)
	}()

	// evidenceCommittedInWt is true when the evidence bundle was successfully
	// committed inside the attempt worktree (normal success path). The deferred
	// publish below is a no-op in that case; for all other paths it copies the
	// bundle from wtPath to projectRoot so VerifyCleanWorktree can commit it.
	evidenceDir := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))
	var evidenceCommittedInWt bool
	defer func() {
		// Runs BEFORE WorktreeRemove (registered later = first in LIFO order).
		if evidenceCommittedInWt {
			publishEvidenceBundleToProjectRoot(projectRoot, wtPath, filepath.ToSlash(filepath.Join(evidenceDir, "embedded")))
		} else {
			publishEvidenceBundleToProjectRoot(projectRoot, wtPath, evidenceDir)
		}
	}()

	// Publish the live run-state so operators and HELIX can observe what is
	// executing without polling the bead tracker (CONTRACT-001 §5). The file
	// is removed on completion; a crashed worker leaves a stale file that
	// RecoverOrphans sweeps before the next attempt.
	runState := RunState{
		BeadID:       beadID,
		AttemptID:    attemptID,
		Harness:      rcfg.Harness(),
		Model:        rcfg.Model(),
		StartedAt:    time.Now().UTC(),
		WorktreePath: wtPath,
	}
	_ = WriteRunState(projectRoot, runState)
	defer func() {
		_ = ClearRunStateAttempt(projectRoot, attemptID)
	}()

	// Repair project-local skill symlinks whose targets do not resolve inside
	// the freshly created worktree.
	_ = materializeWorktreeSkills(wtPath)

	// Prepare artifacts (context load, prompt generation).
	artifacts, err := prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev, rcfg, runtime)
	if err != nil {
		res := &ExecuteBeadResult{
			BeadID:    beadID,
			AttemptID: attemptID,
			WorkerID:  runtime.WorkerID,
			BaseRev:   baseRev,
			ResultRev: baseRev, // no commits; ResultRev == BaseRev signals no output
			ExitCode:  1,
			Error:     err.Error(),
			Outcome:   ExecuteBeadOutcomeTaskFailed,
		}
		// Bundle lives in the attempt worktree after the fix; check there.
		wtBundleDir := filepath.Join(wtPath, ExecuteBeadArtifactDir, attemptID)
		if abInfo, _ := os.Stat(wtBundleDir); abInfo != nil && abInfo.IsDir() {
			res.ExecutionDir = filepath.Join(ExecuteBeadArtifactDir, attemptID)
		}
		res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
		populateWorkerStatus(res)
		_ = writeArtifactJSON(filepath.Join(wtBundleDir, "result.json"), res)
		// The deferred publish will copy from wtPath to projectRoot before cleanup.
		return res, fmt.Errorf("execute-bead context load: %w", err)
	}

	// Pre-create the execution bundle dir in the worktree so the agent can write
	// artifacts (e.g. no_changes_rationale.txt) without needing to create the
	// directory itself. Failures are non-fatal: the agent can create it on demand.
	_ = os.MkdirAll(filepath.Join(wtPath, artifacts.DirRel), 0o755)

	// Redirect per-run session/telemetry output into the DDx-owned execution
	// bundle so the embedded harness does not accumulate state at the worktree root.
	embeddedStateDir := filepath.Join(artifacts.DirAbs, "embedded")
	if err := os.MkdirAll(embeddedStateDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating embedded state dir: %w", err)
	}

	sessionID := GenerateSessionID()
	startedAt := time.Now().UTC()
	runState.SessionID = sessionID
	_ = WriteRunState(projectRoot, runState)

	runRuntime := AgentRunRuntime{
		PromptFile: artifacts.PromptAbs,
		Output:     runtime.Output,
		WorkDir:    wtPath,
		Correlation: map[string]string{
			"bead_id":     beadID,
			"base_rev":    baseRev,
			"attempt_id":  attemptID,
			"session_id":  sessionID,
			"worker_id":   runtime.WorkerID,
			"bundle_path": artifacts.DirRel,
			"prompt_file": artifacts.PromptRel,
			"prompt_sha":  artifacts.PromptSHA,
		},
		SessionLogDirOverride: embeddedStateDir,
		PermissionsOverride:   "unrestricted", // isolated worktree; writes must not require approval
		Role:                  "implementer",
		CorrelationID:         beadID + ":" + attemptID,
		Env: map[string]string{
			DDXModeEnvKey: DDXModeBeadExecution,
		},
	}

	// Operator-cancel mid-attempt poll (ADR-022 §Cancel SLA). The poll
	// re-reads the bead's Extra every CancelPollInterval; on
	// cancel-requested:true it cancels the dispatch context (which kills the
	// agent subprocess at the next syscall boundary) and writes
	// cancel-honored:true so subsequent cancel POSTs are silent no-ops.
	dispatchCtx, dispatchCancel := context.WithCancel(ctx)
	defer dispatchCancel()
	cancelHonored := startCancelPoll(dispatchCtx, dispatchCancel, beadID, runtime.BeadCancel)

	stopRunStateRefresh := startRunStateRefresh(dispatchCtx, projectRoot, runState)
	agentResult, agentErr := dispatchAgentRun(dispatchCtx, projectRoot, runtime.Service, runtime.AgentRunner, rcfg, runRuntime)
	stopRunStateRefresh()
	finishedAt := time.Now().UTC()

	exitCode := 0
	tokens := 0
	inputTokens := 0
	outputTokens := 0
	costUSD := 0.0
	resultModel := rcfg.Model()
	resultHarness := rcfg.Harness()
	resultProvider := ""
	actualPower := 0
	predictedPower := 0
	predictedSpeedTPS := 0.0
	predictedCostUSDPer1kTokens := 0.0
	predictedCostSource := ""
	agentErrMsg := ""
	agentStderr := ""
	if agentResult != nil {
		exitCode = agentResult.ExitCode
		tokens = agentResult.Tokens
		inputTokens = agentResult.InputTokens
		outputTokens = agentResult.OutputTokens
		costUSD = agentResult.CostUSD
		if agentResult.Error != "" {
			agentErrMsg = agentResult.Error
		}
		if agentResult.Stderr != "" {
			agentStderr = agentResult.Stderr
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
		if agentResult.ActualPower > 0 {
			actualPower = agentResult.ActualPower
		}
		predictedPower = agentResult.PredictedPower
		predictedSpeedTPS = agentResult.PredictedSpeedTPS
		predictedCostUSDPer1kTokens = agentResult.PredictedCostUSDPer1kTokens
		predictedCostSource = agentResult.PredictedCostSource
	}
	if agentErr != nil {
		if exitCode == 0 {
			exitCode = 1
		}
		agentErrMsg = agentErr.Error()
	}

	// Capture routing evidence from the agent result. These fields are
	// populated by RunAgent (embedded harness) and RunScript (script harness).
	routeReason := ""
	routeBaseURL := ""
	if agentResult != nil {
		routeReason = agentResult.RouteReason
		routeBaseURL = agentResult.ResolvedBaseURL
	}

	// Get the HEAD of the worktree after the agent ran.
	resultRev, revErr := gitOps.HeadRev(wtPath)
	if revErr != nil {
		headRevErr := fmt.Sprintf("failed to read worktree HEAD: %v", revErr)
		res := &ExecuteBeadResult{
			BeadID:                      beadID,
			AttemptID:                   attemptID,
			WorkerID:                    runtime.WorkerID,
			BaseRev:                     baseRev,
			ResultRev:                   baseRev, // no commits readable; treat as no output
			Harness:                     resultHarness,
			Provider:                    resultProvider,
			Model:                       resultModel,
			ActualPower:                 actualPower,
			PredictedPower:              predictedPower,
			PredictedSpeedTPS:           predictedSpeedTPS,
			PredictedCostUSDPer1kTokens: predictedCostUSDPer1kTokens,
			PredictedCostSource:         predictedCostSource,
			SessionID:                   sessionID,
			DurationMS:                  int(finishedAt.Sub(startedAt).Milliseconds()),
			Tokens:                      tokens,
			CostUSD:                     costUSD,
			ExitCode:                    1,
			Error:                       strings.TrimSpace(strings.Join([]string{agentErrMsg, headRevErr}, "\n")),
			Stderr:                      agentStderr,
			RateLimitBudget:             runtime.RateLimitMaxWait,
			Reason:                      headRevErr, // HeadRev failure; orchestrator prefers this over Error for Reason
			ExecutionDir:                artifacts.DirRel,
			PromptFile:                  artifacts.PromptRel,
			ManifestFile:                artifacts.ManifestRel,
			ResultFile:                  artifacts.ResultRel,
			WorktreePath:                wtPath,
			StartedAt:                   startedAt,
			FinishedAt:                  finishedAt,
			Outcome:                     ExecuteBeadOutcomeTaskFailed,
		}
		res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
		if res.FailureMode == FailureModeWorktreeLost {
			res.AttemptDiagnostics = buildAttemptDiagnostic(projectRoot, wtPath, beadID, attemptID, headRevErr, gitOps)
		}
		populateWorkerStatus(res)
		// Check if wtPath is gone BEFORE writeArtifactJSON — that call uses
		// os.MkdirAll internally and would recreate the directory, making a
		// subsequent os.Stat check fail to detect the vanished worktree.
		_, wtStatErr := os.Stat(wtPath)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		// Fallback: when the worktree was gone before the write above, write
		// evidence directly to projectRoot so the diagnostic files are recoverable.
		if os.IsNotExist(wtStatErr) {
			recoveryDir := filepath.Join(projectRoot, artifacts.DirRel)
			_ = os.MkdirAll(recoveryDir, 0o755)
			_ = writeArtifactJSON(filepath.Join(recoveryDir, "result.json"), res)
			// Stubs for prompt.md and manifest.json that were lost with the worktree.
			_ = os.WriteFile(filepath.Join(recoveryDir, "prompt.md"),
				[]byte("# Evidence Recovery\nWorktree was lost before prompt could be preserved.\n"), 0o644)
			_ = writeArtifactJSON(filepath.Join(recoveryDir, "manifest.json"), map[string]string{
				"attempt_id": attemptID, "bead_id": beadID, "status": "worktree_lost",
			})
		}
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
				BeadID:                      beadID,
				AttemptID:                   attemptID,
				WorkerID:                    runtime.WorkerID,
				BaseRev:                     baseRev,
				ResultRev:                   "", // unknown until commit is made
				Harness:                     resultHarness,
				Provider:                    resultProvider,
				Model:                       resultModel,
				ActualPower:                 actualPower,
				PredictedPower:              predictedPower,
				PredictedSpeedTPS:           predictedSpeedTPS,
				PredictedCostUSDPer1kTokens: predictedCostUSDPer1kTokens,
				PredictedCostSource:         predictedCostSource,
				SessionID:                   sessionID,
				DurationMS:                  int(finishedAt.Sub(startedAt).Milliseconds()),
				Tokens:                      tokens,
				CostUSD:                     costUSD,
				ExitCode:                    exitCode,
				Error:                       agentErrMsg,
				Stderr:                      agentStderr,
				RateLimitBudget:             runtime.RateLimitMaxWait,
				ExecutionDir:                artifacts.DirRel,
				PromptFile:                  artifacts.PromptRel,
				ManifestFile:                artifacts.ManifestRel,
				ResultFile:                  artifacts.ResultRel,
				UsageFile:                   usageFileRel,
				StartedAt:                   startedAt,
				FinishedAt:                  finishedAt,
				Outcome:                     prelimOutcome,
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
		BeadID:                      beadID,
		AttemptID:                   attemptID,
		WorkerID:                    runtime.WorkerID,
		BaseRev:                     baseRev,
		ResultRev:                   resultRev,
		Harness:                     resultHarness,
		Provider:                    resultProvider,
		Model:                       resultModel,
		ActualPower:                 actualPower,
		PredictedPower:              predictedPower,
		PredictedSpeedTPS:           predictedSpeedTPS,
		PredictedCostUSDPer1kTokens: predictedCostUSDPer1kTokens,
		PredictedCostSource:         predictedCostSource,
		SessionID:                   sessionID,
		DurationMS:                  int(finishedAt.Sub(startedAt).Milliseconds()),
		Tokens:                      tokens,
		CostUSD:                     costUSD,
		ExitCode:                    exitCode,
		Error:                       agentErrMsg,
		Stderr:                      agentStderr,
		RateLimitBudget:             runtime.RateLimitMaxWait,
		ExecutionDir:                artifacts.DirRel,
		PromptFile:                  artifacts.PromptRel,
		ManifestFile:                artifacts.ManifestRel,
		ResultFile:                  artifacts.ResultRel,
		UsageFile:                   usageFileRel,
		StartedAt:                   startedAt,
		FinishedAt:                  finishedAt,
	}

	// Classify worker outcome: task_succeeded / task_failed / task_no_changes /
	// task_no_evidence. A clean agent exit with no commit is only a legitimate
	// no_changes signal when the agent also wrote the explicit rationale file.
	// If a rationale file appears alongside implementation commits, reject the
	// mixed signal so the parent orchestrator does not see a clean success.
	// The parent orchestrator (LandBeadResult + ApplyLandingToResult) will
	// overwrite Outcome and Status with the landing decision before output.
	agentReportedError := strings.TrimSpace(agentErrMsg) != ""
	rationaleFile := filepath.Join(wtPath, artifacts.DirRel, "no_changes_rationale.txt")
	rationaleText := ""
	if data, readErr := os.ReadFile(rationaleFile); readErr == nil {
		rationaleText = strings.TrimSpace(string(data))
		if rationaleText != "" {
			res.NoChangesRationale = rationaleText
		}
	}
	mixedCommitAndRationale := resultRev != baseRev && rationaleText != ""
	switch {
	case mixedCommitAndRationale:
		res.Outcome = ExecuteBeadOutcomeTaskFailed
		res.Reason = mixedCommitAndNoChangesRationaleReason
		res.Error = mixedCommitAndNoChangesRationaleReason
	case exitCode != 0 || agentReportedError:
		res.Outcome = ExecuteBeadOutcomeTaskFailed
	case resultRev == baseRev:
		res.Outcome = ExecuteBeadOutcomeTaskNoChanges
	default:
		res.Outcome = ExecuteBeadOutcomeTaskSucceeded
	}

	// When the outcome is no_changes, the agent's rationale file becomes the
	// canonical explanation for the no-op decision. We read it before the
	// deferred worktree cleanup removes the file.
	if res.Outcome == ExecuteBeadOutcomeTaskNoChanges {
		if res.NoChangesRationale == "" {
			res.Outcome = ExecuteBeadOutcomeTaskNoEvidence
			res.Reason = "agent exited without a commit or no_changes_rationale.txt"
			paths := dirtyWorktreePaths(wtPath)
			res.NoEvidencePaths = paths
			if len(paths) > 0 {
				res.Error = fmt.Sprintf("%s; dirty paths: %s", res.Reason, strings.Join(paths, ", "))
			} else {
				res.Error = res.Reason
			}
		}
	}

	// Operator-cancel override (ADR-022 §Cancel SLA). When the mid-attempt
	// poll detected cancel-requested:true on the bead, force the result into
	// the preserved_for_review channel with reason "operator_cancel" so the
	// orchestrator and supervisor surface it as an operator-driven preserve
	// rather than a generic task_failed/no_evidence outcome.
	if cancelHonored != nil && cancelHonored.Load() {
		res.Outcome = "preserved"
		res.Reason = OperatorCancelReason
		res.Error = ""
		res.ExitCode = 0
		res.FailureMode = ""
		res.Status = ExecuteBeadStatusPreservedNeedsReview
		res.Detail = ExecuteBeadStatusDetail(res.Status, OperatorCancelReason, "")
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, nil
	}

	// Classify failure mode from worker-level signals. ApplyLandingToResult
	// may refine this with landing-level signals (merge conflict, gate
	// failure) before the final result is output.
	res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)

	// Record routing evidence on the bead (best-effort; errors are discarded).
	// Include requested passthrough constraints and power bounds so analytics
	// can separate intent from resolved outcome (AC5 of ddx-20047dd5).
	appendBeadRoutingEvidence(runtime.BeadEvents, beadID, resultHarness, resultProvider, resultModel, routeReason, routeBaseURL, rcfg.Passthrough(), rcfg.MinPower(), rcfg.MaxPower(), actualPower)

	// Record per-attempt cost evidence so cost rollup never has to join
	// against the session index. Best-effort; errors are discarded.
	appendBeadCostEvidence(runtime.BeadEvents, beadID, attemptID, costEventBody{
		Harness:      resultHarness,
		Provider:     resultProvider,
		Model:        resultModel,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  tokens,
		CostUSD:      costUSD,
		DurationMS:   res.DurationMS,
		ExitCode:     exitCode,
	})

	populateWorkerStatus(res)
	if err := writeArtifactJSON(artifacts.ResultAbs, res); err != nil {
		return nil, fmt.Errorf("writing execute-bead result artifact: %w", err)
	}

	// For the normal success path, commit the evidence bundle in the attempt
	// worktree so Land() can find it in the landing finalization worktree
	// without copying from the project root (AC: no live-worker noise in main).
	if resultRev != baseRev && exitCode == 0 {
		if newRev := commitEvidenceBundleInWorktree(wtPath, artifacts.DirRel, attemptID); newRev != "" {
			res.ResultRev = newRev
			evidenceCommittedInWt = true
		}
	}

	// Optional out-of-band mirror of the bundle (and, when configured, the
	// associated agent-log and worker state). Wired here so the whole
	// per-attempt directory (manifest, prompt, result, usage, checks,
	// embedded/) is on disk before the upload starts. Failures never affect
	// the bead outcome — see executions_mirror.go.
	MirrorOrLog(MirrorRequest{
		ProjectRoot: projectRoot,
		AttemptID:   attemptID,
		BeadID:      beadID,
		BundleDir:   artifacts.DirAbs,
		Cfg:         rcfg.MirrorConfig(),
		SessionID:   sessionID,
		WorkerID:    runtime.WorkerID,
	})

	return res, nil
}

// dispatchAgentRun is a thin SD-024 wrapper around dispatchViaResolvedConfig
// for the execute-bead worker. It threads the durable knobs from rcfg and the
// per-invocation plumbing from runtime through the shared dispatch seam in
// service_run.go.
//
// The script and virtual harnesses are DDx-side helpers that the agent service
// does not implement; the underlying RunViaServiceWith path delegates those to
// a private Runner internally, so they continue to work through this path.
func dispatchAgentRun(ctx context.Context, projectRoot string, svc agentlib.FizeauService, runner AgentRunner, rcfg config.ResolvedConfig, runtime AgentRunRuntime) (*Result, error) {
	return dispatchViaResolvedConfig(ctx, projectRoot, svc, runner, rcfg, runtime)
}

// populateWorkerStatus fills in the Status and Detail fields on a worker result
// based on the task-level Outcome.
func populateWorkerStatus(res *ExecuteBeadResult) {
	switch res.Outcome {
	case ExecuteBeadOutcomeTaskSucceeded:
		res.Status = ExecuteBeadStatusSuccess
	case ExecuteBeadOutcomeTaskNoChanges:
		res.Status = ExecuteBeadStatusNoChanges
	case ExecuteBeadOutcomeTaskNoEvidence:
		res.Status = ExecuteBeadStatusNoEvidenceProduced
	default:
		res.Status = ExecuteBeadStatusExecutionFailed
	}
	res.Detail = ExecuteBeadStatusDetail(res.Status, "", res.Error)
}

// CommitTracker commits beads.jsonl if it has uncommitted changes. Acquires
// withTrackerLock so concurrent callers serialize on the parent .git index.
// Callers already holding the lock must use commitTrackerLocked instead —
// the lock is not re-entrant.
func CommitTracker(projectRoot string) error {
	return withTrackerLock(projectRoot, func() error {
		return commitTrackerLocked(projectRoot)
	})
}

// commitTrackerLocked is the body of CommitTracker without the lock
// acquisition. Callers must hold withTrackerLock(projectRoot) before calling.
// Used by Run() to fold tracker commit, checkpoint synthesis, and worktree
// creation into a single critical section so concurrent workers do not race
// on the parent's HEAD ref (regression: niflheim concurrent `ddx work` hit
// "cannot lock ref 'HEAD'" between unlocked CommitTracker and SynthesizeCommit).
func commitTrackerLocked(projectRoot string) error {
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

	out, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	msg := fmt.Sprintf("chore: update tracker (execute-bead %s)", time.Now().UTC().Format("20060102T150405"))

	// Re-check inside the lock: a sibling worker may have already
	// committed the tracker changes between our pre-lock check and now.
	diff, err := internalgit.Command(context.Background(), projectRoot, "diff", "--", ".ddx/beads.jsonl").Output()
	if err != nil {
		return fmt.Errorf("checking tracker diff: %w", err)
	}
	if strings.TrimSpace(string(diff)) == "" {
		untracked, err := internalgit.Command(context.Background(), projectRoot, "ls-files", "--others", "--exclude-standard", ".ddx/beads.jsonl").Output()
		if err != nil {
			return fmt.Errorf("checking tracker untracked: %w", err)
		}
		if strings.TrimSpace(string(untracked)) == "" {
			return nil
		}
	}

	commitOut, err := internalgit.Command(context.Background(), projectRoot, "add", ".ddx/beads.jsonl").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staging tracker: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	// `git commit` would fail with "nothing to commit" if the file's
	// content is byte-identical to HEAD even though `git diff` saw a
	// diff (e.g. mode/whitespace race). Bail cleanly in that case.
	if cached, err := internalgit.Command(context.Background(), projectRoot, "diff", "--cached", "--", ".ddx/beads.jsonl").Output(); err == nil && strings.TrimSpace(string(cached)) == "" {
		return nil
	}
	commitOut, err = internalgit.Command(context.Background(), projectRoot, "commit", "--no-verify", "--only", "-m", msg, "--", ".ddx/beads.jsonl").CombinedOutput()
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

func prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev string, rcfg config.ResolvedConfig, runtime ExecuteBeadRuntime) (*executeBeadArtifacts, error) {
	b, refs, err := loadBeadContext(wtPath, beadID)
	if err != nil {
		return nil, err
	}
	artifacts, err := createArtifactBundle(projectRoot, wtPath, attemptID)
	if err != nil {
		return nil, err
	}

	promptContent, promptSource, err := buildPrompt(projectRoot, b, refs, artifacts, baseRev, runtime.PromptFile, rcfg.Harness(), rcfg.ContextBudget())
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(artifacts.PromptAbs, promptContent, 0o644); err != nil {
		return nil, fmt.Errorf("writing execute-bead prompt artifact: %w", err)
	}

	manifest := executeBeadManifest{
		AttemptID: attemptID,
		WorkerID:  runtime.WorkerID,
		BeadID:    beadID,
		BaseRev:   baseRev,
		CreatedAt: time.Now().UTC(),
		Requested: executeBeadRequested{
			Harness:  rcfg.Passthrough().Harness,
			Model:    rcfg.Passthrough().Model,
			Provider: rcfg.Passthrough().Provider,
			Effort:   rcfg.Effort(),
			Prompt:   promptSource,
			MinPower: rcfg.MinPower(),
			MaxPower: rcfg.MaxPower(),
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
		PromptSHA: promptSHA(promptContent),
	}
	if err := writeArtifactJSON(artifacts.ManifestAbs, manifest); err != nil {
		return nil, fmt.Errorf("writing execute-bead manifest artifact: %w", err)
	}
	artifacts.PromptSHA = manifest.PromptSHA
	return artifacts, nil
}

func loadBeadContext(wtPath, beadID string) (*bead.Bead, []executeBeadGoverningRef, error) {
	store := bead.NewStore(filepath.Join(wtPath, ".ddx"))
	b, err := store.Get(context.Background(), beadID)
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
	dirAbs := filepath.Join(wtPath, ExecuteBeadArtifactDir, attemptID)
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

// Load-bearing guardrails (FEAT-022 static-prompt minimum-prompt rule).
// Every static execute-bead prompt string below MUST preserve each item.
// TestExecuteBeadInstructionsLoadBearingGuardrails and
// TestPromptGuardrails_AllPresent enforce this list; add a guardrail here
// AND to both tests when you introduce one.
//
// 19 guardrails (FEAT-022 cross-reference):
//  1. AC checkbox: every AC satisfied by a specific code/test/file (anti-handwave)
//  2. Read named files / referenced specs first, before editing
//  3. Missing-governing fallback note (non-minimal renders only — see
//     executeBeadMissingGoverningText and contextBudget="minimal" branch)
//  4. Commit subject ends with [<bead-id>]
//  5. Commit exactly once, when green
//  6. git add <specific-paths> (prefer explicit staging over globs)
//  7. Never git add -A (worktree may have unrelated WIP)
//  8. Do not commit red code
//  9. Do not modify files outside bead scope
// 10. Never run `ddx init`
// 11. Keep .ddx/executions/ intact
// 12. Do not rewrite CLAUDE.md / AGENTS.md unless asked
// 13. Bead description overrides CLAUDE.md / YAGNI defaults
// 14. Reports go under {{.AttemptDir}}/, never /tmp, committed alongside code
// 15. Write no_changes_rationale.txt before exiting empty
// 16. Step 0 size-check + decomposition (ddx bead create / dep add / update)
// 17. Address every BLOCKING <review-findings> item; no no_changes with blocking findings open
// 18. Stop after the commit (Agent post-commit runaway guard)
// 19. Agent variant only: use tool calls, not `bash: cat`/`rg`/`ls`

// instrStep0SizeCheck is the shared Step 0 size-check + decomposition recipe.
// Both variants emit it verbatim; per-variant preamble runs before it.
const instrStep0SizeCheck = `

## Step 0: size check

Too big if any holds:

- More than ~6 ACs spanning unrelated subsystems.
- AC mixes design, implementation, integration tests, and docs as separate deliverables.
- Description names multiple feature-sized sub-pieces.
- More than ~500 lines across ~5+ files in unrelated packages.
- If the bead description exceeds 8000 bytes, treat Step 0 as a split-first pass.
- Auto-decomposition is capped at depth 2: root beads may split once, decomposed children once more; third-level splits must be rejected with an explanation.

If too big, decompose — do not attempt the work:

1. ` + "`ddx bead create`" + ` for each child slice (copy parent's labels and spec-id).
2. ` + "`ddx bead dep add <child-id> <parent-id>`" + ` to record edges.
3. ` + "`ddx bead update <parent-id> --notes 'decomposed into <child-ids>'`" + `.
4. Write ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` listing child IDs, then stop.

A clean decomposition is a success. Do not mix implementation with decomposition.`

// instrNoChangesContract is the shared NoChangesContract (TD-031 §8.1) rule.
const instrNoChangesContract = `

## no_changes contract

` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` must contain one:

- ` + "`verification_command: <cmd>`" + ` — repo cwd; exit 0 closes, nonzero rejects.
- ` + "`status: open`" + ` + ` + "`reason: <retryable>`" + ` — open, smart retry.
- ` + "`status: proposed`" + ` + ` + "`reason: <operator needed>`" + ` — operator lane.
- ` + "`status: blocked`" + ` + ` + "`reason: <external blocker>`" + ` — blocked lane.

No pseudo-statuses. Bare rationales and ` + "`status: needs_investigation`" + ` reject.

To request orchestrator decomposition, add ` + "`orchestrator_action: decompose`" + ` alongside ` + "`status: open`" + `.`

// instrInvestigationReports is the shared report-output rule.
const instrInvestigationReports = `

## Reports

Freestanding artifacts (investigation reports, findings docs) go under ` + "`{{.AttemptDir}}/`" + ` (the per-attempt evidence directory under ` + "`.ddx/executions/`" + `). **Never write reports to ` + "`/tmp`" + ` or any path outside the repository** — out-of-repo paths are invisible to the post-merge reviewer and cause BLOCK on missing evidence. If the bead names a specific in-repo path, use it; else default to ` + "`{{.AttemptDir}}/<short-name>.md`" + `. Stage and commit the report alongside code.`

// instrReviewGate is the shared review-as-gate rule.
const instrReviewGate = `

## Review gate

- The review is a gate, not an escape hatch — meet every AC in this pass.
- Address every BLOCKING ` + "`<review-findings>`" + ` item; do not declare ` + "`no_changes`" + ` with blocking findings open.`

// instrBeadOverride is the shared mode + bead-overrides-defaults rule.
const instrBeadOverride = `

## Mode and overrides

DDX_MODE=bead_execution: edit code/docs for bead AC. Only broad queue-steward default is overridden; tracker, merge-policy, verification, safety stay active. Bead description/AC override CLAUDE.md, AGENTS.md, and project defaults.`

// instrCoreConstraints is the shared core-constraints tail.
const instrCoreConstraints = `

## Constraints

- Work only inside this execution worktree.
- Keep ` + "`.ddx/executions/`" + ` intact — DDx uses it as execution evidence.
- **Never run ` + "`ddx init`" + `** — the workspace is initialized; running it corrupts the bead queue.
- Do not modify files outside the bead's named scope.
- Do not rewrite CLAUDE.md, AGENTS.md, or other project-instructions files unless the bead asks.`

// executeBeadInstructionsClaudeText is the <instructions> body used when the
// harness carries its own rich system prompt (claude, codex, opencode,
// unknown). It composes a Claude-specific preamble + process body with the
// shared instr* blocks.
const executeBeadInstructionsClaudeText = `You are executing one bead inside an isolated DDx execution worktree. The bead's <description> and <acceptance> are the completion contract — every AC must be provably satisfied by a specific code, test, or file you can point to after your commit.` +
	instrStep0SizeCheck +
	`

## How to work

- Read first. If the bead names files, specs, or prior beads, read them before editing — do not guess.
- Cross-reference each AC against concrete evidence (test name, file path, function) before committing. If you cannot point at it, it is not done.
- Run the project's test and lint commands before committing. **Do not commit red code** — fix failures first.
- Stage with ` + "`git add <specific-paths>`" + `; never ` + "`git add -A`" + ` (the worktree may have unrelated WIP).
- Commit exactly once when green; conventional-commit subject ending with ` + "`[<bead-id>]`" + `. Stop after the commit.
- Do not modify files outside the bead's scope.
- If you cannot finish, write ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` (what is done, what blocks, what a follow-up needs) before exiting. No commit and no rationale ⇒ DDx records ` + "`no_evidence_produced`" + `. A well-justified no_changes beats a bad commit.` +
	instrNoChangesContract +
	instrInvestigationReports +
	instrBeadOverride +
	instrReviewGate +
	instrCoreConstraints +
	`

## When the work is done

After the commit succeeds and every AC is verified, stop. Return control to the orchestrator — do not keep exploring or testing.`

// executeBeadInstructionsAgentText is the <instructions> body used when the
// harness selector routes to the embedded Fizeau agent (agent / fiz /
// embedded). It composes an agent-specific preamble + tool-aware process
// body with the shared instr* blocks. The Agent variant carries the
// stop-after-commit runaway guard (codex catch).
const executeBeadInstructionsAgentText = `You are a coding agent executing one bead inside an isolated DDx execution worktree. Tools: read, write, edit, bash, ls, grep, find. Use them — do not shell out to ` + "`bash: cat`" + `, ` + "`bash: rg`" + `, or ` + "`bash: ls`" + `.

The bead's <description> and <acceptance> are the completion contract. Every AC must be satisfied by code you write in this pass.` +
	instrStep0SizeCheck +
	`

## Process

- Read first. Read the files the bead names before editing — do not guess.
- Use ` + "`edit`" + ` for changes to existing files; ` + "`write`" + ` for new files; ` + "`read`" + ` / ` + "`grep`" + ` / ` + "`ls`" + ` for inspection (never the bash equivalents).
- Run the project's test and lint commands before committing. **Do not commit red code** — diagnose and fix.
- Stage with ` + "`git add <specific-paths>`" + `; never ` + "`git add -A`" + `.
- Commit exactly once when green; conventional-commit subject ending with ` + "`[<bead-id>]`" + `. Implementation and tests in the same commit.
- Stop immediately after the commit succeeds. Do not keep reading, testing, or following up — that risks runaway loops.
- Do not modify files outside the bead's scope.
- If you cannot finish, write ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` (done / blocking / follow-up) before exiting. No commit and no rationale ⇒ ` + "`no_evidence_produced`" + `.` +
	instrNoChangesContract +
	instrInvestigationReports +
	instrBeadOverride +
	instrReviewGate +
	instrCoreConstraints

// executeBeadInstructionsText selects the right instructions variant for the
// given harness. Harnesses with rich system prompts (claude, codex, opencode)
// get the terser claude variant; the embedded Fizeau harness gets the
// fuller agent variant with explicit tool names and stop-after-commit
// scaffolding.
func executeBeadInstructionsText(harness string) string {
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "agent", "fiz", "embedded":
		return executeBeadInstructionsAgentText
	default:
		return executeBeadInstructionsClaudeText
	}
}

// executeBeadMissingGoverningText is emitted inside <governing> when no
// governing references were pre-resolved for the bead. The bead description
// is the primary contract — this note only reminds the agent to treat it as
// such and to ground any unclear decisions in repository state rather than
// guessing.
const executeBeadMissingGoverningText = `No governing references were pre-resolved. The bead description above is the primary contract. If it names files, specs, or prior beads, read those before editing. Ground decisions in what is already in the repository; do not guess.`

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

func buildPrompt(workDir string, b *bead.Bead, refs []executeBeadGoverningRef, artifacts *executeBeadArtifacts, baseRev, promptOverride, harness string, contextBudget string) ([]byte, string, error) {
	if strings.TrimSpace(promptOverride) != "" {
		path := promptOverride
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		data, err := readPromptFileBounded(path)
		if err != nil {
			return nil, "", fmt.Errorf("reading prompt override %q: %w", promptOverride, err)
		}
		return data, promptOverride, nil
	}

	var sb strings.Builder
	sb.WriteString("<execute-bead>\n")

	instructions := strings.ReplaceAll(executeBeadInstructionsText(harness), "{{.AttemptDir}}", artifacts.DirRel)
	instructions += executeBeadDynamicStep0Hints(workDir, b)
	// Put the largely static instructions first so prefix caches can reuse as
	// much of the prompt as possible before the bead-specific XML starts.
	fmt.Fprintf(&sb, "  <instructions>\n%s\n  </instructions>\n", xmlEscape(instructions))

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

	// Bead notes carry review findings from prior iterations, escalation
	// context, or operator hints that did not fit into the description at
	// creation time. Threading them into the prompt as a distinct section
	// lets the agent act on them without the operator having to rewrite the
	// description in place on every reopen.
	if notes := strings.TrimSpace(b.Notes); notes != "" {
		fmt.Fprintf(&sb, "    <notes>\n%s\n    </notes>\n", xmlEscape(notes))
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

	// For minimal budget, omit full governing refs and only include bead metadata.
	// This significantly reduces prompt size for cheap-powerClass attempts on local models.
	if contextBudget == "minimal" {
		sb.WriteString("  <governing>\n    <note>No governing references.</note>\n  </governing>\n")
	} else {
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
	}

	sb.WriteString("</execute-bead>\n")

	return []byte(sb.String()), "synthesized", nil
}

const executeBeadLargeDescriptionHintThreshold = 8000
const executeBeadAutoDecompositionDepthCap = 2

func executeBeadDynamicStep0Hints(workDir string, b *bead.Bead) string {
	if b == nil {
		return ""
	}

	var sb strings.Builder
	desc := strings.TrimSpace(b.Description)
	if len(desc) > executeBeadLargeDescriptionHintThreshold {
		fmt.Fprintf(&sb, "\n\n## Large description hint\n")
		fmt.Fprintf(&sb, "This bead description is %d bytes (> %d). Treat Step 0 as a split-first pass and prefer child-bead scoping before implementation.\n",
			len(desc), executeBeadLargeDescriptionHintThreshold)
	}

	depth := beadDecompositionDepth(workDir, b)
	if depth >= executeBeadAutoDecompositionDepthCap {
		fmt.Fprintf(&sb, "\n\n## Decomposition depth cap\n")
		fmt.Fprintf(&sb, "This bead is already at decomposition depth %d. Do not create another child layer; if it is still too large, reject the split with a short explanation and write no_changes_rationale.txt instead.\n",
			depth)
	}

	return sb.String()
}

func beadDecompositionDepth(workDir string, b *bead.Bead) int {
	if b == nil {
		return 0
	}

	store := bead.NewStore(filepath.Join(workDir, ".ddx"))
	depth := beadDecomposedChildDepth(b)
	seen := map[string]struct{}{}
	current := b
	for current != nil {
		parentID := strings.TrimSpace(current.Parent)
		if parentID == "" {
			break
		}
		if _, ok := seen[parentID]; ok {
			break
		}
		seen[parentID] = struct{}{}
		parent, err := store.Get(context.Background(), parentID)
		if err != nil || parent == nil {
			break
		}
		if beadDecomposedChildDepth(parent) == 0 {
			break
		}
		depth++
		current = parent
	}
	return depth
}

func beadDecomposedChildDepth(b *bead.Bead) int {
	if b == nil || strings.TrimSpace(b.Parent) == "" {
		return 0
	}
	for _, label := range b.Labels {
		if strings.TrimSpace(label) == "decomposed" {
			return 1
		}
	}
	return 0
}

// promptSHA returns the hex-encoded sha256 of the rendered prompt bytes.
// Used as manifest.prompt_sha so analytics can group attempts by the exact
// prompt that produced them.
func promptSHA(promptContent []byte) string {
	sum := sha256.Sum256(promptContent)
	return hex.EncodeToString(sum[:])
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

// VerifyCleanWorktree checks that the project root's working tree has no
// untracked execution evidence files for the given attempt. If evidence files
// remain (e.g. because the land flow did not commit them), it stages and
// commits them as a safety net. Returns nil when the evidence dir is clean
// or was successfully committed.
func VerifyCleanWorktree(projectRoot, attemptID string) error {
	if attemptID == "" {
		return nil
	}
	evidenceDir := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	out, _ := internalgit.Command(context.Background(), projectRoot, "status", "--porcelain", "--", evidenceDir).Output()
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}

	// Exclude embedded session logs from the evidence commit; they stay
	// on disk for post-hoc inspection but must NOT be tracked — the
	// multi-thousand-line .jsonl files are what caused ddx-39e27896
	// (retry prompts ballooning past provider context limits).
	// manifest.json, result.json, prompt.md, usage.json remain tracked
	// per the existing audit-trail contract (gitignore un-ignores them).
	addArgs := append([]string{"add", "--", evidenceDir}, EvidenceLandExcludePathspecs()...)
	addOut, addErr := internalgit.Command(context.Background(), projectRoot, addArgs...).CombinedOutput()
	if addErr != nil {
		return fmt.Errorf("staging leftover evidence: %s: %w", strings.TrimSpace(string(addOut)), addErr)
	}
	diffOut, _ := internalgit.Command(context.Background(), projectRoot, "diff", "--cached", "--name-only", "--", evidenceDir).Output()
	if len(strings.TrimSpace(string(diffOut))) == 0 {
		return nil
	}
	msg := fmt.Sprintf("chore: add execution evidence [%s]", shortAttempt(attemptID))
	commitOut, commitErr := internalgit.Command(context.Background(), projectRoot,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"commit", "-m", msg,
	).CombinedOutput()
	if commitErr != nil {
		return fmt.Errorf("committing leftover evidence: %s: %w", strings.TrimSpace(string(commitOut)), commitErr)
	}
	return nil
}

// commitEvidenceBundleInWorktree stages the evidence directory in the attempt
// worktree and commits it so Land() can find the bundle inside the landing
// finalization worktree without copying from the project root. Returns the new
// HEAD SHA on success, or "" if staging finds nothing to commit (e.g. because
// the worktree is not a real git repo in tests or staging fails).
func commitEvidenceBundleInWorktree(wtPath, dirRel, attemptID string) string {
	if dirRel == "" {
		return ""
	}
	dirArg := filepath.FromSlash(dirRel)
	addArgs := append([]string{"add", "--", dirArg}, EvidenceLandExcludePathspecs()...)
	if _, err := internalgit.Command(context.Background(), wtPath, addArgs...).CombinedOutput(); err != nil {
		return ""
	}
	diffOut, _ := internalgit.Command(context.Background(), wtPath, "diff", "--cached", "--name-only", "--", dirArg).Output()
	if len(strings.TrimSpace(string(diffOut))) == 0 {
		return ""
	}
	msg := fmt.Sprintf("chore: add execution evidence [%s]", shortAttempt(attemptID))
	if _, err := internalgit.Command(context.Background(), wtPath,
		"-c", "user.name=ddx-land-coordinator",
		"-c", "user.email=coordinator@ddx.local",
		"commit", "--no-verify", "-m", msg,
	).CombinedOutput(); err != nil {
		return ""
	}
	headOut, headErr := internalgit.Command(context.Background(), wtPath, "rev-parse", "HEAD").Output()
	if headErr != nil {
		return ""
	}
	return strings.TrimSpace(string(headOut))
}

// publishEvidenceBundleToProjectRoot copies the evidence bundle from the
// isolated attempt worktree to the project root. Used by non-landing paths
// (no-changes, task-failed, operator-cancel) so that VerifyCleanWorktree can
// commit the bundle. A no-op when the source does not exist or when wtPath and
// projectRoot resolve to the same path.
func publishEvidenceBundleToProjectRoot(projectRoot, wtPath, dirRel string) {
	if dirRel == "" || sameFilesystemPath(projectRoot, wtPath) {
		return
	}
	src := filepath.Join(wtPath, filepath.FromSlash(dirRel))
	dst := filepath.Join(projectRoot, filepath.FromSlash(dirRel))
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return
	}
	_ = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func excludeCleanupMetadataFromWorktreeGit(wtPath string) error {
	if _, err := os.Stat(filepath.Join(wtPath, ".git")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	out, err := internalgit.Command(context.Background(), wtPath, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return err
	}
	excludePath := strings.TrimSpace(string(out))
	if excludePath == "" {
		return fmt.Errorf("git exclude path is empty")
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(wtPath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(excludePath)
	if err == nil && strings.Contains(string(existing), "/"+ExecutionCleanupMetadataFileName) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString("/" + ExecutionCleanupMetadataFileName + "\n")
	return err
}

func writeArtifactJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteExecuteBeadResultArtifact rewrites result.json in the project root with
// the final worker/orchestrator result after landing or preservation. The
// worker writes the initial task-level result inside the attempt worktree;
// callers use this helper after ApplyLandResultToExecuteBeadResult so the
// durable evidence bundle does not continue to report worker success when the
// orchestrator preserved the candidate.
func WriteExecuteBeadResultArtifact(projectRoot string, res *ExecuteBeadResult) error {
	if res == nil {
		return nil
	}
	resultFile := strings.TrimSpace(res.ResultFile)
	if resultFile == "" && strings.TrimSpace(res.ExecutionDir) != "" {
		resultFile = filepath.ToSlash(filepath.Join(res.ExecutionDir, "result.json"))
	}
	if resultFile == "" {
		return nil
	}
	path := filepath.FromSlash(resultFile)
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, path)
	}
	return writeArtifactJSON(path, res)
}

// ValidateACCheckFail returns an error when acOut has one or more failed AC
// items and commitMsg does not carry an AC-Waive: trailer. When acOut is nil
// or has no failures it returns nil unconditionally. The check is syntactic:
// any line whose trimmed prefix is "AC-Waive:" satisfies the waive condition.
func ValidateACCheckFail(acOut *accheck.Output, commitMsg string) error {
	if acOut == nil || acOut.Summary.Fail == 0 {
		return nil
	}
	if hasACWaiveTrailer(commitMsg) {
		return nil
	}
	return fmt.Errorf("ac-check: %d AC item(s) failed; add AC-Waive: trailer to bypass", acOut.Summary.Fail)
}

func hasACWaiveTrailer(msg string) bool {
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "AC-Waive:") {
			return true
		}
	}
	return false
}
