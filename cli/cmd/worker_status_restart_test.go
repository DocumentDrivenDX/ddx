package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerStatusAfterServiceRestart_ReportsExactDesiredWorkers(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	addrDir := filepath.Join(xdg, "ddx")
	require.NoError(t, os.MkdirAll(addrDir, 0o755))
	addrPath := filepath.Join(addrDir, "server.addr")

	staleAddr := map[string]any{
		"url": "https://127.0.0.1:1",
		"pid": 999999,
	}
	staleData, err := json.Marshal(staleAddr)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(addrPath, staleData, 0o600))

	reconcileRequests := 0
	restarted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agent/workers":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":           "worker-project-a",
					"state":        "running",
					"project_root": projectA,
					"started_at":   time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
				{
					"id":           "worker-project-b",
					"state":        "running",
					"project_root": projectB,
					"started_at":   time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/agent/workers/reconcile":
			reconcileRequests++
			http.Error(w, "status must not reconcile", http.StatusTeapot)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(restarted.Close)

	freshAddr := map[string]any{
		"url": restarted.URL,
		"pid": os.Getpid(),
	}
	freshData, err := json.Marshal(freshAddr)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(addrPath, freshData, 0o600))
	require.Equal(t, restarted.URL, serverpkg.ReadServerAddr())

	factory := NewCommandFactory(projectA)

	for _, projectRoot := range []string{projectA, projectB} {
		out, err := executeCommand(factory.NewRootCommand(), "worker", "status", "--project", projectRoot, "--json")
		require.NoError(t, err)

		var workers []workerRecord
		require.NoError(t, json.Unmarshal([]byte(out), &workers))
		require.Len(t, workers, 1)
		assert.Equal(t, projectRoot, workers[0].ProjectRoot)
		assert.Equal(t, "running", workers[0].State)
	}

	assert.Zero(t, reconcileRequests, "worker status must not call reconcile after restart")
}
