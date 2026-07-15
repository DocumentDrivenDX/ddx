package graphql

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphQLHasNoHypotheticalRouteDisplay(t *testing.T) {
	files := []string{
		"schema.graphql",
		"resolver.go",
		"resolver_providers.go",
		"generated.go",
		"models.go",
		filepath.Join("..", "frontend", "src", "routes", "nodes", "[nodeId]", "providers", "+page.svelte"),
	}
	forbidden := []string{
		"defaultRouteStatus",
		"DefaultRouteStatus",
		"defaultForProfile",
		"Current route for default profile",
		"ResolveRoute(",
	}
	for _, name := range files {
		body, err := os.ReadFile(name)
		require.NoError(t, err, name)
		for _, phrase := range forbidden {
			assert.NotContains(t, string(body), phrase, "%s retains hypothetical route display", name)
		}
	}

	schema, err := os.ReadFile("schema.graphql")
	require.NoError(t, err)
	assert.NotContains(t, string(schema), "providers: [Provider!]!")
	assert.NotContains(t, string(schema), "type Provider implements Node")
	assert.NotContains(t, string(schema), "provider(\n")
}
