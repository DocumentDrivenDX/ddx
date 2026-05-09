//go:build windows

package agent

import "os/exec"

// cmdSetProcessGroup is a no-op on Windows; process group semantics differ.
func cmdSetProcessGroup(cmd *exec.Cmd) {}

// cmdKillProcessGroup falls back to killing only the leader process on Windows.
func cmdKillProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
