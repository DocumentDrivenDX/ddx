package bead

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackendAxon selects the axon-backed RawBackend. Per TD-030 D3 the on-disk
// shape splits beads from their events: one row per bead in the
// ddx_beads collection, plus one entity per event in the
// ddx_bead_events collection linked back to its bead via event_of.
//
// The implementation in axon_backend.go is an in-process emulation of the
// axon GraphQL surface. It serializes the two collections to JSONL files
// under <dir>/axon/ and acquires a single directory lock around mutations.
// The wire-protocol path (genqlient against a real axon server, per TD-030
// §"Wire transport") is the GA delivery; this RawBackend keeps the same
// two-collection split and OCC-compatible bead row schema so the wire path
// is a contained refactor.
//
// AC §3 ("Behind a feature flag until ddx-3ec0chaos signs off"): NewStore
// only routes to AxonBackend when the operator opts in via
// DDX_AXON_EXPERIMENTAL=1. With the flag unset, beads.backend=axon falls
// through to the JSONL default and a stderr warning is emitted so the
// misconfiguration is visible without breaking the workspace.
const BackendAxon = "axon"

// AxonExperimentalEnv gates whether the axon backend may be selected. The
// flag is intentionally separate from beads.backend so the chaos-test bead
// (ddx-743bc194) can flip operators to axon machine-by-machine without
// editing every project's config.
const AxonExperimentalEnv = "DDX_AXON_EXPERIMENTAL"

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
	Dir        string
	BeadsFile  string
	EventsFile string
	LockDir    string
	LockWait   time.Duration
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

// AxonExperimentalEnabled reports whether the operator has opted in to the
// experimental axon backend via DDX_AXON_EXPERIMENTAL=1. NewStore consults
// this before routing beads.backend=axon to AxonBackend.
func AxonExperimentalEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(AxonExperimentalEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// ReadAll loads bead rows and event entities from disk and re-merges events
// back into each bead's Extra["events"]. Orphan events (event_of pointing at
// a missing bead, e.g. after a partial-write crash) are silently dropped.
func (a *AxonBackend) ReadAll() ([]Bead, error) {
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
	// Group events by bead id, preserving insertion order on disk.
	grouped := make(map[string][]BeadEvent, len(events))
	order := make(map[string]int, len(events))
	for _, ev := range events {
		if _, ok := byID[ev.beadID]; !ok {
			continue // orphan
		}
		grouped[ev.beadID] = append(grouped[ev.beadID], ev.event)
		if _, seen := order[ev.beadID]; !seen {
			order[ev.beadID] = len(order)
		}
	}
	for id, evs := range grouped {
		b := byID[id]
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		// Skip beads whose events have been externalised — the attachment file
		// is the canonical source post-close and inlining stale events would
		// duplicate the history.
		if hasEventsAttachment(b) {
			continue
		}
		b.Extra["events"] = encodeEventsForExtra(evs)
	}
	return beads, nil
}

// WriteAll splits inline events from each bead and persists both collections.
// Writes are serialised at the directory-lock layer (WithLock); this method
// rewrites both files in temp+rename style so a crash mid-write leaves the
// previous snapshot intact rather than producing torn rows.
func (a *AxonBackend) WriteAll(beads []Bead) error {
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
	data, err := marshalBead(b)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage = data
	envelope := axonEntityEnvelope{
		Collection:    AxonBeadsCollection,
		SchemaVersion: axonSchemaVersion,
		Data:          raw,
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
	Collection    string          `json:"_collection"`
	SchemaVersion int             `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
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
	var out []Bead
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env axonEntityEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, fmt.Errorf("bead: axon parse bead row: %w", err)
		}
		if env.Collection != AxonBeadsCollection || len(env.Data) == 0 {
			continue
		}
		b, err := unmarshalBead(env.Data)
		if err != nil {
			return nil, fmt.Errorf("bead: axon parse bead data: %w", err)
		}
		out = append(out, b)
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
	var out []axonEventRecord
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
