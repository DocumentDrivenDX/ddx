package bead

import (
	"context"
	"fmt"
	"os"
	"time"
)

// BeadsArchiveCollection is the logical collection name for archived beads.
// It is registered in the default registry alongside the active "beads"
// collection and is backed by .ddx/beads-archive.jsonl in the JSONL backend.
const BeadsArchiveCollection = "beads-archive"

// DefaultArchiveSizeThreshold is the active .ddx/beads.jsonl size above which
// routine close-time maintenance externalizes closed-bead events and archives
// eligible closed rows. Keep this in the bead package so CLI and Store paths
// share one trigger.
const DefaultArchiveSizeThreshold int64 = 4 * 1024 * 1024

// ArchivePolicy parameterises which closed beads are eligible to move from
// the active collection to the archive. Defaults match TD-027 §(b).
type ArchivePolicy struct {
	// Statuses lists the bead statuses that are eligible for archival.
	Statuses []string
	// MinAge is the minimum time since a bead was last updated/closed
	// before it may be archived.
	MinAge time.Duration
	// MinActiveCount is the floor on active-collection size below which
	// archival is a no-op. Set to 0 to archive regardless of size.
	MinActiveCount int
	// BatchSize caps how many records move per call.
	BatchSize int
}

// archivePartner returns a Store for the beads-archive collection rooted at
// the same .ddx directory as s. The partner reuses the shipping registry
// spec for "beads-archive" so file/lock paths are uniform.
func (s *Store) archivePartner() *Store {
	return NewStoreWithCollection(s.Dir, BeadsArchiveCollection)
}

// Archive moves eligible closed beads from this active store to the
// beads-archive partner. It applies preserve_dependencies semantics from
// TD-027 §(b): a closed bead that an open or in_progress bead still
// references is retained in the active collection so SD-004 queue
// derivation can resolve it without loading the archive.
//
// Returns the IDs that were moved.
func (s *Store) Archive(ctx context.Context, policy ArchivePolicy) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Collection != DefaultCollection {
		return nil, fmt.Errorf("bead: archive only runs from the active %q collection (got %q)", DefaultCollection, s.Collection)
	}
	archive := s.archivePartner()

	var moved []string
	// Lock order is fixed: active first, archive second, per TD-027 §(b).
	err := s.WithLock(func() error {
		return archive.WithLock(func() error {
			activeBeads, _, rerr := s.readAllLatestRaw()
			if rerr != nil {
				return rerr
			}
			if policy.MinActiveCount > 0 && len(activeBeads) < policy.MinActiveCount {
				return nil
			}

			// preserve_dependencies: skip closed beads still referenced by
			// any non-closed bead's dependency list.
			referenced := make(map[string]bool)
			for _, b := range activeBeads {
				if b.Status == StatusClosed {
					continue
				}
				for _, dep := range b.DepIDs() {
					referenced[dep] = true
				}
			}

			now := time.Now().UTC()
			eligible := make(map[string]bool)
			var toMove []Bead
			for _, b := range activeBeads {
				if !containsString(policy.Statuses, b.Status) {
					continue
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
				if policy.MinAge > 0 && now.Sub(ts) < policy.MinAge {
					continue
				}
				toMove = append(toMove, b)
				if policy.BatchSize > 0 && len(toMove) >= policy.BatchSize {
					break
				}
			}
			if len(toMove) == 0 {
				return nil
			}

			archiveBeads, _, aerr := archive.readAllLatestRaw()
			if aerr != nil {
				return aerr
			}
			archiveByID := make(map[string]int, len(archiveBeads))
			for i, b := range archiveBeads {
				archiveByID[b.ID] = i
			}
			stamp := now.Format(time.RFC3339)
			for _, b := range toMove {
				if b.Extra == nil {
					b.Extra = make(map[string]any)
				}
				b.Extra["archived_at"] = stamp
				eligible[b.ID] = true
				if idx, ok := archiveByID[b.ID]; ok {
					archiveBeads[idx] = b
				} else {
					archiveByID[b.ID] = len(archiveBeads)
					archiveBeads = append(archiveBeads, b)
				}
			}

			// Step 7 of TD-027 mutation sequence: write archive first, then
			// active. A crash between the two leaves a duplicate in both
			// collections; merged-view reads in (e) hide that with
			// active-wins precedence.
			if werr := archive.writeAllLocked(archiveBeads); werr != nil {
				return fmt.Errorf("bead: write archive: %w", werr)
			}
			remaining := make([]Bead, 0, len(activeBeads)-len(toMove))
			for _, b := range activeBeads {
				if eligible[b.ID] {
					moved = append(moved, b.ID)
					continue
				}
				remaining = append(remaining, b)
			}
			if werr := s.writeAllLocked(remaining); werr != nil {
				return fmt.Errorf("bead: write active: %w", werr)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

// GetWithArchive resolves a bead ID across the active collection and the
// archive partner, with active-wins precedence. This is the lookup the
// `ddx bead show` command uses so a closed-and-archived bead remains
// addressable by ID.
func (s *Store) GetWithArchive(ctx context.Context, id string) (*Bead, error) {
	if b, err := s.Get(ctx, id); err == nil {
		return b, nil
	}
	if s.Collection != DefaultCollection {
		return nil, fmt.Errorf("bead: not found: %s", id)
	}
	archive := s.archivePartner()
	b, err := archive.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("bead: not found: %s", id)
	}
	return b, nil
}

// ListWithArchive returns the union of active and archive beads filtered
// the same way as List. Active records take precedence over archive
// duplicates that may exist after an interrupted archive run.
func (s *Store) ListWithArchive(status, label string, where map[string]string) ([]Bead, error) {
	active, err := s.List(status, label, where)
	if err != nil {
		return nil, err
	}
	if s.Collection != DefaultCollection {
		return active, nil
	}
	archive := s.archivePartner()
	archived, err := archive.List(status, label, where)
	if err != nil {
		// Archive may not exist yet — treat as empty.
		return active, nil //nolint:nilerr
	}
	seen := make(map[string]bool, len(active))
	out := make([]Bead, 0, len(active)+len(archived))
	for _, b := range active {
		seen[b.ID] = true
		out = append(out, b)
	}
	for _, b := range archived {
		if seen[b.ID] {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

// maybeOpportunisticMaintenance runs the same size-triggered maintenance as
// `ddx bead archive` after close-causing mutations: if active beads.jsonl is
// over the default 4 MiB threshold, closed-bead events are externalized and
// eligible closed rows move to beads-archive. Errors are swallowed because
// maintenance must not fail the close that triggered it.
func (s *Store) maybeOpportunisticMaintenance() {
	if s.Collection != DefaultCollection {
		return
	}
	info, err := os.Stat(s.File)
	if err != nil || info.Size() <= DefaultArchiveSizeThreshold {
		return
	}
	_, _ = s.ArchiveWithEvents(context.Background(), ArchivePolicy{
		Statuses: []string{StatusClosed},
	})
}
