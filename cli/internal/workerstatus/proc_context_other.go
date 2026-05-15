//go:build !linux

package workerstatus

func inferExecutionFromPID(_ int) (beadID, worktree string) {
	return "", ""
}
