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

// ManagedPathspecs returns the DDx-managed tracker and durable-audit pathspecs.
// Callers must treat the returned slice as read-only.
func ManagedPathspecs() []string {
	return append([]string(nil), managedPathspecs...)
}

// IsManagedTrackerPath reports whether path is one of the DDx-managed tracker
// files or lives under the managed attachments directory.
func IsManagedTrackerPath(path string) bool {
	clean := strings.TrimSpace(filepath.ToSlash(path))
	if clean == "" {
		return false
	}
	for _, managed := range managedPathspecs {
		root := strings.TrimSuffix(filepath.ToSlash(managed), "/")
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}
