package agent

// execute_bead_dangling_success.go — detection and recovery for
// dangling-success beads (ddx-2b2d114e, ddx-f1e7dfdf).
//
// A "dangling success" is an in_progress bead whose most recent execution
// produced outcome=task_succeeded and a non-empty result_rev, but the
// bead-close step (CloseWithEvidence) never ran — typically because the
// process crashed between Land() and the store write.
//
// Two sub-cases:
//   A. result_rev IS reachable from HEAD (merge succeeded, close didn't).
//      Recovery: call CloseWithEvidence directly; no re-execution needed.
//   B. result_rev is NOT reachable from HEAD (crash during Land / dangling
//      commit). Recovery: preserve the successful commit under
//      refs/ddx/iterations/... when the object still exists locally, or park
//      the bead for operator attention when DDx cannot recover the commit.

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

type danglingSuccessOutcome string

const (
	danglingSuccessOutcomeClosed           danglingSuccessOutcome = "closed"
	danglingSuccessOutcomePreserved        danglingSuccessOutcome = "preserved"
	danglingSuccessOutcomeOperatorRequired danglingSuccessOutcome = "operator_required"
)

// DanglingSuccessRecovery is the terminal decision for a previously successful
// attempt. Any non-nil result means the caller must skip a fresh execution.
type DanglingSuccessRecovery struct {
	Outcome       danglingSuccessOutcome
	AttemptID     string
	BaseRev       string
	ResultRev     string
	PreserveRef   string
	FailureReason string
	SessionID     string
}

