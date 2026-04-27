package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSessionIndexWritesMonthlyShards(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), ".ddx", "agent-logs")
	jan := time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 1, 0, 0, time.UTC)

	if err := AppendSessionIndex(logDir, SessionIndexEntry{ID: "jan", Harness: "agent", StartedAt: jan}, jan); err != nil {
		t.Fatal(err)
	}
	if err := AppendSessionIndex(logDir, SessionIndexEntry{ID: "feb", Harness: "agent", StartedAt: feb}, feb); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(logDir, "sessions", "sessions-2026-01.jsonl")); err != nil {
		t.Fatalf("missing January shard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "sessions", "sessions-2026-02.jsonl")); err != nil {
		t.Fatalf("missing February shard: %v", err)
	}
}

func TestAppendSessionIndexDerivesAndValidatesBillingMode(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), ".ddx", "agent-logs")
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	for _, mode := range []string{BillingModePaid, BillingModeSubscription, BillingModeLocal} {
		if err := AppendSessionIndex(logDir, SessionIndexEntry{
			ID:          "explicit-" + mode,
			Harness:     "codex",
			StartedAt:   now,
			BillingMode: mode,
		}, now); err != nil {
			t.Fatalf("AppendSessionIndex explicit %s: %v", mode, err)
		}
	}

	if err := AppendSessionIndex(logDir, SessionIndexEntry{
		ID:          "invalid",
		Harness:     "codex",
		StartedAt:   now,
		BillingMode: "free",
	}, now); err == nil {
		t.Fatal("AppendSessionIndex accepted invalid billingMode")
	}

	derivedAt := now.Add(time.Minute)
	if err := AppendSessionIndex(logDir, SessionIndexEntry{
		ID:        "derived",
		Harness:   "codex",
		StartedAt: derivedAt,
	}, derivedAt); err != nil {
		t.Fatalf("AppendSessionIndex derived: %v", err)
	}
	indexed, err := ReadSessionIndex(logDir, SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range indexed {
		if row.BillingMode == "" {
			t.Fatalf("indexed row %q has empty billingMode", row.ID)
		}
		if row.ID == "derived" {
			found = true
			if row.BillingMode != BillingModeSubscription {
				t.Fatalf("derived billingMode=%q, want %q", row.BillingMode, BillingModeSubscription)
			}
		}
	}
	if !found {
		t.Fatal("derived row not found")
	}
}

func TestReindexLegacySessionsSplitsShardsAndDedupes(t *testing.T) {
	projectRoot := t.TempDir()
	logDir := filepath.Join(projectRoot, DefaultLogDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entries := []SessionEntry{
		{ID: "s-jan", Timestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Harness: "agent", Model: "m"},
		{ID: "s-feb", Timestamp: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), Harness: "agent", Model: "m"},
		{ID: "s-mar", Timestamp: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), Harness: "agent", Model: "m"},
	}
	f, err := os.Create(filepath.Join(logDir, LegacySessionsFileName))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	count, err := ReindexLegacySessions(projectRoot, logDir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("first reindex count=%d, want 3", count)
	}
	if _, err := os.Stat(filepath.Join(logDir, LegacySessionsRenameName)); err != nil {
		t.Fatalf("legacy file not renamed: %v", err)
	}
	for _, shard := range []string{"sessions-2026-01.jsonl", "sessions-2026-02.jsonl", "sessions-2026-03.jsonl"} {
		if _, err := os.Stat(filepath.Join(logDir, "sessions", shard)); err != nil {
			t.Fatalf("missing shard %s: %v", shard, err)
		}
	}

	count, err = ReindexLegacySessions(projectRoot, logDir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("second reindex count=%d, want 0", count)
	}
	indexed, err := ReadSessionIndex(logDir, SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed) != 3 {
		t.Fatalf("indexed rows=%d, want 3", len(indexed))
	}
}

