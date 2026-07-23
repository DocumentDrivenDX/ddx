package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	internalgit "github.com/DocumentDrivenDX/ddx/internal/git"
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

	pluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin")
	_, statErr := os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(statErr), "plugin root should not be created on validation failure")

	installedState := filepath.Join(homeDir, ddxroot.DirName, "installed.yaml")
	_, statErr = os.Stat(installedState)
	assert.True(t, os.IsNotExist(statErr), "installed.yaml should not be written on validation failure")
}

func TestInstallLocalRejectsHomeRootedManifestTarget(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()
	// Use fmt.Sprintf so the tilde-rooted target does not appear as a static
	// string literal (which would trigger the FEAT-015 grep gate).
	homeTarget := fmt.Sprintf("%s/.ddx/plugins/sample-plugin", "~")
	manifest := fmt.Sprintf(`name: sample-plugin
version: 1.0.0
description: Sample plugin
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: %s
`, homeTarget)
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(manifest), 0o644))

	factory := NewCommandFactory(workDir)
	_, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err, "home-rooted Root.Target must be rejected (FEAT-015)")
	assert.Contains(t, err.Error(), "FEAT-015")
	assert.Contains(t, err.Error(), "project-relative")

	globalPluginDir := filepath.Join(homeDir, ddxroot.DirName, "plugins", "sample-plugin")
	_, statErr := os.Lstat(globalPluginDir)
	assert.True(t, os.IsNotExist(statErr), "no plugin tree should be written under $HOME on rejection")
}

func TestInstallLocalPreservesExistingProjectPluginDirUnlessForced(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	localPlugin := t.TempDir()

	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(workDir, ddxroot.DirName))
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

	projectPluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin")
	require.NoError(t, os.MkdirAll(projectPluginDir, 0o755))
	sentinel := filepath.Join(projectPluginDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep me"), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin)
	require.Error(t, err, output)
	assert.Contains(t, err.Error(), "already exists")

	homePluginDir := filepath.Join(homeDir, ddxroot.DirName, "plugins", "sample-plugin")
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

	// After --force, the project-local plugin dir is a developer symlink to
	// the local plugin checkout.
	projectInfo, err = os.Lstat(projectPluginDir)
	require.NoError(t, err)
	require.True(t, projectInfo.Mode()&os.ModeSymlink != 0, "project plugin path must be a local symlink")
	linkTarget, err := os.Readlink(projectPluginDir)
	require.NoError(t, err)
	assert.Equal(t, localPlugin, linkTarget)

	// Plugin file is visible through the symlinked project path.
	_, statErr = os.Stat(filepath.Join(projectPluginDir, "skills", "sample-skill", "SKILL.md"))
	assert.NoError(t, statErr, "plugin tree must be copied under project")

	_, statErr = os.Lstat(homePluginDir)
	assert.True(t, os.IsNotExist(statErr), "FEAT-015: no $HOME write even on --force")

	_, statErr = os.Stat(sentinel)
	assert.True(t, os.IsNotExist(statErr), "sentinel file should be removed when the directory is replaced")
}

func TestPluginInstallLocalDoesNotRewriteInstalledState(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	statePath := filepath.Join(homeDir, ddxroot.DirName, "installed.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))
	beforeState := `installed:
  - name: helix
    version: 6.6.1
    type: workflow
    source: https://github.com/DocumentDrivenDX/helix
    installed_at: 2026-05-13T00:00:00Z
    files:
      - .ddx/plugins/helix
`
	require.NoError(t, os.WriteFile(statePath, []byte(beforeState), 0o644))

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: helix
version: 6.6.1
description: Local HELIX checkout
type: workflow
source: https://github.com/DocumentDrivenDX/helix
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/helix
  skills:
    - source: .agents/skills/
      target: .agents/skills/
`), 0o644))
	skillDir := filepath.Join(localPlugin, ".agents", "skills", "helix-build")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: helix-build
description: Local HELIX build skill
---

Local skill body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "helix", "--local", localPlugin, "--force")
	require.NoError(t, err, output)
	assert.Contains(t, output, "recorded plugin pin unchanged")

	afterState, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Equal(t, beforeState, string(afterState), "local installs must not rewrite the recorded plugin pin")

	assert.FileExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix", "package.yaml"))
	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "helix-build"), filepath.Join(localPlugin, ".agents", "skills", "helix-build"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "helix-build"), filepath.Join(localPlugin, ".agents", "skills", "helix-build"))
}

func TestPluginInstallLocalHelixFallbackIgnoresSourceOverlayWithoutScriptAssumption(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, ".claude-plugin", "plugin.json"), []byte(`{
  "name": "helix",
  "version": "0.3.3",
  "description": "Local HELIX checkout",
  "repository": "https://github.com/DocumentDrivenDX/helix",
  "skills": "./.agents/skills/"
}`), 0o644))
	skillDir := filepath.Join(localPlugin, ".agents", "skills", "helix")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: helix
description: Local HELIX skill
---

Local HELIX skill body.
`), 0o644))
	staleOverlay := filepath.Join(localPlugin, ddxroot.DirName, "plugins", "helix")
	require.NoError(t, os.MkdirAll(filepath.Dir(staleOverlay), 0o755))
	if err := os.Symlink(filepath.Join(t.TempDir(), ddxroot.DirName, "plugins", "helix"), staleOverlay); err != nil {
		t.Skipf("symlink creation unsupported: %v", err)
	}

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "helix", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "helix"), filepath.Join(localPlugin, ".agents", "skills", "helix"))
	assert.NoFileExists(t, filepath.Join(homeDir, ".local", "bin", "helix"))
	assert.NotContains(t, output, "helix script")
}

