package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// ResolvePlugin returns the on-disk path and the resolution layer for a plugin.
// Precedence: project → global → baked-in.
//
// Layer values:
//   - "project": found under <projectRoot>/.ddx/plugins/<name>
//   - "global":  found under ${XDG_DATA_HOME}/ddx/global/plugins/<name>
//   - "baked-in": embedded default; only valid for the "ddx" plugin name
//
// For baked-in, path is empty — callers should use defaultplugin.FS().
// Any other unresolved name returns a non-nil error.
func ResolvePlugin(ctx context.Context, projectRoot, name string) (path string, layer string, err error) {
	projectPluginPath := filepath.Join(ddxroot.Path(ctx, projectRoot), "plugins", name)
	if info, statErr := os.Stat(projectPluginPath); statErr == nil && info.IsDir() {
		return projectPluginPath, "project", nil
	}

	globalPluginPath := filepath.Join(ddxroot.GlobalDir(), "plugins", name)
	if info, statErr := os.Stat(globalPluginPath); statErr == nil && info.IsDir() {
		return globalPluginPath, "global", nil
	}

	if name == "ddx" {
		return "", "baked-in", nil
	}

	return "", "", fmt.Errorf("plugin %q not found in project, global, or baked-in layers", name)
}
