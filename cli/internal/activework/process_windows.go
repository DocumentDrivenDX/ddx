//go:build windows

package activework

import "os"

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	return err == nil && proc != nil
}
