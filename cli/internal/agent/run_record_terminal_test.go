package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terminalSessionQueryCountingService emits a public final event with cost and
// usage while counting provider-session query methods. Terminal persistence
// must not call those methods (ddx-281ffb67).
type terminalSessionQueryCountingService struct {
	passthroughTestService

	listSessionLogsCalls atomic.Int64
	tailSessionLogCalls  atomic.Int64
	writeSessionLogCalls atomic.Int64
	replaySessionCalls   atomic.Int64
}

func (s *terminalSessionQueryCountingService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	s.lastReq = req
	s.executeRequests = append(s.executeRequests, req)
	if s.executeErr != nil {
		return nil, s.executeErr
	}

	inTok, outTok, total, cache := 1200, 340, 1540, 80
	cost := 0.042
	routingPayload, err := json.Marshal(map[string]any{
		"harness":    "claude",
		"provider":   "anthropic",
		"model":      "claude-sonnet-4",
		"reason":     "fixture_public_route",
		"session_id": "fizeau-public-sess-281ffb67",
	})
	if err != nil {
		return nil, err
	}
	finalPayload, err := json.Marshal(map[string]any{
		"status":           "success",
		"exit_code":        0,
		"final_text":       "public final text for terminal substrate",
		"duration_ms":      2100,
		"session_log_path": "/var/fizeau/sessions/281ffb67.jsonl",
		// cost_source is required for Fizeau's public cost normalization to
		// retain cost_usd (source-less scalars are dropped as non-authoritative).
		"cost_usd":    cost,
		"cost_source": "reported",
		"usage": map[string]any{
			"input_tokens":      inTok,
			"output_tokens":     outTok,
			"total_tokens":      total,
			"cache_read_tokens": cache,
		},
		"routing_actual": map[string]any{
			"harness":  "claude",
			"provider": "anthropic",
			"model":    "claude-sonnet-4",
			"power":    70,
		},
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan agentlib.ServiceEvent, 2)
	ch <- agentlib.ServiceEvent{Type: "routing_decision", Data: routingPayload}
	ch <- agentlib.ServiceEvent{Type: "final", Data: finalPayload}
	close(ch)
	return ch, nil
}

func (s *terminalSessionQueryCountingService) ListSessionLogs(ctx context.Context) ([]agentlib.SessionLogEntry, error) {
	s.listSessionLogsCalls.Add(1)
	return nil, nil
}

func (s *terminalSessionQueryCountingService) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	s.tailSessionLogCalls.Add(1)
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *terminalSessionQueryCountingService) WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error {
	s.writeSessionLogCalls.Add(1)
	return nil
}

func (s *terminalSessionQueryCountingService) ReplaySession(ctx context.Context, sessionID string, w io.Writer) error {
	s.replaySessionCalls.Add(1)
	return nil
}

