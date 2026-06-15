package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginInstall_RegistryWritesLockNotProjectPayload(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	server := pluginMaterializationArchiveServer(t)
	defer server.Close()

	factory := pluginMaterializationFactory(workDir, server.URL)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--force")
	require.NoError(t, err, output)

	assert.NoDirExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin"),
		"registry install must not copy payload into the project plugin tree")
	assert.FileExists(t, filepath.Join(workDir, ddxroot.DirName, registry.ProjectPluginLockFile))

	lock, err := registry.LoadProjectPluginLock(t.Context(), workDir)
	require.NoError(t, err)
	entry := lock.Find("sample-plugin")
	require.NotNil(t, entry)
	assert.Equal(t, "1.0.0", entry.Version)
	assert.FileExists(t, filepath.Join(entry.CachePath, "package.yaml"))
	assert.NoFileExists(t, filepath.Join(os.Getenv("HOME"), ddxroot.DirName, "installed.yaml"))

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "sample-skill"),
		filepath.Join(".claude", "skills", "sample-skill"),
	} {
		assertLocalSymlink(t, filepath.Join(workDir, rel), filepath.Join(entry.CachePath, "skills", "sample-skill"))
	}
}

func TestPluginSync_MaterializesAgentSkillShimsFromCache(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	server := pluginMaterializationArchiveServer(t)
	defer server.Close()

	factory := pluginMaterializationFactory(workDir, server.URL)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--force")
	require.NoError(t, err, output)
	require.NoError(t, os.RemoveAll(filepath.Join(workDir, ".agents", "skills", "sample-skill")))
	require.NoError(t, os.RemoveAll(filepath.Join(workDir, ".claude", "skills", "sample-skill")))

	output, err = executeCommand(factory.NewRootCommand(), "plugin", "sync")
	require.NoError(t, err, output)
	assert.Contains(t, output, "sample-plugin 1.0.0: ok")

	lock, err := registry.LoadProjectPluginLock(t.Context(), workDir)
	require.NoError(t, err)
	entry := lock.Find("sample-plugin")
	require.NotNil(t, entry)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "sample-skill"), filepath.Join(entry.CachePath, "skills", "sample-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "sample-skill"), filepath.Join(entry.CachePath, "skills", "sample-skill"))
}

func TestPluginConsumer_LazilySyncsMissingShims(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	server := pluginMaterializationArchiveServer(t)
	defer server.Close()

	factory := pluginMaterializationFactory(workDir, server.URL)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--force")
	require.NoError(t, err, output)
	require.NoError(t, os.RemoveAll(filepath.Join(workDir, ".agents", "skills", "sample-skill")))
	require.NoError(t, os.RemoveAll(filepath.Join(workDir, ".claude", "skills", "sample-skill")))

	output, err = executeCommand(factory.NewRootCommand(), "plugin", "show", "sample-plugin")
	require.NoError(t, err, output)
	assert.Contains(t, output, "Name:         sample-plugin")

	lock, err := registry.LoadProjectPluginLock(t.Context(), workDir)
	require.NoError(t, err)
	entry := lock.Find("sample-plugin")
	require.NotNil(t, entry)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "sample-skill"), filepath.Join(entry.CachePath, "skills", "sample-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "sample-skill"), filepath.Join(entry.CachePath, "skills", "sample-skill"))
}

