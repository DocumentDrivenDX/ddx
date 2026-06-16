package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
)

func syncBuiltinDDxSkillAdapters(projectRoot string, force bool) ([]string, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}

	pkg, err := registry.BuiltinRegistry().Find("ddx")
	if err != nil {
		return nil, err
	}
	cachePath := registry.PluginCacheDir(pkg.Name, pkg.Version)
	if err := ensureBuiltinDDxCache(cachePath, force); err != nil {
		return nil, err
	}
	if force {
		for _, surface := range []string{filepath.Join(projectRoot, ".agents", "skills"), filepath.Join(projectRoot, ".claude", "skills")} {
			cleanupBootstrapSkills(surface, []string{"ddx"})
		}
	}
	result, err := registry.SyncProjectPlugin(context.Background(), projectRoot, registry.PluginLockEntry{
		Name:      pkg.Name,
		Version:   pkg.Version,
		Type:      pkg.Type,
		Source:    pkg.Source,
		CachePath: cachePath,
	}, force)
	if err != nil {
		return nil, fmt.Errorf("sync baked-in ddx skill adapters: %w", err)
	}
	return result.GeneratedFiles, nil
}

func ensureBuiltinDDxCache(cachePath string, force bool) error {
	return registry.EnsureBuiltinDDxCache(cachePath, force)
}
