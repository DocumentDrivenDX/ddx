//go:build windows

package gitlock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockStaleLockGuardFile(file *os.File) (bool, error) {
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&overlapped,
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func unlockStaleLockGuardFile(file *os.File) error {
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
}
