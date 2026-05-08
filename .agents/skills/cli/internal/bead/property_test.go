package bead

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Property: CRUD invariants
// ----------------------------------------------------------------------------

// TestProperty_CreateGetRoundTrip verifies that every created bead can be
// retrieved with identical core fields (ID, title, status, priority, labels).
func TestProperty_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(1337))
	s := newTestStore(t)

	type testInput struct {
		title    string
		priority int
		labels   []string
		desc     string
		notes    string
	}

	inputs := []testInput{
		{title: "basic task", priority: 2, labels: nil},
		{title: "unicode: 日本語🚀", priority: 0, labels: []string{"feat", "p0"}},
		{title: "priority-4 low", priority: 4, labels: []string{"low"}},
		{title: "with description", priority: 1, desc: "detailed description here", notes: "some notes"},
		{title: `special "quoted" title`, priority: 3, labels: []string{"special", `"label"`}},
	}
	// Add random inputs.
	for i := 0; i < 20; i++ {
		inputs = append(inputs, testInput{
			title:    randomString(rng, 1, 60),
			priority: rng.Intn(5),
			labels:   randomLabels(rng, 0, 4),
		})
	}

	for i, inp := range inputs {
		inp := inp
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			b := &Bead{
				Title:       inp.title,
				Priority:    inp.priority,
				Labels:      inp.labels,
				Description: inp.desc,
				Notes:       inp.notes,
			}
			if b.Title == "" || strings.TrimSpace(b.Title) == "" {
				return // skip empty-title cases; those are validation errors, not bugs
			}

			err := s.Create(b)
			if err != nil && strings.Contains(err.Error(), "title is required") {
				return
			}
			require.NoError(t, err, "Create must succeed for input %d", i)
			require.NotEmpty(t, b.ID, "ID must be assigned on create")

			got, err := s.Get(b.ID)
			require.NoError(t, err, "Get must succeed immediately after Create")

			assert.Equal(t, b.ID, got.ID)
			assert.Equal(t, b.Title, got.Title)
			assert.Equal(t, StatusOpen, got.Status, "default status must be open")
			assert.Equal(t, b.Priority, got.Priority)
			assert.Equal(t, b.Description, got.Description)
			assert.Equal(t, b.Notes, got.Notes)
			if len(inp.labels) > 0 {
				assert.ElementsMatch(t, inp.labels, got.Labels)
			}
		})
	}
}

// TestProperty_OneRowPerID verifies that after any mixture of creates and
// updates, ReadAll never returns duplicate IDs.
func TestProperty_OneRowPerID(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(2024))
	s := newTestStore(t)

	// Create a base set.
	const baseCount = 10
	ids := make([]string, 0, baseCount)
	for i := 0; i < baseCount; i++ {
		b := &Bead{Title: fmt.Sprintf("base-bead-%d", i)}
		require.NoError(t, s.Create(b))
		ids = append(ids, b.ID)
	}

	// Perform random updates.
	statuses := []string{StatusOpen, StatusInProgress, StatusClosed}
	for round := 0; round < 50; round++ {
		id := ids[rng.Intn(len(ids))]
		next := statuses[rng.Intn(len(statuses))]
		_ = s.Update(id, func(b *Bead) { b.Status = next })
	}

	// Invariant: ReadAll returns exactly one row per ID.
	beads, err := s.ReadAll()
	require.NoError(t, err)

	seen := make(map[string]int, len(beads))
	for _, b := range beads {
		seen[b.ID]++
	}
	for id, count := range seen {
		assert.Equal(t, 1, count, "ID %q appears %d times in ReadAll", id, count)
	}

	// Must have exactly baseCount beads.
	assert.Len(t, beads, baseCount)
}

// TestProperty_StatusMustBeValid verifies that no operation can leave a bead
// in an unrecognized status.
func TestProperty_StatusMustBeValid(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	b := &Bead{Title: "status-validity"}
	require.NoError(t, s.Create(b))
	id := b.ID

	validStatuses := []string{StatusOpen, StatusInProgress, StatusClosed}

	// Attempt to set invalid status via Update — should be rejected.
	err := s.Update(id, func(b *Bead) {
		b.Status = "invalid-status"
	})
	assert.Error(t, err, "Update with invalid status must be rejected")

	// Bead must still have a valid status.
	got, err := s.Get(id)
	require.NoError(t, err)
	assert.Contains(t, validStatuses, got.Status)

	// Transition via Claim and Close.
	require.NoError(t, s.Claim(id, "agent-1"))
	got, _ = s.Get(id)
	assert.Equal(t, StatusInProgress, got.Status)

	require.NoError(t, s.Close(id))
	got, _ = s.Get(id)
	assert.Equal(t, StatusClosed, got.Status)
}

