package gitlock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gitLockHelperEnv = "DDX_GITLOCK_STALE_HELPER"
	gitLockRootEnv   = "DDX_GITLOCK_STALE_ROOT"
	gitLockCoordEnv  = "DDX_GITLOCK_STALE_COORD"
	gitLockRoleEnv   = "DDX_GITLOCK_STALE_ROLE"
)

type gitLockHelperResult struct {
	Removed bool   `json:"removed"`
	Reason  string `json:"reason,omitempty"`
	Error   string `json:"error,omitempty"`
}

func TestGitStaleLockBreakSingleWinner(t *testing.T) {
	if runGitLockStaleHelper(t) {
		return
	}
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("stale-single-winner"))
	coord := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	roles := []string{"left", "right"}
	commands := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmd := spawnGitLockHelper(ctx, "TestGitStaleLockBreakSingleWinner", "contender", root, coord, role)
		require.NoError(t, cmd.Start())
		commands = append(commands, cmd)
	}
	defer killGitLockHelpers(commands)
	require.NoError(t, waitForSignalCount(coord, ".source_observed", 2, 5*time.Second))
	for _, role := range roles {
		writeSignal(t, coord, role+".allow_source")
	}
	require.NoError(t, waitForSignalCount(coord, ".renamed", 1, 5*time.Second))
	require.NoError(t, waitForSignalCount(coord, ".json", 1, 5*time.Second))
	for _, role := range roles {
		writeSignal(t, coord, role+".allow_finish")
	}
	for _, cmd := range commands {
		require.NoError(t, cmd.Wait())
	}

	winners := 0
	for _, role := range roles {
		result := readGitLockHelperResult(t, filepath.Join(coord, role+".json"))
		require.Empty(t, result.Error)
		if result.Removed {
			winners++
		}
	}
	assert.Equal(t, 1, winners)
	assert.NoFileExists(t, lockPath)
	assertNoGitLockTombstones(t, lockPath)
	assert.FileExists(t, staleLockTransitionGuardPath(lockPath))
}

func TestGitStaleLockBreakLoserNeverDeletesWinner(t *testing.T) {
	if runGitLockStaleHelper(t) {
		return
	}
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("stale-S"))
	coord := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := spawnGitLockHelper(ctx, "TestGitStaleLockBreakLoserNeverDeletesWinner", "contender", root, coord, "A")
	b := spawnGitLockHelper(ctx, "TestGitStaleLockBreakLoserNeverDeletesWinner", "contender", root, coord, "B")
	require.NoError(t, a.Start())
	require.NoError(t, b.Start())
	commands := []*exec.Cmd{a, b}
	defer killGitLockHelpers(commands)

	// Both processes have opened and pinned the same stale S before either is
	// permitted to attempt the real advisory transition.
	require.NoError(t, waitForSignalCount(coord, ".source_observed", 2, 5*time.Second))
	writeSignal(t, coord, "A.allow_source")
	require.NoError(t, waitForSignal(filepath.Join(coord, "A.renamed"), 5*time.Second))

	// A remains inside the guard after renaming S. Install byte-distinct W,
	// then let A release the guard and dispose only its private tombstone.
	want := []byte("fresh-native-git-W")
	require.NoError(t, os.WriteFile(lockPath, want, 0o600))
	freshInfo, err := os.Lstat(lockPath)
	require.NoError(t, err)
	writeSignal(t, coord, "A.allow_finish")
	require.NoError(t, a.Wait())
	aResult := readGitLockHelperResult(t, filepath.Join(coord, "A.json"))
	require.True(t, aResult.Removed)
	require.Empty(t, aResult.Error)

	// Delayed B now proceeds with its old pinned S. This proof is deliberately
	// non-vacuous: B must acquire the released guard, then reject W because its
	// identity differs from the pinned source.
	writeSignal(t, coord, "B.allow_source")
	require.NoError(t, b.Wait())
	bResult := readGitLockHelperResult(t, filepath.Join(coord, "B.json"))
	assert.False(t, bResult.Removed)
	assert.Empty(t, bResult.Error)
	assert.Contains(t, bResult.Reason, "identity changed")
	assert.FileExists(t, filepath.Join(coord, "B.guard_acquired"))
	assert.NoFileExists(t, filepath.Join(coord, "B.guard_contended"))

	afterInfo, err := os.Lstat(lockPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(freshInfo, afterInfo))
	got, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assertNoGitLockTombstones(t, lockPath)
}

