//go:build !windows

package server

import (
	"bytes"
	"context"
	"os/exec"
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

type unixProcessSample struct {
	PID  int
	PPID int
	PGID int
}

// cleanupManagedWorkerProcessTree sends SIGTERM, waits grace, then SIGKILLs any
// still-live server-owned process groups for the worker. When a worker owns a
// real process group, pid == pgid and that group is targeted directly. Child
// process groups are discovered from the worker's descendants and any
// additional registered PGIDs are included as well. The helper is idempotent:
// already-gone groups are ignored.
func cleanupManagedWorkerProcessTree(pid int, registeredPGIDs []int, grace time.Duration) managedProcessCleanupReport {
	report := managedProcessCleanupReport{
		RootPID:         pid,
		RegisteredPGIDs: uniqueSortedInts(registeredPGIDs),
	}
	targets := map[int]struct{}{}
	if pgid, ok := managedWorkerRootPGID(pid); ok {
		targets[pgid] = struct{}{}
	}
	for _, pgid := range scanManagedWorkerDescendantPGIDs(pid) {
		targets[pgid] = struct{}{}
	}
	for _, pgid := range report.RegisteredPGIDs {
		targets[pgid] = struct{}{}
	}
	if len(targets) == 0 {
		return report
	}
	report.TargetPGIDs = sortedIntSet(targets)
	for _, pgid := range report.TargetPGIDs {
		if terminateProcessGroupOnce(pgid, syscall.SIGTERM) {
			report.TerminatedPGIDs = append(report.TerminatedPGIDs, pgid)
		}
	}

	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if len(aliveManagedProcessGroups(report.TargetPGIDs)) == 0 {
			return report
		}
		time.Sleep(50 * time.Millisecond)
	}

	for _, pgid := range aliveManagedProcessGroups(report.TargetPGIDs) {
		if terminateProcessGroupOnce(pgid, syscall.SIGKILL) {
			report.KilledPGIDs = append(report.KilledPGIDs, pgid)
		}
	}
	return report
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
	_ = cleanupManagedWorkerProcessTree(pid, nil, grace)
}

func managedWorkerRootPGID(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}
	pgid, err := syscall.Getpgid(pid)
	if err == nil && pgid == pid {
		return pgid, true
	}
	if err == syscall.ESRCH && syscall.Kill(-pid, 0) == nil {
		return pid, true
	}
	return 0, false
}

func scanManagedWorkerDescendantPGIDs(rootPID int) []int {
	if rootPID <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	rows := parseManagedWorkerPS(out)
	children := map[int][]int{}
	pgidByPID := map[int]int{}
	for _, row := range rows {
		children[row.PPID] = append(children[row.PPID], row.PID)
		pgidByPID[row.PID] = row.PGID
	}
	descendants := collectManagedWorkerDescendants(children, rootPID)
	targets := make(map[int]struct{}, len(descendants))
	for pid := range descendants {
		pgid := pgidByPID[pid]
		if pgid > 0 {
			targets[pgid] = struct{}{}
		}
	}
	return sortedIntSet(targets)
}

func parseManagedWorkerPS(out []byte) []unixProcessSample {
	lines := bytes.Split(out, []byte{'\n'})
	rows := make([]unixProcessSample, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		pgid, err3 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		rows = append(rows, unixProcessSample{PID: pid, PPID: ppid, PGID: pgid})
	}
	return rows
}

func collectManagedWorkerDescendants(children map[int][]int, root int) map[int]struct{} {
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

func sortedIntSet(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	// Small slices; insertion order doesn't matter, but the stable sort keeps
	// lifecycle evidence deterministic for tests.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func aliveManagedProcessGroups(pgids []int) []int {
	if len(pgids) == 0 {
		return nil
	}
	alive := make([]int, 0, len(pgids))
	for _, pgid := range pgids {
		if pgid <= 0 {
			continue
		}
		if syscall.Kill(-pgid, 0) == nil {
			alive = append(alive, pgid)
		}
	}
	return alive
}

func terminateProcessGroupOnce(pgid int, sig syscall.Signal) bool {
	if pgid <= 0 {
		return false
	}
	if err := syscall.Kill(-pgid, sig); err != nil && err != syscall.ESRCH {
		return false
	}
	return true
}
