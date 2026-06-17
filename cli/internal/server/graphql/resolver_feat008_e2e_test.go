package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIntegrationInShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short")
	}
}

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
	calls     []capturedDispatch
	stopCalls []string
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
	return &WorkerDispatchResult{
		ID:    "worker-graphql-e2e",
		State: "queued",
		Kind:  kind,
		Workers: []*WorkerLifecycleResult{{
			ID:    "worker-graphql-e2e",
			State: "queued",
			Kind:  kind,
		}},
	}, nil
}

func (c *capturingActionDispatcher) DispatchPlugin(ctx context.Context, projectRoot string, name string, action string, scope string) (*PluginDispatchResult, error) {
	return &PluginDispatchResult{ID: "noop", State: "queued", Action: action}, nil
}

func (c *capturingActionDispatcher) StopWorker(ctx context.Context, id string) (*WorkerLifecycleResult, error) {
	c.stopCalls = append(c.stopCalls, id)
	return &WorkerLifecycleResult{ID: id, State: "stopped", Kind: "work"}, nil
}

func intPtr(v int) *int { return &v }

func TestStartWorker_ForwardsCount(t *testing.T) {
	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"`+"\n"), 0o644))

	dispatcher := &capturingActionDispatcher{}
	mr := &mutationResolver{Resolver: &Resolver{
		WorkingDir: projectRoot,
		Actions:    dispatcher,
	}}

	res, err := mr.StartWorker(context.Background(), StartWorkerInput{
		ProjectID: "",
		Count:     intPtr(2),
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, dispatcher.calls, 1)
	assert.Equal(t, "2", dispatcher.calls[0].args["count"])
}

func TestGraphQLStartWorkerFlowsConfigToWorkerDispatch(t *testing.T) {
	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"`+"\n"), 0o644))

	dispatcher := &capturingActionDispatcher{}
	mr := &mutationResolver{Resolver: &Resolver{
		WorkingDir: projectRoot,
		Actions:    dispatcher,
	}}

	res, err := mr.StartWorker(context.Background(), StartWorkerInput{
		ProjectID: "",
		Harness:   strPtr("claude"),
		Provider:  strPtr("anthropic"),
		Model:     strPtr("opus-4.7"),
		Profile:   strPtr("cheap"),
		Effort:    strPtr("low"),
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, dispatcher.calls, 1)

	got := dispatcher.calls[0]
	assert.Equal(t, "work", got.kind)
	assert.Equal(t, projectRoot, got.projectRoot)
	assert.Equal(t, "claude", got.args["harness"])
	assert.Equal(t, "anthropic", got.args["provider"])
	assert.Equal(t, "opus-4.7", got.args["model"])
	assert.Equal(t, "cheap", got.args["profile"])
	assert.Equal(t, "low", got.args["effort"])
	assert.Equal(t, "watch", got.args["mode"])
	assert.Equal(t, "30s", got.args["idle_interval"])
	assert.NotContains(t, got.args, "count", "omitted count must preserve adapter default of one worker")
}

type workerForwardStateProvider struct {
	*mutationTestStateProvider
	workers map[string]*Worker
}

func (p *workerForwardStateProvider) GetWorkersGraphQL(projectID string) []*Worker {
	out := make([]*Worker, 0, len(p.workers))
	for _, worker := range p.workers {
		if worker == nil {
			continue
		}
		if projectID != "" {
			proj, ok := p.GetProjectSnapshotByID(projectID)
			if !ok || !sameWorkerProjectRoot(proj.Path, worker.ProjectRoot) {
				continue
			}
		}
		copyWorker := *worker
		out = append(out, &copyWorker)
	}
	return out
}

func (p *workerForwardStateProvider) GetWorkerGraphQL(id string) (*Worker, bool) {
	worker, ok := p.workers[id]
	if !ok || worker == nil {
		return nil, false
	}
	copyWorker := *worker
	return &copyWorker, true
}

type workerMutationFederation struct {
	spokes []federation.SpokeRecord
	calls  []*federation.ForwardMutationRequest
}

func (m *workerMutationFederation) Spokes() []federation.SpokeRecord {
	out := make([]federation.SpokeRecord, len(m.spokes))
	copy(out, m.spokes)
	return out
}

func (m *workerMutationFederation) FanOut(context.Context, *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return &federation.FanOutResult{}, nil
}

func (m *workerMutationFederation) ForwardMutation(_ context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	m.calls = append(m.calls, req)
	registry := federation.NewRegistry()
	for _, spoke := range m.spokes {
		if err := registry.UpsertSpoke(spoke); err != nil {
			return nil, err
		}
	}
	owner, err := federation.RouteMutationToProjectOwner(registry, req.TargetProjectID)
	if err != nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner.NodeID != req.TargetNodeID {
		return nil, federation.ErrForwardMutationBroadcastLike
	}
	if owner.Status == federation.StatusOffline {
		return nil, federation.ErrForwardMutationOffline
	}
	if owner.Status == federation.StatusStale {
		return nil, federation.ErrForwardMutationStale
	}
	if !hasWriteCapability(owner.Capabilities) {
		return nil, federation.ErrForwardMutationReadOnly
	}

	var envelope struct {
		Query     string          `json:"query"`
		Variables json.RawMessage `json:"variables"`
	}
	if err := json.Unmarshal(req.Body, &envelope); err != nil {
		return nil, err
	}
	var body map[string]any
	switch {
	case strings.Contains(envelope.Query, "startWorker"):
		body = map[string]any{
			"data": map[string]any{
				"startWorker": map[string]any{
					"id":    "worker-spoke-started",
					"state": "queued",
					"kind":  "work",
					"workers": []map[string]any{{
						"id":    "worker-spoke-started",
						"state": "queued",
						"kind":  "work",
					}},
				},
			},
		}
	case strings.Contains(envelope.Query, "stopWorker"):
		body = map[string]any{
			"data": map[string]any{
				"stopWorker": map[string]any{
					"id":    "worker-spoke-running",
					"state": "stopping",
					"kind":  "work",
				},
			},
		}
	default:
		return nil, fmt.Errorf("unexpected worker mutation: %s", envelope.Query)
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &federation.ForwardMutationResponse{
		OriginIdentity:  req.OriginIdentity,
		ForwardingPath:  append([]string(nil), req.ForwardingPath...),
		RequestID:       req.RequestID,
		IdempotencyKey:  req.IdempotencyKey,
		TargetNodeID:    req.TargetNodeID,
		TargetProjectID: req.TargetProjectID,
		StatusCode:      http.StatusOK,
		Headers:         http.Header{"Content-Type": []string{"application/json"}},
		Body:            rawBody,
	}, nil
}

func newWorkerForwardResolver(t *testing.T, spokeStatus federation.SpokeStatus) (*mutationResolver, *capturingActionDispatcher, *workerMutationFederation, string) {
	t.Helper()
	localRoot := t.TempDir()
	spokeRoot := t.TempDir()
	dispatcher := &capturingActionDispatcher{}
	fed := &workerMutationFederation{spokes: []federation.SpokeRecord{{
		NodeID:       "spoke-node",
		Name:         "Spoke Node",
		URL:          "https://spoke.example.test",
		Status:       spokeStatus,
		ProjectIDs:   []string{"proj-spoke"},
		Capabilities: []string{"write"},
	}}}
	state := &workerForwardStateProvider{
		mutationTestStateProvider: &mutationTestStateProvider{
			node: NodeStateSnapshot{ID: "hub-node", Name: "Hub Node"},
			projects: []*Project{{
				ID:   "proj-hub",
				Name: "Hub Project",
				Path: localRoot,
			}, {
				ID:   "proj-spoke",
				Name: "Spoke Project",
				Path: spokeRoot,
			}},
		},
		workers: map[string]*Worker{
			"worker-spoke-running": {
				ID:          "worker-spoke-running",
				Kind:        "work",
				State:       "running",
				ProjectRoot: spokeRoot,
			},
		},
	}
	resolver := &mutationResolver{Resolver: &Resolver{
		State:      state,
		WorkingDir: localRoot,
		NodeID:     "hub-node",
		Actions:    dispatcher,
		Federation: fed,
	}}
	return resolver, dispatcher, fed, spokeRoot
}

func TestFederatedStartWorker_ForwardsToOwner(t *testing.T) {
	resolver, dispatcher, fed, _ := newWorkerForwardResolver(t, federation.StatusActive)

	result, err := resolver.StartWorker(context.Background(), StartWorkerInput{
		ProjectID: "proj-spoke",
		Count:     intPtr(1),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "worker-spoke-started", result.ID)
	require.Len(t, fed.calls, 1)
	assert.Equal(t, "spoke-node", fed.calls[0].TargetNodeID)
	assert.Equal(t, "proj-spoke", fed.calls[0].TargetProjectID)
	assert.Contains(t, string(fed.calls[0].Body), "startWorker")
	assert.Empty(t, dispatcher.calls, "hub must not dispatch spoke-owned startWorker locally")
}

func TestFederatedStopWorker_ForwardsToOwner(t *testing.T) {
	resolver, dispatcher, fed, _ := newWorkerForwardResolver(t, federation.StatusActive)

	result, err := resolver.StopWorker(context.Background(), "worker-spoke-running")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "worker-spoke-running", result.ID)
	assert.Equal(t, "stopping", result.State)
	require.Len(t, fed.calls, 1)
	assert.Equal(t, "spoke-node", fed.calls[0].TargetNodeID)
	assert.Equal(t, "proj-spoke", fed.calls[0].TargetProjectID)
	assert.Contains(t, string(fed.calls[0].Body), "stopWorker")
	assert.Empty(t, dispatcher.stopCalls, "hub must not stop spoke-owned worker locally")
}

func TestFederatedStartWorker_RejectsOfflineSpoke(t *testing.T) {
	resolver, dispatcher, fed, _ := newWorkerForwardResolver(t, federation.StatusOffline)

	result, err := resolver.StartWorker(context.Background(), StartWorkerInput{
		ProjectID: "proj-spoke",
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "offline")
	require.Len(t, fed.calls, 1)
	assert.Empty(t, dispatcher.calls, "offline spoke target must not create a local worker row")
}

// TestReviewRetryThresholdFromConfigGraphQL is the SD-024 Stage 1 configuration
// wiring proof that the GraphQL StartWorker resolver flows configuration through
// config.LoadAndResolve.
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
//     project root. Candidate-cycle pre-land review now owns close eligibility,
//     so work must not invoke the legacy post-land reviewer/retry path.
//
// Configured values:
//   - .ddx/config.yaml: review_max_retries: 5
//   - StartWorkerInput: harness: claude
//   - fixture: failUntilCall=5, which must remain unused because legacy
//     post-land review is retired
func TestReviewRetryThresholdFromConfigGraphQL(t *testing.T) {
	skipIntegrationInShort(t)
	const (
		threshold   = 5
		beadID      = "ddx-gql-rmr-001"
		fixedRev    = "cafebabe00112233"
		assigneeStr = "graphql-e2e-worker"
		harnessCfg  = "claude"
		providerCfg = "anthropic"
		modelCfg    = "opus-4.7"
		timeoutCfg  = "53s"
	)

	projectRoot := t.TempDir()
	ddxDir := filepath.Join(projectRoot, ddxroot.DirName)
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
	require.NoError(t, store.Init(context.Background()))
	require.NoError(t, store.Create(context.Background(), &bead.Bead{ID: beadID, Title: "graphql e2e review-retry threshold", Priority: 0}))

	// Half 1: drive the GraphQL resolver and assert it flowed through
	// LoadAndResolve before dispatching.
	dispatcher := &capturingActionDispatcher{}
	resolver := &Resolver{
		WorkingDir: projectRoot,
		Actions:    dispatcher,
	}
	mr := &mutationResolver{Resolver: resolver}

	res, err := mr.StartWorker(context.Background(), StartWorkerInput{
		ProjectID:      "",
		Harness:        strPtr(harnessCfg),
		Provider:       strPtr(providerCfg),
		Model:          strPtr(modelCfg),
		RequestTimeout: strPtr(timeoutCfg),
	})
	require.NoError(t, err, "StartWorker must succeed against a valid on-disk config")
	require.NotNil(t, res)
	require.Len(t, dispatcher.calls, 1, "StartWorker must dispatch exactly once")
	got := dispatcher.calls[0]
	assert.Equal(t, "work", got.kind)
	assert.Equal(t, projectRoot, got.projectRoot,
		"resolver must dispatch against the resolved project root")
	assert.Equal(t, harnessCfg, got.args["harness"],
		"input harness override (%s) must reach dispatcher args via LoadAndResolve",
		harnessCfg)
	assert.Equal(t, providerCfg, got.args["provider"],
		"input provider override (%s) must reach dispatcher args via LoadAndResolve",
		providerCfg)
	assert.Equal(t, modelCfg, got.args["model"],
		"input model override (%s) must reach dispatcher args via LoadAndResolve",
		modelCfg)
	assert.Equal(t, "smart", got.args["profile"],
		"with no agent.profile configured and no input override, the "+
			"resolver's legacy fallback of \"smart\" must continue to apply")
	assert.Equal(t, "watch", got.args["mode"],
		"GraphQL startWorker defaults server-managed workers to watch mode")
	assert.Equal(t, "30s", got.args["idle_interval"],
		"GraphQL startWorker defaults watch workers to a 30s idle interval")
	assert.Equal(t, timeoutCfg, got.args["request_timeout"],
		"GraphQL startWorker must forward requestTimeout to the worker spec")

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
		failUntilCall: threshold,
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

	_, runErr := worker.Run(context.Background(), rcfg, runtime)
	require.NoError(t, runErr)

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

	gotBead, err := store.Get(context.Background(), beadID)
	require.NoError(t, err)
	assert.Equal(t, "closed", gotBead.Status,
		"successful work attempt must close directly after candidate-cycle approval")

	// Defensive: a stale heartbeat ticker in the loop could outlive the
	// final iteration. Give it a beat to settle so test cleanup is clean.
	time.Sleep(10 * time.Millisecond)
}
