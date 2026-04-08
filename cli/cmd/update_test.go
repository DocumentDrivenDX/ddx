package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"github.com/DocumentDrivenDX/ddx/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformUpdateUsesBundledDDXVersionWhenReleaseLookupFails(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	state := &registry.InstalledState{
		Installed: []registry.InstalledEntry{
			{
				Name:        "ddx",
				Version:     "0.4.6",
				Type:        registry.PackageTypePlugin,
				Source:      "https://github.com/DocumentDrivenDX/ddx",
				InstalledAt: time.Now(),
			},
		},
	}
	require.NoError(t, registry.SaveState(state))

	originalFetch := fetchLatestPackageRelease
	originalInstall := installRegistryPackage
	t.Cleanup(func() {
		fetchLatestPackageRelease = originalFetch
		installRegistryPackage = originalInstall
	})

	fetchLatestPackageRelease = func(_ string) (*update.GitHubRelease, error) {
		return nil, errors.New("rate limited")
	}

	var installedVersion string
	installRegistryPackage = func(pkg *registry.Package) (registry.InstalledEntry, error) {
		installedVersion = pkg.Version
		return registry.InstalledEntry{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Type:        pkg.Type,
			Source:      pkg.Source,
			InstalledAt: time.Now(),
		}, nil
	}

	result, err := performUpdate("", &UpdateOptions{BundledDDXVersion: "0.5.6"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "0.5.6", installedVersion)
	assert.Equal(t, "Updated: ddx 0.4.6 → 0.5.6", result.Message)
}
