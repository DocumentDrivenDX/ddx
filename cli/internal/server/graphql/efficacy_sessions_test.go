package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

func TestEfficacyRowsReadsSessionIndexAsStrictSupersetOfLegacyEvidence(t *testing.T) {
	workDir := t.TempDir()
	store := bead.NewStore(filepath.Join(workDir, ".ddx"))
	closed := &bead.Bead{Title: "closed legacy evidence", Status: bead.StatusOpen}
	if err := store.Create(closed); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(closed.ID, bead.BeadEvent{Kind: "routing", Body: `{"harness":"codex","resolved_provider":"openai","resolved_model":"gpt-5"}`}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(closed.ID, bead.BeadEvent{Kind: "cost", Body: `{"harness":"codex","provider":"openai","model":"gpt-5","input_tokens":100,"output_tokens":50,"duration_ms":1000,"cost_usd":0.01,"exit_code":0}`}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(closed.ID); err != nil {
		t.Fatal(err)
	}
	open := &bead.Bead{Title: "open bead session", Status: bead.StatusOpen}
	if err := store.Create(open); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{ID: "closed-session", BeadID: closed.ID, Harness: "codex", Provider: "openai", Model: "gpt-5", StartedAt: now, DurationMS: 1000, CostUSD: 0.01, CostPresent: true, InputTokens: 100, OutputTokens: 50, Outcome: "success", BundlePath: ".ddx/executions/closed-session"}, now)
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{ID: "open-bead-session", BeadID: open.ID, Harness: "agent", Provider: "openai", Model: "gpt-5.4", StartedAt: now.Add(time.Minute), DurationMS: 2000, InputTokens: 200, OutputTokens: 70, Outcome: "success", BundlePath: ".ddx/executions/open-bead-session"}, now.Add(time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{ID: "agent-run-session", Harness: "agent-run", Provider: "anthropic", Model: "claude-sonnet-4-6", StartedAt: now.Add(2 * time.Minute), DurationMS: 3000, InputTokens: 300, OutputTokens: 80, Outcome: "success", NativeLogRef: ".ddx/agent-logs/agent-run-session.jsonl"}, now.Add(2*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{ID: "benchmark-session", Harness: "benchmark", Provider: "local", Model: "qwen3.5-27b", StartedAt: now.Add(3 * time.Minute), DurationMS: 4000, InputTokens: 400, OutputTokens: 90, Outcome: "failure", Detail: "benchmark failed"}, now.Add(3*time.Minute))

	rows, err := (&queryResolver{Resolver: &Resolver{WorkingDir: workDir}}).EfficacyRows(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*EfficacyRow{}
	for _, row := range rows {
		got[row.RowKey] = row
	}
	legacy := legacyEfficacyRowKeysForTest(t, store)
	for key := range legacy {
		if _, ok := got[key]; !ok {
			t.Fatalf("new session efficacy output missing legacy key %q; got %v", key, sortedKeys(got))
		}
	}
	for _, key := range []string{
		"agent|openai|gpt-5.4",
		"agent-run|anthropic|claude-sonnet-4-6",
		"benchmark|local|qwen3.5-27b",
	} {
		if _, ok := legacy[key]; ok {
			t.Fatalf("test setup error: legacy evidence unexpectedly contains %q", key)
		}
		if _, ok := got[key]; !ok {
			t.Fatalf("new session efficacy output missing session-only key %q; got %v", key, sortedKeys(got))
		}
	}
	if len(got) <= len(legacy) {
		t.Fatalf("new output is not a strict superset: legacy=%v got=%v", sortedSet(legacy), sortedKeys(got))
	}
}

func TestEfficacyRowsFreshWithinTwoSeconds(t *testing.T) {
	workDir := t.TempDir()
	resolver := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	key := "agent|openai|gpt-5.4-mini"
	now := time.Now().UTC()
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{ID: "fresh-session", Harness: "agent", Provider: "openai", Model: "gpt-5.4-mini", StartedAt: now, DurationMS: 1234, InputTokens: 12, OutputTokens: 34, Outcome: "success"}, now)

	deadline := time.Now().Add(2 * time.Second)
	for {
		rows, err := resolver.EfficacyRows(context.Background(), nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.RowKey == key && row.Attempts == 1 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("fresh session %q not visible within 2s; rows=%v", key, rows)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEfficacyRowsDateFilterAndPerfTargets(t *testing.T) {
	if raceEnabled {
		t.Skip("perf thresholds are measured without the race detector")
	}
	workDir := t.TempDir()
	now := time.Now().UTC()
	seedEfficacySessionFixture(t, workDir, 10_000, 2, now)
	resolver := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	ctx := context.Background()

	allTimeInProcess := measureP95(20, func() {
		if rows, err := resolver.EfficacyRows(ctx, nil, nil, nil); err != nil {
			t.Fatal(err)
		} else if len(rows) < 10 {
			t.Fatalf("fixture produced %d rows, want >= 10", len(rows))
		}
	})
	if allTimeInProcess > 300*time.Millisecond {
		t.Fatalf("10k all-time in-process p95=%s, want <=300ms", allTimeInProcess)
	}

	since := now.AddDate(0, 0, -30).Format(time.RFC3339)
	filteredInProcess := measureP95(20, func() {
		if rows, err := resolver.EfficacyRows(ctx, &since, nil, nil); err != nil {
			t.Fatal(err)
		} else if len(rows) < 10 {
			t.Fatalf("date-filter fixture produced %d rows, want >= 10", len(rows))
		}
	})
	if filteredInProcess > 120*time.Millisecond {
		t.Fatalf("10k last-30-days in-process p95=%s, want <=120ms", filteredInProcess)
	}

	httpHandler := efficacyHTTPHandler(workDir)
	allTimeHTTP := measureP95(12, func() {
		graphqlPOSTForTest(t, httpHandler, `{ efficacyRows { rowKey attempts successes successRate medianInputTokens medianOutputTokens medianDurationMs medianCostUsd } }`)
	})
	if allTimeHTTP > 800*time.Millisecond {
		t.Fatalf("10k all-time HTTP p95=%s, want <=800ms", allTimeHTTP)
	}
	filteredHTTP := measureP95(12, func() {
		graphqlPOSTForTest(t, httpHandler, fmt.Sprintf(`{ efficacyRows(since: %q) { rowKey attempts } }`, since))
	})
	if filteredHTTP > 400*time.Millisecond {
		t.Fatalf("10k last-30-days HTTP p95=%s, want <=400ms", filteredHTTP)
	}

	stretchDir := t.TempDir()
	seedEfficacySessionFixture(t, stretchDir, 50_000, 24, now)
	stretchResolver := &queryResolver{Resolver: &Resolver{WorkingDir: stretchDir}}
	stretchInProcess := measureP95(10, func() {
		if rows, err := stretchResolver.EfficacyRows(ctx, nil, nil, nil); err != nil {
			t.Fatal(err)
		} else if len(rows) < 10 {
			t.Fatalf("stretch fixture produced %d rows, want >= 10", len(rows))
		}
	})
	if stretchInProcess > 800*time.Millisecond {
		t.Fatalf("50k stretch all-time in-process p95=%s, want <=800ms", stretchInProcess)
	}
	stretchHTTPHandler := efficacyHTTPHandler(stretchDir)
	stretchHTTP := measureP95(8, func() {
		graphqlPOSTForTest(t, stretchHTTPHandler, `{ efficacyRows { rowKey attempts successes successRate medianDurationMs } }`)
	})
	if stretchHTTP > 2*time.Second {
		t.Fatalf("50k stretch all-time HTTP p95=%s, want <=2000ms", stretchHTTP)
	}
	t.Logf("efficacy perf baseline: 10k all-time in-process p95=%s http p95=%s; last30 in-process p95=%s http p95=%s; 50k/24-shard all-time in-process p95=%s http p95=%s", allTimeInProcess, allTimeHTTP, filteredInProcess, filteredHTTP, stretchInProcess, stretchHTTP)
}

// TestEfficacyRowsSmokeOverRealBackend is the backend half of the
// AC §8 Playwright smoke: it seeds the same 10k-session index fixture the
// frontend smoke mirrors (see cli/internal/server/frontend/e2e/efficacy.spec.ts
// "smoke: efficacy opens with 10k-session rollup fixture"), then drives the
// real GraphQL HTTP handler end-to-end — first EfficacyRows, then
// EfficacyAttempts for the first row returned (the "click into the row"
// gesture). Together with the Playwright smoke they form the ddx-0a33bc5f
// equivalent of the beads smoke pair (Playwright UI mock +
// cli/internal/server/perf/smoke_test.go real-backend ceiling).
func TestEfficacyRowsSmokeOverRealBackend(t *testing.T) {
	workDir := t.TempDir()
	now := time.Now().UTC()
	seedEfficacySessionFixture(t, workDir, 10_000, 2, now)

	httpHandler := efficacyHTTPHandler(workDir)
	rowsResp := graphqlPOSTForTestReturningBody(t, httpHandler, `{ efficacyRows { rowKey harness provider model attempts successes successRate medianDurationMs medianCostUsd } }`)
	var rowsData struct {
		Data struct {
			EfficacyRows []struct {
				RowKey           string   `json:"rowKey"`
				Attempts         int      `json:"attempts"`
				Successes        int      `json:"successes"`
				SuccessRate      float64  `json:"successRate"`
				MedianDurationMs int      `json:"medianDurationMs"`
				MedianCostUsd    *float64 `json:"medianCostUsd"`
			} `json:"efficacyRows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rowsResp, &rowsData); err != nil {
		t.Fatalf("decode efficacyRows: %v body=%s", err, rowsResp)
	}
	if got := len(rowsData.Data.EfficacyRows); got < 5 {
		t.Fatalf("expected at least 5 efficacy rows from the 10k fixture, got %d", got)
	}

	firstRowKey := rowsData.Data.EfficacyRows[0].RowKey
	start := time.Now()
	attemptsResp := graphqlPOSTForTestReturningBody(t, httpHandler, fmt.Sprintf(`{ efficacyAttempts(rowKey: %q) { rowKey attempts { beadId outcome durationMs costUsd evidenceBundleUrl } } }`, firstRowKey))
	clickInto := time.Since(start)
	var attemptsData struct {
		Data struct {
			EfficacyAttempts struct {
				RowKey   string `json:"rowKey"`
				Attempts []struct {
					BeadID            string   `json:"beadId"`
					Outcome           string   `json:"outcome"`
					DurationMs        int      `json:"durationMs"`
					CostUsd           *float64 `json:"costUsd"`
					EvidenceBundleURL string   `json:"evidenceBundleUrl"`
				} `json:"attempts"`
			} `json:"efficacyAttempts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(attemptsResp, &attemptsData); err != nil {
		t.Fatalf("decode efficacyAttempts: %v body=%s", err, attemptsResp)
	}
	if attemptsData.Data.EfficacyAttempts.RowKey != firstRowKey {
		t.Fatalf("efficacyAttempts returned rowKey %q, want %q", attemptsData.Data.EfficacyAttempts.RowKey, firstRowKey)
	}
	if len(attemptsData.Data.EfficacyAttempts.Attempts) == 0 {
		t.Fatalf("efficacyAttempts(%q) returned no attempts", firstRowKey)
	}
	// "Normal navigation budget": a click-into request against the already-
	// warmed cache must stay well under one second. This is a regression
	// ceiling, not a p95 target.
	if clickInto > time.Second {
		t.Fatalf("efficacyAttempts click-into took %s, exceeding the 1s navigation ceiling", clickInto)
	}
	t.Logf("efficacy real-backend smoke: rows=%d firstRowKey=%s clickInto=%s", len(rowsData.Data.EfficacyRows), firstRowKey, clickInto)
}

func appendSessionForTest(t *testing.T, workDir string, entry agent.SessionIndexEntry, ts time.Time) {
	t.Helper()
	if entry.ProjectID == "" {
		entry.ProjectID = agent.ProjectIDForPath(workDir)
	}
	if err := agent.AppendSessionIndex(agent.SessionLogDirForWorkDir(workDir), entry, ts); err != nil {
		t.Fatal(err)
	}
}

func seedEfficacySessionFixture(t *testing.T, workDir string, count, shards int, now time.Time) {
	t.Helper()
	logDir := agent.SessionLogDirForWorkDir(workDir)
	if err := os.MkdirAll(agent.SessionIndexDir(logDir), 0o755); err != nil {
		t.Fatal(err)
	}
	groups := []struct {
		harness  string
		provider string
		model    string
	}{
		{"agent", "openai", "gpt-5.4"},
		{"agent", "openai", "gpt-5.4-mini"},
		{"codex", "openai", "gpt-5.3-codex"},
		{"claude", "anthropic", "claude-sonnet-4-6"},
		{"claude", "anthropic", "claude-opus-4-6"},
		{"gemini", "google", "gemini-2.5-pro"},
		{"benchmark", "local", "qwen3.5-27b"},
		{"quorum", "openrouter", "minimax/minimax-m2.7"},
		{"agent-run", "moonshot", "moonshot/kimi-k2.5"},
		{"script", "vidar", "qwen/qwen3-coder-next"},
	}
	type shardWriter struct {
		file *os.File
		enc  *json.Encoder
	}
	writers := map[string]shardWriter{}
	defer func() {
		for _, writer := range writers {
			_ = writer.file.Close()
		}
	}()
	projectID := agent.ProjectIDForPath(workDir)
	current := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		shardOffset := i % shards
		var ts time.Time
		if shardOffset == 0 {
			ts = now.Add(-time.Duration(i%5000) * time.Minute)
		} else {
			ts = current.AddDate(0, -shardOffset-1, 0).Add(time.Duration(i%720) * time.Minute)
		}
		path := agent.SessionIndexShardPath(logDir, ts)
		writer, ok := writers[path]
		if !ok {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				t.Fatal(err)
			}
			writer = shardWriter{file: file, enc: json.NewEncoder(file)}
			writers[path] = writer
		}
		group := groups[(i/shards)%len(groups)]
		entry := agent.SessionIndexEntry{
			ID:           fmt.Sprintf("sess-%06d", i),
			ProjectID:    projectID,
			BeadID:       fmt.Sprintf("ddx-fixture-%04d", i%137),
			Harness:      group.harness,
			Provider:     group.provider,
			Model:        group.model,
			StartedAt:    ts,
			EndedAt:      ts.Add(time.Duration(500+i%10_000) * time.Millisecond),
			DurationMS:   500 + i%10_000,
			CostUSD:      float64(i%1000) / 100_000,
			CostPresent:  true,
			InputTokens:  1000 + i%5000,
			OutputTokens: 200 + i%2000,
			Outcome:      "success",
			BundlePath:   fmt.Sprintf(".ddx/executions/fixture-%06d", i),
		}
		if i%11 == 0 {
			entry.Outcome = "failure"
			entry.ExitCode = 1
			entry.Detail = "fixture failure"
		}
		if err := writer.enc.Encode(entry); err != nil {
			t.Fatal(err)
		}
	}
}

func measureP95(iterations int, fn func()) time.Duration {
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		fn()
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	idx := int(float64(iterations)*0.95+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(durations) {
		idx = len(durations) - 1
	}
	return durations[idx]
}

func efficacyHTTPHandler(workDir string) http.Handler {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{WorkingDir: workDir}}))
	srv.AddTransport(transport.POST{})
	return srv
}

func graphqlPOSTForTest(t *testing.T, h http.Handler, query string) {
	t.Helper()
	_ = graphqlPOSTForTestReturningBody(t, h, query)
}

func graphqlPOSTForTestReturningBody(t *testing.T, h http.Handler, query string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("content-type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("graphql status=%d body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.Bytes()
	var payload struct {
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) > 0 {
		t.Fatalf("graphql errors: %s", raw)
	}
	return raw
}

func legacyEfficacyRowKeysForTest(t *testing.T, store *bead.Store) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	beads, err := store.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range beads {
		if b.Status != bead.StatusClosed {
			continue
		}
		events, err := store.Events(b.ID)
		if err != nil {
			t.Fatal(err)
		}
		var route struct {
			Harness          string `json:"harness"`
			Provider         string `json:"provider"`
			Model            string `json:"model"`
			ResolvedProvider string `json:"resolved_provider"`
			ResolvedModel    string `json:"resolved_model"`
		}
		for _, event := range events {
			switch event.Kind {
			case "routing":
				_ = json.Unmarshal([]byte(event.Body), &route)
			case "cost":
				var cost struct {
					Harness  string `json:"harness"`
					Provider string `json:"provider"`
					Model    string `json:"model"`
				}
				_ = json.Unmarshal([]byte(event.Body), &cost)
				harness := firstNonEmpty(cost.Harness, route.Harness, cost.Provider, route.Provider, route.ResolvedProvider, "unknown")
				provider := firstNonEmpty(cost.Provider, route.Provider, route.ResolvedProvider, harness)
				model := firstNonEmpty(cost.Model, route.Model, route.ResolvedModel, "unknown")
				out[strings.Join([]string{harness, provider, model}, "|")] = struct{}{}
			}
		}
	}
	return out
}

func sortedKeys(rows map[string]*EfficacyRow) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
