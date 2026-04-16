package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

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

// executeBeadChecks is the machine-readable schema for checks.json.
// Written by the orchestrator when gate evaluation runs.
type executeBeadChecks struct {
	AttemptID   string            `json:"attempt_id"`
	EvaluatedAt time.Time         `json:"evaluated_at"`
	Summary     string            `json:"summary"`
	Results     []GateCheckResult `json:"results"`
}

// OrchestratorGitOps abstracts the git operations needed by the parent-side
// orchestrator for preserving worker results under iteration refs.
//
// NOTE: The Merge(dir, rev) method that existed here before the land
// coordinator redesign has been DELETED. All target-branch writes now flow
// through Land() in execute_bead_land.go and its per-project serialized
// coordinator. See ddx-8746d8a6 / ddx-e14efc58 / ddx-6aa50e57 for the
// rationale: the old path produced "chore: checkpoint before merge" noise
// and workers racing on the same projectRoot could corrupt each other's
// intermediate state. Land() serializes through a single goroutine and
// uses `git merge --no-ff` when the target has advanced, so the worker's
// commit is never rewritten and replay sees the same inputs it originally saw.
//
// The LandingAdvancer field on BeadLandingOptions is the coordinator
// injection point for LandBeadResult callers that need to ff the target
// branch. When LandingAdvancer is nil (the interactive single-bead CLI),
// LandBeadResult falls back to preserving the result under
// refs/ddx/iterations/<bead-id>/... rather than modifying the target branch.
type OrchestratorGitOps interface {
	UpdateRef(dir, ref, sha string) error
}

// RealOrchestratorGitOps implements OrchestratorGitOps via os/exec git commands.
type RealOrchestratorGitOps struct{}

