//go:build linux

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
)

// TestWork_NonRouteProviderPidNotProcessGroupLeaderIsReaped proves the guard's
// termination call actually kills a non-route provider child even when that
// child is not its own process-group leader (pid != pgid) — e.g. because it
// retained an unclassified wrapper's process group rather than becoming a
// leader itself. Before the fix, signalProviderChildGroup treated ESRCH from
// the process-group kill as "nothing left to do" and never signalled the bare
// pid, so the guard's "terminated by running-phase guard" diagnostic did not
// reflect reality: the process kept running (and, in production, kept being
// replaced by fresh spawns) while status.json reported it as reaped
// (ddx-f2b7cf89).
func TestWork_NonRouteProviderPidNotProcessGroupLeaderIsReaped(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	dir := t.TempDir()
	codexBin := filepath.Join(dir, "codex")
	if err := os.Symlink(sleepPath, codexBin); err != nil {
		t.Fatalf("symlink fake codex: %v", err)
	}
	pidFile := filepath.Join(dir, "codex.pid")

	// The wrapper's own command line is "sh -c ..." — not a provider CLI
	// name, so only the backgrounded codex child is classified as a
	// provider. The child inherits the wrapper's process group
	// (non-interactive sh keeps no separate group for background jobs), so
	// codex.pid != codex.pgid: killing "-codexPID" as a process group
	// targets a group that does not exist, and the bare-pid fallback is
	// what actually has to land the signal.
	cmd := exec.Command(shPath, "-c", `"$1" 120 & echo $! > "$2"; wait`, "wrapper", codexBin, pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake wrapper: %v", err)
	}
	wrapperPID := cmd.Process.Pid
	go func() { _, _ = cmd.Process.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-wrapperPID, syscall.SIGKILL) })

	codexPID := waitForPIDFile(t, pidFile)
	waitForProviderChildren(t, os.Getpid(), codexPID)

	children, reaped := runningProviderChildGuard(context.Background(), os.Getpid(), "claude/sonnet", "", "running", time.Now().UTC())

	var sawCodex bool
	for _, r := range reaped {
		if r.PID == codexPID && r.Provider == "codex" {
			sawCodex = true
		}
	}
	if !sawCodex {
		t.Fatalf("expected codex (pid %d) to be reported reaped; got %+v", codexPID, reaped)
	}
	var codexChild workerstatus.ProviderChild
	for _, c := range children {
		if c.PID == codexPID {
			codexChild = c
		}
	}
	if !codexChild.NonRoute || codexChild.Diagnostic == "" {
		t.Fatalf("codex must report as non-route with a diagnostic: %+v", codexChild)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !processDeadOrZombie(codexPID) {
		time.Sleep(50 * time.Millisecond)
	}
	if !processDeadOrZombie(codexPID) {
		t.Fatalf("non-route provider child %d must actually be killed within 10s, not merely labeled (proc state=%s)", codexPID, processDeadOrZombieStatus(codexPID))
	}
}
