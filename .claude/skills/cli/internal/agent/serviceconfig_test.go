package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceFromWorkDirUsesDDxEndpointConfigAndSkipsDeadEndpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(dead.Close)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"Qwen3.6-35B-A3B-4bit"}]}`))
	}))
	t.Cleanup(live.Close)

	workDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ".ddx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ".ddx", "config.yaml"), []byte(fmt.Sprintf(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/easel/ddx-library
    branch: main
agent:
  endpoints:
    - type: lmstudio
      base_url: %s/v1
    - type: omlx
      base_url: %s/v1
`, dead.URL, live.URL)), 0o644))

	svc, err := NewServiceFromWorkDir(workDir)
	require.NoError(t, err)

	providers, err := svc.ListProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Contains(t, providers[0].Name, "omlx")
	assert.NotContains(t, providers[0].Name, "lmstudio")

	dec, err := svc.ResolveRoute(context.Background(), agentlib.RouteRequest{
		Harness: "agent",
		Profile: "cheap",
		Model:   "qwen/qwen3.6",
	})
	require.NoError(t, err)
	assert.Equal(t, providers[0].Name, dec.Provider)
	assert.Equal(t, "Qwen3.6-35B-A3B-4bit", dec.Model)
}
