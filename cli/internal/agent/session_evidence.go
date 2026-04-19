package agent

// DDx-evidence helpers (per ddx-992c924d). Read DDx-managed files; not
// duplicates of agent service surface.
//
// These workDir-based free-function helpers expose DDx's view of session and
// routing-signal evidence to cmd/ and internal/server/. They intentionally do
// not construct an *agent.Runner and operate strictly on local filesystem
// state managed by DDx.

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// SessionLogDirForWorkDir returns the resolved session-log directory for a
// project root, applying the same precedence rules used by the retired
// f.agentRunner() constructor: project config when present, otherwise the
// default log dir resolved against workDir.
func SessionLogDirForWorkDir(workDir string) string {
	configured := ""
	if c, err := config.LoadWithWorkingDir(workDir); err == nil && c != nil && c.Agent != nil {
		configured = c.Agent.SessionLogDir
	}
	return ResolveLogDir(workDir, configured)
}

// LoadRoutingSignalSnapshotForWorkDir returns the provider-native routing
// signal snapshot for a harness by reading from disk. It is the workDir-based
// replacement for (*Runner).LoadRoutingSignalSnapshot used by
// ddx agent route-status, ddx server providers, and ddx agent usage.
func LoadRoutingSignalSnapshotForWorkDir(workDir, harnessName string, now time.Time) RoutingSignalSnapshot {
	return loadRoutingSignalSnapshotFromFiles(harnessName, SessionLogDirForWorkDir(workDir), now)
}

// loadRoutingSignalSnapshotFromFiles reads provider-native signals from disk
// without requiring a Runner. Mirrors (*Runner).LoadRoutingSignalSnapshot.
func loadRoutingSignalSnapshotFromFiles(harnessName, sessionLogDir string, now time.Time) RoutingSignalSnapshot {
	var snapshot RoutingSignalSnapshot
	switch harnessName {
	case "codex":
		snapshot = loadCodexRoutingSignalFromFiles(now)
	case "claude":
		snapshot = loadClaudeRoutingSignalFromFiles(now)
	case "openrouter":
		return ProbeOpenRouterBalance(8 * time.Second)
	case "lmstudio":
		return loadLMStudioSignalFromFiles(now)
	default:
		return RoutingSignalSnapshot{}
	}
	return overlayRecentQuotaBlockFromFiles(harnessName, snapshot, sessionLogDir, now)
}

// loadCodexRoutingSignalFromFiles mirrors (*Runner).loadCodexRoutingSignal.
// It depends only on environment + disk state, so it has no Runner dependency.
func loadCodexRoutingSignalFromFiles(now time.Time) RoutingSignalSnapshot {
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

	// Promote worst window state to CurrentQuota; skip "extra" overflow buckets.
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

// loadClaudeRoutingSignalFromFiles mirrors (*Runner).loadClaudeRoutingSignal.
func loadClaudeRoutingSignalFromFiles(now time.Time) RoutingSignalSnapshot {
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

// loadLMStudioSignalFromFiles mirrors (*Runner).loadLMStudioSignal.
func loadLMStudioSignalFromFiles(now time.Time) RoutingSignalSnapshot {
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

	allEndpoints := ReadLMStudioEndpointsFromAgentConfig()
	if len(allEndpoints) == 0 {
		return unknown
	}

	statuses := ProbeLMStudioEndpoints(allEndpoints, 3*time.Second)
	return BuildLMStudioSignal("agent-config", statuses, now)
}

// overlayRecentQuotaBlockFromFiles mirrors (*Runner).overlayRecentQuotaBlock —
// applies a recent-session-failure overlay onto a routing snapshot when the
// session log shows a quota-blocked failure within the lookback window.
func overlayRecentQuotaBlockFromFiles(harnessName string, snapshot RoutingSignalSnapshot, sessionLogDir string, now time.Time) RoutingSignalSnapshot {
	logPath := sessionLogPathFromDir(sessionLogDir)
	entry, ok := readLatestQuotaBlockedSessionFromFile(logPath, harnessName, now)
	if !ok {
		return snapshot
	}

	detail := quotaBlockDetail(entry)
	meta := SignalSourceMetadata{
		Provider:   harnessName,
		Kind:       sessionLogSourceKind,
		Path:       logPath,
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

// readLatestQuotaBlockedSessionFromFile mirrors
// (*Runner).readLatestQuotaBlockedSession.
func readLatestQuotaBlockedSessionFromFile(logPath, harnessName string, now time.Time) (SessionEntry, bool) {
	if logPath == "" {
		return SessionEntry{}, false
	}
	cutoff := now.UTC().Add(-12 * time.Hour)
	var latest SessionEntry
	found := false

	_ = ForEachJSONL[SessionEntry](logPath, func(entry SessionEntry) error {
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

// sessionLogPathFromDir mirrors (*Runner).resolveSessionLogPath but takes an
// explicit session log directory.
func sessionLogPathFromDir(dir string) string {
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
