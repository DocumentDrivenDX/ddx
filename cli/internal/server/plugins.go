package server

// MCP server registry and plugin manifest read endpoints — FEAT-009/015.
// GET /api/mcp-servers returns the library's mcp-servers/registry.yml.
// GET /api/plugins    returns the user's ~/.ddx/installed.yaml state.
// MCP tools ddx_list_mcp_servers and ddx_list_plugins mirror the HTTP routes.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

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
	Name        string `json:"name"`
	Version     string `json:"version"`
	Type        string `json:"type"`
	Source      string `json:"source"`
	InstalledAt string `json:"installed_at"`
}

func installedEntryToInfo(e registry.InstalledEntry) pluginInfo {
	ts := ""
	if !e.InstalledAt.IsZero() {
		ts = e.InstalledAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return pluginInfo{
		Name:        e.Name,
		Version:     e.Version,
		Type:        string(e.Type),
		Source:      e.Source,
		InstalledAt: ts,
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

func (s *Server) handleListPlugins(w http.ResponseWriter, _ *http.Request) {
	state, err := registry.LoadState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	infos := make([]pluginInfo, 0, len(state.Installed))
	for _, e := range state.Installed {
		infos = append(infos, installedEntryToInfo(e))
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

func (s *Server) mcpListPlugins() mcpToolResult {
	state, err := registry.LoadState()
	if err != nil {
		return mcpToolResult{
			Content: []mcpContent{mcpText("error reading installed plugins: " + err.Error())},
			IsError: true,
		}
	}
	infos := make([]pluginInfo, 0, len(state.Installed))
	for _, e := range state.Installed {
		infos = append(infos, installedEntryToInfo(e))
	}
	data, _ := json.Marshal(infos)
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}
