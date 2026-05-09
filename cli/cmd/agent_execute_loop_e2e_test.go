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

// TestReviewRetryThresholdFromConfigCLI is the SD-024 Stage 1 configuration
// wiring proof. The CLI dispatch path at runAgentExecuteLoop must still carry
// review_max_retries from .ddx/config.yaml through to the resolved loop config.
//
// The test mirrors the production wiring introduced in agent_cmd.go:
// it calls config.LoadAndResolve against a real on-disk .ddx/config.yaml
// (the same call the CLI dispatch site issues) and then invokes
// ExecuteBeadWorker.RunWithConfig with an ExecuteBeadLoopRuntime shaped
// identically to the runtime the CLI builds.
//
// Candidate-cycle pre-land review now owns close eligibility, so execute-loop
// must not invoke the legacy post-land reviewer/retry path. This test asserts
// the resolved config still carries the threshold while a successful loop
// attempt closes directly without review events.
func TestReviewRetryThresholdFromConfigCLI(t *testing.T) {
	const (
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
		failUntilCall: threshold,
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

	_, runErr := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, runErr)

	assert.Equal(t, 0, runner.ReviewCalls(),
		"legacy post-land reviewer must not be invoked by execute-loop")
	assert.Equal(t, 1, runner.ExecCalls(),
		"executor must be invoked once for the successful attempt")

	events, err := store.Events(beadID)
	require.NoError(t, err)

	var (
		reviewEventCount    int
		reviewErrorCount    int
		manualRequiredCount int
	)
	for _, ev := range events {
		switch ev.Kind {
		case "review-error":
			reviewErrorCount++
		case "review":
			reviewEventCount++
		case "review-manual-required":
			manualRequiredCount++
		}
	}

	assert.Equal(t, 0, reviewErrorCount,
		"execute-loop must not emit legacy post-land review-error events")
	assert.Equal(t, 0, reviewEventCount,
		"execute-loop must not emit legacy post-land review events")
	assert.Equal(t, 0, manualRequiredCount,
		"execute-loop must not emit legacy post-land review-manual-required events")

	got, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", got.Status,
		"successful execute-loop attempt must close directly after pre-land review eligibility")

	// Defensive: a stale heartbeat ticker in the loop could outlive the
	// final iteration. Give it a beat to settle so test cleanup is clean.
	time.Sleep(10 * time.Millisecond)
}
