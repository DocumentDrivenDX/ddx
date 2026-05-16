package bead

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BackendAxon selects the axon-backed RawBackend. Per TD-030 D3 the Axon
// shape splits beads from their events: one row per bead in the
// ddx_beads collection, plus one entity per event in the
// ddx_bead_events collection linked back to its bead via event_of.
//
// When a GraphQL transport is configured (test code wires one in via the
// AxonBackend.GraphQLTransport field), AxonBackend speaks that wire shape
// directly. The fallback path keeps the existing in-process JSONL emulation
// for callers that have not wired a transport yet.
//
// NewStore routes to AxonBackend when bead.backend is set to axon in the
// config or via DDX_BEAD_BACKEND.
const BackendAxon = "axon"

// axonSchemaVersion is written into every persisted entity. Schema upgrades
// are handled lazy-on-read until axon FEAT-017 ships native schema
// migration (per TD-030 v1 policy on schema versioning).
const axonSchemaVersion = 1

// AxonDirName is the per-store axon subdirectory under .ddx that holds the
// two-collection snapshot files and the lock dir.
const AxonDirName = "axon"

// AxonBeadsCollection / AxonEventsCollection are the logical collection names
// from TD-030's schema mapping. They are surfaced as the on-disk filenames so
// inspecting the directory tells the operator which collection an entity
// belongs to.
const (
	AxonBeadsCollection  = "ddx_beads"
	AxonEventsCollection = "ddx_bead_events"
)

// AxonBackend stores the bead corpus in axon's two-collection shape.
//
// On WriteAll the backend splits each bead's inline Extra["events"] into one
// entity per event under ddx_bead_events and persists the bead row (sans
// inline events) under ddx_beads. ReadAll re-merges events back into
// Extra["events"] so the higher-level Store and the chaos suite see the
// uniform inline-events shape they expect.
//
// Beads with Extra[events_attachment] set (post-Close externalisation) are
// persisted with their events array empty in axon's events collection. The
// attachment file remains the canonical source for those events so Store's
// existing read path (eventsForBead) keeps working unchanged.
type AxonBackend struct {
	Dir              string
	BeadsFile        string
	EventsFile       string
	LockDir          string
	LockWait         time.Duration
	GraphQLTransport AxonGraphQLTransport
	GraphQLClient    any
}

// AxonGraphQLTransport is the injectable GraphQL execution boundary used by
// the Axon backend. It mirrors the generated client's transport surface, so a
// caller can wire a real transport into the future GraphQL implementation
// without changing the storage code that uses this backend.
type AxonGraphQLTransport interface {
	Query(ctx context.Context, query string, variables map[string]any, response any) error
}

// Compile-time check: AxonBackend satisfies RawBackend.
var _ RawBackend = (*AxonBackend)(nil)

// NewAxonBackend constructs an axon-backed RawBackend rooted at dir. dir is
// the .ddx directory for the project; collection files and the lock live
// under <dir>/axon/.
func NewAxonBackend(dir string, lockWait time.Duration) *AxonBackend {
	root := filepath.Join(dir, AxonDirName)
	return &AxonBackend{
		Dir:        root,
		BeadsFile:  filepath.Join(root, AxonBeadsCollection+".jsonl"),
		EventsFile: filepath.Join(root, AxonEventsCollection+".jsonl"),
		LockDir:    filepath.Join(root, ".lock"),
		LockWait:   lockWait,
	}
}

func (a *AxonBackend) Init() error {
	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: axon init dir: %w", err)
	}
	for _, path := range []string{a.BeadsFile, a.EventsFile} {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("bead: axon init %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("bead: axon close %s: %w", path, err)
		}
	}
	return nil
}

const (
	axonReadCorpusQuery     = `query AxonReadCorpus { ddxBeads { _collection schema_version data } ddxBeadEvents { _collection schema_version event_of index event } }`
	axonWriteCorpusMutation = `mutation AxonWriteCorpus($beads: [AxonBeadRowInput!]!, $events: [AxonEventRowInput!]!) { saveCorpus(beads: $beads, events: $events) { ok } }`
)

type axonCorpusResponse struct {
	Beads  []axonEntityEnvelope `json:"ddxBeads"`
	Events []axonEventEnvelope  `json:"ddxBeadEvents"`
}

type axonWriteCorpusResponse struct {
	SaveCorpus axonWriteCorpusResult `json:"saveCorpus"`
}

