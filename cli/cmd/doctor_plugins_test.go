package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorPluginsFlagReportsMissingManifest(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	pluginRoot := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	require.NoError(t, os.MkdirAll(pluginRoot, 0o755))

	state := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:    "sample-plugin",
				Version: "1.0.0",
				Type:    registry.PackageTypePlugin,
				Source:  pluginRoot,
				Files:   []string{pluginRoot},
			},
		},
	}
	require.NoError(t, registry.SaveState(state))

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--plugins")
	require.NoError(t, err)
	assert.Contains(t, output, "missing package.yaml")
}

func TestDoctorPluginsFlagAuditsLegacyUntypedPluginEntries(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	pluginRoot := filepath.Join(homeDir, ".ddx", "plugins", "legacy-plugin")
	require.NoError(t, os.MkdirAll(pluginRoot, 0o755))

	state := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:    "legacy-plugin",
				Version: "1.0.0",
				Source:  pluginRoot,
				Files:   []string{pluginRoot},
			},
		},
	}
	require.NoError(t, registry.SaveState(state))

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--plugins")
	require.NoError(t, err)
	assert.Contains(t, output, "missing package.yaml")
}

func TestDoctorPluginsFlagSkipsResourceEntries(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	pluginRoot := filepath.Join(homeDir, ".ddx", "plugins", "sample-plugin")
	require.NoError(t, os.MkdirAll(pluginRoot, 0o755))

	resourceFile := filepath.Join(homeDir, ".ddx", "plugins", "ddx", "personas", "example.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(resourceFile), 0o755))
	require.NoError(t, os.WriteFile(resourceFile, []byte("# Example resource\n"), 0o644))

	state := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:    "sample-plugin",
				Version: "1.0.0",
				Type:    registry.PackageTypePlugin,
				Source:  pluginRoot,
				Files:   []string{pluginRoot},
			},
			{
				Name:    "persona/example",
				Version: "latest",
				Type:    registry.PackageTypeResource,
				Source:  "https://github.com/DocumentDrivenDX/ddx-library",
				Files:   []string{resourceFile},
			},
		},
	}
	require.NoError(t, registry.SaveState(state))

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--plugins")
	require.NoError(t, err)

	assert.Contains(t, output, "missing package.yaml")
	assert.NotContains(t, output, filepath.Join(resourceFile, "package.yaml"))
	assert.NotContains(t, output, "not a directory")
}

// TestDoctorPluginsFlagReportsBothManifestSchemaAndStructuralIssues covers
// ddx-b9747ee3: a single plugin with BOTH a manifest schema defect AND
// structural defects (missing SKILL.md, broken symlink) must produce all
// three findings in a single `doctor --plugins` pass. The pre-fix behavior
// silently swallowed structural issues whenever the manifest failed schema
// validation, because AuditInstalledEntry fell back to an empty Package
// struct and collectSkillRoots returned nothing.
//
// This is the consolidation of the two pre-fix split tests
// (TestDoctorPluginsFlagReportsManifestSchemaIssues +
// TestDoctorPluginsFlagReportsSkillAndSymlinkIssues). They had to be split
// only because no single fixture could trigger both diagnostics under the
// broken audit flow — a consolidated test proves the audit dimensions are
// now independent.
func TestDoctorPluginsFlagReportsBothManifestSchemaAndStructuralIssues(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	pluginRoot := filepath.Join(homeDir, ".ddx", "plugins", "broken-plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, "skills", "missing-skill"), 0o755))
	require.NoError(t, os.Symlink("does-not-exist", filepath.Join(pluginRoot, "broken-link")))

	// Manifest: missing the required `description` field (schema defect)
	// alongside a valid `install.skills` section (so structural audit has
	// something to walk). Pre-fix: the schema error caused audit to skip
	// the install section entirely.
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, "package.yaml"), []byte(`name: broken-plugin
version: 1.0.0
type: plugin
source: https://example.com/broken-plugin
api_version: 1
install:
  skills:
    - source: skills
      target: .agents/skills
`), 0o644))

	state := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:    "broken-plugin",
				Version: "1.0.0",
				Type:    registry.PackageTypePlugin,
				Source:  pluginRoot,
				Files:   []string{pluginRoot},
			},
		},
	}
	require.NoError(t, registry.SaveState(state))

	factory := NewCommandFactory(workDir)
	output, err := executeWithStdoutCapture(t, factory.NewRootCommand(), "doctor", "--plugins")
	require.NoError(t, err)

	assert.Contains(t, output, "missing required field `description`",
		"manifest schema defect must be reported — the whole point of doctor --plugins")
	assert.Contains(t, output, "missing SKILL.md",
		"structural defect must ALSO surface — a schema error must not hide install-tree problems")
	assert.Contains(t, output, "broken symlink",
		"every structural issue must surface in the same pass — otherwise operators fix the schema and re-run, only to discover more")
}

func executeWithStdoutCapture(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()

	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&outBuf, stdoutR)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&errBuf, stderrR)
	}()

	root.SetArgs(args)
	err = root.Execute()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	wg.Wait()

	return outBuf.String() + errBuf.String(), err
}
