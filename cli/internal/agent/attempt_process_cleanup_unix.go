//go:build !windows

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const attemptProcessCleanupArtifact = "process-cleanup.json"

type attemptProcessSnapshot struct {
	PIDs map[int]struct{}
	Err  string
}

type attemptProcessInfo struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	PGID    int    `json:"pgid,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Command string `json:"command,omitempty"`
}

type attemptProcessCleanupReport struct {
	AttemptID   string               `json:"attempt_id"`
	BeadID      string               `json:"bead_id"`
	Worktree    string               `json:"worktree"`
	Trigger     string               `json:"trigger"`
	StartedAt   time.Time            `json:"started_at"`
	FinishedAt  time.Time            `json:"finished_at"`
	Scanned     int                  `json:"scanned"`
	Candidates  []attemptProcessInfo `json:"candidates,omitempty"`
	Killed      []attemptProcessInfo `json:"killed,omitempty"`
	StillAlive  []attemptProcessInfo `json:"still_alive,omitempty"`
	BaselineErr string               `json:"baseline_error,omitempty"`
	ScanErr     string               `json:"scan_error,omitempty"`
	KillErrors  []string             `json:"kill_errors,omitempty"`
}

func captureAttemptProcessBaseline(ctx context.Context, worktree string) attemptProcessSnapshot {
	processes, err := scanAttemptProcesses(ctx)
	if err != nil {
		return attemptProcessSnapshot{PIDs: map[int]struct{}{}, Err: err.Error()}
	}
	pids := make(map[int]struct{}, len(processes))
	for _, proc := range attemptCleanupCandidates(processes, nil, worktree) {
		pids[proc.PID] = struct{}{}
	}
	return attemptProcessSnapshot{PIDs: pids}
}

func cleanupAttemptProcesses(ctx context.Context, projectRoot, beadID, attemptID, worktree string, baseline attemptProcessSnapshot, trigger string) *attemptProcessCleanupReport {
	started := time.Now().UTC()
	report := &attemptProcessCleanupReport{
		AttemptID:   attemptID,
		BeadID:      beadID,
		Worktree:    worktree,
		Trigger:     trigger,
		StartedAt:   started,
		BaselineErr: baseline.Err,
	}
	processes, err := scanAttemptProcesses(context.Background())
	if err != nil {
		report.ScanErr = err.Error()
		report.FinishedAt = time.Now().UTC()
		writeAttemptProcessCleanupArtifact(projectRoot, attemptID, report)
		return report
	}
	report.Scanned = len(processes)
	candidates := attemptCleanupCandidates(processes, baseline.PIDs, worktree)
	report.Candidates = candidates
	if len(candidates) == 0 && trigger == "" && baseline.Err == "" {
		return nil
	}

	killAttemptProcesses(candidates, report)

	after, scanErr := scanAttemptProcesses(context.Background())
	if scanErr != nil {
		report.ScanErr = strings.TrimSpace(strings.Join([]string{report.ScanErr, scanErr.Error()}, "\n"))
	} else {
		live := map[int]attemptProcessInfo{}
		for _, proc := range after {
			live[proc.PID] = proc
		}
		for _, proc := range candidates {
			if current, ok := live[proc.PID]; ok && signalProcessAlive(proc.PID) {
				report.StillAlive = append(report.StillAlive, current)
			} else {
				report.Killed = append(report.Killed, proc)
			}
		}
	}
	report.FinishedAt = time.Now().UTC()
	writeAttemptProcessCleanupArtifact(projectRoot, attemptID, report)
	return report
}

func writeAttemptProcessCleanupArtifact(projectRoot, attemptID string, report *attemptProcessCleanupReport) {
	if report == nil || strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(attemptID) == "" {
		return
	}
	path := filepath.Join(projectRoot, ExecuteBeadArtifactDir, attemptID, attemptProcessCleanupArtifact)
	_ = writeArtifactJSON(path, report)
}

func attemptCleanupCandidates(processes []attemptProcessInfo, baseline map[int]struct{}, worktree string) []attemptProcessInfo {
	self := os.Getpid()
	out := make([]attemptProcessInfo, 0)
	for _, proc := range processes {
		if proc.PID <= 0 || proc.PID == self {
			continue
		}
		if baseline != nil {
			if _, seen := baseline[proc.PID]; seen {
				continue
			}
		}
		if processCwdWithin(proc.Cwd, worktree) || processCommandMentionsAttempt(proc.Command, worktree) {
			out = append(out, proc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		di := processDepth(processes, out[i].PID)
		dj := processDepth(processes, out[j].PID)
		if di == dj {
			return out[i].PID > out[j].PID
		}
		return di > dj
	})
	return out
}

func processDepth(processes []attemptProcessInfo, pid int) int {
	parent := map[int]int{}
	for _, proc := range processes {
		parent[proc.PID] = proc.PPID
	}
	depth := 0
	for pid > 0 {
		ppid := parent[pid]
		if ppid <= 0 || ppid == pid {
			return depth
		}
		depth++
		pid = ppid
	}
	return depth
}

func processCommandMentionsAttempt(command, worktree string) bool {
	if strings.TrimSpace(command) == "" || strings.TrimSpace(worktree) == "" {
		return false
	}
	return strings.Contains(command, worktree) || strings.Contains(command, filepath.Base(worktree))
}

func processCwdWithin(cwd, root string) bool {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(root) == "" {
		return false
	}
	cleanCwd := canonicalCleanupPath(cwd)
	cleanRoot := canonicalCleanupPath(root)
	return cleanCwd == cleanRoot || strings.HasPrefix(cleanCwd, cleanRoot+string(filepath.Separator))
}

func canonicalCleanupPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs)
}

func killAttemptProcesses(candidates []attemptProcessInfo, report *attemptProcessCleanupReport) {
	for _, proc := range candidates {
		if proc.PID <= 0 || proc.PID == os.Getpid() {
			continue
		}
		if err := syscall.Kill(proc.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			report.KillErrors = append(report.KillErrors, fmt.Sprintf("SIGTERM pid %d: %v", proc.PID, err))
		}
	}
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		allGone := true
		for _, proc := range candidates {
			if signalProcessAlive(proc.PID) {
				allGone = false
				break
			}
		}
		if allGone {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, proc := range candidates {
		if !signalProcessAlive(proc.PID) {
			continue
		}
		if err := syscall.Kill(proc.PID, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			report.KillErrors = append(report.KillErrors, fmt.Sprintf("SIGKILL pid %d: %v", proc.PID, err))
		}
	}
}

func signalProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func scanAttemptProcesses(ctx context.Context) ([]attemptProcessInfo, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	processes := parseAttemptPS(out)
	for i := range processes {
		processes[i].Cwd = readProcessCwd(processes[i].PID)
	}
	return processes, nil
}

func parseAttemptPS(out []byte) []attemptProcessInfo {
	lines := bytes.Split(out, []byte{'\n'})
	processes := make([]attemptProcessInfo, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		if pidErr != nil || ppidErr != nil || pgidErr != nil {
			continue
		}
		cmdline := ""
		if len(fields) > 3 {
			cmdline = strings.Join(fields[3:], " ")
		}
		processes = append(processes, attemptProcessInfo{
			PID:     pid,
			PPID:    ppid,
			PGID:    pgid,
			Command: cmdline,
		})
	}
	return processes
}

func readProcessCwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	if cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd")); err == nil {
		return cwd
	}
	return ""
}
