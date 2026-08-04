//go:build !windows

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
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

// TestWorkLoop_PreClaimDecompositionPreservesLoopStartOrdering proves the
// structured event stream still begins with loop.start when preclaim
// decomposition liveness is enabled, and that the resolving heartbeat arrives
// later without inventing an implementation attempt identity.
func TestWorkLoop_PreClaimDecompositionPreservesLoopStartOrdering(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-loop-ordering",
		Title:      "Preserve loop.start ordering",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionPreservesLoopStartOrdering\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHook := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	defer releaseHook()
	var execCalls int32
	sink := &eventCaptureWriter{}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run while preclaim resolving liveness is published")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split for loop.start ordering coverage",
		Children: []PreClaimDecompositionChild{
			{
				Title:       "Child ordering",
				Description: "PROBLEM\nChild\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n",
				Acceptance:  "1. TestChildOrdering\n2. cd cli && go test ./internal/agent/...",
			},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestWorkLoop_PreClaimDecompositionPreservesLoopStartOrdering", Coverage: "covered by Child ordering AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/...", Coverage: "covered by Child ordering AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	projectRoot := store.Dir
	sessionID := "sess-preclaim-loop-ordering"
	type runResult struct {
		attempts int
		err      error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:         true,
			EventSink:    sink,
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

	require.Eventually(t, func() bool {
		events := sink.events()
		if len(events) == 0 {
			return false
		}
		kind, _ := events[0]["type"].(string)
		return kind == "loop.start"
	}, 2*time.Second, 5*time.Millisecond, "loop.start must be the first structured event")

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		releaseHook()
		t.Fatal("decomposition hook never entered")
	}

	require.Eventually(t, func() bool {
		for _, event := range sink.events() {
			kind, _ := event["type"].(string)
			if kind != "worker.heartbeat" {
				continue
			}
			data, _ := event["data"].(map[string]any)
			if data["bead_id"] == candidate.ID && data["phase"] == "resolving" && data["attempt_id"] == "" {
				return true
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond, "preclaim resolving heartbeat must follow loop.start")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "preclaim resolving must not close or execute the bead")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must not run during preclaim resolving")

	releaseHook()
	gotRun := <-done
	require.NoError(t, gotRun.err)
	assert.Equal(t, 0, gotRun.attempts, "preclaim resolving must not count as an implementation attempt")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))
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

func TestWorkLoop_PreClaimDecompositionClearsTransientLiveness(t *testing.T) {
	makeDecomp := func(name string) *PreClaimDecomposition {
		return &PreClaimDecomposition{
			Rationale: "split for " + name,
			Children: []PreClaimDecompositionChild{
				{
					Title:       "Child " + name,
					Description: "PROBLEM\nChild\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n",
					Acceptance:  "1. TestChild" + name + "\n2. cd cli && go test ./internal/agent/...",
				},
			},
			ACMap: []ACMapEntry{
				{ParentAC: "1. parent", Coverage: "covered by Child " + name},
			},
		}
	}
	syntheticErr := errors.New("synthetic decomposition error")

	cases := []struct {
		name    string
		run     func(context.Context, context.CancelFunc, *PreClaimDecomposition) (*PreClaimDecomposition, error)
		wantErr error
		wantNil bool
	}{
		{
			name: "success",
			run: func(ctx context.Context, cancel context.CancelFunc, decomp *PreClaimDecomposition) (*PreClaimDecomposition, error) {
				return decomp, nil
			},
		},
		{
			name:    "deterministic_fallback",
			wantNil: true,
			run: func(ctx context.Context, cancel context.CancelFunc, decomp *PreClaimDecomposition) (*PreClaimDecomposition, error) {
				return nil, nil
			},
		},
		{
			name:    "error",
			wantErr: syntheticErr,
			run: func(ctx context.Context, cancel context.CancelFunc, decomp *PreClaimDecomposition) (*PreClaimDecomposition, error) {
				return nil, syntheticErr
			},
		},
		{
			name:    "timeout",
			wantErr: context.DeadlineExceeded,
			run: func(ctx context.Context, cancel context.CancelFunc, decomp *PreClaimDecomposition) (*PreClaimDecomposition, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		{
			name:    "cancellation",
			wantErr: context.Canceled,
			run: func(ctx context.Context, cancel context.CancelFunc, decomp *PreClaimDecomposition) (*PreClaimDecomposition, error) {
				cancel()
				return nil, context.Canceled
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			store := bead.NewStore(projectRoot)
			require.NoError(t, store.Init(context.Background()))

			candidate := &bead.Bead{
				ID:         "ddx-preclaim-reap",
				Title:      "Preclaim liveness cleanup",
				Acceptance: "1. TestWorkLoop_PreClaimDecompositionClearsTransientLiveness\n2. cd cli && go test ./internal/agent/...",
			}
			require.NoError(t, store.Create(context.Background(), candidate))

			foreignBead := &bead.Bead{
				ID:         "ddx-preclaim-foreign",
				Title:      "Foreign worker liveness",
				Acceptance: "1. TestWorkLoop_PreClaimDecompositionClearsTransientLiveness\n2. cd cli && go test ./internal/agent/...",
			}
			require.NoError(t, store.Create(context.Background(), foreignBead))

			providerDir := t.TempDir()
			baselinePID := startFakeProviderChild(t, providerDir, "claude")
			foreignPID := startFakeProviderChild(t, providerDir, "gemini")
			restoreScanner := providerChildScanner
			restoreTerminate := terminateProviderChild
			var terminated []int
			var mu sync.Mutex
			var hookChildPID int
			var hookActive atomic.Bool
			providerChildScanner = func(_ context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
				if rootPID != os.Getpid() {
					return []providerChildProcess{{
						PID:              foreignPID,
						Provider:         "gemini",
						Command:          "/usr/local/bin/gemini --sleep",
						StartedAt:        now.Add(-time.Minute),
						OwnerProviderPID: foreignPID,
					}}, nil
				}
				procs := []providerChildProcess{{
					PID:       baselinePID,
					Provider:  "claude",
					Command:   "/usr/local/bin/claude --sleep",
					StartedAt: now.Add(-time.Minute),
				}}
				if hookActive.Load() && hookChildPID > 0 {
					procs = append(procs, providerChildProcess{
						PID:       hookChildPID,
						Provider:  "codex",
						Command:   "/usr/local/bin/codex --sleep",
						StartedAt: now,
					})
				}
				return procs, nil
			}
			terminateProviderChild = func(pid int) {
				mu.Lock()
				terminated = append(terminated, pid)
				mu.Unlock()
				restoreTerminate(pid)
			}
			t.Cleanup(func() {
				providerChildScanner = restoreScanner
				terminateProviderChild = restoreTerminate
			})

			foreignWorkerID := "worker-foreign-preclaim"
			require.NoError(t, workerstatus.WriteLiveness(projectRoot, foreignWorkerID, workerstatus.LivenessRecord{
				WorkerID:       foreignWorkerID,
				ProjectRoot:    projectRoot,
				CurrentBead:    foreignBead.ID,
				AttemptID:      "att-foreign-preclaim",
				Phase:          "running",
				PID:            foreignPID,
				LastActivityAt: time.Now().UTC(),
			}))

			ctx, cancel := context.WithCancel(context.Background())
			if tc.name == "timeout" {
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
			}
			defer cancel()

			workerID := "worker-preclaim-reap"
			liveness := work.NewSidecarLivenessReporter(projectRoot, workerID, "sess-preclaim-reap", nil)
			decomp := makeDecomp(tc.name)
			got, err := runPreclaimDecompositionHookWithResolvingLiveness(
				ctx,
				func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
					assert.Equal(t, "ddx-preclaim-reap", beadID)
					hookChildPID = startFakeProviderChild(t, providerDir, "codex")
					hookActive.Store(true)
					return tc.run(ctx, cancel, decomp)
				},
				"ddx-preclaim-reap",
				liveness,
				"codex",
				"gpt-5",
				"",
				50*time.Millisecond,
				5*time.Millisecond,
				time.Now,
			)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr), "err = %v, want %v", err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantNil {
				assert.Nil(t, got)
			} else if tc.wantErr == nil {
				assert.Equal(t, decomp, got)
			}

			hookActive.Store(false)

			assert.Contains(t, terminated, hookChildPID, "hook-created provider child must be selected for cleanup")
			assert.NotContains(t, terminated, baselinePID, "pre-hook baseline provider child must not be selected for cleanup")
			assert.NotContains(t, terminated, foreignPID, "foreign worker provider child must not be selected for cleanup")
			assertProcessGone(t, hookChildPID)
			if !signalProcessAlive(foreignPID) {
				t.Fatalf("foreign worker provider child %d was reaped for %s", foreignPID, tc.name)
			}
			if !signalProcessAlive(baselinePID) {
				t.Fatalf("pre-hook baseline provider child %d was reaped for %s", baselinePID, tc.name)
			}

			require.Eventually(t, func() bool {
				rec, err := workerstatus.ReadLiveness(projectRoot, workerID)
				if err != nil {
					return false
				}
				return rec.CurrentBead == "" &&
					rec.AttemptID == "" &&
					rec.Phase == "" &&
					rec.ChildPID == 0 &&
					len(rec.ProviderChildren) == 0
			}, 2*time.Second, 5*time.Millisecond,
				"preclaim decomposition cleanup must clear the candidate sidecar after %s", tc.name)

			rec, err := workerstatus.ReadLiveness(projectRoot, workerID)
			require.NoError(t, err)
			assert.Empty(t, rec.CurrentBead)
			assert.Empty(t, rec.AttemptID)
			assert.Empty(t, rec.Phase)
			assert.Zero(t, rec.ChildPID)
			assert.Empty(t, rec.ProviderChildren)

			fresh, found, err := store.ClaimHeartbeatFresh(candidate.ID)
			require.NoError(t, err)
			assert.False(t, found, "preclaim decomposition must not leave a claim heartbeat for the candidate")
			assert.False(t, fresh, "preclaim decomposition must not leave a fresh claim heartbeat for the candidate")

			report := runWorkStatusJSONForTest(t, projectRoot)
			assert.Equal(t, 1, report.ActiveWork.Count)
			assert.NotContains(t, report.ActiveWork.BeadIDs, candidate.ID, "candidate must not remain visible in active work after cleanup")
			assert.Contains(t, report.ActiveWork.BeadIDs, foreignBead.ID, "foreign worker active work must remain visible after current-hook cleanup")
			require.Len(t, report.ActiveWork.Records, 1, "only the foreign worker should remain active after cleanup")
			assert.Equal(t, foreignWorkerID, report.ActiveWork.Records[0].WorkerID)
			assert.Equal(t, foreignBead.ID, report.ActiveWork.Records[0].BeadID)
			assert.Equal(t, "att-foreign-preclaim", report.ActiveWork.Records[0].AttemptID)
			assert.Equal(t, "running", report.ActiveWork.Records[0].Phase)

			foreignRec, err := workerstatus.ReadLiveness(projectRoot, foreignWorkerID)
			require.NoError(t, err)
			assert.Equal(t, foreignBead.ID, foreignRec.CurrentBead)
			assert.Equal(t, "att-foreign-preclaim", foreignRec.AttemptID)
			assert.Equal(t, "running", foreignRec.Phase)
			assert.Equal(t, foreignPID, foreignRec.PID)
		})
	}
}

type workStatusJSONReport struct {
	ActiveWork struct {
		Count   int      `json:"count"`
		BeadIDs []string `json:"bead_ids"`
		Records []struct {
			WorkerID  string `json:"worker_id"`
			BeadID    string `json:"bead_id"`
			AttemptID string `json:"attempt_id"`
			Phase     string `json:"phase"`
		} `json:"records"`
	} `json:"active_work"`
}

func runWorkStatusJSONForTest(t *testing.T, projectRoot string) workStatusJSONReport {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve test file path")
	cliDir := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	cmd := exec.Command("go", "run", ".", "work", "status", "--json", "--project", projectRoot)
	cmd.Dir = cliDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ddx work status --json failed: %v\n%s", err, out)
	}

	var report workStatusJSONReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse ddx work status --json output: %v\n%s", err, out)
	}
	return report
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

// TestWorkLoop_PreClaimDecompositionRefreshesResolvingLiveness blocks a
// hermetic decomposition hook long enough to observe at least two sidecar
// snapshots with advancing last_activity_at while phase=resolving for the
// same candidate bead ID. After the hook returns, the resolving heartbeat
// must stop cleanly without leaving active resolving status behind
// (ddx-943b1e1f).
func TestWorkLoop_PreClaimDecompositionRefreshesResolvingLiveness(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-refresh-liveness",
		Title:      "Refresh resolving liveness while decomposing",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionRefreshesResolvingLiveness\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	entered := make(chan struct{})
	release := make(chan struct{})
	var execCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Error("executor must not run while preclaim resolving liveness is heartbeating")
			return ExecuteBeadReport{}, nil
		}),
	}

	decomp := &PreClaimDecomposition{
		Rationale: "split for resolving liveness heartbeat coverage",
		Children: []PreClaimDecompositionChild{
			{
				Title:       "Child refresh liveness",
				Description: "PROBLEM\nChild\n\nROOT CAUSE\ncli/internal/agent/preclaim_decomp_liveness.go:1\n",
				Acceptance:  "1. TestChildRefreshLiveness\n2. cd cli && go test ./internal/agent/...",
			},
		},
		ACMap: []ACMapEntry{
			{ParentAC: "1. TestWorkLoop_PreClaimDecompositionRefreshesResolvingLiveness", Coverage: "covered by Child refresh liveness AC 1"},
			{ParentAC: "2. cd cli && go test ./internal/agent/...", Coverage: "covered by Child refresh liveness AC 2"},
		},
	}

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	projectRoot := store.Dir
	sessionID := "sess-preclaim-refresh-liveness"
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

	// Snapshot 1: candidate resolving with initialized last_activity_at.
	var first workerstatus.LivenessRecord
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		if rec.CurrentBead != candidate.ID || rec.Phase != "resolving" || rec.LastActivityAt.IsZero() {
			return false
		}
		first = rec
		return true
	}, 2*time.Second, 5*time.Millisecond,
		"first sidecar snapshot must report candidate + phase=resolving + last_activity_at")

	// Snapshot 2: same candidate still resolving, last_activity_at advanced.
	var second workerstatus.LivenessRecord
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		if rec.CurrentBead != candidate.ID || rec.Phase != "resolving" {
			return false
		}
		if !rec.LastActivityAt.After(first.LastActivityAt) {
			return false
		}
		second = rec
		return true
	}, 2*time.Second, 5*time.Millisecond,
		"second sidecar snapshot must advance last_activity_at while phase=resolving for the same candidate")

	assert.Equal(t, candidate.ID, second.CurrentBead)
	assert.Equal(t, "resolving", second.Phase)
	assert.True(t, second.LastActivityAt.After(first.LastActivityAt),
		"last_activity_at must strictly advance across resolving heartbeats")
	assert.Empty(t, second.AttemptID, "resolving heartbeat must not invent an implementation attempt_id")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls),
		"implementation executor must not run during resolving heartbeat")

	// Release the hook; heartbeat must stop and clear active resolving status.
	close(release)
	gotRun := <-done
	require.NoError(t, gotRun.err)
	assert.Equal(t, 0, gotRun.attempts,
		"preclaim decomposition must not count as an implementation attempt")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls))

	// After the hook returns and the Once loop exits, the sidecar must not
	// leave candidate+resolving active. ClearAttempt + OnTick writes empty
	// bead/phase; subsequent idle ticks (if any) must also not re-assert
	// resolving for this candidate.
	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		// Active resolving for this candidate is gone.
		if rec.CurrentBead == candidate.ID && rec.Phase == "resolving" {
			return false
		}
		return true
	}, 2*time.Second, 5*time.Millisecond,
		"heartbeat must stop after decomposition hook returns without leaving active resolving status")

	final, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	assert.False(t, final.CurrentBead == candidate.ID && final.Phase == "resolving",
		"post-hook sidecar must not retain active resolving for the candidate (bead=%q phase=%q)",
		final.CurrentBead, final.Phase)

	// Capture a post-completion timestamp and ensure resolving does not
	// reappear / re-advance under the candidate after the hook completed.
	postDoneAt := final.LastActivityAt
	time.Sleep(30 * time.Millisecond)
	after, err := workerstatus.ReadLiveness(projectRoot, sessionID)
	require.NoError(t, err)
	assert.False(t, after.CurrentBead == candidate.ID && after.Phase == "resolving",
		"resolving heartbeat must not resume for the candidate after the hook returns")
	if after.CurrentBead == candidate.ID && after.Phase == "resolving" && !postDoneAt.IsZero() {
		assert.False(t, after.LastActivityAt.After(postDoneAt),
			"last_activity_at must not keep advancing under active resolving after hook return")
	}
}

