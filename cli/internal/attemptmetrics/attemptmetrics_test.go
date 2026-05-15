package attemptmetrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func TestMetrics_SchemaVersion(t *testing.T) {
	if attemptmetrics.SchemaVersion != 1 {
		t.Fatalf("expected SchemaVersion=1, got %d", attemptmetrics.SchemaVersion)
	}
	row := attemptmetrics.AttemptRow{SchemaVersion: attemptmetrics.SchemaVersion}
	if row.SchemaVersion != 1 {
		t.Fatalf("schema_version field not 1, got %d", row.SchemaVersion)
	}
}

func TestMetrics_AppendRowOnFinalization(t *testing.T) {
	dir := t.TempDir()
	row := attemptmetrics.AttemptRow{
		SchemaVersion: attemptmetrics.SchemaVersion,
		AttemptID:     "20260101T000000-aabbccdd",
		BeadID:        "ddx-test001",
		SessionID:     "agent-loop-123",
		TSStart:       "2026-01-01T00:00:00Z",
		TSEnd:         "2026-01-01T00:10:00Z",
		Model:         "claude-sonnet-4-6",
		Harness:       "claude",
		Provider:      "anthropic",
		Profile:       "smart",
		SpecID:        "FEAT-001",
		Outcome:       "task_succeeded",
		ExitCode:      0,
		CostUSD:       1.2345,
		DurationMS:    600000,
		TotalTokens:   50000,
		ReviewVerdict: "APPROVE",
	}

	if err := attemptmetrics.AppendRow(dir, row); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}

	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version=%d, want 1", got.SchemaVersion)
	}
	if got.AttemptID != row.AttemptID {
		t.Errorf("attempt_id=%q, want %q", got.AttemptID, row.AttemptID)
	}
	if got.BeadID != row.BeadID {
		t.Errorf("bead_id=%q, want %q", got.BeadID, row.BeadID)
	}
	if got.Model != row.Model {
		t.Errorf("model=%q, want %q", got.Model, row.Model)
	}
	if got.Outcome != row.Outcome {
		t.Errorf("outcome=%q, want %q", got.Outcome, row.Outcome)
	}
	if got.ReviewVerdict != row.ReviewVerdict {
		t.Errorf("review_verdict=%q, want %q", got.ReviewVerdict, row.ReviewVerdict)
	}
	if got.CostUSD != row.CostUSD {
		t.Errorf("cost_usd=%v, want %v", got.CostUSD, row.CostUSD)
	}

	// Verify the file is valid JSONL (one JSON object per line).
	raw, err := os.ReadFile(attemptmetrics.AttemptsPath(dir))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw[:len(raw)-1], &parsed); err != nil { // trim newline
		t.Errorf("row is not valid JSON: %v", err)
	}
	if parsed["schema_version"] == nil {
		t.Error("schema_version field missing from serialized row")
	}
}

func TestMetrics_AppendRowMultiple(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		row := attemptmetrics.AttemptRow{
			SchemaVersion: attemptmetrics.SchemaVersion,
			AttemptID:     "attempt-" + string(rune('A'+i)),
			BeadID:        "bead-001",
			Outcome:       "task_succeeded",
		}
		if err := attemptmetrics.AppendRow(dir, row); err != nil {
			t.Fatalf("AppendRow %d: %v", i, err)
		}
	}
	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestMetrics_BackfillFromEvents(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	costBody := `{"attempt_id":"20260101T120000-abcd1234","harness":"claude","provider":"anthropic","model":"claude-sonnet-4-6","input_tokens":1000,"output_tokens":500,"total_tokens":1500,"cost_usd":0.75,"duration_ms":60000,"exit_code":0}`
	events := []bead.BeadEvent{
		{Kind: "cost", Body: costBody, CreatedAt: now},
		{Kind: "execute-bead", Summary: "task_succeeded", CreatedAt: now.Add(time.Minute)},
	}

	beads := []attemptmetrics.BeadAttemptEvents{
		{BeadID: "ddx-test001", SpecID: "FEAT-001", Events: events},
	}

	added, err := attemptmetrics.BackfillFromEvents(dir, beads)
	if err != nil {
		t.Fatalf("BackfillFromEvents: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 row added, got %d", added)
	}

	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if row.SchemaVersion != 1 {
		t.Errorf("schema_version=%d, want 1", row.SchemaVersion)
	}
	if row.AttemptID != "20260101T120000-abcd1234" {
		t.Errorf("attempt_id=%q", row.AttemptID)
	}
	if row.BeadID != "ddx-test001" {
		t.Errorf("bead_id=%q, want ddx-test001", row.BeadID)
	}
	if row.Harness != "claude" {
		t.Errorf("harness=%q, want claude", row.Harness)
	}
	if row.Model != "claude-sonnet-4-6" {
		t.Errorf("model=%q", row.Model)
	}
	if row.Outcome != "task_succeeded" {
		t.Errorf("outcome=%q, want task_succeeded", row.Outcome)
	}
	if row.InputTokens != 1000 {
		t.Errorf("input_tokens=%d, want 1000", row.InputTokens)
	}
	if row.OutputTokens != 500 {
		t.Errorf("output_tokens=%d, want 500", row.OutputTokens)
	}
	if row.TotalTokens != 1500 {
		t.Errorf("total_tokens=%d, want 1500", row.TotalTokens)
	}
	if row.CostUSD != 0.75 {
		t.Errorf("cost_usd=%v, want 0.75", row.CostUSD)
	}
	if row.DurationMS != 60000 {
		t.Errorf("duration_ms=%d, want 60000", row.DurationMS)
	}
	if row.SpecID != "FEAT-001" {
		t.Errorf("spec_id=%q, want FEAT-001", row.SpecID)
	}
	if row.TSEnd == "" {
		t.Error("ts_end should be set from cost event created_at")
	}
}

