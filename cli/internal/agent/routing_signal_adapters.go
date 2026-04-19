package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	CurrentQuota QuotaSignal   `json:"current_quota"`
	RecentUsage  UsageSignal   `json:"recent_usage"`
	Account      *AccountInfo  `json:"account,omitempty"`
	QuotaWindows []QuotaWindow `json:"quota_windows,omitempty"`
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
	ObservedAt    time.Time     `json:"observed_at"`
	State         string        `json:"state"`
	UsedPercent   int           `json:"used_percent,omitempty"`
	WindowMinutes int           `json:"window_minutes,omitempty"`
	ResetsAt      string        `json:"resets_at,omitempty"`
	Basis         string        `json:"basis,omitempty"`
	Notes         string        `json:"notes,omitempty"`
	QuotaWindows  []QuotaWindow `json:"quota_windows,omitempty"`
	Account       *AccountInfo  `json:"account,omitempty"`
}

type codexEventEnvelope struct {
	Type    string            `json:"type"`
	Payload codexEventPayload `json:"payload"`
}

type codexEventPayload struct {
	Type       string              `json:"type"`
	Info       *codexPayloadInfo   `json:"info"`
	RateLimits *codexRateLimitsObj `json:"rate_limits"`
}

type codexPayloadInfo struct {
	TotalTokenUsage codexTokenCounts `json:"total_token_usage"`
}

type codexTokenCounts struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	TotalTokens       int `json:"total_tokens"`
}

type codexRateLimitsObj struct {
	LimitID   string           `json:"limit_id"`
	PlanType  string           `json:"plan_type"`
	Primary   *codexRateWindow `json:"primary"`
	Secondary *codexRateWindow `json:"secondary"`
}

type codexRateWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

// ReadCodexNativeSignals scans a native Codex session JSONL file for current
// quota/headroom and recent usage totals.
func ReadCodexNativeSignals(path string, now time.Time) (CodexNativeSignals, error) {
	stat, statErr := os.Stat(path)
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

	// Track latest token usage (cumulative, keep latest) and quota by limit_id.
	var latestUsage *codexTokenCounts
	quotaByLimitID := map[string]*codexRateLimitsObj{}

	if err := ForEachJSONL[codexEventEnvelope](path, func(env codexEventEnvelope) error {
		if env.Type != "event_msg" || env.Payload.Type != "token_count" {
			return nil
		}
		if env.Payload.Info != nil && env.Payload.Info.TotalTokenUsage.TotalTokens != 0 {
			tc := env.Payload.Info.TotalTokenUsage
			latestUsage = &tc
			signals.RecentUsage.SessionCount++
		}
		if rl := env.Payload.RateLimits; rl != nil && rl.LimitID != "" {
			copy := *rl
			quotaByLimitID[rl.LimitID] = &copy
		}
		return nil
	}); err != nil {
		return CodexNativeSignals{}, err
	}

	if latestUsage != nil {
		signals.RecentUsage.InputTokens = latestUsage.InputTokens
		signals.RecentUsage.CachedInputTokens = latestUsage.CachedInputTokens
		signals.RecentUsage.OutputTokens = latestUsage.OutputTokens
		signals.RecentUsage.TotalTokens = latestUsage.TotalTokens
	}

	// Build QuotaWindows from the map.
	var quotaWindows []QuotaWindow
	for limitID, rl := range quotaByLimitID {
		prefix := ""
		if limitID != "codex" && limitID != "" {
			prefix = limitID + "-"
		}
		if rl.Primary != nil {
			label := prefix + quotaWindowLabel(rl.Primary.WindowMinutes)
			qw := QuotaWindow{
				Name:          label,
				LimitID:       limitID,
				WindowMinutes: rl.Primary.WindowMinutes,
				UsedPercent:   rl.Primary.UsedPercent,
				ResetsAt:      formatResetTime(rl.Primary.ResetsAt, now),
				ResetsAtUnix:  rl.Primary.ResetsAt,
				State:         quotaStateFromUsedPercent(int(rl.Primary.UsedPercent + 0.5)),
			}
			quotaWindows = append(quotaWindows, qw)
		}
		if rl.Secondary != nil {
			label := prefix + quotaWindowLabel(rl.Secondary.WindowMinutes)
			qw := QuotaWindow{
				Name:          label,
				LimitID:       limitID,
				WindowMinutes: rl.Secondary.WindowMinutes,
				UsedPercent:   rl.Secondary.UsedPercent,
				ResetsAt:      formatResetTime(rl.Secondary.ResetsAt, now),
				ResetsAtUnix:  rl.Secondary.ResetsAt,
				State:         quotaStateFromUsedPercent(int(rl.Secondary.UsedPercent + 0.5)),
			}
			quotaWindows = append(quotaWindows, qw)
		}
	}
	signals.QuotaWindows = quotaWindows

	// Set CurrentQuota from the "codex" limit_id primary window.
	if rl, ok := quotaByLimitID["codex"]; ok && rl.Primary != nil {
		usedPercent := int(rl.Primary.UsedPercent + 0.5)
		signals.CurrentQuota.State = quotaStateFromUsedPercent(usedPercent)
		signals.CurrentQuota.UsedPercent = usedPercent
		signals.CurrentQuota.WindowMinutes = rl.Primary.WindowMinutes
		signals.CurrentQuota.ResetsAt = formatResetTime(rl.Primary.ResetsAt, now)
	} else {
		signals.CurrentQuota.State = "unknown"
		signals.CurrentQuota.Source.Notes = "no token_count.rate_limits snapshot found in native session jsonl"
	}

	return signals, nil
}

