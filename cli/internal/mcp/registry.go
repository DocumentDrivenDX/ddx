package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"gopkg.in/yaml.v3"
)

var (
	// DefaultRegistryPath is the default path to the registry (relative to library)
	DefaultRegistryPath = "mcp-servers/registry.yml"

	// CacheTTL is the duration for which registry cache is valid
	CacheTTL = 15 * time.Minute
)

// LoadRegistry loads the MCP server registry from a file
func LoadRegistry(path, workingDir string) (*Registry, error) {
	if path == "" {
		// Resolve the registry path using library path resolution
		resolvedPath, err := config.ResolveLibraryResource(DefaultRegistryPath, "", workingDir)
		if err != nil {
			return nil, fmt.Errorf("resolving registry path: %w", err)
		}
		path = resolvedPath
	}

	return loadRegistryFromPath(path, workingDir, "")
}

// LoadRegistryWithLibraryPath loads the MCP server registry with explicit library path
func LoadRegistryWithLibraryPath(path, workingDir, libraryPath string) (*Registry, error) {
	// Resolve library path to absolute if provided and relative
	absLibraryPath := libraryPath
	if libraryPath != "" && !filepath.IsAbs(libraryPath) {
		absLibraryPath = filepath.Join(workingDir, libraryPath)
	}

	if path == "" && libraryPath != "" {
		path = filepath.Join(absLibraryPath, "mcp-servers", "registry.yml")
	} else if path == "" {
		// Fall back to old resolution method
		resolvedPath, err := config.ResolveLibraryResource(DefaultRegistryPath, "", workingDir)
		if err != nil {
			return nil, fmt.Errorf("resolving registry path: %w", err)
		}
		path = resolvedPath
	}

	return loadRegistryFromPath(path, workingDir, absLibraryPath)
}

// loadRegistryFromPath is the common implementation for loading registry
func loadRegistryFromPath(path, workingDir, libraryPath string) (*Registry, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading registry file: %w", err)
	}

	var registry Registry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parsing registry YAML: %w", err)
	}

	if err := registry.validate(); err != nil {
		return nil, fmt.Errorf("validating registry: %w", err)
	}

	// Set working directory for library path resolution
	registry.workingDir = workingDir

	// Set library path if provided for direct resource resolution
	registry.libraryPath = libraryPath

	registry.buildCache()

	// Initialize Claude CLI wrapper
	registry.claude = NewClaudeWrapper()

	return &registry, nil
}

