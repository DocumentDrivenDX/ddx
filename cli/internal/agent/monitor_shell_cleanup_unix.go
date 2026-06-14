//go:build !windows

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func scanMonitorShellsImpl(ctx context.Context, rootPID int, now time.Time) ([]monitorShellProcess, error) {
	// Reuse the existing ps-based enumeration helpers from provider_children_unix.go
	// (parseProviderPS, collectDescendants, providerPSRow — all in this package).
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,etime=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	rows := parseProviderPS(out)
	children := map[int][]int{}
	row := map[int]providerPSRow{}
	for _, r := range rows {
		row[r.PID] = r
		children[r.PPID] = append(children[r.PPID], r.PID)
	}
	self := os.Getpid()
	descendants := collectDescendants(children, rootPID)
	var out2 []monitorShellProcess
	for pid := range descendants {
		if pid == rootPID || pid == self || pid <= 0 {
			continue
		}
		r, ok := row[pid]
		if !ok {
			continue
		}
		if !isShellWithPgrep(r.Command) {
			continue
		}
		started := time.Time{}
		if r.ElapsedSeconds >= 0 {
			started = now.Add(-time.Duration(r.ElapsedSeconds) * time.Second)
		}
		out2 = append(out2, monitorShellProcess{
			PID:       pid,
			Command:   r.Command,
			StartedAt: started,
		})
	}
	return out2, nil
}

// isShellWithPgrep reports whether cmdline belongs to a shell process that
// contains a `pgrep -f` invocation — the precondition for self-matching.
func isShellWithPgrep(cmdline string) bool {
	parts := strings.Fields(strings.TrimSpace(cmdline))
	if len(parts) == 0 {
		return false
	}
	base := filepath.Base(parts[0])
	if _, ok := shellBinaryNames[base]; !ok {
		return false
	}
	return strings.Contains(cmdline, "pgrep -f")
}

// terminateMonitorShellImpl sends SIGTERM then SIGKILL to the specific PID
// (not the process group) so only the stale monitor shell is affected.
func terminateMonitorShellImpl(pid int) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !signalProcessAlive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
