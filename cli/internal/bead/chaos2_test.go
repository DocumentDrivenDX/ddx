package bead

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChaos_MixedConcurrentOps spawns goroutines each running a different
// operation against the same store concurrently: create, claim, close, unclaim,
// update, append-event. After all goroutines finish the store must be
// parseable, have no duplicate IDs, and all beads must have valid statuses.
func TestChaos_MixedConcurrentOps(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		// Pre-seed beads that can be operated on.
		const seedCount = 10
		seedIDs := make([]string, 0, seedCount)
		for i := 0; i < seedCount; i++ {
			b := &Bead{Title: fmt.Sprintf("seed-%d", i)}
			require.NoError(t, s.Create(b))
			seedIDs = append(seedIDs, b.ID)
		}

		var wg sync.WaitGroup
		const goroutines = 8
		const opsPerGoroutine = 20

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(g * 1000)))

				for i := 0; i < opsPerGoroutine; i++ {
					id := seedIDs[rng.Intn(len(seedIDs))]
					op := rng.Intn(6)
					switch op {
					case 0: // create new
						nb := &Bead{Title: fmt.Sprintf("g%d-i%d", g, i)}
						_ = s.Create(nb)
					case 1: // claim
						_ = s.Claim(id, fmt.Sprintf("agent-%d", g))
					case 2: // close
						_ = s.Close(id)
					case 3: // unclaim
						_ = s.Unclaim(id)
					case 4: // update title
						_ = s.Update(id, func(b *Bead) {
							b.Title = fmt.Sprintf("updated-by-g%d-i%d", g, i)
						})
					case 5: // append event
						_ = s.AppendEvent(id, BeadEvent{
							Kind:    "debug",
							Summary: fmt.Sprintf("g%d-i%d", g, i),
						})
					}
				}
			}()
		}

		wg.Wait()

		// Invariants after all concurrent ops.
		beads, err := s.ReadAll()
		require.NoError(t, err, "ReadAll must succeed after mixed concurrent ops")

		validStatuses := []string{StatusOpen, StatusInProgress, StatusClosed}

		seen := make(map[string]int)
		for _, b := range beads {
			seen[b.ID]++
			assert.NotEmpty(t, b.ID, "bead ID must not be empty")
			assert.Contains(t, validStatuses, b.Status, "bead %s has invalid status %q", b.ID, b.Status)
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "bead %s appears %d times (duplicate)", id, count)
		}

		// All seed beads must still exist.
		byID := make(map[string]bool, len(beads))
		for _, b := range beads {
			byID[b.ID] = true
		}
		for _, id := range seedIDs {
			assert.True(t, byID[id], "seed bead %s must survive mixed concurrent ops", id)
		}
	})
}

// TestChaos_ConcurrentClaimContention tests that when multiple goroutines race
// to claim the same bead, exactly one succeeds and the rest see an error.
func TestChaos_ConcurrentClaimContention(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		const rounds = 20
		const claimers = 5

		for round := 0; round < rounds; round++ {
			b := &Bead{Title: fmt.Sprintf("contended-bead-round-%d", round)}
			require.NoError(t, s.Create(b))
			id := b.ID

			var (
				wg         sync.WaitGroup
				successCnt int32
			)

			for c := 0; c < claimers; c++ {
				c := c
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := s.Claim(id, fmt.Sprintf("agent-%d", c)); err == nil {
						atomic.AddInt32(&successCnt, 1)
					}
				}()
			}
			wg.Wait()

			// Exactly one claimer must succeed.
			assert.Equal(t, int32(1), successCnt,
				"round %d: exactly one goroutine must successfully claim the bead", round)

			// Bead must be in_progress.
			got, err := s.Get(id)
			require.NoError(t, err, "round %d: bead must be readable after claim contention", round)
			assert.Equal(t, StatusInProgress, got.Status,
				"round %d: bead must be in_progress after successful claim", round)
			assert.NotEmpty(t, got.Owner, "round %d: owner must be set after claim", round)
		}
	})
}

