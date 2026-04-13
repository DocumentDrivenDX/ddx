package agent

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeQuotaCachePathXDG(t *testing.T) {
	t.Setenv(claudeQuotaCacheEnv, "")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	path, err := ClaudeQuotaCachePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/xdg-state", "ddx", "claude-quota.json"), path)
}

func TestClaudeQuotaCachePathHomeFallback(t *testing.T) {
	t.Setenv(claudeQuotaCacheEnv, "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/fake-home")
	path, err := ClaudeQuotaCachePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/fake-home", ".local", "state", "ddx", "claude-quota.json"), path)
}

func TestClaudeQuotaCachePathEnvOverride(t *testing.T) {
	t.Setenv(claudeQuotaCacheEnv, "/tmp/override/cq.json")
	path, err := ClaudeQuotaCachePath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/override/cq.json", path)
}

func TestClaudeQuotaSnapshotRoundTrip(t *testing.T) {
	captured := time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC)
	original := ClaudeQuotaSnapshot{
		CapturedAt:        captured,
		FiveHourRemaining: 7500,
		FiveHourLimit:     10000,
		WeeklyRemaining:   40000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded ClaudeQuotaSnapshot
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.True(t, decoded.CapturedAt.Equal(original.CapturedAt))
	assert.Equal(t, original.FiveHourRemaining, decoded.FiveHourRemaining)
	assert.Equal(t, original.FiveHourLimit, decoded.FiveHourLimit)
	assert.Equal(t, original.WeeklyRemaining, decoded.WeeklyRemaining)
	assert.Equal(t, original.WeeklyLimit, decoded.WeeklyLimit)
	assert.Equal(t, original.Source, decoded.Source)
}

func TestWriteClaudeQuotaAtomicAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "claude-quota.json")

	snap := ClaudeQuotaSnapshot{
		CapturedAt:        time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
		FiveHourRemaining: 8000,
		FiveHourLimit:     10000,
		WeeklyRemaining:   60000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}

	require.NoError(t, WriteClaudeQuota(path, snap))

	// Read back and compare.
	loaded, ok := ReadClaudeQuotaFrom(path)
	require.True(t, ok)
	require.NotNil(t, loaded)
	assert.True(t, loaded.CapturedAt.Equal(snap.CapturedAt))
	assert.Equal(t, snap.FiveHourRemaining, loaded.FiveHourRemaining)
	assert.Equal(t, snap.FiveHourLimit, loaded.FiveHourLimit)
	assert.Equal(t, snap.WeeklyRemaining, loaded.WeeklyRemaining)
	assert.Equal(t, snap.WeeklyLimit, loaded.WeeklyLimit)
	assert.Equal(t, snap.Source, loaded.Source)

	// Verify mode is 0600.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())

	// No leftover tmp file.
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "tmp file should not remain after rename")

	// Overwriting the same path works (atomic replace).
	snap.FiveHourRemaining = 100
	require.NoError(t, WriteClaudeQuota(path, snap))
	loaded2, ok := ReadClaudeQuotaFrom(path)
	require.True(t, ok)
	assert.Equal(t, 100, loaded2.FiveHourRemaining)
}

func TestReadClaudeQuotaMissingReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	snap, ok := ReadClaudeQuotaFrom(path)
	assert.False(t, ok)
	assert.Nil(t, snap)
}

func TestReadClaudeQuotaCorruptReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	snap, ok := ReadClaudeQuotaFrom(path)
	assert.False(t, ok)
	assert.Nil(t, snap)
}

func TestReadClaudeQuotaUsesDefaultCachePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-quota.json")
	t.Setenv(claudeQuotaCacheEnv, path)

	snap := ClaudeQuotaSnapshot{
		CapturedAt:        time.Now().UTC(),
		FiveHourRemaining: 1,
		FiveHourLimit:     2,
		WeeklyRemaining:   3,
		WeeklyLimit:       4,
		Source:            "pty",
	}
	require.NoError(t, WriteClaudeQuota(path, snap))

	loaded, ok := ReadClaudeQuota()
	require.True(t, ok)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, loaded.FiveHourRemaining)
	assert.Equal(t, "pty", loaded.Source)
}

