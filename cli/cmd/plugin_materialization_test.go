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

func TestPluginInstall_HelixRegistryWritesLockNotProjectPayload(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	server := pluginMaterializationArchiveServerFor(t, "helix", registry.PackageTypeWorkflow, "helix")
	defer server.Close()

	factory := pluginMaterializationFactoryForPackage(workDir, server.URL, "helix", registry.PackageTypeWorkflow, "helix")
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "helix", "--force")
	require.NoError(t, err, output)

	assert.NoDirExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix"),
		"marketplace HELIX install must not copy payload into the project plugin tree")
	assert.FileExists(t, filepath.Join(workDir, ddxroot.DirName, registry.ProjectPluginLockFile))

	lock, err := registry.LoadProjectPluginLock(t.Context(), workDir)
	require.NoError(t, err)
	entry := lock.Find("helix")
	require.NotNil(t, entry)
	assert.Equal(t, "1.0.0", entry.Version)
	assert.FileExists(t, filepath.Join(entry.CachePath, "package.yaml"))

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "helix"),
		filepath.Join(".claude", "skills", "helix"),
	} {
		assertLocalSymlink(t, filepath.Join(workDir, rel), filepath.Join(entry.CachePath, "skills", "helix"))
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

func TestPluginSync_MaterializesBuiltinDDxAdaptersFromXDGCache(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	factory := NewCommandFactory(workDir)

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "sync", "--force")
	require.NoError(t, err, output)
	assert.Contains(t, output, "ddx builtin: ok")

	assert.NoDirExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx"),
		"plugin sync must not copy the baked-in ddx payload into the project plugin tree")
	builtin, err := registry.BuiltinRegistry().Find("ddx")
	require.NoError(t, err)
	cacheSkillDir := filepath.Join(registry.PluginCacheDir("ddx", builtin.Version), "skills", "ddx")
	assert.FileExists(t, filepath.Join(cacheSkillDir, "SKILL.md"))
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "ddx"),
		filepath.Join(".claude", "skills", "ddx"),
	} {
		path := filepath.Join(workDir, rel)
		assert.FileExists(t, filepath.Join(path, "SKILL.md"))
		assertLocalSymlink(t, path, cacheSkillDir)
	}
}

func TestPluginSync_RecreatesBuiltinDDxCacheFromEmbeddedFS(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	factory := NewCommandFactory(workDir)
	builtin, err := registry.BuiltinRegistry().Find("ddx")
	require.NoError(t, err)
	cachePath := registry.PluginCacheDir("ddx", builtin.Version)
	require.NoError(t, os.RemoveAll(cachePath))

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "sync")
	require.NoError(t, err, output)
	assert.Contains(t, output, "ddx builtin: ok")
	assert.FileExists(t, filepath.Join(cachePath, "skills", "ddx", "SKILL.md"))
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "ddx"), filepath.Join(cachePath, "skills", "ddx"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "ddx"), filepath.Join(cachePath, "skills", "ddx"))
	assert.NoDirExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx"),
		"offline built-in cache recreation must not create a project payload tree")
}

func TestPluginInstallDDxUsesBuiltinCacheWithoutProjectLock(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	factory := NewCommandFactory(workDir)
	builtin, err := registry.BuiltinRegistry().Find("ddx")
	require.NoError(t, err)

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "ddx", "--force")
	require.NoError(t, err, output)
	assert.Contains(t, output, "Installed ddx "+builtin.Version+" from built-in cache")

	assert.NoDirExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx"),
		"installing the built-in ddx package must not copy payloads into the project")
	assert.NoFileExists(t, filepath.Join(workDir, ddxroot.DirName, registry.ProjectPluginLockFile),
		"installing the built-in ddx package must not write a project plugin lock")

	cacheSkillDir := filepath.Join(registry.PluginCacheDir("ddx", builtin.Version), "skills", "ddx")
	assert.FileExists(t, filepath.Join(cacheSkillDir, "SKILL.md"))
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "ddx"), cacheSkillDir)
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "ddx"), cacheSkillDir)
}

func TestPluginShowDDxSurfacesBuiltinPackage(t *testing.T) {
	workDir, _ := setupPluginMaterializationProject(t)
	factory := NewCommandFactory(workDir)

	output, err := executeCommand(factory.NewRootCommand(), "plugin", "show", "ddx")
	require.NoError(t, err, output)
	assert.Contains(t, output, "Name:         ddx")
	assert.Contains(t, output, "Status:       built-in")
	assert.Contains(t, output, "Installed at: built-in")
	assert.NoFileExists(t, filepath.Join(workDir, ddxroot.DirName, registry.ProjectPluginLockFile),
		"showing the built-in ddx package must not write a project plugin lock")
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
	return pluginMaterializationFactoryForPackage(workDir, source, "sample-plugin", registry.PackageTypePlugin, "sample-skill")
}

func pluginMaterializationFactoryForPackage(workDir, source, name string, typ registry.PackageType, skillName string) *CommandFactory {
	factory := NewCommandFactory(workDir)
	factory.registryOverride = &registry.Registry{Packages: []registry.Package{{
		Name:        name,
		Version:     "1.0.0",
		Description: "Sample plugin",
		Type:        typ,
		Source:      source,
		Install: registry.PackageInstall{
			Root:   &registry.InstallMapping{Source: ".", Target: ddxroot.JoinRelative("plugins", name)},
			Skills: []registry.InstallMapping{{Source: "skills/", Target: ".agents/skills/"}, {Source: "skills/", Target: ".claude/skills/"}},
		},
		Keywords: []string{skillName},
	}}}
	return factory
}

func pluginMaterializationArchiveServer(t *testing.T) *httptest.Server {
	t.Helper()
	archive := pluginMaterializationTarball(t)
	return pluginMaterializationArchiveServerWithPayload(archive)
}

func pluginMaterializationArchiveServerFor(t *testing.T, pluginName string, typ registry.PackageType, skillName string) *httptest.Server {
	t.Helper()
	archive := pluginMaterializationTarballFor(t, pluginName, typ, skillName)
	return pluginMaterializationArchiveServerWithPayload(archive)
}

func pluginMaterializationArchiveServerWithPayload(archive []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
}

func pluginMaterializationTarball(t *testing.T) []byte {
	return pluginMaterializationTarballFor(t, "sample-plugin", registry.PackageTypePlugin, "sample-skill")
}

func pluginMaterializationTarballFor(t *testing.T, pluginName string, typ registry.PackageType, skillName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	topDir := pluginName + "-1.0.0"
	writePluginMaterializationTarDir(t, tw, topDir)
	writePluginMaterializationTarFile(t, tw, filepath.Join(topDir, "package.yaml"), []byte(`name: `+pluginName+`
version: 1.0.0
description: Sample plugin
type: `+string(typ)+`
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
	writePluginMaterializationTarDir(t, tw, filepath.Join(topDir, "skills"))
	writePluginMaterializationTarDir(t, tw, filepath.Join(topDir, "skills", skillName))
	writePluginMaterializationTarFile(t, tw, filepath.Join(topDir, "skills", skillName, "SKILL.md"), []byte("---\nname: "+skillName+"\ndescription: sample\n---\n\nSample skill body.\n"))
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
