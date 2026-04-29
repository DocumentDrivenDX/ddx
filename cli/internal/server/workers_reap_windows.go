//go:build windows

package server

import (
	"os"
	"time"
)

// isPIDAlive reports whether a process with the given PID is alive.
// On Windows, os.FindProcess always succeeds, so we conservatively return
// true for any positive PID. Prune falls back to maxAge-based pruning on
// Windows rather than PID liveness.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}

// terminateProcessGroup on Windows has no process-group primitive. We call
// os.Process.Kill (maps to TerminateProcess) directly; the grace is observed
// so the API stays uniform across platforms.
func terminateProcessGroup(pid int, grace time.Duration) {
	if pid <= 0 {
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	// Give the caller's own cancel() path a chance to exit cleanly first.
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if err := p.Signal(os.Interrupt); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = p.Kill()
}