func TestGitStaleLockSourceIdentityMismatchFailsClosed(t *testing.T) {
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("stale-observed"))
	displaced := lockPath + ".displaced"
	want := []byte("fresh-replacement")
	var replaced bool
	result, err := recoverGitIndexLockObserved(root, func(stage staleLockTransitionStage) {
		if stage != staleLockStageSourceObserved || replaced {
			return
		}
		replaced = true
		require.NoError(t, os.Rename(lockPath, displaced))
		require.NoError(t, os.WriteFile(lockPath, want, 0o600))
	}, absentOwnerProbe)
	require.NoError(t, err)
	assert.False(t, result.Removed)
	assert.Contains(t, result.Reason, "identity changed")
	got, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assertNoGitLockTombstones(t, lockPath)
}

func TestGitStaleLockGuardCrashReleasesWithoutReplacingSidecar(t *testing.T) {
	if runGitLockStaleHelper(t) {
		return
	}
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("stale-crash"))
	coord := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder := spawnGitLockHelper(ctx, "TestGitStaleLockGuardCrashReleasesWithoutReplacingSidecar", "guard-holder", root, coord, "holder")
	require.NoError(t, holder.Start())
	require.NoError(t, waitForSignal(filepath.Join(coord, "holder.guard_acquired"), 5*time.Second))
	guardPath := staleLockTransitionGuardPath(lockPath)
	before, err := os.Lstat(guardPath)
	require.NoError(t, err)
	require.NoError(t, holder.Process.Kill())
	require.Error(t, holder.Wait())

	require.Eventually(t, func() bool {
		guard, acquired, acquireErr := tryAcquireStaleLockTransitionGuard(lockPath, nil)
		if acquireErr != nil || !acquired {
			return false
		}
		require.NoError(t, releaseStaleLockTransitionGuard(guard))
		return true
	}, 5*time.Second, 10*time.Millisecond)
	after, err := os.Lstat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))

	result, err := recoverGitIndexLockObserved(root, nil, absentOwnerProbe)
	require.NoError(t, err)
	assert.True(t, result.Removed)
	finalInfo, err := os.Lstat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, finalInfo))
}

func TestGitStaleLockGuardContentionDoesNotExtendRecoveryBudget(t *testing.T) {
	if runGitLockStaleHelper(t) {
		return
	}
	root := initGitLockRepo(t)
	writeOldIndexLock(t, root, []byte("stale-contended"))
	coord := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder := spawnGitLockHelper(ctx, "TestGitStaleLockGuardContentionDoesNotExtendRecoveryBudget", "guard-holder", root, coord, "holder")
	require.NoError(t, holder.Start())
	defer killGitLockHelpers([]*exec.Cmd{holder})
	require.NoError(t, waitForSignal(filepath.Join(coord, "holder.guard_acquired"), 5*time.Second))
	var sawContended, sawRenamed bool
	started := time.Now()
	result, err := recoverGitIndexLockObserved(root, func(stage staleLockTransitionStage) {
		sawContended = sawContended || stage == staleLockStageGuardContended
		sawRenamed = sawRenamed || stage == staleLockStageRenamed
	}, absentOwnerProbe)
	elapsed := time.Since(started)
	require.NoError(t, err)
	assert.False(t, result.Removed)
	assert.Contains(t, result.Reason, "guard contended")
	assert.True(t, sawContended)
	assert.False(t, sawRenamed)
	assert.Less(t, elapsed, 250*time.Millisecond)
}

