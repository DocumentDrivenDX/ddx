package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ClaudeQuotaSnapshot captures Claude's current-quota headroom as absolute
// token/message counts. It is written to a durable per-user cache by an
// asynchronous capture path and read by foreground routing consumers.
//
// The snapshot is intentionally distinct from the percentage-based
// QuotaSignal: foreground routing needs concrete numbers to reason about
// 5-hour / weekly headroom without invoking PTY capture inline.
type ClaudeQuotaSnapshot struct {
	CapturedAt        time.Time `json:"captured_at"`
	FiveHourRemaining int       `json:"five_hour_remaining"`
	FiveHourLimit     int       `json:"five_hour_limit"`
	WeeklyRemaining   int       `json:"weekly_remaining"`
	WeeklyLimit       int       `json:"weekly_limit"`
	Source            string    `json:"source"` // e.g. "pty", "heuristic"
}

// DefaultClaudeQuotaStaleAfter is the default maximum age before a cached
// snapshot is considered stale and foreground routing should fall back to
// the safe default.
const DefaultClaudeQuotaStaleAfter = 5 * time.Minute

// claudeQuotaCacheEnv lets tests override the cache file path.
const claudeQuotaCacheEnv = "DDX_CLAUDE_QUOTA_CACHE"

// ClaudeQuotaCachePath returns the durable location for the Claude quota
// cache. It resolves to $XDG_STATE_HOME/ddx/claude-quota.json, or
// ~/.local/state/ddx/claude-quota.json when XDG_STATE_HOME is unset. The
// DDX_CLAUDE_QUOTA_CACHE env var takes precedence (primarily for tests).
func ClaudeQuotaCachePath() (string, error) {
	if path := os.Getenv(claudeQuotaCacheEnv); path != "" {
		return path, nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "ddx", "claude-quota.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "ddx", "claude-quota.json"), nil
}

// WriteClaudeQuota atomically persists a ClaudeQuotaSnapshot to the given
// path. The parent directory is created if necessary. The file is written
// to a sibling .tmp file and renamed into place so readers never observe a
// partially-written snapshot. The final file mode is 0600.
func WriteClaudeQuota(path string, snapshot ClaudeQuotaSnapshot) error {
	if path == "" {
		return fmt.Errorf("claude quota cache path is empty")
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	} else {
		snapshot.CapturedAt = snapshot.CapturedAt.UTC()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create claude quota cache dir: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude quota snapshot: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write claude quota cache tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename claude quota cache: %w", err)
	}
	// Ensure final mode is 0600 in case an older file had a different mode.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod claude quota cache: %w", err)
	}
	return nil
}

// ReadClaudeQuotaFrom reads the snapshot at the given path. Returns
// (nil, false) if the file does not exist or cannot be decoded. Non-
// existence is NOT an error: foreground callers are expected to fall back
// to a safe default when no snapshot is present.
func ReadClaudeQuotaFrom(path string) (*ClaudeQuotaSnapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var snap ClaudeQuotaSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, false
	}
	return &snap, true
}

// ReadClaudeQuota returns the latest cached Claude quota snapshot from the
// default cache path. The second return value is false if no snapshot is
// present or cannot be decoded.
//
// Callers SHOULD check snapshot age via ClaudeQuotaSnapshotAge (or
// IsClaudeQuotaFresh) before trusting the values; this function does not
// itself enforce a TTL so that callers can report stale snapshots in
// diagnostic surfaces like `ddx agent doctor --routing`.
func ReadClaudeQuota() (*ClaudeQuotaSnapshot, bool) {
	path, err := ClaudeQuotaCachePath()
	if err != nil {
		return nil, false
	}
	return ReadClaudeQuotaFrom(path)
}

// ClaudeQuotaSnapshotAge reports the age of a snapshot relative to now.
// A zero or future CapturedAt yields a zero age.
func ClaudeQuotaSnapshotAge(snapshot *ClaudeQuotaSnapshot, now time.Time) time.Duration {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return 0
	}
	age := now.UTC().Sub(snapshot.CapturedAt.UTC())
	if age < 0 {
		return 0
	}
	return age
}

