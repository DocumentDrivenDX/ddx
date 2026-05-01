package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const pluginCacheFileName = "plugin-check.json"

// PluginCacheData stores the latest known version for each installed plugin.
type PluginCacheData struct {
	LastCheck time.Time         `json:"last_check"`
	Plugins   map[string]string `json:"plugins"` // name → latest version
}

// PluginCache manages the plugin version check cache file.
type PluginCache struct {
	filePath string
	Data     PluginCacheData
}

// NewPluginCache creates a PluginCache pointing at the default cache file.
func NewPluginCache() *PluginCache {
	return &PluginCache{
		Data: PluginCacheData{Plugins: make(map[string]string)},
	}
}

// Load reads the cache from disk. Returns nil on success or if the file simply
// does not exist yet (caller should treat missing cache as expired).
func (c *PluginCache) Load() error {
	if c.filePath == "" {
		path, err := c.cacheFilePath()
		if err != nil {
			return fmt.Errorf("plugin cache path: %w", err)
		}
		c.filePath = path
	}

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err // raw error so callers can os.IsNotExist-check
	}

	if err := json.Unmarshal(data, &c.Data); err != nil {
		return fmt.Errorf("plugin cache parse: %w", err)
	}
	if c.Data.Plugins == nil {
		c.Data.Plugins = make(map[string]string)
	}
	return nil
}

// Save writes the cache to disk.
func (c *PluginCache) Save() error {
	if c.filePath == "" {
		path, err := c.cacheFilePath()
		if err != nil {
			return fmt.Errorf("plugin cache path: %w", err)
		}
		c.filePath = path
	}

	if err := os.MkdirAll(filepath.Dir(c.filePath), 0755); err != nil {
		return fmt.Errorf("plugin cache dir: %w", err)
	}

	data, err := json.MarshalIndent(c.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin cache marshal: %w", err)
	}

	return os.WriteFile(c.filePath, data, 0644)
}

// IsExpired returns true when the cache is older than 24 hours or has never
// been written.
func (c *PluginCache) IsExpired() bool {
	return c.Data.LastCheck.IsZero() || time.Since(c.Data.LastCheck) > cacheTTL
}

func (c *PluginCache) cacheFilePath() (string, error) {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheDir, "ddx", pluginCacheFileName), nil
}
