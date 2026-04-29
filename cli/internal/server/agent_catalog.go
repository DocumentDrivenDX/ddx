package server

// Agent model/catalog/capabilities endpoints — FEAT-006 read-coverage gap.
// Adds /api/agent/models, /api/agent/catalog, /api/agent/capabilities HTTP
// routes and corresponding ddx_agent_models, ddx_agent_catalog,
// ddx_agent_capabilities MCP tools.

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
)

// ---- Response types ----

// AgentModelsProvider is one provider entry returned by GET /api/agent/models.
type AgentModelsProvider struct {
	Provider     string               `json:"provider"`
	Type         string               `json:"type"`
	IsDefault    bool                 `json:"is_default"`
	DefaultModel string               `json:"default_model,omitempty"`
	Models       []agentlib.ModelInfo `json:"models"`
}

// AgentCatalogResponse is the response shape for GET /api/agent/catalog.
type AgentCatalogResponse struct {
	Source    string                      `json:"source"` // "file" | "built-in"
	Path      string                      `json:"path,omitempty"`
	UpdatedAt string                      `json:"updated_at,omitempty"`
	Tiers     map[string]AgentCatalogTier `json:"tiers"`
	Models    []agent.ModelEntryYAML      `json:"models"`
}

// AgentCatalogTier is one tier entry in the catalog response.
type AgentCatalogTier struct {
	Description string            `json:"description"`
	Surfaces    map[string]string `json:"surfaces"`
}

// ---- HTTP handlers ----

// handleAgentModels serves GET /api/agent/models.
// Query params:
//   - provider: filter by provider name (default: configured default)
//   - all: if "true", return models for every configured provider
func (s *Server) handleAgentModels(w http.ResponseWriter, r *http.Request) {
	workDir := s.workingDirForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	svc, err := agent.NewServiceFromWorkDir(workDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	providers, err := svc.ListProviders(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	showAll := r.URL.Query().Get("all") == "true"
	providerFilter := r.URL.Query().Get("provider")

	if showAll {
		result := make([]AgentModelsProvider, 0, len(providers))
		for _, p := range providers {
			models, _ := svc.ListModels(ctx, agentlib.ModelFilter{Provider: p.Name})
			if models == nil {
				models = []agentlib.ModelInfo{}
			}
			result = append(result, AgentModelsProvider{
				Provider:     p.Name,
				Type:         p.Type,
				IsDefault:    p.IsDefault,
				DefaultModel: p.DefaultModel,
				Models:       models,
			})
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Single provider: use filter or default.
	name := providerFilter
	if name == "" {
		for _, p := range providers {
			if p.IsDefault {
				name = p.Name
				break
			}
		}
	}
	if name == "" && len(providers) > 0 {
		name = providers[0].Name
	}

	var prov agentlib.ProviderInfo
	for _, p := range providers {
		if p.Name == name {
			prov = p
			break
		}
	}
	if prov.Name == "" && name != "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found: " + name})
		return
	}

	models, _ := svc.ListModels(ctx, agentlib.ModelFilter{Provider: prov.Name})
	if models == nil {
		models = []agentlib.ModelInfo{}
	}
	writeJSON(w, http.StatusOK, AgentModelsProvider{
		Provider:     prov.Name,
		Type:         prov.Type,
		IsDefault:    prov.IsDefault,
		DefaultModel: prov.DefaultModel,
		Models:       models,
	})
}

// handleAgentCatalog serves GET /api/agent/catalog.
// Returns the effective model catalog (file or built-in defaults).
func (s *Server) handleAgentCatalog(w http.ResponseWriter, r *http.Request) {
	path := agent.DefaultModelCatalogPath()
	cat, err := agent.LoadModelCatalogYAML(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	source := "built-in"
	var pathOut string
	var updatedAt string

	if cat == nil {
		cat = agent.DefaultModelCatalogYAML()
	} else {
		source = "file"
		pathOut = path
	}

	if !cat.UpdatedAt.IsZero() {
		updatedAt = cat.UpdatedAt.UTC().Format(time.RFC3339)
	}

	tiers := make(map[string]AgentCatalogTier, len(cat.Tiers))
	for tierName, tierDef := range cat.Tiers {
		surfaces := make(map[string]string, len(tierDef.Surfaces))
		for k, v := range tierDef.Surfaces {
			surfaces[k] = v
		}
		tiers[tierName] = AgentCatalogTier{
			Description: tierDef.Description,
			Surfaces:    surfaces,
		}
	}

	models := cat.Models
	if models == nil {
		models = []agent.ModelEntryYAML{}
	}

	writeJSON(w, http.StatusOK, AgentCatalogResponse{
		Source:    source,
		Path:      pathOut,
		UpdatedAt: updatedAt,
		Tiers:     tiers,
		Models:    models,
	})
}

// handleAgentCapabilities serves GET /api/agent/capabilities.
// Query param: harness (optional; defaults to configured default harness).
func (s *Server) handleAgentCapabilities(w http.ResponseWriter, r *http.Request) {
	workDir := s.workingDirForRequest(r)
	harness := r.URL.Query().Get("harness")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if harness == "" {
		// Derive default harness from config.
		svc, err := agent.NewServiceFromWorkDir(workDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		infos, err := svc.ListHarnesses(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for _, h := range infos {
			if h.Available {
				harness = h.Name
				break
			}
		}
	}

	if harness == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "harness required: no harness specified and no available harness found"})
		return
	}

	caps, err := agent.CapabilitiesViaService(ctx, workDir, harness)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, caps)
}

