package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	codexNativeSessionSourceKind  = "native-session-jsonl"
	claudeStatsCacheSourceKind    = "stats-cache"
	claudeQuotaSnapshotSourceKind = "quota-snapshot"
	claudeUnknownQuotaSourceKind  = "docs-only"
)

// SignalSourceMetadata captures where a provider-native routing signal came from
// and how fresh it is.
type SignalSourceMetadata struct {
	Provider   string    `json:"provider"`
	Kind       string    `json:"kind"`
	Path       string    `json:"path,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	Freshness  string    `json:"freshness"`
	AgeSeconds int64     `json:"age_seconds,omitempty"`
	Basis      string    `json:"basis,omitempty"`
	Notes      string    `json:"notes,omitempty"`
}

// QuotaSignal captures current quota/headroom from a provider-native source.
type QuotaSignal struct {
	Source        SignalSourceMetadata `json:"source"`
	State         string               `json:"state"`
	UsedPercent   int                  `json:"used_percent,omitempty"`
	WindowMinutes int                  `json:"window_minutes,omitempty"`
	ResetsAt      string               `json:"resets_at,omitempty"`
}

// UsageSignal captures token/session totals from a provider-native source.
type UsageSignal struct {
	Source            SignalSourceMetadata `json:"source"`
	InputTokens       int                  `json:"input_tokens,omitempty"`
	CachedInputTokens int                  `json:"cached_input_tokens,omitempty"`
	OutputTokens      int                  `json:"output_tokens,omitempty"`
	TotalTokens       int                  `json:"total_tokens,omitempty"`
	SessionCount      int                  `json:"session_count,omitempty"`
}

// CodexNativeSignals is the provider-native routing signal bundle read from a
// Codex native session JSONL file.
type CodexNativeSignals struct {
	CurrentQuota QuotaSignal `json:"current_quota"`
	RecentUsage  UsageSignal `json:"recent_usage"`
}

// ClaudeNativeSignals is the provider-native routing signal bundle read from
// Claude's stats-cache and docs-backed quota surface.
type ClaudeNativeSignals struct {
	CurrentQuota    QuotaSignal            `json:"current_quota"`
	HistoricalUsage UsageSignal            `json:"historical_usage"`
	ByModel         map[string]UsageSignal `json:"by_model,omitempty"`
	ByDay           map[string]UsageSignal `json:"by_day,omitempty"`
}

type claudeQuotaSnapshotFile struct {
	ObservedAt    time.Time `json:"observed_at"`
	State         string    `json:"state"`
	UsedPercent   int       `json:"used_percent,omitempty"`
	WindowMinutes int       `json:"window_minutes,omitempty"`
	ResetsAt      string    `json:"resets_at,omitempty"`
	Basis         string    `json:"basis,omitempty"`
	Notes         string    `json:"notes,omitempty"`
}

type codexSessionEvent struct {
	Type  string `json:"type"`
	Usage struct {
		InputTokens       int `json:"input_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
		OutputTokens      int `json:"output_tokens"`
	} `json:"usage"`
	TokenCount struct {
		RateLimits map[string]codexRateLimit `json:"rate_limits"`
	} `json:"token_count"`
	RateLimits map[string]codexRateLimit `json:"rate_limits"`
}

type codexRateLimit struct {
	UsedPercent   int    `json:"used_percent"`
	WindowMinutes int    `json:"window_minutes"`
	ResetsAt      string `json:"resets_at"`
}

// ReadCodexNativeSignals scans a native Codex session JSONL file for current
// quota/headroom and recent usage totals.
func ReadCodexNativeSignals(path string, now time.Time) (CodexNativeSignals, error) {
	f, err := os.Open(path)
	if err != nil {
		return CodexNativeSignals{}, err
	}
	defer f.Close()

	stat, statErr := f.Stat()
	meta := SignalSourceMetadata{
		Provider:  "codex",
		Kind:      codexNativeSessionSourceKind,
		Path:      path,
		Freshness: "fresh",
		Basis:     "native session jsonl",
	}
	if statErr == nil {
		meta.ObservedAt = stat.ModTime().UTC()
		if !now.IsZero() {
			if age := now.UTC().Sub(meta.ObservedAt); age > 0 {
				meta.AgeSeconds = int64(age.Seconds())
			}
		}
	}

	signals := CodexNativeSignals{
		CurrentQuota: QuotaSignal{
			Source: SignalSourceMetadata{
				Provider:   "codex",
				Kind:       codexNativeSessionSourceKind,
				Path:       path,
				Freshness:  meta.Freshness,
				Basis:      meta.Basis,
				ObservedAt: meta.ObservedAt,
				AgeSeconds: meta.AgeSeconds,
			},
			State: "unknown",
		},
		RecentUsage: UsageSignal{
			Source: SignalSourceMetadata{
				Provider:   "codex",
				Kind:       codexNativeSessionSourceKind,
				Path:       path,
				Freshness:  meta.Freshness,
				Basis:      meta.Basis,
				ObservedAt: meta.ObservedAt,
				AgeSeconds: meta.AgeSeconds,
			},
		},
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	foundQuota := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event codexSessionEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.Type == "turn.completed" {
			signals.RecentUsage.InputTokens += event.Usage.InputTokens
			signals.RecentUsage.CachedInputTokens += event.Usage.CachedInputTokens
			signals.RecentUsage.OutputTokens += event.Usage.OutputTokens
			signals.RecentUsage.SessionCount++
		}

		if quota, ok := extractCodexRateLimit(event); ok {
			signals.CurrentQuota.State = quotaStateFromUsedPercent(quota.UsedPercent)
			signals.CurrentQuota.UsedPercent = quota.UsedPercent
			signals.CurrentQuota.WindowMinutes = quota.WindowMinutes
			signals.CurrentQuota.ResetsAt = quota.ResetsAt
			foundQuota = true
		}
	}
	if err := scanner.Err(); err != nil {
		return CodexNativeSignals{}, err
	}

	signals.RecentUsage.TotalTokens = signals.RecentUsage.InputTokens + signals.RecentUsage.CachedInputTokens + signals.RecentUsage.OutputTokens
	if !foundQuota {
		signals.CurrentQuota.State = "unknown"
		signals.CurrentQuota.Source.Notes = "no token_count.rate_limits snapshot found in native session jsonl"
	}
	return signals, nil
}

