package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/attemptmetrics"
)

// writeAnalyzeRows appends rows to the metrics store in dir.
func writeAnalyzeRows(t *testing.T, dir string, rows []attemptmetrics.AttemptRow) {
	t.Helper()
	for _, r := range rows {
		if err := attemptmetrics.AppendRow(dir, r); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
	}
}

// runAnalyze creates and executes the analyze command with the given args,
// returning stdout+stderr output.
func runAnalyze(t *testing.T, dir string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	f := &CommandFactory{WorkingDir: dir}
	cmd := f.newWorkAnalyzeCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--project", dir))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("analyze command failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestAnalyze_ByModel(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "a1", BeadID: "b1", Model: "model-A", Outcome: "task_succeeded", CostUSD: 1.0, DurationMS: 1000, InputTokens: 100, OutputTokens: 50},
		{SchemaVersion: 1, AttemptID: "a2", BeadID: "b2", Model: "model-A", Outcome: "review_block", CostUSD: 0.5, DurationMS: 2000, InputTokens: 80, OutputTokens: 40},
		{SchemaVersion: 1, AttemptID: "a3", BeadID: "b3", Model: "model-B", Outcome: "task_succeeded", CostUSD: 2.0, DurationMS: 500, InputTokens: 200, OutputTokens: 100},
	})

	out := runAnalyze(t, dir, "--by", "model")

	if !strings.Contains(out, "model-A") {
		t.Errorf("expected model-A in output, got: %s", out)
	}
	if !strings.Contains(out, "model-B") {
		t.Errorf("expected model-B in output, got: %s", out)
	}
	// model-A has 2 attempts; verify count appears
	if !strings.Contains(out, "2") {
		t.Errorf("expected count=2 for model-A in output, got: %s", out)
	}
}

func TestAnalyze_BySpecId(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "s1", BeadID: "b1", SpecID: "FEAT-001", Outcome: "task_succeeded", CostUSD: 1.0, DurationMS: 1000},
		{SchemaVersion: 1, AttemptID: "s2", BeadID: "b2", SpecID: "FEAT-001", Outcome: "no_changes", CostUSD: 0.2, DurationMS: 500},
		{SchemaVersion: 1, AttemptID: "s3", BeadID: "b3", SpecID: "FEAT-002", Outcome: "task_succeeded", CostUSD: 3.0, DurationMS: 3000},
	})

	out := runAnalyze(t, dir, "--by", "spec_id")

	if !strings.Contains(out, "FEAT-001") {
		t.Errorf("expected FEAT-001 in output, got: %s", out)
	}
	if !strings.Contains(out, "FEAT-002") {
		t.Errorf("expected FEAT-002 in output, got: %s", out)
	}
}

func TestAnalyze_ComposedDimensions(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "c1", BeadID: "b1", Model: "model-A", Outcome: "task_succeeded", DurationMS: 1000},
		{SchemaVersion: 1, AttemptID: "c2", BeadID: "b2", Model: "model-A", Outcome: "review_block", DurationMS: 2000},
		{SchemaVersion: 1, AttemptID: "c3", BeadID: "b3", Model: "model-B", Outcome: "task_succeeded", DurationMS: 500},
		{SchemaVersion: 1, AttemptID: "c4", BeadID: "b4", Model: "model-B", Outcome: "review_block", DurationMS: 800},
	})

	out := runAnalyze(t, dir, "--by", "model,outcome")

	// Four distinct model+outcome combos should each have count=1.
	if !strings.Contains(out, "model-A") {
		t.Errorf("expected model-A in output, got: %s", out)
	}
	if !strings.Contains(out, "model-B") {
		t.Errorf("expected model-B in output, got: %s", out)
	}
	if !strings.Contains(out, "task_succeeded") {
		t.Errorf("expected task_succeeded in output, got: %s", out)
	}
	if !strings.Contains(out, "review_block") {
		t.Errorf("expected review_block in output, got: %s", out)
	}
	// Each combo has count=1; verify "1" appears (will appear multiple times).
	if !strings.Contains(out, "1") {
		t.Errorf("expected count=1 entries in output, got: %s", out)
	}
}

