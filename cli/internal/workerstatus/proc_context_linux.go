//go:build linux

package workerstatus

import (
	"os"
	"path/filepath"
	"strconv"
)

func inferExecutionFromPID(pid int) (beadID, worktree string) {
	if pid <= 0 {
		return "", ""
	}
	base := filepath.Join("/proc", strconv.Itoa(pid))
	cwd, _ := os.Readlink(filepath.Join(base, "cwd"))
	beadID, worktree = InferBead("", cwd)
	if worktree != "" {
		return beadID, worktree
	}
	raw, err := os.ReadFile(filepath.Join(base, "cmdline"))
	if err != nil {
		return beadID, worktree
	}
	return InferBead(decodeCmdline(raw), cwd)
}
