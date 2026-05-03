package bead

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBackend returns a Backend rooted at a fresh temp dir. It exists so
// the chaos suite exercises the JSONL backend through the Backend interface
// (per ddx-bbdd7564 AC §5) rather than through *Store directly.
func newTestBackend(t *testing.T) Backend {
	t.Helper()
	return newTestStore(t)
}

// chaosBackend names a backend constructor for the parameterized chaos suite.
// Per ddx-743bc194, every backend-agnostic chaos scenario must run against both
// the JSONL default and the axon backend so a regression in either path fails
// the same suite.
type chaosBackend struct {
	name string
	make func(*testing.T) Backend
}

func chaosBackends() []chaosBackend {
	return []chaosBackend{
		{name: "jsonl", make: func(t *testing.T) Backend { return newTestStore(t) }},
		{name: "axon", make: func(t *testing.T) Backend { return newAxonStore(t) }},
	}
}

// forEachChaosBackend runs fn once per backend as a parallel subtest. The
// subtest receives a fresh Backend, so scenarios stay isolated per backend.
func forEachChaosBackend(t *testing.T, fn func(*testing.T, Backend)) {
	t.Helper()
	for _, bf := range chaosBackends() {
		bf := bf
		t.Run(bf.name, func(t *testing.T) {
			t.Parallel()
			fn(t, bf.make(t))
		})
	}
}

// TestChaos_ConcurrentAppendSafety spawns 10 goroutines each creating 10 beads
// against the same store. Verifies all 100 beads exist, no duplicates, no corruption.
func TestChaos_ConcurrentAppendSafety(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		const goroutines = 10
		const beadsEach = 10
		const total = goroutines * beadsEach

		var wg sync.WaitGroup
		errCh := make(chan error, total)

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < beadsEach; i++ {
					b := &Bead{Title: fmt.Sprintf("goroutine-%d-bead-%d", g, i)}
					if err := s.Create(b); err != nil {
						errCh <- fmt.Errorf("goroutine %d create %d: %w", g, i, err)
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
		require.Empty(t, errs, "concurrent creates must not error: %v", errs)

		beads, err := s.ReadAll()
		require.NoError(t, err)
		assert.Len(t, beads, total, "all %d beads must be present", total)

		// Check for duplicates
		seen := make(map[string]int)
		for _, b := range beads {
			seen[b.ID]++
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "bead %s appears %d times (duplicate)", id, count)
		}

		// Check no corruption: every bead must have a valid ID, title, and status
		for _, b := range beads {
			assert.NotEmpty(t, b.ID, "bead ID must not be empty")
			assert.NotEmpty(t, b.Title, "bead title must not be empty")
			assert.Contains(t, []string{StatusOpen, StatusInProgress, StatusClosed}, b.Status,
				"bead %s has invalid status %q", b.ID, b.Status)
		}
	})
}

// TestChaos_AtomicStatusTransitions spawns 5 goroutines each cycling a single
// bead through open→in_progress→closed→open. After all iterations the bead
// must have a valid status and the store must be fully parseable.
func TestChaos_AtomicStatusTransitions(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		b := &Bead{Title: "status-churn"}
		require.NoError(t, s.Create(b))
		id := b.ID

		const goroutines = 5
		const iterations = 20

		statuses := []string{StatusOpen, StatusInProgress, StatusClosed}
		var wg sync.WaitGroup

		for g := 0; g < goroutines; g++ {
			g := g
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					next := statuses[(g+i)%len(statuses)]
					// Ignore errors: contention may cause transient failures that are
					// acceptable (e.g. attempting to set a status that is already set).
					// The critical invariant is no corruption.
					_ = s.Update(id, func(bead *Bead) {
						bead.Status = next
					})
				}
			}()
		}

		wg.Wait()

		// Bead must still exist with a valid status
		got, err := s.Get(id)
		require.NoError(t, err, "bead must be readable after concurrent status updates")
		assert.Contains(t, []string{StatusOpen, StatusInProgress, StatusClosed}, got.Status,
			"bead must have a valid status after concurrent updates")

		// Store must be parseable end-to-end
		allBeads, err := s.ReadAll()
		require.NoError(t, err, "store must be parseable after concurrent status transitions")
		assert.NotEmpty(t, allBeads, "store must not be empty after updates")

		// Verify no partial writes: every entry must decode cleanly
		for _, bead := range allBeads {
			assert.NotEmpty(t, bead.ID)
			assert.NotEmpty(t, bead.Title)
		}
	})
}

