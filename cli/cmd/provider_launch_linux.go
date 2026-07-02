//go:build linux

package cmd

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// providerLaunchPrepare sets PR_SET_PDEATHSIG=SIGKILL on the current
// thread/process so the kernel reaps this process when its parent (the
// ddx worker that spawned the shim) dies. Pdeathsig is preserved across
// execve(2) for ordinary binaries — see prctl(2):
//
//	"otherwise, this value is preserved across execve(2)"
//
// so the death-signal carries over once we execve into the real
// provider binary in providerLaunchExec.
func providerLaunchPrepare() error {
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGKILL), 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_PDEATHSIG: %w", err)
	}
	// Defensive: if our parent already died in the gap between fork()
	// and now (so getppid()==1), exit immediately rather than execing
	// into a binary nobody will reap.
	if os.Getppid() == 1 {
		os.Exit(0)
	}
	return nil
}

// providerLaunchExec replaces the current process image with the
// resolved provider binary. On success it does NOT return — the kernel
// transfers control to the new binary while preserving the pdeathsig
// configured above.
func providerLaunchExec(binary string, args []string) error {
	argv := append([]string{binary}, args...)
	return syscall.Exec(binary, argv, os.Environ())
}
