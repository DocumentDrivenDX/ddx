//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris || zos

package gitlock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockStaleLockGuardFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockStaleLockGuardFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
