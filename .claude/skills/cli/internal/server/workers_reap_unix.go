//go:build !windows

package server

import (
	"syscall"
	"time"
)

// terminateProcessGroup sends SIGTERM to the worker's process group; if the
// process is still alive after grace, follows up with SIGKILL. The negative
// pid argument to syscall.Kill targets the whole process group, which is
// essential for workers that fork child harnesses.
//
// Callers set Setpgid=true when spawning the process so pid == pgid and the
// group id matches. If the caller did not set a new pgid, a negative-pid
// signal still works because the worker's own pgid will be the caller's pgid
// — not desirable, so the caller contract is: only register a PID for
// processes you spawned with their own pgid.
func terminateProcessGroup(pid int, grace time.Duration) {
	if pid <= 0 {
		return
	}
	// SIGTERM to the process group.
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	// Poll until grace expires for the leader to exit; then SIGKILL.
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
