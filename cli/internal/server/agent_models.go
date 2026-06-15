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
	Provider     string               `json:"provider"`
	Type         string               `json:"type"`
	IsDefault    bool                 `json:"is_default"`
	DefaultModel string               `json:"default_model,omitempty"`
	Models       []agentlib.ModelInfo `json:"models"`
}

// handleAgentModels serves GET /api/agent/models.
// Query params:
//   - provider: filter by provider name (default: configured default)
//   - all: if "true", return models for every configured provider
func (s *Server) handleAgentModels(w http.ResponseWriter, r *http.Request) {
	workDir := s.workingDirForRequest(r)
	showAll := r.URL.Query().Get("all") == "true"
	providerFilter := r.URL.Query().Get("provider")

	if handled := s.handleConfiguredAgentModels(w, workDir, providerFilter, showAll); handled {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	svc, err := agent.NewServiceFromWorkDirCtx(ctx, workDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer cleanupCurrentProcessProviderProbes()

	providers, err := svc.ListProviders(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
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

func (s *Server) mcpAgentModels(workingDir, providerName string, showAll bool) mcpToolResult {
	if result, handled := mcpConfiguredAgentModels(workingDir, providerName, showAll); handled {
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc, err := agent.NewServiceFromWorkDirCtx(ctx, workingDir)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}
	}
	defer cleanupCurrentProcessProviderProbes()

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
	if prov.Name == "" && name != "" {
		prov.Name = name
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
		return mcpToolResult{Content: []mcpContent{mcpText(`{"error":"marshal failed"}`)}, IsError: true}
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}
}

func (s *Server) handleConfiguredAgentModels(w http.ResponseWriter, workDir, providerName string, showAll bool) bool {
	rows, handled, err := configuredAgentModelRows(workDir, providerName, showAll)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return true
	}
	if !handled {
		return false
	}
	if showAll {
		writeJSON(w, http.StatusOK, rows)
		return true
	}
	if len(rows) == 0 {
		writeJSON(w, http.StatusOK, AgentModelsProvider{Models: []agentlib.ModelInfo{}})
		return true
	}
	writeJSON(w, http.StatusOK, rows[0])
	return true
}

func mcpConfiguredAgentModels(workDir, providerName string, showAll bool) (mcpToolResult, bool) {
	rows, handled, err := configuredAgentModelRows(workDir, providerName, showAll)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText(err.Error())}, IsError: true}, true
	}
	if !handled {
		return mcpToolResult{}, false
	}
	var payload any = rows
	if !showAll {
		if len(rows) == 0 {
			payload = AgentModelsProvider{Provider: providerName, Models: []agentlib.ModelInfo{}}
		} else {
			payload = rows[0]
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return mcpToolResult{Content: []mcpContent{mcpText("marshal failed")}, IsError: true}, true
	}
	return mcpToolResult{Content: []mcpContent{mcpText(string(data))}}, true
}

func configuredAgentModelRows(workDir, providerName string, showAll bool) ([]AgentModelsProvider, bool, error) {
	snapshots, ok, err := agent.ConfiguredProviderSnapshots(workDir)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		if showAll {
			return []AgentModelsProvider{}, true, nil
		}
		if providerName != "" {
			return []AgentModelsProvider{{Provider: providerName, Models: []agentlib.ModelInfo{}}}, true, nil
		}
		return nil, false, nil
	}
	rows := make([]AgentModelsProvider, 0, len(snapshots))
	for _, p := range snapshots {
		if providerName != "" && p.Name != providerName {
			continue
		}
		rows = append(rows, AgentModelsProvider{
			Provider:     p.Name,
			Type:         p.Type,
			IsDefault:    p.IsDefault,
			DefaultModel: p.DefaultModel,
			Models:       []agentlib.ModelInfo{},
		})
	}
	if !showAll && providerName == "" {
		for _, row := range rows {
			if row.IsDefault {
				return []AgentModelsProvider{row}, true, nil
			}
		}
		if len(rows) > 0 {
			return rows[:1], true, nil
		}
	}
	return rows, true, nil
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
