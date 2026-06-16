package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
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
	if !force {
		if info, err := os.Stat(filepath.Join(cachePath, "skills", "ddx", "SKILL.md")); err == nil && !info.IsDir() {
			return nil
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect baked-in ddx cache: %w", err)
		}
	}
	if err := os.RemoveAll(cachePath); err != nil {
		return fmt.Errorf("clear baked-in ddx cache %s: %w", cachePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("create baked-in ddx cache root: %w", err)
	}
	if err := registry.MaterializeFS(defaultplugin.FS(), cachePath); err != nil {
		return fmt.Errorf("materialize baked-in ddx cache: %w", err)
	}
	return nil
}
