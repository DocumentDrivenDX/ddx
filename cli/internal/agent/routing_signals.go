package agent

import (
	"os"
	"path/filepath"
	"time"
)

const (
	codexNativeSessionEnv = "DDX_CODEX_NATIVE_SESSION_JSONL"
	claudeStatsCacheEnv   = "DDX_CLAUDE_STATS_CACHE"
)

// LoadRoutingSignalSnapshot returns the provider-native routing signal snapshot
// for a harness. Missing sources are surfaced as an explicit unknown snapshot
// so callers can distinguish unavailable from blocked.
func (r *Runner) LoadRoutingSignalSnapshot(harnessName string, now time.Time) RoutingSignalSnapshot {
	switch harnessName {
	case "codex":
		return r.loadCodexRoutingSignal(now)
	case "claude":
		return r.loadClaudeRoutingSignal(now)
	default:
		return RoutingSignalSnapshot{}
	}
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