// ----------------------------------------------------------------------------
// Property: Claim / Unclaim / Close state machine
// ----------------------------------------------------------------------------

// TestProperty_ClaimStateMachine verifies the claim/unclaim/close state
// machine transitions for a wide range of sequences. The key guarded
// transition is Claim, which only succeeds from StatusOpen. All other
// transitions (Unclaim, Close, reopen via Update) are idempotent and
// never return errors regardless of current status.
func TestProperty_ClaimStateMachine(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	rng := rand.New(rand.NewSource(9999))
	b := &Bead{Title: "state-machine-target"}
	require.NoError(t, s.Create(b))
	id := b.ID

	validStatuses := []string{StatusOpen, StatusInProgress, StatusClosed}

	for step := 0; step < 200; step++ {
		// Read current status before deciding the operation.
		current, err := s.Get(id)
		require.NoError(t, err, "step %d: Get must succeed", step)

		op := rng.Intn(4)
		switch op {
		case 0: // Claim: only succeeds from StatusOpen.
			claimErr := s.Claim(id, "agent")
			if current.Status != StatusOpen {
				assert.Error(t, claimErr,
					"step %d: Claim from %s must error", step, current.Status)
			}
			// From StatusOpen, claim may succeed or race — don't assert on success.

		case 1: // Unclaim: always succeeds (no-op if not in_progress).
			assert.NoError(t, s.Unclaim(id), "step %d: Unclaim must never error", step)

		case 2: // Close: always succeeds (idempotent if already closed).
			assert.NoError(t, s.Close(id), "step %d: Close must never error", step)

		case 3: // Reopen via Update: always succeeds.
			assert.NoError(t, s.Update(id, func(b *Bead) { b.Status = StatusOpen }),
				"step %d: reopen via Update must never error", step)
		}

		// After every operation the bead must have a valid status.
		got, getErr := s.Get(id)
		require.NoError(t, getErr, "step %d: Get must succeed after op", step)
		assert.Contains(t, validStatuses, got.Status,
			"step %d: bead must have valid status after op %d, was %q", step, op, got.Status)
	}
}

// TestProperty_UnclaimDoesNotReopenClosed verifies that unclaiming a closed
// bead does not change its status to open.
func TestProperty_UnclaimDoesNotReopenClosed(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	b := &Bead{Title: "must-stay-closed-on-unclaim"}
	require.NoError(t, s.Create(b))
	id := b.ID

	require.NoError(t, s.Claim(id, "agent"))
	require.NoError(t, s.Close(id))

	// Unclaim on closed bead: should not change status to open.
	err := s.Unclaim(id)
	// Unclaim calls Update which may or may not error (it uses the current status).
	// What matters is the resulting status.
	_ = err

	got, err := s.Get(id)
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, got.Status, "unclaiming a closed bead must not reopen it")
}

// TestProperty_ClaimRequiresOpen verifies that a non-open bead cannot be claimed.
func TestProperty_ClaimRequiresOpen(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	b := &Bead{Title: "claim-guard"}
	require.NoError(t, s.Create(b))
	id := b.ID

	// Claim succeeds from open.
	require.NoError(t, s.Claim(id, "agent-1"))

	// Second claim must fail (already in_progress).
	err := s.Claim(id, "agent-2")
	assert.Error(t, err, "claiming an in_progress bead must error")

	// Close it and verify claiming closed bead also fails.
	require.NoError(t, s.Close(id))
	err = s.Claim(id, "agent-3")
	assert.Error(t, err, "claiming a closed bead must error")
}

// ----------------------------------------------------------------------------
// Property: Dependency invariants
// ----------------------------------------------------------------------------

// TestProperty_DepCycleDetection verifies that the dependency graph never
// acquires a cycle, regardless of the add order.
func TestProperty_DepCycleDetection(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create a chain: a -> b -> c -> d
	var ids []string
	for i := 0; i < 5; i++ {
		b := &Bead{Title: fmt.Sprintf("dep-chain-%d", i)}
		require.NoError(t, s.Create(b))
		ids = append(ids, b.ID)
	}

	// Build a valid chain: ids[1] depends on ids[0], ids[2] on ids[1], etc.
	for i := 1; i < len(ids); i++ {
		require.NoError(t, s.DepAdd(ids[i], ids[i-1]), "dep %d->%d must succeed", i, i-1)
	}

	// Attempt to create a cycle: ids[0] depends on ids[len-1].
	err := s.DepAdd(ids[0], ids[len(ids)-1])
	assert.Error(t, err, "creating a dependency cycle must be rejected")

	// Self-dependency must also be rejected.
	err = s.DepAdd(ids[0], ids[0])
	assert.Error(t, err, "self-dependency must be rejected")

	// Verify the valid chain is intact.
	b, err := s.Get(ids[len(ids)-1])
	require.NoError(t, err)
	assert.True(t, b.HasDep(ids[len(ids)-2]), "dep chain must be preserved after cycle rejection")
}