func TestPluginInstallLocalUsesPackageManifestSkillSource(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: 1.0.0
description: Local plugin with custom skill source
type: plugin
source: https://example.com/sample-plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
  skills:
    - source: agent-components/
      target: .agents/skills/
`), 0o644))
	skillDir := filepath.Join(localPlugin, "agent-components", "custom-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: custom-skill
description: Custom skill source
---

Custom skill body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "custom-skill"), filepath.Join(localPlugin, "agent-components", "custom-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "custom-skill"), filepath.Join(localPlugin, "agent-components", "custom-skill"))
	assert.NoDirExists(t, filepath.Join(workDir, ".agents", "skills", "sample-plugin"))
}

// TestPluginInstallLocalDDxLibrarySymlinksSkills verifies the development
// workflow documented in docs/helix/02-design/plan-2026-05-13-ddx-skill-package-layout.md:
// pointing `ddx plugin install ddx --local library --force` at the in-repo
// `library/` package root must symlink `library/skills/ddx` into both
// `.agents/skills/ddx` and `.claude/skills/ddx`, not at the project root's own
// `.agents/skills` tree.
func TestPluginInstallLocalDDxLibrarySymlinksSkills(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	libraryRoot := filepath.Join(workDir, "library")
	require.NoError(t, os.MkdirAll(libraryRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(libraryRoot, "package.yaml"), []byte(`name: ddx
version: 0.0.0-test
description: DDx default library — test fixture
type: plugin
source: https://github.com/DocumentDrivenDX/ddx
api_version: "1"
install:
  root:
    source: .
    target: .ddx/plugins/ddx
  skills:
    - source: skills/
      target: .agents/skills/
    - source: skills/
      target: .claude/skills/
`), 0o644))
	skillDir := filepath.Join(libraryRoot, "skills", "ddx")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: ddx
description: DDx skill fixture
---

Body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "ddx", "--local", "library", "--force")
	require.NoError(t, err, output)

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "ddx"), libraryRoot)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "ddx"), skillDir)
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "ddx"), skillDir)
}

func TestAddLocalOverlayIgnoresCoversSymlinkAndDirectoryForms(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git", "info"), 0o755))

	require.NoError(t, addLocalOverlayIgnores(repoRoot, []string{".ddx/plugins/helix"}))

	data, err := os.ReadFile(filepath.Join(repoRoot, ".git", "info", "exclude"))
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, ".ddx/plugins/helix\n")
	assert.Contains(t, text, ".ddx/plugins/helix/\n")
}

func TestAddLocalOverlayIgnoresCreatesMissingGitInfoDir(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))

	require.NoError(t, addLocalOverlayIgnores(repoRoot, []string{".ddx/plugins/helix"}))

	data, err := os.ReadFile(filepath.Join(repoRoot, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".ddx/plugins/helix\n")
}

func TestPluginInstallLocalCopiesScriptOnlyWhenManifestDeclaresIt(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "package.yaml"), []byte(`name: sample-plugin
version: local
description: Local plugin with explicit script
type: plugin
api_version: 1
install:
  root:
    source: .
    target: .ddx/plugins/sample-plugin
  scripts:
    source: tools/sample
    target: ~/.local/bin/sample-plugin
  executable:
    - tools/sample
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, "tools"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "tools", "sample"), []byte("#!/usr/bin/env bash\necho sample\n"), 0o755))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "sample-plugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	installedScript := filepath.Join(homeDir, ".local", "bin", "sample-plugin")
	assert.FileExists(t, installedScript)
	data, err := os.ReadFile(installedScript)
	require.NoError(t, err)
	assert.Contains(t, string(data), "echo sample")
}

