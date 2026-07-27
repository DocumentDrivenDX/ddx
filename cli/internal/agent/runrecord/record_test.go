package runrecord

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestRunRecordCoreRoundTrip marshals a fully populated v1 core record
// (schema version, run/bead/attempt IDs, lifecycle phase, timestamps, typed
// outcome) and asserts equality of every core field after unmarshal. It also
// asserts the serialized record encodes no concrete harness-routing policy
// (no harness, model, or route-reason field).
func TestRunRecordCoreRoundTrip(t *testing.T) {
	t.Parallel()

	finished := time.Date(2026, 7, 27, 7, 48, 19, 0, time.UTC)
	started := time.Date(2026, 7, 27, 7, 48, 0, 0, time.UTC)
	updated := finished

	in := Record{
		Version:    SchemaVersion,
		RunID:      "run_20260727T074800Z_core",
		BeadID:     "ddx-f2cfe3d3",
		AttemptID:  "20260727T074819-7485b068",
		Phase:      PhaseTerminal,
		CreatedAt:  started,
		StartedAt:  started,
		UpdatedAt:  updated,
		FinishedAt: &finished,
		Outcome: &Outcome{
			Status:          "success",
			Reason:          "core_fields_round_trip",
			EvidenceVerdict: "gates_green",
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Core surface must not encode concrete harness-routing policy.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	for _, forbidden := range []string{"harness", "model", "route_reason", "route-reason"} {
		if _, ok := asMap[forbidden]; ok {
			t.Errorf("marshaled core record encodes routing policy field %q", forbidden)
		}
	}

	var out Record
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Version != SchemaVersion {
		t.Errorf("version=%d, want %d (schema version)", out.Version, SchemaVersion)
	}
	if out.Version != in.Version {
		t.Errorf("version round-trip: got %d want %d", out.Version, in.Version)
	}
	if out.RunID != in.RunID {
		t.Errorf("run_id=%q, want %q", out.RunID, in.RunID)
	}
	if out.BeadID != in.BeadID {
		t.Errorf("bead_id=%q, want %q", out.BeadID, in.BeadID)
	}
	if out.AttemptID != in.AttemptID {
		t.Errorf("attempt_id=%q, want %q", out.AttemptID, in.AttemptID)
	}
	if out.Phase != in.Phase {
		t.Errorf("phase=%q, want %q", out.Phase, in.Phase)
	}
	// Phase must be a LifecyclePhase constant, not an arbitrary free-form value.
	if out.Phase != PhaseTerminal {
		t.Errorf("phase constant: got %q want %q", out.Phase, PhaseTerminal)
	}
	if !out.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("created_at=%v, want %v", out.CreatedAt, in.CreatedAt)
	}
	if !out.StartedAt.Equal(in.StartedAt) {
		t.Errorf("started_at=%v, want %v", out.StartedAt, in.StartedAt)
	}
	if !out.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("updated_at=%v, want %v", out.UpdatedAt, in.UpdatedAt)
	}
	if out.FinishedAt == nil || !out.FinishedAt.Equal(*in.FinishedAt) {
		t.Errorf("finished_at=%v, want %v", out.FinishedAt, in.FinishedAt)
	}
	if out.Outcome == nil {
		t.Fatal("outcome is nil after round-trip")
	}
	if out.Outcome.Status != in.Outcome.Status {
		t.Errorf("outcome.status=%q, want %q", out.Outcome.Status, in.Outcome.Status)
	}
	if out.Outcome.Reason != in.Outcome.Reason {
		t.Errorf("outcome.reason=%q, want %q", out.Outcome.Reason, in.Outcome.Reason)
	}
	if out.Outcome.EvidenceVerdict != in.Outcome.EvidenceVerdict {
		t.Errorf("outcome.evidence_verdict=%q, want %q", out.Outcome.EvidenceVerdict, in.Outcome.EvidenceVerdict)
	}

	// Lifecycle phase is a named type with declared exported constants.
	for _, phase := range []LifecyclePhase{PhaseDispatching, PhaseRunning, PhaseTerminal, PhaseInterrupted} {
		rec := Record{
			Version:   SchemaVersion,
			RunID:     "run_phase_probe",
			BeadID:    "ddx-f2cfe3d3",
			AttemptID: "attempt-phase",
			Phase:     phase,
			StartedAt: started,
			UpdatedAt: started,
		}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal phase %q: %v", phase, err)
		}
		var got Record
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal phase %q: %v", phase, err)
		}
		if got.Phase != phase {
			t.Errorf("phase round-trip: got %q want %q", got.Phase, phase)
		}
	}
}