// UpdateRef updates a git ref to point at sha.
func (r *RealOrchestratorGitOps) UpdateRef(dir, ref, sha string) error {
	out, err := osexec.Command("git", "-C", dir, "update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// BeadLandingOptions controls how the orchestrator lands a completed worker result.
type BeadLandingOptions struct {
	// NoMerge skips the land step and preserves the result under
	// refs/ddx/iterations/<bead-id>/... instead.
	NoMerge bool

	// WtPath is the path to a worktree checked out at ResultRev, used for gate
	// evaluation. When empty, gate evaluation is skipped (the worktree has
	// typically been cleaned up by the time the orchestrator runs).
	WtPath string

	// GovernIDs are the governing artifact IDs to use for gate evaluation.
	// Only used when WtPath is non-empty. Typically extracted from the worker's
	// manifest artifact via ExtractGoverningIDsFromManifest.
	GovernIDs []string

	// ChecksArtifactPath is the absolute path to write checks.json. Optional.
	ChecksArtifactPath string
	// ChecksArtifactRel is the relative path stored in the result for checks.json.
	ChecksArtifactRel string

	// LandingAdvancer, when non-nil, replaces the old in-process Merge step
	// with the coordinator-pattern Land() call. The callback is expected to
	// run fetch → (ff or merge) → push serialized against other submissions
	// for the same projectRoot. When nil, LandBeadResult falls back to
	// preserving the result under refs/ddx/iterations/<bead-id>/...
	// rather than touching the target branch.
	LandingAdvancer func(res *ExecuteBeadResult) (*LandResult, error)
}

// BeadLandingResult records the outcome of the orchestrator's landing step.
type BeadLandingResult struct {
	// Outcome is one of: "merged", "preserved", "no-changes".
	Outcome string `json:"outcome"`
	// Reason is a human-readable explanation of the outcome.
	Reason string `json:"reason,omitempty"`
	// PreserveRef is set when the result was preserved under refs/ddx/iterations/...
	PreserveRef string `json:"preserve_ref,omitempty"`
	// GateResults holds the outcome of each required execution gate.
	GateResults []GateCheckResult `json:"gate_results,omitempty"`
	// RequiredExecSummary is "pass", "fail", or "skipped".
	RequiredExecSummary string `json:"required_exec_summary,omitempty"`
	// ChecksFile is the relative path to checks.json when gate results were written.
	ChecksFile string `json:"checks_file,omitempty"`
}

// LandBeadResult is the parent-side orchestrator. It receives a completed worker
// result and decides whether to merge, preserve, or report no-changes. All
// Merge, UpdateRef, gate evaluation, and preserve-ref management live here.
// The worker (ExecuteBead) must not call any of these operations.
//
// Outcome rules:
//   - ResultRev == BaseRev → "no-changes" (agent made no commits)
//   - ExitCode != 0 with commits → "preserved" (agent failed but left output)
//   - Gate failed → "preserved"
//   - NoMerge → "preserved"
//   - Default → attempt merge; on conflict → "preserved"
func LandBeadResult(projectRoot string, res *ExecuteBeadResult, gitOps OrchestratorGitOps, opts BeadLandingOptions) (*BeadLandingResult, error) {
	landing := &BeadLandingResult{}

	// Agent failed with no commits: report as error (not no-changes).
	// Prefer res.Reason (e.g. HeadRev failure set by worker) over res.Error
	// (agent error) so the primary context failure is surfaced as the reason.
	if res.ExitCode != 0 && res.ResultRev == res.BaseRev {
		landing.Outcome = "error"
		switch {
		case res.Reason != "":
			landing.Reason = res.Reason
		case res.Error != "":
			landing.Reason = res.Error
		default:
			landing.Reason = "agent execution failed"
		}
		return landing, nil
	}

	// No changes from the worker: nothing to land.
	if res.ResultRev == res.BaseRev {
		landing.Outcome = "no-changes"
		if res.NoChangesRationale != "" {
			landing.Reason = res.NoChangesRationale
		} else {
			landing.Reason = "agent made no commits"
		}
		return landing, nil
	}

	// Agent failed but produced commits: preserve without attempting merge.
	if res.ExitCode != 0 {
		ref := PreserveRef(res.BeadID, res.BaseRev)
		if err := gitOps.UpdateRef(projectRoot, ref, res.ResultRev); err != nil {
			return nil, fmt.Errorf("preserving result ref: %w", err)
		}
		landing.Outcome = "preserved"
		landing.PreserveRef = ref
		landing.Reason = "agent execution failed"
		return landing, nil
	}

	// Evaluate required gates when a worktree path and governing IDs are provided.
	var gateResults []GateCheckResult
	var anyGateFailed bool
	if opts.WtPath != "" && len(opts.GovernIDs) > 0 {
		var err error
		gateResults, anyGateFailed, err = evaluateRequiredGates(opts.WtPath, opts.GovernIDs)
		if err != nil {
			return nil, fmt.Errorf("evaluating required gates: %w", err)
		}
	}
	landing.GateResults = gateResults
	landing.RequiredExecSummary = summarizeGates(gateResults, anyGateFailed)

	// Write checks.json when gate evaluation ran and a path is provided.
	if len(gateResults) > 0 && opts.ChecksArtifactPath != "" {
		checks := executeBeadChecks{
			AttemptID:   res.AttemptID,
			EvaluatedAt: time.Now().UTC(),
			Summary:     landing.RequiredExecSummary,
			Results:     gateResults,
		}
		if writeErr := writeArtifactJSON(opts.ChecksArtifactPath, checks); writeErr == nil {
			landing.ChecksFile = opts.ChecksArtifactRel
		}
	}

	// Gate failed: preserve instead of merging.
	if anyGateFailed {
		ref := PreserveRef(res.BeadID, res.BaseRev)
		if err := gitOps.UpdateRef(projectRoot, ref, res.ResultRev); err != nil {
			return nil, fmt.Errorf("preserving result ref: %w", err)
		}
		landing.Outcome = "preserved"
		landing.PreserveRef = ref
		landing.Reason = "post-run checks failed"
		return landing, nil
	}

	// --no-merge: preserve unconditionally.
	if opts.NoMerge {
		ref := PreserveRef(res.BeadID, res.BaseRev)
		if err := gitOps.UpdateRef(projectRoot, ref, res.ResultRev); err != nil {
			return nil, fmt.Errorf("preserving result ref: %w", err)
		}
		landing.Outcome = "preserved"
		landing.PreserveRef = ref
		landing.Reason = "--no-merge specified"
		return landing, nil
	}

	// Default: land the worker's commits on the target branch. When a
	// LandingAdvancer is provided (server coordinator / --local coordinator)
	// it runs the fetch → (ff or merge) → push sequence serialized per
	// projectRoot. When no advancer is provided, LandBeadResult falls back
	// to preserving the result under refs/ddx/iterations/ — the interactive
	// single-bead CLI path, which intentionally does NOT auto-advance the
	// target branch (the operator moves the ref themselves).
	if opts.LandingAdvancer != nil {
		land, landErr := opts.LandingAdvancer(res)
		if landErr != nil {
			return nil, fmt.Errorf("land advancer: %w", landErr)
		}
		switch land.Status {
		case "landed":
			landing.Outcome = "merged"
			if land.Merged {
				landing.Reason = "merged onto current tip"
			}
			if land.NewTip != "" {
				res.ResultRev = land.NewTip
			}
			if land.PushFailed {
				landing.Reason = "landed locally; push failed: " + land.PushError
			}
		case "preserved":
			landing.Outcome = "preserved"
			landing.PreserveRef = land.PreserveRef
			landing.Reason = land.Reason
		case "no-changes":
			landing.Outcome = "no-changes"
			landing.Reason = land.Reason
		default:
			landing.Outcome = "preserved"
			landing.Reason = "unknown land status: " + land.Status
		}
		return landing, nil
	}

	// No advancer: preserve under refs/ddx/iterations/ as a safe fallback.
	ref := PreserveRef(res.BeadID, res.BaseRev)
	if err := gitOps.UpdateRef(projectRoot, ref, res.ResultRev); err != nil {
		return nil, fmt.Errorf("preserving result ref (no land advancer): %w", err)
	}
	landing.Outcome = "preserved"
	landing.PreserveRef = ref
	landing.Reason = "no land advancer configured"
	return landing, nil
}

// ApplyLandingToResult merges a BeadLandingResult's fields into an
// ExecuteBeadResult so callers can output a single unified record. It
// overwrites Outcome, Status, Detail, Reason, PreserveRef, GateResults,
// RequiredExecSummary, and ChecksFile based on the landing decision.
func ApplyLandingToResult(res *ExecuteBeadResult, landing *BeadLandingResult) {
	res.Outcome = landing.Outcome
	res.Reason = landing.Reason
	res.PreserveRef = landing.PreserveRef
	res.GateResults = landing.GateResults
	res.RequiredExecSummary = landing.RequiredExecSummary
	res.ChecksFile = landing.ChecksFile
	// Re-classify status based on landing outcome and reason.
	res.Status = ClassifyExecuteBeadStatus(landing.Outcome, res.ExitCode, landing.Reason)
	res.Detail = ExecuteBeadStatusDetail(res.Status, landing.Reason, res.Error)
	// Refine failure_mode with landing-level signals. A clean merge clears
	// the field; merge conflict or gate failure overrides the worker-level
	// classification; other preserved outcomes keep the worker's mode.
	res.FailureMode = classifyLandingFailureMode(landing.Outcome, landing.Reason, landing.GateResults, res.FailureMode)
}

// ExtractGoverningIDsFromManifest reads governing artifact IDs from a manifest
// JSON file at manifestAbs (absolute path). Returns nil when the file cannot
// be read. Callers use this to populate BeadLandingOptions.GovernIDs.
func ExtractGoverningIDsFromManifest(manifestAbs string) []string {
	if manifestAbs == "" {
		return nil
	}
	type manifestShape struct {
		Governing []struct {
			ID string `json:"id"`
		} `json:"governing"`
	}
	raw, err := os.ReadFile(manifestAbs)
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

// RecoverOrphans removes orphaned execute-bead worktrees for a given bead ID.
// This is the parent's responsibility — call it before spawning new workers
// so stale worktrees from crashed previous attempts do not accumulate.
func RecoverOrphans(gitOps GitOps, workDir, beadID string) {
	paths, err := gitOps.WorktreeList(workDir)
	if err != nil {
		return
	}
	// Match by basename prefix so orphans are found regardless of their parent
	// directory (legacy .ddx/ vs new $TMPDIR/ddx-exec-wt/).
	basenamePrefix := ExecuteBeadWtPrefix + beadID + "-"
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), basenamePrefix) {
			_ = gitOps.WorktreeRemove(workDir, p)
		}
	}
	_ = gitOps.WorktreePrune(workDir)
}

// evaluateRequiredGates resolves graph-authored execution documents that are
// required and linked to any of the governing artifact IDs, then runs each one.
func evaluateRequiredGates(wtPath string, governingIDs []string) ([]GateCheckResult, bool, error) {
	if len(governingIDs) == 0 {
		return nil, false, nil
	}

	graph, err := docgraph.BuildGraphWithConfig(wtPath)
	if err != nil {
		// Soft error: skip gate evaluation rather than blocking all landings.
		return nil, false, nil
	}

	governingSet := make(map[string]bool, len(governingIDs))
	for _, id := range governingIDs {
		governingSet[id] = true
	}

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
			continue
		}
		if len(ed.Command) == 0 {
			continue
		}
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

// summarizeGates returns the RequiredExecSummary string for the landing result.
func summarizeGates(results []GateCheckResult, anyFailed bool) string {
	if len(results) == 0 {
		return "skipped"
	}
	if anyFailed {
		return "fail"
	}
	return "pass"
}
