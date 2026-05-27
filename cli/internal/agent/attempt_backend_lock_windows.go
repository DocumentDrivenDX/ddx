//go:build windows

package agent

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func inTreeLockAcquire(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var overlapped syscall.Overlapped
	ok, err := syscall.LockFileEx(handle, syscall.LOCKFILE_FAIL_IMMEDIATELY|syscall.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if ok {
		return nil
	}
	if err == syscall.ERROR_LOCK_VIOLATION {
		return fmt.Errorf("lock file is already held by another process")
	}
	return err
}

func inTreeLockRelease(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var overlapped syscall.Overlapped
	ok, err := syscall.UnlockFileEx(handle, 1, 0, &overlapped)
	if ok {
		return nil
	}
	return err
}
