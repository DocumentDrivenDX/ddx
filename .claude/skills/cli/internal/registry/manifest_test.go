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

func TestLoadPackageManifestReportsUnsupportedAPIVersion(t *testing.T) {
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
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.NotEmpty(t, issues)
	assert.True(t, strings.Contains(issues[0].Error(), "unsupported `api_version`"))
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

func TestLoadPackageManifestPreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: future-plugin
version: 1.0.0
description: Manifest that uses keys introduced after this DDx build
type: plugin
source: https://example.com/future-plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/future-plugin
# Unknown top-level keys a future api_version might introduce.
hooks:
  pre-install: scripts/pre.sh
  post-install: scripts/post.sh
signatures:
  release:
    algo: ed25519
    key: AAA...ZZZ
future_flag: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(manifest), 0o644))

	pkg, issues, err := LoadPackageManifest(dir)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.NotNil(t, pkg)

	// Known keys parse as before.
	assert.Equal(t, "future-plugin", pkg.Name)
	assert.Equal(t, PackageTypePlugin, pkg.Type)

	// Unknown keys survive on Extra, keyed by their original YAML name.
	require.NotNil(t, pkg.Extra)
	assert.Contains(t, pkg.Extra, "hooks")
	assert.Contains(t, pkg.Extra, "signatures")
	assert.Contains(t, pkg.Extra, "future_flag")
	assert.Equal(t, true, pkg.Extra["future_flag"])
	if hooks, ok := pkg.Extra["hooks"].(map[string]any); ok {
		assert.Equal(t, "scripts/pre.sh", hooks["pre-install"])
		assert.Equal(t, "scripts/post.sh", hooks["post-install"])
	} else {
		t.Fatalf("expected hooks to be map[string]any, got %T", pkg.Extra["hooks"])
	}

	// Round-trip: MarshalPackage emits a document that contains the unknown
	// keys. Re-loading reproduces them.
	out, err := MarshalPackage(pkg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), out, 0o644))

	reloaded, issues, err := LoadPackageManifest(dir)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.NotNil(t, reloaded)
	require.NotNil(t, reloaded.Extra)
	assert.Contains(t, reloaded.Extra, "hooks")
	assert.Contains(t, reloaded.Extra, "signatures")
	assert.Contains(t, reloaded.Extra, "future_flag")
}

func TestLoadPackageManifestNoUnknownKeysYieldsNilExtra(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: plain-plugin
version: 1.0.0
description: No unknown top-level keys
type: plugin
source: https://example.com/plain
api_version: 1
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(manifest), 0o644))
	pkg, issues, err := LoadPackageManifest(dir)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.NotNil(t, pkg)
	assert.Nil(t, pkg.Extra, "no unknown keys should leave Extra nil")
}

func TestMarshalPackageExtraNeverOverwritesTypedFields(t *testing.T) {
	pkg := &Package{
		Name:        "guard",
		Version:     "1.0.0",
		Description: "Typed fields must win",
		Type:        PackageTypePlugin,
		Source:      "https://example.com/guard",
		APIVersion:  "1",
		Extra: map[string]any{
			// Defensive: even if a caller stashed a known key in Extra,
			// MarshalPackage must not let it overwrite the typed field.
			"name":       "shadow",
			"future_key": "kept",
		},
	}
	out, err := MarshalPackage(pkg)
	require.NoError(t, err)
	assert.Contains(t, string(out), "name: guard")
	assert.Contains(t, string(out), "future_key: kept")
	assert.NotContains(t, string(out), "name: shadow")
}