func TestRecoverGitIndexLockOwnerProbeUnknownFailsClosed(t *testing.T) {
	cleanNoMatchErr := errors.New("exit status 1")
	tests := []struct {
		name  string
		probe ownerProbeResult
	}{
		{
			name: "unavailable",
			probe: probeIndexLockOwnerWith("ignored", func(string) (string, error) {
				return "", errors.New("not found")
			}, nil),
		},
		{name: "timeout", probe: classifyLsofProbe(nil, nil, -1, context.DeadlineExceeded, true)},
		{name: "execution_error", probe: classifyLsofProbe(nil, nil, -1, errors.New("exec failed"), false)},
		{name: "stderr", probe: classifyLsofProbe([]byte(strconv.Itoa(os.Getpid())), []byte("warning"), 0, nil, false)},
		{name: "malformed", probe: classifyLsofProbe([]byte("not-a-pid"), nil, 0, nil, false)},
		{name: "exit_zero_empty", probe: classifyLsofProbe(nil, nil, 0, nil, false)},
		{name: "ambiguous_dead_set", probe: classifyLsofProbe([]byte("2147483000\n2147483001\n"), nil, 0, nil, false)},
	}
	absent := classifyLsofProbe(nil, nil, 1, cleanNoMatchErr, false)
	require.Equal(t, ownerProbeAbsent, absent.kind, "only clean exit-1 no-match is proven absent")
	live := classifyLsofProbe([]byte(fmt.Sprintf("2147483000\n%d\n", os.Getpid())), nil, 0, nil, false)
	require.Equal(t, ownerProbeLive, live.kind, "all PIDs must be parsed and any live owner wins")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, ownerProbeUnknown, test.probe.kind)
			root := initGitLockRepo(t)
			lockPath := writeOldIndexLock(t, root, []byte("must-survive-unknown"))
			result, err := recoverGitIndexLockObserved(root, nil, func(string) ownerProbeResult { return test.probe })
			require.NoError(t, err)
			assert.False(t, result.Removed)
			assert.Contains(t, result.Reason, "unknown")
			got, readErr := os.ReadFile(lockPath)
			require.NoError(t, readErr)
			assert.Equal(t, []byte("must-survive-unknown"), got)
			assertNoGitLockTombstones(t, lockPath)
		})
	}
}

func TestRecoverGitIndexLockLiveOwnerFailsClosed(t *testing.T) {
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("live-owner"))
	result, err := recoverGitIndexLockObserved(root, nil, func(string) ownerProbeResult {
		return ownerProbeResult{kind: ownerProbeLive, pid: os.Getpid()}
	})
	require.NoError(t, err)
	assert.False(t, result.Removed)
	assert.True(t, result.OwnerAlive)
	assert.FileExists(t, lockPath)
}

func TestRecoverGitIndexLockStaleUnownedStillRecovers(t *testing.T) {
	root := initGitLockRepo(t)
	lockPath := writeOldIndexLock(t, root, []byte("stale-unowned"))
	result, err := recoverGitIndexLockObserved(root, nil, absentOwnerProbe)
	require.NoError(t, err)
	assert.True(t, result.Removed)
	assert.NoFileExists(t, lockPath)
}

func TestRecoverGitIndexLockMalformedPathFailsSafe(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		root := initGitLockRepo(t)
		lockPath, err := resolveIndexLockPath(root)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(lockPath, 0o700))
		result, recoverErr := recoverGitIndexLockObserved(root, nil, absentOwnerProbe)
		require.NoError(t, recoverErr)
		assert.False(t, result.Removed)
		assert.Contains(t, result.Reason, "not a regular file")
		assert.DirExists(t, lockPath)
	})

	t.Run("symlink", func(t *testing.T) {
		root := initGitLockRepo(t)
		lockPath, err := resolveIndexLockPath(root)
		require.NoError(t, err)
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.WriteFile(target, []byte("target"), 0o600))
		if err := os.Symlink(target, lockPath); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink unavailable: %v", err)
			}
			require.NoError(t, err)
		}
		result, recoverErr := recoverGitIndexLockObserved(root, nil, absentOwnerProbe)
		require.NoError(t, recoverErr)
		assert.False(t, result.Removed)
		assert.Contains(t, result.Reason, "not a regular file")
		assert.FileExists(t, target)
	})
}

func TestRecoverGitIndexLockLinkedWorktreeUsesRealGitDir(t *testing.T) {
	root := initGitLockRepo(t)
	writeAndCommitGitLockFixture(t, root)
	linked := filepath.Join(t.TempDir(), "linked")
	runGitLockGit(t, root, "worktree", "add", "-b", "linked-proof", linked)
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", root, "worktree", "remove", "--force", linked).Run()
	})
	expectedRaw := runGitLockGitOutput(t, linked, "rev-parse", "--path-format=absolute", "--git-path", "index.lock")
	expected := strings.TrimSpace(expectedRaw)
	resolved, err := resolveIndexLockPath(linked)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(expected), resolved)
	require.NoError(t, os.WriteFile(resolved, []byte("linked-stale"), 0o600))
	old := time.Now().Add(-2 * StaleAge)
	require.NoError(t, os.Chtimes(resolved, old, old))
	result, err := recoverGitIndexLockObserved(linked, nil, absentOwnerProbe)
	require.NoError(t, err)
	assert.True(t, result.Removed)
	guardPath := staleLockTransitionGuardPath(resolved)
	assert.FileExists(t, guardPath)
	assert.Equal(t, filepath.Dir(resolved), filepath.Dir(guardPath))
	status := runGitLockGitOutput(t, linked, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status))
}