// TestRunRecordSchemaRoundTrip covers version, run ID, bead ID, attempt ID,
// lifecycle phase, timestamps, typed outcome fields, optional public Fizeau
// result fields, and evidence links without encoding concrete harness-routing
// policy (AC1).
func TestRunRecordSchemaRoundTrip(t *testing.T) {
	t.Parallel()

	finished := time.Date(2026, 7, 25, 12, 5, 0, 0, time.UTC)
	exitCode := 0
	durationMS := int64(4200)

	in := Record{
		Version:    SchemaVersion,
		RunID:      "run_20260725T120000Z_abc",
		BeadID:     "ddx-5d431e3e",
		AttemptID:  "20260725T120249-ade5a024",
		Phase:      PhaseTerminal,
		StartedAt:  time.Date(2026, 7, 25, 12, 2, 49, 0, time.UTC),
		UpdatedAt:  finished,
		FinishedAt: &finished,
		Outcome: &Outcome{
			Status:          "success",
			Reason:          "acceptance_met",
			EvidenceVerdict: "gates_green",
		},
		Fizeau: &FizeauPublicResult{
			SessionLogPath:   "/var/ddx/sessions/svc-abc.jsonl",
			PublicSessionRef: "svc-abc",
			PublicResultRef:  "result-svc-abc",
			ImmediateError:   "",
			FinalStatus:      "completed",
			FinalExitCode:    &exitCode,
			DurationMS:       &durationMS,
		},
		Evidence: []EvidenceLink{
			{Name: "prompt", Path: "prompt.md", MediaType: "text/markdown"},
			{Name: "result", Path: "result.json", MediaType: "application/json"},
			{Name: "checks", Path: "evidence/checks.json", MediaType: "application/json"},
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Schema must not encode concrete harness-routing policy.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	for _, forbidden := range []string{
		"harness", "provider", "model", "route_reason", "routing_policy",
		"power_min", "power_max", "min_power", "max_power", "profile",
	} {
		if _, ok := asMap[forbidden]; ok {
			t.Errorf("marshaled record encodes routing policy field %q", forbidden)
		}
	}

	var out Record
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Version != SchemaVersion {
		t.Errorf("version=%d, want %d", out.Version, SchemaVersion)
	}
	if out.RunID != in.RunID {
		t.Errorf("run_id=%q, want %q", out.RunID, in.RunID)
	}
	if out.BeadID != in.BeadID {
		t.Errorf("bead_id=%q, want %q", out.BeadID, in.BeadID)
	}
	if out.AttemptID != in.AttemptID {
		t.Errorf("attempt_id=%q, want %q", out.AttemptID, in.AttemptID)
	}
	if out.Phase != PhaseTerminal {
		t.Errorf("phase=%q, want %q", out.Phase, PhaseTerminal)
	}
	if !out.StartedAt.Equal(in.StartedAt) {
		t.Errorf("started_at=%v, want %v", out.StartedAt, in.StartedAt)
	}
	if !out.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("updated_at=%v, want %v", out.UpdatedAt, in.UpdatedAt)
	}
	if out.FinishedAt == nil || !out.FinishedAt.Equal(finished) {
		t.Errorf("finished_at=%v, want %v", out.FinishedAt, finished)
	}
	if out.Outcome == nil {
		t.Fatal("outcome is nil after round-trip")
	}
	if out.Outcome.Status != "success" || out.Outcome.Reason != "acceptance_met" ||
		out.Outcome.EvidenceVerdict != "gates_green" {
		t.Errorf("outcome=%+v, want status=success reason=acceptance_met evidence_verdict=gates_green", out.Outcome)
	}
	if out.Fizeau == nil {
		t.Fatal("fizeau is nil after round-trip")
	}
	if out.Fizeau.SessionLogPath != in.Fizeau.SessionLogPath {
		t.Errorf("session_log_path=%q, want %q", out.Fizeau.SessionLogPath, in.Fizeau.SessionLogPath)
	}
	if out.Fizeau.PublicSessionRef != in.Fizeau.PublicSessionRef {
		t.Errorf("public_session_ref=%q, want %q", out.Fizeau.PublicSessionRef, in.Fizeau.PublicSessionRef)
	}
	if out.Fizeau.PublicResultRef != in.Fizeau.PublicResultRef {
		t.Errorf("public_result_ref=%q, want %q", out.Fizeau.PublicResultRef, in.Fizeau.PublicResultRef)
	}
	if out.Fizeau.FinalStatus != "completed" {
		t.Errorf("final_status=%q, want completed", out.Fizeau.FinalStatus)
	}
	if out.Fizeau.FinalExitCode == nil || *out.Fizeau.FinalExitCode != 0 {
		t.Errorf("final_exit_code=%v, want 0", out.Fizeau.FinalExitCode)
	}
	if out.Fizeau.DurationMS == nil || *out.Fizeau.DurationMS != durationMS {
		t.Errorf("duration_ms=%v, want %d", out.Fizeau.DurationMS, durationMS)
	}
	if len(out.Evidence) != 3 {
		t.Fatalf("evidence len=%d, want 3", len(out.Evidence))
	}
	if out.Evidence[0].Name != "prompt" || out.Evidence[0].Path != "prompt.md" {
		t.Errorf("evidence[0]=%+v", out.Evidence[0])
	}
	if out.Evidence[1].MediaType != "application/json" {
		t.Errorf("evidence[1].media_type=%q", out.Evidence[1].MediaType)
	}

	// Lifecycle phase constants are the only allowed phase values in v1 docs.
	for _, phase := range []LifecyclePhase{PhaseDispatching, PhaseRunning, PhaseTerminal, PhaseInterrupted} {
		rec := Record{Version: SchemaVersion, RunID: "r", Phase: phase, StartedAt: in.StartedAt}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal phase %q: %v", phase, err)
		}
		var got Record
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal phase %q: %v", phase, err)
		}
		if got.Phase != phase {
			t.Errorf("phase round-trip: got %q want %q", got.Phase, phase)
		}
	}
}

