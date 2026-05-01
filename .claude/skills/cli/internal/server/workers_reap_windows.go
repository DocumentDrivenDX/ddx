//go:build windows

package server

import (
	"os"
	"time"
)

// terminateProcessGroup on Windows has no process-group primitive. We call
// os.Process.Kill (maps to TerminateProcess) directly; the grace is observed
// so the API stays uniform across platforms.
func terminateProcessGroup(pid int, grace time.Duration) {
	if pid <= 0 {
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	// Give the caller's own cancel() path a chance to exit cleanly first.
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if err := p.Signal(os.Interrupt); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = p.Kill()
}
