package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// graphQLBody is a minimal JSON object suitable for asserting against in tests.
func okGraphQLBody(t *testing.T, payload any) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// newSpokeServer returns an httptest.Server that responds to /graphql with the
// given handler and exposes its base URL.
func newSpokeServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, SpokeRecord) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, SpokeRecord{
		NodeID:        "node-" + srv.Listener.Addr().String(),
		Name:          "spoke",
		URL:           srv.URL,
		DDxVersion:    "0.1.0",
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusActive,
	}
}

func TestFanOut_AllSpokesSucceed(t *testing.T) {
	_, s1 := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": map[string]any{"node": "1"}}))
	})
	_, s2 := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": map[string]any{"node": "2"}}))
	})

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion

	res, err := c.Execute(context.Background(), []SpokeRecord{s1, s2}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Responses) != 2 {
		t.Fatalf("want 2 responses, got %d (errors=%v)", len(res.Responses), res.Errors)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("want 0 errors, got %v", res.Errors)
	}
}

func TestFanOut_PartialResultOnOneFailure(t *testing.T) {
	_, good := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})
	dead := SpokeRecord{
		NodeID:        "dead-spoke",
		Name:          "dead",
		URL:           "http://127.0.0.1:1", // port 1 → connection refused
		DDxVersion:    "0.1.0",
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusActive,
	}
	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.PerNodeTimeout = 500 * time.Millisecond

	res, err := c.Execute(context.Background(), []SpokeRecord{good, dead}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := res.Responses[good.NodeID]; !ok {
		t.Fatalf("expected good spoke to succeed, got %+v", res)
	}
	if _, ok := res.Errors[dead.NodeID]; !ok {
		t.Fatalf("expected dead spoke to be in errors map, got %+v", res.Errors)
	}
	if status, ok := res.StatusUpdates[dead.NodeID]; !ok || status != StatusOffline {
		t.Fatalf("expected dead spoke status update offline, got %+v", res.StatusUpdates)
	}
	// The good spoke must NOT receive a status update — only transport failures do.
	if _, ok := res.StatusUpdates[good.NodeID]; ok {
		t.Fatalf("good spoke should not receive a status update")
	}
}

func TestFanOut_PerNodeTimeoutDoesNotBlockOthers(t *testing.T) {
	_, fast := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "fast"}))
	})
	_, slow := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "slow"}))
	})

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.PerNodeTimeout = 100 * time.Millisecond

	start := time.Now()
	res, err := c.Execute(context.Background(), []SpokeRecord{slow, fast}, &FanOutRequest{Query: "{ __typename }"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Should complete in well under the slow spoke's 2s wait.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("fan-out took %s — slow spoke blocked others", elapsed)
	}
	if _, ok := res.Responses[fast.NodeID]; !ok {
		t.Fatalf("fast spoke missing from responses: %+v", res)
	}
	if _, ok := res.Errors[slow.NodeID]; !ok {
		t.Fatalf("slow spoke missing from errors: %+v", res.Errors)
	}
	// Per-node timeout must not flip the spoke to offline (could just be stale).
	if _, ok := res.StatusUpdates[slow.NodeID]; ok {
		t.Fatalf("per-node timeout must not produce a status update; got %+v", res.StatusUpdates)
	}
	// The per-node detail should classify the slow spoke as OutcomeTimeout.
	var found bool
	for _, n := range res.Nodes {
		if n.NodeID == slow.NodeID {
			if n.Outcome != OutcomeTimeout {
				t.Fatalf("slow spoke outcome = %q, want %q", n.Outcome, OutcomeTimeout)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("slow spoke not present in Nodes")
	}
}

func TestFanOut_MaxConcurrencyEnforced(t *testing.T) {
	const totalSpokes = 10
	const maxConc = 3

	var inFlight int32
	var peak int32
	gate := make(chan struct{})

	handler := func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		<-gate
		atomic.AddInt32(&inFlight, -1)
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	}

	var spokes []SpokeRecord
	for i := 0; i < totalSpokes; i++ {
		_, s := newSpokeServer(t, handler)
		s.NodeID = fmt.Sprintf("node-%d", i)
		spokes = append(spokes, s)
	}

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.MaxConcurrency = maxConc
	c.PerNodeTimeout = 5 * time.Second

	done := make(chan *FanOutResult, 1)
	go func() {
		res, _ := c.Execute(context.Background(), spokes, &FanOutRequest{Query: "{ __typename }"})
		done <- res
	}()

	// Give all goroutines time to spin up and observe peak concurrency.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&peak) >= int32(maxConc) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Briefly observe steady-state peak before releasing.
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&peak); got > int32(maxConc) {
		t.Fatalf("peak in-flight %d exceeded max-concurrency %d", got, maxConc)
	}
	close(gate)

	res := <-done
	if len(res.Responses) != totalSpokes {
		t.Fatalf("want %d successes, got %d", totalSpokes, len(res.Responses))
	}
	if got := atomic.LoadInt32(&peak); got > int32(maxConc) {
		t.Fatalf("peak in-flight %d exceeded max-concurrency %d", got, maxConc)
	}
	if got := atomic.LoadInt32(&peak); got == 0 {
		t.Fatalf("peak in-flight is zero — handler never observed concurrency")
	}
}

