package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	trackerStaleHelperEnv   = "DDX_TRACKER_STALE_BREAK_HELPER"
	trackerStaleModeEnv     = "DDX_TRACKER_STALE_BREAK_MODE"
	trackerStaleRootEnv     = "DDX_TRACKER_STALE_BREAK_ROOT"
	trackerStaleLockDirEnv  = "DDX_TRACKER_STALE_BREAK_LOCK_DIR"
	trackerStaleCoordDirEnv = "DDX_TRACKER_STALE_BREAK_COORD_DIR"
	trackerStaleRoleEnv     = "DDX_TRACKER_STALE_BREAK_ROLE"
)

type trackerStaleBreakResult struct {
	Broke bool   `json:"broke"`
	Error string `json:"error,omitempty"`
}

// TestTrackerStaleLockBreakSingleWinner uses coordinated cross-process
// contenders against one stale process-shared main-git lock and proves
// exactly one contender owns the guarded stale-to-tombstone disposal.
func TestTrackerStaleLockBreakSingleWinner(t *testing.T) {
	if runTrackerStaleBreakHelper(t) {
		return
	}

	root := initTrackerRepo(t)
	lockDir := trackerLockPath(root)
	writeStaleTrackerLockDir(t, lockDir, os.Getpid(), time.Now().Add(-3*trackerLockStaleAge))
	coordDir := filepath.Join(t.TempDir(), "single-winner")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	roles := []string{"left", "right"}
	commands := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmd := spawnTrackerStaleBreakHelper(ctx, "TestTrackerStaleLockBreakSingleWinner", "single-winner", role, root, lockDir, coordDir)
		require.NoError(t, cmd.Start())
		commands = append(commands, cmd)
	}
	defer killTrackerStaleBreakHelpers(commands)

	require.NoError(t, waitForTrackerStaleBreakFiles(coordDir, ".ready", len(roles), 5*time.Second))
	// Signals after readiness: both helpers are waiting on start before any
	// attempt, so both observe the same stale object without sleep ordering.
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "start"), []byte("go"), 0o644))
	for _, cmd := range commands {
		require.NoError(t, cmd.Wait())
	}

	winners := 0
	for _, role := range roles {
		result := readTrackerStaleBreakResult(t, filepath.Join(coordDir, role+".json"))
		require.Empty(t, result.Error, "role %s", role)
		if result.Broke {
			winners++
		}
	}
	assert.Equal(t, 1, winners, "exactly one process may own the guarded stale-to-tombstone transition")
	assert.NoDirExists(t, lockDir)
	assertNoTrackerStaleBreakTombstones(t, lockDir)
	guardPath := trackerStaleLockBreakGuardPath(lockDir)
	assert.True(t, strings.HasSuffix(guardPath, ".lock"))
	assert.FileExists(t, guardPath, "stable guard sidecar must survive disposal")
}

