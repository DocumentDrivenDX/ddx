package cmd

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
	"github.com/DocumentDrivenDX/ddx/internal/config"
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

// TestReviewRetryThresholdFromConfigCLI is the SD-024 Stage 1 behavioral
// proof that the CLI dispatch path at runAgentExecuteLoop carries
// review_max_retries from .ddx/config.yaml through to the running loop.
//
// The test mirrors the production wiring introduced in agent_cmd.go:
// it calls config.LoadAndResolve against a real on-disk .ddx/config.yaml
// (the same call the CLI dispatch site issues) and then invokes
// ExecuteBeadWorker.RunWithConfig with an ExecuteBeadLoopRuntime shaped
// identically to the runtime the CLI builds. The deterministic
// review-failure fixture (test-local) provides the executor +
// reviewer pair so behavior is observable end-to-end without a real
// agent harness.
//
// Configured values:
//   - .ddx/config.yaml: review_max_retries: 5
//   - fixture: FailUntilCall=4 (attempts 1-4 return reviewer error,
//     attempt 5 returns APPROVE)
//
// Expected behavior:
//   - 5 RunWithConfig iterations drive 5 reviewer attempts.
//   - attempts 1-4 emit review-error events.
//   - attempt 5 emits an APPROVE review event and the loop closes
//     the bead via CloseWithEvidence.
//   - no review-manual-required event fires (threshold=5 ≥ attempts=5).
func TestReviewRetryThresholdFromConfigCLI(t *testing.T) {
	const (
		failUntil   = 4
		threshold   = 5
		beadID      = "ddx-cli-rmr-001"
		fixedRev    = "cafebabe00112233"
		assigneeStr = "cli-e2e-worker"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Real on-disk .ddx/config.yaml — this is the file the CLI dispatch
	// path's config.LoadAndResolve call reads. The presence of
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
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "cli e2e review-retry threshold", Priority: 0}))

	runner := &reviewFailureRunner{
		resultRev:     fixedRev,
		failUntilCall: failUntil,
	}

	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Executor: runner.Executor(),
		Reviewer: runner.Reviewer(),
	}

	// Same shape the migrated CLI dispatch site builds.
	overrides := config.CLIOverrides{Assignee: assigneeStr}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	require.NoError(t, err)
	require.Equal(t, threshold, rcfg.ReviewMaxRetries(),
		"LoadAndResolve must surface review_max_retries from .ddx/config.yaml")

	runtime := agent.ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(), // execute-bead worktree base; isolated from project root.
	}

	// Drive failUntil + 1 iterations (one --once invocation per attempt,
	// matching the production loop's per-iteration claim/release rhythm).
	const totalIterations = failUntil + 1
	for i := 0; i < totalIterations; i++ {
		_, runErr := worker.Run(context.Background(), rcfg, runtime)
		require.NoErrorf(t, runErr, "iteration %d: RunWithConfig", i+1)
		if i < totalIterations-1 {
			// On a reviewer-error iteration the loop intentionally leaves
			// the bead claimed; production re-picks it up next poll. The
			// test drives that re-pickup explicitly via Unclaim.
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
