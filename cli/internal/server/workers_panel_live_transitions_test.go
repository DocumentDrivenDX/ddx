package server

// ADR-022 step 5c: wired-in integration proof that the GraphQL workers panel
// observes a real worker's freshness transitions (connected → stale →
// disconnected) without sleeping. Stands up the production HTTP path
// (httptest.Server wrapping srv.Handler() — same mux, same trusted gate, real
// TCP socket), re-execs the test binary as a subprocess that POSTs to
// /api/workers/register over the network, then drives the freshness clock via
// the Server's reportedWorkers adapter.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// liveTransitionsChildEnv is the URL of the parent's httptest.Server when the
// test binary is re-execed as the worker process. Empty in normal test runs
// (the helper test below skips), set by the parent integration test.
const liveTransitionsChildEnv = "DDX_WORKERS_LIVE_TRANSITIONS_URL"

// TestWorkersPanel_LiveTransitions stands up a real httptest.Server, has a
// real subprocess "worker" register against it, and asserts the GraphQL
// reportedWorkers query reports each freshness transition (connected at <2×
// probe interval, stale at <10×, disconnected past 10×) by advancing a
// synthetic clock injected into the Server's reportedWorkers adapter — no
// wall-clock sleeping past the probe thresholds.
func TestWorkersPanel_LiveTransitions(t *testing.T) {
	if os.Getenv(liveTransitionsChildEnv) != "" {
		// Parent re-execs only TestHelperLiveTransitionsRegisterChild; this
		// guard makes a misconfigured runner fall through cleanly.
		t.Skip("child invocation; covered by helper test")
	}
	dir := setupTestDir(t)
	srv := New(":0", dir)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Spawn a real worker process: re-exec ourselves into the helper test,
	// which performs a real net/http POST to /api/workers/register.
	cmd := exec.Command(os.Args[0],
		"-test.run", "^TestHelperLiveTransitionsRegisterChild$",
		"-test.v",
		"-test.count=1",
		"-test.timeout=30s",
	)
	cmd.Env = append(os.Environ(), liveTransitionsChildEnv+"="+ts.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worker subprocess failed: %v\n%s", err, out)
	}

	// AC #3: connected within 30s of worker start.
	deadline := time.Now().Add(30 * time.Second)
	var rec *workerRecord
	for time.Now().Before(deadline) {
		snap := srv.workerIngest.snapshot()
		if len(snap) == 1 {
			rec = snap[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rec == nil {
		t.Fatalf("worker did not appear in registry within 30s; subprocess output:\n%s", out)
	}
	base := rec.LastEventAt
	if base.IsZero() {
		t.Fatalf("worker LastEventAt unset after register")
	}

	queryState := func(label string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{
			"query": `{ reportedWorkers { id state } }`,
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/graphql", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("[%s] new request: %v", label, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("[%s] graphql post: %v", label, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("[%s] graphql status=%d body=%s", label, resp.StatusCode, raw)
		}
		var rp struct {
			Data struct {
				ReportedWorkers []struct {
					ID    string `json:"id"`
					State string `json:"state"`
				} `json:"reportedWorkers"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(raw, &rp); err != nil {
			t.Fatalf("[%s] decode %q: %v", label, raw, err)
		}
		if len(rp.Errors) > 0 {
			t.Fatalf("[%s] graphql errors: %+v", label, rp.Errors)
		}
		if got := len(rp.Data.ReportedWorkers); got != 1 {
			t.Fatalf("[%s] reportedWorkers len: got %d, want 1; body=%s", label, got, raw)
		}
		w := rp.Data.ReportedWorkers[0]
		if w.ID != rec.WorkerID {
			t.Fatalf("[%s] worker id: got %q, want %q", label, w.ID, rec.WorkerID)
		}
		return w.State
	}

	// AC #6: clock-injection — advance synthetic time without sleeping past
	// the probe-interval thresholds. Probe interval is 30s (rev 5).
	setSyntheticNow := func(at time.Time) {
		srv.reportedWorkers.setNow(func() time.Time { return at })
	}

	// AC #3: connected.
	setSyntheticNow(base.Add(5 * time.Second))
	if got := queryState("connected"); got != "connected" {
		t.Fatalf("expected state=connected, got %q", got)
	}

	// AC #4: stale after 2× probe interval (60s) without further events.
	setSyntheticNow(base.Add(61 * time.Second))
	if got := queryState("stale"); got != "stale" {
		t.Fatalf("expected state=stale at +61s, got %q", got)
	}

	// AC #5: disconnected after 10× probe interval (300s).
	setSyntheticNow(base.Add(301 * time.Second))
	if got := queryState("disconnected"); got != "disconnected" {
		t.Fatalf("expected state=disconnected at +301s, got %q", got)
	}
}

// TestHelperLiveTransitionsRegisterChild is the worker-process half of
// TestWorkersPanel_LiveTransitions. Skips unless re-execed with the env var
// set, in which case it POSTs a real registration to the parent's HTTP
// server — exercising the live data flow end-to-end.
func TestHelperLiveTransitionsRegisterChild(t *testing.T) {
	url := os.Getenv(liveTransitionsChildEnv)
	if url == "" {
		t.Skip("not invoked as subprocess child; parent test re-execs us")
	}
	body, err := json.Marshal(workerIdentity{
		ProjectRoot:  "/tmp/live-transitions-project",
		Harness:      "claude",
		Model:        "claude-opus-4",
		ExecutorPID:  os.Getpid(),
		ExecutorHost: "live-transitions-host",
		StartedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	resp, err := http.Post(url+"/api/workers/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/workers/register: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, respBody)
	}
	var out struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("decode response %q: %v", respBody, err)
	}
	if out.WorkerID == "" {
		t.Fatal("empty worker_id in response")
	}
}
