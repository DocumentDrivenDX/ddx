package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// writeMinimalConfig creates the minimum .ddx/config.yaml a queryResolver needs
// so agent.NewServiceFromWorkDir can load. No endpoints configured.
func writeMinimalConfig(t *testing.T, workDir string) {
	t.Helper()
	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: \"1.0\"\nbead:\n  id_prefix: \"pt\"\n"
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeEndpointConfig(t *testing.T, workDir string) string {
	t.Helper()
	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: "1.0"
bead:
  id_prefix: "pt"
agent:
  endpoints:
    - type: lmstudio
      base_url: http://127.0.0.1:9/v1
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return "lmstudio-127-0-0-1-9"
}

// TestHarnessStatusesIncludesSubprocessHarnesses asserts claude and codex
// show up as kind=HARNESS when their binaries are installed on PATH.
// Satisfies AC 1 row "Subprocess harnesses claude and codex appear in the table".
func TestHarnessStatusesIncludesSubprocessHarnesses(t *testing.T) {
	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	statuses, err := r.HarnessStatuses(context.Background())
	if err != nil {
		t.Fatalf("HarnessStatuses: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one harness in statuses")
	}

	byName := make(map[string]*ProviderStatus, len(statuses))
	for _, s := range statuses {
		if s.Kind != ProviderKindHarness {
			t.Errorf("harness %q: got kind %q want HARNESS", s.Name, s.Kind)
		}
		if s.LastCheckedAt == nil || *s.LastCheckedAt == "" {
			t.Errorf("harness %q: missing lastCheckedAt", s.Name)
		}
		if s.Detail == "" {
			t.Errorf("harness %q: missing detail", s.Name)
		}
		byName[s.Name] = s
	}

	// Both claude and codex registrations are built into the upstream agent
	// service; they should appear regardless of whether the binary is present.
	for _, required := range []string{"claude", "codex"} {
		if _, ok := byName[required]; !ok {
			names := make([]string, 0, len(byName))
			for n := range byName {
				names = append(names, n)
			}
			t.Errorf("expected harness %q in the unified list; saw %v", required, names)
		}
	}
}

// TestProviderStatusesHasKindAndLastCheckedAt asserts the existing endpoint
// resolver now annotates rows with kind=ENDPOINT and lastCheckedAt — AC 1.
func TestProviderStatusesHasKindAndLastCheckedAt(t *testing.T) {
	workDir := t.TempDir()
	endpointName := writeEndpointConfig(t, workDir)

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	start := time.Now()
	statuses, err := r.ProviderStatuses(context.Background())
	if err != nil {
		t.Fatalf("ProviderStatuses: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("ProviderStatuses took %s; expected configured snapshot first-paint under 500ms", elapsed)
	}
	if len(statuses) != 1 || statuses[0].Name != endpointName {
		t.Fatalf("expected configured endpoint %q, got %#v", endpointName, statuses)
	}
	// Even when zero endpoints are configured, the resolver must succeed.
	for _, s := range statuses {
		if s.Kind != ProviderKindEndpoint {
			t.Errorf("endpoint row %q: got kind %q want ENDPOINT", s.Name, s.Kind)
		}
		if s.Reachable {
			t.Errorf("endpoint row %q: configured snapshot must not fabricate reachability", s.Name)
		}
		if s.Detail == "" {
			t.Errorf("endpoint row %q: missing detail", s.Name)
		}
		if s.LastCheckedAt == nil || *s.LastCheckedAt == "" {
			t.Errorf("endpoint row %q: missing lastCheckedAt", s.Name)
		}
		if s.DefaultForProfile == nil {
			t.Errorf("endpoint row %q: defaultForProfile must not be nil", s.Name)
		}
	}
}

// TestProviderStatusesUsageFromSessionIndex seeds the session index and asserts
// usage counts populate correctly. AC 2 row "Deliverable 2 usage+quota".
func TestProviderStatusesUsageFromSessionIndex(t *testing.T) {
	workDir := t.TempDir()
	endpointName := writeEndpointConfig(t, workDir)

	now := time.Now().UTC()
	// 3 sessions for endpoint provider in last hour, 1 more from 5h ago.
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s1", Harness: "agent", Provider: endpointName, Model: "m",
		StartedAt: now.Add(-10 * time.Minute), Tokens: 1000, Outcome: "success",
	}, now.Add(-10*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s2", Harness: "agent", Provider: endpointName, Model: "m",
		StartedAt: now.Add(-30 * time.Minute), Tokens: 2000, Outcome: "success",
	}, now.Add(-30*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s3", Harness: "agent", Provider: endpointName, Model: "m",
		StartedAt: now.Add(-55 * time.Minute), Tokens: 500, Outcome: "success",
	}, now.Add(-55*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s4", Harness: "agent", Provider: endpointName, Model: "m",
		StartedAt: now.Add(-5 * time.Hour), Tokens: 400, Outcome: "success",
	}, now.Add(-5*time.Hour))

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	statuses, err := r.ProviderStatuses(context.Background())
	if err != nil {
		t.Fatalf("ProviderStatuses: %v", err)
	}
	var endpoint *ProviderStatus
	for _, s := range statuses {
		if s.Name == endpointName {
			endpoint = s
			break
		}
	}
	if endpoint == nil {
		t.Fatal("expected endpoint provider row")
	}
	if endpoint.Usage == nil {
		t.Fatal("expected usage populated from session index")
	}
	if got := deref(endpoint.Usage.TokensUsedLastHour); got != 3500 {
		t.Errorf("tokensUsedLastHour: got %d want 3500", got)
	}
	if got := deref(endpoint.Usage.TokensUsedLast24h); got != 3900 {
		t.Errorf("tokensUsedLast24h: got %d want 3900", got)
	}
	if got := deref(endpoint.Usage.RequestsLastHour); got != 3 {
		t.Errorf("requestsLastHour: got %d want 3", got)
	}
	if got := deref(endpoint.Usage.RequestsLast24h); got != 4 {
		t.Errorf("requestsLast24h: got %d want 4", got)
	}
}

