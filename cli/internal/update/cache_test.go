package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_NewCache(t *testing.T) {
	cache := NewCache()
	assert.NotNil(t, cache)
	assert.NotNil(t, cache.data)
}

func TestCache_Load_NoFile(t *testing.T) {
	// Given: No cache file exists
	tempDir := t.TempDir()
	cache := &Cache{
		filePath: filepath.Join(tempDir, "nonexistent", "cache.json"),
	}

	// When: Load is called
	err := cache.Load()

	// Then: Should return error (file not found)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestCache_SaveAndLoad(t *testing.T) {
	// Given: A cache with data
	tempDir := t.TempDir()
	cache := &Cache{
		filePath: filepath.Join(tempDir, "cache.json"),
		data: &CacheData{
			LastCheck:       time.Now(),
			CurrentVersion:  "v0.1.2",
			LatestVersion:   "v0.1.3",
			UpdateAvailable: true,
		},
	}

	// When: Save is called
	err := cache.Save()
	require.NoError(t, err)

	// And: Load is called from a new cache instance
	loadedCache := &Cache{
		filePath: filepath.Join(tempDir, "cache.json"),
		data:     &CacheData{},
	}
	err = loadedCache.Load()

	// Then: Data should match
	require.NoError(t, err)
	assert.Equal(t, cache.data.CurrentVersion, loadedCache.data.CurrentVersion)
	assert.Equal(t, cache.data.LatestVersion, loadedCache.data.LatestVersion)
	assert.Equal(t, cache.data.UpdateAvailable, loadedCache.data.UpdateAvailable)

	// Timestamps should be within 1 second (JSON serialization precision)
	assert.WithinDuration(t, cache.data.LastCheck, loadedCache.data.LastCheck, time.Second)
}

func TestCache_IsExpired_Fresh(t *testing.T) {
	// Given: A cache with recent timestamp
	cache := &Cache{
		data: &CacheData{
			LastCheck: time.Now().Add(-1 * time.Hour), // 1 hour ago
		},
	}

	// When: IsExpired is called
	expired := cache.IsExpired()

	// Then: Should not be expired
	assert.False(t, expired, "Cache should not be expired after 1 hour")
}

func TestCache_IsExpired_Old(t *testing.T) {
	// Given: A cache with old timestamp
	cache := &Cache{
		data: &CacheData{
			LastCheck: time.Now().Add(-25 * time.Hour), // 25 hours ago
		},
	}

	// When: IsExpired is called
	expired := cache.IsExpired()

	// Then: Should be expired
	assert.True(t, expired, "Cache should be expired after 25 hours")
}

func TestCache_IsExpired_Exactly24Hours(t *testing.T) {
	// Given: A cache at exactly 24 hours (minus a millisecond to avoid timing issues)
	cache := &Cache{
		data: &CacheData{
			LastCheck: time.Now().Add(-24*time.Hour + time.Millisecond),
		},
	}

	// When: IsExpired is called
	expired := cache.IsExpired()

	// Then: Should not be expired (24h is the limit, > 24h is expired)
	assert.False(t, expired, "Cache at exactly 24 hours should not be expired")
}

func TestCache_IsExpired_FailureBackoffFresh(t *testing.T) {
	cache := &Cache{
		data: &CacheData{
			LastCheck:  time.Now().Add(-1 * time.Minute),
			CheckError: "failed to fetch latest release",
		},
	}

	assert.False(t, cache.IsExpired(), "Recent failure should use short retry backoff")
}

func TestCache_IsExpired_FailureBackoffOld(t *testing.T) {
	cache := &Cache{
		data: &CacheData{
			LastCheck:  time.Now().Add(-6 * time.Minute),
			CheckError: "failed to fetch latest release",
		},
	}

	assert.True(t, cache.IsExpired(), "Old failure should be retried after short backoff")
}

func TestCache_Load_CorruptedFile(t *testing.T) {
	// Given: A corrupted cache file
	tempDir := t.TempDir()
	cacheFile := filepath.Join(tempDir, "cache.json")

	err := os.WriteFile(cacheFile, []byte("invalid json {{{"), 0644)
	require.NoError(t, err)

	cache := &Cache{
		filePath: cacheFile,
		data:     &CacheData{},
	}

	// When: Load is called
	err = cache.Load()

	// Then: Should return JSON parse error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse cache")
}

