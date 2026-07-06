package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestLocalServerClient_AcceptsSelfSignedCert guards the loopback bad-cert fix:
// the DDx server presents a self-signed cert, so local clients (notably the
// worker server-health probe) must skip verification. A bare http.Client
// rejects it with "tls: bad certificate" and spams the server log.
func TestLocalServerClient_AcceptsSelfSignedCert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// A bare client must reject the self-signed cert — this is the bug the
	// health probe used to hit.
	if _, err := (&http.Client{Timeout: 5 * time.Second}).Get(srv.URL); err == nil {
		t.Fatal("expected bare http.Client to reject the self-signed cert")
	}

	for name, c := range map[string]*http.Client{
		"newLocalServerClient":        newLocalServerClient(),
		"newLocalServerClientTimeout": newLocalServerClientTimeout(5 * time.Second),
	} {
		resp, err := c.Get(srv.URL)
		if err != nil {
			t.Fatalf("%s: expected self-signed cert accepted, got %v", name, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", name, resp.StatusCode)
		}
	}

	if got := newLocalServerClientTimeout(7 * time.Second).Timeout; got != 7*time.Second {
		t.Fatalf("timeout not honored: got %v, want 7s", got)
	}
}
