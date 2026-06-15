package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingToolService is a FizeauService stub that streams distinct
// tool_call/tool_result events until its Execute context is cancelled. It
// embeds resolveRouteFailingService for the rest of the interface and records
// whether the absolute request-timeout cap cancelled the session.
type streamingToolService struct {
	*resolveRouteFailingService
	mu       sync.Mutex
	canceled bool
	events   int
}

func (s *streamingToolService) markCanceled() {
	s.mu.Lock()
	s.canceled = true
	s.mu.Unlock()
}

func (s *streamingToolService) wasCanceled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceled
}

func (s *streamingToolService) eventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events
}

func (s *streamingToolService) Execute(ctx context.Context, _ agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	ch := make(chan agentlib.ServiceEvent)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				s.markCanceled()
				return
			case <-ticker.C:
				i++
				toolCall := agentlib.ServiceEvent{
					Type: agentlib.ServiceEventTypeToolCall,
					Time: time.Now(),
					Data: json.RawMessage(fmt.Sprintf(`{"id":"tc%d","name":"Bash","input":{"command":"echo %d"}}`, i, i)),
				}
				toolResult := agentlib.ServiceEvent{
					Type: agentlib.ServiceEventTypeToolResult,
					Time: time.Now(),
					Data: json.RawMessage(fmt.Sprintf(`{"id":"tc%d","output":"out %d","error":""}`, i, i)),
				}
				if !s.send(ctx, ch, toolCall) || !s.send(ctx, ch, toolResult) {
					return
				}
				s.mu.Lock()
				s.events++
				s.mu.Unlock()
			}
		}
	}()
	return ch, nil
}

func (s *streamingToolService) send(ctx context.Context, ch chan agentlib.ServiceEvent, ev agentlib.ServiceEvent) bool {
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		s.markCanceled()
		return false
	}
}

// TestServerManagedWorkerForwardsRequestTimeoutToAbsoluteCap proves the server
// worker path forwards a worker spec's request timeout into the agent run as
// the absolute provider-session wall-clock cap: a session that streams tool
// events forever is cancelled at the configured timeout (ddx-9febbad2).
func TestServerManagedWorkerForwardsRequestTimeoutToAbsoluteCap(t *testing.T) {
	const requestTimeout = 300 * time.Millisecond
	spec := ExecuteLoopWorkerSpec{
		Provider:       "openrouter",
		Model:          "qwen/qwen3.6",
		RequestTimeout: executeloop.Duration{Duration: requestTimeout},
	}

	// Forward the spec request timeout exactly as runWorker does.
	overrides := config.CLIOverrides{Provider: spec.Provider, Model: spec.Model}
	applyRequestTimeoutOverride(&overrides, spec.RequestTimeout.Duration)
	require.NotNil(t, overrides.ProviderRequestTimeout)
	require.Equal(t, requestTimeout, *overrides.ProviderRequestTimeout)

	root := t.TempDir()
	rcfg, err := config.LoadAndResolve(root, overrides)
	require.NoError(t, err)
	require.Equal(t, requestTimeout, rcfg.ProviderRequestTimeout())
	require.Equal(t, requestTimeout, agent.ResolveRequestTimeoutCap(root, spec.Provider, rcfg.ProviderRequestTimeout()),
		"forwarded request timeout must resolve to the agent's absolute cap")

	stub := &streamingToolService{resolveRouteFailingService: &resolveRouteFailingService{}}
	agent.SetServiceRunFactory(func(string) (agentlib.FizeauService, error) { return stub, nil })
	t.Cleanup(func() { agent.SetServiceRunFactory(nil) })

	start := time.Now()
	_, runErr := agent.RunWithConfigViaService(context.Background(), root, rcfg, agent.AgentRunRuntime{Prompt: "hello"})
	elapsed := time.Since(start)

	// The cap cancels the drain before any final event arrives, so the run ends
	// in a provider failure rather than success — the point is it ends promptly.
	require.Error(t, runErr, "request-timeout cancellation must surface as a run failure, not a hang")
	assert.Less(t, elapsed, 5*time.Second,
		"absolute cap must cancel the streaming session near the configured timeout, not run unbounded")
	assert.Greater(t, stub.eventCount(), 1,
		"provider must stream multiple tool events past the cap before being cancelled")
	// The cancel propagates to the provider goroutine asynchronously after the
	// drain returns; give it a moment to observe ctx.Done.
	require.Eventually(t, stub.wasCanceled, time.Second, 5*time.Millisecond,
		"provider context must be cancelled when the absolute cap fires")
}
