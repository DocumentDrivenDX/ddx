package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// ResolvePlugin returns the on-disk path and the resolution layer for a plugin.
// Precedence: local project overlay -> project lock/cache -> baked-in default.
//
// Layer values:
//   - "project": found under <projectRoot>/.ddx/plugins/<name>, normally a
//     local developer overlay
//   - "cache": found through <ddx-root>/plugins.lock.yaml and the XDG payload
//     cache
//   - "baked-in": embedded default; only valid for the "ddx" plugin name
//
// For baked-in, path is empty — callers should use defaultplugin.FS().
// Any other unresolved name returns a non-nil error.
func ResolvePlugin(ctx context.Context, projectRoot, name string) (path string, layer string, err error) {
	projectPluginPath := filepath.Join(ddxroot.Path(ctx, projectRoot), "plugins", name)
	if info, statErr := os.Stat(projectPluginPath); statErr == nil && info.IsDir() {
		return projectPluginPath, "project", nil
	}

	if lock, lockErr := LoadProjectPluginLock(ctx, projectRoot); lockErr == nil {
		if entry := lock.Find(name); entry != nil {
			cachePath := entry.CachePath
			if cachePath == "" {
				cachePath = PluginCacheDir(entry.Name, entry.Version)
			}
			if info, statErr := os.Stat(cachePath); statErr == nil && info.IsDir() {
				return cachePath, "cache", nil
			}
		}
	}

	if name == "ddx" {
		return "", "baked-in", nil
	}

	return "", "", fmt.Errorf("plugin %q not found in project lock/cache or baked-in layers", name)
}