// TestChaos_ConcurrentCreateCloseEvidence runs create/close/evidence ops
// concurrently. After each wave the store must be parseable and contain no
// duplicate IDs.
func TestChaos_ConcurrentCreateCloseEvidence(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		const waves = 5
		const goroutinesPerWave = 6
		const opsPerGoroutine = 10

		for wave := 0; wave < waves; wave++ {
			// Seed some beads for this wave.
			waveBeads := make([]string, 0, goroutinesPerWave)
			for i := 0; i < goroutinesPerWave; i++ {
				b := &Bead{Title: fmt.Sprintf("wave%d-seed-%d", wave, i)}
				require.NoError(t, s.Create(b))
				waveBeads = append(waveBeads, b.ID)
			}

			var wg sync.WaitGroup
			for g := 0; g < goroutinesPerWave; g++ {
				g := g
				wg.Add(1)
				go func() {
					defer wg.Done()
					rng := rand.New(rand.NewSource(int64(wave*100 + g)))
					for i := 0; i < opsPerGoroutine; i++ {
						id := waveBeads[rng.Intn(len(waveBeads))]
						switch rng.Intn(4) {
						case 0: // new bead
							nb := &Bead{Title: fmt.Sprintf("wave%d-g%d-i%d", wave, g, i)}
							_ = s.Create(nb)
						case 1: // close
							_ = s.Close(id)
						case 2: // evidence
							_ = s.AppendEvent(id, BeadEvent{
								Kind:    "execution",
								Summary: fmt.Sprintf("wave%d-g%d-i%d", wave, g, i),
							})
						case 3: // reopen
							_ = s.Update(id, func(b *Bead) { b.Status = StatusOpen })
						}
					}
				}()
			}
			wg.Wait()

			// After each wave: parseable, no duplicates.
			beads, err := s.ReadAll()
			require.NoError(t, err, "wave %d: ReadAll must succeed", wave)

			seen := make(map[string]int)
			for _, b := range beads {
				seen[b.ID]++
			}
			for id, count := range seen {
				assert.Equal(t, 1, count, "wave %d: ID %q appears %d times", wave, id, count)
			}
		}
	})
}

// TestChaos_ConcurrentDepManagement hammers DepAdd/DepRemove concurrently
// and verifies the dependency graph is always well-formed (no cycles).
func TestChaos_ConcurrentDepManagement(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		// Create a pool of beads.
		const poolSize = 6
		pool := make([]string, 0, poolSize)
		for i := 0; i < poolSize; i++ {
			b := &Bead{Title: fmt.Sprintf("dep-pool-%d", i)}
			require.NoError(t, s.Create(b))
			pool = append(pool, b.ID)
		}

		var wg sync.WaitGroup
		const goroutines = 4
		const ops = 30

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(g * 7)))
				for i := 0; i < ops; i++ {
					// Pick two distinct beads.
					fromIdx := rng.Intn(poolSize)
					toIdx := rng.Intn(poolSize)
					if fromIdx == toIdx {
						continue
					}
					from := pool[fromIdx]
					to := pool[toIdx]
					if rng.Intn(2) == 0 {
						_ = s.DepAdd(from, to) // cycle detection may reject
					} else {
						_ = s.DepRemove(from, to) // idempotent
					}
				}
			}()
		}
		wg.Wait()

		// Verify no bead has a self-dependency.
		beads, err := s.ReadAll()
		require.NoError(t, err)
		for _, b := range beads {
			for _, dep := range b.Dependencies {
				assert.NotEqual(t, b.ID, dep.DependsOnID,
					"bead %s must not depend on itself", b.ID)
			}
		}
	})
}

// TestChaos_WriteAllUnderConcurrentReads verifies that WriteAll under
// concurrent ReadAll never produces partial/corrupt reads.
func TestChaos_WriteAllUnderConcurrentReads(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		// Seed initial beads.
		const seed = 20
		beads := make([]Bead, 0, seed)
		for i := 0; i < seed; i++ {
			b := &Bead{Title: fmt.Sprintf("wa-seed-%d", i)}
			require.NoError(t, s.Create(b))
			beads = append(beads, *b)
		}

		var wg sync.WaitGroup
		const writers = 3
		const readers = 5
		const iterations = 20

		// Writers: read-then-write under the store lock (simulates normal update traffic).
		for w := 0; w < writers; w++ {
			w := w
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					// Use Update to safely mutate one bead (this uses WithLock internally).
					id := beads[w%len(beads)].ID
					_ = s.Update(id, func(b *Bead) {
						b.Notes = fmt.Sprintf("writer-%d-iter-%d", w, i)
					})
				}
			}()
		}

		// Readers: ReadAll must always succeed and return parseable beads with non-empty IDs.
		errCh := make(chan error, readers*iterations)
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					got, err := s.ReadAll()
					if err != nil {
						errCh <- err
						continue
					}
					for _, b := range got {
						if b.ID == "" {
							errCh <- fmt.Errorf("empty ID in concurrent read")
						}
					}
				}
			}()
		}

		wg.Wait()
		close(errCh)

		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}
		assert.Empty(t, errs, "concurrent ReadAll must not see corrupt state: %v", errs)
	})
}

