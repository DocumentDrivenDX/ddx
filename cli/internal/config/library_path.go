package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
)

// ResolveLibraryPath resolves the DDx library for workingDir.
//
// Explicit config wins. Legacy projects that still carry .ddx/plugins/ddx keep
// working. New cache-backed projects fall through to the built-in DDx package
// cache so they do not need to check plugin payloads into the repository.
func ResolveLibraryPath(workingDir string) (string, error) {
	if workingDir == "" {
		workingDir = "."
	}
	cfg, err := LoadWithWorkingDir(workingDir)
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Library != nil {
		if configured := strings.TrimSpace(cfg.Library.Path); configured != "" {
			if filepath.IsAbs(configured) {
				return configured, nil
			}
			return filepath.Join(workingDir, configured), nil
		}
	}
	if legacyPath, ok := existingLegacyProjectLibraryPath(workingDir); ok {
		return legacyPath, nil
	}
	return builtinDDxLibraryPath()
}

func existingLegacyProjectLibraryPath(workingDir string) (string, bool) {
	legacyPath := ddxroot.InTree(workingDir, "plugins", "ddx")
	info, err := os.Stat(legacyPath)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return legacyPath, true
}

func builtinDDxLibraryPath() (string, error) {
	cachePath, err := registry.BuiltinDDxCachePath()
	if err != nil {
		return "", err
	}
	if err := registry.EnsureBuiltinDDxCache(cachePath, false); err != nil {
		return "", err
	}
	return cachePath, nil
}