type axonWriteCorpusResult struct {
	OK bool `json:"ok"`
}

// ReadAll loads bead rows and event entities from disk and re-merges events
// back into each bead's Extra["events"]. Orphan events (event_of pointing at
// a missing bead, e.g. after a partial-write crash) are silently dropped.
func (a *AxonBackend) ReadAll() ([]Bead, error) {
	if transport := a.graphQLTransport(); transport != nil {
		var resp axonCorpusResponse
		if err := transport.Query(context.Background(), axonReadCorpusQuery, nil, &resp); err != nil {
			return nil, fmt.Errorf("bead: axon read corpus: %w", err)
		}
		beads := make([]Bead, 0, len(resp.Beads))
		for _, env := range resp.Beads {
			if env.Collection != AxonBeadsCollection || env.Data.ID == "" {
				continue
			}
			beads = append(beads, env.Data.Bead)
		}
		events := make([]axonEventRecord, 0, len(resp.Events))
		for _, env := range resp.Events {
			if env.Collection != AxonEventsCollection || env.EventOf == "" {
				continue
			}
			events = append(events, axonEventRecord{
				beadID: env.EventOf,
				index:  env.Index,
				event:  env.Event,
			})
		}
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].beadID != events[j].beadID {
				return events[i].beadID < events[j].beadID
			}
			return events[i].index < events[j].index
		})
		byID := make(map[string]*Bead, len(beads))
		for i := range beads {
			byID[beads[i].ID] = &beads[i]
		}
		counts := make(map[string]int, len(events))
		for _, ev := range events {
			if _, ok := byID[ev.beadID]; !ok {
				continue
			}
			counts[ev.beadID]++
		}
		grouped := make(map[string][]BeadEvent, len(counts))
		for id, count := range counts {
			grouped[id] = make([]BeadEvent, 0, count)
		}
		for _, ev := range events {
			if _, ok := byID[ev.beadID]; !ok {
				continue
			}
			grouped[ev.beadID] = append(grouped[ev.beadID], ev.event)
		}
		for id, evs := range grouped {
			b := byID[id]
			// Always copy b.Extra before writing; two concurrent ReadAll calls
			// may share the underlying map (shallow-copy race via Bead copy-by-value).
			fresh := make(map[string]any, len(b.Extra)+1)
			for k, v := range b.Extra {
				fresh[k] = v
			}
			fresh["events"] = encodeEventsForExtra(evs)
			b.Extra = fresh
		}
		return beads, nil
	}

	beads, err := a.readBeadRows()
	if err != nil {
		return nil, err
	}
	events, err := a.readEventEntities()
	if err != nil {
		return nil, err
	}
	// Index beads by id so we can attach events and drop orphans.
	byID := make(map[string]*Bead, len(beads))
	for i := range beads {
		byID[beads[i].ID] = &beads[i]
	}
	// Group events by bead id with exact per-bead capacity so the read path
	// does not repeatedly grow slices while rebuilding the inline shape.
	counts := make(map[string]int, len(events))
	for _, ev := range events {
		if _, ok := byID[ev.beadID]; !ok {
			continue // orphan
		}
		counts[ev.beadID]++
	}
	grouped := make(map[string][]BeadEvent, len(counts))
	for id, count := range counts {
		grouped[id] = make([]BeadEvent, 0, count)
	}
	for _, ev := range events {
		if _, ok := byID[ev.beadID]; !ok {
			continue
		}
		grouped[ev.beadID] = append(grouped[ev.beadID], ev.event)
	}
	for id, evs := range grouped {
		b := byID[id]
		// Skip beads whose events have been externalised — the attachment file
		// is the canonical source post-close and inlining stale events would
		// duplicate the history.
		if hasEventsAttachment(b) {
			continue
		}
		// Always copy b.Extra before writing; two concurrent ReadAll calls
		// may share the underlying map (shallow-copy race via Bead copy-by-value).
		fresh := make(map[string]any, len(b.Extra)+1)
		for k, v := range b.Extra {
			fresh[k] = v
		}
		fresh["events"] = encodeEventsForExtra(evs)
		b.Extra = fresh
	}
	return beads, nil
}

