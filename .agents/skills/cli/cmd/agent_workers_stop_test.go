package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workersStopStub is the minimal server stub that `ddx agent workers stop`
// talks to: it serves GET /api/agent/workers with a canned worker list and
// records the ids POSTed to /api/agent/workers/{id}/stop.
type workersStopStub struct {
	workers     []map[string]any
	stoppedIDs  []string
	stoppedLock sync.Mutex
}

func newWorkersStopStub(t *testing.T, workers []map[string]any) (*workersStopStub, *httptest.Server) {
	t.Helper()
	stub := &workersStopStub{workers: workers}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/agent/workers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stub.workers)
	})
	mux.HandleFunc("POST /api/agent/workers/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		stub.stoppedLock.Lock()
		stub.stoppedIDs = append(stub.stoppedIDs, id)
		stub.stoppedLock.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + id + `","status":"stopping"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return stub, srv
}

func (s *workersStopStub) ids() []string {
	s.stoppedLock.Lock()
	defer s.stoppedLock.Unlock()
	out := make([]string, len(s.stoppedIDs))
	copy(out, s.stoppedIDs)
	sort.Strings(out)
	return out
}

// TestAgentWorkersStopByID verifies `ddx agent workers stop <worker-id>`
// hits POST /api/agent/workers/{id}/stop on the running server.
func TestAgentWorkersStopByID(t *testing.T) {
	stub, srv := newWorkersStopStub(t, nil)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "stop", "worker-abc")
	require.NoError(t, err)
	assert.Contains(t, out, "stopping worker-abc")
	assert.Equal(t, []string{"worker-abc"}, stub.ids())
}

// TestAgentWorkersStopAllOver verifies `--all-over <duration>` fans out the
// stop POSTs to every running worker whose age exceeds the threshold, and
// leaves younger or non-running workers alone.
func TestAgentWorkersStopAllOver(t *testing.T) {
	now := time.Now()
	workers := []map[string]any{
		{
			"id":         "worker-old-running",
			"kind":       "execute-loop",
			"state":      "running",
			"started_at": now.Add(-3 * time.Hour).Format(time.RFC3339Nano),
		},
		{
			"id":         "worker-young-running",
			"kind":       "execute-loop",
			"state":      "running",
			"started_at": now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
		},
		{
			"id":         "worker-old-exited",
			"kind":       "execute-loop",
			"state":      "exited",
			"started_at": now.Add(-5 * time.Hour).Format(time.RFC3339Nano),
		},
	}
	stub, srv := newWorkersStopStub(t, workers)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "stop", "--all-over", "1h")
	require.NoError(t, err)

	// Only the old running worker should have been targeted.
	assert.Equal(t, []string{"worker-old-running"}, stub.ids(),
		"--all-over must skip young and non-running workers")
}

// TestAgentWorkersStopByBead verifies --bead targets only the worker whose
// current attempt is running that bead.
func TestAgentWorkersStopByBead(t *testing.T) {
	workers := []map[string]any{
		{
			"id":    "worker-a",
			"kind":  "execute-loop",
			"state": "running",
			"current_attempt": map[string]any{
				"attempt_id": "atm-a",
				"bead_id":    "ddx-target",
			},
		},
		{
			"id":    "worker-b",
			"kind":  "execute-loop",
			"state": "running",
			"current_attempt": map[string]any{
				"attempt_id": "atm-b",
				"bead_id":    "ddx-other",
			},
		},
	}
	stub, srv := newWorkersStopStub(t, workers)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "stop", "--bead", "ddx-target")
	require.NoError(t, err)
	assert.Equal(t, []string{"worker-a"}, stub.ids())
}

// TestAgentWorkersStopByState verifies --state targets only workers in the
// given state.
func TestAgentWorkersStopByState(t *testing.T) {
	workers := []map[string]any{
		{"id": "w-running-1", "kind": "execute-loop", "state": "running"},
		{"id": "w-running-2", "kind": "execute-loop", "state": "running"},
		{"id": "w-exited-1", "kind": "execute-loop", "state": "exited"},
	}
	stub, srv := newWorkersStopStub(t, workers)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "stop", "--state", "running")
	require.NoError(t, err)
	assert.Equal(t, []string{"w-running-1", "w-running-2"}, stub.ids())
}

// TestAgentWorkersStopRejectsMultipleModes: exactly one of <id>, --all-over,
// --state, --bead must be specified.
func TestAgentWorkersStopRejectsMultipleModes(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "stop", "worker-x", "--state", "running")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
}

// TestAgentWorkersStopRequiresATarget: no id and no filter → error.
func TestAgentWorkersStopRequiresATarget(t *testing.T) {
	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	_, err := executeCommand(root, "agent", "workers", "stop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify")
}

// TestAgentWorkersJSONIncludesPID verifies that `ddx agent workers --json`
// propagates the PID field so external tooling can target processes directly.
func TestAgentWorkersJSONIncludesPID(t *testing.T) {
	workers := []map[string]any{
		{
			"id":         "worker-pid",
			"kind":       "execute-loop",
			"state":      "running",
			"started_at": time.Now().Format(time.RFC3339Nano),
			"pid":        54321,
		},
	}
	_, srv := newWorkersStopStub(t, workers)
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	out, err := executeCommand(root, "agent", "workers", "--json")
	require.NoError(t, err)

	// Parse the JSON array and confirm PID made it through the display
	// struct. This guards against a future refactor dropping the field.
	var display []struct {
		ID  string `json:"id"`
		PID int    `json:"pid"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &display))
	require.Len(t, display, 1)
	assert.Equal(t, "worker-pid", display[0].ID)
	assert.Equal(t, 54321, display[0].PID)
}
