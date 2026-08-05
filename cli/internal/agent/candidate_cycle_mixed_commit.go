package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/bead/accheck"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// mixedCommitNamedACCheckRunner runs only the bead's named AC checks for a
// salvaged mixed commit. It intentionally avoids the broad repository pre-merge
// gate runner so unrelated package gates do not block recovery.
type mixedCommitNamedACCheckRunner struct {
	bead      *bead.Bead
	beadStore BeadEventAppender
}

func (r *mixedCommitNamedACCheckRunner) RunChecks(ctx context.Context, _ string, candidate CandidateResult) (CandidateCheckResult, error) {
	if r == nil || r.bead == nil {
		return CandidateCheckResult{}, fmt.Errorf("mixed commit candidate checks: bead context required")
	}
	if candidate.WorktreePath == "" {
		return CandidateCheckResult{}, fmt.Errorf("mixed commit candidate checks: worktree path required")
	}

	items := accheck.ParseAcceptance(r.bead.Acceptance)
	if len(items) == 0 {
		return CandidateCheckResult{Passed: true, Detail: "mixed commit candidate has no acceptance criteria"}, nil
	}

	// Surface the worker's own non-zero exit classification before any
	// candidate checks run so transport-only exits remain advisory instead of
	// being relabeled as content failures.
	if advisory := mixedCommitAdvisoryFailure(candidate.Report); advisory != "" {
		r.appendMixedCommitAdvisoryEvent(candidate, advisory)
	}

	packages := mixedCommitCandidateCheckPackages(ctx, candidate.WorktreePath, candidate.Report.BaseRev, candidate.Report.ResultRev, items)
	if len(packages) == 0 {
		packages = nil
	}

	artifactsDir := filepath.Join(candidate.WorktreePath, ExecuteBeadArtifactDir, candidate.Report.AttemptID)
	acCheckPath := filepath.Join(artifactsDir, "ac-check.json")
	if err := os.MkdirAll(filepath.Dir(acCheckPath), 0o755); err != nil {
		return CandidateCheckResult{}, fmt.Errorf("mixed commit candidate checks: create ac-check dir: %w", err)
	}

	runTest := accheck.DefaultRunTest(candidate.WorktreePath)
	entries := accheck.Evaluate(items, accheck.Context{
		WorkingDir: candidate.WorktreePath,
		RevBase:    candidate.Report.BaseRev,
		Packages:   packages,
		RunTest:    runTest,
		GitGrep:    accheck.DefaultGitGrep(candidate.WorktreePath),
		DiffHits:   accheck.DefaultDiffHits(candidate.WorktreePath, candidate.Report.BaseRev),
	})
	out := accheck.Aggregate(r.bead.ID, candidate.Report.AttemptID, entries)
	if err := writeMixedCommitACCheckJSON(acCheckPath, out); err != nil {
		return CandidateCheckResult{}, err
	}

	// Test-name and symbol/negative ACs are the actual content checks. Build
	// gates, command ACs, and mechanical/file-path items are ratified by the
	// reviewer and therefore should not fail the candidate cycle on their own.
	var failures []string
	var transportOnlyErrors []string
	for _, entry := range out.Items {
		switch entry.Result {
		case accheck.ResultFail:
			failures = append(failures, fmt.Sprintf("AC #%d %s", entry.AC, strings.TrimSpace(entry.Evidence)))
		case accheck.ResultError:
			if mixedCommitCheckErrorIsAdvisory(entry.Evidence) {
				transportOnlyErrors = append(transportOnlyErrors, fmt.Sprintf("AC #%d %s", entry.AC, strings.TrimSpace(entry.Evidence)))
				continue
			}
			failures = append(failures, fmt.Sprintf("AC #%d %s", entry.AC, strings.TrimSpace(entry.Evidence)))
		}
	}

	switch {
	case len(failures) > 0:
		return CandidateCheckResult{
			Passed: false,
			Detail: "named AC checks failed: " + strings.Join(failures, "; "),
			Artifacts: []string{
				filepath.ToSlash(acCheckPath),
			},
		}, nil
	case len(transportOnlyErrors) > 0 || mixedCommitAdvisoryFailure(candidate.Report) != "":
		detail := "named AC checks continued after advisory transport-only failure"
		if len(transportOnlyErrors) > 0 {
			detail += ": " + strings.Join(transportOnlyErrors, "; ")
		}
		return CandidateCheckResult{
			Passed: true,
			Detail: detail,
			Artifacts: []string{
				filepath.ToSlash(acCheckPath),
			},
		}, nil
	default:
		return CandidateCheckResult{
			Passed: true,
			Detail: "named AC checks passed",
			Artifacts: []string{
				filepath.ToSlash(acCheckPath),
			},
		}, nil
	}
}

func (r *mixedCommitNamedACCheckRunner) appendMixedCommitAdvisoryEvent(candidate CandidateResult, advisory string) {
	if r == nil || r.beadStore == nil || candidate.Report.BeadID == "" {
		return
	}
	appendWorkEvent(
		r.beadStore,
		candidate.Report.BeadID,
		"operator_attention",
		"mixed-commit advisory",
		map[string]any{
			"reason":        advisory,
			"candidate_ref": candidate.Report.CandidateRef,
			"base_rev":      candidate.Report.BaseRev,
			"result_rev":    candidate.Report.ResultRev,
			"failure_mode":  candidate.Report.OutcomeReason,
			"detail":        candidate.Report.Detail,
			"error":         candidate.Report.Error,
		},
		coalesceWorkerID(candidate.Report.WorkerID),
		time.Now().UTC(),
	)
}