// WriteAll splits inline events from each bead and persists both collections.
// Writes are serialised at the directory-lock layer (WithLock); this method
// rewrites both files in temp+rename style so a crash mid-write leaves the
// previous snapshot intact rather than producing torn rows.
func (a *AxonBackend) WriteAll(beads []Bead) error {
	if transport := a.graphQLTransport(); transport != nil {
		sort.Slice(beads, func(i, j int) bool { return beads[i].ID < beads[j].ID })
		var rowBeads []axonEntityEnvelope
		var rowEvents []axonEventEnvelope
		for _, b := range beads {
			var events []BeadEvent
			if b.Extra != nil {
				if raw, ok := b.Extra["events"]; ok {
					events = decodeBeadEvents(raw)
				}
			}
			stripped := beadWithoutInlineEvents(b)
			if stripped.Extra != nil {
				delete(stripped.Extra, EventsAttachmentExtraKey)
			}
			row, err := axonEncodeBead(stripped)
			if err != nil {
				return fmt.Errorf("bead: axon marshal %s: %w", b.ID, err)
			}
			var env axonEntityEnvelope
			if err := json.Unmarshal(row, &env); err != nil {
				return fmt.Errorf("bead: axon envelope %s: %w", b.ID, err)
			}
			rowBeads = append(rowBeads, env)
			for i, ev := range events {
				data, err := axonEncodeEvent(b.ID, i, ev)
				if err != nil {
					return fmt.Errorf("bead: axon marshal event %s/%d: %w", b.ID, i, err)
				}
				var eventEnv axonEventEnvelope
				if err := json.Unmarshal(data, &eventEnv); err != nil {
					return fmt.Errorf("bead: axon envelope event %s/%d: %w", b.ID, i, err)
				}
				rowEvents = append(rowEvents, eventEnv)
			}
		}
		var resp axonWriteCorpusResponse
		vars := map[string]any{
			"beads":  rowBeads,
			"events": rowEvents,
		}
		if err := transport.Query(context.Background(), axonWriteCorpusMutation, vars, &resp); err != nil {
			return fmt.Errorf("bead: axon write corpus: %w", err)
		}
		return nil
	}

	if err := os.MkdirAll(a.Dir, 0o755); err != nil {
		return fmt.Errorf("bead: axon mkdir: %w", err)
	}
	sort.Slice(beads, func(i, j int) bool { return beads[i].ID < beads[j].ID })

	var beadsBuf, eventsBuf bytes.Buffer
	for _, b := range beads {
		// Extract and strip inline events for the split.
		var events []BeadEvent
		if b.Extra != nil {
			if raw, ok := b.Extra["events"]; ok {
				events = decodeBeadEvents(raw)
			}
		}
		stripped := beadWithoutInlineEvents(b)
		row, err := axonEncodeBead(stripped)
		if err != nil {
			return fmt.Errorf("bead: axon marshal %s: %w", b.ID, err)
		}
		beadsBuf.Write(row)
		beadsBuf.WriteByte('\n')

		for i, ev := range events {
			data, err := axonEncodeEvent(b.ID, i, ev)
			if err != nil {
				return fmt.Errorf("bead: axon marshal event %s/%d: %w", b.ID, i, err)
			}
			eventsBuf.Write(data)
			eventsBuf.WriteByte('\n')
		}
	}

	// Write events first so a crash between the two writes leaves orphan
	// events (which ReadAll silently drops) rather than a bead row whose
	// events have not landed yet — favouring the safer side of the partial-
	// write window for an audit log.
	if err := writeAtomicFile(a.EventsFile, eventsBuf.Bytes()); err != nil {
		return fmt.Errorf("bead: axon write events: %w", err)
	}
	if err := writeAtomicFile(a.BeadsFile, beadsBuf.Bytes()); err != nil {
		return fmt.Errorf("bead: axon write beads: %w", err)
	}
	return nil
}

func (a *AxonBackend) WithLock(fn func() error) error {
	wait := a.LockWait
	if wait <= 0 {
		wait = 10 * time.Second
	}
	if err := acquireDirLock(a.Dir, a.LockDir, wait); err != nil {
		return err
	}
	defer os.RemoveAll(a.LockDir)
	return fn()
}

func (a *AxonBackend) graphQLTransport() AxonGraphQLTransport {
	if a == nil {
		return nil
	}
	if a.GraphQLTransport != nil {
		return a.GraphQLTransport
	}
	if transport, ok := a.GraphQLClient.(AxonGraphQLTransport); ok {
		return transport
	}
	return nil
}

func (a *AxonBackend) usesGraphQL() bool {
	return a.graphQLTransport() != nil
}