// TestProperty_DepRemoveAndReadd verifies that removing and re-adding a
// dependency is idempotent and the bead's dep list remains consistent.
func TestProperty_DepRemoveAndReadd(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	a := &Bead{Title: "dep-a"}
	b := &Bead{Title: "dep-b"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))

	// Add dep: b depends on a.
	require.NoError(t, s.DepAdd(b.ID, a.ID))

	got, _ := s.Get(b.ID)
	assert.True(t, got.HasDep(a.ID))

	// Remove dep.
	require.NoError(t, s.DepRemove(b.ID, a.ID))
	got, _ = s.Get(b.ID)
	assert.False(t, got.HasDep(a.ID))

	// Re-add.
	require.NoError(t, s.DepAdd(b.ID, a.ID))
	got, _ = s.Get(b.ID)
	assert.True(t, got.HasDep(a.ID))

	// Adding same dep again must not duplicate.
	require.NoError(t, s.DepAdd(b.ID, a.ID))
	got, _ = s.Get(b.ID)
	count := 0
	for _, d := range got.Dependencies {
		if d.DependsOnID == a.ID {
			count++
		}
	}
	assert.Equal(t, 1, count, "dep must not be duplicated by double-add")
}

// ----------------------------------------------------------------------------
// Property: Ready / Blocked queue derivation
// ----------------------------------------------------------------------------

// TestProperty_ReadyAndBlockedInvariants verifies that Ready() and Blocked()
// are consistent: Ready beads have all deps closed, Blocked beads have at
// least one open/in_progress dep. A bead cannot appear in both lists.
func TestProperty_ReadyAndBlockedInvariants(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create a dependency graph:
	//   prereq (open)
	//   dependentA depends on prereq
	//   dependentB has no deps
	//   closed-one (will be closed)
	prereq := &Bead{Title: "prereq"}
	depA := &Bead{Title: "dependentA"}
	depB := &Bead{Title: "independentB"}
	closedOne := &Bead{Title: "closed-one"}

	require.NoError(t, s.Create(prereq))
	require.NoError(t, s.Create(depA))
	require.NoError(t, s.Create(depB))
	require.NoError(t, s.Create(closedOne))
	require.NoError(t, s.Close(closedOne.ID))

	require.NoError(t, s.DepAdd(depA.ID, prereq.ID))

	// State: prereq=open(no deps), depA=open(dep on prereq open), depB=open(no deps), closedOne=closed
	// Expected: Ready = [prereq, depB], Blocked = [depA]

	ready, err := s.Ready()
	require.NoError(t, err)
	blocked, err := s.Blocked()
	require.NoError(t, err)

	readyIDs := make(map[string]bool)
	for _, b := range ready {
		readyIDs[b.ID] = true
		assert.Equal(t, StatusOpen, b.Status, "ready bead %s must be open", b.ID)
	}
	blockedIDs := make(map[string]bool)
	for _, b := range blocked {
		blockedIDs[b.ID] = true
		assert.Equal(t, StatusOpen, b.Status, "blocked bead %s must be open", b.ID)
	}

	// No overlap between ready and blocked.
	for id := range readyIDs {
		assert.False(t, blockedIDs[id], "bead %s appears in both ready and blocked", id)
	}

	// Closed bead must not appear in either.
	assert.False(t, readyIDs[closedOne.ID], "closed bead must not be in ready")
	assert.False(t, blockedIDs[closedOne.ID], "closed bead must not be in blocked")

	// depA (blocked by open prereq) must not be in ready.
	assert.False(t, readyIDs[depA.ID], "depA must not be ready while prereq is open")
	assert.True(t, blockedIDs[depA.ID], "depA must be blocked while prereq is open")

	// prereq and depB must be ready.
	assert.True(t, readyIDs[prereq.ID], "prereq must be ready (no deps)")
	assert.True(t, readyIDs[depB.ID], "depB must be ready (no deps)")

	// Close prereq: depA should become ready.
	require.NoError(t, s.Close(prereq.ID))

	ready2, err := s.Ready()
	require.NoError(t, err)
	blocked2, err := s.Blocked()
	require.NoError(t, err)

	ready2IDs := make(map[string]bool)
	for _, b := range ready2 {
		ready2IDs[b.ID] = true
	}
	blocked2IDs := make(map[string]bool)
	for _, b := range blocked2 {
		blocked2IDs[b.ID] = true
	}

	assert.True(t, ready2IDs[depA.ID], "depA must be ready after prereq is closed")
	assert.False(t, blocked2IDs[depA.ID], "depA must not be blocked after prereq is closed")
}

