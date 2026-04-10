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
