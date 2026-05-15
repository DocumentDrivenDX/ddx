package config

import (
	"os"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
)

func resolveDDxProjectRoot(workingDir string) string {
	if workingDir == "" {
		return ""
	}
	if workspaceRoot := gitpkg.FindNearestDDxWorkspace(workingDir); workspaceRoot != "" {
		return workspaceRoot
	}
	return workingDir
}

func projectStatePath(workingDir string, elems ...string) string {
	if workingDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			workingDir = cwd
		}
	}
	return ddxroot.JoinProject(resolveDDxProjectRoot(workingDir), elems...)
}