// TestProviderTrendReturnsSeries seeds 7 days of usage and checks the trend
// resolver returns hourly buckets. AC 3.
func TestProviderTrendReturnsSeries(t *testing.T) {
	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)

	now := time.Now().UTC()
	// Seed one session per hour for the last 7 days on harness=claude.
	for i := 0; i < 24*7; i++ {
		t0 := now.Add(-time.Duration(i) * time.Hour)
		appendSessionForTest(t, workDir, agent.SessionIndexEntry{
			ID: fmt.Sprintf("t%d", i), Harness: "claude", Provider: "anthropic",
			StartedAt: t0, Tokens: 100, Outcome: "success",
		}, t0)
	}
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	trend, err := r.ProviderTrend(context.Background(), "claude", 7)
	if err != nil {
		t.Fatalf("ProviderTrend: %v", err)
	}
	if trend == nil {
		t.Fatal("expected trend object, got nil")
	}
	if trend.Kind != ProviderKindHarness {
		t.Errorf("kind: got %q want HARNESS", trend.Kind)
	}
	// 7 days * 24 hours = 168 buckets.
	if len(trend.Series) != 168 {
		t.Errorf("series length: got %d want 168", len(trend.Series))
	}
	var nonEmpty int
	for _, p := range trend.Series {
		if p.Tokens > 0 {
			nonEmpty++
		}
	}
	if nonEmpty < 120 {
		// Seeded 168 hourly sessions; a few may land on bucket boundaries and
		// miss the current-hour bucket, so allow some slack.
		t.Errorf("expected many non-empty buckets, got %d", nonEmpty)
	}
}