// TestChaos_ConcurrentCloseAndAppend reproduces the reported bug:
// one goroutine closes a bead while another creates a new bead concurrently.
// Both operations must persist; neither must clobber the other.
func TestChaos_ConcurrentCloseAndAppend(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		// Pre-create the bead that will be closed
		existing := &Bead{Title: "to-be-closed"}
		require.NoError(t, s.Create(existing))
		closeID := existing.ID

		const rounds = 50

		for round := 0; round < rounds; round++ {
			// Re-open the bead so we can close it again
			require.NoError(t, s.Update(closeID, func(b *Bead) {
				b.Status = StatusOpen
			}))

			newTitle := fmt.Sprintf("new-bead-round-%d", round)
			var newID string

			var wg sync.WaitGroup
			wg.Add(2)

			// Goroutine A: close the existing bead
			go func() {
				defer wg.Done()
				_ = s.Close(closeID)
			}()

			// Goroutine B: create a new bead
			go func() {
				defer wg.Done()
				nb := &Bead{Title: newTitle}
				if err := s.Create(nb); err == nil {
					newID = nb.ID
				}
			}()

			wg.Wait()

			// The new bead must exist
			if newID != "" {
				got, err := s.Get(newID)
				require.NoError(t, err, "round %d: new bead %s must exist", round, newID)
				assert.Equal(t, newTitle, got.Title, "round %d: new bead title must match", round)
			}

			// The closed bead's status must reflect the close (it may have been
			// re-opened by a subsequent round, but within this round, if the close
			// completed first and create ran after, the close must be visible).
			// The critical check: the closed bead must still exist and be parseable.
			closedBead, err := s.Get(closeID)
			require.NoError(t, err, "round %d: closed bead must still exist", round)
			assert.NotEmpty(t, closedBead.ID, "round %d: closed bead ID must not be empty", round)
		}
	})
}

// TestChaos_ConcurrentCloseNotLost is the strict regression test for the bug:
// close must not be lost when a concurrent create happens after the close acquires
// the lock but before it writes. With the directory-lock implementation, the lock
// serializes these operations, so the closed status must always be visible after
// both goroutines complete (if the close finishes last, the status is closed;
// if create finishes last, the store was read AFTER the close, so closed is preserved).
func TestChaos_ConcurrentCloseNotLost(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		existing := &Bead{Title: "must-stay-closed"}
		require.NoError(t, s.Create(existing))
		closeID := existing.ID

		const rounds = 20

		for round := 0; round < rounds; round++ {
			// Reset to open
			require.NoError(t, s.Update(closeID, func(b *Bead) {
				b.Status = StatusOpen
			}))

			// Run close and create concurrently, then wait for both
			var wg sync.WaitGroup
			wg.Add(2)

			closeErr := make(chan error, 1)
			createErr := make(chan error, 1)
			createdID := make(chan string, 1)

			go func() {
				defer wg.Done()
				closeErr <- s.Close(closeID)
			}()

			go func() {
				defer wg.Done()
				nb := &Bead{Title: fmt.Sprintf("concurrent-create-%d", round)}
				err := s.Create(nb)
				createErr <- err
				if err == nil {
					createdID <- nb.ID
				} else {
					createdID <- ""
				}
			}()

			wg.Wait()

			cErr := <-closeErr
			nErr := <-createErr
			nID := <-createdID

			require.NoError(t, cErr, "round %d: close must not error", round)
			require.NoError(t, nErr, "round %d: create must not error", round)

			// The closed bead must be closed: since the lock serializes operations,
			// the create (which reads the store under the lock) must see the post-close
			// state IF close ran first, OR the close (which reads under the lock) must
			// complete after create without losing the create. In either ordering, the
			// final state must reflect both operations.
			//
			// The close must be visible: after both complete, whichever ran last wrote
			// the store. If close ran last → bead is closed. If create ran last → create
			// read the store AFTER close completed (lock was released), so bead is closed.
			//
			// This test documents and verifies the invariant: close is never lost.
			finalBead, err := s.Get(closeID)
			require.NoError(t, err, "round %d: closed bead must be readable", round)
			assert.Equal(t, StatusClosed, finalBead.Status,
				"round %d: bead must be closed after close+create race — "+
					"if this fails, the create read a stale snapshot before close wrote", round)

			// The new bead must also exist
			if nID != "" {
				_, err := s.Get(nID)
				assert.NoError(t, err, "round %d: newly created bead %s must exist", round, nID)
			}
		}
	})
}

