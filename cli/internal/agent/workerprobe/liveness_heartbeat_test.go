package workerprobe_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
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

	// Isolate server state before constructing the server. New() registers
	// the project immediately.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	srv := serverpkg.New(":0", projectRoot)
	serverHandler := srv.Handler()

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
		AddrFunc:         func() string { return "http://workerprobe.test" },
		HTTPClient:       newInProcessHTTPClient(serverHandler),
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
		when, ok := lastEventAt(t, serverHandler, workerID)
		return ok && !when.IsZero()
	}, 2*time.Second, 10*time.Millisecond, "first heartbeat must reach server before bead.result")
	first, _ := lastEventAt(t, serverHandler, workerID)

	// Wait long enough that a fresh heartbeat advances the timestamp past
	// the time-quantum the server uses for LastEventAt.
	time.Sleep(20 * time.Millisecond)
	reporter.OnTick(time.Now())
	require.Eventually(t, func() bool {
		next, ok := lastEventAt(t, serverHandler, workerID)
		return ok && next.After(first)
	}, 2*time.Second, 10*time.Millisecond, "periodic heartbeat must advance server last_event_at")
}

// TestWorkerProbe_ServerUnavailableDoesNotBlockWork verifies AC #2: when
// AddrFunc returns "" (no server configured) or the configured server refuses
// connections, the same heartbeat path runs without propagating errors to the
// attempt. WithHeartbeat must return the fn's value, ticks must continue to
// fire against the bead store, and no new failure status, cooldown, or stop
// may be introduced by the probe's RecordEvent path.
func TestWorkerProbe_ServerUnavailableDoesNotBlockWork(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	// Use a fixed loopback port with nothing listening instead of binding a
	// listener. The probe must fail open on connection refusal.
	refusedURL := "http://127.0.0.1:1"

	cases := []struct {
		name     string
		addrFunc func() string
	}{
		{"no_addr", func() string { return "" }},
		{"connection_refused", func() string { return refusedURL }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()

			probe := workerprobe.New(workerprobe.Identity{
				ProjectRoot:  projectRoot,
				Harness:      "claude",
				ExecutorPID:  os.Getpid(),
				ExecutorHost: "host.test",
				StartedAt:    time.Now().UTC(),
			}, workerprobe.Config{
				BaseInterval:     10 * time.Millisecond,
				MaxInterval:      20 * time.Millisecond,
				JitterPct:        0.01,
				BackoffThreshold: 5,
				BufferCap:        8,
				AddrFunc:         tc.addrFunc,
				HTTPClient:       &http.Client{Timeout: 200 * time.Millisecond},
				Logger:           io.Discard,
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			probe.Start(ctx)
			defer probe.Stop()

			sink := workerprobe.TeeJSONL(io.Discard, probe)
			reporter := work.NewSidecarLivenessReporter(projectRoot, "wkr-unavail", "sess-unavail", sink)
			reporter.SetAttempt("ddx-long", "att-long-001", "running", "balanced", "claude", "opus", "balanced", 0)

			store := &countingHeartbeatStore{}

			done := make(chan struct{})
			var res string
			var attemptErr error
			go func() {
				defer close(done)
				res, attemptErr = work.WithHeartbeat(
					ctx,
					"ddx-long",
					10*time.Millisecond,
					store,
					reporter,
					func() (string, error) {
						// Simulate a long-running attempt: several heartbeat
						// ticks fire while the server is unreachable.
						time.Sleep(75 * time.Millisecond)
						return "ok", nil
					},
				)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("WithHeartbeat blocked despite server being unavailable — heartbeat path must fail open")
			}

			require.NoError(t, attemptErr, "heartbeat path must not surface errors to the attempt")
			require.Equal(t, "ok", res, "attempt must complete normally with its own return value")
			require.False(t, probe.Connected(), "probe must remain NotConnected when no server is reachable")
			require.GreaterOrEqual(t, store.count.Load(), int32(1), "heartbeat ticks must continue to refresh the bead store")
		})
	}
}

// countingHeartbeatStore satisfies the unexported heartbeatStore interface
// of internal/agent/work via structural typing.
type countingHeartbeatStore struct{ count atomic.Int32 }

func (s *countingHeartbeatStore) TouchClaimHeartbeat(_ string) error {
	s.count.Add(1)
	return nil
}

func lastEventAt(t *testing.T, handler http.Handler, workerID string) (time.Time, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return time.Time{}, false
	}
	var views []struct {
		WorkerID    string    `json:"worker_id"`
		LastEventAt time.Time `json:"last_event_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&views); err != nil {
		return time.Time{}, false
	}
	for _, v := range views {
		if v.WorkerID == workerID {
			return v.LastEventAt, true
		}
	}
	return time.Time{}, false
}
