//go:build linux

package agent

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestCmdSetProcessGroup_Pdeathsig verifies that cmdSetProcessGroup sets
// Pdeathsig to SIGKILL on Linux to ensure harness children die when the
// parent worker dies abnormally.
func TestCmdSetProcessGroup_Pdeathsig(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	cmdSetProcessGroup(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("Setpgid is not true")
	}
	if cmd.SysProcAttr.Pdeathsig != syscall.SIGKILL {
		t.Errorf("Pdeathsig = %v, want SIGKILL", cmd.SysProcAttr.Pdeathsig)
	}
}