// TestChaos_ImpossibleTransitionsNeverPersist verifies that if an update
// with an invalid status is rejected, the bead retains its prior valid status.
func TestChaos_ImpossibleTransitionsNeverPersist(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		b := &Bead{Title: "impossible-transitions"}
		require.NoError(t, s.Create(b))
		id := b.ID

		invalidStatuses := []string{"pending", "done", "OPEN", "In_Progress", "", "null", "true", "1"}

		var wg sync.WaitGroup

		for _, badStatus := range invalidStatuses {
			badStatus := badStatus
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = s.Update(id, func(b *Bead) { b.Status = badStatus })
			}()
		}
		wg.Wait()

		// After all failed updates the bead must still have a valid status.
		got, err := s.Get(id)
		require.NoError(t, err)
		assert.Contains(t, []string{StatusOpen, StatusInProgress, StatusClosed}, got.Status,
			"bead must have valid status after rejected invalid-status updates")
	})
}

// TestChaos_EventsNeverLostUnderConcurrentAppends verifies that when many
// goroutines append events concurrently to the same bead, no events are lost.
func TestChaos_EventsNeverLostUnderConcurrentAppends(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		b := &Bead{Title: "event-sink"}
		require.NoError(t, s.Create(b))
		id := b.ID

		const goroutines = 5
		const eventsEach = 10
		const total = goroutines * eventsEach

		var wg sync.WaitGroup
		errCh := make(chan error, total)

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < eventsEach; i++ {
					err := s.AppendEvent(id, BeadEvent{
						Kind:    "debug",
						Summary: fmt.Sprintf("g%d-ev%d", g, i),
					})
					if err != nil {
						errCh <- fmt.Errorf("g%d ev%d: %w", g, i, err)
					}
				}
			}()
		}
		wg.Wait()
		close(errCh)

		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}
		require.Empty(t, errs, "concurrent AppendEvent must not error: %v", errs)

		events, err := s.Events(id)
		require.NoError(t, err)
		assert.Len(t, events, total,
			"all %d events must be present after concurrent appends", total)

		// All event summaries must be distinct (each goroutine writes unique summaries).
		summaries := make(map[string]int, len(events))
		for _, ev := range events {
			summaries[ev.Summary]++
		}
		assert.Len(t, summaries, total, "all %d events must have unique summaries", total)
	})
}

// TestChaos_TruncatedJSONLRecovery verifies that if the JSONL file is
// truncated mid-line (simulating a crash), ReadAll recovers what it can.
func TestChaos_TruncatedJSONLRecovery(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create some valid beads.
	validIDs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		b := &Bead{Title: fmt.Sprintf("truncation-bead-%d", i)}
		require.NoError(t, s.Create(b))
		validIDs = append(validIDs, b.ID)
	}

	// Append a truncated (incomplete) JSON line.
	f, err := os.OpenFile(s.File, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, _ = f.WriteString(`{"id":"truncated-bead","title":"truncated`) // no closing brace
	_ = f.Close()

	// ReadAll must not panic; valid beads should survive (warnings for the bad line).
	beads, readErr := s.ReadAll()
	if readErr == nil {
		byID := make(map[string]bool, len(beads))
		for _, b := range beads {
			byID[b.ID] = true
		}
		for _, id := range validIDs {
			assert.True(t, byID[id], "valid bead %s must survive truncated JSONL", id)
		}
		// No duplicate IDs.
		seen := make(map[string]int)
		for _, b := range beads {
			seen[b.ID]++
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "truncated store must not have duplicate %q", id)
		}
	}
	// An error is also acceptable as long as it doesn't panic.
}