func TestFanOut_VersionIncompatibleSpokesSkipped(t *testing.T) {
	var hits int32
	_, compat := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})
	_, incompat := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})
	incompat.NodeID = "incompat-spoke"
	incompat.DDxVersion = "9.0.0" // major mismatch

	var logs []string
	var logMu sync.Mutex
	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.Logger = func(format string, args ...any) {
		logMu.Lock()
		logs = append(logs, fmt.Sprintf(format, args...))
		logMu.Unlock()
	}

	res, err := c.Execute(context.Background(), []SpokeRecord{compat, incompat}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected exactly one HTTP hit (compat only); got %d", got)
	}
	reason, ok := res.Skipped[incompat.NodeID]
	if !ok || reason != SkipReasonIncompatibleVersion {
		t.Fatalf("expected incompat spoke to be skipped with %q, got %+v", SkipReasonIncompatibleVersion, res.Skipped)
	}
	if _, ok := res.Errors[incompat.NodeID]; ok {
		t.Fatalf("skipped spoke must not appear in Errors map")
	}
	logMu.Lock()
	defer logMu.Unlock()
	var sawSkipLog bool
	for _, l := range logs {
		if strings.Contains(l, incompat.NodeID) && strings.Contains(l, string(SkipReasonIncompatibleVersion)) {
			sawSkipLog = true
		}
	}
	if !sawSkipLog {
		t.Fatalf("expected a skip log entry for %q; logs=%v", incompat.NodeID, logs)
	}
}

