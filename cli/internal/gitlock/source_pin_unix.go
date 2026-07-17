//go:build !windows

package gitlock

import "os"

func openPinnedStaleLock(path string) (*os.File, error) {
	return os.Open(path)
}
