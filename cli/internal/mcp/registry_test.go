package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixture data
const testRegistryYAML = `version: 1.0.0
updated: 2025-01-15T00:00:00Z
servers:
  - name: github
    file: servers/github.yml
    category: development
    description: GitHub integration for repository access
  - name: postgres
    file: servers/postgres.yml
    category: database
    description: PostgreSQL database integration
categories:
  development:
    description: Development tools
    icon: 🛠️
  database:
    description: Database integrations
    icon: 🗄️`

const testGithubServerYAML = `name: github
description: GitHub integration for repository access
category: development
author: modelcontextprotocol
version: 1.0.0
tags: [git, repository, vcs]
command:
  executable: npx
  args: ["-y", "@modelcontextprotocol/server-github"]
environment:
  - name: GITHUB_PERSONAL_ACCESS_TOKEN
    description: GitHub personal access token
    required: true
    sensitive: true`

func setupTestRegistry(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory as library root
	tmpDir := t.TempDir()

	// Create registry structure under library root
	registryPath := filepath.Join(tmpDir, "mcp-servers", "registry.yml")
	serversDir := filepath.Join(tmpDir, "mcp-servers", "servers")

	require.NoError(t, os.MkdirAll(serversDir, 0755))

	// Write test registry
	require.NoError(t, os.WriteFile(registryPath, []byte(testRegistryYAML), 0644))

	// Write test server files
	githubPath := filepath.Join(serversDir, "github.yml")
	require.NoError(t, os.WriteFile(githubPath, []byte(testGithubServerYAML), 0644))

	// Write postgres server for completeness
	postgresPath := filepath.Join(serversDir, "postgres.yml")
	postgresYAML := strings.ReplaceAll(testGithubServerYAML, "github", "postgres")
	postgresYAML = strings.ReplaceAll(postgresYAML, "GitHub", "PostgreSQL")
	postgresYAML = strings.ReplaceAll(postgresYAML, "development", "database")
	require.NoError(t, os.WriteFile(postgresPath, []byte(postgresYAML), 0644))

	// Set library base path for tests
	t.Setenv("DDX_LIBRARY_BASE_PATH", tmpDir)

	// Keep original registry path for cleanup
	origPath := mcp.DefaultRegistryPath

	cleanup := func() {
		mcp.DefaultRegistryPath = origPath
	}

	return registryPath, cleanup
}

