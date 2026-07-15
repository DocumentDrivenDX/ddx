package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inventoryServiceStub struct {
	providers        []agentlib.ProviderInfo
	models           []agentlib.ModelInfo
	harnesses        []agentlib.HarnessInfo
	routeStatus      *agentlib.RouteStatusReport
	listProviders    int
	listModels       int
	listHarnesses    int
	routeStatusCalls int
}

func (s *inventoryServiceStub) ListProviders(context.Context) ([]agentlib.ProviderInfo, error) {
	s.listProviders++
	return append([]agentlib.ProviderInfo(nil), s.providers...), nil
}

func (s *inventoryServiceStub) ListModels(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	s.listModels++
	return append([]agentlib.ModelInfo(nil), s.models...), nil
}

func (s *inventoryServiceStub) ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error) {
	s.listHarnesses++
	return append([]agentlib.HarnessInfo(nil), s.harnesses...), nil
}

func (s *inventoryServiceStub) RouteStatus(context.Context) (*agentlib.RouteStatusReport, error) {
	s.routeStatusCalls++
	return s.routeStatus, nil
}

func inventoryTestContext(t *testing.T, workDir string, svc *inventoryServiceStub) context.Context {
	t.Helper()
	return withInventoryServiceFactory(context.Background(), func(_ context.Context, gotWorkDir string) (inventoryService, error) {
		assert.Equal(t, workDir, gotWorkDir)
		return svc, nil
	})
}

func writeContradictoryDDXInventoryConfig(t *testing.T, workDir string) {
	t.Helper()
	ddxDir := filepath.Join(workDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
agent:
  model: ddx-model-must-not-appear
  endpoints:
    - type: openai
      base_url: http://ddx-provider-must-not-appear.invalid/v1
      api_key: ddx-credential-must-not-load
`), 0o600))
}

func TestRESTAndMCPInventoryUseOnlyFizeauListings(t *testing.T) {
	workDir := t.TempDir()
	writeContradictoryDDXInventoryConfig(t, workDir)
	svc := &inventoryServiceStub{
		providers: []agentlib.ProviderInfo{{
			Name: "Fizeau-Only-Provider", Type: "future-provider", IsDefault: true,
			IncludeByDefault: false, DefaultModel: "fizeau-default-model", Status: "connected",
		}},
		models: []agentlib.ModelInfo{{
			ID: "fizeau-listed-model", Provider: "Fizeau-Only-Provider", Available: false, IsDefault: true,
		}},
		harnesses: []agentlib.HarnessInfo{{
			Name: "future-harness-v99", Type: "subprocess", Available: true, AutoRoutingEligible: false,
		}},
		routeStatus: &agentlib.RouteStatusReport{Routes: []agentlib.RouteStatusEntry{{
			Candidates: []agentlib.RouteCandidateStatus{
				{Provider: "future-harness-v99", RecentLatencyMS: 10, ProviderReliabilityRate: 0.5},
				{Provider: "future-harness-v99", RecentLatencyMS: 20, ProviderReliabilityRate: 0.75},
				{Provider: "future-harness-v99", RecentLatencyMS: 30, ProviderReliabilityRate: 1},
				{Provider: "route-status-only-ghost", RecentLatencyMS: 1, ProviderReliabilityRate: 1},
			},
		}}},
	}
	ctx := inventoryTestContext(t, workDir, svc)
	srv := &Server{WorkingDir: workDir}

	req := httptest.NewRequest(http.MethodGet, "/api/agent/models?all=true", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	srv.handleAgentModels(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var restRows []AgentModelsProvider
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &restRows))
	require.Len(t, restRows, 1)
	assert.Equal(t, "Fizeau-Only-Provider", restRows[0].Provider)
	assert.True(t, restRows[0].IsDefault)
	assert.False(t, restRows[0].AutoRoutingEligible)
	require.Len(t, restRows[0].Models, 1)
	assert.Equal(t, "fizeau-listed-model", restRows[0].Models[0].ID)
	assert.False(t, restRows[0].Models[0].Available)
	assert.NotContains(t, w.Body.String(), "ddx-provider-must-not-appear")
	assert.NotContains(t, w.Body.String(), "ddx-model-must-not-appear")

	mcpRows := srv.mcpAgentModels(ctx, workDir, "", true)
	require.False(t, mcpRows.IsError)
	require.Len(t, mcpRows.Content, 1)
	var decodedMCP []AgentModelsProvider
	require.NoError(t, json.Unmarshal([]byte(mcpRows.Content[0].Text), &decodedMCP))
	require.Len(t, decodedMCP, 1)
	assert.Equal(t, "Fizeau-Only-Provider", decodedMCP[0].Provider)
	assert.Equal(t, "fizeau-listed-model", decodedMCP[0].Models[0].ID)
	assert.False(t, decodedMCP[0].AutoRoutingEligible)

	missingReq := httptest.NewRequest(http.MethodGet, "/api/agent/models?provider=ddx-provider-must-not-appear", nil).WithContext(ctx)
	missingW := httptest.NewRecorder()
	srv.handleAgentModels(missingW, missingReq)
	assert.Equal(t, http.StatusNotFound, missingW.Code)
	missingMCP := srv.mcpAgentModels(ctx, workDir, "ddx-provider-must-not-appear", false)
	assert.True(t, missingMCP.IsError)
	assert.Contains(t, missingMCP.Content[0].Text, "provider not found")

	providerReq := httptest.NewRequest(http.MethodGet, "/api/providers", nil).WithContext(ctx)
	providerW := httptest.NewRecorder()
	srv.handleListProviders(providerW, providerReq)
	require.Equal(t, http.StatusOK, providerW.Code, providerW.Body.String())
	var harnessRows []ProviderSummary
	require.NoError(t, json.Unmarshal(providerW.Body.Bytes(), &harnessRows))
	require.Len(t, harnessRows, 1)
	assert.Equal(t, "future-harness-v99", harnessRows[0].Harness)
	assert.False(t, harnessRows[0].AutoRoutingEligible)
	assert.Equal(t, 0.75, harnessRows[0].RecentSuccessRate)
	assert.Equal(t, 20, harnessRows[0].RecentLatencyP50MS)
	assert.Equal(t, 1, svc.routeStatusCalls, "factual RouteStatus may enrich the listed row once")
}

func TestArbitraryFizeauHarnessNamesRemainPresentationOnly(t *testing.T) {
	workDir := t.TempDir()
	svc := &inventoryServiceStub{
		harnesses: []agentlib.HarnessInfo{{
			Name: "opaque-harness-from-the-future", Type: "quantum-shell", Available: true,
			AutoRoutingEligible: false, DefaultModel: "opaque-default",
		}},
	}
	ctx := inventoryTestContext(t, workDir, svc)
	srv := &Server{WorkingDir: workDir}

	result := srv.mcpProviderList(ctx)
	require.False(t, result.IsError)
	var rows []ProviderSummary
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "opaque-harness-from-the-future", rows[0].Harness)
	assert.False(t, rows[0].AutoRoutingEligible)
	assert.NotEmpty(t, rows[0].DisplayName, "existing presentation mapping may format the label")
	assert.Equal(t, 1, svc.listHarnesses)
	assert.Equal(t, 1, svc.routeStatusCalls)
}
