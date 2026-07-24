package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	offlineCoordHelperEnv  = "DDX_OFFLINE_COORD_HELPER"
	offlineCoordRootEnv    = "DDX_OFFLINE_COORD_ROOT"
	offlineCoordCoordEnv   = "DDX_OFFLINE_COORD_DIR"
	offlineCoordRoleEnv    = "DDX_OFFLINE_COORD_ROLE"
	offlineCoordHoldEnv    = "DDX_OFFLINE_COORD_HOLD_MS"
	offlineCoordResultFile = "window.json"
)

// offlineCoordWindow is the protected mutation window observed by one helper
// subprocess. EnteredAt/ExitedAt are wall-clock timestamps while holding
// OfflineCoordinator.WithLock.
type offlineCoordWindow struct {
	Role      string    `json:"role"`
	PID       int       `json:"pid"`
	LockPath  string    `json:"lock_path"`
	EnteredAt time.Time `json:"entered_at"`
	ExitedAt  time.Time `json:"exited_at"`
}

// TestOfflineCoordinator_SerializesAcrossProcesses launches two real
// subprocesses against the same project root and verifies their protected
// mutation windows do not overlap. If the second process enters the protected
// section before the first releases the project coordination lock, the
// overlap assertion fails (AC#1, AC#2).
//
// Production OfflineCoordinator.WithLock uses OfflineCoordinationLockPath;
// the helpers call that same production API (AC#3).
func TestOfflineCoordinator_SerializesAcrossProcesses(t *testing.T) {
	if os.Getenv(offlineCoordHelperEnv) == "1" {
		runOfflineCoordHelper(t)
		return
	}

	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	coordDir := t.TempDir()
	readyDir := filepath.Join(coordDir, "ready")
	require.NoError(t, os.MkdirAll(readyDir, 0o755))
	startFile := filepath.Join(coordDir, "start")

	holdMS := 250
	roles := []string{"a", "b"}
	cmds := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmd := exec.Command(os.Args[0],
			"-test.run=^TestOfflineCoordinator_SerializesAcrossProcesses$",
			"-test.count=1",
		)
		cmd.Env = append(os.Environ(),
			offlineCoordHelperEnv+"=1",
			offlineCoordRootEnv+"="+projectRoot,
			offlineCoordCoordEnv+"="+coordDir,
			offlineCoordRoleEnv+"="+role,
			offlineCoordHoldEnv+"="+fmt.Sprintf("%d", holdMS),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmds = append(cmds, cmd)
	}

	for _, cmd := range cmds {
		require.NoError(t, cmd.Start(), "start helper")
	}
	defer func() {
		for _, cmd := range cmds {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}()

	require.NoError(t, waitForOfflineCoordReady(readyDir, roles, 10*time.Second))
	require.NoError(t, os.WriteFile(startFile, []byte("go"), 0o644))

	for i, cmd := range cmds {
		require.NoError(t, cmd.Wait(), "helper %s exit", roles[i])
	}

	windows := make([]offlineCoordWindow, 0, len(roles))
	for _, role := range roles {
		path := filepath.Join(coordDir, role+"."+offlineCoordResultFile)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "read window for role %s", role)
		var w offlineCoordWindow
		require.NoError(t, json.Unmarshal(data, &w), "decode window for role %s", role)
		windows = append(windows, w)
	}

	require.Len(t, windows, 2)
	expectedLock := OfflineCoordinationLockPath(projectRoot)
	for _, w := range windows {
		assert.Equal(t, expectedLock, w.LockPath,
			"helper must exercise the production OfflineCoordinationLockPath")
		assert.False(t, w.EnteredAt.IsZero(), "entered_at must be set")
		assert.False(t, w.ExitedAt.IsZero(), "exited_at must be set")
		assert.True(t, !w.ExitedAt.Before(w.EnteredAt),
			"exit must not precede enter for role %s", w.Role)
	}

	// No overlap: one window must fully complete before the other enters.
	// This is the AC#2 failure mode if the project lock is a no-op.
	a, b := windows[0], windows[1]
	aBeforeB := !a.ExitedAt.After(b.EnteredAt) // a.exit <= b.enter
	bBeforeA := !b.ExitedAt.After(a.EnteredAt) // b.exit <= a.enter
	if !aBeforeB && !bBeforeA {
		t.Fatalf("protected mutation windows overlapped:\n  %s: [%s, %s]\n  %s: [%s, %s]\n"+
			"(second process entered before the first released the project coordination lock)",
			a.Role, a.EnteredAt.Format(time.RFC3339Nano), a.ExitedAt.Format(time.RFC3339Nano),
			b.Role, b.EnteredAt.Format(time.RFC3339Nano), b.ExitedAt.Format(time.RFC3339Nano),
		)
	}

	// Lock directory must be released after both helpers exit.
	_, err := os.Stat(expectedLock)
	assert.True(t, os.IsNotExist(err), "offline coordination lock must be released after helpers exit")
}