// latestTaskSucceededResult scans .ddx/executions/ for the most recent
// result.json that belongs to beadID and has outcome=task_succeeded with a
// non-empty result_rev. Returns nil when no such entry is found.
//
// The scan is intentionally cheap (directory listing + JSON decode per dir);
// it only runs when the work loop has already claimed a bead and is checking
// whether a prior successful attempt must be reconciled before retry.
func latestTaskSucceededResult(projectRoot, beadID string) *ExecuteBeadResult {
	execDir := executeBeadArtifactRoot(projectRoot)
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

func isCommitObjectPresent(projectRoot, rev string) bool {
	if rev == "" || projectRoot == "" {
		return false
	}
	err := internalgit.Command(context.Background(), projectRoot, "cat-file", "-e", rev+"^{commit}").Run()
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
	execDir := executeBeadArtifactRoot(projectRoot)

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

func appendDanglingSuccessEvent(store ExecuteBeadLoopStore, beadID string, event bead.BeadEvent) {
	if store == nil || beadID == "" {
		return
	}
	_ = store.AppendEvent(beadID, event)
}

func parkDanglingSuccessForOperator(store ExecuteBeadLoopStore, beadID, reason, suggestedAction, summary string, at time.Time) error {
	return store.ParkToProposed(beadID, bead.ParkAutoRecoveryFailed, func(b *bead.Bead) {
		bead.SetNeedsHumanMeta(b, bead.NeedsHumanMeta{
			Reason:          reason,
			Since:           at.UTC().Format(time.RFC3339),
			Source:          "ddx work",
			SuggestedAction: suggestedAction,
			Summary:         summary,
		})
	})
}

// recoverDanglingSuccess reconciles the latest task_succeeded result for a
// just-claimed bead before the loop dispatches another agent attempt.
func recoverDanglingSuccess(
	store ExecuteBeadLoopStore,
	projectRoot string,
	beadID string,
	assignee string,
	nowFn func() time.Time,
	emit func(string, map[string]any),
) (*DanglingSuccessRecovery, error) {
	if projectRoot == "" {
		return nil, nil
	}

	prior := latestTaskSucceededResult(projectRoot, beadID)
	if prior == nil {
		return nil, nil
	}

	t := time.Now().UTC()
	if nowFn != nil {
		t = nowFn().UTC()
	}

	reachable := isRevReachableFromHead(projectRoot, prior.ResultRev)
	if reachable {
		if emit != nil {
			emit("bead.dangling_success_recovery", map[string]any{
				"bead_id":    beadID,
				"attempt_id": prior.AttemptID,
				"result_rev": prior.ResultRev,
				"reachable":  true,
			})
		}

		appendDanglingSuccessEvent(store, beadID, bead.BeadEvent{
			Kind:      "dangling-success-recovery",
			Summary:   "recovered dangling success: closing bead after merged result_rev detected",
			Body:      fmt.Sprintf("attempt_id=%s\nresult_rev=%s\naction=idempotent_close", prior.AttemptID, prior.ResultRev),
			Actor:     assignee,
			Source:    "ddx work",
			CreatedAt: t,
		})

		if cerr := store.CloseWithEvidence(beadID, prior.SessionID, prior.ResultRev); cerr != nil {
			return nil, fmt.Errorf("dangling-success recovery: CloseWithEvidence(%s): %w", beadID, cerr)
		}
		return &DanglingSuccessRecovery{
			Outcome:   danglingSuccessOutcomeClosed,
			AttemptID: prior.AttemptID,
			BaseRev:   prior.BaseRev,
			ResultRev: prior.ResultRev,
			SessionID: prior.SessionID,
		}, nil
	}

	payload := map[string]any{
		"bead_id":    beadID,
		"attempt_id": prior.AttemptID,
		"result_rev": prior.ResultRev,
		"reachable":  false,
	}

	if isCommitObjectPresent(projectRoot, prior.ResultRev) {
		ref := PreserveRef(beadID, prior.BaseRev)
		if err := (&RealGitOps{}).UpdateRef(projectRoot, ref, prior.ResultRev); err == nil {
			payload["preserve_ref"] = ref
			payload["action"] = "preserve_and_park"
			if emit != nil {
				emit("bead.dangling_success_preserved", payload)
			}
			appendDanglingSuccessEvent(store, beadID, bead.BeadEvent{
				Kind:      "dangling-success-preserved",
				Summary:   "successful detached result preserved for operator landing",
				Body:      fmt.Sprintf("attempt_id=%s\nresult_rev=%s\npreserve_ref=%s", prior.AttemptID, prior.ResultRev, ref),
				Actor:     assignee,
				Source:    "ddx work",
				CreatedAt: t,
			})
			if err := parkDanglingSuccessForOperator(
				store,
				beadID,
				"successful result commit was preserved for manual landing",
				fmt.Sprintf("inspect %s and land or close the bead manually", ref),
				"successful detached result preserved for operator landing",
				t,
			); err != nil {
				return nil, fmt.Errorf("dangling-success recovery: ParkToProposed(%s): %w", beadID, err)
			}
			return &DanglingSuccessRecovery{
				Outcome:     danglingSuccessOutcomePreserved,
				AttemptID:   prior.AttemptID,
				BaseRev:     prior.BaseRev,
				ResultRev:   prior.ResultRev,
				PreserveRef: ref,
				SessionID:   prior.SessionID,
			}, nil
		} else {
			payload["failure_reason"] = fmt.Sprintf("failed to preserve successful result_rev under durable ref: %v", err)
		}
	} else {
		payload["failure_reason"] = "successful result_rev is not present in the local git object database"
	}

	failureReason, _ := payload["failure_reason"].(string)
	if emit != nil {
		emit("bead.dangling_success_operator_required", payload)
	}
	appendDanglingSuccessEvent(store, beadID, bead.BeadEvent{
		Kind:      "dangling-success-operator-required",
		Summary:   "successful detached result could not be landed automatically",
		Body:      fmt.Sprintf("attempt_id=%s\nresult_rev=%s\nfailure_reason=%s", prior.AttemptID, prior.ResultRev, failureReason),
		Actor:     assignee,
		Source:    "ddx work",
		CreatedAt: t,
	})
	if err := parkDanglingSuccessForOperator(
		store,
		beadID,
		"successful result commit could not be recovered automatically",
		fmt.Sprintf("recover or reconstruct result_rev %s from execution evidence before rerunning the bead", prior.ResultRev),
		"successful detached result requires operator recovery",
		t,
	); err != nil {
		return nil, fmt.Errorf("dangling-success recovery: ParkToProposed(%s): %w", beadID, err)
	}
	return &DanglingSuccessRecovery{
		Outcome:       danglingSuccessOutcomeOperatorRequired,
		AttemptID:     prior.AttemptID,
		BaseRev:       prior.BaseRev,
		ResultRev:     prior.ResultRev,
		FailureReason: failureReason,
		SessionID:     prior.SessionID,
	}, nil
}
