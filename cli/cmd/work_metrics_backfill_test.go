package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// writeBeadEventsFixture writes a minimal beads.jsonl and events.jsonl so
// the store can return events for the given bead. Returns the project root.
func writeBeadEventsFixture(t *testing.T, beadID string, events []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	ddxDir := filepath.Join(dir, ddxroot.DirName)
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatalf("mkdir .ddx: %v", err)
	}

	// Write a minimal bead record to beads.jsonl.
	beadLine := map[string]any{
		"id":         beadID,
		"title":      "test bead",
		"status":     "closed",
		"priority":   3,
		"issue_type": "task",
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T12:00:00Z",
	}
	raw, err := json.Marshal(beadLine)
	if err != nil {
		t.Fatalf("marshal bead: %v", err)
	}
	beadsFile := filepath.Join(ddxDir, "beads.jsonl")
	if err := os.WriteFile(beadsFile, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write beads.jsonl: %v", err)
	}

	// Write events to attachments sidecar.
	attDir := filepath.Join(ddxDir, "attachments", beadID)
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		t.Fatalf("mkdir attachments: %v", err)
	}
	var evLines []byte
	for _, ev := range events {
		line, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		evLines = append(evLines, line...)
		evLines = append(evLines, '\n')
	}
	eventsFile := filepath.Join(attDir, "events.jsonl")
	if err := os.WriteFile(eventsFile, evLines, 0o644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}

	// Write the events_attachment pointer into the bead's Extra by rewriting
	// beads.jsonl with the pointer included.
	beadLineWithRef := map[string]any{
		"id":                beadID,
		"title":             "test bead",
		"status":            "closed",
		"priority":          3,
		"issue_type":        "task",
		"created_at":        "2026-01-01T00:00:00Z",
		"updated_at":        "2026-01-01T12:00:00Z",
		"events_attachment": beadID + "/events.jsonl",
	}
	raw2, err := json.Marshal(beadLineWithRef)
	if err != nil {
		t.Fatalf("marshal bead with ref: %v", err)
	}
	if err := os.WriteFile(beadsFile, append(raw2, '\n'), 0o644); err != nil {
		t.Fatalf("rewrite beads.jsonl: %v", err)
	}

	return dir
}

func TestWorkMetricsBackfill_AddsRows(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []map[string]any{
		{
			"kind":       "cost",
			"body":       `{"attempt_id":"20260110T120000-testabcd","harness":"claude","provider":"anthropic","model":"claude-sonnet-4-6","input_tokens":1000,"output_tokens":500,"total_tokens":1500,"cost_usd":1.5,"duration_ms":60000,"exit_code":0}`,
			"created_at": now,
			"source":     "ddx try",
		},
		{
			"kind":       "execute-bead",
			"summary":    "task_succeeded",
			"created_at": now,
			"source":     "ddx work",
		},
	}

	dir := writeBeadEventsFixture(t, "ddx-backtest01", events)

	var out bytes.Buffer
	f := &CommandFactory{WorkingDir: dir}
	cmd := f.newWorkMetricsCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backfill", "--project", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v\noutput: %s", err, out.String())
	}

	if !strings.Contains(out.String(), "1 new rows") {
		t.Errorf("expected '1 new rows' in output, got: %s", out.String())
	}

	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AttemptID != "20260110T120000-testabcd" {
		t.Errorf("attempt_id=%q", rows[0].AttemptID)
	}
	if rows[0].Outcome != "task_succeeded" {
		t.Errorf("outcome=%q, want task_succeeded", rows[0].Outcome)
	}
}

func TestWorkMetricsBackfill_Idempotent(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []map[string]any{
		{
			"kind":       "cost",
			"body":       `{"attempt_id":"20260110T120000-idemtest1","harness":"claude","total_tokens":100,"cost_usd":0.1,"duration_ms":1000,"exit_code":0}`,
			"created_at": now,
			"source":     "ddx try",
		},
		{
			"kind":       "execute-bead",
			"summary":    "task_succeeded",
			"created_at": now,
			"source":     "ddx work",
		},
	}
	dir := writeBeadEventsFixture(t, "ddx-idemtest01", events)

	run := func() string {
		var out bytes.Buffer
		f := &CommandFactory{WorkingDir: dir}
		cmd := f.newWorkMetricsCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"backfill", "--project", dir})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("command failed: %v", err)
		}
		return out.String()
	}

	out1 := run()
	if !strings.Contains(out1, "1 new rows") {
		t.Errorf("first run: expected '1 new rows', got: %s", out1)
	}

	out2 := run()
	if !strings.Contains(out2, "0 new rows") {
		t.Errorf("second run: expected '0 new rows' (idempotent), got: %s", out2)
	}

	rows, err := attemptmetrics.LoadRows(dir)
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row, got %d", len(rows))
	}
}