func extractCodexRateLimit(event codexSessionEvent) (codexRateLimit, bool) {
	if quota, ok := pickCodexRateLimit(event.TokenCount.RateLimits); ok {
		return quota, true
	}
	if quota, ok := pickCodexRateLimit(event.RateLimits); ok {
		return quota, true
	}
	return codexRateLimit{}, false
}

func pickCodexRateLimit(rateLimits map[string]codexRateLimit) (codexRateLimit, bool) {
	if len(rateLimits) == 0 {
		return codexRateLimit{}, false
	}
	if quota, ok := rateLimits["primary"]; ok {
		return quota, true
	}
	keys := make([]string, 0, len(rateLimits))
	for key := range rateLimits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return rateLimits[keys[0]], true
}

// ReadClaudeNativeSignals reads Claude's stats-cache.json for historical usage
// and returns an explicit unknown current-quota signal when no stable non-PTY
// source is documented.
func ReadClaudeNativeSignals(path string, now time.Time) (ClaudeNativeSignals, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ClaudeNativeSignals{}, err
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return ClaudeNativeSignals{}, err
	}

	stat, statErr := os.Stat(path)
	meta := SignalSourceMetadata{
		Provider:  "claude",
		Kind:      claudeStatsCacheSourceKind,
		Path:      path,
		Freshness: "cached",
		Basis:     "stats-cache.json",
	}
	if statErr == nil {
		meta.ObservedAt = stat.ModTime().UTC()
		if !now.IsZero() {
			if age := now.UTC().Sub(meta.ObservedAt); age > 0 {
				meta.AgeSeconds = int64(age.Seconds())
			}
		}
	}

	signals := ClaudeNativeSignals{
		CurrentQuota: QuotaSignal{
			Source: SignalSourceMetadata{
				Provider:  "claude",
				Kind:      claudeUnknownQuotaSourceKind,
				Freshness: "unknown",
				Basis:     "no stable non-PTY current-quota source documented in repo/docs",
				Notes:     "current quota/headroom remains unknown",
			},
			State: "unknown",
		},
		HistoricalUsage: UsageSignal{
			Source: SignalSourceMetadata{
				Provider:   "claude",
				Kind:       claudeStatsCacheSourceKind,
				Path:       path,
				Freshness:  meta.Freshness,
				Basis:      meta.Basis,
				ObservedAt: meta.ObservedAt,
				AgeSeconds: meta.AgeSeconds,
			},
		},
		ByModel: map[string]UsageSignal{},
		ByDay:   map[string]UsageSignal{},
	}

	if raw, ok := root["dailyActivity"]; ok {
		signals.ByDay = parseClaudeUsageCollection(raw)
		for _, usage := range signals.ByDay {
			signals.HistoricalUsage = mergeUsageSignals(signals.HistoricalUsage, usage)
		}
	}

	modelUsageKey := ""
	if _, ok := root["modelUsage"]; ok {
		modelUsageKey = "modelUsage"
	} else if _, ok := root["cumulativeModelUsage"]; ok {
		modelUsageKey = "cumulativeModelUsage"
	}
	if modelUsageKey != "" {
		signals.ByModel = parseClaudeUsageMap(root[modelUsageKey])
		signals.HistoricalUsage = aggregateUsageSignals(signals.ByModel, signals.HistoricalUsage.Source)
	}

	if len(signals.ByModel) == 0 && len(signals.ByDay) > 0 {
		signals.HistoricalUsage = aggregateUsageSignals(signals.ByDay, signals.HistoricalUsage.Source)
	}
	if signals.HistoricalUsage.TotalTokens == 0 {
		signals.HistoricalUsage.TotalTokens = signals.HistoricalUsage.InputTokens + signals.HistoricalUsage.CachedInputTokens + signals.HistoricalUsage.OutputTokens
	}
	return signals, nil
}