// TestRunRecordTerminalOutcomeFields drives typed immediate-error and
// public-final-event (+ repository evaluation) paths and proves the existing
// record reaches terminal with typed outcome fields, completed timestamp,
// cost/token fields when available, and repository evidence links — without
// provider session queries (AC1–AC3 / ddx-281ffb67).
func TestRunRecordTerminalOutcomeFields(t *testing.T) {
	t.Run("immediate_error", func(t *testing.T) {
		projectRoot := t.TempDir()
		const (
			attemptID = "20260726T062408-281ffb67-imm"
			beadID    = "ddx-281ffb67"
			workerID  = "worker-terminal-imm"
			baseRev   = "deadbeef281ffb67"
			sessionID = "eb-provider-session-must-not-be-queried"
			promptRel = ".ddx/executions/" + attemptID + "/prompt.md"
		)

		svc := &preDispatchFailureTripwireService{
			passthroughTestService: passthroughTestService{
				executeErr: &agentlib.ErrHarnessModelIncompatible{Harness: "claude", Model: "gpt-foo"},
			},
			projectRoot: projectRoot,
			recordKey:   attemptID,
		}
		rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

		var dispatchingAtStart *runrecord.Record
		_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
			Prompt: "terminalize from typed immediate error",
			Correlation: map[string]string{
				"bead_id":     beadID,
				"attempt_id":  attemptID,
				"session_id":  sessionID,
				"worker_id":   workerID,
				"bundle_path": ".ddx/executions/" + attemptID,
				"prompt_file": promptRel,
				"base_rev":    baseRev,
			},
			CorrelationID: beadID + ":" + attemptID,
			OnExecuteStart: func() {
				dispatchingAtStart, _ = runrecord.Read(projectRoot, attemptID)
			},
		})
		require.Error(t, err)
		var pfErr *ProviderFailureError
		require.ErrorAs(t, err, &pfErr)
		require.NotNil(t, dispatchingAtStart)
		assert.Equal(t, runrecord.PhaseDispatching, dispatchingAtStart.Phase)
		startedAt := dispatchingAtStart.StartedAt

		// AC1: existing record reaches terminal with typed outcome + completed timestamp.
		loaded, err := runrecord.Read(projectRoot, attemptID)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, runrecord.PhaseTerminal, loaded.Phase)
		assert.Equal(t, attemptID, loaded.AttemptID)
		assert.Equal(t, beadID, loaded.BeadID)
		assert.True(t, loaded.StartedAt.Equal(startedAt), "started_at must survive terminal finalize")
		require.NotNil(t, loaded.FinishedAt, "completed/finished timestamp must be set")
		assert.False(t, loaded.FinishedAt.IsZero())
		require.NotNil(t, loaded.Outcome)
		assert.Equal(t, "failure", loaded.Outcome.Status)
		assert.Equal(t, FailureModeProviderModelUnavailable, loaded.Outcome.Reason)
		assert.Equal(t, "immediate_error", loaded.Outcome.EvidenceVerdict)
		require.NotNil(t, loaded.Fizeau)
		assert.Equal(t, FailureModeProviderModelUnavailable, loaded.Fizeau.ImmediateError)

		// Correlation evidence survives.
		assert.Equal(t, promptRel, evidencePathByName(loaded.Evidence, "prompt"))
		assert.Equal(t, workerID, evidencePathByName(loaded.Evidence, "worker_id"))

		// AC3: no provider-session identity or process fields.
		raw, err := os.ReadFile(runrecord.RecordPath(projectRoot, attemptID))
		require.NoError(t, err)
		require.True(t, json.Valid(raw))
		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))
		for _, forbidden := range []string{
			"harness", "provider", "model", "route_reason",
			"raw_output", "provider_output", "pid", "provider_pid", "process_tree",
			"session_canonical_state", "provider_session_state",
			"provider_session_canonical_state", "canonical_state",
		} {
			_, ok := asMap[forbidden]
			assert.False(t, ok, "terminal immediate-error record must not contain %q", forbidden)
		}
		assert.NotEqual(t, sessionID, loaded.AttemptID)
		_, err = os.Stat(runrecord.RecordPath(projectRoot, sessionID))
		assert.True(t, os.IsNotExist(err), "must not key substrate by provider session id")
	})

	t.Run("public_final_with_repository_evaluation", func(t *testing.T) {
		projectRoot := t.TempDir()
		const (
			attemptID       = "20260726T062408-281ffb67-fin"
			beadID          = "ddx-281ffb67"
			publicSessionID = "fizeau-public-sess-281ffb67"
			bundleRel       = ".ddx/executions/" + attemptID
		)

		// Seed repository evaluation artifacts under the attempt bundle.
		bundleDir := filepath.Join(projectRoot, bundleRel)
		require.NoError(t, os.MkdirAll(bundleDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "result.json"), []byte(`{"status":"ok"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "prompt.md"), []byte("# prompt\n"), 0o644))

		svc := &terminalSessionQueryCountingService{}
		rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

		var dispatchingAtStart *runrecord.Record
		_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
			Prompt: "terminalize from public final + repository evaluation",
			Correlation: map[string]string{
				"bead_id":     beadID,
				"attempt_id":  attemptID,
				"session_id":  "eb-provider-session-must-not-be-queried",
				"worker_id":   "worker-terminal-fin",
				"bundle_path": bundleRel,
				"prompt_file": bundleRel + "/prompt.md",
				"base_rev":    "cafebabe281ffb67",
			},
			CorrelationID: beadID + ":" + attemptID,
			OnExecuteStart: func() {
				dispatchingAtStart, _ = runrecord.Read(projectRoot, attemptID)
			},
		})
		require.NoError(t, err)
		require.True(t, svc.executeCalled)
		require.NotNil(t, dispatchingAtStart)
		assert.Equal(t, runrecord.PhaseDispatching, dispatchingAtStart.Phase)
		startedAt := dispatchingAtStart.StartedAt

		// AC2: terminal with outcome, completed timestamp, cost/tokens, evidence links.
		loaded, err := runrecord.Read(projectRoot, attemptID)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, runrecord.PhaseTerminal, loaded.Phase)
		assert.Equal(t, attemptID, loaded.AttemptID)
		assert.Equal(t, beadID, loaded.BeadID)
		assert.True(t, loaded.StartedAt.Equal(startedAt))
		require.NotNil(t, loaded.FinishedAt)
		assert.False(t, loaded.FinishedAt.IsZero())
		require.NotNil(t, loaded.Outcome)
		assert.Equal(t, "success", loaded.Outcome.Status)
		assert.Equal(t, "public_final_ok", loaded.Outcome.Reason)
		assert.Equal(t, "result_artifact_present", loaded.Outcome.EvidenceVerdict)

		require.NotNil(t, loaded.Fizeau)
		assert.Equal(t, "/var/fizeau/sessions/281ffb67.jsonl", loaded.Fizeau.SessionLogPath)
		assert.Equal(t, "success", loaded.Fizeau.FinalStatus)
		require.NotNil(t, loaded.Fizeau.FinalExitCode)
		assert.Equal(t, 0, *loaded.Fizeau.FinalExitCode)
		// Cost/token fields from public final usage.
		require.NotNil(t, loaded.Fizeau.CostUSD)
		assert.InDelta(t, 0.042, *loaded.Fizeau.CostUSD, 1e-9)
		require.NotNil(t, loaded.Fizeau.InputTokens)
		assert.Equal(t, 1200, *loaded.Fizeau.InputTokens)
		require.NotNil(t, loaded.Fizeau.OutputTokens)
		assert.Equal(t, 340, *loaded.Fizeau.OutputTokens)
		require.NotNil(t, loaded.Fizeau.TotalTokens)
		assert.Equal(t, 1540, *loaded.Fizeau.TotalTokens)
		require.NotNil(t, loaded.Fizeau.CachedTokens)
		assert.Equal(t, 80, *loaded.Fizeau.CachedTokens)
		// Public session ref from earlier routing event is preserved via merge.
		assert.Equal(t, publicSessionID, loaded.Fizeau.PublicSessionRef)

		// Repository evidence links from evaluation of bundle artifacts.
		assert.Equal(t, bundleRel, evidencePathByName(loaded.Evidence, "repository_bundle"))
		assert.Equal(t, bundleRel+"/result.json", evidencePathByName(loaded.Evidence, "result"))
		assert.Equal(t, bundleRel+"/prompt.md", evidencePathByName(loaded.Evidence, "prompt"))

		// AC3: no provider session queries; forbidden fields absent.
		assert.EqualValues(t, 0, svc.listSessionLogsCalls.Load(), "must not ListSessionLogs")
		assert.EqualValues(t, 0, svc.tailSessionLogCalls.Load(), "must not TailSessionLog")
		assert.EqualValues(t, 0, svc.writeSessionLogCalls.Load(), "must not WriteSessionLog")
		assert.EqualValues(t, 0, svc.replaySessionCalls.Load(), "must not ReplaySession")

		raw, err := os.ReadFile(runrecord.RecordPath(projectRoot, attemptID))
		require.NoError(t, err)
		require.True(t, json.Valid(raw))
		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))
		for _, forbidden := range []string{
			"harness", "provider", "model", "route_reason",
			"routing_policy", "min_power", "max_power", "profile",
			"raw_output", "provider_output", "stdout", "stderr",
			"pid", "provider_pid", "process_tree", "children_pids",
			"session_canonical_state", "provider_session_state",
			"provider_session_canonical_state", "canonical_state",
		} {
			_, ok := asMap[forbidden]
			assert.False(t, ok, "terminal final record must not contain %q", forbidden)
		}
		if fizeauRaw, ok := asMap["fizeau"].(map[string]any); ok {
			for _, forbidden := range []string{
				"harness", "provider", "model", "pid", "raw_output",
				"process_tree", "canonical_state", "routing_actual",
			} {
				_, has := fizeauRaw[forbidden]
				assert.False(t, has, "fizeau public object must not contain %q", forbidden)
			}
			assert.Equal(t, 0.042, fizeauRaw["cost_usd"])
			assert.EqualValues(t, 1200, fizeauRaw["input_tokens"])
		} else {
			t.Fatal("expected fizeau object on terminal record")
		}

		// Exactly one substrate directory keyed by the DDx attempt id.
		entries, err := os.ReadDir(filepath.Join(projectRoot, runrecord.StoreDir))
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, attemptID, entries[0].Name())
	})
}

// TestFinalizeRunRecordTerminal_NoopWithoutAttemptID ensures plain ddx run
// paths without execute-bead correlation do not create or mutate substrate.
func TestFinalizeRunRecordTerminal_NoopWithoutAttemptID(t *testing.T) {
	projectRoot := t.TempDir()
	err := finalizeRunRecordFromImmediateError(projectRoot, AgentRunRuntime{Prompt: "no attempt"}, ProviderFailure{
		Reason: FailureModeProviderModelUnavailable,
	})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(projectRoot, runrecord.StoreDir))
	assert.True(t, os.IsNotExist(err))
}