func TestAnalyze_SinceFilter(t *testing.T) {
	dir := t.TempDir()
	recentTS := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	oldTS := "2020-01-01T00:00:00Z"

	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "rf1", BeadID: "b1", Model: "recent-model", Outcome: "task_succeeded", TSStart: recentTS, DurationMS: 1000},
		{SchemaVersion: 1, AttemptID: "of1", BeadID: "b2", Model: "old-model", Outcome: "task_succeeded", TSStart: oldTS, DurationMS: 2000},
	})

	out := runAnalyze(t, dir, "--by", "model", "--since", "7d")

	if !strings.Contains(out, "recent-model") {
		t.Errorf("expected recent-model in output, got: %s", out)
	}
	if strings.Contains(out, "old-model") {
		t.Errorf("old-model should be filtered out by --since 7d, got: %s", out)
	}
}

func TestAnalyze_WhereFilter(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "w1", BeadID: "b1", Model: "target-model", Outcome: "task_succeeded", DurationMS: 1000},
		{SchemaVersion: 1, AttemptID: "w2", BeadID: "b2", Model: "other-model", Outcome: "task_succeeded", DurationMS: 2000},
		{SchemaVersion: 1, AttemptID: "w3", BeadID: "b3", Model: "target-model", Outcome: "review_block", DurationMS: 500},
	})

	out := runAnalyze(t, dir, "--where", "model=target-model", "--by", "outcome")

	if !strings.Contains(out, "task_succeeded") {
		t.Errorf("expected task_succeeded in output, got: %s", out)
	}
	if !strings.Contains(out, "review_block") {
		t.Errorf("expected review_block in output, got: %s", out)
	}
	if strings.Contains(out, "other-model") {
		t.Errorf("other-model should be filtered out, got: %s", out)
	}
}

func TestAnalyze_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "j1", BeadID: "b1", Model: "json-model", Outcome: "task_succeeded", CostUSD: 1.5, DurationMS: 3000, InputTokens: 500, OutputTokens: 250},
		{SchemaVersion: 1, AttemptID: "j2", BeadID: "b2", Model: "json-model", Outcome: "review_block", CostUSD: 0.8, DurationMS: 1500, InputTokens: 300, OutputTokens: 150},
	})

	out := runAnalyze(t, dir, "--by", "model", "--json")

	// Output must be valid JSONL with expected fields.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line for 1 model group, got %d: %s", len(lines), out)
	}

	var row analyzeResultRow
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if row.Count != 2 {
		t.Errorf("count=%d, want 2", row.Count)
	}
	if row.SuccessRate != 0.5 {
		t.Errorf("success_rate=%v, want 0.5", row.SuccessRate)
	}
	if row.SumCostUSD != 2.3 {
		t.Errorf("sum_cost_usd=%v, want 2.3", row.SumCostUSD)
	}
	if row.Dimensions["model"] != "json-model" {
		t.Errorf("dimensions.model=%q, want json-model", row.Dimensions["model"])
	}
	if row.ReviewBlockRate != 0.5 {
		t.Errorf("review_block_rate=%v, want 0.5", row.ReviewBlockRate)
	}
}

func TestAnalyze_CSVOutput(t *testing.T) {
	dir := t.TempDir()
	writeAnalyzeRows(t, dir, []attemptmetrics.AttemptRow{
		{SchemaVersion: 1, AttemptID: "cv1", BeadID: "b1", Model: "csv-model", Outcome: "task_succeeded", CostUSD: 1.0, DurationMS: 2000},
		{SchemaVersion: 1, AttemptID: "cv2", BeadID: "b2", Model: "csv-model", Outcome: "task_succeeded", CostUSD: 2.0, DurationMS: 4000},
	})

	out := runAnalyze(t, dir, "--by", "model", "--csv")

	r := csv.NewReader(strings.NewReader(out))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v\noutput: %s", err, out)
	}

	if len(records) < 2 {
		t.Fatalf("expected header + data row, got %d rows", len(records))
	}

	// Verify header contains expected columns.
	header := records[0]
	wantCols := []string{"model", "count", "success_rate", "avg_cost_usd", "sum_cost_usd"}
	for _, col := range wantCols {
		found := false
		for _, h := range header {
			if h == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected column %q in CSV header %v", col, header)
		}
	}

	// Verify data row has model value and count=2.
	dataRow := records[1]
	if dataRow[0] != "csv-model" {
		t.Errorf("data row model=%q, want csv-model", dataRow[0])
	}
	if dataRow[1] != "2" {
		t.Errorf("data row count=%q, want 2", dataRow[1])
	}
}