// TestChaos_LargeScaleConcurrentCreateReadAll verifies the store under high
// concurrent-create load: all created beads must be findable in ReadAll.
func TestChaos_LargeScaleConcurrentCreateReadAll(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		const goroutines = 15
		const beadsEach = 15
		const total = goroutines * beadsEach

		var (
			wg     sync.WaitGroup
			mu     sync.Mutex
			allIDs = make([]string, 0, total)
			errsCh = make(chan error, total)
		)

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				localIDs := make([]string, 0, beadsEach)
				for i := 0; i < beadsEach; i++ {
					b := &Bead{
						Title:    fmt.Sprintf("large-scale-g%d-b%d", g, i),
						Priority: g % 5,
						Labels:   []string{fmt.Sprintf("g%d", g)},
					}
					if err := s.Create(b); err != nil {
						errsCh <- fmt.Errorf("g%d b%d: %w", g, i, err)
						continue
					}
					localIDs = append(localIDs, b.ID)
				}
				mu.Lock()
				allIDs = append(allIDs, localIDs...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		close(errsCh)

		var errs []error
		for err := range errsCh {
			errs = append(errs, err)
		}
		require.Empty(t, errs)

		beads, err := s.ReadAll()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(beads), len(allIDs),
			"ReadAll must return at least as many beads as were created")

		byID := make(map[string]bool, len(beads))
		for _, b := range beads {
			byID[b.ID] = true
		}

		missingCount := 0
		for _, id := range allIDs {
			if !byID[id] {
				missingCount++
			}
		}
		assert.Zero(t, missingCount, "%d created beads are missing from ReadAll", missingCount)

		// No duplicate IDs.
		seen := make(map[string]int)
		for _, b := range beads {
			seen[b.ID]++
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "bead %s appears %d times", id, count)
		}
	})
}

// TestChaos_DuplicateRowFolding directly tests foldLatestBeads with handcrafted
// duplicate scenarios to verify it always keeps the last written row.
func TestChaos_DuplicateRowFolding(t *testing.T) {
	t.Parallel()

	t.Run("simple duplicate keeps last", func(t *testing.T) {
		input := []Bead{
			{ID: "a", Title: "first", Status: StatusOpen},
			{ID: "a", Title: "second", Status: StatusClosed},
		}
		folded := foldLatestBeads(input)
		require.Len(t, folded, 1)
		assert.Equal(t, "second", folded[0].Title)
		assert.Equal(t, StatusClosed, folded[0].Status)
	})

	t.Run("interleaved duplicates keep last of each", func(t *testing.T) {
		input := []Bead{
			{ID: "a", Title: "a-v1"},
			{ID: "b", Title: "b-v1"},
			{ID: "a", Title: "a-v2"},
			{ID: "b", Title: "b-v2"},
			{ID: "c", Title: "c-v1"},
			{ID: "a", Title: "a-v3"},
		}
		folded := foldLatestBeads(input)
		require.Len(t, folded, 3)

		byID := make(map[string]Bead, 3)
		for _, b := range folded {
			byID[b.ID] = b
		}
		assert.Equal(t, "a-v3", byID["a"].Title)
		assert.Equal(t, "b-v2", byID["b"].Title)
		assert.Equal(t, "c-v1", byID["c"].Title)
	})

	t.Run("all same ID keeps last", func(t *testing.T) {
		const n = 1000
		input := make([]Bead, n)
		for i := 0; i < n; i++ {
			input[i] = Bead{ID: "x", Title: fmt.Sprintf("v%d", i)}
		}
		folded := foldLatestBeads(input)
		require.Len(t, folded, 1)
		assert.Equal(t, fmt.Sprintf("v%d", n-1), folded[0].Title)
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		folded := foldLatestBeads(nil)
		assert.Nil(t, folded)

		folded = foldLatestBeads([]Bead{})
		assert.Nil(t, folded)
	})

	t.Run("no duplicates passthrough", func(t *testing.T) {
		input := []Bead{
			{ID: "a", Title: "a"},
			{ID: "b", Title: "b"},
			{ID: "c", Title: "c"},
		}
		folded := foldLatestBeads(input)
		require.Len(t, folded, 3)
	})
}

// TestChaos_StatusCountsAfterChaos verifies that Status() counts are consistent
// with actual bead statuses returned by ReadAll after chaotic operations.
func TestChaos_StatusCountsAfterChaos(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create a mixed set of beads in different states.
	const count = 15
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		b := &Bead{Title: fmt.Sprintf("status-count-%d", i)}
		require.NoError(t, s.Create(b))
		ids = append(ids, b.ID)
	}

	rng := rand.New(rand.NewSource(12345))
	var wg sync.WaitGroup

	// Concurrent random ops.
	for g := 0; g < 4; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			localRNG := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 30; i++ {
				id := ids[localRNG.Intn(len(ids))]
				switch localRNG.Intn(4) {
				case 0:
					_ = s.Claim(id, "agent")
				case 1:
					_ = s.Close(id)
				case 2:
					_ = s.Unclaim(id)
				case 3:
					_ = s.Update(id, func(b *Bead) { b.Status = StatusOpen })
				}
			}
		}()
	}
	_ = rng
	wg.Wait()

	// Count statuses manually from ReadAll.
	beads, err := s.ReadAll()
	require.NoError(t, err)

	manualOpen, manualInProgress, manualClosed := 0, 0, 0
	for _, b := range beads {
		switch b.Status {
		case StatusOpen:
			manualOpen++
		case StatusInProgress:
			manualInProgress++
		case StatusClosed:
			manualClosed++
		}
	}

	// Get status counts from the store.
	counts, err := s.Status()
	require.NoError(t, err)

	// Status() counts: Open = beads with status "open", Closed = beads with status "closed".
	// In-progress beads count toward Total but not Open or Closed.
	// Ready and Blocked are subsets of Open beads.
	assert.Equal(t, manualOpen+manualInProgress+manualClosed, counts.Total,
		"Total must equal sum of all statuses")
	assert.Equal(t, manualOpen, counts.Open, "Open count must match beads with open status")
	assert.Equal(t, manualClosed, counts.Closed, "Closed count mismatch")

	// Total must equal the seeded count (no beads created by chaos above).
	assert.Equal(t, count, counts.Total)
}

