package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoDDXProviderConfigSynthesis(t *testing.T) {
	files := []string{
		filepath.Join("..", "agent", "serviceconfig.go"),
		"agent_models.go",
		"providers.go",
		filepath.Join("graphql", "resolver_providers.go"),
		filepath.Join("graphql", "resolver_provider_models.go"),
		filepath.Join("graphql", "resolver.go"),
	}
	forbidden := []string{
		"ConfiguredProviderSnapshots",
		"configuredAgentModelRows",
		"serviceConfigFromDDxEndpointsNoFilter",
		"endpointServiceConfig",
		"endpointProviderEntry",
		"inferEndpointProviderType",
		"defaultProfilesForEndpoint",
		"ResolveRoute(",
		"NewStatusProbeServiceFromWorkDir",
	}
	for _, name := range files {
		body, err := os.ReadFile(name)
		require.NoError(t, err, name)
		for _, symbol := range forbidden {
			assert.NotContains(t, string(body), symbol, "%s retains DDx inventory synthesis or route prediction", name)
		}
	}

	resolver, err := os.ReadFile(filepath.Join("graphql", "resolver.go"))
	require.NoError(t, err)
	assert.NotContains(t, string(resolver), "func (r *queryResolver) Providers(")
	assert.NotContains(t, string(resolver), "func (r *queryResolver) Provider(")
}