func TestCache_Save_CreatesDirectory(t *testing.T) {
	// Given: Cache path in non-existent directory
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "nested", "cache")
	cacheFile := filepath.Join(cacheDir, "cache.json")

	cache := &Cache{
		filePath: cacheFile,
		data: &CacheData{
			LastCheck:      time.Now(),
			CurrentVersion: "v0.1.0",
		},
	}

	// When: Save is called
	err := cache.Save()

	// Then: Should create directory and file
	require.NoError(t, err)
	assert.FileExists(t, cacheFile)
}

func TestCache_GetCacheFilePath_XDGCompliance(t *testing.T) {
	// Given: XDG_CACHE_HOME environment variable set
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() { _ = os.Setenv("XDG_CACHE_HOME", originalXDG) }()

	testCacheDir := t.TempDir()
	_ = os.Setenv("XDG_CACHE_HOME", testCacheDir)

	// When: getCacheFilePath is called
	cache := &Cache{}
	path, err := cache.getCacheFilePath()

	// Then: Should use XDG_CACHE_HOME
	require.NoError(t, err)
	assert.Contains(t, path, testCacheDir)
	assert.Contains(t, path, "ddx")
	assert.Contains(t, path, "last-update-check.json")
}

func TestCache_GetCacheFilePath_FallbackToHome(t *testing.T) {
	// Given: XDG_CACHE_HOME not set
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer func() { _ = os.Setenv("XDG_CACHE_HOME", originalXDG) }()
	_ = os.Unsetenv("XDG_CACHE_HOME")

	// When: getCacheFilePath is called
	cache := &Cache{}
	path, err := cache.getCacheFilePath()

	// Then: Should fall back to ~/.cache/ddx/
	require.NoError(t, err)
	homeDir, _ := os.UserHomeDir()
	expectedPrefix := filepath.Join(homeDir, ".cache", "ddx")
	assert.Contains(t, path, expectedPrefix)
}

func TestCacheData_JSONSerialization(t *testing.T) {
	// Given: Cache data
	now := time.Now()
	data := &CacheData{
		LastCheck:       now,
		CurrentVersion:  "v0.1.2",
		LatestVersion:   "v0.1.3",
		UpdateAvailable: true,
		CheckError:      "",
	}

	// When: Serialize to JSON and back
	jsonBytes, err := json.Marshal(data)
	require.NoError(t, err)

	var loaded CacheData
	err = json.Unmarshal(jsonBytes, &loaded)

	// Then: Should match original
	require.NoError(t, err)
	assert.Equal(t, data.CurrentVersion, loaded.CurrentVersion)
	assert.Equal(t, data.LatestVersion, loaded.LatestVersion)
	assert.Equal(t, data.UpdateAvailable, loaded.UpdateAvailable)
	assert.WithinDuration(t, data.LastCheck, loaded.LastCheck, time.Second)
}

func TestCache_Load_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Test cannot run as root (permissions would be ignored)")
	}

	// Given: A cache file with no read permissions
	tempDir := t.TempDir()
	cacheFile := filepath.Join(tempDir, "cache.json")

	// Create file
	err := os.WriteFile(cacheFile, []byte(`{"last_check":"2025-01-01T00:00:00Z"}`), 0644)
	require.NoError(t, err)

	// Remove read permissions
	err = os.Chmod(cacheFile, 0000)
	require.NoError(t, err)
	defer func() { _ = os.Chmod(cacheFile, 0644) }() // Restore for cleanup

	cache := &Cache{
		filePath: cacheFile,
		data:     &CacheData{},
	}

	// When: Load is called
	err = cache.Load()

	// Then: Should return permission error
	assert.Error(t, err)
	assert.True(t, os.IsPermission(err))
}

func TestCache_Save_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Test cannot run as root (permissions would be ignored)")
	}

	// Given: A directory with no write permissions
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "readonly")

	err := os.Mkdir(cacheDir, 0755)
	require.NoError(t, err)

	// Remove write permissions
	err = os.Chmod(cacheDir, 0555)
	require.NoError(t, err)
	defer func() { _ = os.Chmod(cacheDir, 0755) }() // Restore for cleanup

	cache := &Cache{
		filePath: filepath.Join(cacheDir, "cache.json"),
		data: &CacheData{
			LastCheck: time.Now(),
		},
	}

	// When: Save is called
	err = cache.Save()

	// Then: Should return permission error
	assert.Error(t, err)
}
