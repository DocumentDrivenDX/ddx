package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScrubServerManagedWorkerProcessEnv(t *testing.T) {
	for _, key := range []string{
		"DDX_PROJECT_ROOT",
		"DDX_AGENT_NAME",
		"DDX_SERVER_MANAGED_WORKER_ID",
		"DDX_WORKER_ID",
	} {
		t.Setenv(key, "worker-owned")
	}
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	scrubServerManagedWorkerProcessEnv()

	for _, key := range []string{
		"DDX_PROJECT_ROOT",
		"DDX_AGENT_NAME",
		"DDX_SERVER_MANAGED_WORKER_ID",
		"DDX_WORKER_ID",
	} {
		_, ok := os.LookupEnv(key)
		require.Falsef(t, ok, "%s should be removed from the worker parent process", key)
	}
	require.Equal(t, "1", os.Getenv("DDX_DISABLE_UPDATE_CHECK"))
}

func TestProbeLocalServerHealthAcceptsSelfSignedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	require.NoError(t, probeLocalServerHealth(context.Background(), server.URL))
}
