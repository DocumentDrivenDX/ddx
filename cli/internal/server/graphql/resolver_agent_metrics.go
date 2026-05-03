// resolver_agent_metrics.go implements the agentMetrics(window, groupBy)
// query (Story 11). Aggregations are computed from the agentmetrics package
// (run-store first, .ddx/executions bundles fallback) and cached per
// (project, axis, window) keyed on a revision fingerprint of the underlying
// data sources. The fingerprint covers the run-store dir, the bundles dir,
// and the .ddx/beads-archive.jsonl file so the cache survives an ADR-004
// archive move (closed beads moved out of beads.jsonl into beads-archive
// flip the fingerprint and force a re-aggregate).
package graphql

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agentmetrics"
)

// agentMetricsCacheEntry is one cached aggregation. fingerprint is the
// revision string the entry was computed against; on lookup the resolver
// recomputes the current fingerprint and serves the entry only when they
// match.
type agentMetricsCacheEntry struct {
	fingerprint string
	result      *AgentMetricsResult
}

// agentMetricsCache is process-global. Multi-project servers key by the
// effective workingDir so two projects never collide.
var agentMetricsCache = struct {
	sync.Mutex
	byKey map[string]agentMetricsCacheEntry
}{byKey: map[string]agentMetricsCacheEntry{}}

// AgentMetrics is the resolver for the agentMetrics field.
func (r *queryResolver) AgentMetrics(ctx context.Context, window AgentMetricsWindow, groupBy AgentMetricsAxis) (*AgentMetricsResult, error) {
	if !window.IsValid() {
		return nil, fmt.Errorf("agentMetrics: invalid window %q", window)
	}
	if !groupBy.IsValid() {
		return nil, fmt.Errorf("agentMetrics: invalid groupBy %q", groupBy)
	}
	wd := r.workingDir(ctx)
	fp := agentMetricsFingerprint(wd)
	cacheKey := wd + "|" + string(window) + "|" + string(groupBy)

	agentMetricsCache.Lock()
	if cached, ok := agentMetricsCache.byKey[cacheKey]; ok && cached.fingerprint == fp {
		agentMetricsCache.Unlock()
		return cached.result, nil
	}
	agentMetricsCache.Unlock()

	attempts, err := agentmetrics.LoadAttempts(wd)
	if err != nil {
		return nil, fmt.Errorf("agentMetrics: load attempts: %w", err)
	}
	cutoff := time.Now().UTC().Add(-windowDuration(window))
	rows := buildAgentMetricsRows(attempts, cutoff, groupBy)
	result := &AgentMetricsResult{
		Window:   window,
		GroupBy:  groupBy,
		Revision: fp,
		Rows:     rows,
	}
	agentMetricsCache.Lock()
	agentMetricsCache.byKey[cacheKey] = agentMetricsCacheEntry{fingerprint: fp, result: result}
	agentMetricsCache.Unlock()
	return result, nil
}

