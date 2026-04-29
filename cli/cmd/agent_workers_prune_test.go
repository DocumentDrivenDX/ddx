package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workersPruneStub is a minimal server stub for `ddx agent workers prune`
// tests: it serves POST /api/agent/workers/prune with a canned result list.
type workersPruneStub struct {
	results  []map[string]any
	called   bool
	queryRaw string
}

func newWorkersPruneStub(t *testing.T, results []map[string]any) (*workersPruneStub, *httptest.Server) {
	t.Helper()
	stub := &workersPruneStub{results: results}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/agent/workers/prune", func(w http.ResponseWriter, r *http.Request) {
		stub.called = true
		stub.queryRaw = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stub.results)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return stub, srv
}

// TestAgentWorkersPruneNoStale verifies that prune prints a friendly message
// when the server returns an empty list.
func TestAgentWorkersPruneNoStale(t *testing.T) {
	stub, srv := newWorkersPruneStub(t, []map[string]any{})
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "prune")
	require.NoError(t, err)
	assert.True(t, stub.called, "prune must POST to the server endpoint")
	assert.Contains(t, out, "no stale workers found")
}

// TestAgentWorkersPruneTableOutput verifies that pruned entries are rendered
// as a table (ID, BEAD, HARNESS, AGE, REASON).
func TestAgentWorkersPruneTableOutput(t *testing.T) {
	results := []map[string]any{
		{
			"id":      "worker-old-1",
			"bead_id": "ddx-abc123",
			"harness": "claude",
			"age":     "10h0m0s",
			"reason":  "pid=12345 not alive",
		},
	}
	stub, srv := newWorkersPruneStub(t, results)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "prune")
	require.NoError(t, err)
	assert.True(t, stub.called)
	assert.Contains(t, out, "worker-old-1")
	assert.Contains(t, out, "ddx-abc123")
	assert.Contains(t, out, "claude")
	assert.Contains(t, out, "not alive")
}

// TestAgentWorkersPruneJSONOutput verifies that --json emits raw JSON from
// the server's prune response.
func TestAgentWorkersPruneJSONOutput(t *testing.T) {
	results := []map[string]any{
		{
			"id":      "worker-old-2",
			"bead_id": "ddx-xyz789",
			"harness": "codex",
			"age":     "24h0m0s",
			"reason":  "goroutine not running (server restarted?)",
		},
	}
	stub, srv := newWorkersPruneStub(t, results)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "prune", "--json")
	require.NoError(t, err)
	assert.True(t, stub.called)

	var decoded []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &decoded))
	require.Len(t, decoded, 1)
	assert.Equal(t, "worker-old-2", decoded[0]["id"])
	assert.Equal(t, "ddx-xyz789", decoded[0]["bead_id"])
}

// TestAgentWorkersPruneMaxAge verifies that --max-age passes the query
// parameter to the server endpoint.
func TestAgentWorkersPruneMaxAge(t *testing.T) {
	stub, srv := newWorkersPruneStub(t, []map[string]any{})
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "prune", "--max-age", "48h")
	require.NoError(t, err)
	assert.True(t, stub.called)
	assert.Contains(t, stub.queryRaw, "max_age=")
}

// TestAgentWorkersJSONIncludesPIDAlive verifies that `ddx agent workers --json`
// propagates the pid_alive field so operators can see liveness at a glance
// without running prune (AC4).
func TestAgentWorkersJSONIncludesPIDAlive(t *testing.T) {
	alive := true
	dead := false
	workers := []map[string]any{
		{
			"id":        "worker-alive",
			"kind":      "execute-loop",
			"state":     "running",
			"pid":       54321,
			"pid_alive": alive,
		},
		{
			"id":        "worker-dead",
			"kind":      "execute-loop",
			"state":     "running",
			"pid":       99999,
			"pid_alive": dead,
		},
	}
	_, srv := newWorkersStopStub(t, workers)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "--json")
	require.NoError(t, err)

	var display []struct {
		ID       string `json:"id"`
		PID      int    `json:"pid"`
		PIDAlive *bool  `json:"pid_alive"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &display))
	require.Len(t, display, 2)

	byID := map[string]*bool{}
	for _, d := range display {
		if d.PIDAlive != nil {
			v := *d.PIDAlive
			byID[d.ID] = &v
		}
	}
	require.NotNil(t, byID["worker-alive"], "pid_alive must be present for worker-alive")
	assert.True(t, *byID["worker-alive"])
	require.NotNil(t, byID["worker-dead"], "pid_alive must be present for worker-dead")
	assert.False(t, *byID["worker-dead"])
}
