//go:build windows

package scratchowner

import "os"

// processAlive reports whether pid currently exists. Windows FindProcess
// succeeds for any non-zero PID handle synthesis, so this is a best-effort
// check; callers that need strong identity should rely on
// ProcessStartIdentity when the platform provides it.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	return err == nil && proc != nil
}
