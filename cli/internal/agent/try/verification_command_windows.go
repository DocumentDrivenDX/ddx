//go:build windows

package try

import "os/exec"

func setVerificationCommandProcessGroup(cmd *exec.Cmd) {}

func killVerificationCommandProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