func TestIsClaudeQuotaFresh(t *testing.T) {
	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		snapshot   *ClaudeQuotaSnapshot
		staleAfter time.Duration
		wantFresh  bool
	}{
		{
			name:      "nil snapshot",
			snapshot:  nil,
			wantFresh: false,
		},
		{
			name: "zero captured_at",
			snapshot: &ClaudeQuotaSnapshot{
				CapturedAt: time.Time{},
			},
			wantFresh: false,
		},
		{
			name: "fresh within default ttl",
			snapshot: &ClaudeQuotaSnapshot{
				CapturedAt: now.Add(-2 * time.Minute),
			},
			wantFresh: true,
		},
		{
			name: "stale past default ttl",
			snapshot: &ClaudeQuotaSnapshot{
				CapturedAt: now.Add(-10 * time.Minute),
			},
			wantFresh: false,
		},
		{
			name: "fresh under custom ttl",
			snapshot: &ClaudeQuotaSnapshot{
				CapturedAt: now.Add(-1 * time.Hour),
			},
			staleAfter: 2 * time.Hour,
			wantFresh:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsClaudeQuotaFresh(tc.snapshot, now, tc.staleAfter)
			assert.Equal(t, tc.wantFresh, got)
		})
	}
}

func TestClaudeQuotaSnapshotAge(t *testing.T) {
	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	// Nil snapshot.
	assert.Equal(t, time.Duration(0), ClaudeQuotaSnapshotAge(nil, now))
	// Zero time.
	assert.Equal(t, time.Duration(0), ClaudeQuotaSnapshotAge(&ClaudeQuotaSnapshot{}, now))
	// Future captured_at.
	future := &ClaudeQuotaSnapshot{CapturedAt: now.Add(time.Hour)}
	assert.Equal(t, time.Duration(0), ClaudeQuotaSnapshotAge(future, now))
	// Past captured_at.
	past := &ClaudeQuotaSnapshot{CapturedAt: now.Add(-3 * time.Minute)}
	assert.Equal(t, 3*time.Minute, ClaudeQuotaSnapshotAge(past, now))
}

func TestDecideClaudeQuotaRouting(t *testing.T) {
	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)

	fresh := &ClaudeQuotaSnapshot{
		CapturedAt:        now.Add(-1 * time.Minute),
		FiveHourRemaining: 5000,
		FiveHourLimit:     10000,
		WeeklyRemaining:   50000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}
	freshExhausted := &ClaudeQuotaSnapshot{
		CapturedAt:        now.Add(-1 * time.Minute),
		FiveHourRemaining: 0,
		FiveHourLimit:     10000,
		WeeklyRemaining:   50000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}
	freshWeeklyZero := &ClaudeQuotaSnapshot{
		CapturedAt:        now.Add(-1 * time.Minute),
		FiveHourRemaining: 5000,
		FiveHourLimit:     10000,
		WeeklyRemaining:   0,
		WeeklyLimit:       70000,
		Source:            "pty",
	}
	stale := &ClaudeQuotaSnapshot{
		CapturedAt:        now.Add(-10 * time.Minute),
		FiveHourRemaining: 5000,
		FiveHourLimit:     10000,
		WeeklyRemaining:   50000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}

	cases := []struct {
		name            string
		snapshot        *ClaudeQuotaSnapshot
		wantPrefer      bool
		wantPresent     bool
		wantFresh       bool
		wantReasonSubst string
	}{
		{
			name:            "missing snapshot -> fall back",
			snapshot:        nil,
			wantPrefer:      false,
			wantPresent:     false,
			wantFresh:       false,
			wantReasonSubst: "no cached snapshot",
		},
		{
			name:            "stale snapshot -> fall back",
			snapshot:        stale,
			wantPrefer:      false,
			wantPresent:     true,
			wantFresh:       false,
			wantReasonSubst: "stale",
		},
		{
			name:            "fresh with headroom -> prefer claude",
			snapshot:        fresh,
			wantPrefer:      true,
			wantPresent:     true,
			wantFresh:       true,
			wantReasonSubst: "headroom",
		},
		{
			name:            "fresh but 5h exhausted -> fall back",
			snapshot:        freshExhausted,
			wantPrefer:      false,
			wantPresent:     true,
			wantFresh:       true,
			wantReasonSubst: "exhausted",
		},
		{
			name:            "fresh but weekly exhausted -> fall back",
			snapshot:        freshWeeklyZero,
			wantPrefer:      false,
			wantPresent:     true,
			wantFresh:       true,
			wantReasonSubst: "exhausted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := DecideClaudeQuotaRouting(tc.snapshot, now, DefaultClaudeQuotaStaleAfter)
			assert.Equal(t, tc.wantPrefer, d.PreferClaude)
			assert.Equal(t, tc.wantPresent, d.SnapshotPresent)
			assert.Equal(t, tc.wantFresh, d.Fresh)
			assert.Contains(t, d.Reason, tc.wantReasonSubst)
		})
	}
}