// quotaWindowLabel converts window_minutes to a human label.
func quotaWindowLabel(minutes int) string {
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case minutes%(60*24) == 0:
		return fmt.Sprintf("%dd", minutes/(60*24))
	default:
		return fmt.Sprintf("%dh", minutes/60)
	}
}

// formatResetTime converts a unix timestamp to a human-readable reset time.
// Uses local timezone. Shows time-only if same day, date+time otherwise.
func formatResetTime(unix int64, now time.Time) string {
	if unix == 0 {
		return ""
	}
	t := time.Unix(unix, 0).Local()
	nowLocal := now.Local()
	if t.Year() == nowLocal.Year() && t.YearDay() == nowLocal.YearDay() {
		return t.Format("3:04pm")
	}
	return t.Format("Jan 2 3:04pm")
}

// ReadCodexAccountInfo reads account metadata from the Codex auth.json file.
// It decodes the access token JWT payload without signature verification.
func ReadCodexAccountInfo(path string) (*AccountInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var authFile struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &authFile); err != nil {
		return nil, err
	}
	payload, err := decodeJWTPayload(authFile.Tokens.AccessToken)
	if err != nil {
		return nil, err
	}
	acct := &AccountInfo{}
	if profile, ok := payload["https://api.openai.com/profile"].(map[string]any); ok {
		if email, ok := profile["email"].(string); ok {
			acct.Email = email
		}
	}
	if auth, ok := payload["https://api.openai.com/auth"].(map[string]any); ok {
		if plan, ok := auth["chatgpt_plan_type"].(string); ok {
			acct.PlanType = plan
		}
		if orgs, ok := auth["organizations"].([]any); ok && len(orgs) > 0 {
			if org, ok := orgs[0].(map[string]any); ok {
				if title, ok := org["title"].(string); ok {
					acct.OrgName = title
				}
			}
		}
	}
	if acct.Email == "" && acct.PlanType == "" {
		return nil, fmt.Errorf("no account info found in JWT")
	}
	return acct, nil
}

// discoverCodexAuthPath returns the path to the Codex auth.json file.
func discoverCodexAuthPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	p := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// decodeJWTPayload decodes the payload section of a JWT without verification.
func decodeJWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("JWT payload decode: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("JWT payload unmarshal: %w", err)
	}
	return out, nil
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

// WriteClaudeFullQuotaSnapshot writes a full quota snapshot including all windows and account info.
// The primary window (index 0) is used to populate the top-level state fields for backwards compatibility.
func WriteClaudeFullQuotaSnapshot(path string, windows []QuotaWindow, acct *AccountInfo, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := claudeQuotaSnapshotFile{
		ObservedAt:   now.UTC(),
		Basis:        "tmux-usage",
		QuotaWindows: windows,
		Account:      acct,
	}
	if len(windows) > 0 {
		w := windows[0]
		payload.State = w.State
		payload.UsedPercent = int(w.UsedPercent + 0.5)
		payload.WindowMinutes = w.WindowMinutes
		payload.ResetsAt = w.ResetsAt
	}
	if payload.State == "" {
		payload.State = "ok"
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadClaudeFullSnapshot reads quota windows and account info from the snapshot file.
// Returns the ObservedAt time so callers can apply a TTL check.
func ReadClaudeFullSnapshot(path string, now time.Time) ([]QuotaWindow, *AccountInfo, time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	var snapshot claudeQuotaSnapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, nil, time.Time{}, err
	}
	if snapshot.ObservedAt.IsZero() {
		if stat, statErr := os.Stat(path); statErr == nil {
			snapshot.ObservedAt = stat.ModTime().UTC()
		}
	}
	return snapshot.QuotaWindows, snapshot.Account, snapshot.ObservedAt, nil
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