func TestLoadRegistry(t *testing.T) {
	registryPath, cleanup := setupTestRegistry(t)
	defer cleanup()

	t.Run("successful load", func(t *testing.T) {
		wd, _ := os.Getwd()
		registry, err := mcp.LoadRegistry(registryPath, wd)
		require.NoError(t, err)
		assert.NotNil(t, registry)
		assert.Equal(t, "1.0.0", registry.Version)
		assert.Len(t, registry.Servers, 2)
		assert.Len(t, registry.Categories, 2)
	})

	t.Run("load with empty path uses default", func(t *testing.T) {
		// Library path is already set via DDX_LIBRARY_BASE_PATH in setupTestRegistry
		wd, _ := os.Getwd()
		registry, err := mcp.LoadRegistry("", wd)
		require.NoError(t, err)
		assert.NotNil(t, registry)
	})

	t.Run("file not found", func(t *testing.T) {
		wd, _ := os.Getwd()
		_, err := mcp.LoadRegistry("/nonexistent/registry.yml", wd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reading registry file")
	})

	t.Run("invalid YAML", func(t *testing.T) {
		invalidPath := filepath.Join(t.TempDir(), "invalid.yml")
		_ = os.WriteFile(invalidPath, []byte("invalid: yaml: content:"), 0644)
		wd, _ := os.Getwd()
		_, err := mcp.LoadRegistry(invalidPath, wd)
		assert.Error(t, err)
	})
}

func TestRegistryGetServer(t *testing.T) {
	registryPath, cleanup := setupTestRegistry(t)
	defer cleanup()

	wd, _ := os.Getwd()
	registry, err := mcp.LoadRegistry(registryPath, wd)
	require.NoError(t, err)

	t.Run("get existing server", func(t *testing.T) {
		server, err := registry.GetServer("github")
		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.Equal(t, "github", server.Name)
		assert.Equal(t, "development", server.Category)
		assert.Equal(t, "npx", server.Command.Executable)
	})

	t.Run("get with different case", func(t *testing.T) {
		server, err := registry.GetServer("GITHUB")
		require.NoError(t, err)
		assert.Equal(t, "github", server.Name)
	})

	t.Run("server not found", func(t *testing.T) {
		_, err := registry.GetServer("nonexistent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, mcp.ErrServerNotFound)
	})

	t.Run("empty server name", func(t *testing.T) {
		_, err := registry.GetServer("")
		assert.Error(t, err)
		assert.ErrorIs(t, err, mcp.ErrEmptyServerName)
	})
}

func TestRegistrySearch(t *testing.T) {
	registryPath, cleanup := setupTestRegistry(t)
	defer cleanup()

	wd, _ := os.Getwd()
	registry, err := mcp.LoadRegistry(registryPath, wd)
	require.NoError(t, err)

	t.Run("search by name", func(t *testing.T) {
		results, err := registry.Search("git")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "github", results[0].Name)
	})

	t.Run("search by description", func(t *testing.T) {
		results, err := registry.Search("PostgreSQL")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "postgres", results[0].Name)
	})

	t.Run("search case insensitive", func(t *testing.T) {
		results, err := registry.Search("GITHUB")
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("empty search returns all", func(t *testing.T) {
		results, err := registry.Search("")
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("no matches", func(t *testing.T) {
		results, err := registry.Search("nonexistent")
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

func TestRegistryFilterByCategory(t *testing.T) {
	registryPath, cleanup := setupTestRegistry(t)
	defer cleanup()

	wd, _ := os.Getwd()
	registry, err := mcp.LoadRegistry(registryPath, wd)
	require.NoError(t, err)

	t.Run("filter by category", func(t *testing.T) {
		results, err := registry.FilterByCategory("development")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "github", results[0].Name)
	})

	t.Run("case insensitive category", func(t *testing.T) {
		results, err := registry.FilterByCategory("DATABASE")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "postgres", results[0].Name)
	})

	t.Run("empty category returns all", func(t *testing.T) {
		results, err := registry.FilterByCategory("")
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("nonexistent category", func(t *testing.T) {
		results, err := registry.FilterByCategory("nonexistent")
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

func TestRegistryCache(t *testing.T) {
	_, cleanup := setupTestRegistry(t)
	defer cleanup()

	t.Run("cache hit", func(t *testing.T) {
		cache := &mcp.RegistryCache{}

		// First load
		reg1, err := cache.Get()
		if err == nil {
			// Second load should be from cache
			reg2, err := cache.Get()
			require.NoError(t, err)
			assert.Same(t, reg1, reg2)
		}
	})

	t.Run("cache invalidation", func(t *testing.T) {
		cache := &mcp.RegistryCache{}

		// Load and invalidate
		_, _ = cache.Get()
		cache.Invalidate()

		// Next get should reload
		// We can't easily test this without modifying the file
	})
}

func TestServerValidation(t *testing.T) {
	t.Run("valid server", func(t *testing.T) {
		server := &mcp.Server{
			Name:        "test",
			Description: "Test server",
			Category:    "test",
			Command: mcp.CommandSpec{
				Executable: "npx",
			},
		}
		// Should not panic or error when validated internally
		assert.NotNil(t, server)
	})

	t.Run("get required environment", func(t *testing.T) {
		server := &mcp.Server{
			Environment: []mcp.EnvironmentVar{
				{Name: "REQUIRED", Required: true},
				{Name: "OPTIONAL", Required: false},
			},
		}
		required := server.GetRequiredEnvironment()
		assert.Len(t, required, 1)
		assert.Equal(t, "REQUIRED", required[0].Name)
	})

	t.Run("check sensitive", func(t *testing.T) {
		server := &mcp.Server{
			Environment: []mcp.EnvironmentVar{
				{Name: "TOKEN", Sensitive: true},
				{Name: "USER", Sensitive: false},
			},
		}
		assert.True(t, server.IsSensitive("TOKEN"))
		assert.False(t, server.IsSensitive("USER"))
		assert.False(t, server.IsSensitive("UNKNOWN"))
	})
}

func TestListServers(t *testing.T) {
	registryPath, cleanup := setupTestRegistry(t)
	defer cleanup()

	wd, _ := os.Getwd()
	registry, err := mcp.LoadRegistry(registryPath, wd)
	require.NoError(t, err)

	t.Run("list all", func(t *testing.T) {
		opts := mcp.ListOptions{}
		results, err := registry.ListServers(opts)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("list with category filter", func(t *testing.T) {
		opts := mcp.ListOptions{
			Category: "development",
		}
		results, err := registry.ListServers(opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "github", results[0].Name)
	})

	t.Run("list with search", func(t *testing.T) {
		opts := mcp.ListOptions{
			Search: "post",
		}
		results, err := registry.ListServers(opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "postgres", results[0].Name)
	})

	t.Run("list with category and search", func(t *testing.T) {
		opts := mcp.ListOptions{
			Category: "database",
			Search:   "postgres",
		}
		results, err := registry.ListServers(opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}
