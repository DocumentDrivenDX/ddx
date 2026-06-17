package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/registry/defaultplugin"
)

func BuiltinDDxCachePath() (string, error) {
	pkg, err := BuiltinRegistry().Find("ddx")
	if err != nil {
		return "", err
	}
	return PluginCacheDir(pkg.Name, pkg.Version), nil
}

func EnsureBuiltinDDxCache(cachePath string, force bool) error {
	if !force && BuiltinDDxCacheReady(cachePath) {
		return nil
	}
	if err := os.RemoveAll(cachePath); err != nil {
		return fmt.Errorf("clear baked-in ddx cache %s: %w", cachePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("create baked-in ddx cache root: %w", err)
	}
	if err := MaterializeFS(defaultplugin.FS(), cachePath); err != nil {
		return fmt.Errorf("materialize baked-in ddx cache: %w", err)
	}
	return nil
}

func BuiltinDDxCacheReady(cachePath string) bool {
	for _, rel := range []string{
		"package.yaml",
		filepath.Join("skills", "ddx", "SKILL.md"),
	} {
		info, err := os.Stat(filepath.Join(cachePath, rel))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}
