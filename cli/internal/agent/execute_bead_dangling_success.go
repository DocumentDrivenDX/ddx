package agent

// execute_bead_dangling_success.go — detection and idempotent recovery for
// dangling-success beads (ddx-2b2d114e).
//
// A "dangling success" is an in_progress bead whose most recent execution
// produced outcome=task_succeeded and a non-empty result_rev, but the
// bead-close step (CloseWithEvidence) never ran — typically because the
// process crashed between Land() and the store write.
//
// Two sub-cases:
//   A. result_rev IS reachable from HEAD (merge succeeded, close didn't).
//      Recovery: call CloseWithEvidence directly; no re-execution needed.
//   B. result_rev is NOT reachable from HEAD (crash during Land / dangling commit).
//      Recovery: surface the state via DetectDanglingSuccessBeads so
//      operators can manually resolve or retry.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
)

// DanglingSuccessFinding describes a bead whose last successful execution was
// not finalized. Returned by DetectDanglingSuccessBeads.
type DanglingSuccessFinding struct {
	// BeadID is the bead that stalled.
	BeadID string `json:"bead_id"`
	// AttemptID is the attempt whose result.json shows task_succeeded.
	AttemptID string `json:"attempt_id"`
	// ResultRev is the commit SHA the agent produced.
	ResultRev string `json:"result_rev"`
	// Reachable is true when ResultRev is already merged into the target branch.
	// When true the bead can be closed idempotently.
	// When false the merge itself was lost and manual intervention is required.
	Reachable bool `json:"reachable"`
	// ResultFile is the absolute path to the result.json artifact.
	ResultFile string `json:"result_file"`
}

// latestTaskSucceededResult scans .ddx/executions/ for the most recent
// result.json that belongs to beadID and has outcome=task_succeeded with a
// non-empty result_rev. Returns nil when no such entry is found.
//
// The scan is intentionally cheap (directory listing + JSON decode per dir);
// it only runs when a stale in_progress claim is detected, which is rare.
func latestTaskSucceededResult(projectRoot, beadID string) *ExecuteBeadResult {
	execDir := filepath.Join(projectRoot, ExecuteBeadArtifactDir)
	entries, err := os.ReadDir(execDir)
	if err != nil {
		return nil
	}

	// Attempt IDs are formatted as 20060102T150405-<hex>, so lexicographic
	// descending order gives us newest first.
	// We want the newest entry that matches beadID and task_succeeded.
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		resultPath := filepath.Join(execDir, entry.Name(), "result.json")
		data, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res ExecuteBeadResult
		if err := json.Unmarshal(data, &res); err != nil {
			continue
		}
		if res.BeadID != beadID {
			continue
		}
		if res.Outcome != ExecuteBeadOutcomeTaskSucceeded {
			continue
		}
		if res.ResultRev == "" || res.ResultRev == res.BaseRev {
			continue
		}
		// Found a valid prior success.
		return &res
	}
	return nil
}

// isRevReachableFromHead returns true when rev is reachable from the HEAD of
// the given directory (i.e., `git merge-base --is-ancestor rev HEAD` exits 0).
// Returns false on any error.
func isRevReachableFromHead(projectRoot, rev string) bool {
	if rev == "" || projectRoot == "" {
		return false
	}
	err := internalgit.Command(context.Background(), projectRoot, "merge-base", "--is-ancestor", rev, "HEAD").Run()
	return err == nil
}

