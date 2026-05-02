//go:build !windows

package agent

import "syscall"

// trackerProcessAlive checks if a process with the given PID exists. Uses
// signal 0 which checks existence without sending a signal.
func trackerProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err != syscall.ESRCH
}
