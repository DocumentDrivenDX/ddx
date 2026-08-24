package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiProjectSelfHealingFixture is the shared DDx/Cayce/Snorri/Pqueue-shaped
// four-project fixture used by the TestIntegration_MultiProjectWorkerSelfHealing_*
// regressions (self-healing-workers-revised-design.md, implementation bead 8).
// It proves SupervisorRegistry.ReconcileAll's per-project error isolation across
// the plan's combined failure modes in one multi-project fixture:
//
//   - ddx:     a restart-blocked terminal older than DefaultTerminalBlockTTL,
//     with git status unknown, must expire via TTL and permit a restart
//     (not only via the desired.UpdatedAt freshness check).
//   - cayce:   an in-progress bead with a stale external claim lease AND an
//     unresolved preserved-review block marker must stay excluded from
//     ReadyExecution (not reclaimed), even though the stale claim alone
//     would normally make it reclaimable.
//   - snorri:  a fresh resource-exhausted (fd_exhaustion) terminal must
//     surface via LatestBlockedTerminal and delay (not immediately
//     restart through) the ordinary restart backoff.
//   - pqueue:  an empty, otherwise-healthy project must keep reconciling
//     (spawn its desired worker) independent of the other three.
type multiProjectSelfHealingFixture struct {
	srv *Server

	ddxRoot    string
	cayceRoot  string
	snorriRoot string
	pqueueRoot string

	ddxSup    *WorkerSupervisor
	cayceSup  *WorkerSupervisor
	snorriSup *WorkerSupervisor
	pqueueSup *WorkerSupervisor

	cayceStore  *bead.Store
	cayceBeadID string
}

