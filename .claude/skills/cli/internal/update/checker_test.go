package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewChecker(t *testing.T) {
	cfg := config.DefaultNewConfig()
	checker := NewChecker("v0.1.2", cfg)

	assert.NotNil(t, checker)
	assert.Equal(t, "v0.1.2", checker.currentVersion)
	assert.NotNil(t, checker.cache)
}

func TestChecker_ShouldCheck_Disabled(t *testing.T) {
	// Given: Update check disabled in config
	cfg := config.DefaultNewConfig()
	cfg.UpdateCheck = &config.UpdateCheckConfig{
		Enabled:   false,
		Frequency: "24h",
	}

	checker := NewChecker("v0.1.2", cfg)

	// When: ShouldCheck is called
	should := checker.ShouldCheck()

	// Then: Should return false
	assert.False(t, should, "Should not check when disabled in config")
}

func TestChecker_ShouldCheck_CacheFresh(t *testing.T) {
	// Given: Config enabled, fresh cache
	tempDir := t.TempDir()
	cfg := config.DefaultNewConfig()
	cfg.UpdateCheck.Enabled = true

	checker := NewChecker("v0.1.2", cfg)

	// Create fresh cache
	checker.cache = &Cache{
		filePath: filepath.Join(tempDir, "cache.json"),
		data: &CacheData{
			LastCheck:      time.Now().Add(-1 * time.Hour), // 1 hour ago
			CurrentVersion: "v0.1.2",
		},
	}

	// When: ShouldCheck is called
	should := checker.ShouldCheck()

	// Then: Should return false (cache is fresh)
	assert.False(t, should, "Should not check when cache is fresh")
}

func TestChecker_ShouldCheck_CacheExpired(t *testing.T) {
	// Given: Config enabled, expired cache
	tempDir := t.TempDir()
	cfg := config.DefaultNewConfig()
	cfg.UpdateCheck.Enabled = true

	checker := NewChecker("v0.1.2", cfg)

	// Create expired cache
	checker.cache = &Cache{
		filePath: filepath.Join(tempDir, "cache.json"),
		data: &CacheData{
			LastCheck:      time.Now().Add(-25 * time.Hour), // 25 hours ago
			CurrentVersion: "v0.1.2",
		},
	}

	// When: ShouldCheck is called
	should := checker.ShouldCheck()

	// Then: Should return true (cache is expired)
	assert.True(t, should, "Should check when cache is expired")
}

func TestChecker_ShouldCheck_NoCache(t *testing.T) {
	// Given: Config enabled, no cache
	tempDir := t.TempDir()
	cfg := config.DefaultNewConfig()
	cfg.UpdateCheck.Enabled = true

	checker := NewChecker("v0.1.2", cfg)
	checker.cache = &Cache{
		filePath: filepath.Join(tempDir, "nonexistent.json"),
		data:     &CacheData{},
	}

	// When: ShouldCheck is called
	should := checker.ShouldCheck()

	// Then: Should return true (no cache exists)
	assert.True(t, should, "Should check when no cache exists")
}

func TestChecker_IsUpdateAvailable_NoUpdate(t *testing.T) {
	// Given: Checker with result showing no update
	cfg := config.DefaultNewConfig()
	checker := NewChecker("v0.1.2", cfg)
	checker.result = &UpdateCheckResult{
		UpdateAvailable: false,
		LatestVersion:   "v0.1.2",
		Error:           nil,
	}

	// When: IsUpdateAvailable is called
	available, version, err := checker.IsUpdateAvailable()

	// Then: Should return false
	assert.False(t, available)
	assert.Equal(t, "v0.1.2", version)
	assert.NoError(t, err)
}

func TestChecker_IsUpdateAvailable_UpdateExists(t *testing.T) {
	// Given: Checker with result showing update available
	cfg := config.DefaultNewConfig()
	checker := NewChecker("v0.1.2", cfg)
	checker.result = &UpdateCheckResult{
		UpdateAvailable: true,
		LatestVersion:   "v0.1.3",
		Error:           nil,
	}

	// When: IsUpdateAvailable is called
	available, version, err := checker.IsUpdateAvailable()

	// Then: Should return true with version
	assert.True(t, available)
	assert.Equal(t, "v0.1.3", version)
	assert.NoError(t, err)
}

func TestChecker_IsUpdateAvailable_NoResult(t *testing.T) {
	// Given: Checker with no result yet
	cfg := config.DefaultNewConfig()
	checker := NewChecker("v0.1.2", cfg)
	checker.result = nil

	// When: IsUpdateAvailable is called
	available, version, err := checker.IsUpdateAvailable()

	// Then: Should return false (check not completed)
	assert.False(t, available)
	assert.Empty(t, version)
	assert.NoError(t, err)
}

func TestChecker_CheckForUpdate_Success(t *testing.T) {
	// This test would need to mock the GitHub API
	// Skip for now as it requires network mocking
	t.Skip("Requires GitHub API mocking - to be implemented in BUILD phase")
}

func TestChecker_CheckForUpdate_NetworkError(t *testing.T) {
	// This test would simulate network failure
	t.Skip("Requires network error simulation - to be implemented in BUILD phase")
}

func TestChecker_CheckForUpdate_RateLimited(t *testing.T) {
	// This test would simulate GitHub API rate limiting
	t.Skip("Requires API rate limit simulation - to be implemented in BUILD phase")
}

// NOTE: No global state tests needed - checker is stored in CommandFactory instance

func TestChecker_RespectEnvVar(t *testing.T) {
	// Given: DDX_DISABLE_UPDATE_CHECK is set
	originalEnv := os.Getenv("DDX_DISABLE_UPDATE_CHECK")
	defer func() { _ = os.Setenv("DDX_DISABLE_UPDATE_CHECK", originalEnv) }()

	_ = os.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	cfg := config.DefaultNewConfig()
	cfg.UpdateCheck.Enabled = true // Config says enabled

	checker := NewChecker("v0.1.2", cfg)

	// When: CheckForUpdate is called
	ctx := context.Background()
	_, _ = checker.CheckForUpdate(ctx) // Result and error not used in skipped test

	// Then: Should skip check (respect env var over config)
	// Implementation detail: may return nil or empty result
	// This test documents the expected behavior
	t.Skip("Implementation detail to be determined in BUILD phase")
}

func TestChecker_Integration_FullFlow(t *testing.T) {
	// Integration test for full check flow
	// This would test:
	// 1. Create checker
	// 2. Check if should check (cache logic)
	// 3. Perform check (mocked GitHub API)
	// 4. Save to cache
	// 5. Retrieve result

	t.Skip("Integration test - requires GitHub API mocking - to be implemented in BUILD phase")
}

// NOTE: No concurrent access test needed - no global state, checker stored in factory

func TestChecker_VersionChanged(t *testing.T) {
	// Given: Cache with different version than current
	tempDir := t.TempDir()
	cfg := config.DefaultNewConfig()

	checker := NewChecker("v0.1.3", cfg) // Current version is v0.1.3

	checker.cache = &Cache{
		filePath: filepath.Join(tempDir, "cache.json"),
		data: &CacheData{
			LastCheck:       time.Now().Add(-1 * time.Hour), // Recent
			CurrentVersion:  "v0.1.2",                       // But old version
			LatestVersion:   "v0.1.3",
			UpdateAvailable: true,
		},
	}

	// When: ShouldCheck is called
	should := checker.ShouldCheck()

	// Then: Should check (version mismatch indicates binary was updated)
	assert.True(t, should, "Should check when current version differs from cached version")
}
