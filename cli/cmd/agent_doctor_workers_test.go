package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorWorkerStubServer returns an httptest.Server that serves the runtime
// registry view at GET /api/workers. payload is the JSON returned verbatim.
func doctorWorkerStubServer(t *testing.T, payload []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/workers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// writeDoctorDiskWorker writes a minimal status.json under
// <projectRoot>/.ddx/workers/<id>/status.json so the on-disk fallback can
// pick it up.
func writeDoctorDiskWorker(t *testing.T, projectRoot, id, harness, state string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".ddx", "workers", id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	rec := map[string]any{
		"id":           id,
		"state":        state,
		"project_root": projectRoot,
		"harness":      harness,
		"started_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.json"), data, 0o644))
}

// TestDoctor_ReadsFromServer_WhenAvailable verifies that `ddx agent doctor
// --workers` prefers the runtime registry and surfaces freshness +
// mirror_failures_count fields when the server is reachable.
func TestDoctor_ReadsFromServer_WhenAvailable(t *testing.T) {
	srv := doctorWorkerStubServer(t, []map[string]any{
		{
			"worker_id":             "wkr-aaa111",
			"project_root":          "/tmp/projA",
			"harness":               "claude",
			"registered_at":         time.Now().UTC().Format(time.RFC3339Nano),
			"last_event_at":         time.Now().UTC().Format(time.RFC3339Nano),
			"mirror_failures_count": 3,
			"freshness":             "connected",
		},
	})
	t.Setenv("DDX_SERVER_URL", srv.URL)
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	tmp := t.TempDir()
	// Add a disk worker too — the server path must win.
	writeDoctorDiskWorker(t, tmp, "wkr-disk-only", "codex", "running")

	factory := NewCommandFactory(tmp)
	root := factory.NewRootCommand()
	out, err := executeCommand(root, "agent", "doctor", "--workers")
	require.NoError(t, err)
	assert.Contains(t, out, "Worker source: server")
	assert.Contains(t, out, "wkr-aaa111")
	assert.Contains(t, out, "connected")
	// mirror_failures_count = 3 must be visible on the same line.
	assert.Regexp(t, `wkr-aaa111\s+claude\s+connected\s+\S+\s+3\s+`, out)
	// Disk worker must NOT appear when the server path wins.
	assert.NotContains(t, out, "wkr-disk-only")
}

// TestDoctor_FallsBackToOnDisk_WhenServerDown verifies that with no server
// reachable the doctor reads .ddx/workers/<id>/status.json directly and
// emits the same table layout.
func TestDoctor_FallsBackToOnDisk_WhenServerDown(t *testing.T) {
	// Point at a port nothing is listening on.
	t.Setenv("DDX_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	tmp := t.TempDir()
	writeDoctorDiskWorker(t, tmp, "wkr-disk-001", "codex", "running")

	factory := NewCommandFactory(tmp)
	root := factory.NewRootCommand()
	out, err := executeCommand(root, "agent", "doctor", "--workers")
	require.NoError(t, err)
	assert.Contains(t, out, "Worker source: disk")
	assert.Contains(t, out, "wkr-disk-001")
	assert.Contains(t, out, "codex")
	assert.Contains(t, out, "running")
	// Header is identical across modes — operators see the same columns.
	assert.Contains(t, out, "WORKER")
	assert.Contains(t, out, "FRESHNESS")
	assert.Contains(t, out, "MIRROR_FAILS")
}

// TestDoctor_HandlesServerDownUpDown exercises the WIRED-IN transition
// requirement: server-down → up → down sequencing must produce consistent
// output for the operator at each step. Drives through one CLI process per
// transition since each invocation re-resolves the server target.
func TestDoctor_HandlesServerDownUpDown(t *testing.T) {
	tmp := t.TempDir()
	writeDoctorDiskWorker(t, tmp, "wkr-fallback", "codex", "running")

	runDoctor := func(t *testing.T) string {
		t.Helper()
		factory := NewCommandFactory(tmp)
		root := factory.NewRootCommand()
		out, err := executeCommand(root, "agent", "doctor", "--workers")
		require.NoError(t, err)
		return out
	}

	const headerCols = "WORKER"

	// Phase 1 — server down: operator sees the disk fallback.
	t.Setenv("DDX_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	step1 := runDoctor(t)
	assert.Contains(t, step1, "Worker source: disk")
	assert.Contains(t, step1, "wkr-fallback")
	assert.Contains(t, step1, headerCols)

	// Phase 2 — server up: operator sees the runtime registry instead.
	srv := doctorWorkerStubServer(t, []map[string]any{
		{
			"worker_id":             "wkr-server-up",
			"project_root":          tmp,
			"harness":               "claude",
			"registered_at":         time.Now().UTC().Format(time.RFC3339Nano),
			"last_event_at":         time.Now().UTC().Format(time.RFC3339Nano),
			"mirror_failures_count": 0,
			"freshness":             "connected",
		},
	})
	t.Setenv("DDX_SERVER_URL", srv.URL)
	step2 := runDoctor(t)
	assert.Contains(t, step2, "Worker source: server")
	assert.Contains(t, step2, "wkr-server-up")
	assert.Contains(t, step2, "connected")
	assert.Contains(t, step2, headerCols)
	assert.NotContains(t, step2, "wkr-fallback",
		"server path must hide the disk worker once the registry is reachable")

	// Phase 3 — server down again: must transparently fall back once more.
	srv.Close()
	t.Setenv("DDX_SERVER_URL", "http://127.0.0.1:1")
	step3 := runDoctor(t)
	assert.Contains(t, step3, "Worker source: disk")
	assert.Contains(t, step3, "wkr-fallback")
	assert.Contains(t, step3, headerCols)

	// Consistency: every transition surfaced the same column header so the
	// operator's eye doesn't have to relearn the layout when the server
	// bounces.
	for _, step := range []string{step1, step2, step3} {
		assert.True(t, strings.Contains(step, "WORKER") &&
			strings.Contains(step, "FRESHNESS") &&
			strings.Contains(step, "MIRROR_FAILS"),
			"each transition must emit the same column header; got:\n%s", step)
	}
}
