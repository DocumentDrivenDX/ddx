package agent

import (
	"strings"
	"time"
)

// UsageWindowCounts is a rolling-window usage summary for one endpoint/harness,
// computed from SessionIndexEntry rows. A zero value indicates a window that
// was requested but contained no matching entries — callers decide whether to
// render "0" or "not reported".
type UsageWindowCounts struct {
	TokensLastHour   int
	TokensLast24h    int
	RequestsLastHour int
	RequestsLast24h  int
}

// UsageMatchKind selects which session field identifies the row.
type UsageMatchKind int

const (
	// MatchProvider matches SessionIndexEntry.Provider (endpoint providers).
	MatchProvider UsageMatchKind = iota
	// MatchHarness matches SessionIndexEntry.Harness (subprocess harnesses).
	MatchHarness
)

// AggregateUsageCounts walks a sorted-newest-first slice of SessionIndexEntry
// and returns rolling-window totals for the given name. Matching is
// case-insensitive; empty name returns a zero struct.
func AggregateUsageCounts(entries []SessionIndexEntry, name string, kind UsageMatchKind, now time.Time) UsageWindowCounts {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return UsageWindowCounts{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var out UsageWindowCounts
	cutoffHour := now.Add(-time.Hour)
	cutoff24h := now.Add(-24 * time.Hour)
	for _, e := range entries {
		if !matchesUsageName(e, name, kind) {
			continue
		}
		if e.StartedAt.Before(cutoff24h) {
			continue
		}
		tokens := sessionTokens(e)
		out.TokensLast24h += tokens
		out.RequestsLast24h++
		if !e.StartedAt.Before(cutoffHour) {
			out.TokensLastHour += tokens
			out.RequestsLastHour++
		}
	}
	return out
}

func matchesUsageName(e SessionIndexEntry, lowerName string, kind UsageMatchKind) bool {
	switch kind {
	case MatchProvider:
		return strings.ToLower(strings.TrimSpace(e.Provider)) == lowerName
	case MatchHarness:
		return strings.ToLower(strings.TrimSpace(e.Harness)) == lowerName
	}
	return false
}

func sessionTokens(e SessionIndexEntry) int {
	if e.Tokens > 0 {
		return e.Tokens
	}
	return e.InputTokens + e.OutputTokens
}

// UsageBucket is one time-bucketed aggregate: tokens and request count.
type UsageBucket struct {
	Start    time.Time
	Tokens   int
	Requests int
}

// BucketUsage returns equally-spaced buckets covering the windowDays, aligned
// to `bucketSize`, in ascending order. Buckets with no sessions are emitted as
// zero counts so the series is dense for charts.
func BucketUsage(entries []SessionIndexEntry, name string, kind UsageMatchKind, now time.Time, windowDays int, bucketSize time.Duration) []UsageBucket {
	if windowDays <= 0 {
		return nil
	}
	if bucketSize <= 0 {
		bucketSize = time.Hour
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	end := truncateToBucket(now, bucketSize).Add(bucketSize)
	start := end.Add(-time.Duration(windowDays) * 24 * time.Hour)
	bucketCount := int(end.Sub(start) / bucketSize)
	if bucketCount <= 0 {
		return nil
	}
	buckets := make([]UsageBucket, bucketCount)
	for i := range buckets {
		buckets[i].Start = start.Add(time.Duration(i) * bucketSize)
	}
	for _, e := range entries {
		if !matchesUsageName(e, name, kind) {
			continue
		}
		if e.StartedAt.Before(start) || !e.StartedAt.Before(end) {
			continue
		}
		idx := int(e.StartedAt.Sub(start) / bucketSize)
		if idx < 0 || idx >= bucketCount {
			continue
		}
		buckets[idx].Tokens += sessionTokens(e)
		buckets[idx].Requests++
	}
	return buckets
}

func truncateToBucket(t time.Time, d time.Duration) time.Time {
	t = t.UTC()
	if d <= 0 {
		return t
	}
	return t.Truncate(d)
}

// LinearSlope returns the least-squares slope (units per bucket) of a series
// of observations, or 0 when the series is too small to compute.
func LinearSlope(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	fn := float64(n)
	denom := fn*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (fn*sumXY - sumX*sumY) / denom
}
