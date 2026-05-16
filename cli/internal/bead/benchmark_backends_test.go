package bead

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type benchStoreMaker func(*testing.B, []Bead) *Store

func prepareBenchStore(b *testing.B, makeStore benchStoreMaker, beads []Bead) *Store {
	b.Helper()
	s := makeStore(b, beads)
	policy := defaultArchivePolicy()
	policy.MinActiveCount = 0
	policy.MinAge = 30 * 24 * time.Hour
	_, err := s.Archive(policy)
	require.NoError(b, err)
	return s
}

func benchmarkReady(b *testing.B, makeStore benchStoreMaker) {
	b.Helper()
	beads := makeBenchBeads(benchFixtureSize)
	s := prepareBenchStore(b, makeStore, beads)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Ready(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkBlocked(b *testing.B, makeStore benchStoreMaker) {
	b.Helper()
	beads := makeBenchBeads(benchFixtureSize)
	s := prepareBenchStore(b, makeStore, beads)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Blocked(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkShow(b *testing.B, makeStore benchStoreMaker) {
	b.Helper()
	beads := makeBenchBeads(benchFixtureSize)
	s := prepareBenchStore(b, makeStore, beads)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("bench-%04d", i%benchFixtureSize)
		if _, err := s.GetWithArchive(id); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReady_JSONL_1100(b *testing.B) {
	benchmarkReady(b, newJSONLBenchStore)
}

func BenchmarkReady_Axon_1100(b *testing.B) {
	benchmarkReady(b, newAxonBenchStore)
}

func BenchmarkBlocked_JSONL_1100(b *testing.B) {
	benchmarkBlocked(b, newJSONLBenchStore)
}

func BenchmarkBlocked_Axon_1100(b *testing.B) {
	benchmarkBlocked(b, newAxonBenchStore)
}

func BenchmarkShow_JSONL_1100(b *testing.B) {
	benchmarkShow(b, newJSONLBenchStore)
}

func BenchmarkShow_Axon_1100(b *testing.B) {
	benchmarkShow(b, newAxonBenchStore)
}