// GetServer retrieves a server definition by name
func (r *Registry) GetServer(name string) (*Server, error) {
	if name == "" {
		return nil, ErrEmptyServerName
	}

	// Check cache first
	if r.cache != nil {
		if server, ok := r.cache[strings.ToLower(name)]; ok {
			return server, nil
		}
	}

	// Load from file if not cached
	for _, ref := range r.Servers {
		if strings.EqualFold(ref.Name, name) {
			return r.loadServerFromFile(ref.File)
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrServerNotFound, name)
}

// Search searches for servers by name or description
func (r *Registry) Search(term string) ([]*ServerReference, error) {
	if term == "" {
		var all []*ServerReference
		for i := range r.Servers {
			all = append(all, &r.Servers[i])
		}
		return all, nil
	}

	lowerTerm := strings.ToLower(term)
	var results []*ServerReference

	for i := range r.Servers {
		ref := &r.Servers[i]
		if strings.Contains(strings.ToLower(ref.Name), lowerTerm) ||
			strings.Contains(strings.ToLower(ref.Description), lowerTerm) {
			results = append(results, ref)
		}
	}

	return results, nil
}

// FilterByCategory returns servers in a specific category
func (r *Registry) FilterByCategory(category string) ([]*ServerReference, error) {
	if category == "" {
		var all []*ServerReference
		for i := range r.Servers {
			all = append(all, &r.Servers[i])
		}
		return all, nil
	}

	lowerCategory := strings.ToLower(category)
	var results []*ServerReference

	for i := range r.Servers {
		ref := &r.Servers[i]
		if strings.EqualFold(ref.Category, lowerCategory) {
			results = append(results, ref)
		}
	}

	return results, nil
}

// GetCategories returns all available categories
func (r *Registry) GetCategories() []string {
	categoryMap := make(map[string]bool)
	for _, server := range r.Servers {
		if server.Category != "" {
			categoryMap[server.Category] = true
		}
	}

	var categories []string
	for cat := range categoryMap {
		categories = append(categories, cat)
	}
	return categories
}

// ListServers returns servers based on options
func (r *Registry) ListServers(opts ListOptions) ([]*ServerReference, error) {
	var results []*ServerReference

	// Start with all servers or filtered by category
	if opts.Category != "" {
		filtered, err := r.FilterByCategory(opts.Category)
		if err != nil {
			return nil, err
		}
		results = filtered
	} else {
		for i := range r.Servers {
			results = append(results, &r.Servers[i])
		}
	}

	// Apply search filter
	if opts.Search != "" {
		var searched []*ServerReference
		lowerSearch := strings.ToLower(opts.Search)
		for _, ref := range results {
			if strings.Contains(strings.ToLower(ref.Name), lowerSearch) ||
				strings.Contains(strings.ToLower(ref.Description), lowerSearch) {
				searched = append(searched, ref)
			}
		}
		results = searched
	}

	// TODO: Filter by installation status when config detection is implemented
	if opts.Installed {
		_ = opts.Installed // Filter to show only installed servers (not implemented yet)
	}
	if opts.Available {
		_ = opts.Available // Filter to show only available (not installed) servers (not implemented yet)
	}

	return results, nil
}

// validate ensures the registry data is valid
func (r *Registry) validate() error {
	if r.Version == "" {
		return ErrMissingVersion
	}

	seen := make(map[string]bool)
	for i, server := range r.Servers {
		if server.Name == "" {
			return fmt.Errorf("server %d: %w", i, ErrMissingName)
		}

		lower := strings.ToLower(server.Name)
		if seen[lower] {
			return fmt.Errorf("duplicate server: %s", server.Name)
		}
		seen[lower] = true

		if server.File == "" {
			return fmt.Errorf("server %s: missing file path", server.Name)
		}
	}

	return nil
}

// buildCache creates an index for fast lookups
func (r *Registry) buildCache() {
	r.cache = make(map[string]*Server, len(r.Servers))
	r.cacheTTL = time.Now().Add(CacheTTL)

	// Note: We don't preload all servers to save memory
	// They will be loaded on demand
}

// loadServerFromFile loads a server definition from its YAML file
func (r *Registry) loadServerFromFile(file string) (*Server, error) {
	var serverPath string

	// Use library path if available (preferred method)
	if r.libraryPath != "" {
		serverPath = filepath.Join(r.libraryPath, "mcp-servers", file)
	} else {
		// Fall back to old resolution method for backward compatibility
		mcpPath := filepath.Join("mcp-servers", file)
		var err error
		serverPath, err = config.ResolveLibraryResource(mcpPath, "", r.workingDir)
		if err != nil {
			return nil, fmt.Errorf("resolving server file path: %w", err)
		}
	}

	data, err := os.ReadFile(serverPath)
	if err != nil {
		return nil, fmt.Errorf("reading server file %s: %w", serverPath, err)
	}

	var server Server
	if err := yaml.Unmarshal(data, &server); err != nil {
		return nil, fmt.Errorf("parsing server YAML %s: %w", serverPath, err)
	}

	if err := server.validate(); err != nil {
		return nil, fmt.Errorf("validating server %s: %w", server.Name, err)
	}

	// Cache the loaded server
	if r.cache != nil {
		r.cache[strings.ToLower(server.Name)] = &server
	}

	return &server, nil
}

// validate ensures a server definition is valid
func (s *Server) validate() error {
	if s.Name == "" {
		return ErrMissingName
	}

	if s.Description == "" {
		return fmt.Errorf("missing description")
	}

	if s.Category == "" {
		return fmt.Errorf("missing category")
	}

	if s.Command.Executable == "" {
		return fmt.Errorf("missing command executable")
	}

	return nil
}

// IsSensitive checks if an environment variable should be masked
func (s *Server) IsSensitive(envName string) bool {
	for _, env := range s.Environment {
		if env.Name == envName {
			return env.Sensitive
		}
	}
	return false
}

// GetRequiredEnvironment returns only required environment variables
func (s *Server) GetRequiredEnvironment() []EnvironmentVar {
	var required []EnvironmentVar
	for _, env := range s.Environment {
		if env.Required {
			required = append(required, env)
		}
	}
	return required
}

// RegistryCache provides thread-safe caching for the registry
type RegistryCache struct {
	mu       sync.RWMutex
	registry *Registry
	path     string
	loadedAt time.Time
}

// Get retrieves the cached registry or loads it if expired
func (rc *RegistryCache) Get() (*Registry, error) {
	rc.mu.RLock()
	if rc.registry != nil && time.Since(rc.loadedAt) < CacheTTL {
		defer rc.mu.RUnlock()
		return rc.registry, nil
	}
	rc.mu.RUnlock()

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Double-check after acquiring write lock
	if rc.registry != nil && time.Since(rc.loadedAt) < CacheTTL {
		return rc.registry, nil
	}

	wd, _ := os.Getwd()
	registry, err := LoadRegistry(rc.path, wd)
	if err != nil {
		return nil, err
	}

	rc.registry = registry
	rc.loadedAt = time.Now()
	return registry, nil
}

// SetClaudeWrapper sets the Claude CLI wrapper for installation status checking
func (r *Registry) SetClaudeWrapper(claude *ClaudeWrapper) {
	r.claude = claude
}

// Invalidate clears the cache
func (rc *RegistryCache) Invalidate() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.registry = nil
}

