package cmd

import (
	"github.com/DocumentDrivenDX/ddx/internal/config"
)

func resolveCommandLibraryPath(workingDir string) (string, error) {
	return config.ResolveLibraryPath(workingDir)
}
