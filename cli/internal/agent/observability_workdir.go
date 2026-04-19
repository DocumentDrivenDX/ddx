package agent

// observability_workdir.go provides workDir-based free-function helpers that
// drive the observability surface (harness state probing, routing-signal
// snapshots, tmux quota refresh) without constructing an *agent.Runner.
//
// DDx callers in cmd/ and internal/server/ go through these helpers so the
// last non-test consumer of NewRunner can be retired alongside the Runner
// observability methods on state.go / routing_signals.go (those will be
// deleted as a cascade once the rest of routing moves into the agent
// service).

import (
	"context"
	"fmt"
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

// ProbeHarnessStateForWorkDir reports the routing-relevant state of a harness
// using only the local filesystem and PATH lookup. It is the workDir-based
// replacement for (*Runner).ProbeHarnessState used by ddx agent doctor and
// ddx agent usage.
func ProbeHarnessStateForWorkDir(workDir, harnessName string, timeout time.Duration) HarnessState {
	sessionLogDir := SessionLogDirForWorkDir(workDir)
	registry := NewRegistry()
	executor := &OSExecutor{}

	state := HarnessState{
		LastChecked: time.Now(),
		PolicyOK:    true,
	}

	harness, ok := registry.Get(harnessName)
	if !ok {
		state.Error = fmt.Sprintf("unknown harness: %s", harnessName)
		return state
	}

	// Embedded harnesses are always installed, always reachable.
	if harness.IsLocal || harnessName == "virtual" || harnessName == "agent" {
		state.Installed = true
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		return state
	}

	// HTTP-only providers: probe via API, no binary lookup.
	if harness.IsHTTPProvider {
		state.Installed = true
		signal := loadRoutingSignalSnapshotFromFiles(harnessName, sessionLogDir, state.LastChecked)
		state.RoutingSignal = &signal
		state.Reachable = signal.CurrentQuota.State != "unknown"
		state.Authenticated = signal.CurrentQuota.State != "unknown"
		state.QuotaState = signal.CurrentQuota.State
		state.QuotaOK = signal.CurrentQuota.State == "ok"
		if state.QuotaState == "" {
			state.QuotaState = "unknown"
		}
		return state
	}

	// Check binary on PATH.
	if _, err := DefaultLookPath(harness.Binary); err != nil {
		state.Installed = false
		state.Error = fmt.Sprintf("%s not found in PATH", harness.Binary)
		return state
	}
	state.Installed = true

	// Load provider-native routing signals where the repo/docs define a stable source.
	if harnessName == "codex" || harnessName == "claude" {
		signal := loadRoutingSignalSnapshotFromFiles(harnessName, sessionLogDir, state.LastChecked)
		state.RoutingSignal = &signal
		if signal.CurrentQuota.State != "" {
			state.QuotaState = signal.CurrentQuota.State
			if signal.CurrentQuota.State == "blocked" {
				state.QuotaOK = false
			}
		}
		if signal.CurrentQuota.State == "blocked" {
			state.QuotaOK = false
		}
		if state.Quota == nil && signal.CurrentQuota.State != "unknown" {
			state.Quota = &QuotaInfo{
				PercentUsed: signal.CurrentQuota.UsedPercent,
				LimitWindow: fmt.Sprintf("%d min", signal.CurrentQuota.WindowMinutes),
				ResetDate:   signal.CurrentQuota.ResetsAt,
			}
		}
		if signal.CurrentQuota.State == "unknown" && harnessName == "claude" {
			state.QuotaOK = true
		}
		// Consult the durable Claude current-quota cache for absolute
		// 5-hour/weekly headroom. Foreground routing prefers cached
		// snapshots over inline PTY capture.
		if harnessName == "claude" {
			decision := ReadClaudeQuotaRoutingDecision(state.LastChecked, DefaultClaudeQuotaStaleAfter)
			state.ClaudeQuotaDecision = &decision
			if decision.Fresh && !decision.PreferClaude {
				state.QuotaOK = false
				if state.QuotaState == "" || state.QuotaState == "unknown" || state.QuotaState == "ok" {
					state.QuotaState = "blocked"
				}
			}
		}
	}

	// If there's a quota command, drive it to get quota data.
	if harness.QuotaCommand != "" {
		quota, probeErr := probeQuotaFromFiles(executor, harness, strings.Fields(harness.QuotaCommand), timeout)
		if probeErr != nil {
			state.Reachable = false
			state.Degraded = true
			state.Error = fmt.Sprintf("quota probe failed: %v", probeErr)
			return state
		}
		state.Reachable = true
		state.Authenticated = true
		state.QuotaOK = true
		if quota != nil {
			state.Quota = quota
			if quota.PercentUsed >= 95 {
				state.QuotaOK = false
			}
			if state.QuotaState == "" {
				state.QuotaState = quotaStateFromUsedPercent(quota.PercentUsed)
			}
			recordQuotaSnapshotFromFiles(sessionLogDir, harnessName, harness, quota, "async-probe")
		}
		if state.QuotaState == "" {
			state.QuotaState = "ok"
		}
		return state
	}

	// No quota command: fall back to TUI slash command if native signal is unknown.
	state.Reachable = true
	state.Authenticated = true
	if state.QuotaState == "" {
		state.QuotaState = "unknown"
	}
	if state.QuotaState == "unknown" && harness.TUIQuotaCommand != "" {
		quota, _ := probeQuotaFromFiles(executor, harness, strings.Fields(harness.TUIQuotaCommand), timeout)
		if quota != nil {
			state.Quota = quota
			state.QuotaState = quotaStateFromUsedPercent(quota.PercentUsed)
			if quota.PercentUsed >= 95 {
				state.QuotaOK = false
			}
			recordQuotaSnapshotFromFiles(sessionLogDir, harnessName, harness, quota, "tui-probe")
		}
	}
	if state.QuotaState != "blocked" {
		state.QuotaOK = true
	}
	return state
}

// probeQuotaFromFiles invokes a harness binary with explicit args and parses
// the quota output. Used both for QuotaCommand and TUIQuotaCommand probes.
// Equivalent to (*Runner).probeQuotaWithArgs but takes an explicit Executor.
func probeQuotaFromFiles(executor Executor, harness Harness, args []string, timeout time.Duration) (*QuotaInfo, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := executor.ExecuteInDir(ctx, harness.Binary, args, "", "")
	if err != nil {
		return nil, fmt.Errorf("invoke %s: %w", harness.Binary, err)
	}

	combined := result.Stdout
	if result.Stderr != "" {
		combined += "\n" + result.Stderr
	}
	return ParseQuotaOutput(combined), nil
}

// recordQuotaSnapshotFromFiles writes a quota snapshot to the routing metrics
// store. Equivalent to (*Runner).recordQuotaSnapshot but takes an explicit
// session log directory.
func recordQuotaSnapshotFromFiles(sessionLogDir, harnessName string, harness Harness, quota *QuotaInfo, sampleKind string) {
	if sessionLogDir == "" || quota == nil {
		return
	}

	snapshot := QuotaSnapshot{
		Harness:         harnessName,
		Surface:         harness.Surface,
		CanonicalTarget: harness.DefaultModel,
		Source:          harness.QuotaCommand,
		ObservedAt:      time.Now().UTC(),
		SampleKind:      sampleKind,
		UsedPercent:     quota.PercentUsed,
		ResetsAt:        quota.ResetDate,
		QuotaState:      "ok",
	}
	if snapshot.CanonicalTarget == "" {
		snapshot.CanonicalTarget = harnessName
	}
	if quota.PercentUsed >= 95 {
		snapshot.QuotaState = "blocked"
	}
	snapshot.WindowMinutes = parseWindowMinutes(quota.LimitWindow)

	_ = NewRoutingMetricsStore(sessionLogDir).AppendQuotaSnapshot(snapshot)
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

// RefreshClaudeQuotaViaTmuxForWorkDir refreshes the claude quota snapshot via
// tmux if the existing snapshot is older than maxAge. Safe to call from
// slow-path commands (ddx agent doctor) but NOT from routing hot paths.
// It is the workDir-based replacement for (*Runner).RefreshClaudeQuotaViaTmux.
func RefreshClaudeQuotaViaTmuxForWorkDir(_ string, now time.Time, maxAge time.Duration) error {
	snapshotPath := resolveClaudeQuotaSnapshotPath()
	if snapshotPath == "" {
		return fmt.Errorf("cannot resolve claude quota snapshot path")
	}
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

// RefreshCodexQuotaViaTmuxForWorkDir refreshes the codex quota snapshot via
// tmux if the existing snapshot is older than maxAge. It is the workDir-based
// replacement for (*Runner).RefreshCodexQuotaViaTmux.
func RefreshCodexQuotaViaTmuxForWorkDir(_ string, now time.Time, maxAge time.Duration) error {
	snapshotPath := resolveCodexQuotaSnapshotPath()
	if snapshotPath == "" {
		return fmt.Errorf("cannot resolve codex quota snapshot path")
	}
	if _, _, observedAt, err := ReadClaudeFullSnapshot(snapshotPath, now); err == nil {
		if !observedAt.IsZero() && now.UTC().Sub(observedAt.UTC()) < maxAge {
			return nil
		}
	}

	windows, err := ReadCodexQuotaViaTmux(30 * time.Second)
	if err != nil {
		return err
	}
	return WriteClaudeFullQuotaSnapshot(snapshotPath, windows, nil, now)
}
