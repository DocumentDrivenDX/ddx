package agent

import (
	"fmt"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
}

func makeUsageEntry(started time.Time, provider, harness string, tokens int) SessionIndexEntry {
	return SessionIndexEntry{
		StartedAt: started,
		Provider:  provider,
		Harness:   harness,
		Tokens:    tokens,
	}
}

func TestAggregateUsageCounts_ProviderWindow(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		makeUsageEntry(now.Add(-30*time.Minute), "qwen", "agent", 100),
		makeUsageEntry(now.Add(-2*time.Hour), "qwen", "agent", 200),
		makeUsageEntry(now.Add(-25*time.Hour), "qwen", "agent", 999), // outside 24h
		makeUsageEntry(now.Add(-30*time.Minute), "other", "agent", 50),
	}
	c := AggregateUsageCounts(entries, "qwen", MatchProvider, now)
	if c.TokensLastHour != 100 {
		t.Errorf("tokens last hour: got %d want 100", c.TokensLastHour)
	}
	if c.TokensLast24h != 300 {
		t.Errorf("tokens last 24h: got %d want 300", c.TokensLast24h)
	}
	if c.RequestsLastHour != 1 {
		t.Errorf("requests last hour: got %d want 1", c.RequestsLastHour)
	}
	if c.RequestsLast24h != 2 {
		t.Errorf("requests last 24h: got %d want 2", c.RequestsLast24h)
	}
}

func TestAggregateUsageCounts_HarnessWindow(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		makeUsageEntry(now.Add(-15*time.Minute), "", "claude", 500),
		makeUsageEntry(now.Add(-5*time.Hour), "", "claude", 800),
		makeUsageEntry(now.Add(-15*time.Minute), "", "codex", 100),
	}
	c := AggregateUsageCounts(entries, "claude", MatchHarness, now)
	if c.TokensLastHour != 500 || c.TokensLast24h != 1300 {
		t.Errorf("claude aggregate mismatch: %+v", c)
	}
}

func TestAggregateUsageCounts_FallBackToInputOutput(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		{StartedAt: now.Add(-30 * time.Minute), Harness: "claude", InputTokens: 400, OutputTokens: 100},
	}
	c := AggregateUsageCounts(entries, "claude", MatchHarness, now)
	if c.TokensLastHour != 500 {
		t.Errorf("fallback sum tokens: got %d want 500", c.TokensLastHour)
	}
}

func TestAggregateUsageCounts_CaseInsensitive(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		{StartedAt: now.Add(-1 * time.Minute), Harness: "CLAUDE", Tokens: 42},
	}
	c := AggregateUsageCounts(entries, "claude", MatchHarness, now)
	if c.TokensLastHour != 42 {
		t.Errorf("case-insensitive match: %+v", c)
	}
}

func TestBucketUsage_HourlyBuckets(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		makeUsageEntry(now.Add(-30*time.Minute), "qwen", "", 100),
		makeUsageEntry(now.Add(-90*time.Minute), "qwen", "", 50),
		makeUsageEntry(now.Add(-30*time.Minute), "other", "", 10),
	}
	buckets := BucketUsage(entries, "qwen", MatchProvider, now, 1, time.Hour)
	if len(buckets) != 24 {
		t.Fatalf("expected 24 hourly buckets over 1 day, got %d", len(buckets))
	}
	var total int
	for _, b := range buckets {
		total += b.Tokens
	}
	if total != 150 {
		t.Errorf("total across buckets: got %d want 150", total)
	}
	// Ordering ascending
	for i := 1; i < len(buckets); i++ {
		if !buckets[i].Start.After(buckets[i-1].Start) {
			t.Errorf("buckets not ascending at %d", i)
		}
	}
}

func TestBucketUsage_4HourBuckets(t *testing.T) {
	now := fixedNow()
	entries := []SessionIndexEntry{
		makeUsageEntry(now.Add(-1*time.Hour), "qwen", "", 10),
		makeUsageEntry(now.Add(-5*time.Hour), "qwen", "", 20),
	}
	buckets := BucketUsage(entries, "qwen", MatchProvider, now, 2, 4*time.Hour)
	if len(buckets) != 12 {
		t.Fatalf("expected 12 4h buckets over 2 days, got %d", len(buckets))
	}
}

func TestLinearSlope(t *testing.T) {
	// Ascending series: slope should be positive.
	if s := LinearSlope([]float64{10, 20, 30, 40}); s != 10 {
		t.Errorf("perfect line slope: got %f want 10", s)
	}
	// Flat series: slope 0.
	if s := LinearSlope([]float64{5, 5, 5, 5}); s != 0 {
		t.Errorf("flat slope: got %f want 0", s)
	}
	// Too short.
	if s := LinearSlope([]float64{42}); s != 0 {
		t.Errorf("single point slope: got %f want 0", s)
	}
}

func TestAggregateUsageCounts_PerfOneThousandRows(t *testing.T) {
	// Per standing directive in ddx-9ce6842a: fixture of ≥1k session rows
	// covering last 24h; aggregation must return correct counts and stay
	// well under the 200ms HTTP budget.
	now := fixedNow()
	entries := make([]SessionIndexEntry, 0, 1000)
	for i := 0; i < 1000; i++ {
		minutes := time.Duration(i) * time.Minute
		entries = append(entries, SessionIndexEntry{
			StartedAt: now.Add(-minutes),
			Provider:  fmt.Sprintf("p%d", i%5),
			Harness:   fmt.Sprintf("h%d", i%5),
			Tokens:    10,
		})
	}
	start := time.Now()
	c := AggregateUsageCounts(entries, "p0", MatchProvider, now)
	elapsed := time.Since(start)
	// 200 sessions per provider per 1000/5 over ~17 hours → 60 in the last hour, 200 in last 24h.
	if c.RequestsLast24h != 200 {
		t.Errorf("expected 200 requests in 24h for p0, got %d", c.RequestsLast24h)
	}
	if c.TokensLast24h != 2000 {
		t.Errorf("expected 2000 tokens, got %d", c.TokensLast24h)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("aggregation too slow on 1k fixture: %v (budget 50ms)", elapsed)
	}
}