// TestChaos_JSONLRoundTripIntegrity writes beads with random/unicode/special
// data and verifies all fields survive a round-trip through the store.
func TestChaos_JSONLRoundTripIntegrity(t *testing.T) {
	forEachChaosBackend(t, func(t *testing.T, s Backend) {
		rng := rand.New(rand.NewSource(42))

		type testCase struct {
			name  string
			title string
			desc  string
			notes string
		}

		cases := []testCase{
			{name: "ascii", title: "Simple ASCII title", desc: "plain description", notes: ""},
			{name: "unicode", title: "Unicode: 日本語テスト", desc: "emoji: 🚀🎯✅", notes: "混合 content"},
			{name: "special-chars", title: `Tabs	and "quotes" and \backslash`, desc: "line1\nline2\nline3", notes: ""},
			{name: "empty-optional", title: "Only title is required", desc: "", notes: ""},
			{name: "max-length", title: strings.Repeat("A", 200), desc: strings.Repeat("B", 1000), notes: strings.Repeat("C", 500)},
			{name: "json-injection", title: `{"injected":"json"}`, desc: `"nested":"value"`, notes: `]},{"id":"fake"`},
			{name: "null-like", title: "null", desc: "false", notes: "0"},
			{name: "whitespace", title: "  spaces around  ", desc: "\t\ttabbed\t\t", notes: "   "},
		}

		// Add randomized cases
		for i := 0; i < 10; i++ {
			cases = append(cases, testCase{
				name:  fmt.Sprintf("random-%d", i),
				title: randomString(rng, 5, 80),
				desc:  randomString(rng, 0, 200),
				notes: randomString(rng, 0, 100),
			})
		}

		created := make([]*Bead, 0, len(cases))

		for _, tc := range cases {
			tc := tc
			b := &Bead{
				Title:       tc.title,
				Description: tc.desc,
				Notes:       tc.notes,
				Labels:      []string{"chaos", tc.name},
				Priority:    rng.Intn(5),
			}
			if err := s.Create(b); err != nil {
				// If title is empty after trimming, skip — that's a validation rule, not a bug
				if strings.Contains(err.Error(), "title is required") {
					t.Logf("skipping case %q: title trims to empty", tc.name)
					continue
				}
				t.Fatalf("case %q: create failed: %v", tc.name, err)
			}
			created = append(created, b)
		}

		// Read all back and verify field-by-field
		beads, err := s.ReadAll()
		require.NoError(t, err, "ReadAll must succeed after writing random beads")

		byID := make(map[string]Bead, len(beads))
		for _, b := range beads {
			byID[b.ID] = b
		}

		for _, orig := range created {
			got, ok := byID[orig.ID]
			if !assert.True(t, ok, "bead %s must survive round-trip", orig.ID) {
				continue
			}
			assert.Equal(t, orig.Title, got.Title, "bead %s: Title mismatch", orig.ID)
			assert.Equal(t, orig.Description, got.Description, "bead %s: Description mismatch", orig.ID)
			assert.Equal(t, orig.Notes, got.Notes, "bead %s: Notes mismatch", orig.ID)
			assert.Equal(t, orig.Priority, got.Priority, "bead %s: Priority mismatch", orig.ID)
			assert.Equal(t, orig.Status, got.Status, "bead %s: Status mismatch", orig.ID)
			assert.ElementsMatch(t, orig.Labels, got.Labels, "bead %s: Labels mismatch", orig.ID)
		}
	})
}

// randomString generates a random string with printable characters including
// some unicode, in the range [minLen, maxLen) characters.
func randomString(rng *rand.Rand, minLen, maxLen int) string {
	if maxLen <= minLen {
		return ""
	}
	length := minLen + rng.Intn(maxLen-minLen)
	if length == 0 {
		return ""
	}

	// Character pools
	pools := []string{
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		` !@#$%^&*()_+-=[]{}|;':\",./<>?`,
		"αβγδεζηθικλμνξοπρστυφχψω",
		"日本語中文한국어",
		"🚀🎯✅❌💡🔧",
	}

	var sb strings.Builder
	for i := 0; i < length; i++ {
		pool := pools[rng.Intn(len(pools))]
		runes := []rune(pool)
		r := runes[rng.Intn(len(runes))]
		// Skip control characters that would break JSONL lines
		if r == '\n' || r == '\r' || r == 0 || !unicode.IsPrint(r) {
			r = 'X'
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
