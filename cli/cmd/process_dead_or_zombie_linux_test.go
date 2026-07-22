//go:build linux

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	status := processDeadOrZombieStatus(pid)
	return status == "ESRCH" || status == "Z"
}

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
	state, ok := processStatState(pid)
	if !ok {
		return "stat-unavailable"
	}
	return state
}

func processStatState(pid int) (string, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", false
	}
	close := strings.LastIndexByte(string(data), ')')
	if close < 0 {
		return "", false
	}
	tail := strings.Fields(string(data[close+1:]))
	if len(tail) == 0 {
		return "", false
	}
	return tail[0], true
}

func TestProcessDeadOrZombie_ReportsZombieAsDead(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	var status string
	require.Eventually(t, func() bool {
		status = processDeadOrZombieStatus(pid)
		return status == "Z"
	}, 5*time.Second, 10*time.Millisecond, "expected unreaped child to become a zombie; last observed state=%s", status)

	require.NoError(t, syscall.Kill(pid, 0), "kill -0 should succeed while the child is a zombie; observed state=%s", status)
	require.True(t, processDeadOrZombie(pid), "helper must treat a zombie as dead; observed state=%s", status)

	require.NoError(t, cmd.Wait())
	status = processDeadOrZombieStatus(pid)
	require.Equal(t, "ESRCH", status, "helper must report ESRCH after the zombie is reaped")
	require.True(t, processDeadOrZombie(pid), "helper must treat ESRCH as dead after reaping")
}
