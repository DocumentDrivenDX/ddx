package testfixtures

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewFailureRunner verifies the fixture's contract: configured to
// fail N times then approve, driving real loop iterations against a real
// bead.Store produces N review-error events plus one APPROVE event, with
// the bead closed at the end. This is the integration shape beads 7-9
// build on top of when behavioral-testing ReviewMaxRetries flow from
// .ddx/config.yaml.
func TestReviewFailureRunner(t *testing.T) {
	const (
		failUntil    = 2
		threshold    = 5 // > failUntil + 1 so the fixture's APPROVE wins.
		beadID       = "ddx-rfr-001"
		fixedRev     = "deadbeefdeadbeef"
		failureClass = evidence.OutcomeReviewProviderEmpty
	)

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "rfr smoke", Priority: 0}))

	runner := &ReviewFailureRunner{
		ResultRev:     fixedRev,
		FailureClass:  failureClass,
		FailUntilCall: failUntil,
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Executor: runner.Executor(),
		Reviewer: runner.Reviewer(),
	}

	cfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{
		Assignee:                "rfr-worker",
		ReviewMaxRetries:        threshold,
		NoProgressCooldown:      time.Hour,
		MaxNoChangesBeforeClose: 3,
		HeartbeatInterval:       time.Hour, // long enough not to fire under Once=true
		EvidenceCaps:            config.EvidenceCapsConfig{},
	})
	rcfg := cfg.Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{
		Assignee: "rfr-worker",
	}))

	runtime := agent.ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	}

	const totalIterations = failUntil + 1
	for i := 0; i < totalIterations; i++ {
		_, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
		require.NoErrorf(t, err, "iteration %d: RunWithConfig", i+1)
		// On a reviewer-error iteration the loop intentionally leaves
		// the bead claimed (no Close, no Reopen) so the next iteration
		// in production retries the same result_rev. Tests drive that
		// re-pickup by unclaiming between iterations.
		if i < totalIterations-1 {
			require.NoError(t, store.Unclaim(beadID), "iteration %d: unclaim", i+1)
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
			assert.Contains(t, ev.Body, "failure_class="+failureClass,
				"review-error body must carry the configured failure_class")
			assert.Contains(t, ev.Body, "result_rev="+fixedRev,
				"review-error body must scope to the runner's result_rev")
		case "review":
			if ev.Summary == "APPROVE" {
				reviewApproveCount++
			}
		case "review-manual-required":
			manualRequiredCount++
		}
	}

	assert.Equal(t, failUntil, reviewErrorCount,
		"exactly FailUntilCall review-error events must be observable")
	assert.Equal(t, 1, reviewApproveCount,
		"the (FailUntilCall+1)th iteration must record an APPROVE review event")
	assert.Equal(t, 0, manualRequiredCount,
		"with threshold > FailUntilCall+1 no review-manual-required must fire")

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status,
		"after APPROVE the loop must close the bead via CloseWithEvidence")
}

// TestReviewFailureRunner_ThresholdEscalation confirms the same fixture
// exposes the inverse threshold behavior: when the configured
// ReviewMaxRetries is reached before the runner switches to APPROVE,
// the loop emits the terminal review-manual-required event. This
// mirrors the production failure path beads 7-9 verify against real
// .ddx/config.yaml values.
func TestReviewFailureRunner_ThresholdEscalation(t *testing.T) {
	const (
		failUntil = 3
		threshold = 2 // first 2 failures consume the budget; 2nd trips terminal.
		beadID    = "ddx-rfr-002"
	)

	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "rfr threshold", Priority: 0}))

	runner := &ReviewFailureRunner{
		FailUntilCall: failUntil,
		FailureClass:  evidence.OutcomeReviewTransport,
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Executor: runner.Executor(),
		Reviewer: runner.Reviewer(),
	}

	cfg := config.NewTestConfigForLoop(config.TestLoopConfigOpts{
		Assignee:                "rfr-worker",
		ReviewMaxRetries:        threshold,
		NoProgressCooldown:      time.Hour,
		MaxNoChangesBeforeClose: 3,
		HeartbeatInterval:       time.Hour,
		EvidenceCaps:            config.EvidenceCapsConfig{},
	})
	rcfg := cfg.Resolve(config.TestLoopOverrides(config.TestLoopConfigOpts{
		Assignee: "rfr-worker",
	}))

	runtime := agent.ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	}

	// Drive iterations until the threshold trips. The first failure
	// emits review-error (attempt_count=1); the second trips the
	// configured threshold and emits review-manual-required, parking
	// the bead via SetExecutionCooldown.
	for i := 0; i < threshold; i++ {
		_, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
		require.NoErrorf(t, err, "iteration %d", i+1)
		if i < threshold-1 {
			require.NoError(t, store.Unclaim(beadID))
		}
	}

	events, err := store.Events(beadID)
	require.NoError(t, err)

	var manual *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "review-manual-required" {
			manual = &events[i]
			break
		}
	}
	require.NotNil(t, manual,
		"with ReviewMaxRetries=%d the threshold-th failure must emit review-manual-required", threshold)
}
