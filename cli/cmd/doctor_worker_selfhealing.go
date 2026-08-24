package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/server"
)

// runtimeJSONLMergeUnionPattern is the .gitattributes line that puts tracked
// append-only runtime metrics JSONL (.ddx/metrics/attempts.jsonl,
// .ddx/metrics/locks.jsonl) under the union merge driver (ddx-2520a267).
// Without it, concurrent worker appends to those files hit ordinary
// line-conflict merges instead of resolving cleanly.
const runtimeJSONLMergeUnionPattern = ".ddx/metrics/*.jsonl merge=union"

// checkWorkerSelfHealing reports the final worker self-healing invariants
// from the self-healing-workers plan on doctor's one diagnostic surface:
// stale terminal suppression risk, PID/PGID worker-liveness evidence
// mismatch, runtime JSONL merge/lock coverage, unresolved preserved-review
// gate, and resource pressure. It is read-only in both dry-run and --apply
// mode: it never starts, stops, or reclaims workers or beads (ddx-188ca92a
// NON-SCOPE).
func checkWorkerSelfHealing(projectRoot string) []DiagnosticIssue {
	if projectRoot == "" {
		return nil
	}

	var issues []DiagnosticIssue
	now := time.Now().UTC()

	manager := server.NewWorkerManager(projectRoot)
	supervisor := server.NewWorkerSupervisor(manager)

	livenessMismatches, staleBlocks, err := supervisor.DiagnoseWorkerSelfHealing(now)
	if err != nil {
		issues = append(issues, DiagnosticIssue{
			Type:        "worker_self_healing_scan",
			Description: fmt.Sprintf("Worker self-healing scan failed: %v", err),
			Remediation: []string{"Inspect .ddx/workers manually for corrupt worker records"},
		})
	} else {
		for _, block := range staleBlocks {
			issues = append(issues, DiagnosticIssue{
				Type: "worker_stale_terminal_block",
				Description: fmt.Sprintf(
					"Stale restart-blocked terminal: worker %s blocked (%s) since %s, age %s exceeds the %s block TTL",
					block.WorkerID, block.Reason, block.TerminalAt.Format(time.RFC3339),
					block.Age.Round(time.Second), server.DefaultTerminalBlockTTL,
				),
				Remediation: []string{
					"Run 'ddx worker status' to confirm no supervisor is currently reconciling this project",
					"Restart the worker supervisor (e.g. 'ddx work') so the stale block expires and restarts resume",
				},
			})
		}
		for _, mismatch := range livenessMismatches {
			issues = append(issues, DiagnosticIssue{
				Type: "worker_liveness_mismatch",
				Description: fmt.Sprintf(
					"Worker liveness evidence mismatch: worker %s (state=%s, pid=%d) has a recorded PID but its PGID, liveness sidecar, or run-state evidence says it is not live",
					mismatch.WorkerID, mismatch.State, mismatch.PID,
				),
				Remediation: []string{
					"Run 'ddx doctor --unjam' or restart the worker supervisor to reconcile the stale record",
				},
			})
		}
	}

	if desired, loadErr := supervisor.LoadDesiredState(); loadErr == nil {
		if presence, presenceErr := supervisor.DiagnoseDesiredWorkerPresence(desired, now); presenceErr == nil && presence.FDExhaustionDiagnosis != "" {
			issues = append(issues, DiagnosticIssue{
				Type: "worker_resource_pressure",
				Description: fmt.Sprintf(
					"Resource pressure: %d of %d desired worker(s) missing; newest terminal worker %s diagnosed %s",
					presence.MissingCount, presence.DesiredCount, presence.LastTerminalWorkerID, presence.FDExhaustionDiagnosis,
				),
				Remediation: []string{
					"Raise the host file-descriptor limit (ulimit -n) or reduce desired_count until resource pressure clears",
				},
			})
		}
	}

	if gitattrIssue := checkRuntimeJSONLMergeCoverage(projectRoot); gitattrIssue != nil {
		issues = append(issues, *gitattrIssue)
	}

	if beadIssue := checkPreservedReviewGate(projectRoot); beadIssue != nil {
		issues = append(issues, *beadIssue)
	}

	return issues
}

// checkRuntimeJSONLMergeCoverage reports when the project's .gitattributes is
// missing or lacks union-merge coverage for tracked runtime metrics JSONL
// (ddx-2520a267). A missing/incomplete entry means concurrent worker appends
// to .ddx/metrics/attempts.jsonl or .ddx/metrics/locks.jsonl hit ordinary
// line-conflict merges instead of resolving cleanly.
func checkRuntimeJSONLMergeCoverage(projectRoot string) *DiagnosticIssue {
	path := filepath.Join(projectRoot, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil
		}
		return &DiagnosticIssue{
			Type: "runtime_jsonl_merge_coverage",
			Description: "Runtime JSONL merge/lock coverage gap: no .gitattributes found; " +
				".ddx/metrics/*.jsonl (attempts.jsonl, locks.jsonl) will hit ordinary merge conflicts on concurrent appends",
			Remediation: []string{
				fmt.Sprintf("Add '%s' to a repo-root .gitattributes file", runtimeJSONLMergeUnionPattern),
			},
		}
	}
	if strings.Contains(string(data), runtimeJSONLMergeUnionPattern) {
		return nil
	}
	return &DiagnosticIssue{
		Type: "runtime_jsonl_merge_coverage",
		Description: "Runtime JSONL merge/lock coverage gap: .gitattributes is missing '" + runtimeJSONLMergeUnionPattern +
			"'; attempts.jsonl and locks.jsonl are uncovered by the union merge driver",
		Remediation: []string{
			fmt.Sprintf("Add '%s' to .gitattributes", runtimeJSONLMergeUnionPattern),
		},
	}
}

// checkPreservedReviewGate reports beads carrying an unresolved
// preserved-needs-review block marker (ddx-ec1c1f89). Ready beads with an
// active block are excluded from worker readiness until an operator stamps a
// matching unblock marker, so a stuck one silently stalls the queue unless
// surfaced here.
func checkPreservedReviewGate(projectRoot string) *DiagnosticIssue {
	store := bead.NewStore(resolveBeadStoreRoot(projectRoot))
	blocked, err := store.PreservedReviewBlocked()
	if err != nil || len(blocked) == 0 {
		return nil
	}
	ids := make([]string, 0, len(blocked))
	for _, b := range blocked {
		ids = append(ids, b.ID)
	}
	return &DiagnosticIssue{
		Type: "preserved_review_gate",
		Description: fmt.Sprintf(
			"Unresolved preserved-review gate: %d bead(s) blocked pending operator review: %s",
			len(blocked), strings.Join(ids, ", "),
		),
		Remediation: []string{
			"Review the flagged large-deletion change for each bead",
			"Clear the gate with: ddx bead update <id> --set preserved-review-unblocked-at=<RFC3339-now> --set preserved-review-unblocked-attempt=<blocked-attempt-id>",
		},
	}
}
