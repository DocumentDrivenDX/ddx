package agent

import (
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

func executeBeadArtifactRoot(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "executions")
}

func executeBeadArtifactPath(projectRoot, attemptID string, elems ...string) string {
	parts := []string{"executions"}
	if attemptID != "" {
		parts = append(parts, attemptID)
	}
	parts = append(parts, elems...)
	return ddxroot.JoinProject(projectRoot, parts...)
}

func agentLogRoot(projectRoot string) string {
	return ddxroot.JoinProject(projectRoot, "agent-logs")
}