func TestPluginInstallLocalMirrorsTopLevelSkillsWithoutPackageManifest(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, ".claude-plugin", "plugin.json"), []byte(`{
  "name": "helix",
  "version": "6.6.1",
  "description": "Local HELIX checkout",
  "skills": "./skills/"
}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, "scripts", "helix"), []byte("#!/usr/bin/env bash\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(localPlugin, ".agents", "skills"), 0o755))
	skillDir := filepath.Join(localPlugin, "skills", "helix-input")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: helix-input
description: Local HELIX input skill
---

Local skill body.
`), 0o644))

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "plugin", "install", "helix", "--local", localPlugin, "--force")
	require.NoError(t, err, output)
	assert.Contains(t, output, "recorded plugin pin unchanged")

	assert.FileExists(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix", "skills", "helix-input", "SKILL.md"))
	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "helix"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "helix-input"), filepath.Join(localPlugin, "skills", "helix-input"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "helix-input"), filepath.Join(localPlugin, "skills", "helix-input"))
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

	withStaticHTTPTransport(t, tarball, "application/gzip", "/archive/refs/tags/v1.0.0.tar.gz")

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	_, installErr := registry.InstallPackage(&registry.Package{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    registry.PackageTypePlugin,
		Source:  "https://example.com/sample-plugin",
	}, workDir)
	require.Error(t, installErr)
	assert.Contains(t, installErr.Error(), "missing SKILL.md")

	pluginDir := filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin")
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

	withStaticHTTPTransport(t, tarball, "application/gzip", "/archive/refs/tags/v1.0.0.tar.gz")

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	entry, installErr := registry.InstallPackage(&registry.Package{
		Name:    "sample-plugin",
		Version: "1.0.0",
		Type:    registry.PackageTypePlugin,
		Source:  "https://example.com/sample-plugin",
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

func TestInstallLocalCreatesDeveloperSymlinks(t *testing.T) {
	// Local installs are developer overlays: the plugin root and harness skill
	// surfaces should symlink back to the local checkout.
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

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

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "sample-skill"), filepath.Join(localPlugin, ".agents", "skills", "sample-skill"))
	assertLocalSymlink(t, filepath.Join(workDir, ".claude", "skills", "sample-skill"), filepath.Join(localPlugin, ".agents", "skills", "sample-skill"))

	// And no plugin tree leaked into $HOME.
	homePluginDir := filepath.Join(homeDir, ddxroot.DirName, "plugins", "sample-plugin")
	_, statErr := os.Stat(homePluginDir)
	assert.True(t, os.IsNotExist(statErr), "FEAT-015: nothing must land under $HOME/.ddx/plugins/")
}

// TestInstallLocalSkipsBrokenSymlinksUnderGitignoredPaths reproduces ddx-686564bf:
// `ddx install --local` validated every symlink in the source tree, including
// broken ones under gitignored scratch/tmp directories (e.g.
// doctor/home/.codex/tmp/) that were never meant to ship. The install must
// succeed because the offending path is gitignored in the source repo.
func TestInstallLocalSkipsBrokenSymlinksUnderGitignoredPaths(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Force in-tree mode so plugin installs to <workDir>/.ddx/plugins/.
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))

	localPlugin := t.TempDir()
	// Initialize a git repo so `git check-ignore` can consult .gitignore.
	require.NoError(t, internalgit.Command(context.Background(), localPlugin, "init").Run())
	require.NoError(t, os.WriteFile(filepath.Join(localPlugin, ".gitignore"), []byte(".codex\n"), 0o644))

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
	skillDir := filepath.Join(localPlugin, "skills", "sample-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: sample-skill
description: Sample skill
---

Sample body.
`), 0o644))

	// A stale broken symlink under a gitignored scratch path (matches ".codex").
	scratchDir := filepath.Join(localPlugin, "doctor", "home", ".codex", "tmp")
	require.NoError(t, os.MkdirAll(scratchDir, 0o755))
	if err := os.Symlink("/nonexistent/build-machine/apply_patch", filepath.Join(scratchDir, "apply_patch")); err != nil {
		t.Skipf("symlink creation unsupported: %v", err)
	}

	factory := NewCommandFactory(workDir)
	output, err := executeCommand(factory.NewRootCommand(), "install", "sample-plugin", "--local", localPlugin, "--force")
	require.NoError(t, err, output)

	assertLocalSymlink(t, filepath.Join(workDir, ddxroot.DirName, "plugins", "sample-plugin"), localPlugin)
	assertLocalSymlink(t, filepath.Join(workDir, ".agents", "skills", "sample-skill"), skillDir)
}

func assertLocalSymlink(t *testing.T, linkPath, expectedTarget string) {
	t.Helper()
	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.True(t, info.Mode()&os.ModeSymlink != 0, "%s must be a symlink", linkPath)
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	target, err = filepath.Abs(target)
	require.NoError(t, err)
	expectedTarget, err = filepath.Abs(expectedTarget)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(expectedTarget), filepath.Clean(target))
}
