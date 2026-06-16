package agent

import (
	"context"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
)

// skillLinkDirs lists the project-local skill directories that an execute-bead
// worktree must clean before the agent runs. In the post-FEAT-015 model these
// contain real files; pre-migration worktrees may still have symlinks whose
// targets no longer exist (the old global plugin tree is gone). Broken
// symlinks are removed silently so the harness does not emit "failed to stat"
// errors on startup.
var skillLinkDirs = []string{
	filepath.Join(".agents", "skills"),
	filepath.Join(".claude", "skills"),
}

// materializeWorktreeSkills recreates generated skill adapters in an
// execute-bead worktree. The adapter directories are gitignored in the landing
// project, so `git worktree add` does not copy them. Without recreating them,
// hooks and tests that validate the shipped skill contract fail inside the
// attempt worktree even though the landing project is healthy.
func materializeWorktreeSkills(projectRoot, wtPath string) error {
	for _, rel := range skillLinkDirs {
		dir := filepath.Join(wtPath, rel)
		if err := repairSkillLinkDir(dir); err != nil {
			return err
		}
	}
	if err := syncBuiltinDDxAdaptersForWorktree(wtPath); err != nil {
		return err
	}
	if err := syncProjectPluginAdaptersForWorktree(projectRoot, wtPath); err != nil {
		return err
	}
	return nil
}

// repairSkillLinkDir walks a single skill directory and removes broken
// symlinks. It is a no-op when dir does not exist.
func repairSkillLinkDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		entryPath := filepath.Join(dir, e.Name())
		info, err := os.Lstat(entryPath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		// Resolved successfully? Leave it alone.
		if _, err := os.Stat(entryPath); err == nil {
			continue
		}
		// Broken symlink — remove so the harness stops logging stat errors.
		_ = os.Remove(entryPath)
	}
	return nil
}

func syncBuiltinDDxAdaptersForWorktree(wtPath string) error {
	pkg, err := registry.BuiltinRegistry().Find("ddx")
	if err != nil {
		return err
	}
	cachePath := registry.PluginCacheDir(pkg.Name, pkg.Version)
	if err := ensureBuiltinDDxCacheForWorktree(cachePath, defaultplugin.FS()); err != nil {
		return err
	}
	_, err = registry.SyncProjectPlugin(context.Background(), wtPath, registry.PluginLockEntry{
		Name:      pkg.Name,
		Version:   pkg.Version,
		Type:      pkg.Type,
		Source:    pkg.Source,
		CachePath: cachePath,
	}, false)
	if err != nil {
		return fmt.Errorf("sync built-in ddx skill adapters into execute-bead worktree: %w", err)
	}
	return nil
}

func ensureBuiltinDDxCacheForWorktree(cachePath string, src iofs.FS) error {
	if info, err := os.Stat(filepath.Join(cachePath, "skills", "ddx", "SKILL.md")); err == nil && !info.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect baked-in ddx cache: %w", err)
	}
	if err := os.RemoveAll(cachePath); err != nil {
		return fmt.Errorf("clear baked-in ddx cache %s: %w", cachePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("create baked-in ddx cache root: %w", err)
	}
	if err := registry.MaterializeFS(src, cachePath); err != nil {
		return fmt.Errorf("materialize baked-in ddx cache: %w", err)
	}
	return nil
}

func syncProjectPluginAdaptersForWorktree(projectRoot, wtPath string) error {
	if projectRoot == "" {
		return nil
	}
	lock, err := registry.LoadProjectPluginLock(context.Background(), projectRoot)
	if err != nil {
		return fmt.Errorf("load project plugin lock for execute-bead worktree skills: %w", err)
	}
	for _, entry := range lock.Plugins {
		if _, err := registry.SyncProjectPlugin(context.Background(), wtPath, entry, false); err != nil {
			return fmt.Errorf("sync plugin %s skill adapters into execute-bead worktree: %w", entry.Name, err)
		}
	}
	return nil
}