// TestRunRecordOptionalFizeauFieldsOmitted asserts that a record with unset
// Fizeau public-result fields and no evidence links omits those keys from the
// serialized JSON (omitempty contract for optional opaque refs and evidence).
func TestRunRecordOptionalFizeauFieldsOmitted(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	in := Record{
		Version:   SchemaVersion,
		RunID:     "run_20260727T080000Z_omit",
		BeadID:    "ddx-6d682d7d",
		AttemptID: "20260727T080000-omit",
		Phase:     PhaseDispatching,
		StartedAt: started,
		UpdatedAt: started,
		// Fizeau left nil; Evidence left nil/empty — both must omit.
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}

	// Top-level optional keys must be absent when unset.
	for _, key := range []string{"fizeau", "evidence", "outcome"} {
		if _, ok := asMap[key]; ok {
			t.Errorf("serialized JSON contains optional key %q when unset: %s", key, string(raw))
		}
	}

	// Nested public Fizeau result field names must not appear as top-level keys
	// and must not appear anywhere when the Fizeau object is unset.
	serialized := string(raw)
	for _, key := range []string{
		"public_session_ref",
		"public_result_ref",
		"session_log_path",
		"immediate_error",
		"final_status",
		"final_exit_code",
		"duration_ms",
	} {
		needle := `"` + key + `"`
		if strings.Contains(serialized, needle) {
			t.Errorf("serialized JSON contains unset Fizeau field key %q: %s", key, serialized)
		}
	}

	// Empty (non-nil) evidence slice must also omit.
	in.Evidence = []EvidenceLink{}
	rawEmpty, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal empty evidence: %v", err)
	}
	var emptyMap map[string]json.RawMessage
	if err := json.Unmarshal(rawEmpty, &emptyMap); err != nil {
		t.Fatalf("unmarshal empty evidence map: %v", err)
	}
	if _, ok := emptyMap["evidence"]; ok {
		t.Errorf("empty evidence slice must omit evidence key: %s", string(rawEmpty))
	}
	if _, ok := emptyMap["fizeau"]; ok {
		t.Errorf("unset fizeau must still omit fizeau key: %s", string(rawEmpty))
	}
}

