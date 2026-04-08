package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayUpdateNotificationSkipsNonInteractiveOutput(t *testing.T) {
	originalTTY := isInteractiveOutput
	originalAvailability := updateAvailability
	t.Cleanup(func() {
		isInteractiveOutput = originalTTY
		updateAvailability = originalAvailability
	})

	isInteractiveOutput = func(_ io.Writer) bool {
		return false
	}
	updateAvailability = func(_ *update.Checker) (bool, string, error) {
		return true, "v0.5.6", nil
	}

	factory := NewCommandFactory(t.TempDir())
	factory.updateChecker = &update.Checker{}

	cmd := &cobra.Command{Use: "status"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := factory.displayUpdateNotification(cmd)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestDisplayUpdateNotificationShowsInteractiveOutput(t *testing.T) {
	originalTTY := isInteractiveOutput
	originalAvailability := updateAvailability
	t.Cleanup(func() {
		isInteractiveOutput = originalTTY
		updateAvailability = originalAvailability
	})

	isInteractiveOutput = func(_ io.Writer) bool {
		return true
	}
	updateAvailability = func(_ *update.Checker) (bool, string, error) {
		return true, "v0.5.6", nil
	}

	factory := NewCommandFactory(t.TempDir())
	factory.updateChecker = &update.Checker{}

	cmd := &cobra.Command{Use: "status"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := factory.displayUpdateNotification(cmd)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Update available: v0.5.6")
}

func TestDisplayStalenessHintsSkipsNonInteractiveOutput(t *testing.T) {
	originalTTY := isInteractiveOutput
	t.Cleanup(func() {
		isInteractiveOutput = originalTTY
	})
	isInteractiveOutput = func(_ io.Writer) bool {
		return false
	}

	setupStalenessHintFixtures(t)

	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "versions.yaml"), []byte("ddx_version: \"0.4.6\"\n"), 0o644))

	factory := NewCommandFactory(workingDir)
	factory.Version = "v0.5.6"

	cmd := &cobra.Command{Use: "status"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	factory.displayStalenessHints(cmd)
	assert.Empty(t, buf.String())
}

func TestDisplayStalenessHintsShowsInteractiveOutput(t *testing.T) {
	originalTTY := isInteractiveOutput
	t.Cleanup(func() {
		isInteractiveOutput = originalTTY
	})
	isInteractiveOutput = func(_ io.Writer) bool {
		return true
	}

	setupStalenessHintFixtures(t)

	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, ".ddx", "versions.yaml"), []byte("ddx_version: \"0.4.6\"\n"), 0o644))

	factory := NewCommandFactory(workingDir)
	factory.Version = "v0.5.6"

	cmd := &cobra.Command{Use: "status"}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	factory.displayStalenessHints(cmd)
	output := buf.String()
	assert.Contains(t, output, "Project skills from DDx v0.4.6")
	assert.Contains(t, output, "helix 0.3.2 installed, 0.3.3 available")
}

func setupStalenessHintFixtures(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	installedState := []byte(`installed:
  - name: helix
    version: 0.3.2
    type: workflow
    source: https://github.com/DocumentDrivenDX/helix
    installed_at: 2026-04-07T21:05:08Z
    files:
      - /tmp/helix
`)
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".ddx", "installed.yaml"), installedState, 0o644))

	cache := update.NewPluginCache()
	cache.Data.LastCheck = time.Now()
	cache.Data.Plugins["helix"] = "0.3.3"
	require.NoError(t, cache.Save())
}
