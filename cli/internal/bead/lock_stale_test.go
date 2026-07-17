package bead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	staleBreakHelperEnv   = "DDX_BEAD_STALE_BREAK_HELPER"
	staleBreakModeEnv     = "DDX_BEAD_STALE_BREAK_MODE"
	staleBreakStoreDirEnv = "DDX_BEAD_STALE_BREAK_STORE_DIR"
	staleBreakLockDirEnv  = "DDX_BEAD_STALE_BREAK_LOCK_DIR"
	staleBreakCoordDirEnv = "DDX_BEAD_STALE_BREAK_COORD_DIR"
	staleBreakRoleEnv     = "DDX_BEAD_STALE_BREAK_ROLE"
)

type staleBreakResult struct {
	Broke       bool   `json:"broke"`
	FirstBroke  bool   `json:"first_broke,omitempty"`
	SecondBroke bool   `json:"second_broke,omitempty"`
	PID         string `json:"pid,omitempty"`
	AcquiredAt  string `json:"acquired_at,omitempty"`
	OwnerToken  string `json:"owner_token,omitempty"`
	Error       string `json:"error,omitempty"`
}

func TestStaleLockBreakSingleWinner(t *testing.T) {
	if runStaleBreakHelper(t) {
		return
	}

	store, projectDir := newStaleBreakFixture(t)
	writeStaleCollectionLock(t, store.LockDir, os.Getpid(), time.Now().Add(-3*time.Hour))
	coordDir := filepath.Join(projectDir, "single-winner")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	roles := []string{"left", "right"}
	commands := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmd := spawnStaleBreakHelper(ctx, "TestStaleLockBreakSingleWinner", "single-winner", role, store.Dir, store.LockDir, coordDir)
		require.NoError(t, cmd.Start())
		commands = append(commands, cmd)
	}
	defer killStaleBreakHelpers(commands)

	require.NoError(t, waitForStaleBreakFiles(coordDir, ".ready", len(roles), 5*time.Second))
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "start"), []byte("go"), 0o644))
	for _, cmd := range commands {
		require.NoError(t, cmd.Wait())
	}

	winners := 0
	for _, role := range roles {
		result := readStaleBreakResult(t, filepath.Join(coordDir, role+".json"))
		if result.Broke {
			winners++
		}
	}
	assert.Equal(t, 1, winners, "exactly one process may own the guarded stale-to-tombstone transition")
	assert.NoDirExists(t, store.LockDir)
	assertNoStaleBreakTombstones(t, store.LockDir)
	guardPath := staleLockBreakGuardPath(store.LockDir)
	assert.True(t, strings.HasSuffix(guardPath, ".lock"))
	assert.FileExists(t, guardPath, "stable guard sidecar must survive disposal")
}

func TestStaleLockBreakLoserNeverDeletesWinner(t *testing.T) {
	if runStaleBreakHelper(t) {
		return
	}

	store, projectDir := newStaleBreakFixture(t)
	writeStaleCollectionLock(t, store.LockDir, os.Getpid(), time.Now().Add(-3*time.Hour))
	coordDir := filepath.Join(projectDir, "delayed-observer")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	winner := spawnStaleBreakHelper(ctx, "TestStaleLockBreakLoserNeverDeletesWinner", "barrier-winner", "winner", store.Dir, store.LockDir, coordDir)
	loser := spawnStaleBreakHelper(ctx, "TestStaleLockBreakLoserNeverDeletesWinner", "barrier-loser", "loser", store.Dir, store.LockDir, coordDir)
	require.NoError(t, winner.Start())
	require.NoError(t, loser.Start())
	defer killStaleBreakHelpers([]*exec.Cmd{winner, loser})

	// Waiting for the loser first avoids letting a successful winner block on
	// its final loser-done barrier while the parent waits on the winner.
	require.NoError(t, loser.Wait())
	require.NoError(t, winner.Wait())

	winnerResult := readStaleBreakResult(t, filepath.Join(coordDir, "winner.json"))
	loserResult := readStaleBreakResult(t, filepath.Join(coordDir, "loser.json"))
	assert.True(t, winnerResult.Broke)
	assert.False(t, loserResult.FirstBroke, "contender arriving while guard is owned must not mutate canonical path")
	assert.False(t, loserResult.SecondBroke, "pre-guard stale observation must not authorize deleting the fresh replacement")

	pidData, err := os.ReadFile(filepath.Join(store.LockDir, "pid"))
	require.NoError(t, err)
	assert.Equal(t, winnerResult.PID, strings.TrimSpace(string(pidData)))
	acquiredData, err := os.ReadFile(filepath.Join(store.LockDir, "acquired_at"))
	require.NoError(t, err)
	assert.Equal(t, winnerResult.AcquiredAt, strings.TrimSpace(string(acquiredData)))
	assertNoStaleBreakTombstones(t, store.LockDir)
}