func TestFanOut_MissingCapabilitySkipped(t *testing.T) {
	_, withCap := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})
	withCap.Capabilities = []string{"beads.read", "@identity:fp-x"}
	_, withoutCap := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("spoke without required capability must not be called")
	})
	withoutCap.NodeID = "no-cap-spoke"
	withoutCap.Capabilities = []string{"runs.read"}

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion

	res, err := c.Execute(context.Background(), []SpokeRecord{withCap, withoutCap}, &FanOutRequest{
		Query:                "{ beads { id } }",
		RequiredCapabilities: []string{"beads.read"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := res.Responses[withCap.NodeID]; !ok {
		t.Fatalf("with-cap spoke should have responded")
	}
	if reason, ok := res.Skipped[withoutCap.NodeID]; !ok || reason != SkipReasonMissingCapability {
		t.Fatalf("without-cap spoke should be skipped with %q, got %+v", SkipReasonMissingCapability, res.Skipped)
	}
}

func TestFanOut_StaleVsOfflineDistinct(t *testing.T) {
	// A spoke whose registry status was StatusStale but which now responds
	// must NOT receive a StatusOffline update (the registry's own time-based
	// reconcile owns stale). A spoke whose transport fails must.
	_, alive := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})
	alive.Status = StatusStale // hub considered it stale, but it's actually up

	dead := SpokeRecord{
		NodeID:        "dead",
		URL:           "http://127.0.0.1:1",
		DDxVersion:    "0.1.0",
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusActive,
	}

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.PerNodeTimeout = 500 * time.Millisecond

	res, err := c.Execute(context.Background(), []SpokeRecord{alive, dead}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := res.StatusUpdates[alive.NodeID]; ok {
		t.Fatalf("stale-but-alive spoke must not receive a StatusOffline update; got %+v", res.StatusUpdates)
	}
	if got := res.StatusUpdates[dead.NodeID]; got != StatusOffline {
		t.Fatalf("dead spoke status update = %q, want %q", got, StatusOffline)
	}
}

// TestFanOut_MidResponseDisconnect simulates a spoke that accepts the
// connection, writes partial body, then forcibly closes mid-response.
func TestFanOut_MidResponseDisconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				// Read request roughly.
				buf := make([]byte, 4096)
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _ = c.Read(buf)
				// Send headers claiming larger Content-Length, then close mid-body.
				_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1024\r\n\r\n")
				_, _ = io.WriteString(c, `{"data":`)
				// Drop the connection.
			}(conn)
		}
	}()

	rec := SpokeRecord{
		NodeID:        "flaky",
		URL:           "http://" + ln.Addr().String(),
		DDxVersion:    "0.1.0",
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusActive,
	}
	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	c.PerNodeTimeout = 1 * time.Second

	res, err := c.Execute(context.Background(), []SpokeRecord{rec}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := res.Errors[rec.NodeID]; !ok {
		t.Fatalf("expected mid-response disconnect to surface as an error; got %+v", res)
	}
	if got := res.StatusUpdates[rec.NodeID]; got != StatusOffline {
		t.Fatalf("mid-response disconnect status = %q, want %q", got, StatusOffline)
	}
}

func TestFanOut_HTTPErrorIsNotOffline(t *testing.T) {
	_, errSpoke := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion

	res, err := c.Execute(context.Background(), []SpokeRecord{errSpoke}, &FanOutRequest{Query: "{ __typename }"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := res.Errors[errSpoke.NodeID]; !ok {
		t.Fatalf("expected an error for 500 response")
	}
	// HTTP error must NOT mark the spoke offline (transport worked).
	if _, ok := res.StatusUpdates[errSpoke.NodeID]; ok {
		t.Fatalf("HTTP error must not produce a status update; got %+v", res.StatusUpdates)
	}
	var found bool
	for _, n := range res.Nodes {
		if n.NodeID == errSpoke.NodeID && n.Outcome == OutcomeHTTPError {
			found = true
			if n.HTTPStatus != http.StatusInternalServerError {
				t.Fatalf("HTTPStatus = %d, want 500", n.HTTPStatus)
			}
		}
	}
	if !found {
		t.Fatalf("expected OutcomeHTTPError for spoke, got %+v", res.Nodes)
	}
}

func TestFanOut_RejectsEmptyRequest(t *testing.T) {
	c := NewFanOutClient()
	if _, err := c.Execute(context.Background(), nil, nil); err == nil {
		t.Fatalf("expected error on nil request")
	}
	if _, err := c.Execute(context.Background(), nil, &FanOutRequest{Query: "  "}); err == nil {
		t.Fatalf("expected error on empty query")
	}
}

func TestFanOut_PostsGraphQLPayload(t *testing.T) {
	var got struct {
		Query         string         `json:"query"`
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	_, s := newSpokeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write(okGraphQLBody(t, map[string]any{"data": "ok"}))
	})

	c := NewFanOutClient()
	c.HubDDxVersion = "0.1.0"
	c.HubSchemaVersion = CurrentSchemaVersion
	_, err := c.Execute(context.Background(), []SpokeRecord{s}, &FanOutRequest{
		Query:         "query Q { x }",
		OperationName: "Q",
		Variables:     map[string]any{"foo": "bar"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.Query != "query Q { x }" || got.OperationName != "Q" || got.Variables["foo"] != "bar" {
		t.Fatalf("posted payload mismatch: %+v", got)
	}
}
