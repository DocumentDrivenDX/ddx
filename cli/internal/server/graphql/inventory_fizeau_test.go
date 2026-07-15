package graphql

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type graphqlInventoryStub struct {
	providers     []agentlib.ProviderInfo
	models        []agentlib.ModelInfo
	harnesses     []agentlib.HarnessInfo
	providerCalls atomic.Int32
	modelCalls    atomic.Int32
	harnessCalls  atomic.Int32
	lastFilter    agentlib.ModelFilter
	listModelsFn  func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error)
}

func (s *graphqlInventoryStub) ListProviders(context.Context) ([]agentlib.ProviderInfo, error) {
	s.providerCalls.Add(1)
	return append([]agentlib.ProviderInfo(nil), s.providers...), nil
}

func (s *graphqlInventoryStub) ListModels(ctx context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	s.modelCalls.Add(1)
	s.lastFilter = filter
	if s.listModelsFn != nil {
		return s.listModelsFn(ctx, filter)
	}
	return append([]agentlib.ModelInfo(nil), s.models...), nil
}

func (s *graphqlInventoryStub) ListHarnesses(context.Context) ([]agentlib.HarnessInfo, error) {
	s.harnessCalls.Add(1)
	return append([]agentlib.HarnessInfo(nil), s.harnesses...), nil
}

func graphqlInventoryContext(t *testing.T, workDir string, svc inventoryService) context.Context {
	t.Helper()
	return withInventoryServiceFactory(context.Background(), func(_ context.Context, gotWorkDir string) (inventoryService, error) {
		assert.Equal(t, workDir, gotWorkDir)
		return svc, nil
	})
}

func TestGraphQLInventoryUsesOnlyFizeauFacts(t *testing.T) {
	workDir := t.TempDir()
	ddxDir := filepath.Join(workDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(ddxDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(`version: "1.0"
agent:
  model: ddx-model-must-not-appear
  endpoints:
    - type: openai
      base_url: http://ddx-provider-must-not-appear.invalid/v1
`), 0o600))

	svc := &graphqlInventoryStub{
		providers: []agentlib.ProviderInfo{{
			Name: "Canonical-Fizeau-Provider", Type: "future-provider", BaseURL: "https://example.invalid/v1",
			Status: "connected", IsDefault: true, IncludeByDefault: false, DefaultModel: "listed-default",
		}},
		harnesses: []agentlib.HarnessInfo{{
			Name: "opaque-fizeau-harness", Type: "subprocess", Available: false,
			AutoRoutingEligible: false, DefaultModel: "harness-default",
		}},
		models: []agentlib.ModelInfo{{
			ID: "listed-model", Provider: "Canonical-Fizeau-Provider", Available: false, AutoRoutable: true, IsDefault: true,
		}},
	}
	ctx := graphqlInventoryContext(t, workDir, svc)
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}

	providers, err := r.ProviderStatuses(ctx)
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, "Canonical-Fizeau-Provider", providers[0].Name)
	assert.Equal(t, "future-provider", providers[0].ProviderType)
	assert.Equal(t, "listed-default", providers[0].Model)
	assert.True(t, providers[0].IsDefault)
	assert.False(t, providers[0].AutoRoutingEligible)
	assert.True(t, providers[0].Reachable)

	harnesses, err := r.HarnessStatuses(ctx)
	require.NoError(t, err)
	require.Len(t, harnesses, 1)
	assert.Equal(t, "opaque-fizeau-harness", harnesses[0].Name)
	assert.Equal(t, "harness-default", harnesses[0].Model)
	assert.False(t, harnesses[0].Reachable)
	assert.False(t, harnesses[0].AutoRoutingEligible)

	models, err := r.ProviderModels(ctx, "canonical-fizeau-provider", ProviderKindEndpoint)
	require.NoError(t, err)
	assert.Equal(t, "Canonical-Fizeau-Provider", models.Name, "canonical Fizeau listing identity must survive")
	require.Len(t, models.Models, 1)
	assert.Equal(t, "listed-model", models.Models[0].ID)
	assert.False(t, models.Models[0].Available)
	assert.True(t, models.Models[0].AutoRoutable)
	assert.Equal(t, "Canonical-Fizeau-Provider", svc.lastFilter.Provider)
	assert.NotEqual(t, "ddx-provider-must-not-appear", models.Name)
}