func (s *Store) axonBackend() *AxonBackend {
	if s == nil {
		return nil
	}
	ax, ok := s.backend.(*AxonBackend)
	if !ok || ax == nil || !ax.usesGraphQL() {
		return nil
	}
	return ax
}

func (s *Store) axonGraphQLActive() bool {
	return s.axonBackend() != nil
}

// beadWithoutInlineEvents returns a shallow copy of b whose Extra map omits
// the "events" key. The original Extra map is not mutated so callers can
// continue using the in-memory bead value after the split.
func beadWithoutInlineEvents(b Bead) Bead {
	if b.Extra == nil {
		return b
	}
	if _, ok := b.Extra["events"]; !ok {
		return b
	}
	cp := make(map[string]any, len(b.Extra))
	for k, v := range b.Extra {
		if k == "events" {
			continue
		}
		cp[k] = v
	}
	b.Extra = cp
	return b
}

// axonEncodeBead writes a bead in axon's row shape:
//
//	{ "_collection": "ddx_beads", "schema_version": 1, "data": {<bead JSON>} }
//
// The wrapper carries the collection name and schema version that a future
// genqlient-driven implementation needs; the data field is the same bead
// JSON the JSONL backend writes, so import/export round-trip continues to
// share marshalBead.
func axonEncodeBead(b Bead) ([]byte, error) {
	envelope := axonEntityEnvelope{
		Collection:    AxonBeadsCollection,
		SchemaVersion: axonSchemaVersion,
		Data:          axonBeadData{Bead: b},
	}
	return json.Marshal(envelope)
}

// axonEncodeEvent writes one event entity in the ddx_bead_events
// collection. The event_of field is the link target back to the parent bead
// (per TD-030 D3); index gives a deterministic position so re-reads preserve
// insertion order even when on-disk shuffling occurs.
func axonEncodeEvent(beadID string, index int, ev BeadEvent) ([]byte, error) {
	row := axonEventEnvelope{
		Collection:    AxonEventsCollection,
		SchemaVersion: axonSchemaVersion,
		EventOf:       beadID,
		Index:         index,
		Event:         ev,
	}
	return json.Marshal(row)
}

type axonEntityEnvelope struct {
	Collection    string       `json:"_collection"`
	SchemaVersion int          `json:"schema_version"`
	Data          axonBeadData `json:"data"`
}

type axonBeadData struct {
	Bead
}

func (d axonBeadData) MarshalJSON() ([]byte, error) {
	return marshalBead(d.Bead)
}

func (d *axonBeadData) UnmarshalJSON(data []byte) error {
	b, err := unmarshalBead(data)
	if err != nil {
		return err
	}
	d.Bead = b
	return nil
}

type axonEventEnvelope struct {
	Collection    string    `json:"_collection"`
	SchemaVersion int       `json:"schema_version"`
	EventOf       string    `json:"event_of"`
	Index         int       `json:"index"`
	Event         BeadEvent `json:"event"`
}

func (a *AxonBackend) readBeadRows() ([]Bead, error) {
	data, err := os.ReadFile(a.BeadsFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: axon read %s: %w", a.BeadsFile, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := make([]Bead, 0, bytes.Count(data, []byte{'\n'}))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env axonEntityEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, fmt.Errorf("bead: axon parse bead row: %w", err)
		}
		if env.Collection != AxonBeadsCollection || env.Data.ID == "" {
			continue
		}
		out = append(out, env.Data.Bead)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bead: axon scan beads: %w", err)
	}
	return foldLatestBeads(out), nil
}

type axonEventRecord struct {
	beadID string
	index  int
	event  BeadEvent
}

func (a *AxonBackend) readEventEntities() ([]axonEventRecord, error) {
	data, err := os.ReadFile(a.EventsFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bead: axon read %s: %w", a.EventsFile, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := make([]axonEventRecord, 0, bytes.Count(data, []byte{'\n'}))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env axonEventEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, fmt.Errorf("bead: axon parse event row: %w", err)
		}
		if env.Collection != AxonEventsCollection || env.EventOf == "" {
			continue
		}
		out = append(out, axonEventRecord{
			beadID: env.EventOf,
			index:  env.Index,
			event:  env.Event,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bead: axon scan events: %w", err)
	}
	// Stable sort within each bead so insertion order is recoverable on read
	// regardless of how WriteAll batched lines into the file.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].beadID != out[j].beadID {
			return out[i].beadID < out[j].beadID
		}
		return out[i].index < out[j].index
	})
	return out, nil
}
