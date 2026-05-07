package graphql

import (
	"context"
	"encoding/json"
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

// capturingActionDispatcher records the projectRoot and rawArgs passed to
// DispatchWorker so the test can assert that StartWorker resolved its
// arguments through .ddx/config.yaml + CLIOverrides before dispatch.
type capturingActionDispatcher struct {
	calls []capturedDispatch
}

type capturedDispatch struct {
	kind        string
	projectRoot string
	args        map[string]string
}

func (c *capturingActionDispatcher) DispatchWorker(ctx context.Context, kind string, projectRoot string, args *string) (*WorkerDispatchResult, error) {
	rec := capturedDispatch{kind: kind, projectRoot: projectRoot, args: map[string]string{}}
	if args != nil && *args != "" {
		_ = json.Unmarshal([]byte(*args), &rec.args)
	}
	c.calls = append(c.calls, rec)
	return &WorkerDispatchResult{ID: "worker-graphql-e2e", State: "queued", Kind: kind}, nil
}

func (c *capturingActionDispatcher) DispatchPlugin(ctx context.Context, projectRoot string, name string, action string, scope string) (*PluginDispatchResult, error) {
	return &PluginDispatchResult{ID: "noop", State: "queued", Action: action}, nil
}

func (c *capturingActionDispatcher) StopWorker(ctx context.Context, id string) (*WorkerLifecycleResult, error) {
	return &WorkerLifecycleResult{ID: id, State: "stopped", Kind: "execute-loop"}, nil
}

// TestReviewRetryThresholdFromConfigGraphQL is the SD-024 Stage 1 behavioral
// proof that the GraphQL StartWorker resolver flows configuration through
// config.LoadAndResolve, and that the review_max_retries knob in
// .ddx/config.yaml drives the loop's behavior end-to-end on the GraphQL
// dispatch path.
//
// The test exercises two halves of the production wiring:
//
//  1. Resolver-side: invoke mutationResolver.StartWorker against a real
//     on-disk .ddx/config.yaml. Assert that the dispatched args reflect
//     resolved config values (proving LoadAndResolve was actually called)
//     and that the resolver no longer pre-fills hardcoded "smart"/"medium"
//     defaults that would shadow the on-disk config.
//
//  2. Loop-side: drive ExecuteBeadWorker.RunWithConfig with the same
//     ResolvedConfig the production runWorker would produce from the same
//     project root, using a test-local reviewFailureRunner. Assert the
//     bead closes on the (FailUntilCall+1)th attempt with no
//     review-manual-required event — proving the threshold from config
//     drives observable loop behavior on the GraphQL path.
//
// Configured values:
//   - .ddx/config.yaml: review_max_retries: 5
//   - StartWorkerInput: harness: claude
//   - fixture: FailUntilCall=4 (attempts 1-4 return reviewer error,
//     attempt 5 returns APPROVE)
func TestReviewRetryThresholdFromConfigGraphQL(t *testing.T) {
	const (
		failUntil   = 4
		threshold   = 5
		beadID      = "ddx-gql-rmr-001"
		fixedRev    = "cafebabe00112233"
		assigneeStr = "graphql-e2e-worker"
		harnessCfg  = "claude"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ".ddx")
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))

	// Real on-disk .ddx/config.yaml — this is the file the GraphQL
	// dispatch path's config.LoadAndResolve call reads. The presence of
	// review_max_retries: 5 is the entire config premise of the test.
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
	require.NoError(t, store.Create(&bead.Bead{ID: beadID, Title: "graphql e2e review-retry threshold", Priority: 0}))

	// Half 1: drive the GraphQL resolver and assert it flowed through
	// LoadAndResolve before dispatching.
	dispatcher := &capturingActionDispatcher{}
	resolver := &Resolver{
		WorkingDir: projectRoot,
		Actions:    dispatcher,
	}
	mr := &mutationResolver{Resolver: resolver}

	res, err := mr.StartWorker(context.Background(), StartWorkerInput{ProjectID: "", Harness: strPtr(harnessCfg)})
	require.NoError(t, err, "StartWorker must succeed against a valid on-disk config")
	require.NotNil(t, res)
	require.Len(t, dispatcher.calls, 1, "StartWorker must dispatch exactly once")
	got := dispatcher.calls[0]
	assert.Equal(t, "execute-loop", got.kind)
	assert.Equal(t, projectRoot, got.projectRoot,
		"resolver must dispatch against the resolved project root")
	assert.Equal(t, harnessCfg, got.args["harness"],
		"input harness override (%s) must reach dispatcher args via LoadAndResolve",
		harnessCfg)
	assert.Equal(t, "smart", got.args["profile"],
		"with no agent.profile configured and no input override, the "+
			"resolver's legacy fallback of \"smart\" must continue to apply")

	// Half 2: drive ExecuteBeadWorker.RunWithConfig with the same
	// ResolvedConfig production would produce. This mirrors what
	// runWorker does after the GraphQL resolver hands off — the test
	// observes loop behavior end-to-end without depending on the server
	// package (which would create an import cycle from this package).
	overrides := config.CLIOverrides{Assignee: assigneeStr}
	rcfg, err := config.LoadAndResolve(projectRoot, overrides)
	require.NoError(t, err)
	require.Equal(t, threshold, rcfg.ReviewMaxRetries(),
		"LoadAndResolve must surface review_max_retries from .ddx/config.yaml")

	runner := &reviewFailureRunner{
		resultRev:     fixedRev,
		failUntilCall: failUntil,
	}
	worker := &agent.ExecuteBeadWorker{
		Store:    store,
		Executor: runner.Executor(),
		Reviewer: runner.Reviewer(),
	}
	runtime := agent.ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(), // execute-bead worktree base; isolated from project root.
	}

	const totalIterations = failUntil + 1
	for i := 0; i < totalIterations; i++ {
		_, runErr := worker.Run(context.Background(), rcfg, runtime)
		require.NoErrorf(t, runErr, "iteration %d: RunWithConfig", i+1)
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

	gotBead, err := store.Get(beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", gotBead.Status,
		"after APPROVE on attempt %d the loop must close the bead", totalIterations)

	// Defensive: a stale heartbeat ticker in the loop could outlive the
	// final iteration. Give it a beat to settle so test cleanup is clean.
	time.Sleep(10 * time.Millisecond)
}
