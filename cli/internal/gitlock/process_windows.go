//go:build windows

package gitlock

import "os"

// processAlive conservatively assumes a process is alive on Windows
// (Signal(0) is unsupported). The age-based fallback handles truly stale locks.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p
	return true
}
