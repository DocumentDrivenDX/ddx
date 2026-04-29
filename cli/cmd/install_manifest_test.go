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

func TestInstallLocalRejectsHomeRootedManifestTarget(t *testing.T) {
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
`), 0o644))

	factory := NewCommandFactory(workDir)
	_, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err, "home-rooted Root.Target must be rejected (FEAT-015)")
	assert.Contains(t, err.Error(), "FEAT-015")
	assert.Contains(t, err.Error(), "project-relative")

	globalPluginDir := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Lstat(globalPluginDir)
	assert.True(t, os.IsNotExist(statErr), "no plugin tree should be written under $HOME on rejection")
}

func TestInstallLocalPreservesExistingProjectPluginDirUnlessForced(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()

	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(workDir, ".ddx"))
		_ = os.RemoveAll(filepath.Join(workDir, ".agents"))
		_ = os.RemoveAll(filepath.Join(workDir, ".claude"))
	})
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
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, "skills", "sample-skill"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "skills", "sample-skill", "SKILL.md"), []byte(`---
name: sample-skill
description: Sample skill
---

Sample skill body.
`), 0o644))

	projectPluginDir := filepath.Join(workDir, ".ddx", "plugins", "sample-plugin")
	require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
	sentinel := filepath.Join(projectPluginDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep me"), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "already exists")

	homePluginDir := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Lstat(homePluginDir)
	assert.True(t, os.IsNotExist(statErr), "no plugin tree should ever be created under $HOME (FEAT-015)")

	projectInfo, err := os.Lstat(projectPluginDir)
	require.NoError(t, err)
	assert.False(t, projectInfo.Mode()&os.ModeSymlink != 0, "existing project plugin directory should remain a real directory")

	sentinelBytes, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(sentinelBytes))

	forceOut, forceErr := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin, "--force")
	require.NoError(t, forceErr, forceOut)

	// After --force, the project-local plugin dir is a real directory
	// containing the copied plugin tree, not a symlink to anywhere.
	projectInfo, err = os.Lstat(projectPluginDir)
	require.NoError(t, err)
	assert.False(t, projectInfo.Mode()&os.ModeSymlink != 0, "project plugin path must be a real directory (FEAT-015 no-symlinks)")
	assert.True(t, projectInfo.IsDir(), "project plugin path must be a real directory")

	// Plugin file from the source landed inside the project copy.
	_, statErr = os.Stat(filepath.Join(projectPluginDir, "skills", "sample-skill", "SKILL.md"))
	assert.NoError(t, statErr, "plugin tree must be copied under project")

	_, statErr = os.Lstat(homePluginDir)
	assert.True(t, os.IsNotExist(statErr), "FEAT-015: no $HOME write even on --force")

	_, statErr = os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr), "sentinel file should be removed when the directory is replaced")
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
	}, workDir)
	require.Error(t, installErr)
	assert.Contains(t, installErr.Error(), "missing SKILL.md")

	pluginDir := filepath.Join(workDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(statErr), "plugin root should not be created on validation failure")
}

func TestInstallPackageAllowsRecoverableBrokenSkillSymlinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tarball := mustBuildInstallTarballWithRecoverableSkillLink(t, "sample-plugin-1.0.0", `name: sample-plugin
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
    - source: .agents/skills/
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

	entry, installErr := registry.InstallPackage(&registry.Package{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    registry.PackageTypePlugin,
		Source:  server.URL,
	}, workDir)
	require.NoError(t, installErr)
	require.NotEmpty(t, entry.Files)

	_, statErr := os.Stat(filepath.Join(workDir, ".agents", "skills", "helix-align", "SKILL.md"))
	assert.NoError(t, statErr, "recoverable tarball skill link should install successfully")
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

func mustBuildInstallTarballWithRecoverableSkillLink(t *testing.T, rootName string, manifest string) []byte {
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
	writeSymlink := func(name, target string) {
		t.Helper()
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o777,
			Typeflag: tar.TypeSymlink,
			Linkname: target,
		}))
	}

	writeDir(rootName)
	writeFile(filepath.Join(rootName, "package.yaml"), manifest)
	writeDir(filepath.Join(rootName, "skills"))
	writeDir(filepath.Join(rootName, "skills", "helix-align"))
	writeFile(filepath.Join(rootName, "skills", "helix-align", "SKILL.md"), `---
name: helix-align
description: test skill
---

Test body.
`)
	writeDir(filepath.Join(rootName, ".agents"))
	writeDir(filepath.Join(rootName, ".agents", "skills"))
	writeSymlink(filepath.Join(rootName, ".agents", "skills", "helix-align"), "/nonexistent/build-machine/skills/helix-align")

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestInstall_GlobalFlagRemoved(t *testing.T) {
	// FEAT-015: --global was removed. Cobra rejects the unknown flag.
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "--global")
	require.Error(t, err, "ddx install --global must fail because the flag was removed")
	combined := output + err.Error()
	assert.True(t,
		strings.Contains(combined, "unknown flag") ||
			strings.Contains(combined, "global"),
		"error must reference the unknown --global flag, got: %s", combined)
}

func TestInstall_NoSymlinksCreated(t *testing.T) {
	// End-to-end FEAT-015 invariant: after a project-local install, no
	// symlinks exist under .agents/skills/, .claude/skills/, or
	// .ddx/plugins/<plugin>/.
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
    - source: .agents/skills/
      target: .agents/skills/
`), 0o644))
	skillDir := filepath.Join(localPlugin, ".agents", "skills", "sample-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: sample-skill
description: Sample skill
---

Sample body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.NoError(t, err, output)

	for _, sub := range []string{".ddx/plugins/sample-plugin", ".agents/skills", ".claude/skills"} {
		walkRoot := filepath.Join(workDir, filepath.FromSlash(sub))
		require.NoError(t, filepath.Walk(walkRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Errorf("FEAT-015: unexpected symlink at %s", path)
			}
			return nil
		}))
	}

	// And no plugin tree leaked into $HOME.
	homePluginDir := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	_, statErr := os.Stat(homePluginDir)
	assert.True(t, os.IsNotExist(statErr), "FEAT-015: nothing must land under $HOME/.ddx/plugins/")
}
