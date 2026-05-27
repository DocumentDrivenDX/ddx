//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

// cmdSetProcessGroup configures cmd so that it and all its children share a
// new process group (Setpgid) and are automatically killed when the parent
// thread dies (Pdeathsig). This ensures that if the parent worker dies
// abnormally (SIGKILL, crash, OOM), the harness child is reaped by the kernel
// rather than orphaning to PID 1.
//
// Pdeathsig is sent when the creating thread dies, not the process, so the
// caller must use runtime.LockOSThread around cmd.Start() to prevent Go
// runtime from migrating the goroutine to a different OS thread. See
// golang.org/issue/27505 for details.
//
// Note: Pdeathsig is Linux-only. macOS and Windows do not support it; the
// graceful shutdown path (cmdKillProcessGroup on context cancellation) serves
// as the fallback for those platforms, and the orphan reaper is the final
// backstop for already-dead workers.
func cmdSetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
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
