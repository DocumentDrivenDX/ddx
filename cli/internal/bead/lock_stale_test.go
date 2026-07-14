package bead

import (
	"context"
	"encoding/json"
	"fmt"
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

const (
	staleLockHelperEnv    = "DDX_BEAD_STALE_LOCK_HELPER"
	staleLockModeEnv      = "DDX_BEAD_STALE_LOCK_MODE"
	staleLockStoreDirEnv  = "DDX_BEAD_STALE_LOCK_STORE_DIR"
	staleLockLockDirEnv   = "DDX_BEAD_STALE_LOCK_LOCKDIR"
	staleLockStartEnv     = "DDX_BEAD_STALE_LOCK_START"
	staleLockReadyDirEnv  = "DDX_BEAD_STALE_LOCK_READY_DIR"
	staleLockResultDirEnv = "DDX_BEAD_STALE_LOCK_RESULT_DIR"
	staleLockRoleEnv      = "DDX_BEAD_STALE_LOCK_ROLE"
)

type staleLockBreakResult struct {
	Broke bool `json:"broke"`
}

func TestStaleLockBreakSingleWinner(t *testing.T) {
	if os.Getenv(staleLockHelperEnv) == "1" {
		runStaleLockHelper(t)
		return
	}

	projectDir := t.TempDir()
	storeDir := filepath.Join(projectDir, ddxroot.DirName)
	s := NewStore(storeDir)
	require.NoError(t, os.MkdirAll(s.Dir, 0o755))
	writeStaleLockFixture(t, s.LockDir, os.Getpid())

	startFile := filepath.Join(projectDir, "start")
	readyDir := filepath.Join(projectDir, "ready")
	resultDir := filepath.Join(projectDir, "result")
	require.NoError(t, os.MkdirAll(readyDir, 0o755))
	require.NoError(t, os.MkdirAll(resultDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roles := []string{"left", "right"}
	cmds := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmds = append(cmds, spawnStaleLockChild(t, ctx, "TestStaleLockBreakSingleWinner", storeDir, s.LockDir, startFile, readyDir, resultDir, role, "single-winner"))
	}

	for _, cmd := range cmds {
		require.NoError(t, cmd.Start())
		defer func(c *exec.Cmd) {
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}(cmd)
	}

	require.NoError(t, waitForReadyFiles(readyDir, len(roles), 5*time.Second))
	require.NoError(t, os.WriteFile(startFile, []byte("go"), 0o644))

	for _, cmd := range cmds {
		require.NoError(t, cmd.Wait())
	}

	results := make(map[string]staleLockBreakResult, len(roles))
	for _, role := range roles {
		raw, err := os.ReadFile(filepath.Join(resultDir, role+".json"))
		require.NoError(t, err)
		var res staleLockBreakResult
		require.NoError(t, json.Unmarshal(raw, &res))
		results[role] = res
	}

	trueCount := 0
	for _, res := range results {
		if res.Broke {
			trueCount++
		}
	}
	assert.Equal(t, 1, trueCount, "exactly one contender should win the tombstone transition")

	assertNoTombstones(t, s.LockDir)
	_, err := os.Stat(s.LockDir)
	assert.Error(t, err, "the stale lock directory should be gone after the tombstone handoff")
}

func TestStaleLockBreakLoserNeverDeletesWinner(t *testing.T) {
	if os.Getenv(staleLockHelperEnv) == "1" {
		runStaleLockHelper(t)
		return
	}

	projectDir := t.TempDir()
	storeDir := filepath.Join(projectDir, ddxroot.DirName)
	s := NewStore(storeDir)
	require.NoError(t, os.MkdirAll(s.Dir, 0o755))
	writeStaleLockFixture(t, s.LockDir, os.Getpid())

	startFile := filepath.Join(projectDir, "start")
	readyDir := filepath.Join(projectDir, "ready")
	require.NoError(t, os.MkdirAll(readyDir, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roles := []string{"winner", "loser"}
	cmds := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmds = append(cmds, spawnStaleLockChild(t, ctx, "TestStaleLockBreakLoserNeverDeletesWinner", storeDir, s.LockDir, startFile, readyDir, projectDir, role, "hold-check"))
	}

	for _, cmd := range cmds {
		require.NoError(t, cmd.Start())
		defer func(c *exec.Cmd) {
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}(cmd)
	}

	require.NoError(t, waitForReadyFiles(readyDir, len(roles), 5*time.Second))
	require.NoError(t, os.WriteFile(startFile, []byte("go"), 0o644))

	for _, cmd := range cmds {
		require.NoError(t, cmd.Wait())
	}
}

func runStaleLockHelper(t *testing.T) {
	t.Helper()

	switch os.Getenv(staleLockModeEnv) {
	case "single-winner":
		runStaleLockSingleWinnerChild(t)
	case "hold-check":
		runStaleLockHoldCheckChild(t)
	default:
		t.Fatalf("unknown stale-lock helper mode %q", os.Getenv(staleLockModeEnv))
	}
}

func runStaleLockSingleWinnerChild(t *testing.T) {
	t.Helper()

	lockDir := os.Getenv(staleLockLockDirEnv)
	startFile := os.Getenv(staleLockStartEnv)
	readyDir := os.Getenv(staleLockReadyDirEnv)
	resultDir := os.Getenv(staleLockResultDirEnv)
	role := os.Getenv(staleLockRoleEnv)

	require.NotEmpty(t, lockDir)
	require.NotEmpty(t, startFile)
	require.NotEmpty(t, readyDir)
	require.NotEmpty(t, resultDir)
	require.NotEmpty(t, role)

	readyFile := filepath.Join(readyDir, role+".ready")
	resultFile := filepath.Join(resultDir, role+".json")

	require.NoError(t, os.WriteFile(readyFile, []byte("ready"), 0o644))
	require.NoError(t, waitForFile(startFile, 5*time.Second))

	res := staleLockBreakResult{Broke: breakStaleLockDir(lockDir)}
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(resultFile, raw, 0o644))
}

func runStaleLockHoldCheckChild(t *testing.T) {
	t.Helper()

	storeDir := os.Getenv(staleLockStoreDirEnv)
	startFile := os.Getenv(staleLockStartEnv)
	readyDir := os.Getenv(staleLockReadyDirEnv)
	role := os.Getenv(staleLockRoleEnv)

	require.NotEmpty(t, storeDir)
	require.NotEmpty(t, startFile)
	require.NotEmpty(t, readyDir)
	require.NotEmpty(t, role)

	readyFile := filepath.Join(readyDir, role+".ready")
	require.NoError(t, os.WriteFile(readyFile, []byte("ready"), 0o644))
	require.NoError(t, waitForFile(startFile, 5*time.Second))

	s := NewStore(storeDir)
	s.LockWait = 1 * time.Second
	wantPID := fmt.Sprintf("%d", os.Getpid())

	err := s.WithLock(func() error {
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			pidData, err := os.ReadFile(filepath.Join(s.LockDir, "pid"))
			if err != nil {
				return fmt.Errorf("lock pid missing while held: %w", err)
			}
			if got := strings.TrimSpace(string(pidData)); got != wantPID {
				return fmt.Errorf("lock pid changed while held: got %q want %q", got, wantPID)
			}

			acquiredData, err := os.ReadFile(filepath.Join(s.LockDir, "acquired_at"))
			if err != nil {
				return fmt.Errorf("lock acquired_at missing while held: %w", err)
			}
			if _, err := time.Parse(time.RFC3339, strings.TrimSpace(string(acquiredData))); err != nil {
				return fmt.Errorf("lock acquired_at invalid while held: %w", err)
			}

			time.Sleep(5 * time.Millisecond)
		}
		return nil
	})
	require.NoError(t, err)
}

func spawnStaleLockChild(t *testing.T, ctx context.Context, testName, storeDir, lockDir, startFile, readyDir, resultDir, role, mode string) *exec.Cmd {
	t.Helper()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run="+testName, "-test.count=1")
	cmd.Env = append(os.Environ(),
		staleLockHelperEnv+"=1",
		staleLockModeEnv+"="+mode,
		staleLockStoreDirEnv+"="+storeDir,
		staleLockLockDirEnv+"="+lockDir,
		staleLockStartEnv+"="+startFile,
		staleLockReadyDirEnv+"="+readyDir,
		staleLockResultDirEnv+"="+resultDir,
		staleLockRoleEnv+"="+role,
		"DDX_BEAD_BACKEND=jsonl",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func writeStaleLockFixture(t *testing.T, lockDir string, pid int) {
	t.Helper()

	require.NoError(t, os.MkdirAll(lockDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d", pid)), 0o644))
	staleTime := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "acquired_at"), []byte(staleTime), 0o644))
}

func waitForReadyFiles(dir string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ready") {
				count++
			}
		}
		if count >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %d ready files in %s", want, dir)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertNoTombstones(t *testing.T, lockDir string) {
	t.Helper()

	entries, err := filepath.Glob(filepath.Join(filepath.Dir(lockDir), filepath.Base(lockDir)+".tombstone.*"))
	require.NoError(t, err)
	assert.Empty(t, entries, "stale-lock tombstones should be removed by the contender that owns them")
}
