//go:build !windows

package server

import "syscall"

// processAlive checks if a process with the given PID exists. Uses signal 0
// which probes existence without sending a signal.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	// ESRCH = no such process. EPERM = exists but different user (still alive).
	return err != syscall.ESRCH
}
