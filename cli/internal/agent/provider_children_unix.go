//go:build !windows

package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func scanProviderChildProcessesImpl(ctx context.Context, rootPID int, now time.Time) ([]providerChildProcess, error) {
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
	out2 := make([]providerChildProcess, 0)
	for pid := range descendants {
		if pid == rootPID || pid == self || pid <= 0 {
			continue
		}
		r, ok := row[pid]
		if !ok {
			continue
		}
		provider := providerForCommand(r.Command)
		if provider == "" {
			continue
		}
		started := time.Time{}
		if r.ElapsedSeconds >= 0 {
			started = now.Add(-time.Duration(r.ElapsedSeconds) * time.Second)
		}
		ownerPID, ownerProvider := nearestProviderAncestor(row, rootPID, pid)
		out2 = append(out2, providerChildProcess{
			PID:              pid,
			PPID:             r.PPID,
			Provider:         provider,
			Command:          r.Command,
			StartedAt:        started,
			OwnerProviderPID: ownerPID,
			OwnerProvider:    ownerProvider,
		})
	}
	return out2, nil
}

// nearestProviderAncestor walks pid's PPID chain (via row, the full process
// table — not just provider-classified processes) up to but not including
// rootPID, and returns the PID and provider name of the first ancestor that
// is itself classified as a provider CLI. It returns (0, "") if no such
// ancestor exists before the chain reaches rootPID, is missing from row (the
// parent already exited or is untracked), or a cycle is detected.
func nearestProviderAncestor(row map[int]providerPSRow, rootPID, pid int) (int, string) {
	visited := map[int]struct{}{pid: {}}
	current := pid
	for i := 0; i <= len(row); i++ {
		r, ok := row[current]
		if !ok {
			return 0, ""
		}
		parent := r.PPID
		if parent <= 0 || parent == rootPID || parent == current {
			return 0, ""
		}
		if _, seen := visited[parent]; seen {
			return 0, ""
		}
		visited[parent] = struct{}{}
		pr, ok := row[parent]
		if !ok {
			return 0, ""
		}
		if prov := providerForCommand(pr.Command); prov != "" {
			return parent, prov
		}
		current = parent
	}
	return 0, ""
}

type providerPSRow struct {
	PID            int
	PPID           int
	ElapsedSeconds int64
	Command        string
}

func parseProviderPS(out []byte) []providerPSRow {
	lines := bytes.Split(out, []byte{'\n'})
	rows := make([]providerPSRow, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) < 4 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil {
			continue
		}
		rows = append(rows, providerPSRow{
			PID:            pid,
			PPID:           ppid,
			ElapsedSeconds: parseEtime(fields[2]),
			Command:        strings.Join(fields[3:], " "),
		})
	}
	return rows
}

func parseEtime(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	var days int64
	if i := strings.Index(s, "-"); i >= 0 {
		d, err := strconv.ParseInt(s[:i], 10, 64)
		if err != nil {
			return -1
		}
		days = d
		s = s[i+1:]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int64
	var err error
	switch len(parts) {
	case 3:
		if h, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
			return -1
		}
		if m, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return -1
		}
		if sec, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
			return -1
		}
	case 2:
		if m, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
			return -1
		}
		if sec, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return -1
		}
	default:
		return -1
	}
	return days*86400 + h*3600 + m*60 + sec
}

func collectDescendants(children map[int][]int, root int) map[int]struct{} {
	out := map[int]struct{}{}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range children[pid] {
			if _, seen := out[child]; seen {
				continue
			}
			out[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	return out
}

func terminateProviderChildImpl(pid int) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	signalProviderChildGroup(pid, syscall.SIGTERM)
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !signalProcessAlive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	signalProviderChildGroup(pid, syscall.SIGKILL)
	if err := waitForProviderChildReap(pid, 10*time.Second); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: agent: failed to reap provider child %d after running-phase guard: %v\n", pid, err)
	}
}

// signalProviderChildGroup signals both the process group led by pid (the
// common case: provider CLIs are spawned with Setpgid so pid==pgid, and this
// takes any grandchildren with it) and pid itself. Both are attempted
// unconditionally rather than treating group-kill ESRCH as "nothing more to
// do": ESRCH from kill(-pid, sig) only proves no process GROUP has id==pid —
// it says nothing about whether pid itself is alive. A provider process that
// forked and is no longer its own group leader (e.g. it retained an ancestor
// wrapper's pgid, or the wrapper already exited) hits exactly this case, and
// skipping the bare-pid signal left it running forever while the guard kept
// reporting it as "terminated" (ddx-f2b7cf89).
func signalProviderChildGroup(pid int, sig syscall.Signal) {
	if pid <= 0 {
		return
	}
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid > 0 {
		_ = syscall.Kill(-pgid, sig)
	}
	_ = syscall.Kill(pid, sig)
}

func waitForProviderChildReap(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		_, waitErr := proc.Wait()
		done <- waitErr
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case waitErr := <-done:
		if errors.Is(waitErr, syscall.ECHILD) {
			return nil
		}
		return waitErr
	case <-timer.C:
		return fmt.Errorf("timeout waiting for provider child %d to exit", pid)
	}
}
