package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
	agentlib "github.com/easel/fizeau"
)

// repositoryCandidateCheckRunner evaluates both governing execution gates and
// project pre-merge checks in the live attempt worktree. The coordinator runs
// this before every reviewer pass, including after a repair commit.
type repositoryCandidateCheckRunner struct {
	bead      *bead.Bead
	result    *ExecuteBeadResult
	artifacts *executeBeadArtifacts
}

func (r *repositoryCandidateCheckRunner) RunChecks(ctx context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
	if r == nil || r.bead == nil || r.result == nil || r.artifacts == nil {
		return CandidateCheckResult{}, fmt.Errorf("candidate checks: production runner is not fully configured")
	}
	worktree := strings.TrimSpace(candidate.WorktreePath)
	if worktree == "" {
		return CandidateCheckResult{}, fmt.Errorf("candidate checks: worktree path is required")
	}

	refs := ResolveGoverningRefs(worktree, r.bead)
	governingIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if id := strings.TrimSpace(ref.ID); id != "" {
			governingIDs = append(governingIDs, id)
		}
	}
	gateFailed, ratchetFailed, err := EvaluateRequiredGatesForResult(
		worktree,
		governingIDs,
		r.result,
		worktree,
		r.artifacts.ChecksAbs,
		r.artifacts.ChecksRel,
	)
	if err != nil {
		return CandidateCheckResult{}, err
	}

	preMerge, err := RunPreMergeChecks(
		ctx,
		worktree,
		r.bead,
		candidate.Report.BaseRev,
		candidate.Report.ResultRev,
		r.artifacts.DirAbs,
	)
	if err != nil {
		return CandidateCheckResult{}, err
	}

	evidence := struct {
		RequiredSummary string                 `json:"required_summary,omitempty"`
		Required        []GateCheckResult      `json:"required,omitempty"`
		RatchetFailed   bool                   `json:"ratchet_failed,omitempty"`
		PreMerge        *PreMergeChecksOutcome `json:"pre_merge,omitempty"`
	}{
		RequiredSummary: r.result.RequiredExecSummary,
		Required:        append([]GateCheckResult(nil), r.result.GateResults...),
		RatchetFailed:   ratchetFailed,
		PreMerge:        preMerge,
	}
	detailBytes, marshalErr := json.MarshalIndent(evidence, "", "  ")
	if marshalErr != nil {
		return CandidateCheckResult{}, fmt.Errorf("candidate checks: encode failure evidence: %w", marshalErr)
	}
	artifacts := make([]string, 0, 2)
	if len(r.result.GateResults) > 0 && r.result.ChecksFile != "" {
		artifacts = append(artifacts, r.result.ChecksFile)
	}
	if preMerge != nil && preMerge.EvidenceDir != "" {
		artifacts = append(artifacts, filepath.ToSlash(preMerge.EvidenceDir))
	}
	return CandidateCheckResult{
		Passed:    !gateFailed && !ratchetFailed && (preMerge == nil || !preMerge.Blocked),
		Detail:    string(detailBytes),
		Artifacts: artifacts,
	}, nil
}

// fizeauCandidateRepairPass performs one fresh public Fizeau Execute against
// the same live attempt worktree. It copies the primary runtime/config
// envelope, changes only repair-specific prompt/correlation/role intent, and
// proves that the repair added exactly one descendant commit.
type fizeauCandidateRepairPass struct {
	projectRoot string
	workspace   *AttemptWorkspace
	backend     AttemptBackend
	service     agentlib.FizeauService
	config      config.ResolvedConfig
	runtime     AgentRunRuntime
	gitOps      GitOps
	artifacts   *executeBeadArtifacts
}