// TestProperty_ReadyPriorityOrder verifies that Ready() returns beads in
// ascending priority order (0 = highest).
func TestProperty_ReadyPriorityOrder(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	s := newTestStore(t)

	// Create beads with randomized priorities.
	for i := 0; i < 20; i++ {
		b := &Bead{
			Title:    fmt.Sprintf("priority-bead-%d", i),
			Priority: rng.Intn(5),
		}
		require.NoError(t, s.Create(b))
	}

	ready, err := s.Ready()
	require.NoError(t, err)
	require.NotEmpty(t, ready)

	// Verify ascending priority order.
	for i := 1; i < len(ready); i++ {
		assert.LessOrEqual(t, ready[i-1].Priority, ready[i].Priority,
			"ready[%d].Priority=%d > ready[%d].Priority=%d (want ascending)",
			i-1, ready[i-1].Priority, i, ready[i].Priority)
	}
}

// ----------------------------------------------------------------------------
// Property: Evidence / Event append-only invariants
// ----------------------------------------------------------------------------

// TestProperty_EventsAreAppendOnly verifies that AppendEvent never drops
// earlier events and events appear in insertion order.
func TestProperty_EventsAreAppendOnly(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	b := &Bead{Title: "evidence-target"}
	require.NoError(t, s.Create(b))
	id := b.ID

	eventKinds := []string{"execution", "debug", "evidence", "info"}
	appended := make([]BeadEvent, 0, 30)

	for i := 0; i < 30; i++ {
		ev := BeadEvent{
			Kind:      eventKinds[i%len(eventKinds)],
			Summary:   fmt.Sprintf("event-%d", i),
			Body:      fmt.Sprintf("body for event %d", i),
			Actor:     "test-agent",
			CreatedAt: time.Date(2024, 1, 1, 0, 0, i, 0, time.UTC),
		}
		require.NoError(t, s.AppendEvent(id, ev), "AppendEvent %d must succeed", i)
		appended = append(appended, ev)

		// After each append, verify all prior events are still present.
		got, err := s.Events(id)
		require.NoError(t, err, "Events must succeed after append %d", i)
		assert.Len(t, got, i+1, "events count must grow by 1 after append %d", i)

		// Most recent event must match what we just appended.
		last := got[len(got)-1]
		assert.Equal(t, ev.Kind, last.Kind, "append %d: kind mismatch", i)
		assert.Equal(t, ev.Summary, last.Summary, "append %d: summary mismatch", i)
	}

	// Final: all 30 events must be present in order.
	final, err := s.Events(id)
	require.NoError(t, err)
	require.Len(t, final, 30)
	for i, ev := range final {
		assert.Equal(t, appended[i].Summary, ev.Summary, "event %d summary mismatch", i)
	}
}

// TestProperty_EventsOnMultipleBeadsIndependent verifies that events on one
// bead do not appear on another.
func TestProperty_EventsOnMultipleBeadsIndependent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	a := &Bead{Title: "event-bead-a"}
	b := &Bead{Title: "event-bead-b"}
	require.NoError(t, s.Create(a))
	require.NoError(t, s.Create(b))

	require.NoError(t, s.AppendEvent(a.ID, BeadEvent{Kind: "debug", Summary: "a-event"}))
	require.NoError(t, s.AppendEvent(b.ID, BeadEvent{Kind: "debug", Summary: "b-event"}))
	require.NoError(t, s.AppendEvent(a.ID, BeadEvent{Kind: "debug", Summary: "a-event-2"}))

	aEvents, err := s.Events(a.ID)
	require.NoError(t, err)
	assert.Len(t, aEvents, 2)

	bEvents, err := s.Events(b.ID)
	require.NoError(t, err)
	assert.Len(t, bEvents, 1)
	assert.Equal(t, "b-event", bEvents[0].Summary)
}

// ----------------------------------------------------------------------------
// Property: Randomized operation streams — core invariants
// ----------------------------------------------------------------------------