// DetectDanglingSuccessBeads scans all in_progress beads in the store and
// returns findings for any that have a prior task_succeeded result. Called by
// ddx bead doctor --dangling to surface operator-actionable cases.
//
// For each in_progress bead:
//   - scan .ddx/executions/ for a task_succeeded result.json
//   - check whether result_rev is reachable from HEAD
//   - report a DanglingSuccessFinding with Reachable=true/false
func DetectDanglingSuccessBeads(projectRoot string) ([]DanglingSuccessFinding, error) {
	execDir := filepath.Join(projectRoot, ExecuteBeadArtifactDir)

	entries, err := os.ReadDir(execDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading executions dir: %w", err)
	}

	// Index all task_succeeded results by bead_id → newest entry.
	type candidateEntry struct {
		res        ExecuteBeadResult
		resultFile string
	}
	byBead := make(map[string]candidateEntry)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		resultPath := filepath.Join(execDir, entry.Name(), "result.json")
		data, err := os.ReadFile(resultPath)
		if err != nil {
			continue
		}
		var res ExecuteBeadResult
		if err := json.Unmarshal(data, &res); err != nil {
			continue
		}
		if res.Outcome != ExecuteBeadOutcomeTaskSucceeded {
			continue
		}
		if res.ResultRev == "" || res.ResultRev == res.BaseRev {
			continue
		}
		if _, seen := byBead[res.BeadID]; !seen {
			byBead[res.BeadID] = candidateEntry{res: res, resultFile: resultPath}
		}
	}
	if len(byBead) == 0 {
		return nil, nil
	}

	// Read the tracker to find in_progress beads.
	beadsPath := ddxroot.JoinProject(projectRoot, "beads.jsonl")
	raw, err := os.ReadFile(beadsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading beads.jsonl: %w", err)
	}

	var findings []DanglingSuccessFinding
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var b struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			continue
		}
		if b.Status != "in_progress" {
			continue
		}
		cand, ok := byBead[b.ID]
		if !ok {
			continue
		}
		reachable := isRevReachableFromHead(projectRoot, cand.res.ResultRev)
		findings = append(findings, DanglingSuccessFinding{
			BeadID:     b.ID,
			AttemptID:  cand.res.AttemptID,
			ResultRev:  cand.res.ResultRev,
			Reachable:  reachable,
			ResultFile: cand.resultFile,
		})
	}
	return findings, nil
}

// recoverDanglingSuccess checks whether the just-claimed bead (identified by
// candidateWasInProgress=true) has a prior task_succeeded result whose
// result_rev is already reachable from HEAD. If so, it closes the bead
// idempotently and returns (true, nil) — the caller must skip execution.
//
// If result_rev is NOT reachable (dangling commit, Instance A), returns
// (false, nil) and emits a diagnostic event so the operator can investigate
// via `ddx bead doctor --dangling`.
//
// The function is a no-op (returns false, nil) when candidateWasInProgress is
// false or when no prior success is found.
func recoverDanglingSuccess(
	store ExecuteBeadLoopStore,
	projectRoot string,
	beadID string,
	candidateWasInProgress bool,
	assignee string,
	nowFn func() time.Time,
	emit func(string, map[string]any),
) (recovered bool, err error) {
	if !candidateWasInProgress || projectRoot == "" {
		return false, nil
	}

	prior := latestTaskSucceededResult(projectRoot, beadID)
	if prior == nil {
		return false, nil
	}

	reachable := isRevReachableFromHead(projectRoot, prior.ResultRev)
	if !reachable {
		// Instance A: dangling commit — the merge was never applied.
		// Surface a diagnostic event but don't block execution (the caller
		// will proceed with a fresh attempt, which is the safe fallback).
		if emit != nil {
			emit("bead.dangling_success_unmerged", map[string]any{
				"bead_id":    beadID,
				"attempt_id": prior.AttemptID,
				"result_rev": prior.ResultRev,
				"reachable":  false,
				"action":     "run_ddx_bead_doctor_dangling_to_recover",
			})
		}
		return false, nil
	}

	// Instance B: merge succeeded, close didn't — recover idempotently.
	if emit != nil {
		emit("bead.dangling_success_recovery", map[string]any{
			"bead_id":    beadID,
			"attempt_id": prior.AttemptID,
			"result_rev": prior.ResultRev,
		})
	}

	// Append a recovery event so the audit trail shows what happened.
	t := time.Now().UTC()
	if nowFn != nil {
		t = nowFn().UTC()
	}
	_ = store.AppendEvent(beadID, bead.BeadEvent{
		Kind:      "dangling-success-recovery",
		Summary:   "recovered dangling success: closing bead after merged result_rev detected",
		Body:      fmt.Sprintf("result_rev=%s\nattempt_id=%s\naction=idempotent_close", prior.ResultRev, prior.AttemptID),
		Actor:     assignee,
		Source:    "ddx work",
		CreatedAt: t,
	})

	if cerr := store.CloseWithEvidence(beadID, prior.SessionID, prior.ResultRev); cerr != nil {
		return false, fmt.Errorf("dangling-success recovery: CloseWithEvidence(%s): %w", beadID, cerr)
	}
	return true, nil
}
