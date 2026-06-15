//go:build !windows

package server

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// isPIDAlive reports whether a process with the given PID is alive.
// Returns false if pid <= 0 or if the process does not exist (ESRCH).
// A zombie process (which has exited but not been waited) returns true
// because its PID slot is still held; the caller treats it as dead for
// prune purposes only when combined with an age check.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) != syscall.ESRCH
}

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

type workerProcessRow struct {
	PID     int
	PPID    int
	PGID    int
	Command string
}

type workerDescendantCleanupCandidate struct {
	PID     int
	PGID    int
	Command string
	Reason  string
}

var workerProviderCLINames = map[string]struct{}{
	"claude":   {},
	"codex":    {},
	"gemini":   {},
	"opencode": {},
	"pi":       {},
}

func terminateWorkerDescendants(rootPID int, attemptID string, grace time.Duration) {
	if rootPID <= 0 {
		return
	}
	rows, err := scanWorkerProcesses(context.Background())
	if err != nil {
		return
	}
	candidates := workerDescendantCleanupCandidates(rows, rootPID, attemptID)
	if len(candidates) == 0 {
		return
	}
	killWorkerDescendants(candidates, grace)
}

func terminateWorkerDescendantsUntilQuiet(rootPID int, attemptID string, grace time.Duration) {
	if rootPID <= 0 {
		return
	}
	quietFor := 250 * time.Millisecond
	deadline := time.Now().Add(grace + quietFor + 2*time.Second)
	quietSince := time.Now()
	for {
		rows, err := scanWorkerProcesses(context.Background())
		if err != nil {
			return
		}
		candidates := workerDescendantCleanupCandidates(rows, rootPID, attemptID)
		if len(candidates) > 0 {
			killWorkerDescendants(candidates, grace)
			quietSince = time.Now()
		}
		if time.Since(quietSince) >= quietFor {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func scanWorkerProcesses(ctx context.Context) ([]workerProcessRow, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseWorkerPS(out), nil
}

func parseWorkerPS(out []byte) []workerProcessRow {
	lines := bytes.Split(out, []byte{'\n'})
	rows := make([]workerProcessRow, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) < 4 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil {
			continue
		}
		rows = append(rows, workerProcessRow{
			PID:     pid,
			PPID:    ppid,
			PGID:    pgid,
			Command: strings.Join(fields[3:], " "),
		})
	}
	return rows
}

func workerDescendantCleanupCandidates(rows []workerProcessRow, rootPID int, attemptID string) []workerDescendantCleanupCandidate {
	byPID := map[int]workerProcessRow{}
	children := map[int][]int{}
	for _, row := range rows {
		byPID[row.PID] = row
		children[row.PPID] = append(children[row.PPID], row.PID)
	}
	descendants := collectWorkerDescendants(children, rootPID)
	self := os.Getpid()
	include := map[int]string{}
	for pid := range descendants {
		if pid <= 0 || pid == rootPID || pid == self {
			continue
		}
		row, ok := byPID[pid]
		if !ok {
			continue
		}
		reason := ""
		switch {
		case workerCommandMatchesAttempt(row.Command, attemptID):
			reason = "attempt_finalization"
		case workerProviderForCommand(row.Command) != "":
			reason = "provider_cli"
		}
		if reason == "" {
			continue
		}
		include[pid] = reason
		for childPID := range collectWorkerDescendants(children, pid) {
			if childPID <= 0 || childPID == rootPID || childPID == self {
				continue
			}
			if _, ok := byPID[childPID]; ok {
				include[childPID] = reason
			}
		}
	}
	out := make([]workerDescendantCleanupCandidate, 0, len(include))
	for pid, reason := range include {
		row := byPID[pid]
		out = append(out, workerDescendantCleanupCandidate{
			PID:     row.PID,
			PGID:    row.PGID,
			Command: row.Command,
			Reason:  reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		di := workerProcessDepth(byPID, out[i].PID)
		dj := workerProcessDepth(byPID, out[j].PID)
		if di == dj {
			return out[i].PID > out[j].PID
		}
		return di > dj
	})
	return out
}

func collectWorkerDescendants(children map[int][]int, rootPID int) map[int]struct{} {
	out := map[int]struct{}{}
	queue := []int{rootPID}
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

func workerProcessDepth(byPID map[int]workerProcessRow, pid int) int {
	depth := 0
	for pid > 0 {
		row, ok := byPID[pid]
		if !ok || row.PPID <= 0 || row.PPID == pid {
			return depth
		}
		depth++
		pid = row.PPID
	}
	return depth
}

func workerCommandMatchesAttempt(command, attemptID string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if attemptID = strings.TrimSpace(attemptID); attemptID != "" && strings.Contains(command, attemptID) {
		return true
	}
	return strings.Contains(command, "Ddx-Attempt-Id") ||
		strings.Contains(command, ".git/hooks/pre-commit") ||
		strings.Contains(command, "lefthook run pre-commit")
}

func workerProviderForCommand(cmdline string) string {
	parts := strings.Fields(strings.TrimSpace(cmdline))
	if len(parts) == 0 {
		return ""
	}
	base := filepath.Base(parts[0])
	if strings.HasPrefix(base, "[") && strings.HasSuffix(base, "]") {
		base = strings.TrimSuffix(strings.TrimPrefix(base, "["), "]")
	}
	if _, ok := workerProviderCLINames[base]; ok {
		return base
	}
	if base == "node" && len(parts) >= 2 {
		if argBase := filepath.Base(parts[1]); argBase != "" && argBase != "." {
			if _, ok := workerProviderCLINames[argBase]; ok {
				return argBase
			}
		}
	}
	return ""
}

func killWorkerDescendants(candidates []workerDescendantCleanupCandidate, grace time.Duration) {
	if grace <= 0 || grace > 2*time.Second {
		grace = 2 * time.Second
	}
	for _, proc := range candidates {
		signalWorkerCleanupCandidate(proc, syscall.SIGTERM)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if allWorkerCleanupCandidatesGone(candidates) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, proc := range candidates {
		if !workerProcessAlive(proc.PID) {
			continue
		}
		signalWorkerCleanupCandidate(proc, syscall.SIGKILL)
	}
	killDeadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(killDeadline) {
		if allWorkerCleanupCandidatesGone(candidates) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func signalWorkerCleanupCandidate(proc workerDescendantCleanupCandidate, sig syscall.Signal) {
	if proc.PID <= 0 || proc.PID == os.Getpid() {
		return
	}
	if proc.PGID == proc.PID {
		if err := syscall.Kill(-proc.PID, sig); err == nil || err == syscall.ESRCH {
			return
		}
	}
	_ = syscall.Kill(proc.PID, sig)
}

func allWorkerCleanupCandidatesGone(candidates []workerDescendantCleanupCandidate) bool {
	allGone := true
	for _, proc := range candidates {
		if waitWorkerChildNoHang(proc.PID) {
			continue
		}
		if workerProcessAlive(proc.PID) {
			allGone = false
		}
	}
	return allGone
}

func waitWorkerChildNoHang(pid int) bool {
	if pid <= 0 {
		return true
	}
	var status syscall.WaitStatus
	waited, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
	return err == nil && waited == pid
}

func workerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) != syscall.ESRCH
}