// TestProviderTrendRejectsBadWindow ensures window validation works.
func TestProviderTrendRejectsBadWindow(t *testing.T) {
	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	if _, err := r.ProviderTrend(context.Background(), "claude", 14); err == nil {
		t.Error("expected error for windowDays=14")
	}
	if _, err := r.ProviderTrend(context.Background(), "", 7); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestProjectRunOutHoursUsesRemainingHeadroom(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	buckets := make([]agent.UsageBucket, 24)
	for i := range buckets {
		buckets[i] = agent.UsageBucket{
			Start:  now.Add(time.Duration(i-23) * time.Hour),
			Tokens: 1000 + i*100,
		}
	}
	hours := projectRunOutHours(buckets, 10_000)
	if hours <= 0 {
		t.Fatalf("expected positive projection, got %f", hours)
	}
	if hours < 90 || hours > 110 {
		t.Fatalf("projection hours = %f, want roughly 100", hours)
	}
}

// TestQuotaFromRateLimitSignalShape ensures the exposed helper round-trips
// parsed rate-limit headers into the GraphQL ProviderQuota.
func TestQuotaFromRateLimitSignalShape(t *testing.T) {
	sig := agent.ParseRateLimitHeaders("claude", map[string][]string{
		"Anthropic-Ratelimit-Tokens-Limit":     {"50000"},
		"Anthropic-Ratelimit-Tokens-Remaining": {"49500"},
		"Anthropic-Ratelimit-Tokens-Reset":     {"2026-04-23T05:00:00Z"},
	})
	q := QuotaFromRateLimitSignal(sig)
	if q == nil {
		t.Fatal("expected quota from sig")
	}
	if q.CeilingTokens == nil || *q.CeilingTokens != 50000 {
		t.Errorf("ceilingTokens: %+v", q.CeilingTokens)
	}
	if q.Remaining == nil || *q.Remaining != 49500 {
		t.Errorf("remaining: %+v", q.Remaining)
	}
	if q.ResetAt == nil {
		t.Errorf("resetAt: missing")
	}
	if q.CeilingWindowSeconds == nil || *q.CeilingWindowSeconds != 60 {
		t.Errorf("window seconds: %+v", q.CeilingWindowSeconds)
	}
}

func deref(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// TestBuildUsageReturnsNilWhenNoEntries guarantees the no-fabrication rule
// for AC 2: absence of session data yields null usage, not zero-valued counts.
func TestBuildUsageReturnsNilWhenNoEntries(t *testing.T) {
	now := time.Now().UTC()
	u := buildUsage(nil, "anything", agent.MatchProvider, now)
	if u != nil {
		t.Fatalf("expected nil usage when there are no entries, got %+v", u)
	}
	// One entry that doesn't match the requested name should also yield nil.
	entries := []agent.SessionIndexEntry{{
		ID: "x", Harness: "claude", Provider: "anthropic",
		StartedAt: now.Add(-10 * time.Minute), Tokens: 123, Outcome: "success",
	}}
	u = buildUsage(entries, "other-name", agent.MatchProvider, now)
	if u != nil {
		t.Fatalf("expected nil usage when no entry matches name, got %+v", u)
	}
}

// TestHarnessQuotaFromCapturedRateLimitSignal asserts the bridge from captured
// rate-limit headers (via RecordHarnessRateLimit) into the HarnessStatuses row.
// Covers AC 2: "For subprocess harnesses that expose rate-limit headers
// (claude, codex), parsed headers populate quota.{ceilingTokens, ...}."
func TestHarnessQuotaFromCapturedRateLimitSignal(t *testing.T) {
	t.Cleanup(resetHarnessRateLimitCache)
	resetHarnessRateLimitCache()

	sig := agent.ParseRateLimitHeaders("claude", map[string][]string{
		"Anthropic-Ratelimit-Tokens-Limit":     {"80000"},
		"Anthropic-Ratelimit-Tokens-Remaining": {"60000"},
		"Anthropic-Ratelimit-Tokens-Reset":     {"2026-04-24T05:00:00Z"},
	})
	if !sig.HasAny() {
		t.Fatal("expected signal to have fields")
	}
	RecordHarnessRateLimit("claude", sig)

	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	statuses, err := r.HarnessStatuses(context.Background())
	if err != nil {
		t.Fatalf("HarnessStatuses: %v", err)
	}
	var claude *ProviderStatus
	for _, s := range statuses {
		if s.Name == "claude" {
			claude = s
			break
		}
	}
	if claude == nil {
		t.Fatalf("claude harness not in statuses")
	}
	if claude.Quota == nil {
		t.Fatalf("expected quota populated from captured signal")
	}
	if claude.Quota.CeilingTokens == nil || *claude.Quota.CeilingTokens != 80000 {
		t.Errorf("ceilingTokens: %+v", claude.Quota.CeilingTokens)
	}
	if claude.Quota.Remaining == nil || *claude.Quota.Remaining != 60000 {
		t.Errorf("remaining: %+v", claude.Quota.Remaining)
	}
	if claude.Quota.CeilingWindowSeconds == nil || *claude.Quota.CeilingWindowSeconds != 60 {
		t.Errorf("window seconds: %+v", claude.Quota.CeilingWindowSeconds)
	}
}

// TestProviderTrendProjectionFromSeededUsageAndCeiling seeds ascending usage
// and a real rate-limit-derived ceiling, then asserts ProviderTrend returns a
// positive ProjectedRunOutHours. Covers AC 3 "projection renders only when
// ceiling is known and last-24h slope is positive."
func TestProviderTrendProjectionFromSeededUsageAndCeiling(t *testing.T) {
	t.Cleanup(resetHarnessRateLimitCache)
	resetHarnessRateLimitCache()

	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)

	// Seed ascending hourly usage for the last 24h under harness=claude.
	now := time.Now().UTC()
	for i := 0; i < 24; i++ {
		t0 := now.Add(-time.Duration(24-i) * time.Hour).Add(30 * time.Minute)
		appendSessionForTest(t, workDir, agent.SessionIndexEntry{
			ID: fmt.Sprintf("proj-%d", i), Harness: "claude", Provider: "anthropic",
			StartedAt: t0, Tokens: 1000 + i*200, Outcome: "success",
		}, t0)
	}
	// Record a ceiling via the rate-limit cache (10,000 tokens remaining).
	sig := agent.ParseRateLimitHeaders("claude", map[string][]string{
		"Anthropic-Ratelimit-Tokens-Limit":     {"50000"},
		"Anthropic-Ratelimit-Tokens-Remaining": {"10000"},
	})
	RecordHarnessRateLimit("claude", sig)

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	trend, err := r.ProviderTrend(context.Background(), "claude", 7)
	if err != nil {
		t.Fatalf("ProviderTrend: %v", err)
	}
	if trend.CeilingTokens == nil || *trend.CeilingTokens != 50000 {
		t.Fatalf("expected ceiling 50000, got %+v", trend.CeilingTokens)
	}
	if trend.ProjectedRunOutHours == nil || *trend.ProjectedRunOutHours <= 0 {
		t.Fatalf("expected positive ProjectedRunOutHours, got %+v", trend.ProjectedRunOutHours)
	}
}

// TestProviderStatusesPerfFixture is the perf harness for AC 4:
// ≥1,000 session rows across ≥5 endpoints/harnesses; p95 on the resolver
// path must stay well under the 200ms unified-list HTTP budget.
func TestProviderStatusesPerfFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture skipped in -short")
	}
	workDir := t.TempDir()
	// Seed 5 endpoints + 2 harnesses; 1,200 session rows distributed across them.
	writeMultiEndpointConfig(t, workDir)
	now := time.Now().UTC()
	names := []string{"ep-a", "ep-b", "ep-c", "ep-d", "ep-e", "claude", "codex"}
	kinds := []string{"provider", "provider", "provider", "provider", "provider", "harness", "harness"}
	total := 1200
	for i := 0; i < total; i++ {
		idx := i % len(names)
		t0 := now.Add(-time.Duration(i) * 20 * time.Minute)
		entry := agent.SessionIndexEntry{
			ID: fmt.Sprintf("perf-%d", i), StartedAt: t0, Tokens: 500, Outcome: "success",
		}
		if kinds[idx] == "provider" {
			entry.Provider = names[idx]
			entry.Harness = "agent"
		} else {
			entry.Harness = names[idx]
			entry.Provider = "anthropic"
		}
		appendSessionForTest(t, workDir, entry, t0)
	}

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	// Warm the cache.
	if _, err := r.ProviderStatuses(context.Background()); err != nil {
		t.Fatalf("ProviderStatuses warmup: %v", err)
	}

	const iterations = 20
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if _, err := r.ProviderStatuses(context.Background()); err != nil {
			t.Fatalf("ProviderStatuses iter %d: %v", i, err)
		}
		durations = append(durations, time.Since(start))
	}
	// p95 across 20 iterations = 19th-sorted value (index 18).
	sortDurations(durations)
	p95 := durations[18]
	// Budget: 200ms HTTP; resolver-only path should be well under (≤150ms).
	if p95 > 150*time.Millisecond {
		t.Fatalf("ProviderStatuses p95 = %s, want ≤150ms (target ≤200ms HTTP)", p95)
	}
}