// TestWorkLoop_PreClaimDecompositionUsesConfiguredPreClaimTimeout blocks the
// preclaim decomposer longer than a short configured timeout and proves the
// loop returns inside that bound, emits a structured timeout warning, leaves
// the candidate unclaimed, and clears resolving liveness state after return.
func TestWorkLoop_PreClaimDecompositionUsesConfiguredPreClaimTimeout(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	candidate := &bead.Bead{
		ID:         "ddx-preclaim-timeout",
		Title:      "Bound preclaim decomposition timeout",
		Acceptance: "1. TestWorkLoop_PreClaimDecompositionUsesConfiguredPreClaimTimeout\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), candidate))

	entered := make(chan struct{})
	var execCalls int32

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			atomic.AddInt32(&execCalls, 1)
			t.Fatalf("executor must not run when preclaim decomposition times out")
			return ExecuteBeadReport{}, nil
		}),
	}

	projectRoot := store.Dir
	sessionID := "sess-preclaim-timeout"
	var sink bytes.Buffer

	cfgOpts := config.TestLoopConfigOpts{
		Assignee:              "worker",
		HeartbeatInterval:     5 * time.Millisecond,
		MaxDecompositionDepth: 3,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	type runResult struct {
		attempts int
		err      error
	}
	done := make(chan runResult, 1)
	start := time.Now()
	go func() {
		result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
			Once:            true,
			TargetBeadID:    candidate.ID,
			ProjectRoot:     projectRoot,
			SessionID:       sessionID,
			EventSink:       &sink,
			PreClaimTimeout: 30 * time.Millisecond,
			PreClaimIntakeHook: func(ctx context.Context, beadID string) (PreClaimIntakeResult, error) {
				return PreClaimIntakeResult{
					Outcome: PreClaimIntakeTooLargeDecomposed,
					Detail:  "too large; split required",
				}, nil
			},
			PostAttemptDecompositionHook: func(ctx context.Context, beadID string) (*PreClaimDecomposition, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
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

	gotRun := <-done
	elapsed := time.Since(start)
	require.NoError(t, gotRun.err)
	assert.Equal(t, 0, gotRun.attempts, "timeout must not count as an implementation attempt")
	assert.Less(t, elapsed, 2*time.Second, "preclaim decomposition must return within the configured timeout bound")
	assert.Equal(t, int32(0), atomic.LoadInt32(&execCalls), "executor must not run when decomposition times out")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "timed-out preclaim decomposition must leave the bead unclaimed")
	assert.Empty(t, got.Owner, "timed-out preclaim decomposition must not claim the bead")

	events := parseLoopEvents(t, sink.String())
	warns := loopEventDataByType(events, "pre_claim_intake.warn")
	require.NotEmpty(t, warns, "timeout must emit a structured pre_claim_intake.warn event")
	var sawTimeout bool
	for _, warn := range warns {
		if warn["reason"] == "decomposition_hook_timeout" {
			sawTimeout = true
			assert.Equal(t, "preclaim decomposition timed out after 30ms", warn["detail"])
			assert.Equal(t, "30ms", warn["timeout"])
		}
	}
	assert.True(t, sawTimeout, "timeout warning must carry decomposition_hook_timeout")

	durableEvents, err := store.Events(candidate.ID)
	require.NoError(t, err)
	var sawDurableTimeout bool
	for _, ev := range durableEvents {
		if ev.Kind == "intake.warn" && ev.Summary == "decomposition_hook_timeout" {
			sawDurableTimeout = true
			assert.Contains(t, ev.Body, "preclaim decomposition timed out after 30ms")
			break
		}
	}
	assert.True(t, sawDurableTimeout, "timeout must persist a durable intake.warn event")

	require.Eventually(t, func() bool {
		rec, err := workerstatus.ReadLiveness(projectRoot, sessionID)
		if err != nil {
			return false
		}
		return rec.CurrentBead != candidate.ID && rec.Phase != "resolving" && rec.AttemptID == ""
	}, 2*time.Second, 5*time.Millisecond, "resolving liveness must clear after the timeout returns")
}
