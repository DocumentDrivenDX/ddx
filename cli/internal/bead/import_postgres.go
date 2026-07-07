package bead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// axonImportStore is the narrow write surface the JSONL -> Axon importer uses.
// Store satisfies it directly, and tests can swap in a recording fake.
type axonImportStore interface {
	Create(context.Context, *Bead) error
	Apply(string, Operation) error
}

// AppendEventOp appends one imported event to the bead's inline event log.
// The Axon backend splits that inline representation into the bead and event
// collections when Store.Apply persists the updated bead.
type AppendEventOp struct {
	Event BeadEvent
}

func (op AppendEventOp) Apply(b *Bead) error {
	if b == nil {
		return fmt.Errorf("bead: append event requires bead")
	}
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	events := decodeBeadEvents(b.Extra["events"])
	events = append(events, op.Event)
	b.Extra["events"] = encodeEventsForExtra(events)
	return nil
}

// migrateToAxon imports the JSONL corpus into the configured Axon-backed
// store using the same Create/Apply surface the backend exposes to callers.
func (s *Store) migrateToAxon(ctx context.Context) (MigrateAxonStats, error) {
	if s == nil {
		return MigrateAxonStats{}, fmt.Errorf("bead: nil store")
	}
	if err := ctx.Err(); err != nil {
		return MigrateAxonStats{}, err
	}
	return importJSONLCorpusToAxon(ctx, s, s.Dir)
}

func importJSONLCorpusToAxon(ctx context.Context, axonStore axonImportStore, dir string) (MigrateAxonStats, error) {
	var stats MigrateAxonStats
	if axonStore == nil {
		return stats, fmt.Errorf("bead: nil axon store")
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}

	active, err := readImportCorpus(filepath.Join(dir, DefaultCollection+".jsonl"))
	if err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon read active: %w", err)
	}
	archive, err := readImportCorpus(filepath.Join(dir, BeadsArchiveCollection+".jsonl"))
	if err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon read archive: %w", err)
	}

	seen := make(map[string]struct{}, len(active)+len(archive))
	for _, bead := range append(active, archive...) {
		if _, ok := seen[bead.ID]; ok {
			continue
		}
		seen[bead.ID] = struct{}{}

		imported, err := importBeadToAxon(ctx, axonStore, bead)
		if err != nil {
			return stats, err
		}
		if !imported {
			continue
		}
		stats.BeadsMigrated++
		stats.EventsMigrated += importedEventCount(bead)
	}

	return stats, nil
}

func importBeadToAxon(ctx context.Context, axonStore axonImportStore, bead Bead) (bool, error) {
	// The importer writes the bead row first, then appends events in created_at
	// order. Events are imported via Apply so the backend exercises the same
	// mutation surface as normal callers.
	created := beadWithoutInlineEvents(bead)
	if created.Extra != nil {
		delete(created.Extra, EventsAttachmentExtraKey)
		if len(created.Extra) == 0 {
			created.Extra = nil
		}
	}

	if err := axonStore.Create(ctx, &created); err != nil {
		if isDuplicateImportError(err) {
			return false, nil
		}
		return false, fmt.Errorf("bead: migrate-to-axon create %s: %w", bead.ID, err)
	}

	events := importedEvents(bead)
	for _, ev := range events {
		if err := axonStore.Apply(created.ID, AppendEventOp{Event: ev}); err != nil {
			if isDuplicateImportError(err) {
				continue
			}
			return true, fmt.Errorf("bead: migrate-to-axon append event %s: %w", bead.ID, err)
		}
	}
	return true, nil
}

func importedEvents(bead Bead) []BeadEvent {
	var events []BeadEvent
	if bead.Extra != nil {
		events = decodeBeadEvents(bead.Extra["events"])
	}
	if len(events) < 2 {
		return events
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return events
}

func importedEventCount(bead Bead) int {
	if bead.Extra == nil {
		return 0
	}
	return len(decodeBeadEvents(bead.Extra["events"]))
}

func readImportCorpus(path string) ([]Bead, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: read %s: %w", path, err)
	}

	var incoming []Bead
	var parseErrors int

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("bead: import parse: %w", err)
		}
		for _, r := range raw {
			b, err := unmarshalBead(r)
			if err != nil {
				parseErrors++
				continue
			}
			incoming = append(incoming, b)
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			b, err := unmarshalBead([]byte(line))
			if err != nil {
				parseErrors++
				continue
			}
			incoming = append(incoming, b)
		}
	}

	if parseErrors > 0 && len(incoming) == 0 {
		return nil, fmt.Errorf("bead: import failed: %d malformed record(s), 0 valid", parseErrors)
	}
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "bead: import: skipped %d malformed record(s)\n", parseErrors)
	}
	return incoming, nil
}

func isDuplicateImportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrConflict) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate id") || strings.Contains(msg, "already exists")
}
