package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	codexNativeSessionEnv  = "DDX_CODEX_NATIVE_SESSION_JSONL"
	claudeStatsCacheEnv    = "DDX_CLAUDE_STATS_CACHE"
	claudeQuotaSnapshotEnv = "DDX_CLAUDE_QUOTA_SNAPSHOT"
	sessionLogSourceKind   = "recent-session-log"
)

// LoadRoutingSignalSnapshot returns the provider-native routing signal snapshot
// for a harness. Missing sources are surfaced as an explicit unknown snapshot
// so callers can distinguish unavailable from blocked.
func (r *Runner) LoadRoutingSignalSnapshot(harnessName string, now time.Time) RoutingSignalSnapshot {
	var snapshot RoutingSignalSnapshot
	switch harnessName {
	case "codex":
		snapshot = r.loadCodexRoutingSignal(now)
	case "claude":
		snapshot = r.loadClaudeRoutingSignal(now)
	case "openrouter":
		return ProbeOpenRouterBalance(8 * time.Second)
	case "lmstudio":
		return r.loadLMStudioSignal(now)
	default:
		return RoutingSignalSnapshot{}
	}
	return r.overlayRecentQuotaBlock(harnessName, snapshot, now)
}

func (r *Runner) loadCodexRoutingSignal(now time.Time) RoutingSignalSnapshot {
	path := os.Getenv(codexNativeSessionEnv)
	if path == "" {
		path = discoverCodexNativeSessionPath()
	}
	if path == "" {
		return RoutingSignalSnapshot{
			Provider: "codex",
			Source: SignalSourceMetadata{
				Provider:  "codex",
				Kind:      codexNativeSessionSourceKind,
				Freshness: "unknown",
				Basis:     "session path unavailable",
				Notes:     codexNativeSessionEnv + " is not set and no native session file was discovered",
			},
			CurrentQuota: QuotaSignal{
				Source: SignalSourceMetadata{
					Provider:  "codex",
					Kind:      codexNativeSessionSourceKind,
					Freshness: "unknown",
					Basis:     "session path unavailable",
					Notes:     codexNativeSessionEnv + " is not set and no native session file was discovered",
				},
				State: "unknown",
			},
		}
	}

	signals, err := ReadCodexNativeSignals(path, now)
	if err != nil {
		return RoutingSignalSnapshot{
			Provider: "codex",
			Source: SignalSourceMetadata{
				Provider:  "codex",
				Kind:      codexNativeSessionSourceKind,
				Path:      path,
				Freshness: "unknown",
				Basis:     "native session jsonl",
				Notes:     err.Error(),
			},
			CurrentQuota: QuotaSignal{
				Source: SignalSourceMetadata{
					Provider:  "codex",
					Kind:      codexNativeSessionSourceKind,
					Path:      path,
					Freshness: "unknown",
					Basis:     "native session jsonl",
					Notes:     err.Error(),
				},
				State: "unknown",
			},
		}
	}

	snapshot := RoutingSignalSnapshot{
		Provider:        "codex",
		Source:          signals.CurrentQuota.Source,
		CurrentQuota:    signals.CurrentQuota,
		HistoricalUsage: signals.RecentUsage,
		QuotaWindows:    signals.QuotaWindows,
	}
	if authPath := discoverCodexAuthPath(); authPath != "" {
		if acct, err := ReadCodexAccountInfo(authPath); err == nil {
			snapshot.Account = acct
		}
	}

	// Promote worst window state to CurrentQuota so routing sees a blocked weekly as blocked overall.
	// Skip "extra" usage windows — those are overflow buckets, not hard routing blockers.
	for _, w := range snapshot.QuotaWindows {
		if w.LimitID == "extra" {
			continue
		}
		if w.State == "blocked" && snapshot.CurrentQuota.State != "blocked" {
			snapshot.CurrentQuota.State = "blocked"
			snapshot.CurrentQuota.ResetsAt = w.ResetsAt
		}
	}

	// Overlay codex TUI snapshot if native JSONL quota is unknown.
	if snapshot.CurrentQuota.State == "unknown" {
		if snapshotPath := resolveCodexQuotaSnapshotPath(); snapshotPath != "" {
			if windows, _, _, err := ReadClaudeFullSnapshot(snapshotPath, now); err == nil && len(windows) > 0 {
				snapshot.QuotaWindows = windows
				// Use first window as current quota.
				w := windows[0]
				snapshot.CurrentQuota.State = w.State
				snapshot.CurrentQuota.UsedPercent = int(w.UsedPercent + 0.5)
				snapshot.CurrentQuota.WindowMinutes = w.WindowMinutes
				snapshot.CurrentQuota.ResetsAt = w.ResetsAt
			}
		}
	}

	return snapshot
}

func discoverCodexNativeSessionPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	root := filepath.Join(home, ".codex", "sessions")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ""
	}

	type candidate struct {
		path    string
		modTime time.Time
	}
	var candidates []candidate
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		candidates = append(candidates, candidate{path: path, modTime: info.ModTime().UTC()})
		return nil
	})
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates[0].path
}

func (r *Runner) loadClaudeRoutingSignal(now time.Time) RoutingSignalSnapshot {
	path := os.Getenv(claudeStatsCacheEnv)
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, ".claude", "stats-cache.json")
		}
	}
	if path == "" {
		return RoutingSignalSnapshot{
			Provider: "claude",
			Source: SignalSourceMetadata{
				Provider:  "claude",
				Kind:      claudeUnknownQuotaSourceKind,
				Freshness: "unknown",
				Basis:     "stats-cache path unavailable",
				Notes:     "unable to resolve stats-cache path",
			},
			CurrentQuota: QuotaSignal{
				Source: SignalSourceMetadata{
					Provider:  "claude",
					Kind:      claudeUnknownQuotaSourceKind,
					Freshness: "unknown",
					Basis:     "stats-cache path unavailable",
					Notes:     "unable to resolve stats-cache path",
				},
				State: "unknown",
			},
		}
	}

	signals, err := ReadClaudeNativeSignals(path, now)
	if err != nil {
		return RoutingSignalSnapshot{
			Provider: "claude",
			Source: SignalSourceMetadata{
				Provider:  "claude",
				Kind:      claudeStatsCacheSourceKind,
				Path:      path,
				Freshness: "unknown",
				Basis:     "stats-cache.json",
				Notes:     err.Error(),
			},
			CurrentQuota: QuotaSignal{
				Source: SignalSourceMetadata{
					Provider:  "claude",
					Kind:      claudeStatsCacheSourceKind,
					Path:      path,
					Freshness: "unknown",
					Basis:     "stats-cache.json",
					Notes:     err.Error(),
				},
				State: "unknown",
			},
		}
	}

	result := RoutingSignalSnapshot{
		Provider:        "claude",
		Source:          signals.HistoricalUsage.Source,
		CurrentQuota:    signals.CurrentQuota,
		HistoricalUsage: signals.HistoricalUsage,
	}

	if snapshotPath := resolveClaudeQuotaSnapshotPath(); snapshotPath != "" {
		if quota, quotaErr := ReadClaudeQuotaSnapshot(snapshotPath, now); quotaErr == nil {
			result.CurrentQuota = quota
		}
		if windows, acct, _, err := ReadClaudeFullSnapshot(snapshotPath, now); err == nil {
			if len(windows) > 0 {
				result.QuotaWindows = windows
			}
			if acct != nil {
				result.Account = acct
			}
		}
	}

	// Promote worst window state to CurrentQuota.
	// Skip "extra" usage windows — those are overflow buckets, not hard routing blockers.
	for _, w := range result.QuotaWindows {
		if w.LimitID == "extra" {
			continue
		}
		if w.State == "blocked" && result.CurrentQuota.State != "blocked" {
			result.CurrentQuota.State = "blocked"
			result.CurrentQuota.ResetsAt = w.ResetsAt
		}
	}

	return result
}

