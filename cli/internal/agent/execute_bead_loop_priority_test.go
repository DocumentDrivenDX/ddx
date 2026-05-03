package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteLoop_ClaimsHighestPriorityFirst is the regression test for
// ddx-9d55601f AC #2: with a fixture queue of mixed priorities, a fresh
// worker MUST claim the P0 bead before any P1 or P2 bead.
//
// Investigation note (recorded in
// .ddx/executions/<run-id>/picker-priority-bug.md): the picker code path
// (ExecuteBeadWorker.nextCandidate -> Store.ReadyExecution ->
// readyFiltered(true) -> sortBeadsForQueue) is correct by construction —
// sortBeadsForQueue at cli/internal/bead/store.go:1473 sorts by
// Priority asc (0 first). The four "starved" P0 beads in the operator
// reproducer were actually parked on execute-loop-retry-after, so they
// were correctly EXCLUDED from ReadyExecution. This test exists to
// guarantee that the priority contract holds and to catch any future
// regression that quietly breaks it.
func TestExecuteLoop_ClaimsHighestPriorityFirst(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	// Insert in REVERSE priority order so a naive picker that returned
	// "first ready" would pick P2 first. ReadyExecution must sort.
	for _, b := range []*bead.Bead{
		{ID: "ddx-low", Title: "low priority", Priority: 2},
		{ID: "ddx-mid", Title: "mid priority", Priority: 1},
		{ID: "ddx-top", Title: "top priority", Priority: 0},
	} {
		require.NoError(t, store.Create(b))
	}

	var claimed []string
	var mu sync.Mutex
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			mu.Lock()
			claimed = append(claimed, beadID)
			mu.Unlock()
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged",
				SessionID: "sess-prio",
				ResultRev: "rev-" + beadID,
			}, nil
		}),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-prio"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, 1, result.Attempts, "Once=true should claim exactly one bead")
	require.Equal(t, []string{"ddx-top"}, claimed,
		"fresh worker must claim the P0 bead first; got %v", claimed)
}

// TestExecuteLoop_TwoWorkersBothClaimP0sBeforeP2s is the regression test for
// ddx-9d55601f AC #3: with a queue of 2xP0 + 4xP2, two concurrent workers
// MUST collectively claim both P0 beads before either claims any P2 bead.
//
// This guards against the historical claim-race risk: if a Claim() loser
// fell through to the next candidate without re-consulting ReadyExecution
// in priority order, it could end up claiming a P2 while a P0 was still
// available. Store.Claim is atomic (cli/internal/bead/store.go:558+ via
// WithLock), so the loser's next nextCandidate() call sees the contended
// P0 as in_progress (filtered out of ReadyExecution) AND the second P0
// still ready — and must pick that second P0 next.
func TestExecuteLoop_TwoWorkersBothClaimP0sBeforeP2s(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	for _, b := range []*bead.Bead{
		{ID: "ddx-p0-a", Title: "P0 first", Priority: 0},
		{ID: "ddx-p0-b", Title: "P0 second", Priority: 0},
		{ID: "ddx-p2-a", Title: "P2 a", Priority: 2},
		{ID: "ddx-p2-b", Title: "P2 b", Priority: 2},
		{ID: "ddx-p2-c", Title: "P2 c", Priority: 2},
		{ID: "ddx-p2-d", Title: "P2 d", Priority: 2},
	} {
		require.NoError(t, store.Create(b))
	}

	var (
		mu            sync.Mutex
		claimOrder    []string           // global serialization of claims
		claimPriority = map[string]int{} // bead -> priority for assertions
	)
	// Per-pair barrier: executor blocks until both workers in the current
	// pair have entered. This forces both Claim()s to land before either
	// worker returns from Run, so the second worker really does see the
	// first one's claim already committed.
	var (
		pairBlock      = make(chan struct{})
		pairReleased   atomic.Int32
		pairBaseClaims int // claimOrder length at the start of the current pair
	)
	for _, b := range []struct {
		id  string
		pri int
	}{
		{"ddx-p0-a", 0}, {"ddx-p0-b", 0},
		{"ddx-p2-a", 2}, {"ddx-p2-b", 2}, {"ddx-p2-c", 2}, {"ddx-p2-d", 2},
	} {
		claimPriority[b.id] = b.pri
	}

	// Executor records claim order then blocks until both workers have
	// claimed. This forces a true concurrent race: neither worker can
	// finish (and unblock its next iteration) until both have claimed
	// exactly one bead.
	exec := func(workerID string) ExecuteBeadExecutorFunc {
		return ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			mu.Lock()
			claimOrder = append(claimOrder, beadID)
			pairProgress := len(claimOrder) - pairBaseClaims
			block := pairBlock
			mu.Unlock()
			// Once both workers in the current pair have claimed, release
			// them so each can return from Run.
			if pairProgress == 2 && pairReleased.CompareAndSwap(0, 1) {
				close(block)
			}
			select {
			case <-block:
			case <-ctx.Done():
				return ExecuteBeadReport{}, ctx.Err()
			}
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged by " + workerID,
				SessionID: "sess-" + beadID,
				ResultRev: "rev-" + beadID,
			}, nil
		})
	}

	makeWorker := func(workerID string) *ExecuteBeadWorker {
		return &ExecuteBeadWorker{Store: store, Executor: exec(workerID)}
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-pair"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Each worker runs Once=true so it claims exactly one bead. We launch
	// two pairs back-to-back: the first pair must each claim a P0; the
	// second pair drains two of the P2s.
	runPair := func(label string) {
		mu.Lock()
		pairBaseClaims = len(claimOrder)
		pairBlock = make(chan struct{})
		pairReleased.Store(0)
		mu.Unlock()
		var wg sync.WaitGroup
		for _, id := range []string{label + "-x", label + "-y"} {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				w := makeWorker(id)
				_, err := w.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
	}

	runPair("p0")
	runPair("p2")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, claimOrder, 4, "both pairs should have claimed 4 beads total; got %v", claimOrder)

	first2 := claimOrder[:2]
	last2 := claimOrder[2:]
	for _, id := range first2 {
		require.Equal(t, 0, claimPriority[id],
			"first two claims must be P0 beads; got order=%v (priorities=%v %v %v %v)",
			claimOrder,
			claimPriority[claimOrder[0]], claimPriority[claimOrder[1]],
			claimPriority[claimOrder[2]], claimPriority[claimOrder[3]])
	}
	for _, id := range last2 {
		require.Equal(t, 2, claimPriority[id],
			"after both P0s are taken, remaining claims must be P2; got %v", claimOrder)
	}
}

