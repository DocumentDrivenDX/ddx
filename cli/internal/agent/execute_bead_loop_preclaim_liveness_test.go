//go:build !windows

package agent

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkLoop_PreClaimDecompositionPublishesCandidateResolvingState blocks a
// hermetic decomposition hook and proves the sidecar reports the candidate bead
// ID, phase=resolving, and a non-zero last_activity_at before any claim or lease
// exists. Preclaim resolving must not invent an implementation attempt_id or
// create a claim lease for the candidate (ddx-9205ea9b).
func TestWorkLoop_PreClaimDecompositionPublishesCandidateResolvingState(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-candidate-resolving",
		Title:      "Expose candidate resolving state",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionPublishesCandidateResolvingState\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	entered := make(chan struct{})
	release := make(chan struct{})
	var execCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run while preclaim candidate resolving is published")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split for candidate resolving state coverage",
		Children: []PreClaimDecompositionChild{
			{
				Title:       "Child candidate resolving",
				Description: "PROBLEM\nChild\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n",
				Acceptance:  "1. TestChildCandidateResolving\n2. cd cli && go test ./internal/agent/...",
			},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestWorkLoop_PreClaimDecompositionPublishesCandidateResolvingState", Coverage: "covered by Child candidate resolving AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/...", Coverage: "covered by Child candidate resolving AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	projectRoot := store.Dir
	sessionID := "sess-preclaim-candidate-resolving"
	type runResult struct {
		attempts int
		err      error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			ProjectRoot:  projectRoot,
			SessionID:    sessionID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeTooLargeDecomposed,
					Detail:  "too large; split required",
				}, nil
			},
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return decomp, nil
			},
		})
		attempts := 0
		if result != nil {
			attempts = result.Attempts
		}
		done <- runResult{attempts: attempts, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decomposition hook never entered")
	}

	// While the hook is blocked (pre-claim), sidecar must expose candidate
	// resolving state with an initialized last_activity_at.
	var rec workerstatus.LivenessRecord
	require.Eventually(t, func() bool {
		got, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		if got.CurrentBead != candidate.ID || got.Phase != "resolving" || got.LastActivityAt.IsZero() {
			return false
		}
		rec = got
		return true
	}, 2*time.Second, 5*time.Millisecond,
		"sidecar must publish candidate bead ID, phase=resolving, and non-zero last_activity_at before claim")

	assert.Equal(t, candidate.ID, rec.CurrentBead)
	assert.Equal(t, "resolving", rec.Phase)
	assert.False(t, rec.LastActivityAt.IsZero(), "last_activity_at must be initialized for candidate resolving")
	// Candidate resolving is published without an implementation attempt identity.
	// The worker may hold a pre-dispatch ownership lease (ClaimWithOptions) so
	// concurrent workers skip the candidate, but that lease must not surface as
	// an implementation attempt_id or increment attempt counters.
	assert.Empty(t, rec.AttemptID, "candidate resolving must not invent an implementation attempt_id")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"tracked lifecycle must remain open during preclaim resolving (ClaimWithOptions does not promote to in_progress)")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls),
		"implementation executor must not run during candidate resolving")

	close(release)
	gotRun := <-done
	require.NoError(t, gotRun.err)
	assert.Equal(t, 0, gotRun.attempts,
		"preclaim decomposition must not increment implementation-attempt counters")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls),
		"executor must never run for pure candidate resolving decomposition")
}