func newMultiProjectSelfHealingFixture(t *testing.T) *multiProjectSelfHealingFixture {
	t.Helper()

	workDir := setupTestDir(t)
	srv := New(":0", workDir)
	require.NotNil(t, srv.supervisorRegistry)
	t.Cleanup(func() { _ = srv.Shutdown() })

	fx := &multiProjectSelfHealingFixture{
		srv:        srv,
		ddxRoot:    t.TempDir(),
		cayceRoot:  t.TempDir(),
		snorriRoot: t.TempDir(),
		pqueueRoot: t.TempDir(),
	}

	now := time.Now().UTC()

	// --- ddx shape: stale (TTL-expired) restart-blocked terminal ---
	// No git repo at fx.ddxRoot: projectRestartBlockingDirtyPaths returns
	// known=false, which forces the TTL-expiry path (expireStaleBlockedTerminalsLocked)
	// instead of the dirty-worktree check.
	initSupervisorProject(t, fx.ddxRoot)
	setupBeadStore(t, fx.ddxRoot)
	fx.ddxSup = srv.supervisorRegistry.getOrCreate(fx.ddxRoot)
	require.NotNil(t, fx.ddxSup)
	installBlockingWorkerFactory(fx.ddxSup.manager)
	staleTerminalAt := now.Add(-15 * time.Minute)
	seedTerminalOperatorAttentionWorker(t, fx.ddxSup.manager, fx.ddxRoot, "worker-ddx-blocked", staleTerminalAt)
	ddxDesired := DefaultWorkerDesiredState(fx.ddxRoot)
	ddxDesired.DesiredCount = 1
	ddxDesired.DefaultSpec.OpaquePassthrough = true
	// Desired state predates the block, so the pre-fix freshness-only clear
	// (desired.UpdatedAt > block.TerminalAt) must NOT be what clears this
	// block -- only TTL expiry may.
	ddxDesired.UpdatedAt = staleTerminalAt.Add(-5 * time.Minute)
	require.NoError(t, writeDesiredStateForTest(fx.ddxSup, ddxDesired))

	// --- cayce shape: unresolved preserved-review bead with a stale claim lease ---
	initSupervisorProject(t, fx.cayceRoot)
	setupBeadStore(t, fx.cayceRoot)
	fx.cayceSup = srv.supervisorRegistry.getOrCreate(fx.cayceRoot)
	require.NotNil(t, fx.cayceSup)
	installBlockingWorkerFactory(fx.cayceSup.manager)
	cayceDesired := DefaultWorkerDesiredState(fx.cayceRoot)
	cayceDesired.DesiredCount = 1
	cayceDesired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, writeDesiredStateForTest(fx.cayceSup, cayceDesired))

	cayceDDxDir := ddxroot.JoinProject(fx.cayceRoot)
	fx.cayceStore = bead.NewStore(cayceDDxDir)
	fx.cayceBeadID = "cayce-preserved-review-blocked"
	require.NoError(t, fx.cayceStore.Create(context.Background(), &bead.Bead{
		ID:         fx.cayceBeadID,
		Title:      "Cayce-shape preserved-review-blocked bead",
		Status:     bead.StatusOpen,
		Priority:   0,
		IssueType:  bead.DefaultType,
		Acceptance: "n/a",
	}))
	require.NoError(t, fx.cayceStore.Claim(fx.cayceBeadID, "cayce-worker-1"))
	// Force the external claim lease stale (different machine, old timestamp) so
	// the in-progress claim alone would normally be reclaimable via ReadyExecution.
	writeStaleClaimLease(t, cayceDDxDir, bead.ClaimLeaseRecord{
		BeadID:    fx.cayceBeadID,
		Owner:     "cayce-worker-1",
		Machine:   "stale-machine",
		StartedAt: now.Add(-20 * time.Minute),
		UpdatedAt: now.Add(-20 * time.Minute),
		PID:       999999,
	})
	require.NoError(t, fx.cayceStore.Update(context.Background(), fx.cayceBeadID, func(b *bead.Bead) {
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		b.Extra[bead.ExtraPreservedReviewBlockedAt] = now.Add(-10 * time.Minute).Format(time.RFC3339)
		b.Extra[bead.ExtraPreservedReviewBlockedAttempt] = "attempt-cayce-1"
		b.Extra[bead.ExtraPreservedReviewGate] = bead.PreservedReviewGateLargeDeletion
	}))

	// --- snorri shape: fresh resource-exhausted (fd_exhaustion) terminal ---
	initSupervisorProject(t, fx.snorriRoot)
	setupBeadStore(t, fx.snorriRoot)
	fx.snorriSup = srv.supervisorRegistry.getOrCreate(fx.snorriRoot)
	require.NotNil(t, fx.snorriSup)
	installBlockingWorkerFactory(fx.snorriSup.manager)
	seedTerminalResourceExhaustedWorker(t, fx.snorriSup.manager, fx.snorriRoot, "worker-snorri-fd", now)
	snorriDesired := DefaultWorkerDesiredState(fx.snorriRoot)
	snorriDesired.DesiredCount = 1
	snorriDesired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, writeDesiredStateForTest(fx.snorriSup, snorriDesired))

	// --- pqueue shape: empty queue, otherwise-healthy project ---
	initSupervisorProject(t, fx.pqueueRoot)
	setupBeadStore(t, fx.pqueueRoot)
	fx.pqueueSup = srv.supervisorRegistry.getOrCreate(fx.pqueueRoot)
	require.NotNil(t, fx.pqueueSup)
	installBlockingWorkerFactory(fx.pqueueSup.manager)
	pqueueDesired := DefaultWorkerDesiredState(fx.pqueueRoot)
	pqueueDesired.DesiredCount = 1
	pqueueDesired.DefaultSpec.OpaquePassthrough = true
	require.NoError(t, writeDesiredStateForTest(fx.pqueueSup, pqueueDesired))

	srv.RegisterProject(fx.ddxRoot)
	srv.RegisterProject(fx.cayceRoot)
	srv.RegisterProject(fx.snorriRoot)
	srv.RegisterProject(fx.pqueueRoot)

	return fx
}

// writeStaleClaimLease overwrites the external claim-heartbeat sidecar for a
// bead with an already-stale record, simulating a worker that claimed the
// bead and then died without releasing or renewing its lease.
func writeStaleClaimLease(t *testing.T, ddxDir string, rec bead.ClaimLeaseRecord) {
	t.Helper()
	dir := bead.ClaimLivenessRoot(ddxDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, rec.BeadID+".json"), data, 0o644))
}

// seedTerminalResourceExhaustedWorker writes a terminal worker record plus a
// structured ManagedWorkerResult classifying it as fd-exhaustion, mirroring
// the shape produced by a real worker that hit the file-descriptor limit
// (see TestWorkerSupervisorPrefersStructuredFDExhaustionDiagnosis).
func seedTerminalResourceExhaustedWorker(t *testing.T, m *WorkerManager, root, workerID string, terminalAt time.Time) {
	t.Helper()
	dir := filepath.Join(m.rootDir, workerID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, WriteManagedWorkerResult(root, workerID, ManagedWorkerResult{
		StopCondition:                 "ResourceExhausted",
		LastFailureStatus:             agent.ExecuteBeadStatusResourceExhausted,
		LastFailureDetail:             agent.FDExhaustionStopMessage,
		ResourceExhaustionDiagnosis:   agent.ResourceExhaustionDiagnosisFD,
		ResourceExhaustionRestartable: true,
	}))
	rec := WorkerRecord{
		ID:          workerID,
		Kind:        "work",
		State:       "exited",
		Status:      agent.ExecuteBeadStatusResourceExhausted,
		ProjectRoot: root,
		StartedAt:   terminalAt.Add(-time.Minute),
		FinishedAt:  terminalAt,
	}
	require.NoError(t, m.writeRecord(dir, rec))
}