// TestExecuteLoop_EmitsPickerPrioritySkipEvent verifies AC #4: when a worker
// passes over a higher-priority bead (because it is in the per-Run
// `attempted` map from a prior iteration) and claims a lower-priority bead,
// the loop emits a structured picker.priority_skip event naming the
// skipped bead and the reason. Without this, future starvation regressions
// would be invisible to operators.
func TestExecuteLoop_EmitsPickerPrioritySkipEvent(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())

	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-p0-skipped", Title: "P0 will fail preflight", Priority: 0}))
	require.NoError(t, store.Create(&bead.Bead{ID: "ddx-p2-claimed", Title: "P2 fallback", Priority: 2}))

	// Preflight returns nil for the P2 but a non-nil error for the P0.
	// The current loop responds to preflight rejection by returning early —
	// not what we want here, since we need to demonstrate priority_skip
	// after a higher-priority bead is parked in `attempted`. Instead, use
	// the pre-claim hook: it conditionally fails on the P0 and on the
	// second presentation of the P0 the loop adds it to attempted, so the
	// next candidate becomes the P2.
	hookCalls := 0
	preHook := func(ctx context.Context) error {
		hookCalls++
		// Always pass — we want to demonstrate the skip via a different
		// mechanism: use ComplexityGate instead. Set this aside; see below.
		return nil
	}
	_ = preHook

	// Use ComplexityGate to mark the P0 as "do not claim" — the loop adds
	// the bead to attempted after the gate signals shouldClaim=false (via
	// the implicit attempted-add prior to Claim is bypassed in the
	// not-shouldClaim path; instead the gate's own bookkeeping is its
	// concern). Here we simulate by having the gate return shouldClaim=true
	// for both, but with the P0 "already attempted" by seeding the loop a
	// different way.
	//
	// Simplest: drive two iterations. First iteration: both ready. Loop
	// claims P0, executor returns no_changes which (per loop semantics)
	// loops back. Then on second iteration, P0 is now in `attempted`
	// (added pre-Claim), so picker selects P2. The P2 claim should emit
	// picker.priority_skip naming the P0 with reason=in_attempted.
	var calls atomic.Int32
	executor := ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
		calls.Add(1)
		if beadID == "ddx-p0-skipped" {
			// no_changes leaves bead open. The loop adds it to attempted
			// (line ~480) BEFORE Claim, so on the next iteration the
			// picker will skip it with reason=in_attempted.
			return ExecuteBeadReport{
				BeadID: beadID,
				Status: ExecuteBeadStatusNoChanges,
				Detail: "nothing to do",
			}, nil
		}
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: "sess",
			ResultRev: "rev",
		}, nil
	})

	worker := &ExecuteBeadWorker{Store: store, Executor: executor}

	var sink eventCaptureWriter
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker-skip"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	// Drain the queue: both beads handled in one Run (Once=false, but we
	// stop when no candidates remain since PollInterval=0).
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		EventSink:    &sink,
		SessionID:    "sess-test",
		PollInterval: 0,
	})
	require.NoError(t, err)

	// Expect at least one picker.priority_skip event with chosen=ddx-p2-claimed
	// and skipped containing ddx-p0-skipped with reason=in_attempted.
	found := false
	for _, evt := range sink.events() {
		if evt["type"] != "picker.priority_skip" {
			continue
		}
		data, ok := evt["data"].(map[string]any)
		if !ok {
			continue
		}
		if data["chosen_bead_id"] != "ddx-p2-claimed" {
			continue
		}
		skipped, ok := data["skipped"].([]any)
		if !ok || len(skipped) == 0 {
			continue
		}
		first, _ := skipped[0].(map[string]any)
		if first["bead_id"] == "ddx-p0-skipped" && first["reason"] == "in_attempted" {
			found = true
			break
		}
	}
	require.True(t, found,
		"expected picker.priority_skip event for chosen=ddx-p2-claimed skipping ddx-p0-skipped (reason=in_attempted); got events:\n%s",
		sink.dump())
}

// eventCaptureWriter buffers JSONL events emitted via writeLoopEvent so tests
// can assert on the structured stream without parsing the human log.
type eventCaptureWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *eventCaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *eventCaptureWriter) events() []map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(string(w.buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (w *eventCaptureWriter) dump() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

// helper used when we want fmt.Sprintf in test scaffolding without importing
// fmt at the top (kept for parity with sibling test files).
var _ = fmt.Sprintf

// helper to silence the time import when tests don't need it directly.
var _ = time.Second
