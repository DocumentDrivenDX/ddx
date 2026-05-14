package workerprobe_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/agent/workerprobe"
	serverpkg "github.com/DocumentDrivenDX/ddx/internal/server"
	"github.com/stretchr/testify/require"
)

// TestWorkerProbe_PeriodicHeartbeatRefreshesServerLastEventAt verifies AC #2:
// while a long-running attempt is mid-execution the worker emits periodic
// worker.heartbeat envelopes through the workerprobe TeeJSONL sink. The
// server-derived last_event_at must advance across heartbeats so an
// operator's `agent doctor` / GraphQL view can answer "is this worker
// alive?" before bead.result arrives.
func TestWorkerProbe_PeriodicHeartbeatRefreshesServerLastEventAt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: spins up an httptest server")
	}

	projectRoot := t.TempDir()

	srv := serverpkg.New(":0", projectRoot)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	serverURL := ts.URL

	probe := workerprobe.New(workerprobe.Identity{
		ProjectRoot:  projectRoot,
		Harness:      "claude",
		ExecutorPID:  os.Getpid(),
		ExecutorHost: "host.test",
		StartedAt:    time.Now().UTC(),
	}, workerprobe.Config{
		BaseInterval:     20 * time.Millisecond,
		MaxInterval:      40 * time.Millisecond,
		JitterPct:        0.01,
		BackoffThreshold: 5,
		BufferCap:        8,
		AddrFunc:         func() string { return serverURL },
		HTTPClient:       &http.Client{Timeout: time.Second},
		Logger:           io.Discard,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe.Start(ctx)
	defer probe.Stop()

	require.Eventually(t, func() bool {
		return probe.Connected() && probe.WorkerID() != ""
	}, 2*time.Second, 5*time.Millisecond, "probe never connected to test server")
	workerID := probe.WorkerID()

	sink := workerprobe.TeeJSONL(io.Discard, probe)
	reporter := work.NewSidecarLivenessReporter(projectRoot, workerID, "sess-heartbeat", sink)
	reporter.SetAttempt("ddx-long", "att-long-001", "running", "balanced", "claude", "opus", "balanced", 0)

	reporter.OnTick(time.Now())
	require.Eventually(t, func() bool {
		when, ok := lastEventAt(t, serverURL, workerID)
		return ok && !when.IsZero()
	}, 2*time.Second, 10*time.Millisecond, "first heartbeat must reach server before bead.result")
	first, _ := lastEventAt(t, serverURL, workerID)

	// Wait long enough that a fresh heartbeat advances the timestamp past
	// the time-quantum the server uses for LastEventAt.
	time.Sleep(20 * time.Millisecond)
	reporter.OnTick(time.Now())
	require.Eventually(t, func() bool {
		next, ok := lastEventAt(t, serverURL, workerID)
		return ok && next.After(first)
	}, 2*time.Second, 10*time.Millisecond, "periodic heartbeat must advance server last_event_at")
}

func lastEventAt(t *testing.T, baseURL, workerID string) (time.Time, bool) {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/workers")
	if err != nil {
		return time.Time{}, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, false
	}
	var views []struct {
		WorkerID    string    `json:"worker_id"`
		LastEventAt time.Time `json:"last_event_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		return time.Time{}, false
	}
	for _, v := range views {
		if v.WorkerID == workerID {
			return v.LastEventAt, true
		}
	}
	return time.Time{}, false
}
