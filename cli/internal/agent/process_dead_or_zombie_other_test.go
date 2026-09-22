//go:build !linux && !windows

package agent

import (
	"errors"
	"syscall"
)

type procStateSnapshot struct {
	value *string
}

func (s procStateSnapshot) String() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

func processDeadOrZombie(pid int) bool {
	return processDeadOrZombieStatus(pid) == "ESRCH"
}

// processDeadOrZombieStatus reports whether pid is reachable via kill(pid, 0).
// Non-Linux Unix platforms have no /proc, so the zombie state Linux exposes
// via process_dead_or_zombie_linux_test.go isn't observable here — ESRCH is
// the only "gone" signal available. Callers in this package already reap
// their spawned children via a background cmd.Process.Wait(), so the zombie
// window is brief and this is sufficient for test purposes.
func processDeadOrZombieStatus(pid int) string {
	if pid <= 0 {
		return "invalid-pid"
	}
	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return "ESRCH"
		}
		if !errors.Is(err, syscall.EPERM) {
			return err.Error()
		}
	}
	return "alive"
}
