package server

import (
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// resolveProjectStatePath maps a logical path under .ddx/ to the project's
// configured DDx root, while leaving non-DDx-relative paths rooted at the
// project worktree.
func resolveProjectStatePath(projectRoot, rel string) string {
	if projectRoot == "" || rel == "" {
		return filepath.Join(projectRoot, rel)
	}

	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ddxroot.DirName {
		return ddxroot.JoinProject(projectRoot)
	}

	prefix := ddxroot.DirName + string(filepath.Separator)
	if strings.HasPrefix(clean, prefix) {
		trimmed := strings.TrimPrefix(clean, prefix)
		if trimmed == "" || trimmed == "." {
			return ddxroot.JoinProject(projectRoot)
		}
		return ddxroot.JoinProject(projectRoot, strings.Split(trimmed, string(filepath.Separator))...)
	}

	return filepath.Join(projectRoot, clean)
}
