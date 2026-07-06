package workerprobe

import (
	"io"
	"math/rand"
	"net/http/httptest"
	"testing"
	"time"
)

// TestProbe_RegistersAgainstSelfSignedTLSServer proves that a Probe built
// with a nil Config.HTTPClient (i.e. relying on the package default) can
// complete registration against a self-signed TLS server, as the local
// ddx-server presents. Before the fix, the default client verified the
// server's certificate and every register POST failed the TLS handshake,
// leaving the probe permanently NotConnected.
func TestProbe_RegistersAgainstSelfSignedTLSServer(t *testing.T) {
	fs := newFakeServer("worker-tls-1")
	ts := httptest.NewTLSServer(fs.Handler())
	t.Cleanup(ts.Close)

	p := New(Identity{
		ProjectRoot:  "/tmp/probe-tls-test",
		Harness:      "claude",
		ExecutorPID:  4343,
		ExecutorHost: "host.test",
		StartedAt:    time.Now().UTC(),
	}, Config{
		BaseInterval:     20 * time.Millisecond,
		MaxInterval:      200 * time.Millisecond,
		JitterPct:        0.20,
		BackoffThreshold: 5,
		BufferCap:        4,
		AddrFunc:         func() string { return ts.URL },
		// HTTPClient intentionally left nil so applyDefaults constructs the
		// package default client under test.
		Rand:   rand.New(rand.NewSource(7)),
		Logger: io.Discard,
	})

	if p.Connected() {
		t.Fatal("expected probe to start NotConnected")
	}

	if !p.tick(t.Context()) {
		t.Fatal("expected tick to succeed against self-signed TLS server with default client")
	}

	if !p.Connected() {
		t.Fatal("expected probe to be Connected after successful register")
	}
	if got := p.WorkerID(); got != "worker-tls-1" {
		t.Fatalf("WorkerID() = %q, want %q", got, "worker-tls-1")
	}
}
