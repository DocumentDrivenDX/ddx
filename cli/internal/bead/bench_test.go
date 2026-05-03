package bead

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// benchFixtureSize matches the live `.ddx/beads-archive.jsonl` scale at the
// time this benchmark was written (~1100 closed beads + active queue). The
// fixture loader builds a synthesized corpus matching that shape so the
// benchmark is portable across machines and reproducible across runs.
const benchFixtureSize = 1100

// makeBenchBeads synthesizes an n-bead corpus matching the live archive shape:
// ~70% closed; the remaining ~30% open, partitioned across no-deps (ready),
// closed-dep (ready), and open-dep (blocked) so Ready/Blocked exercise both
// the fast-path and the dep-walk.
func makeBenchBeads(n int) []Bead {
	closedCutoff := (n * 7) / 10
	beads := make([]Bead, n)
	for i := 0; i < n; i++ {
		b := Bead{
			ID:       fmt.Sprintf("bench-%04d", i),
			Title:    fmt.Sprintf("bench bead %d", i),
			Priority: i % 5,
		}
		if i < closedCutoff {
			b.Status = StatusClosed
		} else {
			b.Status = StatusOpen
			switch i % 3 {
			case 1:
				// Dep on a closed bead → ready.
				b.Dependencies = []Dependency{{
					IssueID:     b.ID,
					DependsOnID: fmt.Sprintf("bench-%04d", 50),
					Type:        "blocks",
				}}
			case 2:
				// Dep on an open bead → blocked.
				openTarget := closedCutoff + 1
				if openTarget == i {
					openTarget = closedCutoff + 2
				}
				b.Dependencies = []Dependency{{
					IssueID:     b.ID,
					DependsOnID: fmt.Sprintf("bench-%04d", openTarget),
					Type:        "blocks",
				}}
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
	require.NoError(b, s.Init())
	require.NoError(b, s.WriteAll(beads))
	return s
}

func newAxonBenchStore(b *testing.B, beads []Bead) *Store {
	b.Helper()
	dir := filepath.Join(b.TempDir(), ".ddx")
	s := NewStore(dir)
	s.backend = NewAxonBackend(dir, s.LockWait)
	require.NoError(b, s.Init())
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

func BenchmarkQueueOps_Ready(b *testing.B) {
	beads := makeBenchBeads(benchFixtureSize)
	for _, bk := range benchBackends() {
		bk := bk
		b.Run(bk.name, func(b *testing.B) {
			s := bk.make(b, beads)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Ready(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQueueOps_Blocked(b *testing.B) {
	beads := makeBenchBeads(benchFixtureSize)
	for _, bk := range benchBackends() {
		bk := bk
		b.Run(bk.name, func(b *testing.B) {
			s := bk.make(b, beads)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Blocked(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQueueOps_Show(b *testing.B) {
	beads := makeBenchBeads(benchFixtureSize)
	for _, bk := range benchBackends() {
		bk := bk
		b.Run(bk.name, func(b *testing.B) {
			s := bk.make(b, beads)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				id := fmt.Sprintf("bench-%04d", i%benchFixtureSize)
				if _, err := s.Get(id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
