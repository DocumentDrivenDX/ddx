package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewFailureRunner is a test-local deterministic Executor + Reviewer pair
// driving the "N reviewer failures, then 1 success" scenario. Test-local on
// purpose: a shared cross-package testfixtures package would be unreachable
// from main() under deadcode RTA (production-reachability check).
type reviewFailureRunner struct {
	resultRev     string
	failUntilCall int
	reviewCalls   atomic.Int32
	execCalls     atomic.Int32
}

func (r *reviewFailureRunner) Executor() agent.ExecuteBeadExecutor {
	return agent.ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (agent.ExecuteBeadReport, error) {
		n := r.execCalls.Add(1)
		return agent.ExecuteBeadReport{
			BeadID:    beadID,
			Status:    agent.ExecuteBeadStatusSuccess,
			SessionID: fmt.Sprintf("rfr-sess-%d", n),
			ResultRev: r.resultRev,
		}, nil
	})
}

func (r *reviewFailureRunner) Reviewer() agent.BeadReviewer {
	return reviewerFn(func(_ context.Context, _, resultRev string, _ agent.ImplementerRouting) (*agent.ReviewResult, error) {
		n := int(r.reviewCalls.Add(1))
		if n <= r.failUntilCall {
			class := evidence.OutcomeReviewProviderEmpty
			return &agent.ReviewResult{
					Verdict:   agent.VerdictBlock,
					Error:     class,
					ResultRev: resultRev,
				}, fmt.Errorf("review-failure-runner: %s: %w", class,
					errors.New("simulated reviewer failure"))
		}
		return &agent.ReviewResult{
			Verdict:   agent.VerdictApprove,
			Rationale: "review-failure-runner: APPROVE",
			PerAC: []agent.ReviewAC{
				{Number: 1, Item: "retry threshold", Grade: "pass", Evidence: "APPROVE after FailUntilCall"},
			},
			ResultRev: resultRev,
		}, nil
	})
}

func (r *reviewFailureRunner) ReviewCalls() int { return int(r.reviewCalls.Load()) }
func (r *reviewFailureRunner) ExecCalls() int   { return int(r.execCalls.Load()) }

type reviewerFn func(ctx context.Context, beadID, resultRev string, impl agent.ImplementerRouting) (*agent.ReviewResult, error)

func (f reviewerFn) ReviewBead(ctx context.Context, beadID, resultRev string, impl agent.ImplementerRouting) (*agent.ReviewResult, error) {
	return f(ctx, beadID, resultRev, impl)
}

// TestReviewRetryThresholdFromConfigServer is the SD-024 Stage 1 configuration
// wiring proof that the server dispatch path at runWorker carries
// review_max_retries from .ddx/config.yaml through to the running loop.
//
// The test mirrors the production wiring introduced in workers.go: it
// drives StartExecuteLoop → runWorker against a real on-disk
// .ddx/config.yaml (the same file the runWorker dispatch site's
// config.LoadAndResolve call reads). The deterministic review-failure
// fixture (test-local) provides the executor + reviewer pair via
// BeadWorkerFactory so behavior is observable end-to-end without a real
// agent harness.
//
// Candidate-cycle pre-land review now owns close eligibility, so work
// must not invoke the legacy post-land reviewer/retry path. The fixture sets a
// failing reviewer threshold to prove it remains unused.
func TestReviewRetryThresholdFromConfigServer(t *testing.T) {
	if testing.Short() {
		t.Skip("requires work with review infrastructure; too slow for -short")
	}
	const (
		threshold = 5
		beadID    = "ddx-server-rmr-001"
		fixedRev  = "cafebabe00112233"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
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
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: beadID, Title: "server e2e review-retry threshold", Priority: 0}))

	runner := &reviewFailureRunner{
		resultRev:     fixedRev,
		failUntilCall: threshold,
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

	rec, err := m.StartExecuteLoop(ExecuteLoopWorkerSpec{
		ProjectRoot: projectRoot,
		Mode:        "once",
	})
	require.NoError(t, err)
	_ = waitForWorkerExit(t, m, rec.ID, 10*time.Second)

	assert.Equal(t, 0, runner.ReviewCalls(),
		"legacy post-land reviewer must not be invoked by work")
	assert.Equal(t, 1, runner.ExecCalls(),
		"executor must be invoked once for the successful attempt")

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

	assert.Equal(t, 0, reviewErrorCount,
		"work must not emit legacy post-land review-error events")
	assert.Equal(t, 0, reviewApproveCount,
		"work must not emit legacy post-land review events")
	assert.Equal(t, 0, manualRequiredCount,
		"work must not emit legacy post-land review-manual-required events")

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status,
		"successful work attempt must close directly after candidate-cycle approval")

	// Defensive: a stale heartbeat ticker in the loop could outlive the
	// final iteration. Give it a beat to settle so test cleanup is clean.
	time.Sleep(10 * time.Millisecond)
}
