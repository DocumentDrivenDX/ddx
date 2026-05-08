//go:build !windows

package bead

import "syscall"

// processAlive checks if a process with the given PID exists.
// Uses signal 0 which checks existence without sending a signal.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	// ESRCH = no such process (dead). EPERM = exists but different user (alive). nil = alive.
	return err != syscall.ESRCH
}
