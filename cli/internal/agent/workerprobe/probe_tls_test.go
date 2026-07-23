package workerprobe

import (
	"io"
	mathrand "math/rand"
	"net/http"
	"testing"
	"time"
)

// TestProbe_DefaultHTTPClientSkipsVerification proves that a Probe built
// with a nil Config.HTTPClient constructs the default transport used for the
// local ddx server: a 5s timeout and TLS verification disabled so the
// self-signed server certificate does not block registration.
func TestProbe_DefaultHTTPClientSkipsVerification(t *testing.T) {
	p := New(Identity{
		ProjectRoot:  "/tmp/probe-tls-test",
		Harness:      "claude",
		ExecutorPID:  4343,
		ExecutorHost: "host.test",
		StartedAt:    time.Now().UTC(),
	}, Config{
		AddrFunc: func() string { return "https://workerprobe.test" },
		// HTTPClient intentionally left nil so applyDefaults constructs the
		// package default client under test.
		Rand:   mathrand.New(mathrand.NewSource(7)),
		Logger: io.Discard,
	})

	transport, ok := p.cfg.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default client transport has type %T, want *http.Transport", p.cfg.HTTPClient.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("default client transport missing TLSClientConfig")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("default client must skip TLS verification for local self-signed servers")
	}
	if got, want := p.cfg.HTTPClient.Timeout, 5*time.Second; got != want {
		t.Fatalf("default client timeout = %v, want %v", got, want)
	}
}
