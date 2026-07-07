package bead

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AppendEventOp appends one imported event to the bead's inline event log.
// The Axon backend splits that inline representation into the bead and event
// collections when Store.WriteAll persists the updated bead corpus.
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

type migrateAxonOptions struct {
	DryRun          bool
	Verify          bool
	CopyAttachments bool
}

// migrateToAxon imports the JSONL corpus into the configured Axon-backed
// store using the same write surface the backend exposes to callers.
func (s *Store) migrateToAxon(ctx context.Context) (MigrateAxonStats, error) {
	if s == nil {
		return MigrateAxonStats{}, fmt.Errorf("bead: nil store")
	}
	if err := ctx.Err(); err != nil {
		return MigrateAxonStats{}, err
	}
	return importJSONLCorpusToAxon(ctx, s, s.Dir, migrateAxonOptions{
		CopyAttachments: true,
	})
}

func importJSONLCorpusToAxon(ctx context.Context, target *Store, sourceDir string, opts migrateAxonOptions) (MigrateAxonStats, error) {
	var stats MigrateAxonStats
	if target == nil {
		return stats, fmt.Errorf("bead: nil axon store")
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}

	sourceBeads, err := loadImportCorpusForAxon(sourceDir)
	if err != nil {
		return stats, err
	}

	prepared, eventsMigrated, attachmentsMigrated, err := prepareImportedAxonCorpus(sourceDir, sourceBeads)
	if err != nil {
		return stats, err
	}

	if opts.DryRun {
		stats.BeadsMigrated = len(prepared)
		stats.EventsMigrated = eventsMigrated
		stats.AttachmentsMigrated = attachmentsMigrated
		return stats, nil
	}

	current, err := target.ReadAll(ctx)
	if err != nil {
		return stats, fmt.Errorf("bead: migrate-to-axon read target: %w", err)
	}
	if !corpusMatchesImportedAxon(sourceDir, prepared, current) {
		if err := target.WriteAll(prepared); err != nil {
			return stats, fmt.Errorf("bead: migrate-to-axon write corpus: %w", err)
		}
		stats.BeadsMigrated = len(prepared)
		stats.EventsMigrated = eventsMigrated
	}

	if opts.CopyAttachments {
		written, err := copyImportedAttachmentSidecars(sourceDir, target.Dir, sourceBeads)
		if err != nil {
			return stats, err
		}
		stats.AttachmentsMigrated = written
	}

	if opts.Verify {
		if err := verifyImportedAxonCorpus(ctx, target, sourceDir, prepared); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

func loadImportCorpusForAxon(sourceDir string) ([]Bead, error) {
	active, err := readImportCorpus(filepath.Join(sourceDir, DefaultCollection+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("bead: migrate-to-axon read active: %w", err)
	}
	archive, err := readImportCorpus(filepath.Join(sourceDir, BeadsArchiveCollection+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("bead: migrate-to-axon read archive: %w", err)
	}
	return dedupeImportedBeads(append(active, archive...)), nil
}

func dedupeImportedBeads(incoming []Bead) []Bead {
	seen := make(map[string]struct{}, len(incoming))
	out := make([]Bead, 0, len(incoming))
	for _, bead := range incoming {
		if _, ok := seen[bead.ID]; ok {
			continue
		}
		seen[bead.ID] = struct{}{}
		out = append(out, bead)
	}
	return out
}

func prepareImportedAxonCorpus(sourceDir string, sourceBeads []Bead) ([]Bead, int, int, error) {
	prepared := make([]Bead, 0, len(sourceBeads))
	eventsMigrated := 0
	attachmentsMigrated := 0
	for _, bead := range sourceBeads {
		events, err := importedEventsForAxon(sourceDir, bead)
		if err != nil {
			return nil, 0, 0, err
		}
		cp := bead
		if cp.Extra == nil {
			cp.Extra = make(map[string]any)
		}
		if len(events) > 0 {
			cp.Extra["events"] = encodeEventsForExtra(events)
		} else {
			delete(cp.Extra, "events")
		}
		prepared = append(prepared, cp)
		eventsMigrated += len(events)
		if sourceAttachmentExists(sourceDir, bead) {
			attachmentsMigrated++
		}
	}
	return prepared, eventsMigrated, attachmentsMigrated, nil
}

func importedEventsForAxon(sourceDir string, bead Bead) ([]BeadEvent, error) {
	var events []BeadEvent
	if bead.Extra != nil {
		if hasEventsAttachment(&bead) {
			loaded, err := readSourceEventsAttachment(sourceDir, bead)
			if err != nil {
				return nil, err
			}
			events = loaded
		} else {
			events = decodeBeadEvents(bead.Extra["events"])
		}
	}
	if len(events) < 2 {
		return events, nil
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return events, nil
}

func sourceAttachmentExists(sourceDir string, bead Bead) bool {
	if !hasEventsAttachment(&bead) {
		return false
	}
	_, err := os.Stat(sourceEventsAttachmentPath(sourceDir, bead))
	return err == nil
}

func sourceEventsAttachmentPath(sourceDir string, bead Bead) string {
	if bead.Extra == nil {
		return filepath.Join(sourceDir, "attachments", bead.ID, EventsAttachmentFileName)
	}
	rel, _ := bead.Extra[EventsAttachmentExtraKey].(string)
	if strings.TrimSpace(rel) == "" {
		return filepath.Join(sourceDir, "attachments", bead.ID, EventsAttachmentFileName)
	}
	return filepath.Join(sourceDir, "attachments", filepath.FromSlash(rel))
}

func readSourceEventsAttachment(sourceDir string, bead Bead) ([]BeadEvent, error) {
	path := sourceEventsAttachmentPath(sourceDir, bead)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bead: read source events attachment %s: %w", path, err)
	}
	return decodeEventsAttachmentBytes(data, path)
}

func decodeEventsAttachmentBytes(data []byte, path string) ([]BeadEvent, error) {
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

func copyImportedAttachmentSidecars(sourceDir, targetDir string, sourceBeads []Bead) (int, error) {
	written := 0
	for _, bead := range sourceBeads {
		if !sourceAttachmentExists(sourceDir, bead) {
			continue
		}
		changed, err := copyImportedAttachmentSidecar(sourceDir, targetDir, bead.ID)
		if err != nil {
			return written, err
		}
		if changed {
			written++
		}
	}
	return written, nil
}

func copyImportedAttachmentSidecar(sourceDir, targetDir, beadID string) (bool, error) {
	src := filepath.Join(sourceDir, "attachments", beadID, EventsAttachmentFileName)
	data, err := os.ReadFile(src)
	if err != nil {
		return false, fmt.Errorf("bead: read attachment %s: %w", src, err)
	}
	dst := filepath.Join(targetDir, AxonDirName, "attachments", beadID, EventsAttachmentFileName)
	if existing, err := os.ReadFile(dst); err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("bead: mkdir attachment target: %w", err)
	}
	return true, writeAtomicFile(dst, data)
}

func corpusMatchesImportedAxon(sourceDir string, want []Bead, got []Bead) bool {
	if len(want) != len(got) {
		return false
	}
	wantByID := make(map[string]Bead, len(want))
	for _, bead := range want {
		wantByID[bead.ID] = bead
	}
	for _, bead := range got {
		want, ok := wantByID[bead.ID]
		if !ok {
			return false
		}
		wantNorm, err := canonicalizeImportedBeadForVerify(sourceDir, want)
		if err != nil {
			return false
		}
		gotNorm, err := canonicalizeImportedBeadForVerify(sourceDir, bead)
		if err != nil {
			return false
		}
		wantJSON, err := marshalBead(wantNorm)
		if err != nil {
			return false
		}
		gotJSON, err := marshalBead(gotNorm)
		if err != nil {
			return false
		}
		if string(wantJSON) != string(gotJSON) {
			return false
		}
	}
	return true
}

func verifyImportedAxonCorpus(ctx context.Context, target *Store, sourceDir string, sourceBeads []Bead) error {
	if target == nil {
		return fmt.Errorf("bead: nil axon store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	wantByID := make(map[string]Bead, len(sourceBeads))
	for _, bead := range sourceBeads {
		wantByID[bead.ID] = bead
	}

	got, err := target.ReadAll(ctx)
	if err != nil {
		return fmt.Errorf("bead: verify read back: %w", err)
	}
	gotByID := make(map[string]Bead, len(got))
	for _, bead := range got {
		gotByID[bead.ID] = bead
	}

	for _, id := range sampledBeadIDs(sourceBeads) {
		want, ok := wantByID[id]
		if !ok {
			continue
		}
		got, ok := gotByID[id]
		if !ok {
			return fmt.Errorf("bead: verify missing imported bead %s", id)
		}

		wantNorm, err := canonicalizeImportedBeadForVerify(sourceDir, want)
		if err != nil {
			return err
		}
		gotNorm, err := canonicalizeImportedBeadForVerify(sourceDir, got)
		if err != nil {
			return err
		}

		wantJSON, err := marshalBead(wantNorm)
		if err != nil {
			return fmt.Errorf("bead: verify marshal source %s: %w", id, err)
		}
		gotJSON, err := marshalBead(gotNorm)
		if err != nil {
			return fmt.Errorf("bead: verify marshal target %s: %w", id, err)
		}
		if string(wantJSON) != string(gotJSON) {
			return fmt.Errorf("bead: verify drift for %s: source=%s target=%s", id, string(wantJSON), string(gotJSON))
		}
	}
	return nil
}

func sampledBeadIDs(sourceBeads []Bead) []string {
	if len(sourceBeads) == 0 {
		return nil
	}
	ids := make([]string, 0, len(sourceBeads))
	for _, bead := range sourceBeads {
		ids = append(ids, bead.ID)
	}
	sort.Strings(ids)
	if len(ids) <= 3 {
		return ids
	}
	return []string{ids[0], ids[len(ids)/2], ids[len(ids)-1]}
}

func canonicalizeImportedBeadForVerify(sourceDir string, bead Bead) (Bead, error) {
	cp := bead
	cp.SchemaVersion = 0
	if cp.Extra == nil {
		return cp, nil
	}
	if hasEventsAttachment(&cp) {
		events, err := readSourceEventsAttachment(sourceDir, cp)
		if err != nil {
			return Bead{}, err
		}
		cp.Extra["events"] = encodeEventsForExtra(events)
		delete(cp.Extra, EventsAttachmentExtraKey)
	}
	return cp, nil
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
