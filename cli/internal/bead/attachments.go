package bead

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EventsAttachmentExtraKey is the Extra map key under which a closed/archived
// bead records the path to its externalized events.jsonl sidecar. Per ADR-004
// §"Attachment Model", attachment references live in preserved extras and
// must not shadow bd/br envelope fields. The value is a path relative to
// `<.ddx-dir>/attachments/`, e.g. "ddx-abcdef12/events.jsonl".
const EventsAttachmentExtraKey = "events_attachment"

// EventsAttachmentFileName is the conventional sidecar filename for
// externalized events.
const EventsAttachmentFileName = "events.jsonl"

// attachmentDir returns the absolute directory under .ddx/attachments/ that
// owns this bead's sidecars.
func (s *Store) attachmentDir(beadID string) string {
	return filepath.Join(s.Dir, "attachments", beadID)
}

// eventsAttachmentPath returns the absolute path to a bead's events.jsonl
// sidecar.
func (s *Store) eventsAttachmentPath(beadID string) string {
	return filepath.Join(s.attachmentDir(beadID), EventsAttachmentFileName)
}

// eventsAttachmentRelPath returns the path stored in Extra[events_attachment]:
// it's relative to .ddx/attachments/ and uses forward slashes regardless of
// platform so the value round-trips through bd/br interchange unchanged.
func eventsAttachmentRelPath(beadID string) string {
	return beadID + "/" + EventsAttachmentFileName
}

// hasEventsAttachment reports whether the bead carries an externalized events
// sidecar reference.
func hasEventsAttachment(b *Bead) bool {
	if b == nil || b.Extra == nil {
		return false
	}
	v, ok := b.Extra[EventsAttachmentExtraKey].(string)
	return ok && strings.TrimSpace(v) != ""
}

// readEventsAttachment loads a bead's externalized events from disk. Missing
// file is treated as an empty event log so a half-written close does not
// crash the read path; it returns ([]BeadEvent{}, nil) in that case.
func (s *Store) readEventsAttachment(beadID string) ([]BeadEvent, error) {
	path := s.eventsAttachmentPath(beadID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []BeadEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: read events attachment %s: %w", path, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var out []BeadEvent
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, fmt.Errorf("bead: parse events attachment %s: %w", path, err)
		}
		out = append(out, beadEventFromMap(m))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bead: scan events attachment %s: %w", path, err)
	}
	if out == nil {
		out = []BeadEvent{}
	}
	return out, nil
}

// writeEventsAttachment writes the given events to the bead's sidecar file
// atomically (temp + rename) and creates the parent directory. An empty
// slice removes the sidecar.
func (s *Store) writeEventsAttachment(beadID string, events []BeadEvent) error {
	dir := s.attachmentDir(beadID)
	path := s.eventsAttachmentPath(beadID)
	if len(events) == 0 {
		_ = os.Remove(path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("bead: mkdir attachments: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, e := range events {
		row := map[string]any{
			"kind":       e.Kind,
			"summary":    e.Summary,
			"body":       e.Body,
			"actor":      e.Actor,
			"created_at": e.CreatedAt.UTC().Format(time.RFC3339Nano),
			"source":     e.Source,
		}
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("bead: encode events attachment: %w", err)
		}
	}
	return writeAtomicFile(path, buf.Bytes())
}

// eventsForBead returns the bead's events from whichever physical store
// currently holds them: the inline Extra["events"] array, or the externalized
// sidecar referenced by Extra[events_attachment]. When both are present the
// sidecar wins, matching the externalize-then-clear ordering used by Close.
func (s *Store) eventsForBead(b *Bead) ([]BeadEvent, error) {
	if b == nil {
		return nil, nil
	}
	if hasEventsAttachment(b) {
		return s.readEventsAttachment(b.ID)
	}
	if b.Extra == nil {
		return nil, nil
	}
	if raw, ok := b.Extra["events"]; ok {
		return decodeBeadEvents(raw), nil
	}
	return nil, nil
}

// encodeEventsForExtra mirrors the in-memory shape AppendEvent already uses
// when storing events inline so that round-trips between attachment and inline
// produce identical JSON.
func encodeEventsForExtra(events []BeadEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, e := range events {
		out = append(out, map[string]any{
			"kind":       e.Kind,
			"summary":    e.Summary,
			"body":       e.Body,
			"actor":      e.Actor,
			"created_at": e.CreatedAt,
			"source":     e.Source,
		})
	}
	return out
}

// externalizeEventsInPlace moves a bead's inline events to its sidecar. It
// mutates b: drops Extra["events"] and sets Extra[events_attachment]. The
// sidecar file is written first so a crash leaves the inline copy intact;
// after the row is persisted the inline copy is dropped.
//
// No-op when there are no inline events to move (already externalized, or
// never had any). Removes Extra[events_attachment] when the bead has neither
// inline nor attached events so the row stays clean.
func (s *Store) externalizeEventsInPlace(b *Bead) error {
	if b == nil {
		return nil
	}
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	var inline []BeadEvent
	if raw, ok := b.Extra["events"]; ok {
		inline = decodeBeadEvents(raw)
	}
	if len(inline) == 0 {
		return nil
	}
	if err := s.writeEventsAttachment(b.ID, inline); err != nil {
		return err
	}
	delete(b.Extra, "events")
	b.Extra[EventsAttachmentExtraKey] = eventsAttachmentRelPath(b.ID)
	return nil
}

// LoadEventsInline pulls a bead's externalized events back into Extra["events"]
// for callers that need a single uniform shape (notably JSON output and
// export-for-interchange paths). It does not touch the on-disk row; the bead
// value is mutated in memory only.
func (s *Store) LoadEventsInline(b *Bead) error {
	return s.inlineEventsInPlace(b)
}

// inlineEventsInPlace is the inverse of externalizeEventsInPlace: it loads any
// sidecar events back into Extra["events"] and clears the attachment ref.
// Used by Reopen so an active bead returns to fully-inline storage, and by
// the export path which inlines transiently for marshaling without touching
// the on-disk row (callers that don't want the bead mutated must work on a
// copy — Bead is a value type so the typical usage is to pass a copy).
func (s *Store) inlineEventsInPlace(b *Bead) error {
	if b == nil {
		return nil
	}
	if !hasEventsAttachment(b) {
		return nil
	}
	events, err := s.readEventsAttachment(b.ID)
	if err != nil {
		return err
	}
	if b.Extra == nil {
		b.Extra = make(map[string]any)
	}
	if len(events) > 0 {
		b.Extra["events"] = encodeEventsForExtra(events)
	} else {
		delete(b.Extra, "events")
	}
	delete(b.Extra, EventsAttachmentExtraKey)
	return nil
}
