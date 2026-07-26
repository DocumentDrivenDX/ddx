// Package runrecord defines the versioned v1 durable run-record schema for
// .ddx/runs/<run-id>/record.json (Phase 3 WB-2 run substrate).
//
// The record is DDx orchestration truth for one attempt: versioned identity,
// lifecycle phase, timestamps, typed outcome, optional opaque Fizeau public
// result references, and evidence links. It deliberately excludes provider
// process internals (raw output, PID, process-tree metadata, provider-session
// canonical state) and concrete harness-routing policy.
package runrecord

import "time"

// SchemaVersion is the current on-disk schema version for Record.
const SchemaVersion = 1

// RecordFileName is the basename of the durable run record under a run dir.
const RecordFileName = "record.json"

// StoreDir is the project-relative root for durable run records
// (.ddx/runs/<run-id>/record.json).
const StoreDir = ".ddx/runs"

// LifecyclePhase is the DDx-owned orchestration phase of one attempt record.
// Values match Phase 3 WB-2: dispatching | running | terminal | interrupted.
type LifecyclePhase string

const (
	// PhaseDispatching is published before Fizeau Execute is called.
	PhaseDispatching LifecyclePhase = "dispatching"
	// PhaseRunning means a public Fizeau execution stream exists for the attempt.
	PhaseRunning LifecyclePhase = "running"
	// PhaseTerminal is set after a typed immediate error or public final event
	// and DDx repository evaluation complete.
	PhaseTerminal LifecyclePhase = "terminal"
	// PhaseInterrupted is set only with DDx worker-death proof (recovery path).
	PhaseInterrupted LifecyclePhase = "interrupted"
)

// Record is the v1 on-disk shape for .ddx/runs/<run-id>/record.json.
//
// Fields are limited to durable DDx correlation and public orchestration
// outcome. Harness/provider/model pins, route decisions, provider PIDs,
// process trees, raw provider streams, and provider-session canonical state
// are intentionally absent.
type Record struct {
	// Version is the schema version; always SchemaVersion for writers of v1.
	Version int `json:"version"`
	// RunID is the stable identifier for this record (directory name under StoreDir).
	RunID string `json:"run_id"`
	// BeadID correlates the record to a tracker bead when the attempt is bead-scoped.
	BeadID string `json:"bead_id,omitempty"`
	// AttemptID is the DDx attempt / execution id for this run.
	AttemptID string `json:"attempt_id,omitempty"`
	// Phase is the typed lifecycle phase of the record.
	Phase LifecyclePhase `json:"phase"`

	// StartedAt is when the record was first published (typically dispatching).
	StartedAt time.Time `json:"started_at"`
	// UpdatedAt is the last atomic publish time for this record.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// FinishedAt is set when the record enters a terminal or interrupted phase.
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Outcome holds typed DDx-owned outcome fields once known.
	Outcome *Outcome `json:"outcome,omitempty"`
	// Fizeau holds optional opaque public Fizeau result fields (session/result
	// refs, typed final status). Never raw provider streams or process metadata.
	Fizeau *FizeauPublicResult `json:"fizeau,omitempty"`
	// Evidence lists pointers to evidence artifacts for this run.
	Evidence []EvidenceLink `json:"evidence,omitempty"`
}

// Outcome is the typed DDx-owned outcome of the run (repository evaluation and
// orchestration disposition), distinct from opaque Fizeau public fields.
type Outcome struct {
	// Status is a short typed disposition (e.g. success, failure, interrupted).
	Status string `json:"status,omitempty"`
	// Reason is an optional machine-readable or short human reason string.
	Reason string `json:"reason,omitempty"`
	// EvidenceVerdict is the DDx repository-evidence evaluation summary.
	EvidenceVerdict string `json:"evidence_verdict,omitempty"`
}

// FizeauPublicResult holds optional opaque Fizeau public-result fields only.
// SessionLogPath and public session/result references are pointers into Fizeau
// durable outputs; they are not scraped provider streams or process state.
type FizeauPublicResult struct {
	// SessionLogPath is the opaque Fizeau public session log path when known.
	SessionLogPath string `json:"session_log_path,omitempty"`
	// PublicSessionRef is an opaque public session reference (not provider PID/state).
	PublicSessionRef string `json:"public_session_ref,omitempty"`
	// PublicResultRef is an opaque public final-result reference.
	PublicResultRef string `json:"public_result_ref,omitempty"`
	// ImmediateError is a typed immediate-error string from the public contract.
	ImmediateError string `json:"immediate_error,omitempty"`
	// FinalStatus is the public final status string when a final event exists.
	FinalStatus string `json:"final_status,omitempty"`
	// FinalExitCode is the public final exit code when reported.
	FinalExitCode *int `json:"final_exit_code,omitempty"`
	// DurationMS is the public final duration when reported.
	DurationMS *int64 `json:"duration_ms,omitempty"`
	// CostUSD is the public final cost when reported (nil = unknown).
	CostUSD *float64 `json:"cost_usd,omitempty"`
	// InputTokens is the public final input token count when reported.
	InputTokens *int `json:"input_tokens,omitempty"`
	// OutputTokens is the public final output token count when reported.
	OutputTokens *int `json:"output_tokens,omitempty"`
	// TotalTokens is the public final total token count when reported.
	TotalTokens *int `json:"total_tokens,omitempty"`
	// CachedTokens is the public final cache-read token count when reported.
	CachedTokens *int `json:"cached_tokens,omitempty"`
}

// EvidenceLink points at an evidence artifact. Path is relative to the run
// directory (or another stable project-relative path the reader understands).
type EvidenceLink struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
}
