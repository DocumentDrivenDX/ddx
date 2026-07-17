//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris || zos

package bead

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockStaleBreakGuardFile(guard *os.File) (bool, error) {
	err := unix.Flock(int(guard.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockStaleBreakGuardFile(guard *os.File) error {
	return unix.Flock(int(guard.Fd()), unix.LOCK_UN)
}