func TestPluginDoctor_ReportsLockCacheAndShimState(t *testing.T) {
	workDir, xdgDir := setupPluginMaterializationProject(t)
	cacheDir := filepath.Join(xdgDir, "ddx", "cache", "plugins", "shim-plugin", "1.0.0")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	require.NoError(t, registry.SaveProjectPluginLock(t.Context(), workDir, &registry.PluginLock{
		Plugins: []registry.PluginLockEntry{
			{Name: "missing-cache", Version: "1.0.0", Type: registry.PackageTypePlugin, Source: "https://example.com/missing", CachePath: filepath.Join(xdgDir, "missing"), GeneratedFiles: []string{filepath.Join(".agents", "skills", "missing-cache")}},
			{Name: "shim-plugin", Version: "1.0.0", Type: registry.PackageTypePlugin, Source: "https://example.com/shim", CachePath: cacheDir, GeneratedFiles: []string{filepath.Join(".agents", "skills", "shim-plugin")}},
			{Name: "local-plugin", Version: "1.0.0", Type: registry.PackageTypePlugin, Source: "https://example.com/local", CachePath: filepath.Join(xdgDir, "missing-local")},
		},
	}))
	localCheckout := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName, "plugins"), 0o755))
	require.NoError(t, os.Symlink(localCheckout, filepath.Join(workDir, ddxroot.DirName, "plugins", "local-plugin")))

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--plugins")
	require.NoError(t, err, output)

	assert.Contains(t, output, "lock present")
	assert.Contains(t, output, "cache missing")
	assert.Contains(t, output, "shims missing")
	assert.Contains(t, output, "local overlay active")
}

func TestPluginInstallLocal_PreservesCheckoutOverlay(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	lockPath := filepath.Join(workDir, ddxroot.DirName, registry.ProjectPluginLockFile)
	beforeLock := `plugins:
  - name: sample-plugin
    version: 1.0.0
    type: plugin
    source: https://example.com/sample-plugin
    cache_path: /tmp/ddx-cache/sample-plugin/1.0.0
    installed_at: 2026-06-15T00:00:00Z
`
	require.NoError(t, os.WriteFile(lockPath, []byte(beforeLock), 0o644))

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Local sample checkout
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
`), 0o644))
	skillDir := filepath.Join(localPlugin, "skills", "sample-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: sample-skill\ndescription: local\n---\n\nLocal sample skill body.\n"), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	afterLock, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, beforeLock, string(afterLock))
	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "sample-skill"), filepath.Join(localPlugin, "skills", "sample-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "sample-skill"), filepath.Join(localPlugin, "skills", "sample-skill"))
}

func setupPluginMaterializationProject(t *testing.T) (workDir, xdgDir string) {
	t.Helper()
	workDir = t.TempDir()
	xdgDir = t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("HOME", homeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))
	return workDir, xdgDir
}

func pluginMaterializationFactory(workDir, source string) *CommandFactory {
	factory := NewCommandFactory(workDir)
	factory.registryOverride = &registry.Registry{Packages: []registry.Package{{
		Name:        "sample-plugin",
		Version:     "1.0.0",
		Description: "Sample plugin",
		Type:        registry.PackageTypePlugin,
		Source:      source,
		Install: registry.PackageInstall{
			Root:   &registry.InstallMapping{Source: ".", Target: ".ddx/plugins/sample-plugin"},
			Skills: []registry.InstallMapping{{Source: "skills/", Target: ".agents/skills/"}, {Source: "skills/", Target: ".claude/skills/"}},
		},
	}}}
	return factory
}

func pluginMaterializationArchiveServer(t *testing.T) *httptest.Server {
	t.Helper()
	archive := pluginMaterializationTarball(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
}

func pluginMaterializationTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writePluginMaterializationTarDir(t, tw, "sample-plugin-1.0.0")
	writePluginMaterializationTarFile(t, tw, filepath.Join("sample-plugin-1.0.0", "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
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
    - source: skills/
      target: .claude/skills/
`))
	writePluginMaterializationTarDir(t, tw, filepath.Join("sample-plugin-1.0.0", "skills"))
	writePluginMaterializationTarDir(t, tw, filepath.Join("sample-plugin-1.0.0", "skills", "sample-skill"))
	writePluginMaterializationTarFile(t, tw, filepath.Join("sample-plugin-1.0.0", "skills", "sample-skill", "SKILL.md"), []byte("---\nname: sample-skill\ndescription: sample\n---\n\nSample skill body.\n"))
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func writePluginMaterializationTarDir(t *testing.T, tw *tar.Writer, name string) {
	t.Helper()
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name) + "/", Typeflag: tar.TypeDir, Mode: 0o755}))
}

func writePluginMaterializationTarFile(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))}))
	_, err := tw.Write(data)
	require.NoError(t, err)
}