func TestReindexLegacySessionsBackfillsBillingMode(t *testing.T) {
	projectRoot := t.TempDir()
	logDir := filepath.Join(projectRoot, DefaultLogDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	entries := []SessionEntry{
		{ID: "cash", Timestamp: now, Harness: "openrouter", Model: "openai/gpt-5.4", CostUSD: 0.03},
		{ID: "sub", Timestamp: now.Add(time.Minute), Harness: "claude", Model: "claude-sonnet-4-6", CostUSD: 0.04},
		{ID: "local", Timestamp: now.Add(2 * time.Minute), Harness: "agent", Surface: "openai-compat", BaseURL: "http://127.0.0.1:1234/v1"},
	}
	f, err := os.Create(filepath.Join(logDir, LegacySessionsFileName))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	count, err := ReindexLegacySessions(projectRoot, logDir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("reindex count=%d, want 3", count)
	}
	indexed, err := ReadSessionIndex(logDir, SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, row := range indexed {
		if row.BillingMode == "" {
			t.Fatalf("row %q missing billingMode", row.ID)
		}
		got[row.ID] = row.BillingMode
	}
	want := map[string]string{"cash": BillingModePaid, "sub": BillingModeSubscription, "local": BillingModeLocal}
	for id, mode := range want {
		if got[id] != mode {
			t.Fatalf("billingMode[%s]=%q, want %q (all=%v)", id, got[id], mode, got)
		}
	}
}

func TestSessionIndexPreservesWorkerIDCorrelation(t *testing.T) {
	projectRoot := t.TempDir()
	started := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	inputs := SessionIndexInputs{
		Harness: "codex",
		Model:   "gpt-5.4",
		Correlation: map[string]string{
			"session_id": "session-worker",
			"bead_id":    "ddx-worker",
			"worker_id":  "worker-abc",
		},
	}
	entry := SessionIndexEntryFromResult(projectRoot, inputs, &Result{ExitCode: 0}, started, started.Add(time.Second))

	if entry.WorkerID != "worker-abc" {
		t.Fatalf("WorkerID=%q, want worker-abc", entry.WorkerID)
	}
	legacy := SessionIndexEntryToLegacy(entry)
	if got := legacy.Correlation["worker_id"]; got != "worker-abc" {
		t.Fatalf("legacy correlation worker_id=%q, want worker-abc", got)
	}
}

func TestSessionIndexShardFilesSelectsOnlyIntersectingDateRange(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), ".ddx", "agent-logs")
	for month := time.January; month <= time.June; month++ {
		ts := time.Date(2026, month, 2, 0, 0, 0, 0, time.UTC)
		if err := AppendSessionIndex(logDir, SessionIndexEntry{ID: month.String(), Harness: "agent", StartedAt: ts}, ts); err != nil {
			t.Fatal(err)
		}
	}

	after := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	files, err := SessionIndexShardFiles(logDir, SessionIndexQuery{
		StartedAfter:  &after,
		StartedBefore: &before,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("selected shard files=%d, want 2: %v", len(files), files)
	}
	got := []string{filepath.Base(files[0]), filepath.Base(files[1])}
	want := []string{"sessions-2026-04.jsonl", "sessions-2026-03.jsonl"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("selected shards=%v, want %v", got, want)
		}
	}

	entries, err := ReadSessionIndex(logDir, SessionIndexQuery{
		StartedAfter:  &after,
		StartedBefore: &before,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("range entries=%d, want 2", len(entries))
	}
}

func TestLoadSessionBodiesFromBundle(t *testing.T) {
	projectRoot := t.TempDir()
	bundle := filepath.Join(projectRoot, ExecuteBeadArtifactDir, "attempt-1")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "prompt.md"), []byte("prompt body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "result.json"), []byte(`{"response":"response body","stderr":"stderr body"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	bodies := LoadSessionBodies(projectRoot, SessionIndexEntry{BundlePath: filepath.Join(ExecuteBeadArtifactDir, "attempt-1")})
	if bodies.Prompt != "prompt body" || bodies.Response != "response body" || bodies.Stderr != "stderr body" {
		t.Fatalf("unexpected bodies: %+v", bodies)
	}
}

func TestRunViaServiceWithAppendsOneSessionIndexRow(t *testing.T) {
	workDir := t.TempDir()
	embeddedLogDir := filepath.Join(workDir, ExecuteBeadArtifactDir, "attempt-1", "embedded")
	svc := &noopCompactionDdxAgent{interval: time.Millisecond, total: 0}
	var opts RunOptions
	opts.Harness = "agent"
	opts.Prompt = "hello"
	opts.Model = "fake-model"
	opts.SessionLogDir = embeddedLogDir
	_, err := RunViaServiceWith(context.Background(), svc, workDir, opts)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ReadSessionIndex(ResolveLogDir(workDir, ""), SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("session index rows=%d, want 1", len(entries))
	}
	if entries[0].Harness != "agent" {
		t.Fatalf("harness=%q, want agent", entries[0].Harness)
	}
	embeddedEntries, err := ReadSessionIndex(embeddedLogDir, SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddedEntries) != 0 {
		t.Fatalf("embedded session index rows=%d, want 0", len(embeddedEntries))
	}
}

func TestProductionAgentExecutionPathsUseIndexedServiceWriter(t *testing.T) {
	root := filepath.Join("..", "..")
	checks := map[string]string{
		"ddx agent run":  filepath.Join(root, "cmd", "agent_cmd.go"),
		"quorum":         filepath.Join(root, "internal", "agent", "compare_adapter.go"),
		"compare":        filepath.Join(root, "internal", "agent", "compare_adapter.go"),
		"execute-bead":   filepath.Join(root, "internal", "agent", "execute_bead.go"),
		"execute-loop":   filepath.Join(root, "internal", "agent", "execute_bead_loop.go"),
		"replay":         filepath.Join(root, "cmd", "agent_cmd.go"),
		"service writer": filepath.Join(root, "internal", "agent", "service_run.go"),
	}
	for name, path := range checks {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := string(data)
		switch name {
		case "service writer":
			if !strings.Contains(text, "AppendSessionIndex(") {
				t.Fatalf("%s does not append the session index", name)
			}
		case "execute-bead":
			if !strings.Contains(text, "dispatchViaResolvedConfig(") {
				t.Fatalf("%s is not routed through the SD-024 dispatch seam", name)
			}
		case "execute-loop":
			if !strings.Contains(text, "w.Executor.Execute(ctx, candidate.ID)") {
				t.Fatalf("%s no longer invokes the execute-bead executor", name)
			}
		default:
			if !strings.Contains(text, "RunViaService") && !strings.Contains(text, "RunWithConfigViaService") {
				t.Fatalf("%s is not routed through RunViaService", name)
			}
		}
	}
}

func BenchmarkReadSessionIndexDefaultWindow(b *testing.B) {
	logDir := filepath.Join(b.TempDir(), ".ddx", "agent-logs")
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	for month := 0; month < 12; month++ {
		ts := start.AddDate(0, month, 0)
		path := SessionIndexShardPath(logDir, ts)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		enc := json.NewEncoder(f)
		for i := 0; i < 10000; i++ {
			if err := enc.Encode(SessionIndexEntry{
				ID:        "bench",
				Harness:   "agent",
				StartedAt: ts.Add(time.Duration(i) * time.Second),
			}); err != nil {
				b.Fatal(err)
			}
		}
		_ = f.Close()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ReadSessionIndex(logDir, SessionIndexQuery{DefaultRecent: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLegacySessionsJSONLFullScan(b *testing.B) {
	logDir := filepath.Join(b.TempDir(), ".ddx", "agent-logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(logDir, LegacySessionsFileName)
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	enc := json.NewEncoder(f)
	start := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	response := strings.Repeat("x", 2048)
	for i := 0; i < 120000; i++ {
		if err := enc.Encode(SessionEntry{
			ID:        "bench",
			Timestamp: start.Add(time.Duration(i) * time.Second),
			Harness:   "agent",
			Response:  response,
		}); err != nil {
			b.Fatal(err)
		}
	}
	_ = f.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var entries []SessionEntry
		if err := ForEachJSONL[SessionEntry](path, func(entry SessionEntry) error {
			entries = append(entries, entry)
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Timestamp.After(entries[j].Timestamp)
		})
	}
}