// IsClaudeQuotaFresh reports whether a snapshot exists and is newer than
// staleAfter relative to now. A nil snapshot is never fresh. A zero
// staleAfter falls back to DefaultClaudeQuotaStaleAfter.
func IsClaudeQuotaFresh(snapshot *ClaudeQuotaSnapshot, now time.Time, staleAfter time.Duration) bool {
	if snapshot == nil || snapshot.CapturedAt.IsZero() {
		return false
	}
	if staleAfter <= 0 {
		staleAfter = DefaultClaudeQuotaStaleAfter
	}
	return ClaudeQuotaSnapshotAge(snapshot, now) <= staleAfter
}

// ClaudeQuotaRoutingDecision summarises what foreground routing should do
// given the current cached snapshot.
type ClaudeQuotaRoutingDecision struct {
	// PreferClaude is true when a fresh snapshot shows headroom in both the
	// 5-hour and weekly windows. When false, routing should prefer a
	// non-claude fallback harness.
	PreferClaude bool
	// SnapshotPresent is true when a snapshot was found in the cache (even
	// if stale).
	SnapshotPresent bool
	// Fresh is true when the snapshot is present and newer than staleAfter.
	Fresh bool
	// Age is the age of the snapshot relative to now (zero when absent).
	Age time.Duration
	// Snapshot is the cached snapshot when present.
	Snapshot *ClaudeQuotaSnapshot
	// Reason describes why the decision was made (diagnostic surface).
	Reason string
}

// DecideClaudeQuotaRouting turns a cached snapshot into a routing decision
// for foreground callers. When the snapshot is missing or stale, the safe
// default is NOT to prefer claude (assume limited).
//
// A snapshot counts as "limited" when either window reports zero or
// negative remaining headroom.
func DecideClaudeQuotaRouting(snapshot *ClaudeQuotaSnapshot, now time.Time, staleAfter time.Duration) ClaudeQuotaRoutingDecision {
	decision := ClaudeQuotaRoutingDecision{
		Snapshot: snapshot,
	}
	if snapshot == nil {
		decision.Reason = "no cached snapshot; assume limited"
		return decision
	}
	decision.SnapshotPresent = true
	decision.Age = ClaudeQuotaSnapshotAge(snapshot, now)
	if !IsClaudeQuotaFresh(snapshot, now, staleAfter) {
		decision.Reason = "cached snapshot is stale; assume limited"
		return decision
	}
	decision.Fresh = true
	if snapshot.FiveHourRemaining <= 0 || snapshot.WeeklyRemaining <= 0 {
		decision.Reason = "fresh snapshot reports exhausted window; assume limited"
		return decision
	}
	decision.PreferClaude = true
	decision.Reason = "fresh snapshot has headroom"
	return decision
}

// ReadClaudeQuotaRoutingDecision is a convenience wrapper that reads the
// default cache and produces a routing decision in one call. It is the
// entry point foreground routing should use instead of any inline PTY
// capture.
func ReadClaudeQuotaRoutingDecision(now time.Time, staleAfter time.Duration) ClaudeQuotaRoutingDecision {
	snap, _ := ReadClaudeQuota()
	return DecideClaudeQuotaRouting(snap, now, staleAfter)
}

// RefreshClaudeQuotaAsync launches a background goroutine that invokes
// capture and writes the result to the default cache path. It returns
// immediately; callers never block on capture.
//
// The capture function is injected so that the durable cache layer does
// not depend on any specific PTY implementation. A future `ddx claude
// refresh-quota` subcommand can pass a real PTY-backed capture; tests can
// pass deterministic fakes.
//
// If capture returns an error, the cache is not touched.
func RefreshClaudeQuotaAsync(capture func() (ClaudeQuotaSnapshot, error)) {
	if capture == nil {
		return
	}
	go func() {
		snap, err := capture()
		if err != nil {
			return
		}
		path, pathErr := ClaudeQuotaCachePath()
		if pathErr != nil {
			return
		}
		_ = WriteClaudeQuota(path, snap)
	}()
}
