package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallLocalRejectsUnsupportedAPIVersion(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 2
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err)
	assert.True(t, strings.Contains(output, "validating package manifest") || strings.Contains(err.Error(), "api_version"))
}

func TestInstallLocalRejectsMissingSkillMetadata(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
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
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, "skills", "bad-skill"), 0o755))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing SKILL.md")
	assert.False(t, strings.Contains(output, "Installed sample-plugin"), "install should stop before writing state")

	pluginDir := filepath.Join(workDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(statErr), "plugin root should not be created on validation failure")

	installedState := filepath.Join(homeDir, ".ddx", "installed.yaml")
	_, statErr = os.Stat(installedState)
	assert.True(t, os.IsNotExist(statErr), "installed.yaml should not be written on validation failure")
}

func TestInstallLocalCreatesProjectPluginSymlinkForGlobalRoot(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: ~/.ddx/plugins/sample-plugin
  skills:
    - source: skills/
      target: .agents/skills/
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, "skills", "sample-skill"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "skills", "sample-skill", "SKILL.md"), []byte(`---
name: sample-skill
description: Sample skill
---

Sample skill body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.NoError(t, err, output)

	globalPluginDir := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	projectPluginDir := filepath.Join(workDir, ".ddx", "plugins", "sample-plugin")

	globalInfo, err := os.Lstat(globalPluginDir)
	require.NoError(t, err)
	assert.True(t, globalInfo.Mode()&os.ModeSymlink != 0, "global plugin root should be a symlink")

	projectInfo, err := os.Lstat(projectPluginDir)
	require.NoError(t, err)
	assert.True(t, projectInfo.Mode()&os.ModeSymlink != 0, "project plugin path should be a symlink")

	linkTarget, err := os.Readlink(projectPluginDir)
	require.NoError(t, err)
	assert.Equal(t, globalPluginDir, linkTarget, "project plugin symlink should resolve to the global plugin root")
}

func TestInstallPackageRejectsMissingSkillMetadata(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tarball := mustBuildInstallTarball(t, "sample-plugin-1.0.0", `name: sample-plugin
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
`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	_, installErr := registry.InstallPackage(&registry.Package{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    registry.PackageTypePlugin,
		Source:  server.URL,
	})
	require.Error(t, installErr)
	assert.Contains(t, installErr.Error(), "missing SKILL.md")

	pluginDir := filepath.Join(workDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(statErr), "plugin root should not be created on validation failure")
}

func mustBuildInstallTarball(t *testing.T, rootName string, manifest string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	writeDir := func(name string) {
		t.Helper()
		if !strings.HasSuffix(name, "/") {
			name += "/"
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Typeflag: tar.TypeDir,
		}))
	}
	writeFile := func(name, body string) {
		t.Helper()
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}

	writeDir(rootName)
	writeFile(filepath.Join(rootName, "package.yaml"), manifest)
	writeDir(filepath.Join(rootName, "skills"))
	writeDir(filepath.Join(rootName, "skills", "bad-skill"))

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}
