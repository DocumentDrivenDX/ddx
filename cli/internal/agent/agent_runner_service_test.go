package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/DocumentDrivenDX/fizeau"
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

	final, _, _, _ := drainServiceEvents(events)
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

	final, _, _, _ := drainServiceEvents(events)
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

	cfg := config.NewTestConfigForBead(config.TestBeadConfigOpts{
		Harness: "agent",
	})
	rcfg := cfg.Resolve(config.CLIOverrides{})
	res, err := ExecuteBeadWithConfig(context.Background(), projectRoot, beadID, rcfg, ExecuteBeadRuntime{
		Service: &noopCompactionFizeauService{
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

// TestDrainServiceEvents_ExtractsPowerFromRoutingDecisionCandidates covers
// AC#2/AC#4 of ddx-1534c574: the routing_decision event's winning candidate
// (eligible=true, model matches payload.model) carries Components.Power; DDx
// must surface this as ActualPower without touching the final event.
func TestDrainServiceEvents_ExtractsPowerFromRoutingDecisionCandidates(t *testing.T) {
	events := make(chan agentlib.ServiceEvent, 4)

	routingPayload, err := json.Marshal(map[string]any{
		"harness":  "agent",
		"provider": "anthropic",
		"model":    "claude-3-5-sonnet",
		"candidates": []map[string]any{
			{
				"model":      "claude-3-haiku",
				"eligible":   false,
				"components": map[string]any{"power": 20},
			},
			{
				"model":      "claude-3-5-sonnet",
				"eligible":   true,
				"components": map[string]any{"power": 65},
			},
		},
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "routing_decision",
		Time: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Data: routingPayload,
	}

	finalPayload, err := json.Marshal(map[string]any{
		"status":     "success",
		"exit_code":  0,
		"final_text": "done",
	})
	require.NoError(t, err)
	events <- agentlib.ServiceEvent{
		Type: "final",
		Time: time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC),
		Data: finalPayload,
	}
	close(events)

	_, _, _, actualPower := drainServiceEvents(events)
	assert.Equal(t, 65, actualPower,
		"power must come from the eligible winning candidate in routing_decision.candidates")
}

func noopCompactionServiceEvent(ts time.Time) agentlib.ServiceEvent {
	return agentlib.ServiceEvent{
		Type: "compaction",
		Time: ts,
		Data: json.RawMessage(`{"no_compaction":true,"messages_before":42,"messages_after":42}`),
	}
}

type noopCompactionFizeauService struct {
	interval time.Duration
	total    time.Duration
}

func (s *noopCompactionFizeauService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
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

func (s *noopCompactionFizeauService) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	events := make(chan agentlib.ServiceEvent)
	close(events)
	return events, nil
}

func (s *noopCompactionFizeauService) ListHarnesses(ctx context.Context) ([]agentlib.HarnessInfo, error) {
	return []agentlib.HarnessInfo{{Name: "agent", Available: true}}, nil
}

func (s *noopCompactionFizeauService) ListProviders(ctx context.Context) ([]agentlib.ProviderInfo, error) {
	return nil, nil
}

func (s *noopCompactionFizeauService) ListModels(ctx context.Context, filter agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
	return nil, nil
}

func (s *noopCompactionFizeauService) HealthCheck(ctx context.Context, target agentlib.HealthTarget) error {
	return nil
}

func (s *noopCompactionFizeauService) ResolveRoute(ctx context.Context, req agentlib.RouteRequest) (*agentlib.RouteDecision, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *noopCompactionFizeauService) RouteStatus(ctx context.Context) (*agentlib.RouteStatusReport, error) {
	return nil, nil
}

func (s *noopCompactionFizeauService) ListProfiles(ctx context.Context) ([]agentlib.ProfileInfo, error) {
	return nil, nil
}

func (s *noopCompactionFizeauService) ResolveProfile(ctx context.Context, name string) (*agentlib.ResolvedProfile, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *noopCompactionFizeauService) ProfileAliases(ctx context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *noopCompactionFizeauService) RecordRouteAttempt(ctx context.Context, attempt agentlib.RouteAttempt) error {
	return nil
}

func (s *noopCompactionFizeauService) ListSessionLogs(ctx context.Context) ([]agentlib.SessionLogEntry, error) {
	return nil, nil
}
func (s *noopCompactionFizeauService) WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}
func (s *noopCompactionFizeauService) ReplaySession(ctx context.Context, sessionID string, w io.Writer) error {
	return nil
}
func (s *noopCompactionFizeauService) UsageReport(ctx context.Context, opts agentlib.UsageReportOptions) (*agentlib.UsageReport, error) {
	return nil, nil
}
