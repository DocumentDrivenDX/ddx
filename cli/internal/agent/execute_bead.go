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
	"strconv"
	"strings"
	"time"

	"errors"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/DocumentDrivenDX/ddx/internal/lockmetrics"
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
	// TargetBranch is the resolved branch that received the landing. It uses the
	// landed_branch JSON name because callers care about the post-land location.
	TargetBranch string `json:"landed_branch,omitempty"`
	// EvidenceRev is retained for decoding legacy results that referenced a
	// trailing evidence commit. Current attempts never populate it.
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
	CycleIndex int `json:"cycle_index,omitempty"`
	// ReviewVerdict is the pre-land candidate-cycle reviewer verdict (APPROVE,
	// REQUEST_CHANGES, BLOCK) when a reviewer ran. Empty when review was
	// skipped or never completed. Mirrors ExecuteBeadReport.ReviewVerdict.
	ReviewVerdict string `json:"review_verdict,omitempty"`
	// ReviewRationale carries the actionable reviewer-authored findings for
	// non-APPROVE review outcomes. Mirrors ExecuteBeadReport.ReviewRationale.
	ReviewRationale string `json:"review_rationale,omitempty"`
	// ReviewSkipReason carries the durable review:skip-reason:* label when a
	// success path is allowed to close without running a reviewer. Mirrors
	// ExecuteBeadReport.ReviewSkipReason.
	ReviewSkipReason    string            `json:"review_skip_reason,omitempty"`
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
	ProjectRoot                 string  `json:"project_root,omitempty"`

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

// ExecutionCycleRequestedRouteFacts captures the operator/request-side routing
// facts available to the worker for one execution cycle.
type ExecutionCycleRequestedRouteFacts struct {
	Harness                 string `json:"harness,omitempty"`
	Provider                string `json:"provider,omitempty"`
	Model                   string `json:"model,omitempty"`
	Profile                 string `json:"profile,omitempty"`
	RoutingIntentSource     string `json:"routing_intent_source,omitempty"`
	EstimatedDifficulty     string `json:"estimated_difficulty,omitempty"`
	InferredPowerClass      string `json:"inferred_power_class,omitempty"`
	RequestedPowerClass     string `json:"requested_power_class,omitempty"`
	RequestedPolicy         string `json:"requested_policy,omitempty"`
	InferredMinPower        int    `json:"inferred_min_power"`
	InferredMinPowerPresent bool   `json:"inferred_min_power_present,omitempty"`
	RequestedMinPower       int    `json:"requested_min_power"`
	RequestedMaxPower       int    `json:"requested_max_power"`
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
	CycleIndex           int                               `json:"cycle_index"`
	AttemptID            string                            `json:"attempt_id,omitempty"`
	ResultRev            string                            `json:"result_rev,omitempty"`
	ImplementerRoute     ExecutionCycleRouteFacts          `json:"implementer_route"`
	RequestedRoute       ExecutionCycleRequestedRouteFacts `json:"requested_route"`
	ActualRoute          ExecutionCycleRouteFacts          `json:"actual_route"`
	ReviewGroupID        string                            `json:"review_group_id,omitempty"`
	ReviewerIndices      []int                             `json:"reviewer_indices,omitempty"`
	ReviewVerdicts       []string                          `json:"review_verdicts,omitempty"`
	ReviewerRoute        ExecutionCycleRouteFacts          `json:"reviewer_route,omitempty"`
	ReviewResult         ExecutionCycleReviewResult        `json:"review_result,omitempty"`
	FinalDecision        string                            `json:"final_decision,omitempty"`
	FailureClass         string                            `json:"failure_class"`
	RetryAction          string                            `json:"retry_action"`
	EscalationCount      int                               `json:"escalation_count"`
	ReviewStatus         string                            `json:"review_status"`
	ReviewSkipReason     string                            `json:"review_skip_reason"`
	ReviewClassification string                            `json:"review_classification"`
	LandStatus           string                            `json:"land_status"`
	ReconcileStatus      string                            `json:"reconcile_status"`
	DecomposedChildIDs   []string                          `json:"decomposed_child_ids,omitempty"`
	ExecutionDecision    string                            `json:"execution_decision,omitempty"`
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
// Kept as a minimal interface so the agent package can consume bead event
// backends without depending on the concrete tracker type.
type BeadEventAppender interface {
	AppendEvent(id string, event bead.BeadEvent) error
}

func appendWorkEvent(store BeadEventAppender, beadID, kind, summary string, body map[string]any, actor string, at time.Time) {
	if store == nil || beadID == "" {
		return
	}
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      kind,
		Summary:   summary,
		Body:      string(encodedBody),
		Actor:     actor,
		Source:    "ddx work",
		CreatedAt: at,
	})
}

// BeadCancelStore reads/writes the cancel markers on a bead's Extra map.
// ADR-022 §Cancel SLA: the worker polls IsCancelRequested mid-attempt and
// on a positive read writes MarkCancelHonored before aborting at the next
// safe point.
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
	FromRev                 string // base git revision (default: HEAD)
	PromptFile              string // override prompt file (auto-generated if empty)
	Output                  io.Writer
	WorkerID                string // from DDX_WORKER_ID env or caller
	BeadStoreRoot           string // canonical bead store for linked/external tracker roots
	BeadEvents              BeadEventAppender
	BeadCancel              BeadCancelStore // optional: enables operator-cancel mid-attempt poll
	ResourceChecker         ExecutionResourceChecker
	Service                 agentlib.FizeauService
	AgentRunner             AgentRunner
	Checks                  CandidateCheckRunner
	Reviewer                CandidateReviewer
	Repair                  RepairPass
	RepairMaxCycles         int
	CandidateRefStore       CandidateRefStore
	NoReview                bool
	AttemptBackend          AttemptBackend
	candidateImport         func(candidate CandidateResult) error
	candidateImportRelease  func(candidate CandidateResult) error
	candidateOriginalTask   string
	candidateDiff           func(candidate CandidateResult) (string, error)
	candidateRepairCaps     evidence.Caps
	candidateCapsConfigured bool
	// EvidenceFileCopier is an internal test seam for controlled local-evidence
	// publication. Production leaves it nil and uses the filesystem copier.
	EvidenceFileCopier func(source, target string, mode os.FileMode) error
	// RateLimitMaxWait bounds the per-bead total wait spent on rate-limit
	// retries (ddx-c6e3db02 RateLimitRetryContract / TD-031 §8.4). Zero uses
	// RateLimitRetryDefaultBudget (5 min). Negative disables the wrapper —
	// rate-limit responses fall through to the standard execution_failed path.
	RateLimitMaxWait time.Duration
	// PreserveAttemptWorktree keeps the attempt worktree on disk after the
	// attempt finishes. It is used by inspection-oriented runs such as
	// `ddx try --no-merge`.
	PreserveAttemptWorktree bool
	// ACCheckRunner, when non-nil, runs ddx bead ac-check for the given bead
	// and attempt after the agent commits, writing ac-check.json to the
	// attempt dir under wtPath. When nil, ac-check is skipped.
	ACCheckRunner func(ctx context.Context, beadID, attemptID, wtPath string) (*accheck.Output, error)
}

type onRouteResolvedKeyType struct{}
type onExecuteStartKeyType struct{}

// contextWithOnRouteResolved stores fn in ctx so execute-bead can retrieve it
// after the Executor dispatches through agenttry.Attempt.
func contextWithOnRouteResolved(ctx context.Context, fn func(harness, provider, model string)) context.Context {
	return context.WithValue(ctx, onRouteResolvedKeyType{}, fn)
}

// onRouteResolvedFromContext retrieves the callback stored by
// contextWithOnRouteResolved, or nil if none was stored.
func onRouteResolvedFromContext(ctx context.Context) func(harness, provider, model string) {
	fn, _ := ctx.Value(onRouteResolvedKeyType{}).(func(harness, provider, model string))
	return fn
}

// contextWithOnExecuteStart stores the Fizeau Execute boundary callback so
// execute-bead can forward it through AgentRunRuntime after local preparation.
func contextWithOnExecuteStart(ctx context.Context, fn func()) context.Context {
	return context.WithValue(ctx, onExecuteStartKeyType{}, fn)
}