func TestMetrics_BackfillIdempotent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	costBody := `{"attempt_id":"idempotent-attempt-1","harness":"claude","input_tokens":100,"output_tokens":50,"total_tokens":150,"cost_usd":0.01,"duration_ms":1000,"exit_code":0}`
	events := []bead.BeadEvent{
		{Kind: "cost", Body: costBody, CreatedAt: now},
		{Kind: "execute-bead", Summary: "task_succeeded", CreatedAt: now.Add(time.Minute)},
	}
	beads := []attemptmetrics.BeadAttemptEvents{
		{BeadID: "ddx-idem001", Events: events},
	}

	// First run.
	added1, err := attemptmetrics.BackfillFromEvents(dir, beads)
	if err != nil {
		t.Fatalf("first BackfillFromEvents: %v", err)
	}
	if added1 != 1 {
		t.Fatalf("first run: expected 1, got %d", added1)
	}

	// Second run must add 0 rows (idempotent).
	added2, err := attemptmetrics.BackfillFromEvents(dir, beads)
	if err != nil {
		t.Fatalf("second BackfillFromEvents: %v", err)
	}
	if added2 != 0 {
		t.Fatalf("second run: expected 0 (idempotent), got %d", added2)
	}

	// Total rows should still be 1.
	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row total, got %d", len(rows))
	}
}

func TestMetrics_BackfillSkipsMalformedCostEvents(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []bead.BeadEvent{
		// Missing attempt_id → should be skipped.
		{Kind: "cost", Body: `{"harness":"claude","total_tokens":100}`, CreatedAt: now},
		// Malformed JSON → should be skipped.
		{Kind: "cost", Body: `not-json`, CreatedAt: now},
		// Valid row.
		{Kind: "cost", Body: `{"attempt_id":"good-attempt","harness":"claude","input_tokens":10,"output_tokens":5,"total_tokens":15,"cost_usd":0.01,"duration_ms":500,"exit_code":0}`, CreatedAt: now},
		{Kind: "execute-bead", Summary: "task_succeeded", CreatedAt: now.Add(time.Minute)},
	}
	beads := []attemptmetrics.BeadAttemptEvents{
		{BeadID: "ddx-skip001", Events: events},
	}

	added, err := attemptmetrics.BackfillFromEvents(dir, beads)
	if err != nil {
		t.Fatalf("BackfillFromEvents: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 valid row, got %d", added)
	}
}

func TestMetrics_AttemptsPathUnderDdxMetrics(t *testing.T) {
	path := attemptmetrics.AttemptsPath("/project/root")
	want := filepath.Join("/project/root", ddxroot.DirName, "metrics", "attempts.jsonl")
	if path != want {
		t.Errorf("AttemptsPath=%q, want %q", path, want)
	}
}

func TestMetrics_LoadAttemptIDs_EmptyWhenMissing(t *testing.T) {
	dir := t.TempDir()
	ids, err := attemptmetrics.LoadAttemptIDs(dir)
	if err != nil {
		t.Fatalf("LoadAttemptIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %v", ids)
	}
}

func TestMetrics_BackfillMultipleBeads(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	makeCostBody := func(id, harness string) string {
		return `{"attempt_id":"` + id + `","harness":"` + harness + `","total_tokens":100,"cost_usd":0.1,"duration_ms":1000,"exit_code":0}`
	}

	beads := []attemptmetrics.BeadAttemptEvents{
		{
			BeadID: "ddx-multi001",
			Events: []bead.BeadEvent{
				{Kind: "cost", Body: makeCostBody("attempt-A", "claude"), CreatedAt: now},
				{Kind: "execute-bead", Summary: "task_succeeded", CreatedAt: now.Add(time.Minute)},
			},
		},
		{
			BeadID: "ddx-multi002",
			Events: []bead.BeadEvent{
				{Kind: "cost", Body: makeCostBody("attempt-B", "gemini"), CreatedAt: now},
				{Kind: "execute-bead", Summary: "review_block", CreatedAt: now.Add(time.Minute)},
				{Kind: "cost", Body: makeCostBody("attempt-C", "claude"), CreatedAt: now.Add(2 * time.Minute)},
				{Kind: "execute-bead", Summary: "task_succeeded", CreatedAt: now.Add(3 * time.Minute)},
			},
		},
	}

	added, err := attemptmetrics.BackfillFromEvents(dir, beads)
	if err != nil {
		t.Fatalf("BackfillFromEvents: %v", err)
	}
	if added != 3 {
		t.Fatalf("expected 3 rows, got %d", added)
	}

	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Verify outcome pairing: attempt-B should pair with review_block.
	var rowB attemptmetrics.AttemptRow
	for _, r := range rows {
		if r.AttemptID == "attempt-B" {
			rowB = r
		}
	}
	if rowB.Outcome != "review_block" {
		t.Errorf("attempt-B outcome=%q, want review_block", rowB.Outcome)
	}
}
