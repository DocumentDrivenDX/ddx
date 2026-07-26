//go:build !linux

package scratchowner

// processStartIdentity is unavailable on non-Linux platforms. The marker
// still records PID + creation time; Classify falls back to PID liveness.
func processStartIdentity(pid int) string {
	return ""
}
