package bead

import (
	"fmt"
)

// MigrateStats reports what Migrate did in a single pass.
type MigrateStats struct {
	// EventsExternalized is the number of beads whose inline events were
	// moved to the .ddx/attachments/<id>/events.jsonl sidecar.
	EventsExternalized int
	// Archived is the number of closed beads moved from the active
	// collection to beads-archive.
	Archived int
}

// Changed reports whether Migrate mutated on-disk state in this pass.
func (m MigrateStats) Changed() bool {
	return m.EventsExternalized > 0 || m.Archived > 0
}

// Migrate splits the existing active beads collection into the modern
// layout: closed beads' inline events are externalized to the attachment
// store (per ADR-004), and eligible closed beads are moved to the
// beads-archive partner (per TD-027). It is idempotent — a second call
// with no further changes is a no-op.
//
// Migrate uses a permissive archival policy (MinAge=0, MinActiveCount=0)
// so it drains the historical backlog. Routine archival (after Close)
// continues to use DefaultArchivePolicy.
func (s *Store) Migrate() (MigrateStats, error) {
	return s.ArchiveWithEvents(ArchivePolicy{
		Statuses:       []string{StatusClosed},
		MinAge:         0,
		MinActiveCount: 0,
		BatchSize:      0, // unlimited
	})
}

// ArchiveWithEvents externalizes inline events for every bead whose status
// matches policy.Statuses, then archives eligible beads under the same
// policy. This is the operator-facing path used by `ddx bead archive` and
// the internal path used by `ddx bead migrate`.
func (s *Store) ArchiveWithEvents(policy ArchivePolicy) (MigrateStats, error) {
	var stats MigrateStats
	if s.Collection != DefaultCollection {
		return stats, fmt.Errorf("bead: archive only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}

	// Step 1: externalize inline events on every eligible-status bead under
	// the active store's lock. This shrinks the row size before we try to
	// archive, so the archive partner inherits already-thin rows.
	err := s.WithLock(func() error {
		beads, _, rerr := s.readAllLatestRaw()
		if rerr != nil {
			return rerr
		}
		dirty := false
		for i := range beads {
			if !containsString(policy.Statuses, beads[i].Status) {
				continue
			}
			if !hasInlineEvents(&beads[i]) {
				continue
			}
			if eerr := s.externalizeEventsInPlace(&beads[i]); eerr != nil {
				return eerr
			}
			stats.EventsExternalized++
			dirty = true
		}
		if !dirty {
			return nil
		}
		return s.WriteAll(beads)
	})
	if err != nil {
		return stats, fmt.Errorf("bead: archive externalize: %w", err)
	}

	moved, err := s.Archive(policy)
	if err != nil {
		return stats, fmt.Errorf("bead: archive: %w", err)
	}
	stats.Archived = len(moved)
	return stats, nil
}

// hasInlineEvents reports whether a bead carries any inline events that
// can still be externalized. An empty array counts as "no inline events"
// so a second migrate pass on an already-externalized bead is a no-op.
func hasInlineEvents(b *Bead) bool {
	if b == nil || b.Extra == nil {
		return false
	}
	raw, ok := b.Extra["events"]
	if !ok {
		return false
	}
	return len(decodeBeadEvents(raw)) > 0
}
