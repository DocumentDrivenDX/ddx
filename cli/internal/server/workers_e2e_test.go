package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/testfixtures"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewRetryThresholdFromConfigServer is the SD-024 Stage 1 behavioral
// proof that the server dispatch path at runWorker carries
// review_max_retries from .ddx/config.yaml through to the running loop.
//
// The test mirrors the production wiring introduced in workers.go: it
// drives StartExecuteLoop → runWorker against a real on-disk
// .ddx/config.yaml (the same file the runWorker dispatch site's
// config.LoadAndResolve call reads). The deterministic review-failure
// fixture from testfixtures provides the executor + reviewer pair via
// BeadWorkerFactory so behavior is observable end-to-end without a real
// agent harness.
//
// Configured values:
//   - .ddx/config.yaml: review_max_retries: 5
//   - fixture: FailUntilCall=4 (attempts 1-4 return reviewer error,
//     attempt 5 returns APPROVE)
//
// Expected behavior:
//   - 5 StartExecuteLoop iterations drive 5 reviewer attempts.
//   - attempts 1-4 emit review-error events.
//   - attempt 5 emits an APPROVE review event and the loop closes
//     the bead via CloseWithEvidence.
//   - no review-manual-required event fires (threshold=5 ≥ attempts=5).
func TestReviewRetryThresholdFromConfigServer(t *testing.T) {
	const (
		failUntil = 4
		threshold = 5
		beadID    = "ddx-server-rmr-001"
		fixedRev  = "cafebabe00112233"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Real on-disk .ddx/config.yaml — this is the file the server
	// dispatch path's config.LoadAndResolve call reads. The presence of
	// review_max_retries: 5 here is the entire premise of the test.
	cfgYAML := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://github.com/example/repo
    branch: main
review_max_retries: 5
`
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfgYAML), 0o644))

	store := bead.NewStore(ddxDir)
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "server e2e review-retry threshold", Priority: 0}))

	runner := &testfixtures.ReviewFailureRunner{
		ResultRev:     fixedRev,
		FailUntilCall: failUntil,
	}

	m := NewWorkerManager(projectRoot)
	defer m.StopWatchdog()
	// Inject the deterministic executor+reviewer pair through the same
	// factory seam runWorker honours in production. The factory bypasses
	// the real agent runner so behavior is fully observable.
	m.BeadWorkerFactory = func(s agent.ExecuteBeadLoopStore) *agent.ExecuteBeadWorker {
		return &agent.ExecuteBeadWorker{
			Store:    s,
			Executor: runner.Executor(),
			Reviewer: runner.Reviewer(),
		}
	}

	// Drive failUntil + 1 iterations through StartExecuteLoop → runWorker
	// (one --once invocation per attempt, matching the production loop's
	// per-iteration claim/release rhythm). Between iterations the test
	// unclaims the bead because the loop intentionally leaves it claimed
	// on a reviewer-error iteration.
	const totalIterations = failUntil + 1
	for i := 0; i < totalIterations; i++ {
		rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
			ProjectRoot: projectRoot,
			Once:        true,
		})
		require.NoErrorf(t, err, "iteration %d: StartExecuteLoop", i+1)
		_ = waitForWorkerExit(t, m, rec.ID, 10*time.Second)
		if i < totalIterations-1 {
			require.NoErrorf(t, store.Unclaim(beadID), "iteration %d: unclaim", i+1)
		}
	}

	assert.Equal(t, totalIterations, runner.ReviewCalls(),
		"reviewer must be invoked once per driven iteration")
	assert.Equal(t, totalIterations, runner.ExecCalls(),
		"executor must be invoked once per driven iteration")

	events, err := store.Events(beadID)
	require.NoError(t, err)

	var (
		reviewErrorCount    int
		reviewApproveCount  int
		manualRequiredCount int
	)
	for _, ev := range events {
		switch ev.Kind {
		case "review-error":
			reviewErrorCount++
		case "review":
			if ev.Summary == "APPROVE" {
				reviewApproveCount++
			}
		case "review-manual-required":
			manualRequiredCount++
		}
	}

	assert.Equal(t, failUntil, reviewErrorCount,
		"reviewer-error count must match the fixture's FailUntilCall")
	assert.Equal(t, 1, reviewApproveCount,
		"the (FailUntilCall+1)th iteration must record an APPROVE review event")
	assert.Equal(t, 0, manualRequiredCount,
		"with review_max_retries=%d ≥ attempts=%d the loop must NOT emit review-manual-required",
		threshold, totalIterations)

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status,
		"after APPROVE on attempt %d the loop must close the bead", totalIterations)

	// Defensive: a stale heartbeat ticker in the loop could outlive the
	// final iteration. Give it a beat to settle so test cleanup is clean.
	time.Sleep(10 * time.Millisecond)
}
