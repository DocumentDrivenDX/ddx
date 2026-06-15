package cmd

import (
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