// v1RecordJSONAllowlist is the complete set of JSON field names permitted on
// the v1 run-record surface (Record plus nested Outcome, FizeauPublicResult,
// and EvidenceLink). Any newly added schema field must be listed here
// intentionally; the guard test fails until the allowlist is updated.
//
// Provider process internals (raw output, PID, process-tree metadata,
// provider-session canonical state) must never appear here.
var v1RecordJSONAllowlist = map[string]struct{}{
	// Record
	"version": {}, "run_id": {}, "bead_id": {}, "attempt_id": {},
	"worker_id": {}, "base_rev": {}, "phase": {},
	"created_at": {}, "started_at": {}, "updated_at": {}, "finished_at": {},
	"correlation": {}, "outcome": {}, "fizeau": {}, "evidence": {},
	// Outcome
	"status": {}, "reason": {}, "evidence_verdict": {},
	// FizeauPublicResult (public refs only)
	"session_log_path": {}, "public_session_ref": {}, "public_result_ref": {},
	"immediate_error": {}, "final_status": {}, "final_exit_code": {},
	"duration_ms": {}, "cost_usd": {},
	"input_tokens": {}, "output_tokens": {}, "total_tokens": {}, "cached_tokens": {},
	// EvidenceLink
	"name": {}, "path": {}, "media_type": {},
}

// providerProcessForbiddenJSONTags are field names that represent provider
// process internals and must never appear on the v1 schema.
var providerProcessForbiddenJSONTags = []string{
	// raw provider output
	"raw_output", "provider_output", "output_excerpt", "stdout", "stderr",
	// provider PID
	"pid", "provider_pid",
	// provider process-tree metadata
	"process_tree", "provider_process_tree", "children_pids", "process_tree_metadata",
	// provider-session canonical state
	"session_canonical_state", "provider_session_state",
	"provider_session_canonical_state", "canonical_state",
}

