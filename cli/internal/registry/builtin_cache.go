package registry

import (
	"bytes"
	"fmt"
	"io/fs"
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
	expected := map[string]bool{}
	if err := fs.WalkDir(defaultplugin.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		expected[rel] = true
		embedded, err := fs.ReadFile(defaultplugin.FS(), rel)
		if err != nil {
			return err
		}
		cached, err := os.ReadFile(filepath.Join(cachePath, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if !bytes.Equal(embedded, cached) {
			return fmt.Errorf("stale cached file %s", rel)
		}
		return nil
	}); err != nil {
		return false
	}

	if len(expected) == 0 {
		return false
	}

	if err := filepath.WalkDir(cachePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(cachePath, path)
		if err != nil {
			return err
		}
		if !expected[filepath.ToSlash(rel)] {
			return fmt.Errorf("stale extra cached file %s", rel)
		}
		return nil
	}); err != nil {
		return false
	}
	return true
}
