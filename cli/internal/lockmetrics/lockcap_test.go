package lockmetrics

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scrubbedGitEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "GIT_CONFIG_NOSYSTEM=1")
}

func initTrackerLockRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = scrubbedGitEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@ddx.test")
	run("config", "user.name", "DDx Test")

	require.NoError(t, os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644))
	run("add", "seed.txt")
	run("commit", "-m", "chore: initial seed")

	require.NoError(t, os.MkdirAll(filepath.Join(root, ddxroot.DirName), 0o755))
	return root
}

func requireLockCapViolation(t *testing.T, path string) Violation {
	t.Helper()

	var violation Violation
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		return json.Unmarshal(data, &violation) == nil
	}, 2*time.Second, 10*time.Millisecond, "cap violation evidence was not written")
	return violation
}

// TestLockCap_DefaultIndexLockCap10s asserts the default cap for
// .git/index.lock is 10s and is overridable via DDX_LOCK_CAP_INDEX_MS.
func TestLockCap_DefaultIndexLockCap10s(t *testing.T) {
	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "")
	assert.Equal(t, 10*time.Second, CapFor("index.lock"),
		"default index.lock cap must be 10s")

	t.Setenv("DDX_LOCK_CAP_INDEX_MS", "2500")
	assert.Equal(t, 2500*time.Millisecond, CapFor("index.lock"),
		"index.lock cap must be configurable via DDX_LOCK_CAP_INDEX_MS")
}

// TestLockCapViolationDoesNotReleaseLiveLock proves that a cap is an
// observation threshold: evidence is written while the owner retains the live
// lock and a contender still cannot enter.
func TestLockCapViolationDoesNotReleaseLiveLock(t *testing.T) {
	SetSink(nil)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	cfg := CapConfig{Cap: 30 * time.Millisecond, LockPath: lockPath, EvidenceDir: dir}
	var violation Violation
	err := InstrumentCapped("index.lock", "index.commit", cfg, func() error {
		violation = requireLockCapViolation(t, filepath.Join(dir, "lock-violation.json"))

		contender, contenderErr := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if contender != nil {
			require.NoError(t, contender.Close())
		}
		require.Error(t, contenderErr, "a contender must not enter while the live owner still holds the lock")
		assert.True(t, os.IsExist(contenderErr), "contender failure must report the owner's existing lock")

		_, statErr := os.Stat(lockPath)
		require.NoError(t, statErr, "cap observation must retain the live lock")
		return os.Remove(lockPath)
	})
	require.NoError(t, err)

	assert.Equal(t, "index.lock", violation.LockName)
	assert.GreaterOrEqual(t, violation.ActualHoldMS, cfg.Cap.Milliseconds())
	assert.NotEmpty(t, violation.Stack)
}

// TestLockCapNormalRelease proves that an owner can release normally after an
// over-cap observation and a contender can acquire only after that release.
func TestLockCapNormalRelease(t *testing.T) {
	SetSink(nil)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tracker.lock")
	require.NoError(t, os.Mkdir(lockPath, 0o755))

	cfg := CapConfig{Cap: 30 * time.Millisecond, LockPath: lockPath, EvidenceDir: dir}
	require.NoError(t, InstrumentCapped("tracker.lock", "tracker.commit", cfg, func() error {
		requireLockCapViolation(t, filepath.Join(dir, "lock-violation.json"))
		return os.Remove(lockPath)
	}))

	require.NoError(t, os.Mkdir(lockPath, 0o755),
		"a contender must acquire after the owner performs its normal release")
}

// TestLockCap_ViolationWrittenToEvidence asserts a lock-violation.json appears
// under the worker's evidence directory carrying the required fields.
func TestLockCap_ViolationWrittenToEvidence(t *testing.T) {
	SetSink(nil)
	dir := t.TempDir()
	evidence := filepath.Join(dir, "20260527T021507-08bcbb0f")
	lockPath := filepath.Join(dir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	cfg := CapConfig{Cap: 30 * time.Millisecond, LockPath: lockPath, EvidenceDir: evidence}
	require.NoError(t, InstrumentCapped("index.lock", "index.commit", cfg, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}))

	data, err := os.ReadFile(filepath.Join(evidence, "lock-violation.json"))
	require.NoError(t, err, "lock-violation.json must exist under the evidence dir")

	var v Violation
	require.NoError(t, json.Unmarshal(data, &v))
	assert.Equal(t, "index.lock", v.LockName)
	assert.Equal(t, int64(30), v.CapMS)
	assert.GreaterOrEqual(t, v.ActualHoldMS, int64(30),
		"actual_hold_ms must be at least the cap")
	assert.Equal(t, os.Getpid(), v.HolderPID)
	assert.NotEmpty(t, v.Stack, "violation record must include a stack trace")
}

