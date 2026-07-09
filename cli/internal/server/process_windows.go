//go:build windows

package server

import "os"

// processAlive checks if a process with the given PID exists on Windows.
// On Windows, os.FindProcess always succeeds, so a non-error return is the
// best portable signal we have. We conservatively assume the process is
// alive to avoid prematurely breaking valid locks.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}

// livePGID has no Windows equivalent — the platform lacks POSIX process
// groups. ok is always false so callers skip the PGID-match check rather
// than treating the absence of the primitive as a mismatch.
func livePGID(pid int) (int, bool) {
	return 0, false
}
