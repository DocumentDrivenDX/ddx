//go:build !windows

package scratchowner

import "syscall"

// processAlive reports whether pid currently exists. Signal 0 checks
// existence without delivering a signal. ESRCH means gone; EPERM means the
// process exists but is owned by another user (still alive).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err != syscall.ESRCH
}
