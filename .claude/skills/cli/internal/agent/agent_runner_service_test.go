package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentlib "github.com/DocumentDrivenDX/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrainServiceEventsNoopCompactionWallClockBreaker(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 400)
	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for elapsed := time.Duration(0); elapsed <= serviceNoopCompactionWallClockLimit; elapsed += 3 * time.Second {
		events <- noopCompactionServiceEvent(start.Add(elapsed))
	}
	close(events)

	final, _, _ := drainServiceEvents(events)
	require.NotNil(t, final)
	assert.Equal(t, "stalled", final.Status)
	assert.Contains(t, final.Error, serviceNoopCompactionWallClockReason)
	assert.Contains(t, final.Error, "time-based breaker")
	assert.Contains(t, final.Error, "15m0s")
}

func TestDrainServiceEventsProgressResetsNoopCompactionWallClockBreaker(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 700)
	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for elapsed := time.Duration(0); elapsed < serviceNoopCompactionWallClockLimit; elapsed += 3 * time.Second {
		events <- noopCompactionServiceEvent(start.Add(elapsed))
	}
	events <- agentlib.ServiceEvent{
		Type: "tool_call",
		Time: start.Add(serviceNoopCompactionWallClockLimit),
		Data: json.RawMessage(`{"id":"call-1","name":"read","input":{"path":"README.md"}}`),
	}
	for elapsed := 3 * time.Second; elapsed < serviceNoopCompactionWallClockLimit; elapsed += 3 * time.Second {
		events <- noopCompactionServiceEvent(start.Add(serviceNoopCompactionWallClockLimit + elapsed))
	}
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: start.Add(2 * serviceNoopCompactionWallClockLimit),
		Data: json.RawMessage(`{"status":"success","exit_code":0,"duration_ms":1}`),
	}
	close(events)

	final, _, _ := drainServiceEvents(events)
	require.NotNil(t, final)
	assert.Equal(t, "success", final.Status)
	assert.Empty(t, final.Error)
}

func TestExecuteBeadResultDetailReportsNoopCompactionWallClockBreaker(t *testing.T) {
	const beadID = "ddx-compaction-stuck"

	projectRoot := setupArtifactTestProjectRoot(t)
	gitOps := &artifactTestGitOps{
		projectRoot: projectRoot,
		baseRev:     "aaaa000000000001",
		resultRev:   "aaaa000000000001",
		wtSetupFn: func(wtPath string) {
			setupArtifactTestWorktree(t, wtPath, beadID, "", false, 0)
		},
	}

	res, err := ExecuteBead(context.Background(), projectRoot, beadID, ExecuteBeadOptions{
		Harness: "agent",
		Service: &noopCompactionDdxAgent{
			interval: 3 * time.Second,
			total:    serviceNoopCompactionWallClockLimit + time.Minute,
		},
	}, gitOps)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, ExecuteBeadOutcomeTaskFailed, res.Outcome)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, res.Status)
	assert.Contains(t, res.Detail, serviceNoopCompactionWallClockReason)
	assert.Contains(t, res.Detail, "time-based breaker")

	raw, err := os.ReadFile(filepath.Join(projectRoot, ".ddx", "executions", res.AttemptID, "result.json"))
	require.NoError(t, err)
	var artifact ExecuteBeadResult
	require.NoError(t, json.Unmarshal(raw, &artifact))
	assert.Contains(t, artifact.Detail, serviceNoopCompactionWallClockReason)
	assert.Contains(t, artifact.Detail, "time-based breaker")
}

func noopCompactionServiceEvent(ts time.Time) agentlib.ServiceEvent {
	return agentlib.ServiceEvent{
		Type: "compaction",
		Time: ts,
		Data: json.RawMessage(`{"no_compaction":true,"messages_before":42,"messages_after":42}`),
	}
}

type noopCompactionDdxAgent struct {
	interval time.Duration
	total    time.Duration
}

func (s *noopCompactionDdxAgent) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	events := make(chan agentlib.ServiceEvent, 400)
	go func() {
		defer close(events)
		start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
		routingData, _ := json.Marshal(map[string]string{
			"harness":  "agent",
			"provider": "fake",
			"model":    "fake-model",
		})
		if !sendServiceEvent(ctx, events, agentlib.ServiceEvent{
			Type: "routing_decision",
			Time: start,
			Data: routingData,
		}) {
			return
		}
		for elapsed := time.Duration(0); elapsed <= s.total; elapsed += s.interval {
			if !sendServiceEvent(ctx, events, noopCompactionServiceEvent(start.Add(elapsed))) {
				return
			}
		}
	}()
	return events, nil
}

func sendServiceEvent(ctx context.Context, events chan<- agentlib.ServiceEvent, ev agentlib.ServiceEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- ev:
		return true
	}
}

func (s *noopCompactionDdxAgent) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	events := make(chan agentlib.ServiceEvent)
	close(events)
	return events, nil
}

func (s *noopCompactionDdxAgent) ListHarnesses(ctx context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "agent", Available: true}}, nil
}

func (s *noopCompactionDdxAgent) ListProviders(ctx context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *noopCompactionDdxAgent) ListModels(ctx context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}

func (s *noopCompactionDdxAgent) HealthCheck(ctx context.Context, target agentlib.HealthTarget) error {
	return nil
}

func (s *noopCompactionDdxAgent) ResolveRoute(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *noopCompactionDdxAgent) RouteStatus(ctx context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *noopCompactionDdxAgent) ListProfiles(ctx context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}

func (s *noopCompactionDdxAgent) ResolveProfile(ctx context.Context, name string) (*agentlib.ResolvedProfile, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *noopCompactionDdxAgent) ProfileAliases(ctx context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *noopCompactionDdxAgent) RecordRouteAttempt(ctx context.Context, attempt agentlib.RouteAttempt) error {
	return nil
}