// loadLMStudioSignal probes all LM Studio endpoints from ~/.config/agent/config.yaml.
func (r *Runner) loadLMStudioSignal(now time.Time) RoutingSignalSnapshot {
	unknown := RoutingSignalSnapshot{
		Provider: "lmstudio",
		Source: SignalSourceMetadata{
			Provider:  "lmstudio",
			Kind:      lmstudioSourceKind,
			Freshness: "unknown",
			Notes:     "no LM Studio endpoints configured in ~/.config/agent/config.yaml",
		},
		CurrentQuota: QuotaSignal{
			Source: SignalSourceMetadata{
				Provider:  "lmstudio",
				Kind:      lmstudioSourceKind,
				Freshness: "unknown",
			},
			State: "unknown",
		},
	}

	// Read LM Studio endpoints from native ~/.config/agent/config.yaml.
	allEndpoints := ReadLMStudioEndpointsFromAgentConfig()
	if len(allEndpoints) == 0 {
		return unknown
	}

	statuses := ProbeLMStudioEndpoints(allEndpoints, 3*time.Second)
	return BuildLMStudioSignal("agent-config", statuses, now)
}

// RefreshCodexQuotaViaTmux refreshes the codex quota snapshot via tmux if the
// existing snapshot is older than maxAge (or native JSONL is unavailable).
// Safe to call from slow-path commands (ddx agent doctor), not from routing hot paths.
func (r *Runner) RefreshCodexQuotaViaTmux(now time.Time, maxAge time.Duration) error {
	snapshotPath := resolveCodexQuotaSnapshotPath()
	if snapshotPath == "" {
		return fmt.Errorf("cannot resolve codex quota snapshot path")
	}
	// Skip if fresh.
	if _, _, observedAt, err := ReadClaudeFullSnapshot(snapshotPath, now); err == nil {
		if !observedAt.IsZero() && now.UTC().Sub(observedAt.UTC()) < maxAge {
			return nil
		}
	}

	windows, err := ReadCodexQuotaViaTmux(30 * time.Second)
	if err != nil {
		return err
	}
	// Use WriteClaudeFullQuotaSnapshot (same format) for the codex snapshot.
	return WriteClaudeFullQuotaSnapshot(snapshotPath, windows, nil, now)
}

func resolveCodexQuotaSnapshotPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".ddx", "provider-state", "codex-quota.json")
}

// RefreshClaudeQuotaViaTmux refreshes the claude quota snapshot via tmux if the
// existing snapshot is older than maxAge. Safe to call from slow-path commands
// (ddx agent doctor) but NOT from routing hot paths.
func (r *Runner) RefreshClaudeQuotaViaTmux(now time.Time, maxAge time.Duration) error {
	snapshotPath := resolveClaudeQuotaSnapshotPath()
	if snapshotPath == "" {
		return fmt.Errorf("cannot resolve claude quota snapshot path")
	}

	// Skip if snapshot is fresh enough.
	if _, _, observedAt, err := ReadClaudeFullSnapshot(snapshotPath, now); err == nil {
		if !observedAt.IsZero() && now.UTC().Sub(observedAt.UTC()) < maxAge {
			return nil
		}
	}

	windows, acct, err := ReadClaudeQuotaViaTmux(30 * time.Second)
	if err != nil {
		return err
	}
	return WriteClaudeFullQuotaSnapshot(snapshotPath, windows, acct, now)
}

var sessionQuotaBlockPattern = regexp.MustCompile(`(?i)(you've hit your limit|quota exceeded|quota_exceeded|insufficient credits|insufficient_credits|429|rate limit|ratelimit)`)