// windowDuration maps the schema enum to a Duration. Falls back to the 7d
// default for unknown values (the IsValid guard above already rejects them).
func windowDuration(w AgentMetricsWindow) time.Duration {
	switch w {
	case AgentMetricsWindowW24h:
		return 24 * time.Hour
	case AgentMetricsWindowW30d:
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// buildAgentMetricsRows groups attempts by axis, applies the window cutoff,
// and computes per-bucket aggregates. Output is sorted by attempt count
// descending then key ascending so the UI gets a stable ordering.
func buildAgentMetricsRows(attempts []agentmetrics.Attempt, cutoff time.Time, axis AgentMetricsAxis) []*AgentMetricsRow {
	type acc struct {
		durations    []int
		costs        []float64
		inTokens     []int
		outTokens    []int
		successes    int
		totalCost    float64
		lastFinished time.Time
	}
	groups := map[string]*acc{}
	for _, a := range attempts {
		if !a.StartedAt.IsZero() && a.StartedAt.Before(cutoff) {
			continue
		}
		key := axisKey(a, axis)
		if key == "" {
			continue
		}
		g, ok := groups[key]
		if !ok {
			g = &acc{}
			groups[key] = g
		}
		g.durations = append(g.durations, a.DurationMS)
		g.costs = append(g.costs, a.CostUSD)
		g.inTokens = append(g.inTokens, a.InputTokens)
		g.outTokens = append(g.outTokens, a.OutputTokens)
		g.totalCost += a.CostUSD
		if a.Bucket.Successful() {
			g.successes++
		}
		ts := a.FinishedAt
		if ts.IsZero() {
			ts = a.StartedAt
		}
		if !ts.IsZero() && ts.After(g.lastFinished) {
			g.lastFinished = ts
		}
	}

	out := make([]*AgentMetricsRow, 0, len(groups))
	for key, g := range groups {
		attemptCount := len(g.durations)
		row := &AgentMetricsRow{
			Key:              key,
			Attempts:         attemptCount,
			Successes:        g.successes,
			SuccessRate:      ratio(g.successes, attemptCount),
			MeanDurationMs:   meanInt(g.durations),
			P50DurationMs:    percentileInt(g.durations, 50),
			P95DurationMs:    percentileInt(g.durations, 95),
			MeanCostUsd:      meanFloat(g.costs),
			MeanInputTokens:  meanInt(g.inTokens),
			MeanOutputTokens: meanInt(g.outTokens),
		}
		if g.successes > 0 {
			eff := g.totalCost / float64(g.successes)
			row.EffectiveCostPerSuccessUsd = &eff
		}
		if !g.lastFinished.IsZero() {
			s := g.lastFinished.UTC().Format(time.RFC3339)
			row.LastSeenAt = &s
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attempts != out[j].Attempts {
			return out[i].Attempts > out[j].Attempts
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// axisKey extracts the bucket label for one Attempt against one axis. Empty
// string means "skip this attempt for this axis" (e.g. a model-axis bucket
// for a result.json with no model field).
func axisKey(a agentmetrics.Attempt, axis AgentMetricsAxis) string {
	switch axis {
	case AgentMetricsAxisModel:
		return a.Model
	case AgentMetricsAxisHarness:
		return a.Harness
	case AgentMetricsAxisProvider:
		return a.Provider
	case AgentMetricsAxisTier:
		return a.Tier
	}
	return ""
}

func ratio(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func meanInt(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum int
	for _, v := range xs {
		sum += v
	}
	return float64(sum) / float64(len(xs))
}

func meanFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs))
}

// percentileInt returns the nearest-rank percentile of xs (0-100). For p50 on
// an even-length slice it picks the lower of the two middle values; that
// matches Go's standard nearest-rank choice and is sufficient for the
// operator-facing summary.
func percentileInt(xs []int, p int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int(nil), xs...)
	sort.Ints(sorted)
	rank := (p * len(sorted)) / 100
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// agentMetricsFingerprint produces a stable revision string for the
// agentmetrics inputs at workingDir. The hash inputs are: the .ddx/exec/runs
// directory listing (name + size + modtime), the .ddx/executions directory
// listing of result.json files, and the .ddx/beads-archive.jsonl file's
// stat. Any add/remove/modify in those sources flips the fingerprint and
// invalidates the cache. The fingerprint survives ADR-004 archive moves
// because the archive file is included.
func agentMetricsFingerprint(workingDir string) string {
	h := sha256.New()
	addDir(h, filepath.Join(workingDir, ".ddx", "exec", "runs"), "")
	addDir(h, filepath.Join(workingDir, ".ddx", "executions"), "result.json")
	addStat(h, filepath.Join(workingDir, ".ddx", "beads-archive.jsonl"))
	addStat(h, filepath.Join(workingDir, ".ddx", "beads.jsonl"))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func addDir(h interface{ Write([]byte) (int, error) }, dir, requireChild string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(writerOf(h), "missing:%s\n", dir)
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		full := filepath.Join(dir, name)
		if requireChild != "" {
			full = filepath.Join(full, requireChild)
		}
		addStat(h, full)
	}
}

func addStat(h interface{ Write([]byte) (int, error) }, path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(writerOf(h), "absent:%s\n", path)
		return
	}
	fmt.Fprintf(writerOf(h), "%s:%d:%d\n", path, info.Size(), info.ModTime().UnixNano())
}

// writerOf adapts an io.Writer-like to fmt.Fprintf. Kept tiny so the helpers
// above stay one-liners.
func writerOf(h interface{ Write([]byte) (int, error) }) interface{ Write([]byte) (int, error) } {
	return h
}