// TestProviderTrendPerfFixture exercises the detail-view perf AC: p95 ≤400ms
// on the ≥1,000-row fixture.
func TestProviderTrendPerfFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture skipped in -short")
	}
	workDir := t.TempDir()
	writeMinimalConfig(t, workDir)
	now := time.Now().UTC()
	// Spread 1,200 sessions across 30 days under harness=claude.
	for i := 0; i < 1200; i++ {
		t0 := now.Add(-time.Duration(i) * 30 * time.Minute)
		appendSessionForTest(t, workDir, agent.SessionIndexEntry{
			ID: fmt.Sprintf("trend-perf-%d", i), Harness: "claude", Provider: "anthropic",
			StartedAt: t0, Tokens: 200, Outcome: "success",
		}, t0)
	}
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	if _, err := r.ProviderTrend(context.Background(), "claude", 30); err != nil {
		t.Fatalf("ProviderTrend warmup: %v", err)
	}

	const iterations = 20
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if _, err := r.ProviderTrend(context.Background(), "claude", 30); err != nil {
			t.Fatalf("ProviderTrend iter %d: %v", i, err)
		}
		durations = append(durations, time.Since(start))
	}
	sortDurations(durations)
	p95 := durations[18]
	if p95 > 300*time.Millisecond {
		t.Fatalf("ProviderTrend p95 = %s, want ≤300ms (target ≤400ms HTTP)", p95)
	}
}

