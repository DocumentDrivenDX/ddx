package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	installScriptURL = "https://raw.githubusercontent.com/DocumentDrivenDX/ddx/main/install.sh"
)

var (
	fetchLatestReleaseForUpgrade = update.FetchLatestRelease
	executeUpgradeScript         = executeUpgrade
	refreshInstalledPackages     = performUpdate
)

func (f *CommandFactory) runUpgrade(cmd *cobra.Command, args []string) error {
	return f.runUpgradeWithOptions(cmd, args, true)
}

func (f *CommandFactory) runBinaryUpgrade(cmd *cobra.Command, args []string) error {
	return f.runUpgradeWithOptions(cmd, args, false)
}

func (f *CommandFactory) runUpgradeWithOptions(cmd *cobra.Command, args []string, syncInstalled bool) error {
	checkOnly, _ := cmd.Flags().GetBool("check")
	force, _ := cmd.Flags().GetBool("force")

	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	out := cmd.OutOrStdout()

	_, _ = cyan.Fprintln(out, "🔍 Checking for DDx updates...")
	_, _ = fmt.Fprintln(out)

	// Get current version
	currentVersion := f.Version
	if currentVersion == "" || currentVersion == "dev" {
		currentVersion = "v0.0.1-dev"
	}

	// Fetch latest release from GitHub
	latestRelease, err := fetchLatestReleaseForUpgrade()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := latestRelease.TagName

	// Display current and latest versions
	_, _ = fmt.Fprintf(out, "Current version: %s\n", currentVersion)
	_, _ = fmt.Fprintf(out, "Latest version:  %s\n", latestVersion)
	_, _ = fmt.Fprintln(out)

	// Compare versions
	needsUpgrade, err := update.NeedsUpgrade(currentVersion, latestVersion)
	if err != nil && !force {
		return fmt.Errorf("failed to compare versions: %w", err)
	}

	if checkOnly {
		if needsUpgrade || force {
			_, _ = yellow.Fprintln(out, "⬆️  A new version of DDx is available!")
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, "To upgrade, run:")
			_, _ = green.Fprintln(out, "  ddx upgrade")
		} else {
			_, _ = green.Fprintln(out, "✅ You are already running the latest version of DDx!")
		}
		return nil
	}

	if needsUpgrade || force {
		if force {
			_, _ = yellow.Fprintf(out, "⚠️  Force upgrading to %s...\n", latestVersion)
		} else {
			_, _ = cyan.Fprintf(out, "⬆️  Upgrading DDx from %s to %s...\n", currentVersion, latestVersion)
		}
		_, _ = fmt.Fprintln(out)

		// Download and execute install script
		if err := executeUpgradeScript(out); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		_, _ = fmt.Fprintln(out)
		_, _ = green.Fprintln(out, "✅ DDx has been upgraded successfully!")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Run 'ddx version' to verify the new version.")
	} else {
		_, _ = green.Fprintln(out, "✅ You are already running the latest version of DDx!")
	}

	f.Version = strings.TrimPrefix(latestVersion, "v")

	if syncInstalled {
		result, err := refreshInstalledPackages(
			f.WorkingDir,
			&UpdateOptions{
				Force:             force,
				BundledDDXVersion: f.Version,
			},
		)
		if err != nil {
			return fmt.Errorf("refreshing installed packages: %w", err)
		}
		if result != nil && result.Message != "" && result.Message != "No packages installed." {
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, result.Message)
		}
	}

	return nil
}

// executeUpgrade downloads and executes the install script
func executeUpgrade(out io.Writer) error {
	// Download install script
	resp, err := http.Get(installScriptURL)
	if err != nil {
		return fmt.Errorf("failed to download install script: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download install script: status %d", resp.StatusCode)
	}

	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read install script: %w", err)
	}

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "ddx-install-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(scriptContent); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write install script: %w", err)
	}

	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to make script executable: %w", err)
	}
	_ = tmpFile.Close()

	// Execute install script
	cmd := exec.Command("bash", tmpFile.Name())
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install script failed: %w", err)
	}

	return nil
}
