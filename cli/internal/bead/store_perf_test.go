package bead

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPerformance_BeadStoreConcurrentClaimReadyHeartbeatUnderBudget(t *testing.T) {
	skipFullBeadSuiteInShort(t)
	s := newTestStore(t)

	// Seed a small, deterministic open-bead corpus. The IDs, timestamps, and
	// titles are fixed so the test exercises the same JSONL shape every run.
	const corpusSize = 8
	corpus := make([]Bead, 0, corpusSize)
	baseTime := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < corpusSize; i++ {
		corpus = append(corpus, Bead{
			ID:        fmt.Sprintf("ddx-perf-%02d", i),
			Title:     fmt.Sprintf("perf bead %02d", i),
			IssueType: DefaultType,
			Status:    StatusOpen,
			Priority:  i % 3,
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
			UpdatedAt: baseTime.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, s.WriteAll(corpus))

	var (
		mu      sync.Mutex
		samples []LockSample
	)
	prevSink := LockMetricsSink
	LockMetricsSink = func(sample LockSample) {
		mu.Lock()
		samples = append(samples, sample)
		mu.Unlock()
	}
	t.Cleanup(func() {
		LockMetricsSink = prevSink
	})

	const (
		workerCount = 4
		rounds      = 8
	)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, workerCount+1)

	// Writer workers each own a bead so the workload remains deterministic
	// while still contending on the store-wide lock.
	for worker := 0; worker < workerCount; worker++ {
		worker := worker
		beadID := corpus[worker].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			owner := fmt.Sprintf("perf-worker-%d", worker)
			for round := 0; round < rounds; round++ {
				if err := s.Claim(beadID, owner); err != nil {
					errCh <- fmt.Errorf("worker %d claim round %d: %w", worker, round, err)
					return
				}
				if err := s.Heartbeat(beadID); err != nil {
					errCh <- fmt.Errorf("worker %d heartbeat round %d: %w", worker, round, err)
					return
				}
				if err := s.AppendEvent(beadID, BeadEvent{
					Kind:    "performance",
					Summary: fmt.Sprintf("worker=%d round=%d", worker, round),
				}); err != nil {
					errCh <- fmt.Errorf("worker %d append round %d: %w", worker, round, err)
					return
				}
				if err := s.Release(beadID, owner, StatusOpen); err != nil {
					errCh <- fmt.Errorf("worker %d release round %d: %w", worker, round, err)
					return
				}
			}
		}()
	}

	// ReadyExecution is read-only, so run it in parallel to keep the open-bead
	// corpus under mixed claim/read pressure while the writer goroutines churn.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < rounds*workerCount; i++ {
			_, err := s.ReadyExecution()
			if err != nil {
				errCh <- fmt.Errorf("ready execution iteration %d: %w", i, err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	finalReady, err := s.ReadyExecution()
	require.NoError(t, err)
	require.Len(t, finalReady, corpusSize)

	mu.Lock()
	got := append([]LockSample(nil), samples...)
	mu.Unlock()

	require.GreaterOrEqual(t, len(got), workerCount*rounds*4,
		"expected lock samples from claim/heartbeat/event/release cycles")

	waitSamples := make([]float64, 0, len(got))
	holdSamples := make([]float64, 0, len(got))
	for _, sample := range got {
		waitSamples = append(waitSamples, float64(sample.Wait)/float64(time.Millisecond))
		holdSamples = append(holdSamples, float64(sample.Hold)/float64(time.Millisecond))
	}

	p95Wait := percentileFloat(waitSamples, 0.95)
	p99Wait := percentileFloat(waitSamples, 0.99)
	p95Hold := percentileFloat(holdSamples, 0.95)
	p99Hold := percentileFloat(holdSamples, 0.99)

	require.NotEmpty(t, got, "expected lock samples from claim/heartbeat/event/release cycles")

	const (
		maxP95WaitMS = 25.0
		maxP99WaitMS = 1000.0
		maxP95HoldMS = 30.0
		maxP99HoldMS = 50.0
	)

	require.Less(t, p95Wait, maxP95WaitMS,
		"p95 lock wait %.2fms exceeded budget %.2fms", p95Wait, maxP95WaitMS)
	require.Less(t, p99Wait, maxP99WaitMS,
		"p99 lock wait %.2fms exceeded budget %.2fms", p99Wait, maxP99WaitMS)
	require.Less(t, p95Hold, maxP95HoldMS,
		"p95 lock hold %.2fms exceeded budget %.2fms", p95Hold, maxP95HoldMS)
	require.Less(t, p99Hold, maxP99HoldMS,
		"p99 lock hold %.2fms exceeded budget %.2fms", p99Hold, maxP99HoldMS)
}

func percentileFloat(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	rank := int(math.Ceil(p*float64(len(cp)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(cp) {
		rank = len(cp) - 1
	}
	return cp[rank]
}
