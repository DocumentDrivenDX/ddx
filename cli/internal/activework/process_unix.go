//go:build !windows

package activework

import "syscall"

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) != syscall.ESRCH
}
