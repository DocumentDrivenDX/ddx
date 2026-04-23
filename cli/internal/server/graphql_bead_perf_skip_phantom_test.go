package server

// Perf-amplifier regression for ddx-15f7ee0b AC §4: GetBeadSnapshots must not
// open a bead store for projects that are tombstoned (Unreachable=true) —
// those entries were the primary multiplier on a polluted state file. The
// test-dir side of the guarantee is enforced at load time by migrate() and is
// covered by TestMigrateDropsTestDirPollution; this test focuses on the hot
// path that runs per-query.

import (
	"testing"
	"time"
)

func TestGetBeadSnapshotsSkipsTombstonedProjects(t *testing.T) {
	// Hand-built ServerState: we need deterministic control over which
	// entries are present. New() would reshape the slice through migrate.
	s := &ServerState{}
	now := time.Now().UTC()
	s.Projects = []ProjectEntry{
		{ID: "real", Path: "/home/example/real", RegisteredAt: now, LastSeen: now},
		{ID: "tomb", Path: "/home/example/tombstoned", RegisteredAt: now, LastSeen: now, Unreachable: true, TombstonedAt: &now},
	}

	// Record every project path the GraphQL bead path opens a store for.
	opened := make([]string, 0, 2)
	prev := beadStoreOpenHook
	beadStoreOpenHook = func(p string) { opened = append(opened, p) }
	t.Cleanup(func() { beadStoreOpenHook = prev })

	_ = s.GetBeadSnapshots("", "", "", "")

	for _, p := range opened {
		if p == "/home/example/tombstoned" {
			t.Errorf("GetBeadSnapshots opened store for tombstoned project %q", p)
		}
	}
	realSeen := false
	for _, p := range opened {
		if p == "/home/example/real" {
			realSeen = true
		}
	}
	if !realSeen {
		t.Errorf("expected real project /home/example/real to be opened; opened=%v", opened)
	}
}
