package agent

import "strings"

// IsLockContentionError reports whether errMsg names a transient lock-acquire
// failure that the dispatcher should retry. Patterns cover both git's index
// lock and ddx's own staging/tracker locks.
func IsLockContentionError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return containsAny(lower,
		".git/index.lock",
		"unable to create '.git/index.lock'",
		"another git process seems to be running",
		"index.lock: file exists",
		"staging-tracker lock",
		"tracker lock held",
		"tracker is locked",
	)
}