// TestLockCap_ViolationLoggedAsError asserts an error-severity event is
// emitted via the metric helper when the cap is exceeded.
func TestLockCap_ViolationLoggedAsError(t *testing.T) {
	snapshot := capture(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	cfg := CapConfig{Cap: 30 * time.Millisecond, LockPath: lockPath, EvidenceDir: dir}
	require.NoError(t, InstrumentCapped("index.lock", "index.commit", cfg, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}))

	var violation *Event
	for _, ev := range snapshot() {
		if ev.Event == "violation" {
			ev := ev
			violation = &ev
		}
	}
	require.NotNil(t, violation, "expected a violation event")
	assert.Equal(t, "error", violation.Severity)
	assert.Equal(t, "index.lock", violation.LockName)
	assert.Equal(t, int64(30), violation.CapMS)
}

// TestLockCap_WithinCapDoesNotForceRelease asserts that a hold under the cap
// neither removes the lock nor records a violation.
func TestLockCap_WithinCapDoesNotForceRelease(t *testing.T) {
	snapshot := capture(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o644))

	cfg := CapConfig{Cap: 200 * time.Millisecond, LockPath: lockPath, EvidenceDir: dir}
	require.NoError(t, InstrumentCapped("index.lock", "index.commit", cfg, func() error {
		return nil
	}))

	_, statErr := os.Stat(lockPath)
	assert.NoError(t, statErr, "lock file must survive a within-cap hold")
	_, vErr := os.Stat(filepath.Join(dir, "lock-violation.json"))
	assert.True(t, os.IsNotExist(vErr), "no violation record for a within-cap hold")
	for _, ev := range snapshot() {
		assert.NotEqual(t, "violation", ev.Event, "no violation event for a within-cap hold")
	}
}

// TestLockCap_DefaultTrackerLockCap30s asserts the default cap for
// .ddx/.git-tracker.lock is 30s and is overridable via DDX_LOCK_CAP_TRACKER_MS.
func TestLockCap_DefaultTrackerLockCap30s(t *testing.T) {
	t.Setenv("DDX_LOCK_CAP_TRACKER_MS", "")
	assert.Equal(t, 30*time.Second, CapFor("tracker.lock"))

	t.Setenv("DDX_LOCK_CAP_TRACKER_MS", "5000")
	assert.Equal(t, 5*time.Second, CapFor("tracker.lock"))
}

// TestLockCap_NoEnforcementWhenDisabled asserts that with cap enforcement off
// (the default), Instrument resolves no cap for the named locks.
func TestLockCap_NoEnforcementWhenDisabled(t *testing.T) {
	SetCapEnforcement("", "")
	t.Cleanup(func() { SetCapEnforcement("", "") })
	assert.Equal(t, CapConfig{}, resolveCapConfig("index.lock"))
	assert.Equal(t, CapConfig{}, resolveCapConfig("tracker.lock"))
}

// TestLockCap_EnforcementResolvesLockPaths asserts that enabling enforcement
// resolves the on-disk lock paths for the two named locks under projectRoot.
func TestLockCap_EnforcementResolvesLockPaths(t *testing.T) {
	root := t.TempDir()
	SetCapEnforcement(root, filepath.Join(root, "evidence"))
	t.Cleanup(func() { SetCapEnforcement("", "") })

	idx := resolveCapConfig("index.lock")
	assert.Equal(t, filepath.Join(root, ".git", "index.lock"), idx.LockPath)
	assert.Greater(t, idx.Cap, time.Duration(0))

	trk := resolveCapConfig("tracker.lock")
	assert.Contains(t, trk.LockPath, ".git-tracker.lock")
	assert.Greater(t, trk.Cap, time.Duration(0))
}

// TestLockCap_TrackerLockPathUsesSharedMainGitRoot asserts that tracker.lock
// cap enforcement resolves through the shared main-worktree DDx root when the
// request originates from a linked worktree.
func TestLockCap_TrackerLockPathUsesSharedMainGitRoot(t *testing.T) {
	root := initTrackerLockRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = scrubbedGitEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("worktree", "add", "--detach", linked)
	t.Cleanup(func() { runGit("worktree", "remove", "--force", linked) })
	t.Cleanup(func() { SetCapEnforcement("", "") })

	SetCapEnforcement(linked, filepath.Join(linked, "evidence"))

	trk := resolveCapConfig("tracker.lock")
	wantRoot := SharedMainGitLockRoot(linked)
	assert.Equal(t, root, wantRoot, "linked worktrees must resolve tracker caps through the primary workspace")
	assert.Equal(t, filepath.Join(root, ddxroot.DirName, ".git-tracker.lock"), trk.LockPath)
	assert.Equal(t, filepath.Join(wantRoot, ddxroot.DirName, ".git-tracker.lock"), trk.LockPath)
	assert.Greater(t, trk.Cap, time.Duration(0))
}
