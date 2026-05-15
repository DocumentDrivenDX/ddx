package exec

import (
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func execRootPath(workingDir string) string {
	return ddxroot.JoinProject(workingDir, "exec")
}

func execDefinitionsPath(workingDir string) string {
	return ddxroot.JoinProject(workingDir, "exec", "definitions")
}

func execRunsPath(workingDir string) string {
	return ddxroot.JoinProject(workingDir, "exec", "runs")
}

func execAttachmentRootPath(workingDir string) string {
	return ddxroot.JoinProject(workingDir, execRunAttachmentDir)
}

func execAttachmentPath(workingDir, ref string) string {
	if ref == "" {
		return ""
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return ddxroot.JoinProject(workingDir, filepath.FromSlash(ref))
}