func (r *fizeauCandidateRepairPass) Repair(ctx context.Context, candidate CandidateResult, prompt string) (CandidateResult, error) {
	if r == nil || r.backend == nil || r.workspace == nil || r.artifacts == nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair is not fully configured")
	}
	gitOps := r.gitOps
	if gitOps == nil {
		gitOps = &RealGitOps{}
	}
	failedRev := strings.TrimSpace(candidate.Report.ResultRev)
	if failedRev == "" {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair requires failed candidate revision")
	}
	before, err := gitOps.HeadRev(candidate.WorktreePath)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: resolve candidate head: %w", err)
	}
	if before != failedRev {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: candidate head %s does not match failed revision %s", before, failedRev)
	}

	cycleIndex := candidate.CycleIndex + 1
	promptPath := filepath.Join(r.artifacts.DirAbs, fmt.Sprintf("repair-%d.md", cycleIndex))
	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: write prompt: %w", err)
	}
	runtime := r.runtime
	runtime.Prompt = ""
	runtime.PromptFile = promptPath
	runtime.PromptSource = filepath.ToSlash(promptPath)
	runtime.WorkDir = candidate.WorktreePath
	runtime.WorkLogPhase = "repair"
	runtime.Role = config.EvidenceRoleImplementer
	runtime.CorrelationID = fmt.Sprintf("%s:repair:%d", strings.TrimSpace(r.runtime.CorrelationID), cycleIndex)
	runtime.Correlation = cloneRepairStringMap(r.runtime.Correlation)
	runtime.Correlation["phase"] = "repair"
	runtime.Correlation["repair_cycle"] = strconv.Itoa(cycleIndex)
	runtime.Correlation["failed_candidate_rev"] = failedRev
	runtime.Env = cloneRepairStringMap(r.runtime.Env)

	result, err := r.backend.Run(ctx, AttemptBackendRunRequest{
		ProjectRoot: r.projectRoot,
		Workspace:   r.workspace,
		Service:     r.service,
		AgentRunner: nil, // repairs must cross the public Fizeau Execute boundary
		Config:      r.config,
		Runtime:     runtime,
	})
	if err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair Execute: %w", err)
	}
	if result == nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair Execute returned no result")
	}
	if result.ExitCode != 0 {
		failure := strings.TrimSpace(firstNonEmpty(result.Error, result.Stderr, result.Output))
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair Execute failed with exit code %d: %s", result.ExitCode, failure)
	}

	after, err := gitOps.HeadRev(candidate.WorktreePath)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: resolve repaired head: %w", err)
	}
	dirty, err := gitOps.IsDirty(candidate.WorktreePath)
	if err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: inspect worktree: %w", err)
	}
	if after == failedRev {
		if !dirty {
			return CandidateResult{}, fmt.Errorf("fresh Fizeau repair produced no commit or repairable changes")
		}
		committed, synthErr := gitOps.SynthesizeCommit(candidate.WorktreePath, fmt.Sprintf("fix: repair close-gate findings [%s]", candidate.Report.BeadID))
		if synthErr != nil {
			return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: synthesize repair commit: %w", synthErr)
		}
		if !committed {
			return CandidateResult{}, fmt.Errorf("fresh Fizeau repair produced no committable changes")
		}
		after, err = gitOps.HeadRev(candidate.WorktreePath)
		if err != nil {
			return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: resolve synthesized head: %w", err)
		}
	} else if dirty {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair left uncommitted changes after committing")
	}

	if err := internalgit.Command(ctx, candidate.WorktreePath, "merge-base", "--is-ancestor", failedRev, after).Run(); err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair rewrote candidate history: %w", err)
	}
	countOut, err := internalgit.Command(ctx, candidate.WorktreePath, "rev-list", "--count", failedRev+".."+after).Output()
	if err != nil {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair: count appended commits: %w", err)
	}
	if strings.TrimSpace(string(countOut)) != "1" {
		return CandidateResult{}, fmt.Errorf("fresh Fizeau repair must append exactly one commit; got %s", strings.TrimSpace(string(countOut)))
	}

	repaired := candidate
	repaired.CycleIndex = cycleIndex
	repaired.Report.ResultRev = after
	repaired.Report.ImplementationRev = after
	repaired.Report.CycleIndex = cycleIndex
	repaired.Report.Status = ExecuteBeadStatusSuccess
	repaired.Report.Detail = "fresh Fizeau repair committed"
	repaired.Report.Error = ""
	repaired.Report.Harness = result.Harness
	repaired.Report.Provider = result.Provider
	repaired.Report.Model = result.Model
	repaired.Report.ActualPower = result.ActualPower
	repaired.Report.PredictedPower = result.PredictedPower
	repaired.Report.PredictedSpeedTPS = result.PredictedSpeedTPS
	repaired.Report.PredictedCostUSDPer1kTokens = result.PredictedCostUSDPer1kTokens
	repaired.Report.PredictedCostSource = result.PredictedCostSource
	repaired.Report.CostUSD += result.CostUSD
	repaired.Report.DurationMS += int64(result.DurationMS)
	return repaired, nil
}

func cloneRepairStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+3)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func repairOriginalTask(b *bead.Bead) string {
	if b == nil {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "bead: %s\ntitle: %s\n", strings.TrimSpace(b.ID), strings.TrimSpace(b.Title))
	if description := strings.TrimSpace(b.Description); description != "" {
		fmt.Fprintf(&out, "description:\n%s\n", description)
	}
	if acceptance := strings.TrimSpace(b.Acceptance); acceptance != "" {
		fmt.Fprintf(&out, "acceptance:\n%s\n", acceptance)
	}
	if notes := strings.TrimSpace(b.Notes); notes != "" {
		fmt.Fprintf(&out, "notes:\n%s\n", notes)
	}
	return strings.TrimSpace(out.String())
}