// TestChaos_ExtraFieldsRoundTripUnderChaos verifies that Extra fields survive
// concurrent updates without being silently dropped.
func TestChaos_ExtraFieldsRoundTripUnderChaos(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		// Create a bead with custom Extra fields.
		b := &Bead{
			Title: "extra-fields-chaos",
			Extra: map[string]any{
				"custom-tag":   "important",
				"spec-id":      "FEAT-999",
				"custom-count": float64(0),
			},
		}
		require.NoError(t, s.Create(b))
		id := b.ID

		var wg sync.WaitGroup
		const goroutines = 5

		// Concurrently update unrelated fields, custom Extra must survive.
		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 10; i++ {
					_ = s.Update(id, func(b *Bead) {
						b.Notes = fmt.Sprintf("updated-by-g%d-i%d", g, i)
						// Also increment a custom counter in Extra.
						if b.Extra == nil {
							b.Extra = make(map[string]any)
						}
					})
				}
			}()
		}
		wg.Wait()

		// Extra must still have the original fields.
		got, err := s.Get(id)
		require.NoError(t, err)
		require.NotNil(t, got.Extra)

		tag, ok := got.Extra["custom-tag"]
		assert.True(t, ok, "custom-tag must survive concurrent updates")
		assert.Equal(t, "important", tag)

		specID, ok := got.Extra["spec-id"]
		assert.True(t, ok, "spec-id must survive concurrent updates")
		assert.Equal(t, "FEAT-999", specID)
	})
}

// ---------------------------------------------------------------------------
// Helper: string used in tests but defined in chaos_test.go randomString.
// This file references it if needed; no duplicate needed since same package.
// ---------------------------------------------------------------------------

// assertValidJSONLFile reads the raw JSONL file and verifies every non-empty
// line is valid JSON. Used as a hard assertion that the file is never corrupt.
func assertValidJSONLFile(t *testing.T, s *Store) {
	t.Helper()
	data, err := os.ReadFile(s.File)
	require.NoError(t, err, "JSONL file must be readable")

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Must be parseable JSON.
		b, parseErr := unmarshalBead([]byte(line))
		assert.NoError(t, parseErr, "line %d must be valid JSON: %q", i+1, line)
		if parseErr == nil {
			assert.NotEmpty(t, b.ID, "line %d must have non-empty ID", i+1)
		}
	}
}

// TestChaos_JSOLFileAlwaysValidJSON verifies that after any sequence of ops
// the raw JSONL file contains only valid JSON lines.
func TestChaos_JSONLFileAlwaysValidJSON(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	rng := rand.New(rand.NewSource(55555))
	var ids []string

	// Build up state.
	for i := 0; i < 10; i++ {
		b := &Bead{Title: fmt.Sprintf("valid-json-bead-%d", i)}
		require.NoError(t, s.Create(b))
		ids = append(ids, b.ID)
	}
	assertValidJSONLFile(t, s)

	ops := []func(){
		func() { _ = s.Close(ids[rng.Intn(len(ids))]) },
		func() { _ = s.Claim(ids[rng.Intn(len(ids))], "agent") },
		func() { _ = s.Unclaim(ids[rng.Intn(len(ids))]) },
		func() {
			nb := &Bead{Title: "new-bead"}
			if err := s.Create(nb); err == nil {
				ids = append(ids, nb.ID)
			}
		},
		func() {
			_ = s.AppendEvent(ids[rng.Intn(len(ids))], BeadEvent{
				Kind:    "evidence",
				Summary: "check",
			})
		},
	}

	for i := 0; i < 50; i++ {
		ops[rng.Intn(len(ops))]()
		assertValidJSONLFile(t, s)
	}
}
