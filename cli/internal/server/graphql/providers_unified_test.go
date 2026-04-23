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
	writeMinimalConfig(t, workDir)

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	statuses, err := r.ProviderStatuses(context.Background())
	if err != nil {
		t.Fatalf("ProviderStatuses: %v", err)
	}
	// Even when zero endpoints are configured, the resolver must succeed.
	for _, s := range statuses {
		if s.Kind != ProviderKindEndpoint {
			t.Errorf("endpoint row %q: got kind %q want ENDPOINT", s.Name, s.Kind)
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
	writeMinimalConfig(t, workDir)

	now := time.Now().UTC()
	// 3 sessions for harness=claude in last hour, 1 more from 5h ago.
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s1", Harness: "claude", Provider: "anthropic", Model: "m",
		StartedAt: now.Add(-10 * time.Minute), Tokens: 1000, Outcome: "success",
	}, now.Add(-10*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s2", Harness: "claude", Provider: "anthropic", Model: "m",
		StartedAt: now.Add(-30 * time.Minute), Tokens: 2000, Outcome: "success",
	}, now.Add(-30*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s3", Harness: "claude", Provider: "anthropic", Model: "m",
		StartedAt: now.Add(-55 * time.Minute), Tokens: 500, Outcome: "success",
	}, now.Add(-55*time.Minute))
	appendSessionForTest(t, workDir, agent.SessionIndexEntry{
		ID: "s4", Harness: "claude", Provider: "anthropic", Model: "m",
		StartedAt: now.Add(-5 * time.Hour), Tokens: 400, Outcome: "success",
	}, now.Add(-5*time.Hour))

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
		t.Fatal("expected claude harness row")
	}
	if claude.Usage == nil {
		t.Fatal("expected usage populated from session index")
	}
	if got := deref(claude.Usage.TokensUsedLastHour); got != 3500 {
		t.Errorf("tokensUsedLastHour: got %d want 3500", got)
	}
	if got := deref(claude.Usage.TokensUsedLast24h); got != 3900 {
		t.Errorf("tokensUsedLast24h: got %d want 3900", got)
	}
	if got := deref(claude.Usage.RequestsLastHour); got != 3 {
		t.Errorf("requestsLastHour: got %d want 3", got)
	}
	if got := deref(claude.Usage.RequestsLast24h); got != 4 {
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