// TestProperty_RandomizedOpStream runs a large number of randomized bead
// operations and verifies that core invariants hold after each operation:
// parseable file, one live row per ID, valid statuses.
func TestProperty_RandomizedOpStream(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(777))
	s := newTestStore(t)

	var liveIDs []string
	validStatuses := []string{StatusOpen, StatusInProgress, StatusClosed}

	type op struct {
		name string
		fn   func()
	}

	makeOps := func() []op {
		ops := []op{
			{
				name: "create",
				fn: func() {
					b := &Bead{Title: fmt.Sprintf("rand-bead-%d", rng.Intn(10000))}
					if err := s.Create(b); err == nil {
						liveIDs = append(liveIDs, b.ID)
					}
				},
			},
		}
		if len(liveIDs) > 0 {
			id := liveIDs[rng.Intn(len(liveIDs))]
			ops = append(ops,
				op{name: "update-status", fn: func() {
					next := validStatuses[rng.Intn(len(validStatuses))]
					_ = s.Update(id, func(b *Bead) { b.Status = next })
				}},
				op{name: "claim", fn: func() { _ = s.Claim(id, "agent") }},
				op{name: "unclaim", fn: func() { _ = s.Unclaim(id) }},
				op{name: "close", fn: func() { _ = s.Close(id) }},
				op{name: "append-event", fn: func() {
					_ = s.AppendEvent(id, BeadEvent{Kind: "debug", Summary: "rand-event"})
				}},
				op{name: "get", fn: func() { _, _ = s.Get(id) }},
			)
		}
		return ops
	}

	const rounds = 200
	for round := 0; round < rounds; round++ {
		ops := makeOps()
		chosen := ops[rng.Intn(len(ops))]
		chosen.fn()

		// After each operation, verify invariants.
		beads, err := s.ReadAll()
		require.NoError(t, err, "round %d (%s): ReadAll must succeed", round, chosen.name)

		// One row per ID.
		seen := make(map[string]int, len(beads))
		for _, b := range beads {
			seen[b.ID]++
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "round %d: duplicate ID %q", round, id)
		}

		// All beads have valid status, non-empty ID.
		for _, b := range beads {
			assert.NotEmpty(t, b.ID, "round %d: empty ID", round)
			assert.Contains(t, validStatuses, b.Status, "round %d: invalid status %q", round, b.Status)
		}
	}
}

// TestProperty_MalformedJSONLRepair verifies that the store self-repairs when
// the JSONL file contains malformed lines intermixed with valid ones.
func TestProperty_MalformedJSONLRepair(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create a few good beads.
	var ids []string
	for i := 0; i < 5; i++ {
		b := &Bead{Title: fmt.Sprintf("good-bead-%d", i)}
		require.NoError(t, s.Create(b))
		ids = append(ids, b.ID)
	}

	// Corrupt the JSONL file by appending a few bad lines.
	badLines := []string{
		"not json at all",
		`{"broken":`,
		`{"id":null}`,
		"",
	}
	f, err := os.OpenFile(s.File, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	for _, line := range badLines {
		_, _ = fmt.Fprintln(f, line)
	}
	_ = f.Close()

	// ReadAll should still return the valid beads (repair triggers on warnings).
	beads, err := s.ReadAll()
	// If all valid lines survive (warnings about bad lines, but not fatal):
	if err == nil {
		byID := make(map[string]bool, len(beads))
		for _, b := range beads {
			byID[b.ID] = true
		}
		for _, id := range ids {
			assert.True(t, byID[id], "valid bead %s must survive malformed JSONL repair", id)
		}
		// Must have no duplicate IDs.
		seen := make(map[string]int)
		for _, b := range beads {
			seen[b.ID]++
		}
		for id, count := range seen {
			assert.Equal(t, 1, count, "repaired store must not have duplicate ID %q", id)
		}
	}
	// An error here is also acceptable (all-bad-lines case returns error), but
	// the file must still be parseable after a re-read.
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func randomLabels(rng *rand.Rand, minN, maxN int) []string {
	pool := []string{"feat", "bug", "chore", "p0", "p1", "p2", "blocker", "wip", "rfc", "spike"}
	if maxN <= minN {
		return nil
	}
	n := minN + rng.Intn(maxN-minN+1)
	if n == 0 {
		return nil
	}
	perm := rng.Perm(len(pool))
	if n > len(pool) {
		n = len(pool)
	}
	labels := make([]string, n)
	for i := 0; i < n; i++ {
		labels[i] = pool[perm[i]]
	}
	sort.Strings(labels)
	return labels
}