func onExecuteStartFromContext(ctx context.Context) func() {
	fn, _ := ctx.Value(onExecuteStartKeyType{}).(func())
	return fn
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

type staticCandidateResultPass struct {
	candidate CandidateResult
}

func (p staticCandidateResultPass) Execute(context.Context, string) (CandidateResult, error) {
	return p.candidate, nil
}

func applyWorkerCandidateCycle(ctx context.Context, projectRoot, wtPath string, runtime ExecuteBeadRuntime, res *ExecuteBeadResult) error {
	if res == nil || wtPath == "" || res.Status != ExecuteBeadStatusSuccess {
		return nil
	}

	_ = MarkWorktreeActiveCycle(wtPath)
	defer func() {
		_ = ClearWorktreeActiveCycle(wtPath)
	}()

	refStore := runtime.CandidateRefStore
	if refStore == nil {
		insideGitWorktree, err := gitWorktreeStatus(projectRoot)
		if err != nil {
			return fmt.Errorf("checking project root before candidate pinning: %w", err)
		}
		if insideGitWorktree {
			refStore = &GitCandidateRefStore{}
		}
	}
	coord := &AttemptCycleCoordinator{
		Pass: staticCandidateResultPass{
			candidate: CandidateResult{
				Report:       ReportFromExecuteBeadResult(res, ""),
				WorktreePath: wtPath,
				CycleIndex:   res.CycleIndex,
			},
		},
		RefStore:             refStore,
		ProjectRoot:          projectRoot,
		BeadEvents:           runtime.BeadEvents,
		Checks:               runtime.Checks,
		Reviewer:             runtime.Reviewer,
		Repair:               runtime.Repair,
		RepairMaxCycles:      runtime.RepairMaxCycles,
		NoReview:             runtime.NoReview,
		OriginalTask:         runtime.candidateOriginalTask,
		CandidateDiff:        runtime.candidateDiff,
		RepairCaps:           runtime.candidateRepairCaps,
		RepairCapsConfigured: runtime.candidateCapsConfigured,
		ValidateCandidate: func(candidate CandidateResult) error {
			return VerifyCandidateHasNoExecutionEvidence(candidate.WorktreePath, candidate.Report.BaseRev, candidate.Report.ResultRev)
		},
		ImportCandidate:        runtime.candidateImport,
		ReleaseCandidateImport: runtime.candidateImportRelease,
	}
	cycleResult, err := coord.Run(ctx, res.BeadID)
	projectCandidateCycleReport(res, cycleResult.Report)
	return err
}

func projectCandidateCycleReport(res *ExecuteBeadResult, report ExecuteBeadReport) {
	if res == nil {
		return
	}
	// Project every outcome-bearing field from the candidate-cycle report back
	// onto the execute result. A review malfunction, request-changes, block, or
	// exhausted repair cycle must overwrite the worker's provisional "success"
	// status so the loop's landing/close decision (and the durable decision
	// audit) reflect the cycle's real disposition instead of a stale success
	// (ddx-0c7d976c).
	res.ResultRev = report.ResultRev
	if report.ImplementationRev != "" {
		res.ImplementationRev = report.ImplementationRev
	}
	res.Status = report.Status
	res.Detail = report.Detail
	if report.Error != "" {
		res.Error = report.Error
	}
	if report.OutcomeReason != "" {
		res.FailureMode = report.OutcomeReason
	}
	res.ReviewVerdict = report.ReviewVerdict
	res.ReviewRationale = report.ReviewRationale
	res.ReviewSkipReason = report.ReviewSkipReason
	res.CandidateRef = report.CandidateRef
	res.CycleIndex = report.CycleIndex
	res.CycleTrace = append([]ExecutionCycleTrace(nil), report.CycleTrace...)
	res.CostUSD = report.CostUSD
	res.DurationMS = int(report.DurationMS)
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

	// ExecuteBeadTmpSubdir is the legacy $TMPDIR subdirectory retained for
	// cleanup and detection of older isolated worktrees.
	ExecuteBeadTmpSubdir = config.DefaultExecutionTempSubdir
)

// executeBeadWorktreePath returns the absolute path where an execute-bead
// isolated worktree for (beadID, attemptID) should live.
func executeBeadWorktreePath(projectRoot, beadID, attemptID string) string {
	return filepath.Join(config.ExecutionTempRoot(projectRoot), ExecuteBeadWtPrefix+beadID+"-"+attemptID)
}

const mixedCommitAndNoChangesRationaleReason = "mixed_commit_and_no_changes_rationale"

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
	RouteReason  string  `json:"route_reason,omitempty"`
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

// ExecuteBeadWithConfig is the thin worker: it creates an isolated attempt workspace,
// constructs the agent prompt from bead context, runs the agent harness,
// synthesizes a commit if the agent left uncommitted changes, then cleans up
// the workspace and returns the result. It classifies outcomes as exactly one
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
// Agent dispatch: tests may set runtime.AgentRunner to inject a fake that
// returns canned Result values; when set, it takes precedence over the normal
// dispatch path. Production execute-bead always dispatches through Fizeau,
// including when the operator supplies opaque harness/provider/model pins.
func ExecuteBeadWithConfig(ctx context.Context, projectRoot string, beadID string, rcfg config.ResolvedConfig, runtime ExecuteBeadRuntime, gitOps GitOps) (result *ExecuteBeadResult, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = rcfg.Harness()
	if runtime.WorkerID == "" {
		runtime.WorkerID = os.Getenv("DDX_WORKER_ID")
	}
	attemptBackend := runtime.AttemptBackend
	if attemptBackend == nil {
		var backendErr error
		attemptBackend, backendErr = ResolveAttemptBackend(rcfg)
		if backendErr != nil {
			return nil, backendErr
		}
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
				ProjectRoot:       projectRoot,
				ResourceExhausted: &resourceErr.Result,
			}
			res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
			return res, nil
		}
		return nil, err
	}

	attemptID := GenerateAttemptID()
	if projectRoot != "" {
		evidenceDir := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID)
		lockmetrics.SetCapEnforcement(projectRoot, evidenceDir)
		defer lockmetrics.SetCapEnforcement(projectRoot, "")
	}
	gitRepair := defaultPreDispatchGitRepairer(ctx, projectRoot)
	if detail, failed := preDispatchGitRepairFailure(gitRepair); failed {
		res := &ExecuteBeadResult{
			BeadID:      beadID,
			WorkerID:    runtime.WorkerID,
			AttemptID:   attemptID,
			ExitCode:    1,
			Error:       preDispatchGitRepairFailedMarker + detail,
			Reason:      preDispatchGitRepairFailedReason,
			Outcome:     ExecuteBeadOutcomeTaskFailed,
			Status:      ExecuteBeadStatusExecutionFailed,
			ProjectRoot: projectRoot,
		}
		res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
		return res, nil
	}
	if runtime.Output != nil && len(gitRepair.RepairedTypes) > 0 {
		_, _ = fmt.Fprintf(runtime.Output,
			"repaired project git config before execute-bead: %s (%s)\n",
			strings.Join(gitRepair.RepairedTypes, ", "),
			strings.Join(gitRepair.Commands, "; "),
		)
	}

	// Serialize only the parent-repo writes in pre-dispatch (tracker commit
	// plus the caller-dirt checkpoint) under the main-git lock. Base
	// resolution and isolated worktree creation are read-only / per-worktree
	// operations and can proceed after the lock is released.
	var baseRev string
	var workspace *AttemptWorkspace
	if err := withTrackerLock(projectRoot, "pre_dispatch_commits", func() error {
		// Commit beads.jsonl before spawning the attempt workspace so its snapshot
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
		return nil
	}); err != nil {
		// A disk/resource-exhaustion failure during the pre-dispatch sequence
		// (most commonly `git worktree add` running out of space while checking
		// out the isolated worktree) must surface as a resource_exhausted
		// outcome, not a raw error. The execute-loop releases the claim for a
		// resource_exhausted report (parity with the pre-execution resource
		// check above); a raw error leaves the bead claimed-but-open and
		// execution-ineligible until a manual --unclaim (ddx-f677a50b).
		if classifyReadinessSystemReason(err.Error(), nil) == ReadinessSystemReasonResourceExhausted {
			res := &ExecuteBeadResult{
				BeadID:      beadID,
				WorkerID:    runtime.WorkerID,
				ExitCode:    1,
				Error:       err.Error(),
				Reason:      err.Error(),
				Outcome:     ExecuteBeadOutcomeTaskFailed,
				Status:      ExecuteBeadStatusResourceExhausted,
				ProjectRoot: projectRoot,
			}
			res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
			return res, nil
		}
		return nil, err
	}
	// Re-resolve after the tracker/checkpoint commits so the worker's base
	// sees the committed parent-repo state without holding the lock across
	// the read.
	rev, err := resolveBase(gitOps, projectRoot, runtime.FromRev)
	if err != nil {
		return nil, err
	}
	baseRev = rev
	ws, err := attemptBackend.Prepare(ctx, AttemptBackendPrepareRequest{
		ProjectRoot: projectRoot,
		BeadID:      beadID,
		AttemptID:   attemptID,
		BaseRev:     baseRev,
		GitOps:      gitOps,
	})
	if err != nil {
		// A disk/resource-exhaustion failure during the pre-dispatch sequence
		// (most commonly `git worktree add` running out of space while checking
		// out the isolated worktree) must surface as a resource_exhausted
		// outcome, not a raw error. The execute-loop releases the claim for a
		// resource_exhausted report (parity with the pre-execution resource
		// check above); a raw error leaves the bead claimed-but-open and
		// execution-ineligible until a manual --unclaim (ddx-f677a50b).
		if classifyReadinessSystemReason(err.Error(), nil) == ReadinessSystemReasonResourceExhausted {
			res := &ExecuteBeadResult{
				BeadID:      beadID,
				WorkerID:    runtime.WorkerID,
				ExitCode:    1,
				Error:       err.Error(),
				Reason:      err.Error(),
				Outcome:     ExecuteBeadOutcomeTaskFailed,
				Status:      ExecuteBeadStatusResourceExhausted,
				ProjectRoot: projectRoot,
			}
			res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)
			return res, nil
		}
		return nil, err
	}
	workspace = ws
	if workspace == nil || strings.TrimSpace(workspace.WorkDir) == "" {
		return nil, fmt.Errorf("attempt backend %s did not return a workspace", attemptBackend.Name())
	}
	wtPath := workspace.WorkDir
	if err := excludeCleanupMetadataFromWorktreeGit(wtPath); err != nil {
		_ = attemptBackend.Cleanup(ctx, workspace)
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
		_ = attemptBackend.Cleanup(ctx, workspace)
		return nil, fmt.Errorf("writing execute-bead cleanup metadata: %w", err)
	}
	var res *ExecuteBeadResult
	preserveEvidenceSource := false
	defer func() {
		if preserveEvidenceSource {
			return
		}
		preserveAttemptWorktree := runtime.PreserveAttemptWorktree && attemptBackend.Name() == AttemptBackendWorktree
		if preserveAttemptWorktree {
			return
		}
		if result != nil && attemptBackend.Name() == AttemptBackendWorktree {
			if cleanupAttemptWorktree(gitOps, projectRoot, wtPath, result.Outcome, false) {
				return
			}
		}
		_ = attemptBackend.Cleanup(ctx, workspace)
	}()

	evidenceDir := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))
	// Install the shared repository-local exclusion before prompts, bundles, or
	// harness commands can create and stage execution evidence in this worktree.
	// This closes script/custom harness paths that run their own broad git add.
	if err := withMainGitLock(projectRoot, "evidence_local_exclude", func() error {
		// The project root must be protected before an independent clone backend
		// publishes into it; the attempt repo needs its own rule so its harness
		// cannot stage the bundle. Linked worktrees resolve both calls to the
		// common repository exclude and the second call is an idempotent no-op.
		if err := ensureExecutionEvidenceLocalExcludeForWorkspace(projectRoot); err != nil {
			return err
		}
		if sameFilesystemPath(projectRoot, wtPath) {
			return nil
		}
		return ensureExecutionEvidenceLocalExcludeForWorkspace(wtPath)
	}); err != nil {
		return nil, fmt.Errorf("installing pre-harness execution evidence exclude: %w", err)
	}
	defer func() {
		// Runs BEFORE WorktreeRemove (registered later = first in LIFO order).
		// Any publication/verification failure keeps the source worktree intact
		// and joins the returned error, including exits that otherwise return a
		// nil result after bundle creation.
		if err := finalizeLocalExecutionEvidenceWithCopier(projectRoot, wtPath, evidenceDir, attemptID, runtime.EvidenceFileCopier); err != nil {
			preserveEvidenceSource = true
			if result != nil {
				if result.Error == "" {
					result.Error = err.Error()
				} else {
					result.Error = result.Error + "; " + err.Error()
				}
				result.Reason = "local execution evidence publication failed"
				result.Outcome = ExecuteBeadOutcomeTaskFailed
				result.Status = ExecuteBeadStatusExecutionFailed
				if result.ExitCode == 0 {
					result.ExitCode = 1
				}
				result.FailureMode = ClassifyFailureMode(result.Outcome, result.ExitCode, result.Error)
				if artifactsErr := writeArtifactJSON(filepath.Join(wtPath, filepath.FromSlash(evidenceDir), "result.json"), result); artifactsErr != nil {
					retErr = errors.Join(retErr, fmt.Errorf("recording local evidence publication failure: %w", artifactsErr))
				}
			}
			retErr = errors.Join(retErr, err)
		}
	}()
	publishAttemptResult := func(res *ExecuteBeadResult) error {
		if res == nil || strings.TrimSpace(res.ResultRev) == "" || res.ResultRev == res.BaseRev {
			return nil
		}
		return attemptBackend.PublishResult(ctx, workspace, res)
	}

	// Publish the live run-state so operators and HELIX can observe what is
	// executing without polling the bead tracker (CONTRACT-001 §5). The file
	// is removed on completion; a crashed worker still leaves a stale file that
	// RecoverOrphans can sweep later as a backstop.
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
	artifacts, beadCtx, err := prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev, rcfg, runtime)
	if err != nil {
		res = &ExecuteBeadResult{
			BeadID:       beadID,
			AttemptID:    attemptID,
			WorkerID:     runtime.WorkerID,
			BaseRev:      baseRev,
			ResultRev:    baseRev, // no commits; ResultRev == BaseRev signals no output
			ExitCode:     1,
			Error:        err.Error(),
			Outcome:      ExecuteBeadOutcomeTaskFailed,
			ProjectRoot:  projectRoot,
			WorktreePath: wtPath,
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
	gitIsolationEnv := map[string]string{}
	if _, statErr := os.Stat(filepath.Join(wtPath, ".git")); statErr == nil {
		var err error
		gitIsolationEnv, err = prepareExecuteBeadGitIsolation(projectRoot, wtPath, embeddedStateDir)
		if err != nil {
			return nil, fmt.Errorf("preparing execute-bead git isolation: %w", err)
		}
	} else if !os.IsNotExist(statErr) || runtime.AgentRunner == nil {
		return nil, fmt.Errorf("preparing execute-bead git isolation: stat worktree .git: %w", statErr)
	}
	sessionID := GenerateSessionID()
	startedAt := time.Now().UTC()
	runState.SessionID = sessionID
	_ = WriteRunState(projectRoot, runState)

	minPowerOverride := noChangesMinPowerOverride(beadCtx, rcfg.MinPower())

	// Running-phase provider guard: continuously quarantine provider CLIs that
	// do not belong to the active route while the attempt runs (ddx-2c973f8c).
	// Seed it with any pinned harness/model and refresh it when fizeau resolves
	// the route, then chain to the caller-supplied OnRouteResolved callback.
	providerGuard := newRunningProviderGuard(projectRoot, beadID, attemptID, os.Getpid())
	providerGuard.UpdateRoute(rcfg.Harness(), "", rcfg.Model())
	baseOnRouteResolved := onRouteResolvedFromContext(ctx)
	baseOnExecuteStart := onExecuteStartFromContext(ctx)

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
		// The implementation pass runs in an isolated attempt worktree and must
		// be able to write and commit without an approval loop. When the project
		// pins agent.permissions we honour it unchanged (ddx-642a9f26: the
		// sealed config wins); only an unset value falls back to unrestricted,
		// because the native fiz harness maps "" to its read-only toolset and
		// the attempt would produce no evidence at all.
		PermissionsOverride: implementerPermissions(rcfg.Permissions()),
		Role:                config.EvidenceRoleImplementer,
		CorrelationID:       beadID + ":" + attemptID,
		Env:                 gitIsolationEnv,
		OnRouteResolved: func(harness, provider, model string) {
			providerGuard.UpdateRoute(harness, provider, model)
			if baseOnRouteResolved != nil {
				baseOnRouteResolved(harness, provider, model)
			}
		},
		OnExecuteStart: baseOnExecuteStart,
	}
	if minPowerOverride > 0 {
		runRuntime.MinPowerOverride = minPowerOverride
	}
	if runtime.BeadStoreRoot != "" {
		runRuntime.Env["DDX_BEAD_DIR"] = runtime.BeadStoreRoot
	}
	runRuntime.Env[DDXModeEnvKey] = DDXModeBeadExecution
	runRuntime.Env[DDXBeadIDEnvKey] = beadID
	runRuntime.Env[DDXAttemptIDEnvKey] = attemptID

	// Operator-cancel mid-attempt poll (ADR-022 §Cancel SLA). The poll
	// re-reads the bead's Extra every CancelPollInterval; on
	// cancel-requested:true it cancels the dispatch context (which kills the
	// agent subprocess at the next syscall boundary) and writes
	// cancel-honored:true so subsequent cancel POSTs are silent no-ops.
	dispatchCtx, dispatchCancel := context.WithCancel(ctx)
	defer dispatchCancel()
	cancelHonored := startCancelPoll(dispatchCtx, dispatchCancel, beadID, runtime.BeadCancel)

	// If the active route's own harness process disappears mid-attempt
	// (crash, OOM, SIGKILL) without ever emitting a final event, cancelling
	// dispatchCtx here is what lets the drain loop (agent_runner_service.go's
	// drainWatchdog.ctx backstop) give up promptly instead of waiting on the
	// generic multi-hour idle timeout (ddx-f2b7cf89).
	providerGuard.SetHarnessDeadWatch(routeHarnessDeadGrace, dispatchCancel)

	processBaseline := captureAttemptProcessBaseline(dispatchCtx, wtPath)
	stopRunStateRefresh := startRunStateRefresh(dispatchCtx, projectRoot, runState)
	stopProviderGuard := providerGuard.Start(dispatchCtx)
	agentResult, agentErr := attemptBackend.Run(dispatchCtx, AttemptBackendRunRequest{
		ProjectRoot: projectRoot,
		Workspace:   workspace,
		Service:     runtime.Service,
		AgentRunner: runtime.AgentRunner,
		Config:      rcfg,
		Runtime:     runRuntime,
	})
	stopProviderGuard()
	stopRunStateRefresh()
	cleanupTrigger := ""
	if dispatchCtx.Err() != nil {
		cleanupTrigger = dispatchCtx.Err().Error()
	} else if agentErr != nil {
		cleanupTrigger = agentErr.Error()
	}
	_ = cleanupAttemptProcesses(context.Background(), projectRoot, beadID, attemptID, wtPath, processBaseline, cleanupTrigger)
	// Attempt-end backstop: reap every remaining provider child regardless of
	// route, then fold the running-phase guard's evidence into the final
	// provider-children.json so mid-attempt reaps survive in the audit trail.
	attemptEndReaped := reapAllProviderChildren(context.Background(), os.Getpid(), time.Now().UTC())
	if allReaped := append(providerGuard.Reaped(), attemptEndReaped...); len(allReaped) > 0 {
		writeProviderChildCleanupArtifact(projectRoot, attemptID, &providerChildCleanupReport{
			AttemptID: attemptID,
			BeadID:    beadID,
			Trigger:   firstNonEmpty(cleanupTrigger, reasonAttemptEnded),
			ScannedAt: time.Now().UTC(),
			Reaped:    allReaped,
		})
	}
	finishedAt := time.Now().UTC()

	exitCode := 0
	tokens := 0
	inputTokens := 0
	cachedTokens := 0
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
		cachedTokens = agentResult.CachedTokens
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
	sanitizeExecuteBeadWorktreeConfig(wtPath)

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
		res = &ExecuteBeadResult{
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
			ProjectRoot:                 projectRoot,
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
	if tokens > 0 || inputTokens > 0 || cachedTokens > 0 || outputTokens > 0 || costUSD > 0 {
		usage := executeBeadUsage{
			AttemptID:    attemptID,
			Harness:      resultHarness,
			Provider:     resultProvider,
			Model:        resultModel,
			Tokens:       tokens,
			InputTokens:  inputTokens,
			CachedTokens: cachedTokens,
			CacheHitRate: executeBeadCacheHitRate(inputTokens, cachedTokens),
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
				ProjectRoot:                 projectRoot,
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

	res = &ExecuteBeadResult{
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
		ProjectRoot:                 projectRoot,
		ExecutionDir:                artifacts.DirRel,
		PromptFile:                  artifacts.PromptRel,
		ManifestFile:                artifacts.ManifestRel,
		ResultFile:                  artifacts.ResultRel,
		UsageFile:                   usageFileRel,
		WorktreePath:                wtPath,
		StartedAt:                   startedAt,
		FinishedAt:                  finishedAt,
	}
	if resultRev != baseRev {
		res.ImplementationRev = resultRev
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
	evidenceOnlyNoChangesCommit := false
	if resultRev != baseRev && rationaleText != "" {
		if evidenceOnly, diffErr := isEvidenceOnlyNoChangesCommit(wtPath, baseRev, resultRev, artifacts.DirRel); diffErr == nil && evidenceOnly {
			evidenceOnlyNoChangesCommit = true
		}
	}
	switch {
	case resultRev != baseRev && rationaleText != "" && !evidenceOnlyNoChangesCommit:
		res.Outcome = ExecuteBeadOutcomeTaskFailed
		res.Reason = mixedCommitAndNoChangesRationaleReason
		res.Error = mixedCommitAndNoChangesRationaleReason
	case exitCode != 0 || agentReportedError:
		res.Outcome = ExecuteBeadOutcomeTaskFailed
	case resultRev == baseRev || evidenceOnlyNoChangesCommit:
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
				rescueRef := preserveDirtyNoEvidenceAttempt(wtPath, artifacts.DirAbs, artifacts.DirRel)
				if rescueRef != "" {
					res.PreserveRef = rescueRef
					res.Error = fmt.Sprintf("%s; dirty paths: %s; rescue: %s", res.Reason, strings.Join(paths, ", "), rescueRef)
				} else {
					res.Error = fmt.Sprintf("%s; dirty paths: %s", res.Reason, strings.Join(paths, ", "))
				}
			} else {
				res.Error = res.Reason
			}
		}
	}

	// Reject evidence-bearing candidates before any backend publication,
	// preserve-ref creation, gate, or landing path can make them durable.
	if err := VerifyCandidateHasNoExecutionEvidence(wtPath, res.BaseRev, res.ResultRev); err != nil {
		res.Outcome = ExecuteBeadOutcomeTaskFailed
		res.Reason = "local execution evidence entered candidate history"
		res.Error = err.Error()
		res.FailureMode = FailureModeAttemptIntegrity
		populateWorkerStatus(res)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, err
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
		if err := publishAttemptResult(res); err != nil {
			return res, fmt.Errorf("publishing attempt result: %w", err)
		}
		return res, nil
	}

	// Classify failure mode from worker-level signals. ApplyLandingToResult
	// may refine this with landing-level signals (merge conflict, gate
	// failure) before the final result is output.
	res.FailureMode = ClassifyFailureMode(res.Outcome, res.ExitCode, res.Error)

	// Structured post-agent integrity validation (ddx-725b65b4). A clean agent
	// exit with a commit is not sufficient evidence of a contract-honoring
	// attempt: reject attempts that rewrote the implementation commit after the
	// first commit, left required pre-commit gate evidence empty, or left tracked
	// files uncommitted. Such attempts are demoted to task_failed with a distinct
	// FailureMode so the orchestrator preserves them for review (rather than
	// merging) and an operator can tell the DDx validation rejection apart from
	// an implementation failure. Execution evidence stays outside Git, so the
	// reflog carries only candidate implementation commits.
	if res.Outcome == ExecuteBeadOutcomeTaskSucceeded && res.ExitCode == 0 && res.ImplementationRev != "" {
		verdict := ValidateAttemptIntegrity(AttemptIntegrityInput{
			BaseRev:           baseRev,
			ImplementationRev: res.ImplementationRev,
			CommitEvents:      readWorktreeCommitEvents(wtPath),
			DirtyPaths:        integrityDirtyPaths(wtPath),
			CodeChanging:      true,
		})
		if !verdict.OK {
			res.Outcome = ExecuteBeadOutcomeTaskFailed
			res.Reason = AttemptIntegrityPreserveReason
			res.Error = verdict.Detail
			res.FailureMode = FailureModeAttemptIntegrity
		}
	}

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
		RouteReason:  routeReason,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  tokens,
		CostUSD:      costUSD,
		DurationMS:   res.DurationMS,
		ExitCode:     exitCode,
	})

	populateWorkerStatus(res)
	cycleRuntime := runtime
	cycleRuntime.candidateImport = func(candidate CandidateResult) error {
		candidateResult := *res
		projectCandidateCycleReport(&candidateResult, candidate.Report)
		return attemptBackend.ImportCandidate(ctx, workspace, &candidateResult)
	}
	cycleRuntime.candidateImportRelease = func(CandidateResult) error {
		return attemptBackend.ReleaseCandidateImport(ctx, workspace)
	}
	if cycleRuntime.Checks == nil && runtime.AgentRunner == nil {
		cycleRuntime.Checks = &repositoryCandidateCheckRunner{
			bead:      beadCtx,
			result:    res,
			artifacts: artifacts,
		}
	}
	if cycleRuntime.Repair == nil && runtime.AgentRunner == nil {
		cycleRuntime.Repair = &fizeauCandidateRepairPass{
			projectRoot: projectRoot,
			workspace:   workspace,
			backend:     attemptBackend,
			service:     runtime.Service,
			config:      rcfg,
			runtime:     runRuntime,
			gitOps:      gitOps,
			artifacts:   artifacts,
		}
	}
	implementerCaps := rcfg.EvidenceCapsForRole(config.EvidenceRoleImplementer)
	cycleRuntime.candidateOriginalTask = repairOriginalTask(beadCtx)
	configureImplementerRepairEvidence(&cycleRuntime, implementerCaps)
	if err := applyWorkerCandidateCycle(ctx, projectRoot, wtPath, cycleRuntime, res); err != nil {
		res.Outcome = ExecuteBeadOutcomeTaskFailed
		res.ExitCode = 1
		if res.FailureMode == FailureModeAttemptIntegrity {
			res.Reason = "local execution evidence entered candidate history"
		} else if res.Reason == "" {
			res.Reason = res.Detail
		}
		populateWorkerStatus(res)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, fmt.Errorf("recording candidate cycle state: %w", err)
	}
	if err := writeArtifactJSON(artifacts.ResultAbs, res); err != nil {
		return nil, fmt.Errorf("writing execute-bead result artifact: %w", err)
	}
	// Defense in depth immediately before backend publication. Candidate-cycle
	// repair is allowed to replace ResultRev, so re-scan the projected final
	// history even though every cycle is also validated before ref pinning.
	if err := VerifyCandidateHasNoExecutionEvidence(wtPath, res.BaseRev, res.ResultRev); err != nil {
		res.Outcome = ExecuteBeadOutcomeTaskFailed
		res.Reason = "local execution evidence entered candidate history"
		res.Error = err.Error()
		res.ExitCode = 1
		res.FailureMode = FailureModeAttemptIntegrity
		populateWorkerStatus(res)
		_ = writeArtifactJSON(artifacts.ResultAbs, res)
		return res, err
	}

	// Execution evidence is per-machine working state and must NEVER be
	// committed (ddx-d10073a8). ResultRev stays the implementation/code commit;
	// the evidence bundle is published to the project root on disk (the deferred
	// publish above) where landing/review/audit read it directly. Nothing
	// evidence-bearing rides the merge/ff into the durable branch.
	if err := publishAttemptResult(res); err != nil {
		return res, fmt.Errorf("publishing attempt result: %w", err)
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

	if res.ExitCode == 0 && strings.TrimSpace(res.Error) == "" {
		workspace.KeepOnError = false
	}
	return res, nil
}

func configureImplementerRepairEvidence(runtime *ExecuteBeadRuntime, caps evidence.Caps) {
	if runtime == nil {
		return
	}
	if runtime.candidateDiff == nil {
		runtime.candidateDiff = func(candidate CandidateResult) (string, error) {
			return boundedCandidateRepairDiffWithCaps(candidate, caps)
		}
	}
	runtime.candidateRepairCaps = caps
	runtime.candidateCapsConfigured = true
}

func isEvidenceOnlyNoChangesCommit(wtPath, baseRev, resultRev, artifactsDirRel string) (bool, error) {
	rationalePath := filepath.ToSlash(filepath.Join(artifactsDirRel, "no_changes_rationale.txt"))
	out, err := internalgit.Command(context.Background(), wtPath, "diff", "--name-only", baseRev, resultRev, "--").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking no_changes rationale commit diff: %s: %w", strings.TrimSpace(string(out)), err)
	}
	var changed []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			changed = append(changed, filepath.ToSlash(p))
		}
	}
	if len(changed) != 1 {
		return false, nil
	}
	return changed[0] == rationalePath, nil
}