func runOfflineCoordHelper(t *testing.T) {
	t.Helper()

	projectRoot := os.Getenv(offlineCoordRootEnv)
	coordDir := os.Getenv(offlineCoordCoordEnv)
	role := os.Getenv(offlineCoordRoleEnv)
	holdMS := 250
	if v := os.Getenv(offlineCoordHoldEnv); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &holdMS); err != nil {
			t.Fatalf("parse hold ms: %v", err)
		}
	}
	require.NotEmpty(t, projectRoot)
	require.NotEmpty(t, coordDir)
	require.NotEmpty(t, role)

	readyFile := filepath.Join(coordDir, "ready", role+".ready")
	require.NoError(t, os.MkdirAll(filepath.Dir(readyFile), 0o755))
	require.NoError(t, os.WriteFile(readyFile, []byte("ready"), 0o644))

	startFile := filepath.Join(coordDir, "start")
	require.NoError(t, waitForOfflineCoordFile(startFile, 10*time.Second))

	coord := NewOfflineCoordinator(projectRoot)
	lockPath := OfflineCoordinationLockPath(projectRoot)

	var window offlineCoordWindow
	window.Role = role
	window.PID = os.Getpid()
	window.LockPath = lockPath

	err := coord.WithLock(context.Background(), func() error {
		// Confirm the production lock path is held for the mutation window.
		if info, statErr := os.Stat(lockPath); statErr != nil || !info.IsDir() {
			return fmt.Errorf("expected lock dir %s held during protected section: %v", lockPath, statErr)
		}
		window.EnteredAt = time.Now().UTC()
		time.Sleep(time.Duration(holdMS) * time.Millisecond)
		window.ExitedAt = time.Now().UTC()
		return nil
	})
	require.NoError(t, err, "WithLock protected mutation")

	out, err := json.Marshal(window)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(coordDir, role+"."+offlineCoordResultFile),
		out,
		0o644,
	))
}

func waitForOfflineCoordReady(readyDir string, roles []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allReady := true
		for _, role := range roles {
			if _, err := os.Stat(filepath.Join(readyDir, role+".ready")); err != nil {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for ready files in %s", readyDir)
}

func waitForOfflineCoordFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", path)
}

// TestOfflineCoordinator_LockPathIsProjectScoped verifies production and
// tests resolve the same durable lock location under the project DDx root.
func TestOfflineCoordinator_LockPathIsProjectScoped(t *testing.T) {
	projectRoot := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, projectRoot)

	path := OfflineCoordinationLockPath(projectRoot)
	assert.Contains(t, path, offlineCoordinationLockDirName)
	// Lock lives under the project DDx state root, not under .git.
	assert.NotContains(t, path, string(filepath.Separator)+".git"+string(filepath.Separator))
	// Holding the lock creates the directory; release removes it.
	coord := NewOfflineCoordinator(projectRoot)
	held := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = coord.WithLock(context.Background(), func() error {
			close(held)
			<-done
			return nil
		})
	}()
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting to acquire offline coordination lock")
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	close(done)
}
