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

// livePGID returns the current process-group id of pid, if the platform
// supports the query and the process is alive. ok is false when the query
// is unsupported (Windows) or the process is gone, in which case callers
// must not treat the result as a definitive mismatch.
func livePGID(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return 0, false
	}
	return pgid, true
}