// TestRunRecordRejectsProviderProcessFields reflects over the v1 record type,
// asserts its full JSON tag set equals the explicit v1 allowlist (so any new
// field fails), asserts provider process-internal field names are absent, and
// proves unmarshaling JSON that contains those keys populates no Record field
// and re-marshaling drops them.
func TestRunRecordRejectsProviderProcessFields(t *testing.T) {
	t.Parallel()

	// 1) Full JSON tag set of Record (and nested named structs) must equal the
	// explicit v1 allowlist. Adding any field without updating the allowlist fails.
	tags := collectJSONTags(reflect.TypeOf(Record{}))
	if !reflect.DeepEqual(tags, v1RecordJSONAllowlist) {
		for name := range tags {
			if _, ok := v1RecordJSONAllowlist[name]; !ok {
				t.Errorf("v1 schema declares unexpected JSON field %q (update allowlist only if intentional and non-process)", name)
			}
		}
		for name := range v1RecordJSONAllowlist {
			if _, ok := tags[name]; !ok {
				t.Errorf("v1 allowlist field %q is missing from Record JSON tags", name)
			}
		}
	}

	// 2) Specifically: no field represents raw provider output, provider PID,
	// provider process-tree metadata, or provider-session canonical state.
	for _, tag := range providerProcessForbiddenJSONTags {
		if _, ok := tags[tag]; ok {
			t.Errorf("v1 schema declares forbidden provider-process field %q", tag)
		}
		if _, ok := v1RecordJSONAllowlist[tag]; ok {
			t.Errorf("v1 allowlist must not include provider-process field %q", tag)
		}
	}

	// Nested public Fizeau result also must not grow process fields.
	fizeauTags := collectJSONTags(reflect.TypeOf(FizeauPublicResult{}))
	for _, tag := range []string{"pid", "provider_pid", "raw_output", "process_tree", "canonical_state"} {
		if _, ok := fizeauTags[tag]; ok {
			t.Errorf("FizeauPublicResult declares forbidden field %q", tag)
		}
	}

	// 3) Unmarshal a document containing provider PID, raw provider output,
	// provider process-tree, and provider-session-state keys: no field of the
	// v1 type is populated by them, and re-marshaling drops them.
	const toxicJSON = `{
		"version": 1,
		"run_id": "run_toxic",
		"phase": "running",
		"started_at": "2026-07-25T12:00:00Z",
		"raw_output": "provider stream dump",
		"provider_output": "more dump",
		"output_excerpt": "snippet",
		"stdout": "out",
		"stderr": "err",
		"pid": 4242,
		"provider_pid": 4242,
		"process_tree": {"root": 1, "children": [2, 3]},
		"provider_process_tree": "1-2-3",
		"children_pids": [2, 3],
		"process_tree_metadata": {"ppid": 1},
		"session_canonical_state": {"status": "running"},
		"provider_session_state": {"alive": true},
		"provider_session_canonical_state": {"id": "sess"},
		"canonical_state": "active"
	}`

	var rec Record
	if err := json.Unmarshal([]byte(toxicJSON), &rec); err != nil {
		t.Fatalf("unmarshal toxic JSON: %v", err)
	}

	// Legitimate core keys from the document are populated.
	if rec.Version != SchemaVersion {
		t.Errorf("version=%d, want %d", rec.Version, SchemaVersion)
	}
	if rec.RunID != "run_toxic" {
		t.Errorf("run_id=%q, want run_toxic", rec.RunID)
	}
	if rec.Phase != PhaseRunning {
		t.Errorf("phase=%q, want %q", rec.Phase, PhaseRunning)
	}
	wantStarted := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if !rec.StartedAt.Equal(wantStarted) {
		t.Errorf("started_at=%v, want %v", rec.StartedAt, wantStarted)
	}

	// Toxic keys must not populate any other v1 field (all remain zero).
	if rec.BeadID != "" || rec.AttemptID != "" || rec.WorkerID != "" || rec.BaseRev != "" {
		t.Errorf("toxic keys must not populate identity fields: bead=%q attempt=%q worker=%q base=%q",
			rec.BeadID, rec.AttemptID, rec.WorkerID, rec.BaseRev)
	}
	if !rec.CreatedAt.IsZero() || !rec.UpdatedAt.IsZero() || rec.FinishedAt != nil {
		t.Errorf("toxic keys must not populate optional timestamps: created=%v updated=%v finished=%v",
			rec.CreatedAt, rec.UpdatedAt, rec.FinishedAt)
	}
	if len(rec.Correlation) != 0 {
		t.Errorf("toxic keys must not populate correlation: %+v", rec.Correlation)
	}
	if rec.Outcome != nil {
		t.Errorf("toxic keys must not populate outcome: %+v", rec.Outcome)
	}
	if rec.Fizeau != nil {
		t.Errorf("toxic keys must not populate fizeau: %+v", rec.Fizeau)
	}
	if len(rec.Evidence) != 0 {
		t.Errorf("toxic keys must not populate evidence: %+v", rec.Evidence)
	}

	// Zero-value Record fields that toxic input might have aliased onto via a
	// mistaken schema tag would leave the typed value non-zero; the field-level
	// zero checks above plus remashal coverage below close that gap.
	rewritten, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	rewrittenStr := string(rewritten)
	for _, tag := range providerProcessForbiddenJSONTags {
		needle := `"` + tag + `"`
		if strings.Contains(rewrittenStr, needle) {
			t.Errorf("re-marshaled record retains forbidden field %q: %s", tag, rewrittenStr)
		}
	}

	// Re-marshaled surface may only use allowlisted keys.
	var rewrittenMap map[string]json.RawMessage
	if err := json.Unmarshal(rewritten, &rewrittenMap); err != nil {
		t.Fatalf("unmarshal rewritten map: %v", err)
	}
	for key := range rewrittenMap {
		if _, ok := v1RecordJSONAllowlist[key]; !ok {
			t.Errorf("re-marshaled record contains non-allowlisted key %q", key)
		}
	}
}

// collectJSONTags walks exported struct fields (including nested named
// structs) and returns the set of json tag names.
func collectJSONTags(t reflect.Type) map[string]struct{} {
	out := make(map[string]struct{})
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "" || tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name != "" {
				out[name] = struct{}{}
			}
			ft := f.Type
			for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && ft.Name() != "Time" {
				walk(ft)
			}
		}
	}
	walk(t)
	return out
}
