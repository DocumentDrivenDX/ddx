package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServiceFromWorkDirPassesUnreachableEndpointsToService asserts that
// NewServiceFromWorkDir no longer filters configured endpoints by /models
// reachability. Both reachable and unreachable endpoints must be present in
// the service config so the agent service can surface them with appropriate
// status rather than DDx silently removing them.
func TestNewServiceFromWorkDirPassesUnreachableEndpointsToService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(dead.Close)
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"Qwen3.6-35B-A3B-4bit"}]}`))
	}))
	t.Cleanup(live.Close)

	workDir := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, workDir)
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ddxroot.DirName, "config.yaml"), []byte(fmt.Sprintf(`version: "1.0"
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

	sc, err := serviceConfigFromDDxEndpointsNoFilter(workDir)
	require.NoError(t, err)
	require.NotNil(t, sc)

	// Both endpoints must be present: DDx does not filter by reachability.
	names := sc.ProviderNames()
	require.Len(t, names, 2)
	var hasLmstudio, hasOmlx bool
	for _, n := range names {
		if strings.Contains(n, "lmstudio") {
			hasLmstudio = true
			entry, ok := sc.Provider(n)
			require.True(t, ok)
			assert.True(t, entry.IncludeByDefault, "configured local endpoint must participate in automatic routing")
			assert.True(t, entry.IncludeByDefaultSet)
			assert.Equal(t, "fixed", string(entry.Billing))
		}
		if strings.Contains(n, "omlx") {
			hasOmlx = true
			entry, ok := sc.Provider(n)
			require.True(t, ok)
			assert.True(t, entry.IncludeByDefault, "configured local endpoint must participate in automatic routing")
			assert.True(t, entry.IncludeByDefaultSet)
			assert.Equal(t, "fixed", string(entry.Billing))
		}
	}
	assert.True(t, hasLmstudio, "unreachable lmstudio endpoint must be passed to service, not removed by DDx")
	assert.True(t, hasOmlx, "live omlx endpoint must be present")
}

func TestNewServiceFromWorkDirUsesInheritedGlobalEndpoints(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	globalDir := filepath.Join(homeDir, ddxroot.DirName)
	require.NoError(t, os.MkdirAll(globalDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`version: "1.0"
agent:
  endpoints:
    - type: lmstudio
      host: 127.0.0.1
      port: 1234
      api_key: lmstudio
`), 0o644))

	workDir := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, workDir)
	require.NoError(t, os.WriteFile(filepath.Join(workDir, ddxroot.DirName, "config.yaml"), []byte(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/project/repo
    branch: main
`), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc, err := NewServiceFromWorkDirCtx(ctx, workDir)
	require.NoError(t, err)

	providers, err := svc.ListProviders(ctx)
	require.NoError(t, err)
	assert.Contains(t, providerNames(providers), "lmstudio-127-0-0-1-1234")
}

func providerNames(providers []agentlib.ProviderInfo) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	return names
}