func ReadClaudeQuotaSnapshot(path string, now time.Time) (QuotaSignal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return QuotaSignal{}, err
	}

	var snapshot claudeQuotaSnapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return QuotaSignal{}, err
	}
	if snapshot.ObservedAt.IsZero() {
		if stat, statErr := os.Stat(path); statErr == nil {
			snapshot.ObservedAt = stat.ModTime().UTC()
		}
	}

	age := now.UTC().Sub(snapshot.ObservedAt.UTC())
	if snapshot.ObservedAt.IsZero() || age < 0 {
		age = 0
	}

	return QuotaSignal{
		Source: SignalSourceMetadata{
			Provider:   "claude",
			Kind:       claudeQuotaSnapshotSourceKind,
			Path:       path,
			ObservedAt: snapshot.ObservedAt.UTC(),
			Freshness:  freshnessForAge(age),
			AgeSeconds: int64(age.Seconds()),
			Basis:      coalesce(snapshot.Basis, "durable async quota snapshot"),
			Notes:      snapshot.Notes,
		},
		State:         coalesce(snapshot.State, "unknown"),
		UsedPercent:   snapshot.UsedPercent,
		WindowMinutes: snapshot.WindowMinutes,
		ResetsAt:      snapshot.ResetsAt,
	}, nil
}

func WriteClaudeQuotaSnapshot(path string, signal QuotaSignal) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := claudeQuotaSnapshotFile{
		ObservedAt:    signal.Source.ObservedAt,
		State:         signal.State,
		UsedPercent:   signal.UsedPercent,
		WindowMinutes: signal.WindowMinutes,
		ResetsAt:      signal.ResetsAt,
		Basis:         signal.Source.Basis,
		Notes:         signal.Source.Notes,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseClaudeUsageCollection(raw any) map[string]UsageSignal {
	out := map[string]UsageSignal{}
	items, ok := raw.([]any)
	if !ok {
		return out
	}
	for idx, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := firstString(m, "date", "day", "timestamp", "time")
		if key == "" {
			key = strconv.Itoa(idx)
		}
		out[key] = usageSignalFromMap(m)
	}
	return out
}

func parseClaudeUsageMap(raw any) map[string]UsageSignal {
	out := map[string]UsageSignal{}
	m, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range m {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		out[key] = usageSignalFromMap(item)
	}
	return out
}

func usageSignalFromMap(m map[string]any) UsageSignal {
	usage := UsageSignal{
		InputTokens:       firstInt(m, "inputTokens", "input_tokens", "input"),
		CachedInputTokens: firstInt(m, "cachedInputTokens", "cached_input_tokens", "cacheReadInputTokens", "cache_read_input_tokens", "cacheCreationInputTokens", "cache_creation_input_tokens"),
		OutputTokens:      firstInt(m, "outputTokens", "output_tokens", "output"),
		TotalTokens:       firstInt(m, "totalTokens", "total_tokens", "tokens", "total"),
		SessionCount:      firstInt(m, "sessionCount", "session_count", "sessions", "count"),
	}
	usage = mergeUsageSignals(usage, usageSignalFromNestedMap(m, "usage"))
	usage = mergeUsageSignals(usage, usageSignalFromNestedMap(m, "tokens"))
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.CachedInputTokens + usage.OutputTokens
	}
	return usage
}

func usageSignalFromNestedMap(m map[string]any, key string) UsageSignal {
	nested, ok := m[key].(map[string]any)
	if !ok {
		return UsageSignal{}
	}
	return usageSignalFromMap(nested)
}

func mergeUsageSignals(dst, src UsageSignal) UsageSignal {
	if dst.InputTokens == 0 {
		dst.InputTokens = src.InputTokens
	}
	if dst.CachedInputTokens == 0 {
		dst.CachedInputTokens = src.CachedInputTokens
	}
	if dst.OutputTokens == 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if dst.TotalTokens == 0 {
		dst.TotalTokens = src.TotalTokens
	}
	if dst.SessionCount == 0 {
		dst.SessionCount = src.SessionCount
	}
	return dst
}

func aggregateUsageSignals(items map[string]UsageSignal, source SignalSourceMetadata) UsageSignal {
	out := UsageSignal{Source: source}
	for _, usage := range items {
		out.InputTokens += usage.InputTokens
		out.CachedInputTokens += usage.CachedInputTokens
		out.OutputTokens += usage.OutputTokens
		out.TotalTokens += usage.TotalTokens
		out.SessionCount += usage.SessionCount
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.CachedInputTokens + out.OutputTokens
	}
	return out
}

func firstInt(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if n, ok := asInt(v); ok {
				return n
			}
		}
	}
	return 0
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i), true
		}
		if f, err := strconv.ParseFloat(string(n), 64); err == nil {
			return int(f), true
		}
		return 0, false
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err == nil {
			return i, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func quotaStateFromUsedPercent(usedPercent int) string {
	if usedPercent >= 95 {
		return "blocked"
	}
	if usedPercent >= 0 {
		return "ok"
	}
	return "unknown"
}
