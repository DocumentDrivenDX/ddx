package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	codexNativeSessionEnv = "DDX_CODEX_NATIVE_SESSION_JSONL"
	claudeStatsCacheEnv   = "DDX_CLAUDE_STATS_CACHE"
	sessionLogSourceKind  = "recent-session-log"
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
	default:
		return RoutingSignalSnapshot{}
	}
	return r.overlayRecentQuotaBlock(harnessName, snapshot, now)
}

func (r *Runner) loadCodexRoutingSignal(now time.Time) RoutingSignalSnapshot {
	path := os.Getenv(codexNativeSessionEnv)
	if path == "" {
		return RoutingSignalSnapshot{
			Provider: "codex",
			Source: SignalSourceMetadata{
				Provider:  "codex",
				Kind:      codexNativeSessionSourceKind,
				Freshness: "unknown",
				Basis:     "path not configured",
				Notes:     codexNativeSessionEnv + " is not set",
			},
			CurrentQuota: QuotaSignal{
				Source: SignalSourceMetadata{
					Provider:  "codex",
					Kind:      codexNativeSessionSourceKind,
					Freshness: "unknown",
					Basis:     "path not configured",
					Notes:     codexNativeSessionEnv + " is not set",
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

	return RoutingSignalSnapshot{
		Provider:        "codex",
		Source:          signals.CurrentQuota.Source,
		CurrentQuota:    signals.CurrentQuota,
		HistoricalUsage: signals.RecentUsage,
	}
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

	return RoutingSignalSnapshot{
		Provider:        "claude",
		Source:          signals.HistoricalUsage.Source,
		CurrentQuota:    signals.CurrentQuota,
		HistoricalUsage: signals.HistoricalUsage,
	}
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
	return snapshot
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
	f, err := os.Open(path)
	if err != nil {
		return SessionEntry{}, false
	}
	defer f.Close()

	cutoff := now.UTC().Add(-12 * time.Hour)
	var latest SessionEntry
	found := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry SessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Harness != harnessName {
			continue
		}
		if entry.Timestamp.IsZero() || entry.Timestamp.UTC().Before(cutoff) {
			continue
		}
		if !sessionQuotaBlockPattern.MatchString(strings.ToLower(entry.Error + "\n" + entry.Response + "\n" + entry.Stderr)) {
			continue
		}
		if !found || entry.Timestamp.After(latest.Timestamp) {
			latest = entry
			found = true
		}
	}
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
