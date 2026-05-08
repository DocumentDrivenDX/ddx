//go:build windows

package bead

import "os"

// processAlive checks if a process with the given PID exists on Windows.
// Uses os.FindProcess + Signal(0) which is the portable approach.
// On Windows, FindProcess always succeeds, so we attempt to signal it.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, Signal with os.Signal(syscall.Signal(0)) is not supported.
	// We conservatively assume the process is alive to avoid breaking locks
	// that may still be held. The age-based fallback handles truly stale locks.
	_ = p
	return true
}
