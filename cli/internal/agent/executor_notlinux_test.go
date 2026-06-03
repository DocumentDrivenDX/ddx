//go:build !linux && !windows

package agent

import (
	"os/exec"
	"runtime"
	"testing"
)

// TestOSExecutorExecuteInDir_NonLinuxParentDeathFallbackDocumentsGap documents
// the parent-death cleanup gap on non-Linux platforms. executor_unix.go sets
// Pdeathsig in SysProcAttr for all non-Windows platforms, but the kernel only
// enforces it on Linux (and NetBSD). On macOS and other BSDs the field compiles
// and is accepted but has no effect.
//
// Compensating controls on non-Linux platforms:
//   - cmdKillProcessGroup via ctx cancellation (graceful-shutdown path in executor.go)
//   - startup orphan reaper (ddx-8f2e0ebf) as a final backstop for already-dead workers
//
// This test verifies that process-group isolation (Setpgid) is still configured
// so that cmdKillProcessGroup can reach all descendants via kill(-pgid), and
// logs the platform gap so reviewers have an explicit record.
func TestOSExecutorExecuteInDir_NonLinuxParentDeathFallbackDocumentsGap(t *testing.T) {
	cmd := exec.Command("sleep", "1")
	cmdSetProcessGroup(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr must be set: process-group isolation is required even on non-Linux platforms")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("Setpgid must be true so cmdKillProcessGroup can reach all descendants via kill(-pgid)")
	}

	// Pdeathsig is set in SysProcAttr (executor_unix.go) but is not kernel-enforced
	// on this platform. Abnormal-exit orphan cleanup relies on the compensating
	// controls documented above.
	t.Logf("non-Linux platform (%s): Pdeathsig in SysProcAttr is not kernel-enforced; "+
		"graceful-cancel path (cmdKillProcessGroup) and orphan-reaper (ddx-8f2e0ebf) "+
		"are the compensating controls", runtime.GOOS)
}
