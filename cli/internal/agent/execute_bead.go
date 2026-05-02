package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/DocumentDrivenDX/fizeau"
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
	// Ratchet fields — populated by ApplyLandingToResult when declarative
	// ratchet thresholds were evaluated during landing. HELIX and other
	// consumers use these to distinguish ratchet-preserved attempts from
	// generic execution failures.
	RatchetEvidence []RatchetEvidence `json:"ratchet_evidence,omitempty"`
	RatchetSummary  string            `json:"ratchet_summary,omitempty"`

	// NoChangesRationale is populated when outcome == task_no_changes and the
	// agent wrote a rationale file to the execution bundle dir inside the
	// worktree. It carries the agent's explanation of why no commits were made.
	NoChangesRationale string `json:"no_changes_rationale,omitempty"`

	// NoEvidencePaths names worktree paths that remained dirty when the agent
	// exited without creating a commit or no_changes_rationale.txt. It helps
	// operators diagnose silent commit failures before the worktree is cleaned up.
	NoEvidencePaths []string `json:"no_evidence_paths,omitempty"`

	Harness    string  `json:"harness,omitempty"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	SessionID  string  `json:"session_id,omitempty"`
	DurationMS int     `json:"duration_ms"`
	Tokens     int     `json:"tokens,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	ExitCode   int     `json:"exit_code"`
	Error      string  `json:"error,omitempty"`

	// FailureMode classifies why an execution did not land cleanly. Empty
	// when the bead was merged (task_succeeded landing outcome). Populated
	// by the orchestrator from known patterns; see ClassifyFailureMode and
	// the FailureMode* constants in execute_bead_status.go.
	FailureMode string `json:"failure_mode,omitempty"`

	ExecutionDir string `json:"execution_dir,omitempty"`
	PromptFile   string `json:"prompt_file,omitempty"`
	ManifestFile string `json:"manifest_file,omitempty"`
	ResultFile   string `json:"result_file,omitempty"`
	UsageFile    string `json:"usage_file,omitempty"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// BeadEventAppender records append-only evidence events on a bead.
// Implemented by *bead.Store — kept as a minimal interface so the agent
// package does not need to import a concrete store type in tests.
type BeadEventAppender interface {
	AppendEvent(id string, event bead.BeadEvent) error
}

// ExecuteBeadRuntime carries the non-durable plumbing for an execute-bead
// run: per-invocation intent (FromRev, PromptFile, WorkerID) and
// non-serializable injection seams (BeadEvents, Service, AgentRunner).
// Durable knobs (Harness, Model, Provider, ModelRef, Effort,
// ContextBudget, MirrorCfg) live on config.ResolvedConfig and are passed
// via ExecuteBeadWithConfig's rcfg argument.
//
// See SD-024 / TD-024 §Runtime structs and §Stage 3.
type ExecuteBeadRuntime struct {
	FromRev     string // base git revision (default: HEAD)
	PromptFile  string // override prompt file (auto-generated if empty)
	WorkerID    string // from DDX_WORKER_ID env or caller
	BeadEvents  BeadEventAppender
	Service     agentlib.FizeauService
	AgentRunner AgentRunner
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
	// Passthrough constraints:
	Harness  string `json:"harness,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	ModelRef string `json:"model_ref,omitempty"`
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
	out, _ := internalgit.Command(context.Background(), dir, "status", "--porcelain").Output()
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
		line = strings.TrimSpace(line)
		if line == "" || len(line) < 4 {
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
		{
			pathspec:    ":(exclude).claude/skills",
			ignoreProbe: ".claude/skills",
		},
		{
			pathspec:    ":(exclude).agents/skills",
			ignoreProbe: ".agents/skills",
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
		Source:  "ddx agent execute-bead",
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
		Source:  "ddx agent execute-bead",
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
	attemptID := GenerateAttemptID()
	if runtime.WorkerID == "" {
		runtime.WorkerID = os.Getenv("DDX_WORKER_ID")
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

	// Checkpoint any remaining caller dirt as a real commit on the current
	// branch (FEAT-012 §22, US-126 AC#1). The new HEAD becomes the effective
	// base for the worker worktree; caller's edits are preserved as a normal
	// commit they can `git reset HEAD~` if they want them back uncommitted.
	if _, err := gitOps.SynthesizeCommit(projectRoot, "chore: checkpoint pre-execute-bead "+attemptID); err != nil {
		return nil, fmt.Errorf("pre-execute-bead checkpoint: %w", err)
	}

	// Resolve base revision after the tracker + checkpoint commits.
	baseRev, err := resolveBase(gitOps, projectRoot, runtime.FromRev)
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

	// Publish the live run-state so operators and HELIX can observe what is
	// executing without polling the bead tracker (CONTRACT-001 §5). The file
	// is removed on completion; a crashed worker leaves a stale file that
	// RecoverOrphans sweeps before the next attempt.
	_ = WriteRunState(projectRoot, RunState{
		BeadID:       beadID,
		AttemptID:    attemptID,
		Harness:      rcfg.Harness(),
		Model:        rcfg.Model(),
		StartedAt:    time.Now().UTC(),
		WorktreePath: wtPath,
	})
	defer func() {
		_ = ClearRunState(projectRoot)
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
		if abInfo, _ := os.Stat(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)); abInfo != nil && abInfo.IsDir() {
			res.ExecutionDir = filepath.Join(ExecuteBeadArtifactDir, attemptID)
		}
		res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
		populateWorkerStatus(res)
		_ = writeArtifactJSON(filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, "result.json"), res)
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

	runRuntime := AgentRunRuntime{
		PromptFile: artifacts.PromptAbs,
		WorkDir:    wtPath,
		Correlation: map[string]string{
			"bead_id":     beadID,
			"base_rev":    baseRev,
			"attempt_id":  attemptID,
			"session_id":  sessionID,
			"worker_id":   runtime.WorkerID,
			"bundle_path": artifacts.DirRel,
			"prompt_file": artifacts.PromptRel,
		},
		SessionLogDirOverride: embeddedStateDir,
		PermissionsOverride:   "unrestricted", // isolated worktree; writes must not require approval
	}

	agentResult, agentErr := dispatchAgentRun(ctx, projectRoot, runtime.Service, runtime.AgentRunner, rcfg, runRuntime)
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
		if agentResult.ActualPower > 0 {
			actualPower = agentResult.ActualPower
		}
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
		res := &ExecuteBeadResult{
			BeadID:       beadID,
			AttemptID:    attemptID,
			WorkerID:     runtime.WorkerID,
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
		res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
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
				WorkerID:     runtime.WorkerID,
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
		WorkerID:     runtime.WorkerID,
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

	// Classify worker outcome: task_succeeded / task_failed / task_no_changes /
	// task_no_evidence. A clean agent exit with no commit is only a legitimate
	// no_changes signal when the agent also wrote the explicit rationale file.
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

	// When the outcome is no_changes, attempt to read the agent's rationale file.
	// The agent is instructed to write this file (relative to the worktree) when it
	// determines the bead's work is already present. We read it before the deferred
	// worktree cleanup removes the file.
	if res.Outcome == ExecuteBeadOutcomeTaskNoChanges {
		rationaleFile := filepath.Join(wtPath, artifacts.DirRel, "no_changes_rationale.txt")
		if data, readErr := os.ReadFile(rationaleFile); readErr == nil {
			res.NoChangesRationale = strings.TrimSpace(string(data))
		}
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

	// Optional out-of-band mirror of the bundle. Wired here so the whole
	// per-attempt directory (manifest, prompt, result, usage, checks,
	// embedded/) is on disk before the upload starts. Failures never affect
	// the bead outcome — see executions_mirror.go.
	MirrorOrLog(MirrorRequest{
		ProjectRoot: projectRoot,
		AttemptID:   attemptID,
		BeadID:      beadID,
		BundleDir:   artifacts.DirAbs,
		Cfg:         rcfg.MirrorConfig(),
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

	out, err := internalgit.Command(context.Background(), projectRoot, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	out, err = internalgit.Command(context.Background(), projectRoot, "diff", "--", ".ddx/beads.jsonl").Output()
	if err != nil {
		return fmt.Errorf("checking tracker diff: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		out, err = internalgit.Command(context.Background(), projectRoot, "ls-files", "--others", "--exclude-standard", ".ddx/beads.jsonl").Output()
		if err != nil {
			return fmt.Errorf("checking tracker untracked: %w", err)
		}
		if strings.TrimSpace(string(out)) == "" {
			return nil
		}
	}

	msg := fmt.Sprintf("chore: update tracker (execute-bead %s)", time.Now().UTC().Format("20060102T150405"))
	commitOut, err := internalgit.Command(context.Background(), projectRoot, "add", ".ddx/beads.jsonl").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staging tracker: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	commitOut, err = internalgit.Command(context.Background(), projectRoot, "commit", "--no-verify", "-m", msg).CombinedOutput()
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
			ModelRef: rcfg.ModelRef(),
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

// Shared instruction-block constants. Each holds a section opener emitted
// verbatim by both execute-bead variants; per-variant prose continues
// inline after these constants. Extracting the common substrings keeps
// each guardrail (Step 0 size check, investigation/report path, review
// gate, bead-override clause, core constraints) in a single source
// location while preserving the exact wording each variant ships today.
const instrStep0SizeCheck = "\n\n## Step 0: size check (do this first)\n\nBefore writing any code, decide whether this bead "

const instrInvestigationReports = "\n\n## Investigation and report outputs\n\nIf the bead asks you to \"output a report\", \"document findings\", or otherwise produce a freestanding artifact that is not source code, write it under `{{.AttemptDir}}/` (the per-attempt evidence directory under `.ddx/executions/`). **Never write reports to `/tmp` or any path outside the repository** — paths outside the repo are invisible to the post-merge reviewer and do not survive between machines"

const instrReviewGate = "\n\n## Quality bar and the review step\n\nAfter your commit merges, an automated review step may check your work against the acceptance criteria. If any AC item is unmet, the bead reopens and escalates to a higher-capability model, and the review findings are threaded into the next attempt's prompt as a `<review-findings>` section. **The review is a gate, not an escape hatch — meet the AC in this pass"

const instrBeadOverride = "\n\n## The bead contract overrides project defaults\n\nThe bead description and AC override any CLAUDE.md, AGENTS.md, or project-level conservative defaults in this worktree. "

const instrCoreConstraints = "\n\n## Constraints\n\n- Work only inside this execution worktree.\n- Keep `.ddx/executions/` intact — DDx uses it as execution evidence.\n- **Never run `ddx init`** — "

// executeBeadInstructionsClaudeText is the per-bead task instructions emitted
// inside <instructions> when running against a harness that carries its own
// rich system prompt (claude, codex, opencode). These harnesses already know
// how to read files, run tools, and manage sessions — this text adds only the
// DDx-specific execution contract.
const executeBeadInstructionsClaudeText = `You are executing one bead inside an isolated DDx execution worktree. The bead's <description> and <acceptance> sections above are the completion contract — every AC checkbox must be provably satisfied by a specific piece of code, test, or file you can point to after your commit.` +
	instrStep0SizeCheck +
	`is sized for a single bounded-context attempt. Coarse signals that the bead is too big:

- More than ~6 acceptance criteria spanning unrelated subsystems.
- AC mixes design, implementation, integration tests, and docs as separate top-level deliverables rather than a single coherent change.
- Description explicitly lists multiple feature-sized pieces ("AC4 is a tier-inference engine", "AC5 introduces a new status type").
- You estimate more than ~500 lines of meaningful code change across more than ~5 files in unrelated packages.

If the bead is too big, do NOT attempt the work. Instead, in this single attempt:

1. Use the ` + "`bash`" + ` tool to run ` + "`ddx bead create`" + ` to file child beads, one per coherent slice of work. Pass each child a focused title, description, and acceptance criteria; copy labels and ` + "`spec-id`" + ` from the parent.
2. Use ` + "`ddx bead dep add <child-id> <parent-id>`" + ` to record the dependency edge.
3. Append a short note to the parent (` + "`ddx bead update <parent-id> --notes 'decomposed into <child-ids>'`" + `) so future runs see why no implementation landed.
4. Write ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` explaining the decomposition: list each child bead ID and the slice it covers, and stop.

A clean decomposition that produces tractable children is a successful attempt. Do not push partial implementation alongside decomposition.

If the bead IS sized for one attempt, continue with the rest of these instructions.

## How to work

1. Read first. If the bead description names files, read them to see what is already there. If the bead references a spec, ADR, or API contract, read the relevant sections before writing code. Do not start editing until you understand what the bead wants changed and why.

2. Cross-reference your work against the acceptance criteria as you go. Before committing, walk through the AC one item at a time and identify the specific test name, file path, or function that satisfies each checkbox. If you cannot point to concrete evidence for an AC item, it is not done.

3. Run the project's verification commands before committing. Use whatever commands match the crate or package you touched — typically ` + "`cargo test`" + ` and ` + "`cargo clippy`" + ` for Rust, ` + "`bun test`" + ` or ` + "`npm test`" + ` for JS/TS, ` + "`go test`" + ` for Go. The bead description usually names the exact commands. **Do not commit red code.** If a test or lint fails, fix it first.

4. Commit once, when everything is green. Stage only the files you intentionally changed with ` + "`git add <specific-paths>`" + ` — never ` + "`git add -A`" + `, because there may be unrelated WIP in this worktree. Use a conventional-commit-style subject ending with ` + "`[<bead-id>]`" + `. DDx will merge your commits back to the base branch.

5. If you cannot complete the bead in this pass, writing ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` is mandatory before you exit. Include: (a) what is done, (b) what is blocking, (c) what a follow-up attempt would need. If you exit without a commit and without this file, DDx records ` + "`no_evidence_produced`" + ` and treats the attempt as a harness failure, not as legitimate no_changes. Do NOT commit partial, exploratory, or red code hoping the reviewer will accept it — a well-justified no_changes is better than a bad commit. The bead will be re-queued for another attempt, potentially with a stronger model.` +
	instrInvestigationReports +
	`, which causes the reviewer to BLOCK on missing evidence. If the bead names a specific in-repo path for the report, use that path; otherwise default to ` + "`{{.AttemptDir}}/<short-name>.md`" + `. Stage and commit the report alongside any code changes so it lands in the merge.` +
	instrBeadOverride +
	`If the bead asks for new documentation, write it. If the bead adds a new module or crate, add it. Conservative rules (YAGNI, DOWITYTD, no-docs-unless-asked) do not apply inside execute-bead — the bead IS the ask.` +
	instrReviewGate +
	` so the bead closes cleanly.**

If this prompt already contains a ` + "`<review-findings>`" + ` section, address every BLOCKING finding before claiming the work complete. Those findings are the precise list of what the previous attempt missed.` +
	instrCoreConstraints +
	`the workspace is already initialized. Running it corrupts the bead queue and project configuration.
- Do not modify files outside the scope the bead description names.
- Do not rewrite CLAUDE.md, AGENTS.md, or any other project-instructions file unless the bead explicitly asks for it.

## When the work is done

After the commit succeeds and you have verified every AC item, stop. Return control to the orchestrator. Do not continue to explore the repository, run extra tests, or generate follow-up notes — the bead is complete, and returning promptly is how execute-bead signals success.`

// executeBeadInstructionsAgentText is the per-bead task instructions emitted
// inside <instructions> when running against the embedded Fizeau harness,
// which has a minimal system prompt and needs more scaffolding to produce
// reliable output. This version is more explicit about tools, process, and
// stopping cleanly after the commit (to avoid the known post-commit runaway
// failure mode).
const executeBeadInstructionsAgentText = `You are a coding agent executing one bead inside an isolated DDx execution worktree. You have tools: read, write, edit, bash, ls, grep, find. Use them directly — do not shell out to cat/tail/rg/find; use the tools.

The bead's <description> and <acceptance> sections above are the completion contract. Every AC checkbox must be satisfied by code you write in this pass.` +
	instrStep0SizeCheck +
	`fits in one bounded-context attempt. Coarse signals it does not:

- More than ~6 acceptance criteria spanning unrelated subsystems.
- AC mixes design, implementation, integration tests, and docs as separate top-level deliverables.
- Description explicitly names multiple feature-sized sub-pieces.
- You estimate more than ~500 lines across more than ~5 files in unrelated packages.

If too big, do NOT attempt the work. Instead, in this single attempt:

1. Run ` + "`ddx bead create`" + ` (via bash) to file child beads, one per coherent slice — focused title, description, AC; copy parent's labels and spec-id.
2. Run ` + "`ddx bead dep add <child-id> <parent-id>`" + ` to wire the dependency edge.
3. Run ` + "`ddx bead update <parent-id> --notes 'decomposed into <child-ids>'`" + ` so the decomposition is visible.
4. Write ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` listing each child bead ID and the slice it covers, then stop.

A clean decomposition is a successful attempt. Do not mix partial implementation with decomposition.

If sized for one attempt, continue.

## Process

1. **Read first.** Before writing any code, read the files the bead description names. If the description says "A1 landed X at <path>", read that path. If it references a spec section, read that section. Do NOT start editing without seeing the existing code — this is how you avoid making the same change twice or breaking an invariant you did not know about.

2. **Plan the work briefly.** Internally note which files you will touch, which tests you will add, and which verification commands you will run. Keep the plan short — one or two sentences.

3. **Implement.** Use ` + "`edit`" + ` for targeted changes to existing files and ` + "`write`" + ` for brand-new files. Use ` + "`read`" + ` (not ` + "`bash: cat`" + `) to read files. Use ` + "`grep`" + ` (not ` + "`bash: rg`" + `) to search. Use ` + "`ls`" + ` (not ` + "`bash: ls`" + `) for directory listings.

4. **Verify before committing.** Run the project's test and lint commands — typically ` + "`cargo test -p <crate>`" + ` and ` + "`cargo clippy -p <crate> -- -D warnings`" + ` for Rust, ` + "`bun test`" + ` for JS/TS, ` + "`go test`" + ` for Go. The bead description will usually name the exact commands. **Do not commit red code.** If anything fails, diagnose and fix it before committing.

5. **Commit exactly once.** Use ` + "`git add <specific-paths>`" + ` with the file paths you actually changed — never ` + "`git add -A`" + `, which can pick up unrelated files in the worktree. Use a conventional-commit subject ending with ` + "`[<bead-id>]`" + `. Commit implementation and tests in the same commit so the reviewer sees them together.

6. **Stop after the commit succeeds.** Return immediately. Do not continue reading files, running extra tests, or asking yourself follow-up questions — the work is done, and returning promptly is how execute-bead signals success to the orchestrator. Continuing past the commit risks runaway loops.

## If you cannot finish

Writing ` + "`{{.AttemptDir}}/no_changes_rationale.txt`" + ` is mandatory before you exit without a commit. Include: (a) what is done, (b) what is blocking, (c) what a follow-up would need. If you exit without a commit and without this file, DDx records ` + "`no_evidence_produced`" + ` and treats the attempt as a harness failure, not as legitimate no_changes. Do NOT commit partial or red code — a well-justified no_changes is better than a bad commit. The bead will be re-queued for another attempt.` +
	instrInvestigationReports +
	`. If the bead names a specific in-repo path, use that; otherwise default to ` + "`{{.AttemptDir}}/<short-name>.md`" + `. Stage and commit the report alongside any code changes.` +
	instrReviewGate +
	`.**

If this prompt already contains a ` + "`<review-findings>`" + ` section, every BLOCKING finding in it is a concrete thing the previous attempt missed. Address each one before declaring the work complete — do not declare no_changes with blocking findings still unaddressed.` +
	instrBeadOverride +
	`If the bead asks for new files, write them. Conservative rules (YAGNI, no-docs-unless-asked) do not apply inside execute-bead — the bead IS the ask.` +
	instrCoreConstraints +
	`it corrupts the bead queue.
- Do not touch CLAUDE.md, AGENTS.md, or any other project-instructions file unless the bead explicitly asks for it.
- Stage only the files you intentionally changed.`

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
	// This significantly reduces prompt size for cheap-tier attempts on local models.
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

	instructions := strings.ReplaceAll(executeBeadInstructionsText(harness), "{{.AttemptDir}}", artifacts.DirRel)
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

func writeArtifactJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
