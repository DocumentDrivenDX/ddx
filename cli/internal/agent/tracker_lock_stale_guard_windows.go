//go:build windows

package agent

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockTrackerStaleBreakGuardFile(guard *os.File) (bool, error) {
	handle := windows.Handle(guard.Fd())
	var overlapped windows.Overlapped
	err := windows.LockFileEx(handle, windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func unlockTrackerStaleBreakGuardFile(guard *os.File) error {
	handle := windows.Handle(guard.Fd())
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
}