func absentOwnerProbe(string) ownerProbeResult {
	return ownerProbeResult{kind: ownerProbeAbsent, detail: "no owner found"}
}

func runGitLockStaleHelper(t *testing.T) bool {
	t.Helper()
	mode := os.Getenv(gitLockHelperEnv)
	if mode == "" {
		return false
	}
	root := os.Getenv(gitLockRootEnv)
	coord := os.Getenv(gitLockCoordEnv)
	role := os.Getenv(gitLockRoleEnv)
	require.NotEmpty(t, root)
	require.NotEmpty(t, coord)
	require.NotEmpty(t, role)
	lockPath, err := resolveIndexLockPath(root)
	require.NoError(t, err)

	switch mode {
	case "contender":
		result, recoverErr := recoverGitIndexLockObserved(root, func(stage staleLockTransitionStage) {
			writeSignal(t, coord, role+"."+string(stage))
			switch stage {
			case staleLockStageSourceObserved:
				require.NoError(t, waitForSignal(filepath.Join(coord, role+".allow_source"), 5*time.Second))
			case staleLockStageRenamed:
				require.NoError(t, waitForSignal(filepath.Join(coord, role+".allow_finish"), 5*time.Second))
			}
		}, absentOwnerProbe)
		helperResult := gitLockHelperResult{Removed: result.Removed, Reason: result.Reason}
		if recoverErr != nil {
			helperResult.Error = recoverErr.Error()
		}
		writeGitLockHelperResult(t, filepath.Join(coord, role+".json"), helperResult)
	case "guard-holder":
		guard, acquired, guardErr := tryAcquireStaleLockTransitionGuard(lockPath, func(stage staleLockTransitionStage) {
			if stage == staleLockStageGuardAcquired {
				writeSignal(t, coord, role+".guard_acquired")
			}
		})
		require.NoError(t, guardErr)
		require.True(t, acquired)
		defer func() { _ = releaseStaleLockTransitionGuard(guard) }()
		select {}
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	return true
}

func spawnGitLockHelper(ctx context.Context, testName, mode, root, coord, role string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		gitLockHelperEnv+"="+mode,
		gitLockRootEnv+"="+root,
		gitLockCoordEnv+"="+coord,
		gitLockRoleEnv+"="+role,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func initGitLockRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitLockGit(t, root, "init")
	runGitLockGit(t, root, "config", "user.name", "DDx Test")
	runGitLockGit(t, root, "config", "user.email", "ddx@example.invalid")
	return root
}

func writeAndCommitGitLockFixture(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture"), 0o600))
	runGitLockGit(t, root, "add", "fixture.txt")
	runGitLockGit(t, root, "commit", "-m", "fixture")
}

func runGitLockGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runGitLockGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func writeOldIndexLock(t *testing.T, root string, contents []byte) string {
	t.Helper()
	lockPath, err := resolveIndexLockPath(root)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath, contents, 0o600))
	old := time.Now().Add(-2 * StaleAge)
	require.NoError(t, os.Chtimes(lockPath, old, old))
	return lockPath
}

func writeSignal(t *testing.T, coord, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(coord, name), []byte("ready"), 0o600))
}

func waitForSignal(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForSignalCount(dir, suffix string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		count := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), suffix) {
				count++
			}
		}
		if count >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %d *%s files", want, suffix)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeGitLockHelperResult(t *testing.T, path string, result gitLockHelperResult) {
	t.Helper()
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func readGitLockHelperResult(t *testing.T, path string) gitLockHelperResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var result gitLockHelperResult
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func killGitLockHelpers(commands []*exec.Cmd) {
	for _, cmd := range commands {
		if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
}

func assertNoGitLockTombstones(t *testing.T, lockPath string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(lockPath), filepath.Base(lockPath)+".tombstone.*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
