package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestServer_SingletonGuardRefusesSecondLaunch verifies that once the
// singleton lock is held by an alive process, a second acquisition refuses
// with a SingletonLockError naming the holder pid and recorded URL.
func TestServer_SingletonGuardRefusesSecondLaunch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Pretend a server has already written its address file (so the error
	// message can include the URL of the running server).
	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	addr := map[string]any{
		"url": "https://127.0.0.1:7743",
		"pid": os.Getpid(),
	}
	data, _ := json.Marshal(addr)
	if err := os.WriteFile(filepath.Join(dir, "server.addr"), data, 0600); err != nil {
		t.Fatalf("write addr: %v", err)
	}

	// First acquisition succeeds.
	release1, err := acquireSingletonLock()
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer release1()

	// Second acquisition (same pid is the holder; the function detects this
	// because the lock dir exists with our pid recorded — but since the
	// caller is the same process, processAlive returns true and we get a
	// SingletonLockError).
	_, err = acquireSingletonLock()
	if err == nil {
		t.Fatal("expected second acquire to fail, got nil")
	}
	var sle *SingletonLockError
	if !errors.As(err, &sle) {
		t.Fatalf("expected *SingletonLockError, got %T: %v", err, err)
	}
	if sle.PID != os.Getpid() {
		t.Errorf("expected error pid=%d, got %d", os.Getpid(), sle.PID)
	}
	if !strings.Contains(sle.Addr, "7743") {
		t.Errorf("expected error addr to contain 7743, got %q", sle.Addr)
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' in message, got %q", err.Error())
	}

	// Verify pid file exists and matches our pid.
	pidData, err := os.ReadFile(filepath.Join(dir, "server.lock", "pid"))
	if err != nil {
		t.Fatalf("read lock pid: %v", err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if pid != os.Getpid() {
		t.Errorf("expected lock pid=%d, got %d", os.Getpid(), pid)
	}
}

// TestServer_SingletonGuardBreaksStaleLock verifies that if the recorded lock
// pid is dead, a fresh acquire breaks the stale lock and succeeds.
func TestServer_SingletonGuardBreaksStaleLock(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lockDir := filepath.Join(dir, "server.lock")
	if err := os.Mkdir(lockDir, 0700); err != nil {
		t.Fatalf("mkdir lock: %v", err)
	}
	// PID 999999 is overwhelmingly unlikely to exist.
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte("999999"), 0600); err != nil {
		t.Fatalf("write stale pid: %v", err)
	}

	release, err := acquireSingletonLock()
	if err != nil {
		t.Fatalf("expected stale lock to be broken, got error: %v", err)
	}
	defer release()

	pidData, _ := os.ReadFile(filepath.Join(lockDir, "pid"))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if pid != os.Getpid() {
		t.Errorf("expected our pid in lock after break, got %d", pid)
	}
}

// TestServer_StaleAddressFallbackWithWarning verifies that ReadServerAddr
// detects a server.addr whose recorded pid is not alive, emits a warning,
// and returns "" so callers fall back to the default URL.
func TestServer_StaleAddressFallbackWithWarning(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stalePID := 999999
	addr := map[string]any{
		"url": "https://127.0.0.1:18099",
		"pid": stalePID,
	}
	data, _ := json.Marshal(addr)
	addrPath := filepath.Join(dir, "server.addr")
	if err := os.WriteFile(addrPath, data, 0600); err != nil {
		t.Fatalf("write addr: %v", err)
	}

	var buf bytes.Buffer
	prev := addrFallbackWarnWriter
	addrFallbackWarnWriter = &buf
	defer func() { addrFallbackWarnWriter = prev }()

	got := ReadServerAddr()
	if got != "" {
		t.Errorf("expected ReadServerAddr to return empty for dead pid, got %q", got)
	}
	if !strings.Contains(buf.String(), "dead pid") {
		t.Errorf("expected 'dead pid' warning, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "127.0.0.1:7743") {
		t.Errorf("expected fallback URL in warning, got %q", buf.String())
	}
	// Stale file should be removed best-effort.
	if _, err := os.Stat(addrPath); !os.IsNotExist(err) {
		t.Errorf("expected stale addr file removed, stat err=%v", err)
	}
}

// TestServer_ReadServerAddrReturnsURLForLivePID verifies that ReadServerAddr
// returns the recorded URL when the pid in server.addr is alive.
func TestServer_ReadServerAddrReturnsURLForLivePID(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	addr := map[string]any{
		"url": "https://127.0.0.1:7743",
		"pid": os.Getpid(),
	}
	data, _ := json.Marshal(addr)
	if err := os.WriteFile(filepath.Join(dir, "server.addr"), data, 0600); err != nil {
		t.Fatalf("write addr: %v", err)
	}
	if got := ReadServerAddr(); got != "https://127.0.0.1:7743" {
		t.Errorf("expected live URL, got %q", got)
	}
}
