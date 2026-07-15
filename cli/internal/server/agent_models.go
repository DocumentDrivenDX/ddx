package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/easel/fizeau"
)

// AgentModelsProvider is one provider entry returned by GET /api/agent/models.
type AgentModelsProvider struct {
	Provider            string               `json:"provider"`
	Type                string               `json:"type"`
	IsDefault           bool                 `json:"is_default"`
	AutoRoutingEligible bool                 `json:"auto_routing_eligible"`
	DefaultModel        string               `json:"default_model,omitempty"`
	Models              []agentlib.ModelInfo `json:"models"`
}

// handleAgentModels serves GET /api/agent/models.
// Query params:
//   - provider: filter by provider name (default: configured default)
//   - all: if "true", return models for every configured provider
func (s *Server) handleAgentModels(w http.ResponseWriter, r *http.Request) {
	workDir := s.workingDirForRequest(r)
	showAll := r.URL.Query().Get("all") == "true"
	providerFilter := r.URL.Query().Get("provider")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	svc, err := inventoryServiceForRequest(ctx, workDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	providers, err := svc.ListProviders(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if showAll {
		result := make([]AgentModelsProvider, 0, len(providers))
		for _, p := range providers {
			models, err := svc.ListModels(ctx, agentlib.ModelFilter{Provider: p.Name})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if models == nil {
				models = []agentlib.ModelInfo{}
			}
			result = append(result, AgentModelsProvider{
				Provider:            p.Name,
				Type:                p.Type,
				IsDefault:           p.IsDefault,
				AutoRoutingEligible: p.IncludeByDefault,
				DefaultModel:        p.DefaultModel,
				Models:              models,
			})
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Single provider: use the explicit filter or the default reported by
	// Fizeau. Never promote the first row to a synthetic default.
	name := providerFilter
	if name == "" {
		for _, p := range providers {
			if p.IsDefault {
				name = p.Name
				break
			}
		}
	}
	var prov agentlib.ProviderInfo
	for _, p := range providers {
		if p.Name == name {
			prov = p
			break
		}
	}
	if prov.Name == "" {
		label := name
		if label == "" {
			label = "default"
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found: " + label})
		return
	}

	models, err := svc.ListModels(ctx, agentlib.ModelFilter{Provider: prov.Name})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if models == nil {
		models = []agentlib.ModelInfo{}
	}
	writeJSON(w, http.StatusOK, AgentModelsProvider{
		Provider:            prov.Name,
		Type:                prov.Type,
		IsDefault:           prov.IsDefault,
		AutoRoutingEligible: prov.IncludeByDefault,
		DefaultModel:        prov.DefaultModel,
		Models:              models,
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

func (s *Server) mcpAgentModels(requestCtx context.Context, workingDir, providerName string, showAll bool) mcpToolResult {
	if requestCtx == nil {
		requestCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(requestCtx, 10*time.Second)
	defer cancel()

	svc, err := inventoryServiceForRequest(ctx, workingDir)
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
			models, err := svc.ListModels(ctx, agentlib.ModelFilter{Provider: p.Name})
			if err != nil {
				return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
			}
			if models == nil {
				models = []agentlib.ModelInfo{}
			}
			result = append(result, AgentModelsProvider{
				Provider:            p.Name,
				Type:                p.Type,
				IsDefault:           p.IsDefault,
				AutoRoutingEligible: p.IncludeByDefault,
				DefaultModel:        p.DefaultModel,
				Models:              models,
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
	var prov agentlib.ProviderInfo
	for _, p := range providers {
		if p.Name == name {
			prov = p
			break
		}
	}
	if prov.Name == "" {
		label := name
		if label == "" {
			label = "default"
		}
		return mcpToolResult{Content: []mcpContent{mcpText("provider not found: " + label)}, IsError: true}
	}

	models, err := svc.ListModels(ctx, agentlib.ModelFilter{Provider: prov.Name})
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	if models == nil {
		models = []agentlib.ModelInfo{}
	}
	data, err := json.Marshal(AgentModelsProvider{
		Provider:            prov.Name,
		Type:                prov.Type,
		IsDefault:           prov.IsDefault,
		AutoRoutingEligible: prov.IncludeByDefault,
		DefaultModel:        prov.DefaultModel,
		Models:              models,
	})
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(`{"error":"marshal failed"}`)}, IsError: true}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) mcpAgentCapabilities(workingDir, harness string) mcpToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if harness == "" {
		return mcpToolResult{Content: []mcpContent{mcpText(`{"error":"harness required: no harness specified and no available harness found"}`)}, IsError: true}
	}

	caps, err := agent.CapabilitiesViaService(ctx, workingDir, harness)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	data, err := json.Marshal(caps)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(`{"error":"marshal failed"}`)}, IsError: true}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}
