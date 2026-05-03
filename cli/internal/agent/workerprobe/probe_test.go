package workerprobe

import (
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer is a minimal stand-in for the worker-ingest endpoints. Tests
// drive Reachable to flip the server's "presence" without tearing down the
// httptest.Server.
type fakeServer struct {
	mu        sync.Mutex
	reachable bool
	workerID  string

	registered       int32
	events           int32
	backfills        int32
	lastEvents       []Event
	lastDropped      bool
	failNextRegister bool
}

func newFakeServer(workerID string) *fakeServer {
	return &fakeServer{reachable: true, workerID: workerID}
}

func (f *fakeServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/workers/register", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		if !f.reachable {
			f.mu.Unlock()
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		if f.failNextRegister {
			f.failNextRegister = false
			f.mu.Unlock()
			http.Error(w, "fail", http.StatusInternalServerError)
			return
		}
		atomic.AddInt32(&f.registered, 1)
		id := f.workerID
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"worker_id": id})
	})
	mux.HandleFunc("/api/workers/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		reachable := f.reachable
		f.mu.Unlock()
		if !reachable {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		// Expect /api/workers/{id}/event or /backfill
		switch {
		case hasSuffix(r.URL.Path, "/event"):
			var ev Event
			if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.lastEvents = append(f.lastEvents, ev)
			f.mu.Unlock()
			atomic.AddInt32(&f.events, 1)
			w.WriteHeader(http.StatusNoContent)
		case hasSuffix(r.URL.Path, "/backfill"):
			var req struct {
				Events  []Event `json:"events"`
				Dropped bool    `json:"dropped"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.lastEvents = append(f.lastEvents, req.Events...)
			f.lastDropped = f.lastDropped || req.Dropped
			f.mu.Unlock()
			atomic.AddInt32(&f.backfills, 1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}

// startFakeServer returns a running httptest.Server and a function to stop it.
func startFakeServer(t *testing.T, fs *fakeServer) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(fs.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// newTestProbe constructs a Probe with short intervals + deterministic jitter,
// pointed at fs's URL via the supplied addrFunc.
func newTestProbe(addrFunc func() string) *Probe {
	return New(Identity{
		ProjectRoot:  "/tmp/probe-test",
		Harness:      "claude",
		ExecutorPID:  4242,
		ExecutorHost: "host.test",
		StartedAt:    time.Now().UTC(),
	}, Config{
		BaseInterval:     20 * time.Millisecond,
		MaxInterval:      200 * time.Millisecond,
		JitterPct:        0.20,
		BackoffThreshold: 5,
		BufferCap:        4,
		AddrFunc:         addrFunc,
		HTTPClient:       &http.Client{Timeout: 1 * time.Second},
		Rand:             rand.New(rand.NewSource(42)),
		Logger:           io.Discard,
	})
}

// TestWorker_ProbeImmediateFirst — the first probe must fire without waiting
// for BaseInterval, so registration with an already-running server happens
// promptly.
func TestWorker_ProbeImmediateFirst(t *testing.T) {
	fs := newFakeServer("wkr-imm")
	ts := startFakeServer(t, fs)

	p := newTestProbe(func() string { return ts.URL })
	// Set BaseInterval very long so we ONLY observe the immediate first probe.
	p.cfg.BaseInterval = 10 * time.Second
	p.cfg.MaxInterval = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if p.Connected() && p.WorkerID() == "wkr-imm" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("probe did not register immediately; connected=%v workerID=%q registered=%d",
		p.Connected(), p.WorkerID(), atomic.LoadInt32(&fs.registered))
}

// TestWorker_ProbeJitter — successive intervals must vary within ±JitterPct
// of BaseInterval (and stay positive). We sample 30 intervals and require
// that they span a non-trivial range (proves jitter is applied) and stay
// within the [1-pct, 1+pct] envelope.
func TestWorker_ProbeJitter(t *testing.T) {
	fs := newFakeServer("wkr-jit")
	ts := startFakeServer(t, fs)

	p := newTestProbe(func() string { return ts.URL })
	const base = 50 * time.Millisecond
	const pct = 0.20
	p.cfg.BaseInterval = base
	p.cfg.MaxInterval = base
	p.cfg.JitterPct = pct
	p.cfg.Rand = rand.New(rand.NewSource(7))

	const samples = 30
	intervals := make([]time.Duration, 0, samples)
	var mu sync.Mutex
	done := make(chan struct{})
	p.OnProbe = func(connected bool, interval time.Duration) {
		mu.Lock()
		if len(intervals) < samples {
			intervals = append(intervals, interval)
			if len(intervals) == samples {
				close(done)
			}
		}
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("did not collect %d intervals (got %d)", samples, len(intervals))
	}

	mu.Lock()
	defer mu.Unlock()
	min := intervals[0]
	max := intervals[0]
	low := time.Duration(float64(base) * (1 - pct))
	high := time.Duration(float64(base) * (1 + pct))
	for _, d := range intervals {
		if d < low || d > high {
			t.Errorf("interval %v out of envelope [%v,%v]", d, low, high)
		}
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	if max-min < base/10 {
		t.Errorf("intervals show ≤10%% variance (min=%v max=%v); jitter not applied", min, max)
	}
}

// TestWorker_BackfillOnReconnect — events recorded while NotConnected MUST
// be delivered via /backfill on the NotConnected→Connected transition.
func TestWorker_BackfillOnReconnect(t *testing.T) {
	fs := newFakeServer("wkr-bf")
	ts := startFakeServer(t, fs)

	// Start with the addr unreachable (AddrFunc returns "").
	var addr atomic.Value
	addr.Store("") // empty means no server
	addrFunc := func() string {
		if v := addr.Load(); v != nil {
			return v.(string)
		}
		return ""
	}
	p := newTestProbe(addrFunc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	// Wait one probe cycle to ensure the goroutine ran the immediate first probe
	// in NotConnected state.
	time.Sleep(60 * time.Millisecond)
	if p.Connected() {
		t.Fatalf("probe should be NotConnected with empty addr")
	}

	// Record events while disconnected — they should buffer.
	p.RecordEvent(Event{BeadID: "ddx-aa", Kind: "attempt.started"})
	p.RecordEvent(Event{BeadID: "ddx-aa", Kind: "result"})
	if got := p.BufferLen(); got != 2 {
		t.Fatalf("buffer len after 2 disconnected events: got %d, want 2", got)
	}

	// Flip addr to point at the fake server.
	addr.Store(ts.URL)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&fs.backfills) > 0 && p.BufferLen() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&fs.backfills) == 0 {
		t.Fatalf("no backfill POST received")
	}
	if got := p.BufferLen(); got != 0 {
		t.Errorf("ring buffer not cleared after successful backfill; got len=%d", got)
	}
	fs.mu.Lock()
	got := len(fs.lastEvents)
	fs.mu.Unlock()
	if got != 2 {
		t.Errorf("server received %d events via backfill, want 2", got)
	}
}

// TestWorker_BackoffOnFailures — five consecutive probe failures MUST
// stretch the next interval to MaxInterval (backoff).
func TestWorker_BackoffOnFailures(t *testing.T) {
	// AddrFunc returns "" forever => every probe tick fails.
	p := newTestProbe(func() string { return "" })
	p.cfg.BaseInterval = 10 * time.Millisecond
	p.cfg.MaxInterval = 200 * time.Millisecond
	p.cfg.JitterPct = 0 // deterministic
	p.cfg.BackoffThreshold = 5

	var seen []time.Duration
	var mu sync.Mutex
	want := 7 // first 5 = base, then 2 = max
	done := make(chan struct{})
	p.OnProbe = func(connected bool, interval time.Duration) {
		mu.Lock()
		if len(seen) < want {
			seen = append(seen, interval)
			if len(seen) == want {
				close(done)
			}
		}
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("did not collect %d intervals (got %d)", want, len(seen))
	}

	mu.Lock()
	defer mu.Unlock()
	for i, d := range seen {
		if i < 5 {
			if d != 10*time.Millisecond {
				t.Errorf("interval[%d]=%v, want base=10ms", i, d)
			}
		} else {
			if d != 200*time.Millisecond {
				t.Errorf("interval[%d]=%v, want max=200ms (backoff)", i, d)
			}
		}
	}
}

// TestWorker_NoServer_StillWorks — RecordEvent never panics, never blocks,
// the probe goroutine starts/stops cleanly when no server is reachable, and
// events accumulate in the ring buffer up to BufferCap.
func TestWorker_NoServer_StillWorks(t *testing.T) {
	p := newTestProbe(func() string { return "" })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	defer p.Stop()

	// Record more events than BufferCap to exercise overflow.
	for i := 0; i < p.cfg.BufferCap+3; i++ {
		p.RecordEvent(Event{
			BeadID: "ddx-overflow",
			Kind:   "attempt.started",
		})
	}

	if got := p.BufferLen(); got != p.cfg.BufferCap {
		t.Errorf("buffer len: got %d, want %d (BufferCap)", got, p.cfg.BufferCap)
	}
	if !p.HadDroppedBackfill() {
		t.Error("HadDroppedBackfill should be true after overflow")
	}
	if p.Connected() {
		t.Error("probe should remain NotConnected without a server")
	}
	if p.WorkerID() != "" {
		t.Errorf("WorkerID should be empty when not registered; got %q", p.WorkerID())
	}
}
