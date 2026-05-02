//go:build windows

package agent

import "os"

// trackerProcessAlive conservatively assumes a process is alive on Windows
// (Signal(0) is unsupported). The age-based fallback handles truly stale
// locks. Mirrors cli/internal/bead/lock_windows.go.
func trackerProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p
	return true
}