// TestIntegration_MultiProjectWorkerSelfHealing_StaleTerminalRestartsDDxShape
// proves that a restart-blocked terminal older than DefaultTerminalBlockTTL
// expires and permits a restart even when desired.UpdatedAt predates the
// block (the freshness-only clear alone would suppress restart forever).
func TestIntegration_MultiProjectWorkerSelfHealing_StaleTerminalRestartsDDxShape(t *testing.T) {
	fx := newMultiProjectSelfHealingFixture(t)

	require.NoError(t, fx.srv.supervisorRegistry.ReconcileAll())

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, fx.ddxSup.manager, fx.ddxRoot) == 1
	}, 2*time.Second, 20*time.Millisecond,
		"ddx-shape project must restart its desired worker once the stale (TTL-expired) terminal block clears")
}

// TestIntegration_MultiProjectWorkerSelfHealing_PreservedReviewDoesNotReclaimCayceShape
// proves that a bead carrying an unresolved preserved-review block marker
// stays excluded from ReadyExecution even though its external claim lease is
// stale and would otherwise make it reclaimable.
func TestIntegration_MultiProjectWorkerSelfHealing_PreservedReviewDoesNotReclaimCayceShape(t *testing.T) {
	fx := newMultiProjectSelfHealingFixture(t)

	require.NoError(t, fx.srv.supervisorRegistry.ReconcileAll())

	ready, err := fx.cayceStore.ReadyExecution()
	require.NoError(t, err)
	for _, b := range ready {
		assert.NotEqual(t, fx.cayceBeadID, b.ID,
			"cayce-shape bead with an unresolved preserved-review block and a stale claim lease must not be reclaimed via ReadyExecution")
	}

	blocked, err := fx.cayceStore.PreservedReviewBlocked()
	require.NoError(t, err)
	found := false
	for _, b := range blocked {
		if b.ID == fx.cayceBeadID {
			found = true
		}
	}
	assert.True(t, found, "cayce-shape bead must be surfaced as preserved-review-blocked")

	// cayce's own desired worker still reconciles independently of ddx's
	// stale-terminal recovery and snorri's resource-exhaustion backoff.
	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, fx.cayceSup.manager, fx.cayceRoot) == 1
	}, 2*time.Second, 20*time.Millisecond,
		"cayce-shape project must keep reconciling its desired worker independent of the preserved-review-blocked bead")
}

// TestIntegration_MultiProjectWorkerSelfHealing_ResourceExhaustedSnorriShapeDoesNotBlockPqueueShape
// proves that a fresh resource-exhausted terminal surfaces diagnostics and
// delays (via the ordinary restart backoff) rather than immediately
// restarting, and that this does not block an unrelated healthy project from
// reconciling its own desired worker.
func TestIntegration_MultiProjectWorkerSelfHealing_ResourceExhaustedSnorriShapeDoesNotBlockPqueueShape(t *testing.T) {
	fx := newMultiProjectSelfHealingFixture(t)

	require.NoError(t, fx.srv.supervisorRegistry.ReconcileAll())

	diag, ok := fx.snorriSup.LatestBlockedTerminal()
	require.True(t, ok, "resource-exhausted terminal must surface a diagnostic")
	assert.Equal(t, agent.ResourceExhaustionDiagnosisFD, diag.Diagnosis)
	assert.True(t, diag.Restartable, "fd exhaustion is worker-local and restartable")
	assert.Zero(t, runningManagedWorkerCount(t, fx.snorriSup.manager, fx.snorriRoot),
		"snorri-shape project must not restart immediately after a fresh resource-exhausted terminal; backoff must still apply")

	require.Eventually(t, func() bool {
		return runningManagedWorkerCount(t, fx.pqueueSup.manager, fx.pqueueRoot) == 1
	}, 2*time.Second, 20*time.Millisecond,
		"pqueue-shape project must keep reconciling independently of snorri's resource-exhaustion backoff")
}
