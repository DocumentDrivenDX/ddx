package trackerpaths

import (
	"path/filepath"
	"strings"
)

var managedPathspecs = []string{
	".ddx/beads.jsonl",
	".ddx/beads-archive.jsonl",
	".ddx/metrics/attempts.jsonl",
	".ddx/attachments",
}

var preClaimNonBlockingRoots = []string{
	".ddx/executions",
}

// ManagedPathspecs returns the DDx-managed tracker and durable-audit pathspecs.
// Callers must treat the returned slice as read-only.
func ManagedPathspecs() []string {
	return append([]string(nil), managedPathspecs...)
}

// IsManagedTrackerPath reports whether path is one of the DDx-managed tracker
// files or lives under the managed attachments directory.
func IsManagedTrackerPath(path string) bool {
	clean := cleanPath(path)
	if clean == "" {
		return false
	}
	for _, managed := range managedPathspecs {
		if pathInRoot(clean, managed) {
			return true
		}
	}
	return false
}

// IsNonBlockingPreClaimPath reports whether a staged path is DDx-owned
// metadata that must not block a worker's next claim. This intentionally
// includes ignored execution evidence, which is too broad for durable-audit
// force-staging but is still not operator code/doc/test work.
func IsNonBlockingPreClaimPath(path string) bool {
	clean := cleanPath(path)
	if clean == "" {
		return false
	}
	if IsManagedTrackerPath(clean) {
		return true
	}
	for _, root := range preClaimNonBlockingRoots {
		if pathInRoot(clean, root) {
			return true
		}
	}
	return false
}

func cleanPath(path string) string {
	return strings.TrimSpace(filepath.ToSlash(path))
}

func pathInRoot(clean, root string) bool {
	root = strings.TrimSuffix(filepath.ToSlash(root), "/")
	return clean == root || strings.HasPrefix(clean, root+"/")
}
