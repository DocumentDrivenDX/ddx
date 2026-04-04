package update

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
)

// Checker handles update checking logic
type Checker struct {
	currentVersion string
	config         *config.NewConfig
	cache          *Cache
	result         *UpdateCheckResult
}

// NewChecker creates a new Checker instance
func NewChecker(version string, cfg *config.NewConfig) *Checker {
	cache := NewCache()
	// Try to load cache, but don't fail if it doesn't exist
	_ = cache.Load()

	return &Checker{
		currentVersion: version,
		config:         cfg,
		cache:          cache,
	}
}

// ShouldCheck returns true if an update check is needed
func (c *Checker) ShouldCheck() bool {
	// Check if disabled in config
	if c.config.UpdateCheck != nil && !c.config.UpdateCheck.Enabled {
		return false
	}

	// Check if cache is expired or missing
	if c.cache.IsExpired() {
		return true
	}

	// Check if version changed (binary was upgraded)
	if c.cache.data.CurrentVersion != "" && c.cache.data.CurrentVersion != c.currentVersion {
		return true
	}

	// Cache is valid and fresh
	return false
}

// CheckForUpdate performs an update check if needed
// Returns the result, which is also stored in c.result for later retrieval
func (c *Checker) CheckForUpdate(ctx context.Context) (*UpdateCheckResult, error) {
	// Check if disabled via env var (takes precedence)
	if os.Getenv("DDX_DISABLE_UPDATE_CHECK") == "1" {
		c.result = &UpdateCheckResult{
			UpdateAvailable: false,
			LatestVersion:   c.currentVersion,
			Error:           nil,
		}
		return c.result, nil
	}

	// If check not needed, return cached result
	if !c.ShouldCheck() {
		c.result = &UpdateCheckResult{
			UpdateAvailable: c.cache.data.UpdateAvailable,
			LatestVersion:   c.cache.data.LatestVersion,
			Error:           nil,
		}
		return c.result, nil
	}

	// Perform actual check
	result := &UpdateCheckResult{}

	// Fetch latest release from GitHub
	release, err := FetchLatestRelease()
	if err != nil {
		result.Error = fmt.Errorf("failed to fetch latest release: %w", err)
		c.result = result

		// Update cache with error
		c.cache.data.LastCheck = time.Now()
		c.cache.data.CurrentVersion = c.currentVersion
		c.cache.data.CheckError = err.Error()
		_ = c.cache.Save() // Ignore save errors

		return result, err
	}

	// Compare versions
	needsUpgrade, err := NeedsUpgrade(c.currentVersion, release.TagName)
	if err != nil {
		result.Error = fmt.Errorf("failed to compare versions: %w", err)
		c.result = result

		// Update cache with error
		c.cache.data.LastCheck = time.Now()
		c.cache.data.CurrentVersion = c.currentVersion
		c.cache.data.CheckError = err.Error()
		_ = c.cache.Save() // Ignore save errors

		return result, err
	}

	// Update result
	result.UpdateAvailable = needsUpgrade
	result.LatestVersion = release.TagName
	c.result = result

	// Update cache with successful check
	c.cache.data.LastCheck = time.Now()
	c.cache.data.CurrentVersion = c.currentVersion
	c.cache.data.LatestVersion = release.TagName
	c.cache.data.UpdateAvailable = needsUpgrade
	c.cache.data.CheckError = ""

	_ = c.cache.Save() // Ignore save errors

	return result, nil
}

// IsUpdateAvailable returns the result from the last check
func (c *Checker) IsUpdateAvailable() (bool, string, error) {
	if c.result == nil {
		return false, "", nil
	}
	return c.result.UpdateAvailable, c.result.LatestVersion, c.result.Error
}
