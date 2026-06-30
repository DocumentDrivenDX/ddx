//go:build unix && !linux

package cmd

import (
	"os"
	"syscall"
)

// providerLaunchPrepare is a no-op on non-Linux Unix platforms because
// PR_SET_PDEATHSIG is Linux-only. The orphan reaper plus the
// cmdKillProcessGroup graceful-cancel path serve as the fallback for
// macOS / BSD callers.
func providerLaunchPrepare() error { return nil }

// providerLaunchExec replaces the current process image with the
// resolved provider binary. Same semantics as the Linux variant; only
// the prctl step differs.
func providerLaunchExec(binary string, args []string) error {
	argv := append([]string{binary}, args...)
	return syscall.Exec(binary, argv, os.Environ())
}
