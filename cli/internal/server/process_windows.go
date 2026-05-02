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