// TestTrackerStaleLockBreakLoserNeverDeletesWinner forces the delayed-observer
// counterexample from an internal guard-stage barrier and proves the delayed
// contender cannot move, delete, or alter the fresh replacement pid,
// acquired_at, or owner_token bytes.
func TestTrackerStaleLockBreakLoserNeverDeletesWinner(t *testing.T) {
	if runTrackerStaleBreakHelper(t) {
		return
	}

	root := initTrackerRepo(t)
	lockDir := trackerLockPath(root)
	writeStaleTrackerLockDir(t, lockDir, os.Getpid(), time.Now().Add(-3*trackerLockStaleAge))
	coordDir := filepath.Join(t.TempDir(), "pre-rename-replacement")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	contender := spawnTrackerStaleBreakHelper(ctx, "TestTrackerStaleLockBreakLoserNeverDeletesWinner", "pre-rename-contender", "contender", root, lockDir, coordDir)
	require.NoError(t, contender.Start())
	defer killTrackerStaleBreakHelpers([]*exec.Cmd{contender})

	// Contender signal is emitted after advisory-guard acquisition and after
	// under-guard stale classification, at the last internal stage before rename.
	require.NoError(t, waitForTrackerStaleBreakFile(filepath.Join(coordDir, "classified-stale"), 5*time.Second))
	assert.FileExists(t, filepath.Join(coordDir, "guard-acquired"), "barrier must follow actual advisory-guard acquisition")

	// Model an ordinary owner replacing the canonical directory in that window.
	displaced := filepath.Join(root, "displaced-stale.lock")
	require.NoError(t, os.Rename(lockDir, displaced))
	require.NoError(t, os.Mkdir(lockDir, 0o755))
	wantPID := []byte(fmt.Sprintf("%d", os.Getpid()))
	wantAcquiredAt := []byte(time.Now().UTC().Format(time.RFC3339))
	wantOwnerToken := []byte(strings.Repeat("a", 64))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), wantPID, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), wantAcquiredAt, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, trackerLockOwnerTokenFile), wantOwnerToken, 0o600))
	freshIdentity, err := os.Lstat(lockDir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "resume"), []byte("resume"), 0o644))
	require.NoError(t, contender.Wait())
	result := readTrackerStaleBreakResult(t, filepath.Join(coordDir, "contender.json"))
	assert.False(t, result.Broke, "stale classification of the displaced inode must not authorize renaming its fresh replacement")
	assert.Empty(t, result.Error)

	afterIdentity, err := os.Lstat(lockDir)
	require.NoError(t, err)
	assert.True(t, os.SameFile(freshIdentity, afterIdentity), "delayed contender must not replace the fresh canonical inode")
	assertTrackerFileBytesEqual(t, filepath.Join(lockDir, "pid"), wantPID)
	assertTrackerFileBytesEqual(t, filepath.Join(lockDir, "acquired_at"), wantAcquiredAt)
	assertTrackerFileBytesEqual(t, filepath.Join(lockDir, trackerLockOwnerTokenFile), wantOwnerToken)
	require.NoError(t, os.RemoveAll(displaced))
	assertNoTrackerStaleBreakTombstones(t, lockDir)
}

// TestTrackerStaleLockMetadataPolicyPreserved proves valid dead-PID OR valid
// over-age acquired_at remains sufficient, while missing/malformed metadata
// without a valid stale criterion is preserved.
func TestTrackerStaleLockMetadataPolicyPreserved(t *testing.T) {
	t.Run("malformed metadata preserves canonical", func(t *testing.T) {
		root := initTrackerRepo(t)
		lockDir := trackerLockPath(root)
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte("not-a-pid"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte("not-a-time"), 0o644))

		assert.False(t, mustBreakStaleTrackerLock(t, lockDir))
		assert.DirExists(t, lockDir, "malformed current metadata supplies no guarded stale criterion")
	})

	t.Run("missing metadata preserves canonical", func(t *testing.T) {
		root := initTrackerRepo(t)
		lockDir := trackerLockPath(root)
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		// Empty directory: neither pid nor acquired_at present.
		assert.False(t, mustBreakStaleTrackerLock(t, lockDir))
		assert.DirExists(t, lockDir, "missing metadata alone is not a stale criterion")
	})

	t.Run("over-age criterion does not require valid PID", func(t *testing.T) {
		root := initTrackerRepo(t)
		lockDir := trackerLockPath(root)
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte("not-a-pid"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"),
			[]byte(time.Now().Add(-3*trackerLockStaleAge).UTC().Format(time.RFC3339)), 0o644))

		assert.True(t, mustBreakStaleTrackerLock(t, lockDir), "valid over-age metadata remains an independent stale criterion")
		assert.NoDirExists(t, lockDir)
		assertNoTrackerStaleBreakTombstones(t, lockDir)
	})

	t.Run("dead PID criterion does not require acquired_at", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows trackerProcessAlive conservatively treats PIDs as live")
		}
		root := initTrackerRepo(t)
		lockDir := trackerLockPath(root)
		deadProcess := exec.Command(os.Args[0], "-test.run=^$")
		require.NoError(t, deadProcess.Start())
		deadPID := deadProcess.Process.Pid
		require.NoError(t, deadProcess.Wait())
		require.NoError(t, os.MkdirAll(lockDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", deadPID)), 0o644))

		assert.True(t, mustBreakStaleTrackerLock(t, lockDir), "valid dead PID remains an independent stale criterion")
		assert.NoDirExists(t, lockDir)
		assertNoTrackerStaleBreakTombstones(t, lockDir)
	})

	t.Run("malformed stale regular file is disposed under guard", func(t *testing.T) {
		root := initTrackerRepo(t)
		lockPath := trackerLockPath(root)
		require.NoError(t, os.WriteFile(lockPath, []byte("stale"), 0o644))
		staleTime := time.Now().Add(-2 * trackerLockStaleAge)
		require.NoError(t, os.Chtimes(lockPath, staleTime, staleTime))

		assert.True(t, mustBreakStaleTrackerLock(t, lockPath))
		_, err := os.Lstat(lockPath)
		assert.True(t, os.IsNotExist(err), "stale regular file must be renamed and removed via its tombstone")
		assertNoTrackerStaleBreakTombstones(t, lockPath)
	})
}

