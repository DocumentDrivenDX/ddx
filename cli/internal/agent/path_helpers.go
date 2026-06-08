package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func projectStatePath(projectRoot, rel string) string {
	if projectRoot == "" || rel == "" {
		return filepath.Join(projectRoot, rel)
	}

	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == ddxroot.DirName {
		return beadStoreRoot(projectRoot)
	}

	prefix := ddxroot.DirName + string(filepath.Separator)
	if strings.HasPrefix(clean, prefix) {
		trimmed := strings.TrimPrefix(clean, prefix)
		if trimmed == "" || trimmed == "." {
			return beadStoreRoot(projectRoot)
		}
		return filepath.Join(append([]string{beadStoreRoot(projectRoot)}, strings.Split(trimmed, string(filepath.Separator))...)...)
	}

	return filepath.Join(projectRoot, clean)
}

func executeBeadArtifactRoot(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "executions")
}

func beadStoreRoot(projectRoot string) string {
	if projectRoot == "" {
		return ddxroot.JoinProject(projectRoot)
	}
	inTree := filepath.Join(projectRoot, ddxroot.DirName)
	if info, err := os.Stat(inTree); err == nil && info.IsDir() {
		return inTree
	}
	return ddxroot.JoinProject(projectRoot)
}