// TestWorkLoop_PreClaimDecompositionPublishesProviderChildMetadata blocks a
// hermetic decomposition hook and proves the sidecar includes provider-child
// metadata for the candidate while phase=resolving, excluding processes that
// predate the hook baseline or belong to another worker.
func TestWorkLoop_PreClaimDecompositionPublishesProviderChildMetadata(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-provider-meta",
		Title:      "Too large for one attempt",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionPublishesProviderChildMetadata\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	// Three synthetic providers: baseline (pre-hook), foreign (other worker),
	// and hook-introduced (must appear in candidate metadata).
	const (
		baselinePID  = 11001
		foreignPID   = 22002
		hookChildPID = 33003
	)
	var (
		mu           sync.Mutex
		hookActive   bool
		observedPIDs []int
	)
	restoreScanner := providerChildScanner
	t.Cleanup(func() { providerChildScanner = restoreScanner })
	providerChildScanner = func(_ context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
		// rootPID-scoped scan: only report foreignPID when the caller asks
		// for a different worker root (other-worker exclusion).
		mu.Lock()
		active := hookActive
		mu.Unlock()
		var procs []providerChildProcess
		if rootPID == os.Getpid() {
			// Always present under this worker before and during the hook.
			procs = append(procs, providerChildProcess{
				PID: baselinePID, Provider: "claude", StartedAt: now.Add(-time.Minute),
			})
			if active {
				procs = append(procs, providerChildProcess{
					PID: hookChildPID, Provider: "codex", StartedAt: now,
				})
			}
		} else {
			// Other worker's tree — must never appear in candidate metadata.
			procs = append(procs, providerChildProcess{
				PID: foreignPID, Provider: "gemini", StartedAt: now,
			})
		}
		mu.Lock()
		for _, p := range procs {
			observedPIDs = append(observedPIDs, p.PID)
		}
		mu.Unlock()
		return procs, nil
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var execCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run while preclaim decomposition is in progress")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split for provider-child metadata coverage",
		Children: []PreClaimDecompositionChild{
			{Title: "Child A", Description: "PROBLEM\nChild A\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n", Acceptance: "1. TestChildA\n2. cd cli && go test ./internal/agent/..."},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestWorkLoop_PreClaimDecompositionPublishesProviderChildMetadata", Coverage: "covered by Child A AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/...", Coverage: "covered by Child A AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	projectRoot := store.Dir
	sessionID := "sess-preclaim-provider-meta"
	done := make(chan error, 1)
	go func() {
		_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			ProjectRoot:  projectRoot,
			SessionID:    sessionID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeTooLargeDecomposed,
					Detail:  "too large; split required",
				}, nil
			},
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				mu.Lock()
				hookActive = true
				mu.Unlock()
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return decomp, nil
			},
		})
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decomposition hook never entered")
	}

	// While the hook is blocked, sidecar must report candidate + resolving +
	// only the hook-introduced provider child.
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		if rec.CurrentBead != candidate.ID || rec.Phase != "resolving" {
			return false
		}
		for _, child := range rec.ProviderChildren {
			if child.PID == hookChildPID {
				return true
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond, "sidecar must publish candidate provider-child metadata while phase=resolving")

	rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	assert.Equal(t, candidate.ID, rec.CurrentBead)
	assert.Equal(t, "resolving", rec.Phase)
	assert.Empty(t, rec.AttemptID, "preclaim resolving must not assign an implementation attempt_id")

	var pids []int
	for _, child := range rec.ProviderChildren {
		pids = append(pids, child.PID)
	}
	assert.Contains(t, pids, hookChildPID, "hook-introduced provider child must appear")
	assert.NotContains(t, pids, baselinePID, "pre-baseline provider process must be excluded")
	assert.NotContains(t, pids, foreignPID, "other-worker provider process must be excluded")

	// Prove no implementation attempt / executor activity while resolving.
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "tracked status must remain open during preclaim decomposition")

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must never run for pure decomposition")
}

// TestWorkLoop_PreClaimDecompositionPublishesResolvingLiveness proves the
// combined sidecar report contains candidate bead ID, phase=resolving,
// refreshed last_activity_at, and provider-child metadata while no
// implementation attempt identity exists.
func TestWorkLoop_PreClaimDecompositionPublishesResolvingLiveness(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-resolving",
		Title:      "Publish resolving liveness",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionPublishesResolvingLiveness\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	const hookChildPID = 44004
	var hookActive atomic.Bool
	restoreScanner := providerChildScanner
	t.Cleanup(func() { providerChildScanner = restoreScanner })
	providerChildScanner = func(_ context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
		if rootPID != os.Getpid() || !hookActive.Load() {
			return nil, nil
		}
		return []providerChildProcess{{
			PID: hookChildPID, Provider: "codex", StartedAt: now,
		}}, nil
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var execCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run for preclaim resolving liveness")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "combined resolving liveness coverage",
		Children: []PreClaimDecompositionChild{
			{Title: "Child resolving", Description: "PROBLEM\nChild\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n", Acceptance: "1. TestChildResolving\n2. cd cli && go test ./internal/agent/..."},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestWorkLoop_PreClaimDecompositionPublishesResolvingLiveness", Coverage: "covered by Child resolving AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/...", Coverage: "covered by Child resolving AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	projectRoot := store.Dir
	sessionID := "sess-preclaim-resolving"
	type runResult struct {
		attempts int
		err      error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			TargetBeadID: candidate.ID,
			ProjectRoot:  projectRoot,
			SessionID:    sessionID,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeTooLargeDecomposed,
					Detail:  "too large; split required",
				}, nil
			},
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				hookActive.Store(true)
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return decomp, nil
			},
		})
		attempts := 0
		if result != nil {
			attempts = result.Attempts
		}
		done <- runResult{attempts: attempts, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decomposition hook never entered")
	}

	// First snapshot: candidate + resolving + provider child.
	var first workerstatus.LivenessRecord
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		if rec.CurrentBead != candidate.ID || rec.Phase != "resolving" || rec.LastActivityAt.IsZero() {
			return false
		}
		for _, child := range rec.ProviderChildren {
			if child.PID == hookChildPID {
				first = rec
				return true
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond, "combined resolving sidecar must include candidate, phase, and provider-child metadata")

	assert.Empty(t, first.AttemptID, "resolving liveness must not invent an implementation attempt_id")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))

	// Heartbeat must refresh last_activity_at while the hook remains blocked.
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		return rec.CurrentBead == candidate.ID &&
			rec.Phase == "resolving" &&
			rec.LastActivityAt.After(first.LastActivityAt)
	}, 2*time.Second, 5*time.Millisecond, "last_activity_at must refresh while preclaim decomposition runs")

	// Still no implementation attempt identity / executor after refresh.
	rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	assert.Empty(t, rec.AttemptID)
	assert.NotEmpty(t, rec.ProviderChildren)
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)

	close(release)
	gotRun := <-done
	require.NoError(t, gotRun.err)
	assert.Equal(t, 0, gotRun.attempts, "decomposition must not count as implementation attempts")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
}
