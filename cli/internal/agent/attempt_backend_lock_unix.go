//go:build !windows

package agent

import (
	"os"
	"syscall"
)

func inTreeLockAcquire(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func inTreeLockRelease(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
