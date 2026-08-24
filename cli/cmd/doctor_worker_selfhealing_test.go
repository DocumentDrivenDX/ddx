package cmd

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoctorReportsStaleTerminalSuppressionRisk covers ddx-188ca92a AC #1: a
// restart-blocked terminal worker (operator_attention/dirty_root/
// resource_exhausted, per isRestartBlockedWorker) whose age exceeds
// server.DefaultTerminalBlockTTL is a stale suppression risk that a running
// supervisor would auto-expire on its next reconcile tick. doctor surfaces it
// so operators can see the risk even when no supervisor is currently running.
func TestDoctorReportsStaleTerminalSuppressionRisk(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)

	terminalAt := time.Now().Add(-2 * server.DefaultTerminalBlockTTL).UTC()
	writeWorkerRecord(t, projectRoot, "worker-stale-block", server.WorkerRecord{
		ID:          "worker-stale-block",
		Kind:        "work",
		State:       "exited",
		Status:      "operator_attention",
		ReapReason:  "operator_attention",
		ProjectRoot: projectRoot,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
	})

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, output, "Stale restart-blocked terminal")
	assert.Contains(t, output, "worker-stale-block")
}

// TestDoctorReportsWorkerLivenessEvidenceMismatch covers ddx-188ca92a AC #2: a
// worker record shows State=running with a live PID, but its recorded PGID no
// longer matches the live process group. Naive PID-alive checks (e.g.
// WorkerManager.List's PIDAlive) would call this worker alive; the fuller
// liveness evidence used by the supervisor disagrees. doctor surfaces the
// mismatch.
func TestDoctorReportsWorkerLivenessEvidenceMismatch(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)

	pid := os.Getpid()
	actualPGID, err := syscall.Getpgid(pid)
	require.NoError(t, err)

	writeWorkerRecord(t, projectRoot, "worker-liveness-mismatch", server.WorkerRecord{
		ID:          "worker-liveness-mismatch",
		Kind:        "work",
		State:       "running",
		ProjectRoot: projectRoot,
		StartedAt:   time.Now().Add(-time.Minute).UTC(),
		PID:         pid,
		PGID:        actualPGID + 999999,
	})

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, output, "Worker liveness evidence mismatch")
	assert.Contains(t, output, "worker-liveness-mismatch")
}

// TestDoctorReportsRuntimeJSONLMergeAndLockCoverage covers ddx-188ca92a AC #3:
// a project whose .gitattributes is missing the union-merge coverage for
// tracked runtime metrics JSONL (ddx-2520a267) leaves .ddx/metrics/
// attempts.jsonl and locks.jsonl exposed to ordinary merge conflicts on
// concurrent worker appends. doctor surfaces the gap.
func TestDoctorReportsRuntimeJSONLMergeAndLockCoverage(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, output, "Runtime JSONL merge/lock coverage gap")
	assert.Contains(t, output, ".ddx/metrics/*.jsonl merge=union")
}

// TestDoctorReportsUnresolvedPreservedReviewGate covers ddx-188ca92a AC #4: a
// bead carrying an unresolved preserved-needs-review block marker
// (ddx-ec1c1f89) is excluded from worker readiness until an operator stamps a
// matching unblock marker. doctor surfaces the stuck bead so it does not
// silently stall the queue.
func TestDoctorReportsUnresolvedPreservedReviewGate(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)

	store := bead.NewStore(ddxroot.JoinProject(projectRoot))
	b := &bead.Bead{
		Title:     "large deletion needs review",
		Status:    bead.StatusOpen,
		Priority:  1,
		IssueType: bead.IssueTypeOperatorPrompt,
	}
	require.NoError(t, store.Create(context.Background(), b))
	require.NoError(t, store.Update(context.Background(), b.ID, func(target *bead.Bead) {
		if target.Extra == nil {
			target.Extra = map[string]any{}
		}
		target.Extra[bead.ExtraPreservedReviewBlockedAt] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		target.Extra[bead.ExtraPreservedReviewBlockedAttempt] = "20260824T000000-deadbeef"
		target.Extra[bead.ExtraPreservedReviewGate] = bead.PreservedReviewGateLargeDeletion
	}))

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, output, "Unresolved preserved-review gate")
	assert.Contains(t, output, b.ID)
}

// TestDoctorReportsResourcePressure covers ddx-188ca92a AC #5: desired
// worker(s) are missing and the newest terminal worker's structured status
// diagnoses file-descriptor exhaustion (agent.ResourceExhaustionDiagnosisFD).
// doctor surfaces the resource pressure via the existing read-only
// DiagnoseDesiredWorkerPresence API.
func TestDoctorReportsResourcePressure(t *testing.T) {
	projectRoot := setupWorkStartupCleanupProjectRoot(t)

	manager := server.NewWorkerManager(projectRoot)
	manageWorkers := true
	manager.SetManageWorkers(&manageWorkers)
	supervisor := server.NewWorkerSupervisor(manager)
	desired := server.DefaultWorkerDesiredState(projectRoot)
	desired.DesiredCount = 1
	require.NoError(t, supervisor.SaveDesiredState(&desired))

	terminalAt := time.Now().Add(-time.Minute).UTC()
	writeWorkerRecord(t, projectRoot, "worker-fd-exhausted", server.WorkerRecord{
		ID:          "worker-fd-exhausted",
		Kind:        "work",
		State:       "exited",
		Status:      agent.ExecuteBeadStatusResourceExhausted,
		ProjectRoot: projectRoot,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
		LastError:   agent.FDExhaustionStopMessage,
	})

	factory := NewCommandFactory(projectRoot)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, output, "Resource pressure")
	assert.Contains(t, output, agent.ResourceExhaustionDiagnosisFD)
}