func mixedCommitAdvisoryFailure(report ExecuteBeadReport) string {
	mode := strings.TrimSpace(report.OutcomeReason)
	switch mode {
	case FailureModeTimeout,
		FailureModeAuthError,
		FailureModeNoViableProvider,
		FailureModeProviderConnectivity,
		FailureModeServerUnavailable,
		FailureModeHarnessNotInstalled,
		FailureModeBlockedByPassthroughConstraint,
		FailureModeAgentPowerUnsatisfied,
		FailureModeLockContention,
		FailureModeWorktreeLost,
		FailureModeRouteResolutionTimeout,
		FailureModeProgressWatchdog,
		FailureModeAttemptWallClockTimeout,
		FailureModeConsecutiveWedge,
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency:
		return mode
	}
	if mode == "" {
		combined := strings.TrimSpace(strings.Join([]string{report.Detail, report.Error, report.Stderr}, "\n"))
		if combined == "" {
			return ""
		}
		mode = ClassifyFailureMode(ExecuteBeadOutcomeTaskFailed, 1, combined)
		switch mode {
		case FailureModeTimeout,
			FailureModeAuthError,
			FailureModeNoViableProvider,
			FailureModeProviderConnectivity,
			FailureModeServerUnavailable,
			FailureModeHarnessNotInstalled,
			FailureModeBlockedByPassthroughConstraint,
			FailureModeAgentPowerUnsatisfied,
			FailureModeLockContention,
			FailureModeWorktreeLost,
			FailureModeRouteResolutionTimeout,
			FailureModeProgressWatchdog,
			FailureModeAttemptWallClockTimeout,
			FailureModeConsecutiveWedge,
			ReadinessSystemReasonResourceExhausted,
			ReadinessSystemReasonRepoConcurrency:
			return mode
		}
	}
	return ""
}

func mixedCommitCheckErrorIsAdvisory(evidence string) bool {
	mode := ClassifyFailureMode(ExecuteBeadOutcomeTaskFailed, 1, evidence)
	switch mode {
	case FailureModeTimeout,
		FailureModeAuthError,
		FailureModeNoViableProvider,
		FailureModeProviderConnectivity,
		FailureModeServerUnavailable,
		FailureModeHarnessNotInstalled,
		FailureModeBlockedByPassthroughConstraint,
		FailureModeAgentPowerUnsatisfied,
		FailureModeLockContention,
		FailureModeWorktreeLost,
		FailureModeRouteResolutionTimeout,
		FailureModeProgressWatchdog,
		FailureModeAttemptWallClockTimeout,
		FailureModeConsecutiveWedge,
		ReadinessSystemReasonResourceExhausted,
		ReadinessSystemReasonRepoConcurrency:
		return true
	default:
		return false
	}
}

func mixedCommitCandidateCheckPackages(ctx context.Context, worktreePath, baseRev, resultRev string, items []accheck.Item) []string {
	packages := map[string]struct{}{}
	for _, item := range items {
		if item.Kind != accheck.KindTestName || strings.TrimSpace(item.Name) == "" {
			continue
		}
		matches := gitGrepCandidateTestFiles(ctx, worktreePath, item.Name)
		for _, match := range matches {
			if pkg := candidatePackagePatternFromPath(match); pkg != "" {
				packages[pkg] = struct{}{}
			}
		}
	}
	if len(packages) == 0 {
		for _, pkg := range changedCandidatePackages(ctx, worktreePath, baseRev, resultRev) {
			packages[pkg] = struct{}{}
		}
	}
	if len(packages) == 0 {
		return nil
	}
	out := make([]string, 0, len(packages))
	for pkg := range packages {
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

func gitGrepCandidateTestFiles(ctx context.Context, worktreePath, testName string) []string {
	if strings.TrimSpace(worktreePath) == "" || strings.TrimSpace(testName) == "" {
		return nil
	}
	out, err := internalgit.Command(ctx, worktreePath, "grep", "-l", "-F", "-e", "func "+testName, "--", "cli").CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) == "" {
		return nil
	}
	var matches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			matches = append(matches, line)
		}
	}
	return matches
}

func changedCandidatePackages(ctx context.Context, worktreePath, baseRev, resultRev string) []string {
	if strings.TrimSpace(worktreePath) == "" || strings.TrimSpace(baseRev) == "" || strings.TrimSpace(resultRev) == "" || baseRev == resultRev {
		return nil
	}
	out, err := internalgit.Command(ctx, worktreePath, "diff", "--name-only", baseRev, resultRev, "--", "cli").CombinedOutput()
	if err != nil && strings.TrimSpace(string(out)) == "" {
		return nil
	}
	packages := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(line, ".go") {
			continue
		}
		if pkg := candidatePackagePatternFromPath(line); pkg != "" {
			packages[pkg] = struct{}{}
		}
	}
	if len(packages) == 0 {
		return nil
	}
	outPackages := make([]string, 0, len(packages))
	for pkg := range packages {
		outPackages = append(outPackages, pkg)
	}
	sort.Strings(outPackages)
	return outPackages
}

func candidatePackagePatternFromPath(path string) string {
	trimmed := strings.TrimSpace(filepath.ToSlash(path))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "cli/")
	if trimmed == "" {
		return "./..."
	}
	dir := filepath.Dir(trimmed)
	if dir == "." || dir == "" {
		return "./..."
	}
	return "./" + dir + "/..."
}

func writeMixedCommitACCheckJSON(path string, out accheck.Output) error {
	blob, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("mixed commit candidate checks: encode ac-check.json: %w", err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		return fmt.Errorf("mixed commit candidate checks: write ac-check.json: %w", err)
	}
	return nil
}

func coalesceWorkerID(workerID string) string {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return "ddx work"
	}
	return workerID
}