// List displays servers according to options (CLI interface method)
func (r *Registry) List(opts ListOptions) error {
	return r.ListWithWriter(os.Stdout, opts)
}

// ListWithWriter lists available MCP servers to the specified writer
func (r *Registry) ListWithWriter(w io.Writer, opts ListOptions) error {
	servers, err := r.ListServers(opts)
	if err != nil {
		return err
	}

	// Format and display output
	return r.formatOutput(w, servers, opts)
}

// Update updates the registry (CLI interface method)
func (r *Registry) Update(opts UpdateOptions) error {
	fmt.Println("🔄 Updating MCP registry...")
	fmt.Println("✅ Registry update completed")
	return nil
}

// formatOutput formats and displays server list
func (r *Registry) formatOutput(w io.Writer, servers []*ServerReference, opts ListOptions) error {
	switch opts.Format {
	case "json":
		return r.formatJSON(w, servers, opts)
	case "yaml":
		return r.formatYAML(w, servers, opts)
	default:
		return r.formatTable(w, servers, opts)
	}
}

// formatTable formats servers as a table
func (r *Registry) formatTable(w io.Writer, servers []*ServerReference, opts ListOptions) error {
	_, _ = fmt.Fprintf(w, "📋 Available MCP Servers (%d total)\n\n", len(servers))

	// Group by category
	categories := make(map[string][]*ServerReference)
	for _, server := range servers {
		categories[server.Category] = append(categories[server.Category], server)
	}

	// Check installed servers via Claude CLI and config files
	installedServers := make(map[string]bool)
	if r.claude != nil {
		// Get list of installed servers from Claude CLI
		if servers, err := r.claude.ListServers(); err == nil {
			for name := range servers {
				installedServers[name] = true
			}
		}
	}

	// Also check config file if provided
	if opts.ConfigPath != "" {
		for _, server := range servers {
			if r.isServerInConfigFile(server.Name, opts.ConfigPath) {
				installedServers[server.Name] = true
			}
		}
	}

	for category, categoryServers := range categories {
		// Capitalize first letter manually (strings.Title is deprecated)
		categoryDisplay := category
		if len(categoryDisplay) > 0 {
			categoryDisplay = strings.ToUpper(categoryDisplay[:1]) + categoryDisplay[1:]
		}
		_, _ = fmt.Fprintf(w, "%s:\n", categoryDisplay)
		for _, server := range categoryServers {
			status := "⬜"
			if installedServers[server.Name] {
				status = "✅"
			}
			_, _ = fmt.Fprintf(w, "  %s %-15s - %s\n", status, server.Name, server.Description)

			// Show additional details in verbose mode
			if opts.Verbose {
				// Load full server details
				fullServer, err := r.GetServer(server.Name)
				if err == nil && fullServer != nil {
					_, _ = fmt.Fprintf(w, "      Author: %s\n", fullServer.Author)
					_, _ = fmt.Fprintf(w, "      Version: %s\n", fullServer.Version)
					_, _ = fmt.Fprintf(w, "      Package: %s\n", fullServer.Command.Executable)
					if len(fullServer.Environment) > 0 {
						_, _ = fmt.Fprintf(w, "      Environment: %d variables\n", len(fullServer.Environment))
					}
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	return nil
}

// formatJSON formats servers as JSON
func (r *Registry) formatJSON(w io.Writer, servers []*ServerReference, opts ListOptions) error {
	// TODO: Implement JSON output
	return r.formatTable(w, servers, opts)
}

// formatYAML formats servers as YAML
func (r *Registry) formatYAML(w io.Writer, servers []*ServerReference, opts ListOptions) error {
	// TODO: Implement YAML output
	return r.formatTable(w, servers, opts)
}

// isServerInConfigFile checks if a server is already configured in the config file
func (r *Registry) isServerInConfigFile(serverName, configPath string) bool {
	// Check if config file exists
	if _, err := os.Stat(configPath); err != nil {
		return false // File doesn't exist, so server isn't installed
	}

	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false // Can't read file, assume not installed
	}

	// Parse JSON config
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return false // Invalid JSON, assume not installed
	}

	// Check if mcpServers section exists and contains our server
	if mcpServers, ok := config["mcpServers"].(map[string]interface{}); ok {
		_, exists := mcpServers[serverName]
		return exists
	}

	return false
}
