package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheTTL      = 24 * time.Hour
	errorRetryTTL = 5 * time.Minute
	cacheFileName = "last-update-check.json"
)

// Cache manages the update check cache file
type Cache struct {
	filePath string
	data     *CacheData
}

// NewCache creates a new Cache instance
func NewCache() *Cache {
	return &Cache{
		data: &CacheData{},
	}
}

// Load reads cache from disk
func (c *Cache) Load() error {
	// Get cache file path
	if c.filePath == "" {
		path, err := c.getCacheFilePath()
		if err != nil {
			return fmt.Errorf("failed to determine cache path: %w", err)
		}
		c.filePath = path
	}

	// Read file
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err // Return raw error for os.IsNotExist check
	}

	// Parse JSON
	if err := json.Unmarshal(data, c.data); err != nil {
		return fmt.Errorf("failed to parse cache: %w", err)
	}

	return nil
}

// Save writes cache to disk
func (c *Cache) Save() error {
	// Get cache file path
	if c.filePath == "" {
		path, err := c.getCacheFilePath()
		if err != nil {
			return fmt.Errorf("failed to determine cache path: %w", err)
		}
		c.filePath = path
	}

	// Ensure directory exists
	dir := filepath.Dir(c.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write to file
	if err := os.WriteFile(c.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// Invalidate resets the cache so the next ShouldCheck() returns true.
func (c *Cache) Invalidate() {
	c.data.LastCheck = time.Time{}
}

// InvalidateCache loads the on-disk cache and resets it so the next check
// goes to GitHub. Silent on error (best-effort).
func InvalidateCache() {
	c := NewCache()
	_ = c.Load()
	c.Invalidate()
	_ = c.Save()
}

// IsExpired checks whether the cache should be refreshed.
// Successful checks use the long TTL; failed checks retry sooner to avoid dogpiling.
func (c *Cache) IsExpired() bool {
	if c.data.LastCheck.IsZero() {
		return true // No check recorded
	}
	ttl := cacheTTL
	if c.data.CheckError != "" {
		ttl = errorRetryTTL
	}
	return time.Since(c.data.LastCheck) > ttl
}

// getCacheFilePath returns the cache file path following XDG Base Directory spec
func (c *Cache) getCacheFilePath() (string, error) {
	// Check XDG_CACHE_HOME first
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		// Fall back to ~/.cache
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}

	return filepath.Join(cacheDir, "ddx", cacheFileName), nil
}