func TestStaleLockBreakGuardCrashReleasesWithoutReplacingSidecar(t *testing.T) {
	if runStaleBreakHelper(t) {
		return
	}

	store, projectDir := newStaleBreakFixture(t)
	writeStaleCollectionLock(t, store.LockDir, os.Getpid(), time.Now().Add(-3*time.Hour))
	coordDir := filepath.Join(projectDir, "guard-crash")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder := spawnStaleBreakHelper(ctx, "TestStaleLockBreakGuardCrashReleasesWithoutReplacingSidecar", "crash-holder", "holder", store.Dir, store.LockDir, coordDir)
	require.NoError(t, holder.Start())
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "guard-held"), 5*time.Second))

	guardPath := staleLockBreakGuardPath(store.LockDir)
	before, err := os.Stat(guardPath)
	require.NoError(t, err)
	require.NoError(t, holder.Process.Kill())
	require.Error(t, holder.Wait(), "killed guard holder must exit abnormally")

	require.Eventually(t, func() bool {
		guard, acquired := tryAcquireStaleLockBreakGuard(store.LockDir)
		if acquired {
			require.NoError(t, releaseStaleLockBreakGuard(guard))
		}
		return acquired
	}, 5*time.Second, 10*time.Millisecond, "advisory guard must release when its holder crashes")

	assert.True(t, mustBreakStaleLockDir(t, store.LockDir), "later contender must recover the stale canonical lock")
	afterBreak, err := os.Stat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, afterBreak), "stale breaking must retain the stable guard sidecar inode")

	require.NoError(t, store.WithLock(func() error { return nil }))
	afterOrdinaryAttempt, err := os.Stat(guardPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, afterOrdinaryAttempt), "ordinary acquisition/release must not replace the guard sidecar")
	assertNoStaleBreakTombstones(t, store.LockDir)
}

func TestStaleLockBreakMalformedMetadataPreservesCanonical(t *testing.T) {
	store, _ := newStaleBreakFixture(t)
	require.NoError(t, os.MkdirAll(store.LockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.LockDir, "pid"), []byte("not-a-pid"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(store.LockDir, "acquired_at"), []byte("not-a-time"), 0o644))

	assert.False(t, mustBreakStaleLockDir(t, store.LockDir))
	assert.DirExists(t, store.LockDir, "malformed current metadata supplies no guarded stale criterion")
}

func TestStaleLockBreakOverAgeCriterionDoesNotRequireValidPID(t *testing.T) {
	store, _ := newStaleBreakFixture(t)
	require.NoError(t, os.MkdirAll(store.LockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.LockDir, "pid"), []byte("not-a-pid"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(store.LockDir, "acquired_at"), []byte(time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339)), 0o644))

	assert.True(t, mustBreakStaleLockDir(t, store.LockDir), "valid over-age metadata remains an independent stale criterion")
	assert.NoDirExists(t, store.LockDir)
}

func TestStaleLockBreakDeadPIDCriterionDoesNotRequireAcquiredAt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows processAlive conservatively treats PIDs as live")
	}
	store, _ := newStaleBreakFixture(t)
	deadProcess := exec.Command(os.Args[0], "-test.run=^$")
	require.NoError(t, deadProcess.Start())
	deadPID := deadProcess.Process.Pid
	require.NoError(t, deadProcess.Wait())
	require.NoError(t, os.MkdirAll(store.LockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.LockDir, "pid"), []byte(fmt.Sprintf("%d", deadPID)), 0o644))

	assert.True(t, mustBreakStaleLockDir(t, store.LockDir), "valid dead PID remains an independent stale criterion")
	assert.NoDirExists(t, store.LockDir)
}

