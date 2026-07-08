//go:build windows

package jsonl

import (
	"fmt"
	"os"
	"syscall"
)

func tryLockFile(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var overlapped syscall.Overlapped
	ok, err := syscall.LockFileEx(handle, syscall.LOCKFILE_FAIL_IMMEDIATELY|syscall.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if ok {
		return nil
	}
	if err == syscall.ERROR_LOCK_VIOLATION {
		return errLockHeld
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("lock acquisition failed")
}

func unlockFile(f *os.File) error {
	handle := syscall.Handle(f.Fd())
	var overlapped syscall.Overlapped
	ok, err := syscall.UnlockFileEx(handle, 1, 0, &overlapped)
	if ok {
		return nil
	}
	return err
}
