package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// addrFallbackWarnWriter receives the warning emitted when ReadServerAddr
// detects a stale server.addr (recorded pid is not alive). Tests swap this
// for a buffer to assert the warning is produced.
var addrFallbackWarnWriter io.Writer = os.Stderr

// SingletonLockError is returned by acquireSingletonLock when another live
// ddx-server already holds the per-machine lock.
type SingletonLockError struct {
	PID  int
	Addr string
}

func (e *SingletonLockError) Error() string {
	addr := e.Addr
	if addr == "" {
		addr = "unknown"
	}
	return fmt.Sprintf("ddx server already running, pid=%d, addr=%s; only one ddx server per machine", e.PID, addr)
}

// acquireSingletonLock takes a per-machine lock so at most one ddx-server runs
// on a given host. The lock is a directory at <serverAddrDir>/server.lock
// containing a pid file. If the directory exists with an alive pid, an error
// is returned. If the pid is dead, the lock is broken and re-acquired (this
// covers crashes that left a stale lock). The returned release function
// removes the lock; callers should defer it.
func acquireSingletonLock() (func(), error) {
	dir := serverAddrDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("server: lock dir: %w", err)
	}
	lockDir := filepath.Join(dir, "server.lock")
	pidPath := filepath.Join(lockDir, "pid")

	for attempt := 0; attempt < 2; attempt++ {
		if err := os.Mkdir(lockDir, 0700); err == nil {
			if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
				_ = os.RemoveAll(lockDir)
				return nil, fmt.Errorf("server: write lock pid: %w", err)
			}
			release := func() {
				data, _ := os.ReadFile(pidPath)
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid == os.Getpid() {
					_ = os.RemoveAll(lockDir)
				}
			}
			return release, nil
		}

		pidData, _ := os.ReadFile(pidPath)
		pid, perr := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if perr == nil && pid > 0 && processAlive(pid) {
			return nil, &SingletonLockError{PID: pid, Addr: ReadRawServerAddr()}
		}

		// Stale lock (dead pid, missing pid file, or our own pid leftover).
		if err := os.RemoveAll(lockDir); err != nil {
			return nil, fmt.Errorf("server: break stale lock: %w", err)
		}
	}
	return nil, fmt.Errorf("server: could not acquire singleton lock at %s", lockDir)
}

// ReadRawServerAddr returns the URL recorded in server.addr without checking
// that the recorded pid is alive. Useful for diagnostics and for the singleton
// guard, which wants to report the prior URL even if the prior process is
// alive but unresponsive.
func ReadRawServerAddr() string {
	type addrFile struct {
		URL string `json:"url"`
	}
	path := filepath.Join(serverAddrDir(), "server.addr")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var af addrFile
	if err := json.Unmarshal(data, &af); err != nil {
		return ""
	}
	return af.URL
}

// readAddrFilePID returns the pid recorded in server.addr (0 if absent).
func readAddrFilePID() int {
	type addrFile struct {
		PID int `json:"pid"`
	}
	path := filepath.Join(serverAddrDir(), "server.addr")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var af addrFile
	if err := json.Unmarshal(data, &af); err != nil {
		return 0
	}
	return af.PID
}