// dispatchAgentRun is a thin SD-024 wrapper around dispatchViaResolvedConfig
// for the execute-bead worker. It threads the durable knobs from rcfg and the
// per-invocation plumbing from runtime through the shared dispatch seam in
// service_run.go.
//
// Production dispatch always goes through Fizeau, including explicit script
// and virtual harness constraints. A non-nil AgentRunner is retained only as
// an explicitly injected test seam.
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

// cleanupAttemptWorktree removes the per-attempt execute-bead worktree for
// non-success outcomes unless the caller explicitly asked to preserve the
// worktree for inspection. It returns true when it performed the removal.
func cleanupAttemptWorktree(gitOps GitOps, workDir, wtPath, outcome string, preserveFlag bool) bool {
	if preserveFlag || outcome == ExecuteBeadOutcomeTaskSucceeded {
		return false
	}
	if gitOps == nil || workDir == "" || wtPath == "" {
		return false
	}
	_ = gitOps.WorktreeRemove(workDir, wtPath)
	return true
}

// preserveDirtyNoEvidenceAttempt stages all dirty files in the attempt
// worktree, generates a binary patch of staged content, and writes it to the
// artifact bundle directory so operators can recover the agent's uncommitted
// work. Returns the bundle-relative path of the patch on success, or "" if
// nothing was staged or the write failed. The patch survives worktree cleanup
// because publishEvidenceBundleToProjectRoot copies the artifact dir to the
// project root before cleanup runs.
func preserveDirtyNoEvidenceAttempt(wtPath, artifactsDirAbs, artifactsDirRel string) string {
	// Use the same exclusion pathspecs as SynthesizeCommit so that DDx-managed
	// execution-bundle files (prompt.md, manifest.json, embedded/, etc.) are not
	// included in the rescue patch. If only those noise files are dirty, the
	// staging produces an empty index and we return "" (clean no-evidence).
	addArgs := append([]string{"add", "-A", "--", "."}, synthesizeCommitExcludePathspecs(wtPath)...)
	if err := internalgit.Command(context.Background(), wtPath, addArgs...).Run(); err != nil {
		return ""
	}
	patchData, err := internalgit.Command(context.Background(), wtPath, "diff", "--cached", "--binary").Output()
	if err != nil || len(bytes.TrimSpace(patchData)) == 0 {
		return ""
	}
	if err := os.MkdirAll(artifactsDirAbs, 0o755); err != nil {
		return ""
	}
	patchPath := filepath.Join(artifactsDirAbs, "dirty_rescue.patch")
	if err := os.WriteFile(patchPath, patchData, 0o644); err != nil {
		return ""
	}
	return filepath.ToSlash(filepath.Join(artifactsDirRel, "dirty_rescue.patch"))
}

