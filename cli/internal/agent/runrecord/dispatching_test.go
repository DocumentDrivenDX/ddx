package runrecord

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestRunRecordInitialCorrelationFields verifies the initial dispatching
// record contains version, run ID, bead ID, attempt ID, worker ID, prompt
// evidence pointer, base revision, timestamps, and correlation metadata (AC1).
func TestRunRecordInitialCorrelationFields(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 26, 8, 3, 57, 0, time.UTC)
	const (
		runID      = "run_20260726T080357_corr"
		beadID     = "ddx-aa1de121"
		attemptID  = "20260726T080357-63484665"
		workerID   = "worker-host-1"
		baseRev    = "9175d61a197eadfcacc3b3d0d97ebac1cdc47306"
		promptPath = ".ddx/executions/20260726T080357-63484665/prompt.md"
		sessionID  = "eb-unit-session"
		bundlePath = ".ddx/executions/20260726T080357-63484665"
		promptSHA  = "deadbeefcafebabe"
	)

	rec := NewDispatchingRecord(DispatchingParams{
		RunID:      runID,
		BeadID:     beadID,
		AttemptID:  attemptID,
		WorkerID:   workerID,
		BaseRev:    baseRev,
		PromptPath: promptPath,
		Correlation: map[string]string{
			"session_id":  sessionID,
			"bundle_path": bundlePath,
			"prompt_sha":  promptSHA,
		},
		Now: fixed,
	})

	if rec.Version != SchemaVersion {
		t.Errorf("version=%d, want %d", rec.Version, SchemaVersion)
	}
	if rec.RunID != runID {
		t.Errorf("run_id=%q, want %q", rec.RunID, runID)
	}
	if rec.BeadID != beadID {
		t.Errorf("bead_id=%q, want %q", rec.BeadID, beadID)
	}
	if rec.AttemptID != attemptID {
		t.Errorf("attempt_id=%q, want %q", rec.AttemptID, attemptID)
	}
	if rec.WorkerID != workerID {
		t.Errorf("worker_id=%q, want %q", rec.WorkerID, workerID)
	}
	if rec.BaseRev != baseRev {
		t.Errorf("base_rev=%q, want %q", rec.BaseRev, baseRev)
	}
	if rec.Phase != PhaseDispatching {
		t.Errorf("phase=%q, want %q", rec.Phase, PhaseDispatching)
	}
	if rec.CreatedAt.IsZero() || rec.UpdatedAt.IsZero() {
		t.Fatalf("timestamps must be set: created_at=%v updated_at=%v", rec.CreatedAt, rec.UpdatedAt)
	}
	if rec.Correlation == nil {
		t.Fatal("correlation metadata is nil")
	}
	if rec.Correlation["session_id"] != sessionID {
		t.Errorf("correlation.session_id=%q, want %q", rec.Correlation["session_id"], sessionID)
	}
	if rec.Correlation["bundle_path"] != bundlePath {
		t.Errorf("correlation.bundle_path=%q, want %q", rec.Correlation["bundle_path"], bundlePath)
	}
	if rec.Correlation["prompt_sha"] != promptSHA {
		t.Errorf("correlation.prompt_sha=%q, want %q", rec.Correlation["prompt_sha"], promptSHA)
	}

	prompt := evidenceByName(rec.Evidence, "prompt")
	if prompt == nil {
		t.Fatal("prompt evidence pointer missing")
	}
	if prompt.Path != promptPath {
		t.Errorf("prompt path=%q, want %q", prompt.Path, promptPath)
	}

	// Stable JSON field names for later CLI/server readers.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	for _, key := range []string{
		"version", "run_id", "bead_id", "attempt_id", "worker_id", "base_rev",
		"created_at", "updated_at", "correlation", "evidence", "phase",
	} {
		if _, ok := asMap[key]; !ok {
			t.Errorf("marshaled record missing stable field %q; keys=%v", key, mapKeys(asMap))
		}
	}

	// Round-trip preserves first-class correlation.
	var out Record
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if out.WorkerID != workerID || out.BaseRev != baseRev || out.AttemptID != attemptID {
		t.Errorf("round-trip correlation: worker=%q base=%q attempt=%q", out.WorkerID, out.BaseRev, out.AttemptID)
	}
	if out.Correlation["session_id"] != sessionID {
		t.Errorf("round-trip correlation.session_id=%q", out.Correlation["session_id"])
	}
	if out.Fizeau != nil {
		t.Errorf("initial record must not carry Fizeau public fields, got %+v", out.Fizeau)
	}
}