func TestDirLockLeaseReleaseDoesNotDeleteReplacement(t *testing.T) {
	if runStaleBreakHelper(t) {
		return
	}

	store, projectDir := newStaleBreakFixture(t)
	coordDir := filepath.Join(projectDir, "aged-live-release")
	require.NoError(t, os.MkdirAll(coordDir, 0o755))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	oldHolder := spawnStaleBreakHelper(ctx, "TestDirLockLeaseReleaseDoesNotDeleteReplacement", "aged-live-holder", "old-holder", store.Dir, store.LockDir, coordDir)
	require.NoError(t, oldHolder.Start())
	defer killStaleBreakHelpers([]*exec.Cmd{oldHolder})
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "old-lease-held"), 5*time.Second))

	require.True(t, mustBreakStaleLockDir(t, store.LockDir), "age fallback must preserve legacy stale reclamation")
	replacementLease, err := acquireDirLock(store.Dir, store.LockDir, time.Second)
	require.NoError(t, err)
	wantPID, err := os.ReadFile(filepath.Join(store.LockDir, "pid"))
	require.NoError(t, err)
	wantAcquiredAt, err := os.ReadFile(filepath.Join(store.LockDir, "acquired_at"))
	require.NoError(t, err)
	wantToken, err := os.ReadFile(filepath.Join(store.LockDir, collectionLockOwnerTokenFile))
	require.NoError(t, err)

	// The original live process now returns from its callback and releases its
	// superseded lease through the guarded token check.
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "release-old-lease"), []byte("release"), 0o644))
	require.NoError(t, oldHolder.Wait())
	oldResult := readStaleBreakResult(t, filepath.Join(coordDir, "old-holder.json"))
	assert.Empty(t, oldResult.Error)
	assert.FileExists(t, filepath.Join(coordDir, "old-release-guard-acquired"), "release barrier must originate inside guard acquisition")

	replacementPID, err := os.ReadFile(filepath.Join(store.LockDir, "pid"))
	require.NoError(t, err)
	replacementAcquiredAt, err := os.ReadFile(filepath.Join(store.LockDir, "acquired_at"))
	require.NoError(t, err)
	replacementToken, err := os.ReadFile(filepath.Join(store.LockDir, collectionLockOwnerTokenFile))
	require.NoError(t, err)
	assert.Equal(t, wantPID, replacementPID)
	assert.Equal(t, wantAcquiredAt, replacementAcquiredAt)
	assert.Equal(t, wantToken, replacementToken)
	assert.Equal(t, replacementLease.ownerToken, strings.TrimSpace(string(replacementToken)))
	assert.DirExists(t, store.LockDir, "old lease must not delete a canonical replacement with a new owner token")

	require.NoError(t, replacementLease.Release())
	assert.NoDirExists(t, store.LockDir)
}

func TestStaleLockBreakGuardSerializesSameProcess(t *testing.T) {
	store, _ := newStaleBreakFixture(t)
	first, acquired := tryAcquireStaleLockBreakGuard(store.LockDir)
	require.True(t, acquired)

	second, secondAcquired := tryAcquireStaleLockBreakGuard(store.LockDir)
	assert.False(t, secondAcquired, "keyed process mutex must serialize same-process guard attempts")
	assert.Nil(t, second)

	require.NoError(t, releaseStaleLockBreakGuard(first))
	third, thirdAcquired := tryAcquireStaleLockBreakGuard(store.LockDir)
	require.True(t, thirdAcquired)
	require.NoError(t, releaseStaleLockBreakGuard(third))
}

func TestDirLockLeaseReleaseGuardTimeoutFailsSafe(t *testing.T) {
	store, _ := newStaleBreakFixture(t)
	lease, err := acquireDirLock(store.Dir, store.LockDir, 40*time.Millisecond)
	require.NoError(t, err)
	guard, acquired := tryAcquireStaleLockBreakGuard(store.LockDir)
	require.True(t, acquired)

	started := time.Now()
	err = lease.Release()
	assert.ErrorContains(t, err, "stale-break guard timeout")
	assert.GreaterOrEqual(t, time.Since(started), 40*time.Millisecond)
	assert.DirExists(t, store.LockDir, "unproven guarded release must fail safe")

	require.NoError(t, releaseStaleLockBreakGuard(guard))
	require.NoError(t, lease.Release())
	assert.NoDirExists(t, store.LockDir)
}

func TestCollectionLockCallersJoinGuardedReleaseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	tests := []struct {
		name    string
		fixture func(t *testing.T) (string, func(func() error) error)
	}{
		{
			name: "store",
			fixture: func(t *testing.T) (string, func(func() error) error) {
				store, _ := newStaleBreakFixture(t)
				store.LockWait = 40 * time.Millisecond
				return store.LockDir, store.WithLock
			},
		},
		{
			name: "jsonl backend",
			fixture: func(t *testing.T) (string, func(func() error) error) {
				dir := filepath.Join(t.TempDir(), ddxroot.DirName)
				lockDir := filepath.Join(dir, "beads.lock")
				backend := NewJSONLBackend(dir, filepath.Join(dir, "beads.jsonl"), lockDir, 40*time.Millisecond)
				return lockDir, backend.WithLock
			},
		},
		{
			name: "axon backend",
			fixture: func(t *testing.T) (string, func(func() error) error) {
				backend := NewAxonBackend(filepath.Join(t.TempDir(), ddxroot.DirName), 40*time.Millisecond)
				return backend.LockDir, backend.WithLock
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lockDir, withLock := test.fixture(t)
			err := withLock(func() error {
				require.NoError(t, os.Mkdir(staleLockBreakGuardPath(lockDir), 0o755))
				return callbackErr
			})
			assert.ErrorIs(t, err, callbackErr)
			assert.ErrorContains(t, err, "acquire stale-break guard for release")
			assert.DirExists(t, lockDir, "release guard failure must preserve canonical lock")
		})
	}
}

