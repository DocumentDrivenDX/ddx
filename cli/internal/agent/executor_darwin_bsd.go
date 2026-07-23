//go:build !windows && !linux

package agent

import (
	"os/exec"
	"syscall"
)

// cmdSetProcessGroup configures cmd so that it and all its children share a
// new process group (Setpgid). Pdeathsig is Linux-only; on other Unix
// platforms the graceful shutdown path (cmdKillProcessGroup on context
// cancellation) is the primary cleanup and the orphan reaper is the final
// backstop for already-dead workers.
func cmdSetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// cmdKillProcessGroup sends SIGKILL to the entire process group of cmd.
// Because cmd was started with Setpgid=true, its PID equals its PGID, so
// passing -pid kills all descendants that inherited the group.
func cmdKillProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
