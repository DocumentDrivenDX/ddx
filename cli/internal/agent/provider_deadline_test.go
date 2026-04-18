package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	oai "github.com/DocumentDrivenDX/agent/provider/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeoutProviderChatStalledServer replays the RC4 hang scenario: an
// OpenAI-compatible server that accepts the TCP connection, writes HTTP
// response headers, then stops sending body bytes. Without a per-request
// deadline the openai SDK Chat call would block on the half-open socket
// until the outer wall-clock (3h) fired.
//
// With wrapProviderWithDeadlinesTimeouts(...), Chat must unwind within a
// bounded multiple of the configured requestTimeout, returning an error
// that errors.Is(err, ErrProviderRequestTimeout).
func TestTimeoutProviderChatStalledServer(t *testing.T) {
	// Server handler: send 200 + headers + flush, then block indefinitely
	// without sending body bytes. This mimics a proxy that crashed
	// mid-stream or a TCP socket that went half-open after headers.
	serverDone := make(chan struct{})
	defer close(serverDone)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Hang the request until the test tears the server down, or the
		// client disconnects. Either way no body bytes are emitted.
		select {
		case <-serverDone:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	// Build a real openai provider pointed at the stalled server.
	inner := oai.New(oai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "not-needed",
		Model:   "stub-model",
		Flavor:  "openai", // skip the flavor probe
	})
	requestTimeout := 500 * time.Millisecond
	idleTimeout := 500 * time.Millisecond
	provider := wrapProviderWithDeadlinesTimeouts(inner, requestTimeout, idleTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err := provider.Chat(ctx, []agentlib.Message{
		{Role: agentlib.RoleUser, Content: "hello"},
	}, nil, agentlib.Options{})
	elapsed := time.Since(start)

	require.Error(t, err, "Chat against a stalled server must return an error")
	assert.True(t, errors.Is(err, ErrProviderRequestTimeout),
		"expected ErrProviderRequestTimeout; got %v", err)
	// Generous bound: request cap + some slack for the goroutine to wake.
	assert.Less(t, elapsed, 5*time.Second,
		"Chat must unwind within a bounded multiple of requestTimeout; took %v", elapsed)
	// The caller ctx should still be alive (its deadline is 10s).
	assert.NoError(t, ctx.Err(), "wrapper-triggered timeout must not cancel caller ctx")
}

// TestTimeoutProviderChatStreamStalledServer is the streaming-path variant:
// the server accepts the stream request, sends headers, then stops. The
// wrapper's idle-read timer must fire and emit a StreamDelta{Err: ...} that
// errors.Is matches ErrProviderRequestTimeout.
func TestTimeoutProviderChatStreamStalledServer(t *testing.T) {
	serverDone := make(chan struct{})
	defer close(serverDone)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Claim SSE so the SDK enters streaming-read mode.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-serverDone:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	inner := oai.New(oai.Config{
		BaseURL: srv.URL + "/v1",
		APIKey:  "not-needed",
		Model:   "stub-model",
		Flavor:  "openai",
	})
	requestTimeout := 10 * time.Second // keep this loose so we isolate idle-read
	idleTimeout := 400 * time.Millisecond
	wrapped := wrapProviderWithDeadlinesTimeouts(inner, requestTimeout, idleTimeout)
	sp, ok := wrapped.(agentlib.StreamingProvider)
	require.True(t, ok, "openai wrapper must advertise StreamingProvider")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	ch, err := sp.ChatStream(ctx, []agentlib.Message{
		{Role: agentlib.RoleUser, Content: "hello"},
	}, nil, agentlib.Options{})
	require.NoError(t, err, "ChatStream must return a channel before the stall fires")

	var sawTimeoutDelta bool
	for delta := range ch {
		if delta.Err != nil {
			assert.True(t, errors.Is(delta.Err, ErrProviderRequestTimeout),
				"expected delta.Err to wrap ErrProviderRequestTimeout; got %v", delta.Err)
			sawTimeoutDelta = true
			break
		}
		if delta.Done {
			break
		}
	}
	elapsed := time.Since(start)
	assert.True(t, sawTimeoutDelta, "stream must emit a timeout error delta before closing")
	// idleTimeout + slack for goroutine wakeup and inner-channel drain.
	assert.Less(t, elapsed, 5*time.Second,
		"ChatStream must unwind within a bounded multiple of idleTimeout; took %v", elapsed)
	assert.NoError(t, ctx.Err())
}

// TestTimeoutProviderForwardsMetadata verifies the wrapper delegates the
// optional metadata interfaces (SessionStartMetadata, ChatStartMetadata) to
// the inner provider, so agentlib.Run's telemetry does not regress to
// "unknown" when the wrapper is installed.
func TestTimeoutProviderForwardsMetadata(t *testing.T) {
	inner := oai.New(oai.Config{
		BaseURL: "http://127.0.0.1:1/v1",
		APIKey:  "not-needed",
		Model:   "stub-model",
		Flavor:  "openai",
	})
	wrapped := wrapProviderWithDeadlines(inner)

	ssm, ok := wrapped.(interface {
		SessionStartMetadata() (string, string)
	})
	require.True(t, ok)
	provName, modelName := ssm.SessionStartMetadata()
	assert.Equal(t, "openai-compat", provName)
	assert.Equal(t, "stub-model", modelName)

	csm, ok := wrapped.(interface {
		ChatStartMetadata() (string, string, int)
	})
	require.True(t, ok)
	system, addr, port := csm.ChatStartMetadata()
	assert.NotEmpty(t, system)
	assert.NotEmpty(t, addr)
	assert.Equal(t, 1, port)
}

// TestTimeoutProviderChatCallerCancelNotTimeout verifies that when the
// caller cancels their own ctx (not the wrapper's deadline), the returned
// error does NOT masquerade as ErrProviderRequestTimeout.
func TestTimeoutProviderChatCallerCancelNotTimeout(t *testing.T) {
	inner := &sleepProvider{
		delay: 30 * time.Second,
	}
	wrapped := wrapProviderWithDeadlinesTimeouts(inner, 10*time.Second, 10*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, err := wrapped.Chat(ctx, nil, nil, agentlib.Options{})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrProviderRequestTimeout),
		"caller-cancel must not report as a wrapper timeout; got %v", err)
}