func TestReadClaudeQuotaRoutingDecisionDefaultPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-quota.json")
	t.Setenv(claudeQuotaCacheEnv, path)

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)

	// No snapshot: fall back.
	d := ReadClaudeQuotaRoutingDecision(now, DefaultClaudeQuotaStaleAfter)
	assert.False(t, d.PreferClaude)
	assert.False(t, d.SnapshotPresent)

	// Write a fresh snapshot with headroom.
	require.NoError(t, WriteClaudeQuota(path, ClaudeQuotaSnapshot{
		CapturedAt:        now.Add(-30 * time.Second),
		FiveHourRemaining: 9000,
		FiveHourLimit:     10000,
		WeeklyRemaining:   65000,
		WeeklyLimit:       70000,
		Source:            "pty",
	}))
	d = ReadClaudeQuotaRoutingDecision(now, DefaultClaudeQuotaStaleAfter)
	assert.True(t, d.PreferClaude)
	assert.True(t, d.Fresh)
	assert.True(t, d.SnapshotPresent)
}

func TestRefreshClaudeQuotaAsyncWritesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-quota.json")
	t.Setenv(claudeQuotaCacheEnv, path)

	var wg sync.WaitGroup
	wg.Add(1)
	captureCalled := false
	capture := func() (ClaudeQuotaSnapshot, error) {
		defer wg.Done()
		captureCalled = true
		return ClaudeQuotaSnapshot{
			CapturedAt:        time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC),
			FiveHourRemaining: 1234,
			FiveHourLimit:     10000,
			WeeklyRemaining:   55000,
			WeeklyLimit:       70000,
			Source:            "pty",
		}, nil
	}

	RefreshClaudeQuotaAsync(capture)
	wg.Wait()

	// Poll briefly for the file to land (goroutine writes after capture returns).
	deadline := time.Now().Add(2 * time.Second)
	var loaded *ClaudeQuotaSnapshot
	for time.Now().Before(deadline) {
		if s, ok := ReadClaudeQuotaFrom(path); ok {
			loaded = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, captureCalled)
	require.NotNil(t, loaded, "expected cache file to be written")
	assert.Equal(t, 1234, loaded.FiveHourRemaining)
	assert.Equal(t, "pty", loaded.Source)
}

func TestRefreshClaudeQuotaAsyncNilCapture(t *testing.T) {
	// Nil capture is a no-op and must not panic.
	RefreshClaudeQuotaAsync(nil)
}

func TestRefreshClaudeQuotaAsyncCaptureErrorLeavesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude-quota.json")
	t.Setenv(claudeQuotaCacheEnv, path)

	var wg sync.WaitGroup
	wg.Add(1)
	capture := func() (ClaudeQuotaSnapshot, error) {
		defer wg.Done()
		return ClaudeQuotaSnapshot{}, assert.AnError
	}
	RefreshClaudeQuotaAsync(capture)
	wg.Wait()
	// Give the goroutine a moment in case it would (wrongly) write.
	time.Sleep(20 * time.Millisecond)
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "cache must not be created when capture errors")
}
