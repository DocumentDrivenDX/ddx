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
	"syscall"
)

type procOrphanHarnessScanner struct{}

func newOrphanHarnessProcessScanner() orphanHarnessProcessScanner {
	return procOrphanHarnessScanner{}
}

func (procOrphanHarnessScanner) Scan(ctx context.Context) ([]orphanHarnessProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}
	out := make([]orphanHarnessProcess, 0)
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
		proc, ok := inspectOrphanHarnessProcess(pid)
		if !ok {
			continue
		}
		out = append(out, proc)
	}
	return out, nil
}

func inspectOrphanHarnessProcess(pid int) (orphanHarnessProcess, bool) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	cmdlineRaw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return orphanHarnessProcess{}, false
	}
	cmdline := decodeOrphanHarnessCmdline(cmdlineRaw)
	if cmdline == "" {
		return orphanHarnessProcess{}, false
	}
	cwd, _ := os.Readlink(filepath.Join(base, "cwd"))
	ppid, ok := readOrphanHarnessParentPID(filepath.Join(base, "stat"))
	if !ok {
		return orphanHarnessProcess{}, false
	}
	return orphanHarnessProcess{
		PID:     pid,
		PPID:    ppid,
		Command: cmdline,
		Cwd:     cwd,
	}, true
}

func decodeOrphanHarnessCmdline(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}
	parts := bytes.Split(raw, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return strings.Join(out, " ")
}

func readOrphanHarnessParentPID(statPath string) (int, bool) {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, false
	}
	close := strings.LastIndex(string(data), ")")
	if close < 0 {
		return 0, false
	}
	tail := strings.Fields(string(data[close+1:]))
	if len(tail) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(tail[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}
