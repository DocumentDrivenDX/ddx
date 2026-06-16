package server

// MCP server registry and plugin manifest read endpoints — FEAT-009/015.
// GET /api/mcp-servers returns the library's mcp-servers/registry.yml.
// GET /api/plugins    returns the project's plugin lock/cache/shim state.
// MCP tools ddx_list_mcp_servers and ddx_list_plugins mirror the HTTP routes.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"gopkg.in/yaml.v3"
)

// mcpServerEntry is a single item in the GET /api/mcp-servers response.
type mcpServerEntry struct {
	Name        string `json:"name"        yaml:"name"`
	Category    string `json:"category"    yaml:"category"`
	Description string `json:"description" yaml:"description"`
}

// mcpServerRegistryFile is the parsed shape of mcp-servers/registry.yml.
type mcpServerRegistryFile struct {
	Servers []mcpServerEntry `yaml:"servers"`
}

// pluginInfo is the JSON representation of an installed plugin.
type pluginInfo struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Type           string   `json:"type"`
	Source         string   `json:"source"`
	InstalledAt    string   `json:"installed_at"`
	CachePath      string   `json:"cache_path,omitempty"`
	GeneratedFiles []string `json:"generated_files,omitempty"`
	Status         string   `json:"status"`
}

func pluginLockEntryToInfo(e registry.PluginLockEntry, projectRoot string) pluginInfo {
	ts := ""
	if !e.InstalledAt.IsZero() {
		ts = e.InstalledAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	status := projectPluginStatus(projectRoot, e)
	return pluginInfo{
		Name:           e.Name,
		Version:        e.Version,
		Type:           string(e.Type),
		Source:         e.Source,
		InstalledAt:    ts,
		CachePath:      e.CachePath,
		GeneratedFiles: append([]string(nil), e.GeneratedFiles...),
		Status:         status,
	}
}

func (s *Server) listMCPServersFor(workingDir string) ([]mcpServerEntry, error) {
	libPath := s.libraryPathFor(workingDir)
	if libPath == "" {
		return []mcpServerEntry{}, nil
	}
	data, err := os.ReadFile(filepath.Join(libPath, "mcp-servers", "registry.yml"))
	if err != nil {
		return []mcpServerEntry{}, nil
	}
	var reg mcpServerRegistryFile
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, err
	}
	if reg.Servers == nil {
		return []mcpServerEntry{}, nil
	}
	return reg.Servers, nil
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.listMCPServersFor(s.workingDirForRequest(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	workingDir := s.workingDirForRequest(r)
	infos, err := listProjectPlugins(workingDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, infos)
}

func (s *Server) mcpListMCPServers(workingDir string) mcpToolResult {
	servers, err := s.listMCPServersFor(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("error reading MCP server registry: " + err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(servers)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpListPlugins(workingDir string) mcpToolResult {
	infos, err := listProjectPlugins(workingDir)
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("error reading project plugins: " + err.Error())},
			IsError: true,
		}
	}
	data, _ := json.Marshal(infos)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func listProjectPlugins(workingDir string) ([]pluginInfo, error) {
	lock, err := registry.LoadProjectPluginLock(context.Background(), workingDir)
	if err != nil {
		return nil, err
	}
	infos := make([]pluginInfo, 0, len(lock.Plugins))
	for _, e := range lock.Plugins {
		infos = append(infos, pluginLockEntryToInfo(e, workingDir))
	}
	return infos, nil
}

func projectPluginStatus(projectRoot string, entry registry.PluginLockEntry) string {
	if projectRoot != "" && projectPluginLocalOverlay(projectRoot, entry.Name) {
		return "local-overlay"
	}
	cachePath := entry.CachePath
	if cachePath == "" {
		cachePath = registry.PluginCacheDir(entry.Name, entry.Version)
	}
	if info, err := os.Stat(cachePath); err != nil || !info.IsDir() {
		return "cache-missing"
	}
	for _, rel := range entry.GeneratedFiles {
		if rel == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			return "shims-missing"
		}
	}
	return "installed"
}

func projectPluginLocalOverlay(projectRoot, name string) bool {
	path := filepath.Join(ddxroot.Path(context.Background(), projectRoot), "plugins", name)
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
