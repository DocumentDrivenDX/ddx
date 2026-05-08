package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BuildLandingGateContext prepares the inputs the gate evaluator needs to
// score a worker's result before LandBeadResult decides merge-vs-preserve:
//
//  1. Read the worker's manifest at projectRoot/<res.ManifestFile> and
//     extract the governing-IDs list. When the manifest declares no governing
//     IDs the function returns an empty wtPath/govern-IDs pair and a no-op
//     cleanup — caller skips gate eval entirely.
//  2. Otherwise pin res.ResultRev to refs/ddx/gate-pins/<beadID>/<attemptID>
//     so the SHA stays alive while the ephemeral worktree exists.
//  3. WorktreeAdd a temp worktree (under os.TempDir() with prefix
//     "ddx-gate-wt-") at the pinned ref so gate evaluators can read the
//     result tree without disturbing the main worktree.
//
// The returned cleanup closure removes the worktree and deletes the pin ref.
// Callers MUST defer cleanup() before returning to avoid orphaned worktrees.
//
// On any error the function attempts to roll back partial state (pin+remove)
// and returns ("", nil, noop, err).
func BuildLandingGateContext(projectRoot string, res *ExecuteBeadResult, gitOps GitOps) (wtPath string, governIDs []string, cleanup func(), err error) {
	noop := func() {}

	if res == nil || res.ManifestFile == "" {
		return "", nil, noop, nil
	}
	manifestPath := filepath.Join(projectRoot, res.ManifestFile)
	if _, statErr := os.Stat(manifestPath); statErr != nil {
		// Soft-skip: missing manifest = no gate eval.
		return "", nil, noop, nil
	}
	ids := ExtractGoverningIDsFromManifest(manifestPath)
	if len(ids) == 0 {
		return "", nil, noop, nil
	}
	if res.ResultRev == "" {
		return "", nil, noop, fmt.Errorf("BuildLandingGateContext: ResultRev empty, cannot pin")
	}

	// Pin ResultRev so the worktree-add target stays alive even though the worker
	// worktree was removed. Use bead+attempt to avoid collisions across attempts.
	attemptKey := res.AttemptID
	if attemptKey == "" {
		attemptKey = NowFunc().UTC().Format("20060102T150405Z")
	}
	pinRef := fmt.Sprintf("refs/ddx/gate-pins/%s/%s", res.BeadID, attemptKey)
	if upErr := gitOps.UpdateRef(projectRoot, pinRef, res.ResultRev); upErr != nil {
		return "", nil, noop, fmt.Errorf("pin ResultRev: %w", upErr)
	}
	unpin := func() { _ = gitOps.DeleteRef(projectRoot, pinRef) }

	wt, mkErr := os.MkdirTemp("", "ddx-gate-wt-"+attemptKey+"-")
	if mkErr != nil {
		unpin()
		return "", nil, noop, fmt.Errorf("create temp worktree dir: %w", mkErr)
	}
	// os.MkdirTemp creates the directory, but `git worktree add` refuses to
	// run when the target already exists. Remove it so git can recreate it.
	_ = os.RemoveAll(wt)
	if addErr := gitOps.WorktreeAdd(projectRoot, wt, res.ResultRev); addErr != nil {
		_ = os.RemoveAll(wt)
		unpin()
		return "", nil, noop, fmt.Errorf("worktree add at %s: %w", res.ResultRev, addErr)
	}

	cleanup = func() {
		_ = gitOps.WorktreeRemove(projectRoot, wt)
		_ = os.RemoveAll(wt) // belt-and-suspenders if WorktreeRemove failed
		unpin()
	}
	return wt, ids, cleanup, nil
}

// EvaluateRequiredGatesForResult is the canonical "run gate evaluation and
// write the bundle artifacts" sequence used by LandBeadResult and by upstream
// callers (server execute-loop) that want gate eval before submitting through
// the coordinator. It delegates to evaluateRequiredGates for the actual gate
// scoring, then mutates res to set GateResults, RequiredExecSummary,
// RatchetEvidence, and RatchetSummary, and writes checks.json into the
// execution bundle. Returns (anyRequiredFailed, ratchetFailed, err).
//
// When wtPath is empty or no governing IDs are provided, gate evaluation is
// skipped and res.RequiredExecSummary is set to "skipped" (matching
// summarizeGates behavior). checksArtifactPath, when non-empty and gate
// results exist, is the absolute path checks.json is written to;
// checksArtifactRel is the bundle-relative path stored in res.ChecksFile on
// successful write.
func EvaluateRequiredGatesForResult(wtPath string, governIDs []string, res *ExecuteBeadResult, projectRoot string, checksArtifactPath, checksArtifactRel string) (anyRequiredFailed bool, ratchetFailed bool, err error) {
	if res == nil {
		return false, false, fmt.Errorf("EvaluateRequiredGatesForResult: res is nil")
	}

	var gateResults []GateCheckResult
	var anyGateFailed, anyRatchetFailed bool
	if wtPath != "" && len(governIDs) > 0 {
		var gerr error
		gateResults, anyGateFailed, anyRatchetFailed, gerr = evaluateRequiredGates(wtPath, governIDs)
		if gerr != nil {
			return false, false, fmt.Errorf("evaluating required gates: %w", gerr)
		}
	}

	res.GateResults = gateResults
	res.RequiredExecSummary = summarizeGates(gateResults, anyGateFailed)
	res.RatchetEvidence = collectRatchetEvidence(gateResults)
	res.RatchetSummary = summarizeRatchets(res.RatchetEvidence)

	// Write checks.json when gate evaluation produced results and a path was
	// provided. Best-effort: a write failure leaves res.ChecksFile empty but
	// does not abort the gate evaluation.
	if len(gateResults) > 0 && checksArtifactPath != "" {
		checks := executeBeadChecks{
			AttemptID:       res.AttemptID,
			EvaluatedAt:     time.Now().UTC(),
			Summary:         res.RequiredExecSummary,
			Results:         gateResults,
			RatchetSummary:  res.RatchetSummary,
			RatchetEvidence: res.RatchetEvidence,
		}
		if writeErr := writeArtifactJSON(checksArtifactPath, checks); writeErr == nil {
			res.ChecksFile = checksArtifactRel
		}
	}

	return anyGateFailed, anyRatchetFailed, nil
}
