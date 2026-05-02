// Package federation defines the hub-side federation registry: state types,
// persistence, and reconciliation helpers. No HTTP layer here — that lands in
// later beads. The on-disk format carries a schema_version field for forward
// compatibility (see ADR-007 and FEAT-026).
package federation

import (
	"fmt"
	"time"
)

// CurrentSchemaVersion is the schema_version this build writes. Reading code
// must accept older versions and migrate; writing code always emits this.
const CurrentSchemaVersion = "1"

// SpokeStatus is the lifecycle/health status of a registered spoke.
//
// The states are:
//   - StatusRegistered: spoke has registered but no heartbeat has been observed yet.
//   - StatusActive:     last heartbeat is recent (the live/healthy state).
//   - StatusStale:      no heartbeat for ≥ 2 minutes.
//   - StatusOffline:    fan-out to the spoke failed (connection refused, unreachable).
//   - StatusDegraded:   version handshake accepted with minor/schema drift.
//
// The bead-level enum collapses ADR-007's split between compat_status and
// liveness into a single field for v1 persistence; richer surface fields can
// be added in later schema versions without breaking older readers.
type SpokeStatus string

const (
	StatusRegistered SpokeStatus = "registered"
	StatusActive     SpokeStatus = "active"
	StatusStale      SpokeStatus = "stale"
	StatusOffline    SpokeStatus = "offline"
	StatusDegraded   SpokeStatus = "degraded"
)

// Valid reports whether s is one of the known status values.
func (s SpokeStatus) Valid() bool {
	switch s {
	case StatusRegistered, StatusActive, StatusStale, StatusOffline, StatusDegraded:
		return true
	}
	return false
}

// SpokeRecord is one entry in the federation registry.
type SpokeRecord struct {
	NodeID        string      `json:"node_id"`
	Name          string      `json:"name"`
	URL           string      `json:"url"`
	DDxVersion    string      `json:"ddx_version,omitempty"`
	SchemaVersion string      `json:"schema_version,omitempty"`
	Capabilities  []string    `json:"capabilities,omitempty"`
	RegisteredAt  time.Time   `json:"registered_at"`
	LastHeartbeat time.Time   `json:"last_heartbeat"`
	Status        SpokeStatus `json:"status"`
}

// FederationRegistry is the persisted hub state.
type FederationRegistry struct {
	SchemaVersion string        `json:"schema_version"`
	Spokes        []SpokeRecord `json:"spokes"`
}

// NewRegistry returns an empty registry stamped with the current schema version.
func NewRegistry() *FederationRegistry {
	return &FederationRegistry{
		SchemaVersion: CurrentSchemaVersion,
		Spokes:        []SpokeRecord{},
	}
}

// FindSpoke returns the record for nodeID, or nil if absent.
func (r *FederationRegistry) FindSpoke(nodeID string) *SpokeRecord {
	for i := range r.Spokes {
		if r.Spokes[i].NodeID == nodeID {
			return &r.Spokes[i]
		}
	}
	return nil
}

// UpsertSpoke inserts or replaces a spoke by NodeID. NodeID must be non-empty.
func (r *FederationRegistry) UpsertSpoke(rec SpokeRecord) error {
	if rec.NodeID == "" {
		return fmt.Errorf("federation: spoke node_id is required")
	}
	if rec.Status == "" {
		rec.Status = StatusRegistered
	}
	if !rec.Status.Valid() {
		return fmt.Errorf("federation: invalid spoke status %q", rec.Status)
	}
	for i := range r.Spokes {
		if r.Spokes[i].NodeID == rec.NodeID {
			r.Spokes[i] = rec
			return nil
		}
	}
	r.Spokes = append(r.Spokes, rec)
	return nil
}

// RemoveSpoke removes a spoke by NodeID; reports whether it was present.
func (r *FederationRegistry) RemoveSpoke(nodeID string) bool {
	for i := range r.Spokes {
		if r.Spokes[i].NodeID == nodeID {
			r.Spokes = append(r.Spokes[:i], r.Spokes[i+1:]...)
			return true
		}
	}
	return false
}

// SetStatus updates the status of a spoke and returns true on success. Refuses
// unknown status values so callers cannot silently corrupt persisted state.
func (r *FederationRegistry) SetStatus(nodeID string, status SpokeStatus) error {
	if !status.Valid() {
		return fmt.Errorf("federation: invalid status %q", status)
	}
	rec := r.FindSpoke(nodeID)
	if rec == nil {
		return fmt.Errorf("federation: spoke %q not found", nodeID)
	}
	rec.Status = status
	return nil
}

// ReconcileLiveness derives status purely from elapsed time since the last
// heartbeat for spokes whose status is currently in {registered, active,
// stale}. Statuses set externally (offline from fan-out failure, degraded from
// version handshake) are left alone — those are not time-driven.
//
//   - now - last_heartbeat <  staleAfter            → active
//   - now - last_heartbeat >= staleAfter            → stale
//
// A zero LastHeartbeat keeps a freshly-registered spoke in StatusRegistered
// until the first heartbeat is observed.
func (r *FederationRegistry) ReconcileLiveness(now time.Time, staleAfter time.Duration) {
	for i := range r.Spokes {
		s := &r.Spokes[i]
		switch s.Status {
		case StatusOffline, StatusDegraded:
			continue
		}
		if s.LastHeartbeat.IsZero() {
			s.Status = StatusRegistered
			continue
		}
		if now.Sub(s.LastHeartbeat) >= staleAfter {
			s.Status = StatusStale
		} else {
			s.Status = StatusActive
		}
	}
}
