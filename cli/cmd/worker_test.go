package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerCLISetStatusRestartReconcile(t *testing.T) {
	projectRoot := t.TempDir()

	// Track which endpoints were called
	var setCalled, restartCalled, reconcileCalled bool
	var setPayload map[string]interface{}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agent/workers":
			// status subcommand
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "worker-test-001", "state": "running", "project_root": projectRoot, "started_at": "2026-01-01T00:00:00Z"},
			})

		case r.Method == http.MethodPut && r.URL.Path == "/api/agent/workers/desired":
			// set subcommand
			setCalled = true
			_ = json.NewDecoder(r.Body).Decode(&setPayload)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   2,
				"restart_enabled": true,
				"status":          "saved",
			})

		case r.Method == http.MethodPost && r.URL.Path == "/api/agent/workers/worker-test-001/restart":
			// restart subcommand
			restartCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":     "worker-test-002",
				"old_id": "worker-test-001",
				"status": "running",
			})

		case r.Method == http.MethodPost && r.URL.Path == "/api/agent/workers/reconcile":
			// reconcile subcommand
			reconcileCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"started":   []string{"worker-test-003"},
				"stopped":   nil,
				"restarted": nil,
			})

		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DDX_SERVER_URL", srv.URL)

	factory := NewCommandFactory(projectRoot)
	root := factory.NewRootCommand()

	// Test: status
	out, err := executeCommand(root, "worker", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "worker-test-001")

	// Test: set
	out, err = executeCommand(root, "worker", "set", "--project", projectRoot, "--count", "2", "--restart")
	require.NoError(t, err)
	assert.True(t, setCalled, "expected PUT /api/agent/workers/desired to be called")
	assert.Contains(t, out, "saved")

	// Test: restart
	out, err = executeCommand(root, "worker", "restart", "worker-test-001")
	require.NoError(t, err)
	assert.True(t, restartCalled, "expected POST /api/agent/workers/worker-test-001/restart to be called")
	assert.Contains(t, out, "worker-test-002")

	// Test: reconcile
	out, err = executeCommand(root, "worker", "reconcile", "--project", projectRoot)
	require.NoError(t, err)
	assert.True(t, reconcileCalled, "expected POST /api/agent/workers/reconcile to be called")
	assert.Contains(t, out, "worker-test-003")
}
