package graphql_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
)

// newRunRequeueResolver wires a Resolver with a real bead store, a runs
// provider seeded with the supplied runs, and a fresh idempotency cache.
func newRunRequeueResolver(t *testing.T, runs []*ddxgraphql.Run) (*ddxgraphql.Resolver, *bead.Store, string) {
	t.Helper()
	workDir, store := setupIntegrationDir(t)
	base := newTestStateProvider(workDir, store)
	provider := &runsTestProvider{testStateProvider: base, all: runs}
	r, err := ddxgraphql.NewResolver(provider, workDir)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	r.RunRequeueIdempotency = ddxgraphql.NewMemoryIdempotencyCache()
	return r, store, workDir
}

// seedClosedBead writes a closed bead into the store and returns its id.
func seedClosedBead(t *testing.T, store *bead.Store, title string) string {
	t.Helper()
	b := &bead.Bead{Title: title, Status: bead.StatusOpen}
	if err := store.Create(b); err != nil {
		t.Fatalf("create bead: %v", err)
	}
	if err := store.Update(b.ID, func(x *bead.Bead) {
		x.Status = bead.StatusClosed
		x.Owner = "agent-prior"
	}); err != nil {
		t.Fatalf("close bead: %v", err)
	}
	return b.ID
}

// runForBead returns a run-layer Run pointing at beadID.
func runForBead(runID, beadID string) *ddxgraphql.Run {
	return &ddxgraphql.Run{
		ID:          runID,
		Layer:       ddxgraphql.RunLayerRun,
		Status:      "failure",
		BeadID:      &beadID,
		ChildRunIds: []string{},
	}
}

// TestRunRequeue_ReopensBeadAndAuditLogs covers the happy path: the
// originating bead is reopened (status=open, owner cleared) and a
// run_requeue audit event is appended on the originating bead.
func TestRunRequeue_ReopensBeadAndAuditLogs(t *testing.T) {
	r, store, _ := newRunRequeueResolver(t, nil)
	beadID := seedClosedBead(t, store, "requeue target")

	// Inject the run after we know beadID.
	provider := r.State.(*runsTestProvider)
	provider.all = []*ddxgraphql.Run{runForBead("run-001", beadID)}

	layer := "run"
	res, err := r.Mutation().RunRequeue(context.Background(), ddxgraphql.RunRequeueInput{
		RunID:          "run-001",
		IdempotencyKey: "key-A",
		Layer:          &layer,
	})
	if err != nil {
		t.Fatalf("RunRequeue: %v", err)
	}
	if res.Deduplicated {
		t.Fatalf("first call must not be deduplicated")
	}
	if res.Bead == nil || res.Bead.ID != beadID {
		t.Fatalf("expected bead %s, got %+v", beadID, res.Bead)
	}
	if res.Bead.Status != bead.StatusOpen {
		t.Fatalf("expected status=open after requeue, got %q", res.Bead.Status)
	}
	if res.Bead.Owner != nil && *res.Bead.Owner != "" {
		t.Fatalf("expected owner cleared, got %q", *res.Bead.Owner)
	}

	// Audit event lands on the originating bead.
	events, err := store.Events(beadID)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	found := false
	for _, ev := range events {
		if ev.Kind == ddxgraphql.RunRequeueEventKind {
			found = true
			if !strings.Contains(ev.Body, "run_id=run-001") {
				t.Fatalf("audit body missing run_id: %q", ev.Body)
			}
			if !strings.Contains(ev.Body, "idempotency_key=key-A") {
				t.Fatalf("audit body missing idempotency_key: %q", ev.Body)
			}
			if !strings.Contains(ev.Body, "layer_override=run") {
				t.Fatalf("audit body missing layer_override: %q", ev.Body)
			}
			if ev.Source != "graphql:runRequeue" {
				t.Fatalf("expected source=graphql:runRequeue, got %q", ev.Source)
			}
		}
	}
	if !found {
		t.Fatalf("expected run_requeue audit event on originating bead, got %d events", len(events))
	}
}