func (r *Runner) overlayRecentQuotaBlock(harnessName string, snapshot RoutingSignalSnapshot, now time.Time) RoutingSignalSnapshot {
	entry, ok := r.readLatestQuotaBlockedSession(harnessName, now)
	if !ok {
		return snapshot
	}

	detail := quotaBlockDetail(entry)

	meta := SignalSourceMetadata{
		Provider:   harnessName,
		Kind:       sessionLogSourceKind,
		Path:       r.resolveSessionLogPath(),
		ObservedAt: entry.Timestamp.UTC(),
		Freshness:  freshnessForAge(now.UTC().Sub(entry.Timestamp.UTC())),
		AgeSeconds: int64(now.UTC().Sub(entry.Timestamp.UTC()).Seconds()),
		Basis:      "recent session failure",
		Notes:      detail,
	}
	if meta.AgeSeconds < 0 {
		meta.AgeSeconds = 0
	}

	snapshot.Provider = harnessName
	snapshot.Source = meta
	snapshot.CurrentQuota = QuotaSignal{
		Source:   meta,
		State:    "blocked",
		ResetsAt: inferResetAt(detail, entry.Timestamp),
	}
	if harnessName == "claude" {
		_ = WriteClaudeQuotaSnapshot(resolveClaudeQuotaSnapshotPath(), snapshot.CurrentQuota)
	}
	return snapshot
}

func resolveClaudeQuotaSnapshotPath() string {
	if path := strings.TrimSpace(os.Getenv(claudeQuotaSnapshotEnv)); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".ddx", "provider-state", "claude-quota.json")
}

func quotaBlockDetail(entry SessionEntry) string {
	candidates := []string{
		strings.TrimSpace(entry.Response),
		strings.TrimSpace(entry.Stderr),
		strings.TrimSpace(entry.Error),
	}
	for _, candidate := range candidates {
		if candidate != "" && sessionQuotaBlockPattern.MatchString(strings.ToLower(candidate)) {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func (r *Runner) readLatestQuotaBlockedSession(harnessName string, now time.Time) (SessionEntry, bool) {
	path := r.resolveSessionLogPath()
	if path == "" {
		return SessionEntry{}, false
	}

	cutoff := now.UTC().Add(-12 * time.Hour)
	var latest SessionEntry
	found := false

	_ = ForEachJSONL[SessionEntry](path, func(entry SessionEntry) error {
		if entry.Harness != harnessName {
			return nil
		}
		if entry.Timestamp.IsZero() || entry.Timestamp.UTC().Before(cutoff) {
			return nil
		}
		if !sessionQuotaBlockPattern.MatchString(strings.ToLower(entry.Error + "\n" + entry.Response + "\n" + entry.Stderr)) {
			return nil
		}
		if !found || entry.Timestamp.After(latest.Timestamp) {
			latest = entry
			found = true
		}
		return nil
	})
	return latest, found
}

func (r *Runner) resolveSessionLogPath() string {
	dir := ""
	if r != nil {
		dir = r.Config.SessionLogDir
	}
	if dir == "" {
		dir = DefaultLogDir
	}
	if filepath.IsAbs(dir) {
		return filepath.Join(dir, "sessions.jsonl")
	}
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		return ""
	}
	return filepath.Join(wd, dir, "sessions.jsonl")
}

func freshnessForAge(age time.Duration) string {
	switch {
	case age <= 0:
		return "fresh"
	case age <= 15*time.Minute:
		return "fresh"
	case age <= 2*time.Hour:
		return "recent"
	default:
		return "stale"
	}
}

var resetAtPattern = regexp.MustCompile(`(?i)resets?\s+12am\s+\(([^)]+)\)`)

func inferResetAt(detail string, observedAt time.Time) string {
	match := resetAtPattern.FindStringSubmatch(detail)
	if len(match) != 2 {
		return ""
	}
	loc, err := time.LoadLocation(match[1])
	if err != nil {
		return ""
	}
	observed := observedAt.In(loc)
	nextMidnight := time.Date(observed.Year(), observed.Month(), observed.Day()+1, 0, 0, 0, 0, loc)
	return nextMidnight.UTC().Format(time.RFC3339)
}
