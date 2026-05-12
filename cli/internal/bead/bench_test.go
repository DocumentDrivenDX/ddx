package bead

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// benchFixtureSize matches the live `.ddx/beads-archive.jsonl` scale at the
// time this benchmark was written (~1100 beads across active + archive). The
// fixture loader builds a synthesized corpus with open, closed, and archived
// records so the benchmark is portable across machines and reproducible across
// runs.
const benchFixtureSize = 1100

func benchEvents(beadID, prefix string, n int, bodyScale int, start time.Time) []map[string]any {
	events := make([]BeadEvent, 0, n)
	for i := 0; i < n; i++ {
		events = append(events, BeadEvent{
			Kind:      prefix,
			Summary:   fmt.Sprintf("%s-%s-%d", prefix, beadID, i),
			Body:      strings.Repeat(fmt.Sprintf("%s-%s-%d|", prefix, beadID, i), bodyScale),
			Actor:     "bench",
			CreatedAt: start.Add(time.Duration(i) * time.Minute),
			Source:    "benchmark",
		})
	}
	return encodeEventsForExtra(events)
}

// makeBenchBeads synthesizes an n-bead corpus with a mix of open, closed, and
// archived candidates. The first third are open beads split across ready and
// blocked dependency shapes, the second third are recent closed beads that
// stay in the active collection, and the final third are old closed beads that
// Archive(policy) will move to the archive partner. The closed rows carry
// inline event history so the axon backend can exercise its split event store.
func makeBenchBeads(n int) []Bead {
	archivedCutoff := n / 3
	activeClosedCutoff := archivedCutoff * 2
	now := time.Now().UTC()
	recentBase := now.Add(-24 * time.Hour)
	archiveBase := now.Add(-90 * 24 * time.Hour)

	beads := make([]Bead, n)
	for i := 0; i < n; i++ {
		ts := recentBase.Add(-time.Duration(i) * time.Minute)
		if i >= activeClosedCutoff {
			ts = archiveBase.Add(-time.Duration(i-activeClosedCutoff) * time.Minute)
		}
		b := Bead{
			ID:        fmt.Sprintf("bench-%04d", i),
			Title:     fmt.Sprintf("bench bead %d", i),
			Priority:  i % 5,
			IssueType: DefaultType,
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		switch {
		case i < archivedCutoff:
			b.Status = StatusOpen
			b.Extra = map[string]any{
				"events": benchEvents(b.ID, "open", 1, 64, ts.Add(-2*time.Hour)),
			}
			switch i % 3 {
			case 1:
				// Dep on a recent closed bead -> ready.
				target := archivedCutoff + (i % archivedCutoff)
				b.Dependencies = []Dependency{{
					IssueID:     b.ID,
					DependsOnID: fmt.Sprintf("bench-%04d", target),
					Type:        "blocks",
				}}
			case 2:
				// Dep on a different open bead -> blocked.
				openTarget := (i + 1) % archivedCutoff
				b.Dependencies = []Dependency{{
					IssueID:     b.ID,
					DependsOnID: fmt.Sprintf("bench-%04d", openTarget),
					Type:        "blocks",
				}}
			}
		case i < activeClosedCutoff:
			b.Status = StatusClosed
			b.Extra = map[string]any{
				"events": benchEvents(b.ID, "closed", 2, 128, ts.Add(-4*time.Hour)),
			}
		default:
			b.Status = StatusClosed
			b.Extra = map[string]any{
				"events": benchEvents(b.ID, "archived", 4, 192, ts.Add(-8*time.Hour)),
			}
		}
		beads[i] = b
	}
	return beads
}

func newJSONLBenchStore(b *testing.B, beads []Bead) *Store {
	b.Helper()
	dir := filepath.Join(b.TempDir(), ".ddx")
	s := NewStore(dir)
	require.NoError(b, s.Init(testCtx()))
	require.NoError(b, s.WriteAll(beads))
	return s
}

func newAxonBenchStore(b *testing.B, beads []Bead) *Store {
	b.Helper()
	dir := filepath.Join(b.TempDir(), ".ddx")
	s := NewStore(dir)
	s.backend = NewAxonBackend(dir, s.LockWait)
	require.NoError(b, s.Init(testCtx()))
	require.NoError(b, s.WriteAll(beads))
	return s
}

type benchBackendCtor struct {
	name string
	make func(*testing.B, []Bead) *Store
}

func benchBackends() []benchBackendCtor {
	return []benchBackendCtor{
		{name: "jsonl", make: newJSONLBenchStore},
		{name: "axon", make: newAxonBenchStore},
	}
}