// commitTrackerLocked commits beads.jsonl if it has uncommitted changes.
// Callers must hold withTrackerLock(projectRoot) before calling. Used by Run()
// to fold tracker commit, checkpoint synthesis, and worktree creation into a
// single critical section so concurrent workers do not race on the parent's
// HEAD ref.
func commitTrackerLocked(projectRoot string) error {
	trackerFile := filepath.Join(beadStoreRoot(projectRoot), "beads.jsonl")
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

	gitDir, trackerPathspecs := ddxStateGitScope(projectRoot, ".ddx/beads.jsonl")
	trackerPathspec := trackerPathspecs[0]

	out, err := internalgit.Command(context.Background(), gitDir, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil
	}

	msg := fmt.Sprintf("chore: update tracker (execute-bead %s)", time.Now().UTC().Format("20060102T150405"))

	// Re-check inside the lock: a sibling worker may have already
	// committed the tracker changes between our pre-lock check and now.
	diff, err := internalgit.Command(context.Background(), gitDir, "diff", "--", trackerPathspec).Output()
	if err != nil {
		return fmt.Errorf("checking tracker diff: %w", err)
	}
	if strings.TrimSpace(string(diff)) == "" {
		untracked, err := internalgit.Command(context.Background(), gitDir, "ls-files", "--others", "--exclude-standard", trackerPathspec).Output()
		if err != nil {
			return fmt.Errorf("checking tracker untracked: %w", err)
		}
		if strings.TrimSpace(string(untracked)) == "" {
			return nil
		}
	}

	commitOut, err := runGitWithIndexLockRecovery(context.Background(), gitDir, "add", trackerPathspec)
	if err != nil {
		return fmt.Errorf("staging tracker: %s: %w", strings.TrimSpace(string(commitOut)), err)
	}
	// `git commit` would fail with "nothing to commit" if the file's
	// content is byte-identical to HEAD even though `git diff` saw a
	// diff (e.g. mode/whitespace race). Bail cleanly in that case.
	if cached, err := internalgit.Command(context.Background(), gitDir, "diff", "--cached", "--", trackerPathspec).Output(); err == nil && strings.TrimSpace(string(cached)) == "" {
		return nil
	}
	commitArgs := ddxStateCommitArgs(projectRoot, gitDir, "commit", "--no-verify", "--only", "-m", msg, "--", trackerPathspec)
	commitOut, err = runGitWithIndexLockRecovery(context.Background(), gitDir, commitArgs...)
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

func prepareArtifacts(projectRoot, wtPath, beadID, attemptID, baseRev string, rcfg config.ResolvedConfig, runtime ExecuteBeadRuntime) (*executeBeadArtifacts, *bead.Bead, error) {
	artifacts, err := createArtifactBundle(projectRoot, wtPath, attemptID)
	if err != nil {
		return nil, nil, err
	}
	b, refs, err := loadBeadContext(projectRoot, wtPath, beadID, runtime.BeadStoreRoot)
	if err != nil {
		return artifacts, nil, err
	}

	promptContent, promptSource, err := buildPromptWithCaps(projectRoot, b, refs, artifacts, baseRev, runtime.PromptFile, rcfg.ContextBudget(), rcfg.EvidenceCapsForRole(config.EvidenceRoleImplementer))
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(artifacts.PromptAbs, promptContent, 0o644); err != nil {
		return nil, nil, fmt.Errorf("writing execute-bead prompt artifact: %w", err)
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
		return nil, nil, fmt.Errorf("writing execute-bead manifest artifact: %w", err)
	}
	artifacts.PromptSHA = manifest.PromptSHA
	return artifacts, b, nil
}

func loadBeadContext(projectRoot, wtPath, beadID, canonicalStoreRoot string) (*bead.Bead, []executeBeadGoverningRef, error) {
	type candidate struct {
		storeRoot string
		refRoot   string
	}
	var candidates []candidate
	if strings.TrimSpace(canonicalStoreRoot) != "" {
		candidates = append(candidates, candidate{storeRoot: canonicalStoreRoot, refRoot: wtPath})
	}
	candidates = append(candidates, candidate{storeRoot: beadStoreRoot(wtPath), refRoot: wtPath})
	if projectRoot != "" && projectRoot != wtPath {
		candidates = append(candidates, candidate{storeRoot: beadStoreRoot(projectRoot), refRoot: projectRoot})
	}
	var lastErr error
	seen := map[string]struct{}{}
	for _, c := range candidates {
		if strings.TrimSpace(c.storeRoot) == "" {
			continue
		}
		cleanStore := filepath.Clean(c.storeRoot)
		if _, ok := seen[cleanStore]; ok {
			continue
		}
		seen[cleanStore] = struct{}{}
		store := bead.NewStore(c.storeRoot)
		b, err := store.Get(context.Background(), beadID)
		if err == nil {
			refRoot := c.refRoot
			if strings.TrimSpace(refRoot) == "" {
				refRoot = projectRoot
			}
			return b, ResolveGoverningRefs(refRoot, b), nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("bead: not found: %s", beadID)
	}
	return nil, nil, fmt.Errorf("loading bead %s from worktree snapshot: %w", beadID, lastErr)
}

func noChangesMinPowerOverride(b *bead.Bead, currentMinPower int) int {
	if b == nil || b.Extra == nil {
		return 0
	}
	raw, ok := b.Extra[executeLoopNoChangesNextMinPowerKey]
	if !ok {
		return 0
	}
	next, ok := beadExtraMinPower(raw)
	if !ok || next <= currentMinPower {
		return 0
	}
	return next
}

func beadExtraMinPower(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v > 0
	case int8:
		return int(v), v > 0
	case int16:
		return int(v), v > 0
	case int32:
		return int(v), v > 0
	case int64:
		return int(v), v > 0
	case uint:
		return int(v), v > 0
	case uint8:
		return int(v), v > 0
	case uint16:
		return int(v), v > 0
	case uint32:
		return int(v), v > 0
	case uint64:
		if v > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(v), v > 0
	case float64:
		return int(v), v > 0
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil && i > 0
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		return i, err == nil && i > 0
	default:
		return 0, false
	}
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
// 24 guardrails (FEAT-022 cross-reference):
//  1. AC checkbox: every AC satisfied by a specific code/test/file (anti-handwave)
//  2. Read named files / referenced specs first, before editing
//  3. Missing-governing fallback note (non-minimal renders only — see
//     executeBeadMissingGoverningText and contextBudget="minimal" branch)
//  4. Commit subject ends with [<bead-id>]
//  5. Commit exactly once, when green
//  6. git add <specific-paths> (prefer explicit staging over globs)
//  7. Never git add -A (worktree may have unrelated WIP)
//  8. Run git/index mutations sequentially; never parallelize git add/commit
//  9. Do not commit red code
// 10. Treat the normal git commit pre-commit hook as the single authoritative
//     staged gate; stage the exact commit set, run git commit normally, and use
//     that hook's output/exit status as the acceptance evidence. If you already
//     ran lefthook run pre-commit on the same staged tree and hook inputs, only
//     reuse it when the fingerprint matches; otherwise rerun after staged-tree
//     or hook-config changes. Pre-staging no-staged-files runs do not count as
//     acceptance evidence
// 11. Do not modify files outside bead scope
// 12. Never run `ddx init`
// 13. Keep .ddx/executions/ intact, local, and untracked
// 14. Do not rewrite CLAUDE.md / AGENTS.md unless asked
// 15. Bead description overrides CLAUDE.md / YAGNI defaults
// 16. Reports go under the bead metadata bundle path, never /tmp, and remain
//     untracked local evidence
// 17. Write no_changes_rationale.txt before exiting empty
// 18. Step 0 size-check + decomposition (ddx bead create / dep add / update)
// 19. Current-bead lifecycle mutations stay orchestrator-owned; only Step 0
//     parent-note updates may touch current-bead tracker state
// 20. Address every BLOCKING <review-findings> item; no no_changes with blocking findings open
// 21. Stop after the commit (Agent post-commit runaway guard)
// 22. Agent variant only: use tool calls, not `bash: cat`/`rg`/`ls`
// 23. Validation gates run sequentially: wait for a focused gate to finish
//     and pass before starting the broader gate, then wait again before lint
//     or pre-commit. Do not overlap go test / cargo test / npm test / make test
//     / lefthook invocations; parallelism inside a single command is fine
// 24. Long-running matrix/benchmark beads (FEAT-010): require explicit matrix
//     plan before running expensive commands; document output path and completion criterion
// 25. Prohibit rerunning identical long-running commands without documenting why
//     prior output is invalid and what changed before the retry

// instrStep0SizeCheck is the shared Step 0 size-check + decomposition recipe.
// Both variants emit it verbatim; per-variant preamble runs before it.
const instrStep0SizeCheck = `

## Step 0: size check

Too big if any holds:

- More than ~6 unrelated ACs.
- AC mixes design, implementation, integration tests, and docs.
- Description names multiple feature pieces.
- More than ~500 lines across ~5+ unrelated package files.
- If the bead description exceeds 8000 bytes, use a split-first pass.
- Auto-decomposition is capped at depth 2: root beads may split once, children once more; reject third-level splits.

If too big, decompose:

1. ` + "`ddx bead create`" + ` for each child (copy parent's labels and spec-id).
2. ` + "`ddx bead dep add`" + ` only for legitimate child-to-child or sibling/replacement edges; never the parent.
3. Use ` + "`parent=<parent-id>`" + ` metadata and wire ` + "`parent -> child`" + `.
4. Write ` + "`no_changes_rationale.txt`" + ` under the ` + "`bundle`" + ` path, then stop.

Decomposition alone is success. Don't mix it with implementation.`

// instrNoChangesContract is the shared NoChangesContract (TD-031 §8.1) rule.
const instrNoChangesContract = `

## no_changes contract

The ` + "`no_changes_rationale.txt`" + ` file under the ` + "`bundle`" + ` path must contain one of:

- ` + "`verification_command: <cmd>`" + ` — exit 0 closes, nonzero rejects.
- ` + "`status: open`" + ` + ` + "`reason: <retryable>`" + ` — open, smart retry.
- ` + "`status: proposed`" + ` + ` + "`reason: <operator needed>`" + ` — operator lane.
- ` + "`status: blocked`" + ` + ` + "`reason: <external blocker>`" + ` — blocked lane.

No pseudo-statuses. Bare rationales and ` + "`status: needs_investigation`" + ` are rejected.

For orchestrator decomposition, add ` + "`orchestrator_action: decompose`" + ` alongside ` + "`status: open`" + `.`

// instrInvestigationReports is the shared report-output rule.
const instrInvestigationReports = `

## Reports

Write reports under the metadata ` + "`bundle`" + ` path in ` + "`.ddx/executions/`" + `, never ` + "`/tmp`" + `. Keep bundle files local and untracked. Commit only requested deliverables outside the bundle.`

// instrReviewGate is the shared review-as-gate rule.
const instrReviewGate = `

## Review gate

- The review is a gate, not an escape hatch.
- Address every BLOCKING ` + "`<review-findings>`" + ` item; do not declare ` + "`no_changes`" + ` with blocking findings open.`

// instrBeadOverride is the shared mode + bead-overrides-defaults rule.
const instrBeadOverride = `

## Mode and overrides

DDX_MODE=bead_execution: edit code/docs for bead AC. Only broad queue-steward default is overridden; tracker, merge-policy, verification, safety stay active. Bead description/AC override CLAUDE.md, AGENTS.md, and defaults.`

// instrCoreConstraints is the shared core-constraints tail.
const instrCoreConstraints = `

## Constraints

- Work only inside this execution worktree.
- Keep ` + "`.ddx/executions/`" + ` intact, local, and untracked; never stage or commit it.
- **Never run ` + "`ddx init`" + `** — the workspace is initialized.
- Do not modify files outside the bead's named scope.
- Do not rewrite CLAUDE.md, AGENTS.md, or other instruction files unless the bead asks.`

// instrLongRunningMatrixGuards is the shared long-running matrix/benchmark guardrails (FEAT-010).
const instrLongRunningMatrixGuards = `

## Long-running matrix/benchmark beads

For expensive commands (>60s per variant):

- Write a matrix plan: configs, output paths, completion criteria.
- Do not re-run the same long-running command unless the fingerprint changed and you document why prior output is invalid and what changed.
- If a long-running command times out or is incomplete, exit with ` + "`status: open`" + ` + retryable ` + "`reason`" + ` in ` + "`no_changes_rationale.txt`" + `. Do not silently retry.`

// executeBeadInstructionsText is the harness-neutral <instructions> body sent
// for every execute-bead dispatch. Fizeau owns concrete harness selection and
// capability details, so DDx must not vary its prompt by requested or resolved
// harness identity.
const executeBeadInstructionsText = `You are executing one bead in an isolated DDx execution worktree. The bead's <description> and <acceptance> are the contract: every AC must be provably satisfied by a specific code, test, or file after your commit.` +
	instrStep0SizeCheck +
	`

## How to work

- Read first. If the bead names files, specs, or prior beads, read them before editing.
- Cross-reference each AC to concrete evidence. If you cannot point at it, it is not done.
- Run tests and lint before committing. **Do not commit red code**.
- Run validation gates sequentially: wait for a focused gate to finish and pass before starting the broader gate. Then wait before lint or pre-commit.
- Do not overlap validation processes in one attempt: ` + "`go test`" + `, ` + "`cargo test`" + `, ` + "`npm test`" + `, ` + "`make test`" + `, and ` + "`lefthook`" + ` run sequentially.
- Parallelism inside one command is fine.
- Run git/index mutations sequentially; don't parallelize ` + "`git add`" + `, ` + "`git commit`" + `, or staging/commit commands.
- Stage with ` + "`git add <specific-paths>`" + `; never ` + "`git add -A`" + `.
- Treat the ` + "`git commit`" + ` hook as the single authoritative staged gate. Stage the exact commit set, run ` + "`git commit`" + ` normally, and use that hook's output/exit status as the acceptance evidence. If you already ran ` + "`lefthook run pre-commit`" + ` on the same staged tree and hook inputs, reuse it only when the fingerprint matches. A ` + "`no-staged-files`" + ` run is not acceptance evidence.
- Commit exactly once; subject ends with ` + "`[<bead-id>]`" + `.
- Do not modify files outside the bead's scope.
- Current-bead lifecycle is orchestrator-owned. Do not run ` + "`ddx bead update <bead-id> --claim`" + `, ` + "`ddx bead update <bead-id> --status <status>`" + `, ` + "`ddx bead update <bead-id> --unclaim`" + `, or ` + "`ddx bead close <bead-id>`" + `. Step 0 allows ` + "`ddx bead create`" + `, ` + "`ddx bead dep add`" + ` for child-to-child or sibling/replacement edges, and ` + "`ddx bead update <parent-id> --notes 'decomposed into <child-ids>'`" + `.
- If you cannot finish, write ` + "`no_changes_rationale.txt`" + ` under the bead metadata ` + "`bundle`" + ` path before exiting.` +
	instrNoChangesContract +
	instrInvestigationReports +
	instrBeadOverride +
	instrReviewGate +
	instrCoreConstraints +
	instrLongRunningMatrixGuards +
	`

## When the work is done

After the commit succeeds and every AC is verified, stop. Return control to the orchestrator.`

// executeBeadMissingGoverningText is emitted inside <governing> when no
// governing references were pre-resolved for the bead. The bead description
// is the primary contract — this note only reminds the agent to treat it as
// such and to ground any unclear decisions in repository state rather than
// guessing.
const executeBeadMissingGoverningText = `No governing references were pre-resolved. The bead description above is the primary contract. If it names files, specs, or prior beads, read them first. Ground decisions in repository state; do not guess.`

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

func buildPrompt(workDir string, b *bead.Bead, refs []executeBeadGoverningRef, artifacts *executeBeadArtifacts, baseRev, promptOverride, contextBudget string) ([]byte, string, error) {
	return buildPromptWithCaps(workDir, b, refs, artifacts, baseRev, promptOverride, contextBudget, evidence.DefaultCaps())
}

func buildPromptWithCaps(workDir string, b *bead.Bead, refs []executeBeadGoverningRef, artifacts *executeBeadArtifacts, baseRev, promptOverride, contextBudget string, caps evidence.Caps) ([]byte, string, error) {
	if strings.TrimSpace(promptOverride) != "" {
		path := promptOverride
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		data, err := readPromptFileBoundedWithCap(path, caps.MaxPromptBytes)
		if err != nil {
			return nil, "", fmt.Errorf("reading prompt override %q: %w", promptOverride, err)
		}
		return data, promptOverride, nil
	}

	var sb strings.Builder
	sb.WriteString("<execute-bead>\n")

	instructions := executeBeadInstructionsText + executeBeadDynamicStep0Hints(workDir, b)
	// Keep the initial prefix stable for provider prefix caches: the shared
	// instructions render first, bead-specific hints append at the end of that
	// block, and the attempt-specific bundle path only appears later on
	// <metadata bundle="..."/>.
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

	prompt := sb.String()
	if err := validateInlinePromptCap("synthesized implementer prompt", prompt, caps.MaxPromptBytes); err != nil {
		return nil, "", err
	}
	return []byte(prompt), "synthesized", nil
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

	store := bead.NewStore(beadStoreRoot(workDir))
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
	CachedTokens int     `json:"cached_tokens,omitempty"`
	CacheHitRate float64 `json:"cache_hit_rate,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

func executeBeadCacheHitRate(inputTokens, cachedTokens int) float64 {
	totalPromptInput := inputTokens + cachedTokens
	if cachedTokens <= 0 || totalPromptInput <= 0 {
		return 0
	}
	return float64(cachedTokens) / float64(totalPromptInput)
}

const executionEvidenceLocalExclude = "/.ddx/executions/"

// gitWorktreeStatus reports whether workspace resolves inside a Git worktree.
// Locale is pinned because the only recoverable error is Git's explicit
// not-a-repository diagnosis. Missing Git, corrupt metadata, permission
// failures, and every other probe error remain fail-closed.
func gitWorktreeStatus(workspace string) (bool, error) {
	cmd := internalgit.Command(context.Background(), workspace,
		"rev-parse", "--is-inside-work-tree",
	)
	cmd.Env = envWithOverrides(cmd.Env, map[string]string{"LC_ALL": "C", "LANG": "C"})
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.ToLower(string(out))
		if strings.Contains(message, "not a git repository") {
			return false, nil
		}
		return false, fmt.Errorf("checking workspace Git metadata: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if strings.TrimSpace(string(out)) != "true" {
		return false, fmt.Errorf("workspace %s is not inside a Git worktree", workspace)
	}
	return true, nil
}

// VerifyCleanWorktree keeps local execution evidence out of Git without
// deleting it. Repositories created before execution evidence became
// per-machine state may be missing the project .gitignore rule, so the safety
// net installs an idempotent repository-local exclude in Git's info/exclude.
// It never changes the tracked .gitignore, stages files, or creates commits.
//
// Evidence that is already tracked or staged is an invariant violation. That
// state requires an operator decision, so fail before mutating even the local
// exclude file and leave the worktree, index, and history untouched.
func VerifyCleanWorktree(projectRoot, attemptID string) error {
	if attemptID == "" {
		return nil
	}
	insideGitWorktree, err := gitWorktreeStatus(projectRoot)
	if err != nil {
		return err
	}
	if !insideGitWorktree {
		// Plain-directory attempt backends cannot have staged or tracked files.
		// They still retain the published bundle on local disk.
		return nil
	}
	evidenceDir := filepath.ToSlash(filepath.Join(ExecuteBeadArtifactDir, attemptID))

	stagedOut, stagedErr := internalgit.Command(context.Background(), projectRoot,
		"diff", "--cached", "--name-only", "--", evidenceDir,
	).CombinedOutput()
	if stagedErr != nil {
		return fmt.Errorf("checking staged execution evidence: %s: %w", strings.TrimSpace(string(stagedOut)), stagedErr)
	}
	if len(strings.TrimSpace(string(stagedOut))) != 0 {
		return fmt.Errorf("execution evidence is staged under %s; refusing to alter the index, history, or evidence files", evidenceDir)
	}

	trackedOut, trackedErr := internalgit.Command(context.Background(), projectRoot,
		"ls-files", "--", evidenceDir,
	).CombinedOutput()
	if trackedErr != nil {
		return fmt.Errorf("checking tracked execution evidence: %s: %w", strings.TrimSpace(string(trackedOut)), trackedErr)
	}
	if len(strings.TrimSpace(string(trackedOut))) != 0 {
		return fmt.Errorf("execution evidence is tracked under %s; remove it from Git explicitly before retrying (DDx made no changes)", evidenceDir)
	}

	evidenceAbs := filepath.Join(projectRoot, filepath.FromSlash(evidenceDir))
	if _, err := os.Stat(evidenceAbs); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking local execution evidence %s: %w", evidenceDir, err)
	}

	// Serialize only the short repository-metadata update. This is the same
	// process-shared lock used by other main-Git mutations, and it is acquired
	// after the harness has returned, never across LLM work.
	if err := withMainGitLock(projectRoot, "evidence_local_exclude", func() error {
		return ensureExecutionEvidenceLocalExclude(projectRoot)
	}); err != nil {
		return fmt.Errorf("installing repository-local execution evidence exclude: %w", err)
	}

	statusOut, statusErr := internalgit.Command(context.Background(), projectRoot,
		"status", "--porcelain", "--untracked-files=all", "--", evidenceDir,
	).CombinedOutput()
	if statusErr != nil {
		return fmt.Errorf("verifying local execution evidence exclusion: %s: %w", strings.TrimSpace(string(statusOut)), statusErr)
	}
	if len(strings.TrimSpace(string(statusOut))) != 0 {
		return fmt.Errorf("execution evidence under %s remains visible to Git after installing %s in info/exclude; inspect repository ignore rules (DDx did not stage, commit, or delete evidence)", evidenceDir, executionEvidenceLocalExclude)
	}
	return nil
}

// VerifyCandidateHasNoExecutionEvidence rejects implementation revisions that
// change any path under the local-only execution tree. The local exclude and
// synthesis filters prevent normal paths from creating such commits; this
// revision-level check is the final defence against harness-authored commits.
func VerifyCandidateHasNoExecutionEvidence(projectRoot, baseRev, resultRev string) error {
	if strings.TrimSpace(baseRev) == "" || strings.TrimSpace(resultRev) == "" || baseRev == resultRev {
		return nil
	}
	insideGitWorktree, err := gitWorktreeStatus(projectRoot)
	if err != nil {
		return err
	}
	if !insideGitWorktree {
		// Plain-directory test and in-memory backends have no candidate Git
		// history to inspect. Real repositories remain fail-closed above.
		return nil
	}
	revsOut, err := internalgit.Command(context.Background(), projectRoot,
		"rev-list", "--reverse", baseRev+".."+resultRev,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("listing candidate commits for local execution evidence: %s: %w", strings.TrimSpace(string(revsOut)), err)
	}
	for _, commit := range strings.Fields(string(revsOut)) {
		pathsOut, diffErr := internalgit.Command(context.Background(), projectRoot,
			"diff-tree", "--root", "-m", "--no-commit-id", "--name-only", "-r", "-z", commit, "--",
		).CombinedOutput()
		if diffErr != nil {
			return fmt.Errorf("checking candidate commit %s for local execution evidence: %s: %w", commit, strings.TrimSpace(string(pathsOut)), diffErr)
		}
		for _, raw := range bytes.Split(pathsOut, []byte{0}) {
			path := filepath.ToSlash(strings.TrimSpace(string(raw)))
			if isExecutionEvidencePath(path) {
				return fmt.Errorf("candidate history commit %s changes local execution evidence path %s; refusing to preserve or land it", commit, path)
			}
		}
	}
	return nil
}

func ensureExecutionEvidenceLocalExclude(projectRoot string) error {
	pathOut, pathErr := internalgit.Command(context.Background(), projectRoot,
		"rev-parse", "--git-path", "info/exclude",
	).CombinedOutput()
	if pathErr != nil {
		return fmt.Errorf("resolving repository-local Git exclude: %s: %w", strings.TrimSpace(string(pathOut)), pathErr)
	}
	excludePath := strings.TrimSpace(string(pathOut))
	if excludePath == "" {
		return fmt.Errorf("resolving repository-local Git exclude: Git returned an empty path")
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(projectRoot, filepath.FromSlash(excludePath))
	}

	content, readErr := os.ReadFile(excludePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("reading repository-local Git exclude %s: %w", excludePath, readErr)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == executionEvidenceLocalExclude {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("creating repository-local Git exclude directory: %w", err)
	}
	f, openErr := os.OpenFile(excludePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if openErr != nil {
		return fmt.Errorf("opening repository-local Git exclude %s: %w", excludePath, openErr)
	}
	prefix := ""
	if len(content) > 0 && content[len(content)-1] != '\n' {
		prefix = "\n"
	}
	if _, writeErr := io.WriteString(f, prefix+executionEvidenceLocalExclude+"\n"); writeErr != nil {
		_ = f.Close()
		return fmt.Errorf("writing repository-local Git exclude %s: %w", excludePath, writeErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		return fmt.Errorf("closing repository-local Git exclude %s: %w", excludePath, closeErr)
	}
	return nil
}

// ensureExecutionEvidenceLocalExcludeForWorkspace installs the local rule when
// Git resolves workspace (including a subdirectory) into a worktree. Some
// unit-test attempt backends use plain directories; only Git's explicit
// not-a-repository result is skipped. Missing Git, corrupt metadata, permission
// failures, and all other errors remain fail-closed.
func ensureExecutionEvidenceLocalExcludeForWorkspace(workspace string) error {
	insideGitWorktree, err := gitWorktreeStatus(workspace)
	if err != nil {
		return err
	}
	if !insideGitWorktree {
		return nil
	}
	return ensureExecutionEvidenceLocalExclude(workspace)
}

// publishEvidenceBundleToProjectRoot copies the evidence bundle from the
// isolated attempt worktree to the project root. Used by non-landing paths
// (no-changes, task-failed, operator-cancel) so local evidence remains
// available after the isolated worktree is removed. A no-op when the source
// does not exist or when wtPath and projectRoot resolve to the same path.
func publishEvidenceBundleToProjectRoot(projectRoot, wtPath, dirRel string) error {
	return publishEvidenceBundleToProjectRootWithCopier(projectRoot, wtPath, dirRel, nil)
}

func publishEvidenceBundleToProjectRootWithCopier(projectRoot, wtPath, dirRel string, copier func(source, target string, mode os.FileMode) error) error {
	if dirRel == "" || sameFilesystemPath(projectRoot, wtPath) {
		return nil
	}
	if copier == nil {
		copier = copyLocalExecutionEvidenceFile
	}
	src := filepath.Join(wtPath, filepath.FromSlash(dirRel))
	dst := filepath.Join(projectRoot, filepath.FromSlash(dirRel))
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat local execution evidence source %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local execution evidence source %s is not a directory", src)
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("preparing local execution evidence destination %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(dst)+".publish-")
	if err != nil {
		return fmt.Errorf("preparing local execution evidence staging directory: %w", err)
	}
	stagingOwned := true
	defer func() {
		if stagingOwned {
			_ = os.RemoveAll(staging)
		}
	}()

	if dstInfo, statErr := os.Stat(dst); statErr == nil {
		if !dstInfo.IsDir() {
			return fmt.Errorf("local execution evidence destination %s is not a directory", dst)
		}
		if err := copyLocalExecutionEvidenceTree(dst, staging, copier); err != nil {
			return fmt.Errorf("copying existing local execution evidence %s into staging: %w", dst, err)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("checking local execution evidence destination %s: %w", dst, statErr)
	}
	if err := copyLocalExecutionEvidenceTree(src, staging, copier); err != nil {
		return fmt.Errorf("copying local execution evidence %s to %s: %w", src, dst, err)
	}

	backup := ""
	if _, statErr := os.Stat(dst); statErr == nil {
		backupDir, backupErr := os.MkdirTemp(parent, "."+filepath.Base(dst)+".previous-")
		if backupErr != nil {
			return fmt.Errorf("preparing local execution evidence replacement: %w", backupErr)
		}
		if removeErr := os.Remove(backupDir); removeErr != nil {
			return fmt.Errorf("preparing local execution evidence backup path: %w", removeErr)
		}
		backup = backupDir
		if renameErr := os.Rename(dst, backup); renameErr != nil {
			return fmt.Errorf("backing up local execution evidence destination %s: %w", dst, renameErr)
		}
	}
	if renameErr := os.Rename(staging, dst); renameErr != nil {
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		return fmt.Errorf("publishing local execution evidence %s: %w", dst, renameErr)
	}
	stagingOwned = false
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func copyLocalExecutionEvidenceTree(src, dst string, copier func(source, target string, mode os.FileMode) error) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
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
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		return copier(path, target, info.Mode().Perm())
	})
}

func copyLocalExecutionEvidenceFile(source, target string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, mode)
}

func finalizeLocalExecutionEvidence(projectRoot, wtPath, dirRel, attemptID string) error {
	return finalizeLocalExecutionEvidenceWithCopier(projectRoot, wtPath, dirRel, attemptID, nil)
}

func finalizeLocalExecutionEvidenceWithCopier(projectRoot, wtPath, dirRel, attemptID string, copier func(source, target string, mode os.FileMode) error) error {
	if err := publishEvidenceBundleToProjectRootWithCopier(projectRoot, wtPath, dirRel, copier); err != nil {
		return err
	}
	if err := VerifyCleanWorktree(projectRoot, attemptID); err != nil {
		return fmt.Errorf("verifying published local execution evidence: %w", err)
	}
	return nil
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

// implementerPermissions resolves the permission mode for the execute-bead
// implementation dispatch. An explicit project setting is always honoured; an
// unset value defaults to unrestricted so the attempt can write and commit in
// its isolated worktree instead of inheriting the harness read-only default.
func implementerPermissions(sealed string) string {
	if strings.TrimSpace(sealed) != "" {
		return ""
	}
	return "unrestricted"
}