// ---- MCP tool implementations ----

func (s *Server) mcpAgentModels(workingDir, providerName string, showAll bool) mcpToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, err := agent.NewServiceFromWorkDir(workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	providers, err := svc.ListProviders(ctx)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	if showAll {
		result := make([]AgentModelsProvider, 0, len(providers))
		for _, p := range providers {
			models, _ := svc.ListModels(ctx, agentlib.ModelFilter{Provider: p.Name})
			if models == nil {
				models = []agentlib.ModelInfo{}
			}
			result = append(result, AgentModelsProvider{
				Provider:     p.Name,
				Type:         p.Type,
				IsDefault:    p.IsDefault,
				DefaultModel: p.DefaultModel,
				Models:       models,
			})
		}
		data, err := json.Marshal(result)
		if err != nil {
			return mcpToolResult{Content: []mcpContent{mcpText("[]")}}
		}
		return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
	}

	name := providerName
	if name == "" {
		for _, p := range providers {
			if p.IsDefault {
				name = p.Name
				break
			}
		}
	}
	if name == "" && len(providers) > 0 {
		name = providers[0].Name
	}

	var prov agentlib.ProviderInfo
	for _, p := range providers {
		if p.Name == name {
			prov = p
			break
		}
	}
	if prov.Name == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("provider not found: " + name)}, IsError: true}
	}

	models, _ := svc.ListModels(ctx, agentlib.ModelFilter{Provider: prov.Name})
	if models == nil {
		models = []agentlib.ModelInfo{}
	}
	data, err := json.Marshal(AgentModelsProvider{
		Provider:     prov.Name,
		Type:         prov.Type,
		IsDefault:    prov.IsDefault,
		DefaultModel: prov.DefaultModel,
		Models:       models,
	})
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("{}")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpAgentCatalog() mcpToolResult {
	path := agent.DefaultModelCatalogPath()
	cat, err := agent.LoadModelCatalogYAML(path)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	source := "built-in"
	var pathOut string
	var updatedAt string

	if cat == nil {
		cat = agent.DefaultModelCatalogYAML()
	} else {
		source = "file"
		pathOut = path
	}

	if !cat.UpdatedAt.IsZero() {
		updatedAt = cat.UpdatedAt.UTC().Format(time.RFC3339)
	}

	tiers := make(map[string]AgentCatalogTier, len(cat.Tiers))
	for tierName, tierDef := range cat.Tiers {
		surfaces := make(map[string]string, len(tierDef.Surfaces))
		for k, v := range tierDef.Surfaces {
			surfaces[k] = v
		}
		tiers[tierName] = AgentCatalogTier{
			Description: tierDef.Description,
			Surfaces:    surfaces,
		}
	}

	models := cat.Models
	if models == nil {
		models = []agent.ModelEntryYAML{}
	}

	data, err := json.Marshal(AgentCatalogResponse{
		Source:    source,
		Path:      pathOut,
		UpdatedAt: updatedAt,
		Tiers:     tiers,
		Models:    models,
	})
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("{}")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpAgentCapabilities(workingDir, harnessName string) mcpToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if harnessName == "" {
		svc, err := agent.NewServiceFromWorkDir(workingDir)
		if err != nil {
			return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
		}
		infos, err := svc.ListHarnesses(ctx)
		if err != nil {
			return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
		}
		for _, h := range infos {
			if h.Available {
				harnessName = h.Name
				break
			}
		}
	}

	if harnessName == "" {
		return mcpToolResult{Content: []mcpContent{mcpText("harness required: no harness specified and no available harness found")}, IsError: true}
	}

	caps, err := agent.CapabilitiesViaService(ctx, workingDir, harnessName)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}

	data, err := json.Marshal(caps)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("{}")}}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}
