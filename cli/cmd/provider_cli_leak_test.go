package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	providerCLIScanInterval = 50 * time.Millisecond
	providerCLIScanTimeout  = 2 * time.Second
)

type processSnapshot struct {
	PID  int
	PPID int
	Comm string
}

type providerCLIDescendant struct {
	PID  int
	PPID int
	Comm string
}

// TestCmdTestsDoNotSpawnRealProviderCLIs reuses representative cmd tests and
// checks only descendants of this test process, so unrelated interactive
// provider sessions on the host are ignored.
func TestCmdTestsDoNotSpawnRealProviderCLIs(t *testing.T) {
	cases := []struct {
		name string
		fn   func(*testing.T)
	}{
		{
			name: "TestWorkResourceExhaustionEndToEnd_StopsBeforeNextClaim",
			fn:   TestWorkResourceExhaustionEndToEnd_StopsBeforeNextClaim,
		},
		{
			name: "TestWorkDoesNotSpawnProviderAfterUnderSpecifiedRouting",
			fn:   TestWorkDoesNotSpawnProviderAfterUnderSpecifiedRouting,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				if err := ensureNoRealProviderCLIDescendants(os.Getpid()); err != nil {
					t.Fatalf("provider descendant leak check failed: %v", err)
				}
			})
			tc.fn(t)
		})
	}
}

func ensureNoRealProviderCLIDescendants(rootPID int) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	leaks, err := waitForNoRealProviderCLIDescendants(rootPID, providerCLIScanTimeout)
	if err != nil {
		return err
	}
	if len(leaks) > 0 {
		return fmt.Errorf("real provider CLI descendants still alive:\n%s", formatProviderCLIDescendants(leaks))
	}
	return nil
}

func waitForNoRealProviderCLIDescendants(rootPID int, timeout time.Duration) ([]providerCLIDescendant, error) {
	deadline := time.Now().Add(timeout)
	for {
		leaks, err := realProviderCLIDescendants(rootPID)
		if err != nil {
			return nil, err
		}
		if len(leaks) == 0 {
			return nil, nil
		}
		if time.Now().After(deadline) {
			return leaks, nil
		}
		time.Sleep(providerCLIScanInterval)
	}
}

func realProviderCLIDescendants(rootPID int) ([]providerCLIDescendant, error) {
	snaps, err := snapshotProcessTable()
	if err != nil {
		return nil, err
	}

	childrenByPPID := make(map[int][]processSnapshot, len(snaps))
	for _, snap := range snaps {
		childrenByPPID[snap.PPID] = append(childrenByPPID[snap.PPID], snap)
	}

	seen := map[int]struct{}{rootPID: struct{}{}}
	queue := []int{rootPID}
	var leaks []providerCLIDescendant

	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range childrenByPPID[pid] {
			if _, ok := seen[child.PID]; ok {
				continue
			}
			seen[child.PID] = struct{}{}
			queue = append(queue, child.PID)
			if isRealProviderCLIComm(child.Comm) {
				leaks = append(leaks, providerCLIDescendant(child))
			}
		}
	}

	sort.Slice(leaks, func(i, j int) bool {
		if leaks[i].Comm == leaks[j].Comm {
			return leaks[i].PID < leaks[j].PID
		}
		return leaks[i].Comm < leaks[j].Comm
	})
	return leaks, nil
}

// snapshotProcessTable gathers a process snapshot that we can walk from the
// current test binary downward. The scan never looks outside that descendant
// tree, which keeps unrelated interactive provider sessions out of scope.
func snapshotProcessTable() ([]processSnapshot, error) {
	var out []byte
	var err error
	for _, args := range [][]string{
		{"-A", "-o", "pid=", "-o", "ppid=", "-o", "comm="},
		{"-ax", "-o", "pid=", "-o", "ppid=", "-o", "comm="},
	} {
		out, err = exec.Command("ps", args...).CombinedOutput()
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("listing processes with ps failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	outSnaps := make([]processSnapshot, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		if pidErr != nil || ppidErr != nil {
			continue
		}
		outSnaps = append(outSnaps, processSnapshot{
			PID:  pid,
			PPID: ppid,
			Comm: filepath.Base(strings.ToLower(strings.TrimSpace(fields[2]))),
		})
	}
	return outSnaps, nil
}

func isRealProviderCLIComm(comm string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(comm))) {
	case "codex", "claude", "gemini":
		return true
	default:
		return false
	}
}

func formatProviderCLIDescendants(leaks []providerCLIDescendant) string {
	if len(leaks) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(leaks))
	for _, leak := range leaks {
		lines = append(lines, fmt.Sprintf("pid=%d ppid=%d comm=%q", leak.PID, leak.PPID, leak.Comm))
	}
	return strings.Join(lines, "\n")
}
