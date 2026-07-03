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

	"github.com/DocumentDrivenDX/ddx/internal/bead/axon"
)

// BackendAxon selects the axon-backed RawBackend. Per TD-030 D3 the Axon
// shape splits beads from their events: one row per bead in the
// ddx_beads collection, plus one entity per event in the
// ddx_bead_events collection linked back to its bead via event_of.
//
// When a GraphQL transport is configured (test code wires one in via the
// AxonBackend.GraphQLTransport field), AxonBackend uses the typed GraphQL
// client in cli/internal/bead/axon/ as the primary path. The fallback keeps
// the existing in-process JSONL emulation for callers that have not wired a
// transport yet.
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

// ReadAll loads bead rows and event entities from Axon when a GraphQL
// transport is configured, then re-merges events back into each bead's
// Extra["events"]. The legacy on-disk snapshot fallback keeps the old
// compatibility path for callers that have not wired a transport yet.
// Orphan events (event_of pointing at a missing bead, e.g. after a
// partial-write crash) are silently dropped.
func (a *AxonBackend) ReadAll() ([]Bead, error) {
	if transport := a.graphQLTransport(); transport != nil {
		return a.readGraphQLCorpus()
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
// On the GraphQL path it reconciles remote beads, links, and events through
// the typed client; the fallback path still rewrites the legacy snapshot
// files in temp+rename style for compatibility with existing offline tests.
func (a *AxonBackend) WriteAll(beads []Bead) error {
	if transport := a.graphQLTransport(); transport != nil {
		return a.writeGraphQLCorpus(beads)
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

func (a *AxonBackend) graphQLClient() *axon.Client {
	transport := a.graphQLTransport()
	if transport == nil {
		return nil
	}
	return axon.NewClient(transport)
}

func (a *AxonBackend) readGraphQLCorpusState() ([]axon.Bead, []axon.BeadEvent, error) {
	client := a.graphQLClient()
	if client == nil {
		return nil, nil, fmt.Errorf("bead: graphQL corpus requires transport")
	}
	beads, err := client.ListBeads(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("bead: axon read beads: %w", err)
	}
	events, err := client.ListBeadEvents(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("bead: axon read events: %w", err)
	}
	sort.SliceStable(beads, func(i, j int) bool { return beads[i].ID < beads[j].ID })
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].EventOf != events[j].EventOf {
			return events[i].EventOf < events[j].EventOf
		}
		if events[i].Index != events[j].Index {
			return events[i].Index < events[j].Index
		}
		if events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].Kind < events[j].Kind
		}
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return beads, events, nil
}

func (a *AxonBackend) readGraphQLCorpus() ([]Bead, error) {
	remoteBeads, remoteEvents, err := a.readGraphQLCorpusState()
	if err != nil {
		return nil, err
	}
	beads := make([]Bead, 0, len(remoteBeads))
	byID := make(map[string]*Bead, len(remoteBeads))
	for _, row := range remoteBeads {
		local := axonBeadToLocal(row)
		if local.Extra != nil {
			delete(local.Extra, EventsAttachmentExtraKey)
		}
		beads = append(beads, local)
		byID[local.ID] = &beads[len(beads)-1]
	}
	grouped := make(map[string][]BeadEvent, len(remoteEvents))
	for _, ev := range remoteEvents {
		if _, ok := byID[ev.EventOf]; !ok {
			continue
		}
		grouped[ev.EventOf] = append(grouped[ev.EventOf], axonEventToLocal(ev))
	}
	for id, events := range grouped {
		b := byID[id]
		fresh := cloneStringAnyMap(b.Extra)
		if fresh == nil {
			fresh = make(map[string]any, 1)
		}
		delete(fresh, EventsAttachmentExtraKey)
		fresh["events"] = encodeEventsForExtra(events)
		b.Extra = fresh
	}
	return beads, nil
}

func (a *AxonBackend) writeGraphQLCorpus(beads []Bead) error {
	client := a.graphQLClient()
	if client == nil {
		return fmt.Errorf("bead: graphQL corpus requires transport")
	}
	remoteBeads, remoteEvents, err := a.readGraphQLCorpusState()
	if err != nil {
		return err
	}
	remoteByID := make(map[string]axon.Bead, len(remoteBeads))
	for _, row := range remoteBeads {
		remoteByID[row.ID] = row
	}
	currentEvents := make(map[string][]axon.BeadEvent, len(remoteEvents))
	for _, ev := range remoteEvents {
		if ev.EventOf == "" {
			continue
		}
		currentEvents[ev.EventOf] = append(currentEvents[ev.EventOf], ev)
	}

	sort.Slice(beads, func(i, j int) bool { return beads[i].ID < beads[j].ID })
	for _, local := range beads {
		desired := axonBeadInputFromLocal(beadWithoutInlineEvents(local))
		desired.Dependencies = []axon.DependencyInput{}
		if desired.Extra != nil {
			delete(desired.Extra, EventsAttachmentExtraKey)
		}
		current, exists := remoteByID[local.ID]
		if exists {
			updated, err := client.UpdateBead(context.Background(), local.ID, current.Version, desired)
			if err != nil {
				return fmt.Errorf("bead: axon update %s: %w", local.ID, err)
			}
			if updated != nil {
				remoteByID[local.ID] = *updated
			}
		} else {
			created, err := client.CreateBead(context.Background(), desired)
			if err != nil {
				return fmt.Errorf("bead: axon create %s: %w", local.ID, err)
			}
			if created != nil {
				remoteByID[local.ID] = *created
			} else {
				remoteByID[local.ID] = axonBeadFromLocal(local)
			}
		}

		currentDeps := dependencySet(axonBeadToLocal(remoteByID[local.ID]).Dependencies)
		desiredDeps := dependencySet(local.Dependencies)
		for depID := range currentDeps {
			if _, ok := desiredDeps[depID]; ok {
				continue
			}
			if err := client.DeleteDependencyLink(context.Background(), local.ID, depID); err != nil {
				return fmt.Errorf("bead: axon dep remove %s -> %s: %w", local.ID, depID, err)
			}
		}
		for depID := range desiredDeps {
			if _, ok := currentDeps[depID]; ok {
				continue
			}
			if err := client.CreateDependencyLink(context.Background(), local.ID, depID); err != nil {
				return fmt.Errorf("bead: axon dep add %s -> %s: %w", local.ID, depID, err)
			}
		}

		desiredEvents := localEventsFromBead(local)
		existingEvents := currentEvents[local.ID]
		if len(desiredEvents) < len(existingEvents) {
			continue
		}
		for i := len(existingEvents); i < len(desiredEvents); i++ {
			row := axonEventFromLocal(local.ID, i, desiredEvents[i])
			if _, err := client.CreateBeadEvent(context.Background(), row); err != nil {
				return fmt.Errorf("bead: axon append event %s/%d: %w", local.ID, i, err)
			}
		}
	}
	return nil
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
