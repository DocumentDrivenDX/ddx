package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	sc, err := serviceConfigFromDDxEndpoints(workDir)
	require.NoError(t, err)
	require.NotNil(t, sc)

	// Both endpoints must be present: DDx does not filter by reachability.
	names := sc.ProviderNames()
	require.Len(t, names, 2)
	var hasLmstudio, hasOmlx bool
	for _, n := range names {
		if strings.Contains(n, "lmstudio") {
			hasLmstudio = true
		}
		if strings.Contains(n, "omlx") {
			hasOmlx = true
		}
	}
	assert.True(t, hasLmstudio, "unreachable lmstudio endpoint must be passed to service, not removed by DDx")
	assert.True(t, hasOmlx, "live omlx endpoint must be present")
}
