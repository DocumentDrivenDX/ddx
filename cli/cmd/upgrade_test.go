package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunUpgradeRefreshesInstalledPackagesWhenBinaryIsCurrent(t *testing.T) {
	t.Helper()

	originalFetch := fetchLatestReleaseForUpgrade
	originalExecute := executeUpgradeScript
	originalRefresh := refreshInstalledPackages
	t.Cleanup(func() {
		fetchLatestReleaseForUpgrade = originalFetch
		executeUpgradeScript = originalExecute
		refreshInstalledPackages = originalRefresh
	})

	fetchLatestReleaseForUpgrade = func() (*update.GitHubRelease, error) {
		return &update.GitHubRelease{TagName: "v0.5.6"}, nil
	}

	executeCalled := false
	executeUpgradeScript = func(_ io.Writer) error {
		executeCalled = true
		return nil
	}

	refreshCalled := false
	refreshInstalledPackages = func(workingDir string, opts *UpdateOptions) (*UpdateResult, error) {
		refreshCalled = true
		assert.Equal(t, "/tmp/project", workingDir)
		require.NotNil(t, opts)
		assert.False(t, opts.Force)
		return &UpdateResult{Success: true, Message: "Updated: ddx 0.4.6 → 0.5.6"}, nil
	}

	factory := &CommandFactory{Version: "v0.5.6", WorkingDir: "/tmp/project"}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("check", false, "")
	cmd.Flags().Bool("force", false, "")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := factory.runUpgrade(cmd, nil)
	require.NoError(t, err)
	assert.False(t, executeCalled)
	assert.True(t, refreshCalled)
	output := buf.String()
	assert.Contains(t, output, "already running the latest version")
	assert.Contains(t, output, "Updated: ddx 0.4.6")
}

func TestRunUpdateOnlyRefreshesInstalledPackagesOnce(t *testing.T) {
	t.Helper()

	originalFetch := fetchLatestReleaseForUpgrade
	originalExecute := executeUpgradeScript
	originalRefresh := refreshInstalledPackages
	t.Cleanup(func() {
		fetchLatestReleaseForUpgrade = originalFetch
		executeUpgradeScript = originalExecute
		refreshInstalledPackages = originalRefresh
	})

	fetchLatestReleaseForUpgrade = func() (*update.GitHubRelease, error) {
		return &update.GitHubRelease{TagName: "v0.5.6"}, nil
	}

	executeUpgradeScript = func(_ io.Writer) error {
		t.Fatal("binary-only preflight should not execute installer when already current")
		return nil
	}

	refreshCalls := 0
	refreshInstalledPackages = func(workingDir string, opts *UpdateOptions) (*UpdateResult, error) {
		refreshCalls++
		assert.Equal(t, "/tmp/project", workingDir)
		require.NotNil(t, opts)
		return &UpdateResult{Success: true, Message: "All packages are up to date."}, nil
	}

	factory := &CommandFactory{Version: "v0.5.6", WorkingDir: "/tmp/project"}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("check", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("reset", false, "")
	cmd.Flags().Bool("sync", false, "")
	cmd.Flags().String("strategy", "", "")
	cmd.Flags().Bool("backup", false, "")
	cmd.Flags().Bool("interactive", false, "")
	cmd.Flags().Bool("abort", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := factory.runUpdate(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, refreshCalls)
	output := buf.String()
	assert.True(t, strings.Contains(output, "All packages are up to date.") || strings.Contains(output, "Update check completed"))
}
