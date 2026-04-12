package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPackageManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: sample-plugin
version: 1.2.3
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
  skills:
    - source: skills/
      target: .agents/skills/
keywords:
  - sample
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(manifest), 0o644))

	pkg, issues, err := LoadPackageManifest(dir)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.NotNil(t, pkg)

	assert.Equal(t, "sample-plugin", pkg.Name)
	assert.Equal(t, "1.2.3", pkg.Version)
	assert.Equal(t, "Sample plugin", pkg.Description)
	assert.Equal(t, PackageTypePlugin, pkg.Type)
	assert.Equal(t, "https://example.com/sample-plugin", pkg.Source)
	assert.Equal(t, SupportedPackageAPIVersion, pkg.APIVersion)
	require.NotNil(t, pkg.Install.Root)
	assert.Equal(t, ".", pkg.Install.Root.Source)
	assert.Equal(t, ".ddx/plugins/sample-plugin", pkg.Install.Root.Target)
	require.Len(t, pkg.Install.Skills, 1)
	assert.Equal(t, "skills/", pkg.Install.Skills[0].Source)
	assert.Equal(t, ".agents/skills/", pkg.Install.Skills[0].Target)
}

func TestLoadPackageManifestRejectsUnsupportedAPIVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: sample-plugin
version: 1.2.3
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 2
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(manifest), 0o644))

	pkg, issues, err := LoadPackageManifest(dir)
	require.Error(t, err)
	assert.Nil(t, pkg)
	require.NotEmpty(t, issues)
	assert.True(t, strings.Contains(err.Error(), "unsupported `api_version`"))
}

func TestLoadPackageManifestWithFallbackUsesFallbackWhenManifestMissing(t *testing.T) {
	dir := t.TempDir()
	fallback := &Package{
		Name:        "sample-plugin",
		Version:     "1.2.3",
		Description: "Sample plugin",
		Type:        PackageTypePlugin,
		Source:      "https://example.com/sample-plugin",
	}

	pkg, missing, issues, err := LoadPackageManifestWithFallback(dir, fallback)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	assert.True(t, missing)
	require.Empty(t, issues)
	assert.Same(t, fallback, pkg)
}