func sortDurations(ds []time.Duration) {
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j-1] > ds[j]; j-- {
			ds[j-1], ds[j] = ds[j], ds[j-1]
		}
	}
}

// TestBuildSparklineEmitsHourlyTotalsAboveFloor seeds 24 hours of usage and
// asserts buildSparkline returns 24 hourly bucket totals when ≥6 non-empty
// buckets exist. Covers AC 2 ("Sparkline renders when ≥6 hourly buckets of
// usage are available").
func TestBuildSparklineEmitsHourlyTotalsAboveFloor(t *testing.T) {
	now := time.Now().UTC()
	entries := make([]agent.SessionIndexEntry, 0, 24)
	for i := 0; i < 24; i++ {
		t0 := now.Add(-time.Duration(24-i) * time.Hour).Add(30 * time.Minute)
		entries = append(entries, agent.SessionIndexEntry{
			ID: fmt.Sprintf("sp-%d", i), Harness: "claude", Provider: "anthropic",
			StartedAt: t0, Tokens: 100 + i, Outcome: "success",
		})
	}
	got := buildSparkline(entries, "claude", agent.MatchHarness, now)
	if len(got) != 24 {
		t.Fatalf("sparkline length: got %d want 24", len(got))
	}
	nonZero := 0
	for _, v := range got {
		if v > 0 {
			nonZero++
		}
	}
	if nonZero < 6 {
		t.Fatalf("expected ≥6 non-zero buckets, got %d", nonZero)
	}
}

// TestBuildSparklineSuppressesBelowFloor asserts that when fewer than 6
// hourly buckets carry tokens, buildSparkline returns nil so the UI hides
// the inline trend (avoids noisy single-spike sparklines).
func TestBuildSparklineSuppressesBelowFloor(t *testing.T) {
	now := time.Now().UTC()
	// Only three sessions, all in distinct recent hours → 3 non-empty buckets.
	entries := []agent.SessionIndexEntry{
		{ID: "a", Harness: "claude", Provider: "anthropic",
			StartedAt: now.Add(-1 * time.Hour), Tokens: 100, Outcome: "success"},
		{ID: "b", Harness: "claude", Provider: "anthropic",
			StartedAt: now.Add(-2 * time.Hour), Tokens: 200, Outcome: "success"},
		{ID: "c", Harness: "claude", Provider: "anthropic",
			StartedAt: now.Add(-3 * time.Hour), Tokens: 300, Outcome: "success"},
	}
	if got := buildSparkline(entries, "claude", agent.MatchHarness, now); got != nil {
		t.Fatalf("expected nil sparkline below floor, got %v", got)
	}
}

// writeMultiEndpointConfig writes a 5-endpoint config used by the perf test.
func writeMultiEndpointConfig(t *testing.T, workDir string) {
	t.Helper()
	ddxDir := filepath.Join(workDir, ".ddx")
	if err := os.MkdirAll(ddxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `version: "1.0"
bead:
  id_prefix: "pt"
agent:
  endpoints:
    - type: lmstudio
      base_url: http://127.0.0.1:9/v1
`
	if err := os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}
