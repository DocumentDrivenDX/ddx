//go:build linux

package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type procExecutionCleanupAttemptProcessScanner struct {
	now func() time.Time
}

func newExecutionCleanupAttemptProcessScannerImpl() executionCleanupAttemptProcessScanner {
	return &procExecutionCleanupAttemptProcessScanner{now: time.Now}
}

func (s *procExecutionCleanupAttemptProcessScanner) Scan(ctx context.Context) ([]executionCleanupAttemptProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}
	bootTime := readExecutionCleanupBootTime("/proc/stat")
	out := make([]executionCleanupAttemptProcess, 0)
	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		proc, ok := s.inspect(pid, bootTime)
		if !ok {
			continue
		}
		out = append(out, proc)
	}
	return out, nil
}

func (s *procExecutionCleanupAttemptProcessScanner) inspect(pid int, bootTime time.Time) (executionCleanupAttemptProcess, bool) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	cmdlineRaw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return executionCleanupAttemptProcess{}, false
	}
	cwd, _ := os.Readlink(filepath.Join(base, "cwd"))
	ppid, pgid, startedAt, ok := readExecutionCleanupProcessStat(filepath.Join(base, "stat"), bootTime, s.now)
	if !ok {
		return executionCleanupAttemptProcess{}, false
	}
	cmdline := decodeExecutionCleanupCmdline(cmdlineRaw)
	proc := executionCleanupAttemptProcessFromWorkerStatus(cmdline, cwd, pid, ppid, pgid, startedAt)
	if proc.Worktree == "" {
		return executionCleanupAttemptProcess{}, false
	}
	return proc, true
}

func decodeExecutionCleanupCmdline(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}
	parts := bytes.Split(raw, []byte{0})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		out = append(out, string(part))
	}
	return strings.Join(out, " ")
}

func readExecutionCleanupBootTime(statPath string) time.Time {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sec, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(sec, 0)
	}
	return time.Time{}
}

func readExecutionCleanupProcessStat(statPath string, bootTime time.Time, now func() time.Time) (ppid int, pgid int, startedAt time.Time, ok bool) {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	close := strings.LastIndex(string(data), ")")
	if close < 0 {
		return 0, 0, time.Time{}, false
	}
	tail := strings.Fields(string(data[close+1:]))
	if len(tail) < 20 {
		return 0, 0, time.Time{}, false
	}
	parsedPPID, err := strconv.Atoi(tail[1])
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	parsedPGID, err := strconv.Atoi(tail[2])
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	startClk, err := strconv.ParseInt(tail[19], 10, 64)
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	if !bootTime.IsZero() {
		seconds := float64(startClk) / 100.0
		startedAt = bootTime.Add(time.Duration(seconds * float64(time.Second)))
	}
	if startedAt.IsZero() {
		if now == nil {
			now = time.Now
		}
		startedAt = now().UTC()
	}
	return parsedPPID, parsedPGID, startedAt.UTC(), true
}