// TestRunRecordInitialTimestampsAreStable verifies created_at and updated_at
// are populated from the caller-supplied clock and are equal for the initial
// dispatching record (AC2).
func TestRunRecordInitialTimestampsAreStable(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 7, 26, 12, 0, 0, 123456789, time.UTC)

	rec := NewDispatchingRecord(DispatchingParams{
		RunID:     "run_clock_pin",
		AttemptID: "20260726T120000-clock",
		BeadID:    "ddx-aa1de121",
		Now:       fixed,
	})

	if !rec.CreatedAt.Equal(fixed.UTC()) {
		t.Errorf("created_at=%v, want %v (caller-supplied clock)", rec.CreatedAt, fixed.UTC())
	}
	if !rec.UpdatedAt.Equal(fixed.UTC()) {
		t.Errorf("updated_at=%v, want %v (caller-supplied clock)", rec.UpdatedAt, fixed.UTC())
	}
	if !rec.CreatedAt.Equal(rec.UpdatedAt) {
		t.Errorf("initial created_at (%v) != updated_at (%v)", rec.CreatedAt, rec.UpdatedAt)
	}
	// StartedAt remains the lifecycle first-publish stamp and matches the same clock.
	if !rec.StartedAt.Equal(fixed.UTC()) {
		t.Errorf("started_at=%v, want %v", rec.StartedAt, fixed.UTC())
	}

	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	created, okC := asMap["created_at"].(string)
	updated, okU := asMap["updated_at"].(string)
	if !okC || !okU {
		t.Fatalf("created_at/updated_at JSON types: created=%T updated=%T", asMap["created_at"], asMap["updated_at"])
	}
	if created != updated {
		t.Errorf("JSON created_at=%q != updated_at=%q", created, updated)
	}
	// Fixed nanosecond component must survive encoding (RFC3339Nano).
	if !strings.Contains(created, "2026-07-26T12:00:00") {
		t.Errorf("created_at JSON unexpected: %q", created)
	}

	// A second construction with a different clock must not share the first instant.
	later := fixed.Add(5 * time.Second)
	rec2 := NewDispatchingRecord(DispatchingParams{
		AttemptID: "20260726T120005-clock2",
		Now:       later,
	})
	if rec2.CreatedAt.Equal(rec.CreatedAt) {
		t.Error("second record used the first clock value")
	}
	if !rec2.CreatedAt.Equal(later.UTC()) || !rec2.UpdatedAt.Equal(later.UTC()) {
		t.Errorf("second record clock: created=%v updated=%v want %v", rec2.CreatedAt, rec2.UpdatedAt, later.UTC())
	}
}

// TestRunRecordInitialPromptEvidencePointer verifies the prompt path is
// persisted as a DDx-owned evidence pointer, not as provider transcript
// content (AC3).
func TestRunRecordInitialPromptEvidencePointer(t *testing.T) {
	t.Parallel()

	const promptPath = ".ddx/executions/20260726T080357-63484665/prompt.md"

	rec := NewDispatchingRecord(DispatchingParams{
		RunID:      "run_prompt_pointer",
		AttemptID:  "20260726T080357-prompt",
		BeadID:     "ddx-aa1de121",
		PromptPath: promptPath,
		// Correlation may name the relative prompt path, but never transcript body.
		Correlation: map[string]string{
			"prompt_file": promptPath,
			"prompt_sha":  "abc123sha",
		},
		Now: time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC),
	})

	prompt := evidenceByName(rec.Evidence, "prompt")
	if prompt == nil {
		t.Fatal("expected EvidenceLink name=prompt")
	}
	if prompt.Path != promptPath {
		t.Errorf("prompt evidence path=%q, want %q", prompt.Path, promptPath)
	}
	if prompt.MediaType != "text/markdown" {
		t.Errorf("prompt media_type=%q, want text/markdown", prompt.MediaType)
	}

	// Evidence path is a pointer; the struct must not grow transcript-body fields.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rawStr := string(raw)
	for _, forbidden := range []string{
		`"transcript"`,
		`"provider_transcript"`,
		`"raw_output"`,
		`"prompt_body"`,
		`"prompt_content"`,
		`"stdout"`,
		`"stderr"`,
	} {
		if strings.Contains(rawStr, forbidden) {
			t.Errorf("record encodes forbidden provider/transcript field %s: %s", forbidden, rawStr)
		}
	}

	// The path string itself is present as the pointer value, not as free-form body.
	if !strings.Contains(rawStr, promptPath) {
		t.Errorf("marshaled record missing prompt path pointer %q", promptPath)
	}

	// Round-trip evidence is still a path pointer only.
	var out Record
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := evidenceByName(out.Evidence, "prompt")
	if got == nil || got.Path != promptPath {
		t.Fatalf("round-trip prompt evidence=%+v, want path %q", got, promptPath)
	}
	// No accidental body field via correlation: values are metadata, not transcript.
	if body, ok := out.Correlation["prompt_body"]; ok {
		t.Errorf("correlation must not carry prompt_body, got %q", body)
	}
	if out.Correlation["prompt_file"] != promptPath {
		t.Errorf("correlation.prompt_file=%q, want path pointer %q", out.Correlation["prompt_file"], promptPath)
	}
}

func evidenceByName(links []EvidenceLink, name string) *EvidenceLink {
	for i := range links {
		if links[i].Name == name {
			return &links[i]
		}
	}
	return nil
}

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
