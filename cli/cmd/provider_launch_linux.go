//go:build linux

package cmd

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// providerLaunchPrepare moves this process into its own process group and
// sets PR_SET_PDEATHSIG=SIGKILL on it, so the kernel reaps this process
// when its parent (the ddx worker that spawned the shim) dies.
//
// The production provider path never calls BuildProviderLaunchCmd's
// Setpgid (that helper is only used by the unit-testable construction
// site, not the PATH-shim chain fizeau actually execs), so whatever
// process group this wrapper was forked into is whatever the caller
// (the PATH shim's shell, ultimately started by fizeau) happened to
// inherit — which may be the ddx worker's own group. setpgid(0, 0) here
// makes this process the leader of a brand-new group (pgid == pid)
// regardless of what the caller did, so an upstream lifecycle timeout
// that signals "the provider's process group" cannot also signal the
// worker's group. Process-group membership survives execve(2)
// unconditionally, so the new group carries over once we execve into
// the real provider binary in providerLaunchExec.
//
// Pdeathsig is preserved across execve(2) for ordinary binaries — see
// prctl(2):
//
//	"otherwise, this value is preserved across execve(2)"
//
// so the death-signal carries over once we execve into the real
// provider binary in providerLaunchExec.
func providerLaunchPrepare() error {
	setpgidErr := unix.Setpgid(0, 0)

	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGKILL), 0, 0, 0); err != nil {
		return errors.Join(setpgidErrorf(setpgidErr), fmt.Errorf("PR_SET_PDEATHSIG: %w", err))
	}
	// Defensive: if our parent already died in the gap between fork()
	// and now (so getppid()==1), exit immediately rather than execing
	// into a binary nobody will reap.
	if os.Getppid() == 1 {
		os.Exit(0)
	}
	return setpgidErrorf(setpgidErr)
}

// setpgidErrorf wraps a setpgid failure for reporting, or returns nil if
// setpgid succeeded. Kept separate so providerLaunchPrepare's PR_SET_PDEATHSIG
// and setpgid failures can both surface without one masking the other.
func setpgidErrorf(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("setpgid: %w", err)
}

// providerLaunchExec replaces the current process image with the
// resolved provider binary. On success it does NOT return — the kernel
// transfers control to the new binary while preserving the pdeathsig
// configured above.
func providerLaunchExec(binary string, args []string) error {
	argv := append([]string{binary}, args...)
	return syscall.Exec(binary, argv, os.Environ())
}