// TestRunRequeue_DuplicateKeyReturnsExisting covers AC #3:
// "Duplicate-key returns existing requeued bead, not error."
func TestRunRequeue_DuplicateKeyReturnsExisting(t *testing.T) {
	r, store, _ := newRunRequeueResolver(t, nil)
	beadID := seedClosedBead(t, store, "dup target")
	provider := r.State.(*runsTestProvider)
	provider.all = []*ddxgraphql.Run{runForBead("run-dup", beadID)}

	first, err := r.Mutation().RunRequeue(context.Background(), ddxgraphql.RunRequeueInput{
		RunID:          "run-dup",
		IdempotencyKey: "same-key",
	})
	if err != nil {
		t.Fatalf("first RunRequeue: %v", err)
	}
	if first.Deduplicated {
		t.Fatalf("first call must not be deduplicated")
	}

	// Close the bead again to prove the second call does NOT re-reopen it
	// (no second enqueue) — duplicate key must collapse.
	if err := store.Update(beadID, func(b *bead.Bead) {
		b.Status = bead.StatusClosed
	}); err != nil {
		t.Fatalf("re-close: %v", err)
	}

	second, err := r.Mutation().RunRequeue(context.Background(), ddxgraphql.RunRequeueInput{
		RunID:          "run-dup",
		IdempotencyKey: "same-key",
	})
	if err != nil {
		t.Fatalf("second RunRequeue: %v", err)
	}
	if !second.Deduplicated {
		t.Fatalf("second call with same idempotency key must return deduplicated=true")
	}
	if second.Bead.ID != beadID {
		t.Fatalf("expected same bead id, got %s vs %s", second.Bead.ID, beadID)
	}
	// Status must reflect the current bead state — we re-closed above and
	// the second call must NOT have reopened it (refuses to enqueue duplicate).
	if second.Bead.Status != bead.StatusClosed {
		t.Fatalf("duplicate-key call must not re-enqueue; expected status=closed, got %q", second.Bead.Status)
	}

	// Exactly one run_requeue event on the bead.
	events, err := store.Events(beadID)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	count := 0
	for _, ev := range events {
		if ev.Kind == ddxgraphql.RunRequeueEventKind {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 run_requeue audit event, got %d", count)
	}
}

// TestRunRequeue_ConcurrentSameKey covers AC #5: concurrent re-queue with the
// same idempotency key must collapse to a single enqueue (atomic claims).
func TestRunRequeue_ConcurrentSameKey(t *testing.T) {
	r, store, _ := newRunRequeueResolver(t, nil)
	beadID := seedClosedBead(t, store, "concurrent target")
	provider := r.State.(*runsTestProvider)
	provider.all = []*ddxgraphql.Run{runForBead("run-conc", beadID)}

	const N = 16
	var wg sync.WaitGroup
	results := make([]*ddxgraphql.RunRequeueResult, N)
	errs := make([]error, N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = r.Mutation().RunRequeue(context.Background(), ddxgraphql.RunRequeueInput{
				RunID:          "run-conc",
				IdempotencyKey: "shared-key",
			})
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d error: %v", i, e)
		}
		if results[i] == nil || results[i].Bead == nil {
			t.Fatalf("goroutine %d nil result", i)
		}
		if results[i].Bead.ID != beadID {
			t.Fatalf("goroutine %d returned wrong bead id %s", i, results[i].Bead.ID)
		}
		if !results[i].Deduplicated {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 non-deduplicated winner under concurrent same-key calls, got %d", winners)
	}

	// Exactly one audit event on the originating bead — atomic claims.
	events, err := store.Events(beadID)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	count := 0
	for _, ev := range events {
		if ev.Kind == ddxgraphql.RunRequeueEventKind {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 run_requeue audit event under concurrent calls, got %d", count)
	}
}

// TestRunRequeue_MissingIdempotencyKey covers AC #2: idempotencyKey is REQUIRED.
// (The schema marks it non-null, but the resolver also defends against an
// empty string at runtime.)
func TestRunRequeue_MissingIdempotencyKey(t *testing.T) {
	r, _, _ := newRunRequeueResolver(t, []*ddxgraphql.Run{runForBead("run-x", "ddx-x")})
	_, err := r.Mutation().RunRequeue(context.Background(), ddxgraphql.RunRequeueInput{
		RunID:          "run-x",
		IdempotencyKey: "   ",
	})
	if err == nil {
		t.Fatalf("expected error for empty idempotencyKey")
	}
	if !strings.Contains(err.Error(), "idempotencyKey") {
		t.Fatalf("expected idempotencyKey error, got %v", err)
	}
}