func runStaleBreakHelper(t *testing.T) bool {
	t.Helper()
	if os.Getenv(staleBreakHelperEnv) != "1" {
		return false
	}

	switch os.Getenv(staleBreakModeEnv) {
	case "single-winner":
		runSingleWinnerStaleBreakHelper(t)
	case "barrier-winner":
		runBarrierWinnerStaleBreakHelper(t)
	case "barrier-loser":
		runBarrierLoserStaleBreakHelper(t)
	case "crash-holder":
		runCrashHolderStaleBreakHelper(t)
	case "aged-live-holder":
		runAgedLiveHolderStaleBreakHelper(t)
	default:
		t.Fatalf("unknown stale-break helper mode %q", os.Getenv(staleBreakModeEnv))
	}
	return true
}

func runSingleWinnerStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, role := staleBreakHelperInputs(t)
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, role+".ready"), []byte("ready"), 0o644))
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "start"), 5*time.Second))
	writeStaleBreakResult(t, filepath.Join(coordDir, role+".json"), staleBreakResult{Broke: mustBreakStaleLockDir(t, lockDir)})
}

func runBarrierWinnerStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, _ := staleBreakHelperInputs(t)
	guard, acquired, err := tryAcquireStaleLockTransitionGuardObserved(lockDir, func(stage staleLockGuardStage) {
		if stage == staleLockGuardStageAcquired {
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "winner-guard-held"), []byte("held"), 0o644))
		}
	})
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "loser-attempted"), 5*time.Second))

	tombstone, broke, err := renameFreshlyStaleLockDir(lockDir)
	require.NoError(t, err)
	require.True(t, broke)
	require.True(t, strings.HasSuffix(tombstone, ".lock"), "private tombstone must remain covered by generated lock ignores")
	require.NoError(t, releaseStaleLockBreakGuard(guard))
	require.NoError(t, os.RemoveAll(tombstone))

	// This is the ordinary canonical mkdir retry. It deliberately occurs only
	// after the advisory guard is released.
	storeDir := os.Getenv(staleBreakStoreDirEnv)
	require.NotEmpty(t, storeDir)
	winnerLease, err := acquireDirLock(storeDir, lockDir, time.Second)
	require.NoError(t, err)
	winnerPID, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	require.NoError(t, err)
	winnerAcquiredAt, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	require.NoError(t, err)
	winnerOwnerToken, err := os.ReadFile(filepath.Join(lockDir, collectionLockOwnerTokenFile))
	require.NoError(t, err)
	require.Equal(t, winnerLease.ownerToken, string(winnerOwnerToken))
	writeStaleBreakResult(t, filepath.Join(coordDir, "winner.json"), staleBreakResult{
		Broke:      broke,
		PID:        string(winnerPID),
		AcquiredAt: string(winnerAcquiredAt),
		OwnerToken: string(winnerOwnerToken),
	})
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "fresh-installed"), []byte("fresh"), 0o644))
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "loser-done"), 5*time.Second))
}

func runBarrierLoserStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, _ := staleBreakHelperInputs(t)
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "winner-guard-held"), 5*time.Second))
	firstBroke, err := breakStaleLockDirObserved(lockDir, func(stage staleLockGuardStage) {
		if stage == staleLockGuardStageContended {
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "loser-attempted"), []byte("attempted"), 0o644))
		}
	})
	require.NoError(t, err)
	require.False(t, firstBroke)
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "fresh-installed"), 5*time.Second))

	want := readStaleBreakResult(t, filepath.Join(coordDir, "winner.json"))
	secondBroke := mustBreakStaleLockDir(t, lockDir)
	pidData, err := os.ReadFile(filepath.Join(lockDir, "pid"))
	require.NoError(t, err)
	acquiredData, err := os.ReadFile(filepath.Join(lockDir, "acquired_at"))
	require.NoError(t, err)
	ownerTokenData, err := os.ReadFile(filepath.Join(lockDir, collectionLockOwnerTokenFile))
	require.NoError(t, err)
	require.Equal(t, want.PID, string(pidData))
	require.Equal(t, want.AcquiredAt, string(acquiredData))
	require.Equal(t, want.OwnerToken, string(ownerTokenData))

	writeStaleBreakResult(t, filepath.Join(coordDir, "loser.json"), staleBreakResult{
		FirstBroke:  firstBroke,
		SecondBroke: secondBroke,
	})
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "loser-done"), []byte("done"), 0o644))
}

func runCrashHolderStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, _ := staleBreakHelperInputs(t)
	guard, acquired, err := tryAcquireStaleLockTransitionGuardObserved(lockDir, func(stage staleLockGuardStage) {
		if stage == staleLockGuardStageAcquired {
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "guard-held"), []byte("held"), 0o644))
		}
	})
	require.NoError(t, err)
	require.True(t, acquired)
	defer func() { _ = releaseStaleLockBreakGuard(guard) }()
	for {
		time.Sleep(time.Hour)
	}
}

func runAgedLiveHolderStaleBreakHelper(t *testing.T) {
	coordDir, lockDir, role := staleBreakHelperInputs(t)
	storeDir := os.Getenv(staleBreakStoreDirEnv)
	require.NotEmpty(t, storeDir)
	lease, err := acquireDirLock(storeDir, lockDir, time.Second)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(lockDir, "acquired_at"),
		[]byte(time.Now().Add(-3*time.Hour).UTC().Format(time.RFC3339)),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(coordDir, "old-lease-held"), []byte("held"), 0o644))
	require.NoError(t, waitForStaleBreakFile(filepath.Join(coordDir, "release-old-lease"), 5*time.Second))

	releaseErr := lease.releaseObserved(func(stage staleLockGuardStage) {
		if stage == staleLockGuardStageAcquired {
			require.NoError(t, os.WriteFile(filepath.Join(coordDir, "old-release-guard-acquired"), []byte("acquired"), 0o644))
		}
	})
	result := staleBreakResult{}
	if releaseErr != nil {
		result.Error = releaseErr.Error()
	}
	writeStaleBreakResult(t, filepath.Join(coordDir, role+".json"), result)
}

func staleBreakHelperInputs(t *testing.T) (coordDir, lockDir, role string) {
	t.Helper()
	coordDir = os.Getenv(staleBreakCoordDirEnv)
	lockDir = os.Getenv(staleBreakLockDirEnv)
	role = os.Getenv(staleBreakRoleEnv)
	require.NotEmpty(t, coordDir)
	require.NotEmpty(t, lockDir)
	return coordDir, lockDir, role
}

func newStaleBreakFixture(t *testing.T) (*Store, string) {
	t.Helper()
	t.Setenv("DDX_BEAD_BACKEND", "jsonl")
	projectDir := t.TempDir()
	store := NewStore(filepath.Join(projectDir, ddxroot.DirName))
	require.NoError(t, os.MkdirAll(store.Dir, 0o755))
	return store, projectDir
}

func writeStaleCollectionLock(t *testing.T, lockDir string, pid int, acquiredAt time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", pid)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(acquiredAt.UTC().Format(time.RFC3339)), 0o644))
}

func spawnStaleBreakHelper(ctx context.Context, testName, mode, role, storeDir, lockDir, coordDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		staleBreakHelperEnv+"=1",
		staleBreakModeEnv+"="+mode,
		staleBreakStoreDirEnv+"="+storeDir,
		staleBreakLockDirEnv+"="+lockDir,
		staleBreakCoordDirEnv+"="+coordDir,
		staleBreakRoleEnv+"="+role,
		"DDX_BEAD_BACKEND=jsonl",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func killStaleBreakHelpers(commands []*exec.Cmd) {
	for _, cmd := range commands {
		if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
}

func waitForStaleBreakFiles(dir, suffix string, want int, timeout time.Duration) error {
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

func waitForStaleBreakFile(path string, timeout time.Duration) error {
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

func writeStaleBreakResult(t *testing.T, path string, result staleBreakResult) {
	t.Helper()
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
}

func readStaleBreakResult(t *testing.T, path string) staleBreakResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var result staleBreakResult
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func assertNoStaleBreakTombstones(t *testing.T, lockDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone.*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "breaker must remove only its uniquely owned tombstone")
}

func mustBreakStaleLockDir(t *testing.T, lockDir string) bool {
	t.Helper()
	broke, err := breakStaleLockDir(lockDir)
	require.NoError(t, err)
	return broke
}