func runTrackerStaleBreakHelper(t *testing.T) bool {
	t.Helper()
	if os.Getenv(trackerStaleHelperEnv) != "1" {
		return false
	}

	switch os.Getenv(trackerStaleModeEnv) {
	case "single-winner":
		runTrackerSingleWinnerStaleBreakHelper(t)
	case "pre-rename-contender":
		runTrackerPreRenameContenderStaleBreakHelper(t)
	default:
		t.Fatalf("unknown tracker stale-break helper mode %q", os.Getenv(trackerStaleModeEnv))
	}
	return true
}

func runTrackerSingleWinnerStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, role := trackerStaleBreakHelperInputs(t)
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, role+".ready"), []byte("ready"), 0o644))
	require.NoError(t, waitForTrackerStaleBreakFile(filepath.Join(coordDir, "start"), 5*time.Second))
	writeTrackerStaleBreakResult(t, filepath.Join(coordDir, role+".json"), trackerStaleBreakResult{
		Broke: mustBreakStaleTrackerLock(t, lockDir),
	})
}

func runTrackerPreRenameContenderStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, role := trackerStaleBreakHelperInputs(t)
	broke, err := breakStaleTrackerLockObserved(lockDir, func(stage trackerStaleLockGuardStage) {
		switch stage {
		case trackerStaleGuardStageAcquired:
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "guard-acquired"), []byte("acquired"), 0o644))
		case trackerStaleGuardStageBeforeRename:
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "classified-stale"), []byte("classified"), 0o644))
			require.NoError(t, waitForTrackerStaleBreakFile(filepath.Join(coordDir, "resume"), 5*time.Second))
		}
	})
	result := trackerStaleBreakResult{Broke: broke}
	if err != nil {
		result.Error = err.Error()
	}
	writeTrackerStaleBreakResult(t, filepath.Join(coordDir, role+".json"), result)
}

func trackerStaleBreakHelperInputs(t *testing.T) (coordDir, lockDir, role string) {
	t.Helper()
	coordDir = os.Getenv(trackerStaleCoordDirEnv)
	lockDir = os.Getenv(trackerStaleLockDirEnv)
	role = os.Getenv(trackerStaleRoleEnv)
	require.NotEmpty(t, coordDir)
	require.NotEmpty(t, lockDir)
	return coordDir, lockDir, role
}

func writeStaleTrackerLockDir(t *testing.T, lockDir string, pid int, acquiredAt time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", pid)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(acquiredAt.UTC().Format(time.RFC3339)), 0o644))
}

func spawnTrackerStaleBreakHelper(ctx context.Context, testName, mode, role, root, lockDir, coordDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		trackerStaleHelperEnv+"=1",
		trackerStaleModeEnv+"="+mode,
		trackerStaleRootEnv+"="+root,
		trackerStaleLockDirEnv+"="+lockDir,
		trackerStaleCoordDirEnv+"="+coordDir,
		trackerStaleRoleEnv+"="+role,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func killTrackerStaleBreakHelpers(commands []*exec.Cmd) {
	for _, cmd := range commands {
		if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
}

func waitForTrackerStaleBreakFiles(dir, suffix string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
				count++
			}
		}
		if count >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %d %s files in %s", want, suffix, dir)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForTrackerStaleBreakFile(path string, timeout time.Duration) error {
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

func writeTrackerStaleBreakResult(t *testing.T, path string, result trackerStaleBreakResult) {
	t.Helper()
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
}

func readTrackerStaleBreakResult(t *testing.T, path string) trackerStaleBreakResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var result trackerStaleBreakResult
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func assertTrackerFileBytesEqual(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func assertNoTrackerStaleBreakTombstones(t *testing.T, lockDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone.*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "breaker must remove only its uniquely owned tombstone")
}

func mustBreakStaleTrackerLock(t *testing.T, lockDir string) bool {
	t.Helper()
	broke, err := breakStaleTrackerLock(lockDir)
	require.NoError(t, err)
	return broke
}
