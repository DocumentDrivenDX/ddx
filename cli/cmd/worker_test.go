package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerCLISetStatusRestartReconcile(t *testing.T) {
	projectRoot := t.TempDir()

	// Track which endpoints were called.
	var restartCalled bool
	var desiredRequests, reconcileRequests int
	var setPayload map[string]interface{}
	var desiredCounts []float64
	var reconcileProjects []string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agent/workers":
			// status subcommand
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "worker-test-001", "state": "running", "project_root": projectRoot, "started_at": "2026-01-01T00:00:00Z"},
			})

		case r.Method == http.MethodPut && r.URL.Path == "/api/agent/workers/desired":
			// set/start subcommands
			desiredRequests++
			_ = json.NewDecoder(r.Body).Decode(&setPayload)
			desiredCounts = append(desiredCounts, setPayload["desired_count"].(float64))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   setPayload["desired_count"],
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
			// set/start/reconcile subcommands
			reconcileRequests++
			reconcileProjects = append(reconcileProjects, r.URL.Query().Get("project"))
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
	assert.Equal(t, 1, desiredRequests, "expected PUT /api/agent/workers/desired to be called")
	assert.Equal(t, 1, reconcileRequests, "worker set must reconcile after saving desired state")
	assert.Equal(t, []float64{2}, desiredCounts)
	assert.Equal(t, []string{projectRoot}, reconcileProjects)
	assert.Contains(t, out, "saved")
	assert.Contains(t, out, "worker-test-003")

	// Test: start
	out, err = executeCommand(root, "worker", "start", "--project", projectRoot)
	require.NoError(t, err)
	assert.Equal(t, 2, desiredRequests, "expected start to save desired state")
	assert.Equal(t, 2, reconcileRequests, "worker start must reconcile after saving desired state")
	assert.Equal(t, []float64{2, 1}, desiredCounts)
	assert.Equal(t, []string{projectRoot, projectRoot}, reconcileProjects)
	assert.Contains(t, out, "saved")
	assert.Contains(t, out, "worker-test-003")

	// Test: restart
	out, err = executeCommand(root, "worker", "restart", "worker-test-001")
	require.NoError(t, err)
	assert.True(t, restartCalled, "expected POST /api/agent/workers/worker-test-001/restart to be called")
	assert.Contains(t, out, "worker-test-002")

	// Test: reconcile
	out, err = executeCommand(root, "worker", "reconcile", "--project", projectRoot)
	require.NoError(t, err)
	assert.Equal(t, 3, reconcileRequests, "expected POST /api/agent/workers/reconcile to be called")
	assert.Contains(t, out, "worker-test-003")
}

func TestWorkerCLISetStartSuppressEmptyReconcileOutput(t *testing.T) {
	projectRoot := t.TempDir()

	var desiredRequests, reconcileRequests int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/agent/workers/desired":
			desiredRequests++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   1,
				"restart_enabled": true,
				"status":          "saved",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/agent/workers/reconcile":
			reconcileRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DDX_SERVER_URL", srv.URL)

	factory := NewCommandFactory(projectRoot)

	out, err := executeCommand(factory.NewRootCommand(), "worker", "set", "--project", projectRoot, "--count", "1", "--restart")
	require.NoError(t, err)
	assert.Contains(t, out, "saved")
	assert.NotContains(t, out, "\n{}\n")

	out, err = executeCommand(factory.NewRootCommand(), "worker", "start", "--project", projectRoot)
	require.NoError(t, err)
	assert.Contains(t, out, "saved")
	assert.NotContains(t, out, "\n{}\n")

	assert.Equal(t, 2, desiredRequests)
	assert.Equal(t, 2, reconcileRequests)
}

func TestWorkerSet_RetriesDuringRestartListenerReadiness(t *testing.T) {
	projectRoot := t.TempDir()
	addr := reserveLocalTCPAddr(t)
	t.Setenv("DDX_SERVER_URL", "https://"+addr)
	withWorkerReadinessRetryTiming(t, 2*time.Second, 10*time.Millisecond)

	var desiredRequests, reconcileRequests int
	serverStarted := make(chan *httptest.Server, 1)
	serverErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/agent/workers/desired":
			desiredRequests++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"project_root":    projectRoot,
				"desired_count":   1,
				"restart_enabled": true,
				"status":          "saved",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/agent/workers/reconcile":
			reconcileRequests++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}\n"))
		default:
			http.NotFound(w, r)
		}
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			serverErr <- err
			return
		}
		srv := httptest.NewUnstartedServer(handler)
		srv.Listener = ln
		srv.StartTLS()
		serverStarted <- srv
	}()
	t.Cleanup(func() {
		select {
		case srv := <-serverStarted:
			srv.Close()
		case <-time.After(time.Second):
		}
	})

	factory := NewCommandFactory(projectRoot)
	out, err := executeCommand(factory.NewRootCommand(), "worker", "set", "--project", projectRoot, "--count", "1", "--restart")
	require.NoError(t, err)
	assert.Contains(t, out, "saved")
	assert.Equal(t, 1, desiredRequests)
	assert.Equal(t, 1, reconcileRequests)
	select {
	case err := <-serverErr:
		require.NoError(t, err)
	default:
	}
}

func TestWorkerSet_ReadinessRetryBudgetExpires(t *testing.T) {
	projectRoot := t.TempDir()
	addr := reserveLocalTCPAddr(t)
	t.Setenv("DDX_SERVER_URL", "https://"+addr)
	withWorkerReadinessRetryTiming(t, 40*time.Millisecond, 5*time.Millisecond)

	factory := NewCommandFactory(projectRoot)
	out, err := executeCommand(factory.NewRootCommand(), "worker", "set", "--project", projectRoot, "--count", "1", "--restart")
	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "local server readiness timeout after")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestWorkerStatusJSONHonorsProjectFilter(t *testing.T) {
	projectA := t.TempDir()
	projectB := t.TempDir()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/agent/workers", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "worker-project-a", "state": "running", "project_root": projectA, "started_at": "2026-01-01T00:00:00Z"},
			{"id": "worker-project-b", "state": "running", "project_root": projectB, "started_at": "2026-01-01T00:00:00Z"},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DDX_SERVER_URL", srv.URL)

	factory := NewCommandFactory(projectA)
	out, err := executeCommand(factory.NewRootCommand(), "worker", "status", "--project", projectA, "--json")
	require.NoError(t, err)
	var workers []workerRecord
	require.NoError(t, json.Unmarshal([]byte(out), &workers))
	require.Len(t, workers, 1)
	assert.Equal(t, "worker-project-a", workers[0].ID)
	assert.Equal(t, projectA, workers[0].ProjectRoot)
	assert.NotContains(t, out, "worker-project-b")
	assert.NotContains(t, out, projectB)

	textOut, err := executeCommand(factory.NewRootCommand(), "worker", "status", "--project", projectA)
	require.NoError(t, err)
	assert.Contains(t, textOut, "worker-project-a")
	assert.NotContains(t, textOut, "worker-project-b")
	assert.NotContains(t, textOut, projectB)
}

func reserveLocalTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func withWorkerReadinessRetryTiming(t *testing.T, budget, delay time.Duration) {
	t.Helper()
	oldBudget := workerServerReadinessRetryBudget
	oldDelay := workerServerReadinessRetryDelay
	workerServerReadinessRetryBudget = budget
	workerServerReadinessRetryDelay = delay
	t.Cleanup(func() {
		workerServerReadinessRetryBudget = oldBudget
		workerServerReadinessRetryDelay = oldDelay
	})
}
