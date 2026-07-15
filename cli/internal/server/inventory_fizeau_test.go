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

func writeGenericDDXInventoryConfig(t *testing.T, workDir string) {
	t.Helper()
	ddxDir := filepath.Join(workDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
agent:
  timeout_ms: 300000
`), 0o600))
}

func TestRESTAndMCPInventoryUseOnlyFizeauListings(t *testing.T) {
	workDir := t.TempDir()
	writeGenericDDXInventoryConfig(t, workDir)
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

func TestProviderDisplayNameUsesFizeauInventoryVerbatim(t *testing.T) {
	tests := []struct {
		name       string
		fizeauName string
	}{
		{name: "known", fizeauName: "claude"},
		{name: "unknown", fizeauName: "future-harness-v99"},
		{name: "mixed_case", fizeauName: "MiXeD-Harness"},
		{name: "whitespace", fizeauName: "  harness with spaces\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			svc := &inventoryServiceStub{
				harnesses: []agentlib.HarnessInfo{{
					Name: tt.fizeauName, Type: "subprocess", Available: true,
				}},
			}
			ctx := inventoryTestContext(t, workDir, svc)
			srv := &Server{WorkingDir: workDir}

			// The dashboard frontend consumes these REST DTOs. MCP invokes the
			// same production summary/detail builders through its own transport.
			listReq := httptest.NewRequest(http.MethodGet, "/api/providers", nil).WithContext(ctx)
			listW := httptest.NewRecorder()
			srv.handleListProviders(listW, listReq)
			require.Equal(t, http.StatusOK, listW.Code, listW.Body.String())
			var restSummaries []ProviderSummary
			require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &restSummaries))
			require.Len(t, restSummaries, 1)

			showReq := httptest.NewRequest(http.MethodGet, "/api/providers/harness", nil).WithContext(ctx)
			showReq.SetPathValue("harness", tt.fizeauName)
			showW := httptest.NewRecorder()
			srv.handleShowProvider(showW, showReq)
			require.Equal(t, http.StatusOK, showW.Code, showW.Body.String())
			var restDetail ProviderDetail
			require.NoError(t, json.Unmarshal(showW.Body.Bytes(), &restDetail))

			mcpList := srv.mcpProviderList(ctx)
			require.False(t, mcpList.IsError)
			var mcpSummaries []ProviderSummary
			require.NoError(t, json.Unmarshal([]byte(mcpList.Content[0].Text), &mcpSummaries))
			require.Len(t, mcpSummaries, 1)

			mcpShow := srv.mcpProviderShow(ctx, tt.fizeauName)
			require.False(t, mcpShow.IsError)
			var mcpDetail ProviderDetail
			require.NoError(t, json.Unmarshal([]byte(mcpShow.Content[0].Text), &mcpDetail))

			assert.Equal(t, tt.fizeauName, restSummaries[0].DisplayName)
			assert.Equal(t, tt.fizeauName, restDetail.DisplayName)
			assert.Equal(t, tt.fizeauName, mcpSummaries[0].DisplayName)
			assert.Equal(t, tt.fizeauName, mcpDetail.DisplayName)
			assert.Equal(t, tt.fizeauName, restSummaries[0].Harness)
			assert.Equal(t, tt.fizeauName, restDetail.Harness)
			assert.Equal(t, tt.fizeauName, mcpSummaries[0].Harness)
			assert.Equal(t, tt.fizeauName, mcpDetail.Harness)
			assert.Equal(t, 4, svc.listHarnesses)
			assert.Equal(t, 4, svc.routeStatusCalls)
		})
	}
}
