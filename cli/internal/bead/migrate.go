package bead

import (
	"fmt"
	"time"
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
	return s.ArchiveWithEvents(migratePolicy())
}

// MigrateDryRun reports what Migrate would do without mutating disk. It uses
// the same eligibility logic as Migrate (Statuses=[closed], no MinAge floor,
// preserve_dependencies retention).
func (s *Store) MigrateDryRun() (MigrateStats, error) {
	var stats MigrateStats
	if s.Collection != DefaultCollection {
		return stats, fmt.Errorf("bead: migrate dry-run only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}
	policy := migratePolicy()

	var beads []Bead
	err := s.WithLock(func() error {
		all, _, rerr := s.readAllLatestRaw()
		if rerr != nil {
			return rerr
		}
		beads = all
		return nil
	})
	if err != nil {
		return stats, err
	}

	referenced := make(map[string]bool)
	for _, b := range beads {
		if b.Status == StatusClosed {
			continue
		}
		for _, dep := range b.DepIDs() {
			referenced[dep] = true
		}
	}

	for _, b := range beads {
		if !containsString(policy.Statuses, b.Status) {
			continue
		}
		if hasInlineEvents(&b) {
			stats.EventsExternalized++
		}
		if referenced[b.ID] {
			continue
		}
		ts := b.UpdatedAt
		if raw, ok := b.Extra["closed_at"].(string); ok {
			if parsed, perr := time.Parse(time.RFC3339, raw); perr == nil {
				ts = parsed
			}
		}
		if policy.MinAge > 0 && time.Now().UTC().Sub(ts) < policy.MinAge {
			continue
		}
		stats.Archived++
	}
	return stats, nil
}

func migratePolicy() ArchivePolicy {
	return ArchivePolicy{
		Statuses:       []string{StatusClosed},
		MinAge:         0,
		MinActiveCount: 0,
		BatchSize:      0,
	}
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

// MigrateAxonStats reports what MigrateToAxon copied in a single pass.
type MigrateAxonStats struct {
	// BeadsMigrated is the total number of distinct bead IDs written into
	// the axon backend (active + archive, deduped on ID).
	BeadsMigrated int
	// EventsMigrated is the total number of inline events written into the
	// ddx_bead_events collection. Externalized events (referenced via
	// Extra[events_attachment]) are not counted because they are not
	// rewritten by the migration — the attachment file remains canonical.
	EventsMigrated int
}

// MigrateToAxon reads beads from the JSONL active collection
// (.ddx/beads.jsonl) and the JSONL archive partner
// (.ddx/beads-archive.jsonl) rooted under s.Dir and writes them losslessly
// into the axon backend (.ddx/axon/). Source files are not modified.
//
// Reads always go through fresh JSONL backends regardless of s.backend so
// the migration is deterministic and not affected by the operator's
// configured backend (e.g. an axon-misconfigured store still migrates from
// the on-disk JSONL files). Active wins on duplicate IDs.
//
// Idempotent: AxonBackend.WriteAll overwrites both collection files in
// temp+rename style, so re-running on the same source state produces an
// identical axon snapshot with no duplicates.
func (s *Store) MigrateToAxon() (MigrateAxonStats, error) {
	var stats MigrateAxonStats

	activeSpec := DefaultRegistry().Resolve(DefaultCollection)
	activeFile, activeLock := activeSpec.PathsUnder(s.Dir)
	activeBackend := NewJSONLBackend(s.Dir, activeFile, activeLock, s.LockWait)

	archiveSpec := DefaultRegistry().Resolve(BeadsArchiveCollection)
	archiveFile, archiveLock := archiveSpec.PathsUnder(s.Dir)
	archiveBackend := NewJSONLBackend(s.Dir, archiveFile, archiveLock, s.LockWait)

	activeBeads, err := activeBackend.ReadAll()
	if err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon read active: %w", err)
	}
	archiveBeads, err := archiveBackend.ReadAll()
	if err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon read archive: %w", err)
	}

	seen := make(map[string]bool, len(activeBeads))
	merged := make([]Bead, 0, len(activeBeads)+len(archiveBeads))
	for _, b := range activeBeads {
		seen[b.ID] = true
		merged = append(merged, b)
	}
	for _, b := range archiveBeads {
		if seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		merged = append(merged, b)
	}

	for i := range merged {
		stats.BeadsMigrated++
		if hasInlineEvents(&merged[i]) {
			stats.EventsMigrated += len(decodeBeadEvents(merged[i].Extra["events"]))
		}
	}

	axon := NewAxonBackend(s.Dir, s.LockWait)
	if err := axon.Init(); err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon init: %w", err)
	}
	if err := axon.WithLock(func() error {
		return axon.WriteAll(merged)
	}); err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon write: %w", err)
	}
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
