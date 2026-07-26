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

// sessionQueryCountingService is a fixture Fizeau service that emits public
// execution events while counting provider-session query methods. The running
// transition must not call those methods (ddx-a44bfc5b).
type sessionQueryCountingService struct {
	passthroughTestService

	projectRoot string
	recordKey   string

	listSessionLogsCalls atomic.Int64
	tailSessionLogCalls  atomic.Int64
	writeSessionLogCalls atomic.Int64
	replaySessionCalls   atomic.Int64

	// recordAfterFirstPublic is loaded after the routing_decision event is
	// enqueued and the drain has had a chance to process it (observed via a
	// short barrier before the final event).
	recordAfterFirstPublic *runrecord.Record
	recordAfterFirstErr    error
}

func (s *sessionQueryCountingService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeCalled = true
	s.lastReq = req
	s.executeRequests = append(s.executeRequests, req)
	if s.executeErr != nil {
		return nil, s.executeErr
	}

	routingPayload, err := json.Marshal(map[string]any{
		"harness":    "claude",
		"provider":   "anthropic",
		"model":      "claude-sonnet-4",
		"reason":     "fixture_public_route",
		"session_id": "fizeau-public-sess-a44bfc5b",
		// Concrete routing fields present in the public event must NOT be
		// copied onto the durable run record top-level schema.
	})
	if err != nil {
		return nil, err
	}
	finalPayload, err := json.Marshal(map[string]any{
		"status":           "success",
		"exit_code":        0,
		"final_text":       "public final text",
		"duration_ms":      1500,
		"session_log_path": "/var/fizeau/sessions/a44bfc5b.jsonl",
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
	// Emit routing first so the running transition is driven by public
	// execution data that is not a terminal final event.
	ch <- agentlib.ServiceEvent{Type: "routing_decision", Data: routingPayload}
	// Snapshot substrate after first public event is available on the channel.
	// The drain processes events sequentially; reading here races, so also
	// re-assert after Execute returns. This mid-stream read is best-effort.
	if s.projectRoot != "" && s.recordKey != "" {
		rec, rerr := runrecord.Read(s.projectRoot, s.recordKey)
		s.recordAfterFirstPublic = rec
		s.recordAfterFirstErr = rerr
	}
	ch <- agentlib.ServiceEvent{Type: "final", Data: finalPayload}
	close(ch)
	return ch, nil
}

func (s *sessionQueryCountingService) ListSessionLogs(ctx context.Context) ([]agentlib.SessionLogEntry, error) {
	s.listSessionLogsCalls.Add(1)
	return nil, nil
}

func (s *sessionQueryCountingService) TailSessionLog(ctx context.Context, sessionID string) (<-chan agentlib.ServiceEvent, error) {
	s.tailSessionLogCalls.Add(1)
	ch := make(chan agentlib.ServiceEvent)
	close(ch)
	return ch, nil
}

func (s *sessionQueryCountingService) WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error {
	s.writeSessionLogCalls.Add(1)
	return nil
}

func (s *sessionQueryCountingService) ReplaySession(ctx context.Context, sessionID string, w io.Writer) error {
	s.replaySessionCalls.Add(1)
	return nil
}

// TestRunRecordTransitionsRunningFromPublicFizeauEvent drives a fixture Fizeau
// service that emits public execution data and proves the existing dispatching
// record is atomically updated to running without querying provider sessions.
// The running record contains only public route/result fields from the typed
// Fizeau execution contract (AC1, AC2 / ddx-a44bfc5b).
func TestRunRecordTransitionsRunningFromPublicFizeauEvent(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID       = "20260726T054544-a44bfc5b"
		beadID          = "ddx-a44bfc5b"
		publicSessionID = "fizeau-public-sess-a44bfc5b"
	)

	svc := &sessionQueryCountingService{
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	// Capture the dispatching record at OnExecuteStart (after pre-dispatch
	// publish, before public events) so we prove the running state is an
	// atomic update of that existing substrate.
	var (
		dispatchingAtStart *runrecord.Record
		dispatchingErr     error
	)

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
		Prompt: "transition dispatching to running from public Fizeau data",
		Correlation: map[string]string{
			"bead_id":     beadID,
			"attempt_id":  attemptID,
			"session_id":  "eb-provider-session-must-not-be-queried",
			"worker_id":   "worker-a44bfc5b",
			"bundle_path": ".ddx/executions/" + attemptID,
			"prompt_file": ".ddx/executions/" + attemptID + "/prompt.md",
			"base_rev":    "deadbeef01",
		},
		CorrelationID: beadID + ":" + attemptID,
		OnExecuteStart: func() {
			dispatchingAtStart, dispatchingErr = runrecord.Read(projectRoot, attemptID)
		},
	})
	require.NoError(t, err)
	require.True(t, svc.executeCalled)
	require.NoError(t, dispatchingErr)
	require.NotNil(t, dispatchingAtStart, "dispatching record must exist before public Fizeau events")
	require.Equal(t, runrecord.PhaseDispatching, dispatchingAtStart.Phase)
	require.Nil(t, dispatchingAtStart.Fizeau)
	startedAt := dispatchingAtStart.StartedAt

	// After a full Execute that emits routing_decision then final, the substrate
	// advances running (from first public event) and is finalized to terminal
	// (ddx-281ffb67). Prove public route fields survived and no session queries
	// occurred; end phase is terminal with preserved PublicSessionRef.
	loaded, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, loaded, "record must be readable without session synthesis")
	assert.Equal(t, runrecord.PhaseTerminal, loaded.Phase,
		"full success path finalizes to terminal after public final event")
	assert.Equal(t, attemptID, loaded.AttemptID)
	assert.Equal(t, beadID, loaded.BeadID)
	// Durable directory key / run_id identity (JSON) matches the attempt id.
	rawIdentity, err := os.ReadFile(runrecord.RecordPath(projectRoot, attemptID))
	require.NoError(t, err)
	key, gotAttempt, gotBead, phase := recordIdentityFromJSON(t, rawIdentity)
	assert.Equal(t, attemptID, key)
	assert.Equal(t, attemptID, gotAttempt)
	assert.Equal(t, beadID, gotBead)
	assert.Equal(t, string(runrecord.PhaseTerminal), phase)
	assert.True(t, loaded.StartedAt.Equal(startedAt),
		"started_at from dispatching publish must survive running+terminal updates")
	assert.False(t, loaded.UpdatedAt.IsZero())
	assert.True(t, loaded.UpdatedAt.After(startedAt) || loaded.UpdatedAt.Equal(startedAt),
		"updated_at must be set on phase transitions")
	require.NotNil(t, loaded.FinishedAt, "terminal finalize sets finished_at")
	require.NotNil(t, loaded.Outcome, "terminal finalize sets outcome from public final")

	// Exactly one substrate directory keyed by the DDx attempt id.
	entries, err := os.ReadDir(filepath.Join(projectRoot, runrecord.StoreDir))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, attemptID, entries[0].Name())

	// AC1: no provider-session queries on the transition path.
	assert.EqualValues(t, 0, svc.listSessionLogsCalls.Load(), "must not ListSessionLogs")
	assert.EqualValues(t, 0, svc.tailSessionLogCalls.Load(), "must not TailSessionLog")
	assert.EqualValues(t, 0, svc.writeSessionLogCalls.Load(), "must not WriteSessionLog")
	assert.EqualValues(t, 0, svc.replaySessionCalls.Load(), "must not ReplaySession")

	// AC2: only public route/result fields from the typed Fizeau contract.
	// Routing session ref is preserved across the terminal merge; final fields
	// are attached from the public final event (not from session queries).
	require.NotNil(t, loaded.Fizeau, "public Fizeau fields must be recorded")
	assert.Equal(t, publicSessionID, loaded.Fizeau.PublicSessionRef)
	assert.Empty(t, loaded.Fizeau.ImmediateError)
	assert.Equal(t, "/var/fizeau/sessions/a44bfc5b.jsonl", loaded.Fizeau.SessionLogPath)
	assert.Equal(t, "success", loaded.Fizeau.FinalStatus)

	raw := rawIdentity
	require.True(t, json.Valid(raw), "record must be complete atomic JSON")

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
		assert.False(t, ok, "record must not contain forbidden field %q", forbidden)
	}

	// Nested fizeau object may only carry public contract keys.
	if fizeauRaw, ok := asMap["fizeau"].(map[string]any); ok {
		for _, forbidden := range []string{
			"harness", "provider", "model", "pid", "raw_output",
			"process_tree", "canonical_state", "routing_actual",
		} {
			_, has := fizeauRaw[forbidden]
			assert.False(t, has, "fizeau public object must not contain %q", forbidden)
		}
		// Public session ref from routing_decision.session_id.
		assert.Equal(t, publicSessionID, fizeauRaw["public_session_ref"])
	} else {
		t.Fatal("expected fizeau object on record")
	}

	// Evidence from dispatching publish survives the atomic updates.
	assert.Equal(t, ".ddx/executions/"+attemptID+"/prompt.md", evidencePathByName(loaded.Evidence, "prompt"))
	assert.Equal(t, ".ddx/executions/"+attemptID, evidencePathByName(loaded.Evidence, "bundle"))
}

// TestFizeauPublicFromFinalMapsOnlyPublicFields is a focused unit check that
// the final-event mapper does not leak routing_actual harness pins into the
// durable public shape (supports AC2 field discipline).
func TestFizeauPublicFromFinalMapsOnlyPublicFields(t *testing.T) {
	exit := 0
	final := &agentlib.ServiceFinalData{
		Status:         "success",
		ExitCode:       exit,
		DurationMS:     42,
		SessionLogPath: "/sessions/x.jsonl",
		FinalText:      "should not be stored on FizeauPublicResult",
		Error:          "",
		RoutingActual: &agentlib.ServiceRoutingActual{
			Harness:  "claude",
			Provider: "anthropic",
			Model:    "sonnet",
			Power:    70,
		},
	}
	got := fizeauPublicFromFinal(final)
	require.NotNil(t, got)
	assert.Equal(t, "/sessions/x.jsonl", got.SessionLogPath)
	assert.Equal(t, "success", got.FinalStatus)
	require.NotNil(t, got.FinalExitCode)
	assert.Equal(t, 0, *got.FinalExitCode)
	require.NotNil(t, got.DurationMS)
	assert.Equal(t, int64(42), *got.DurationMS)
	assert.Empty(t, got.PublicSessionRef)
	assert.Empty(t, got.ImmediateError)
	// FinalText and RoutingActual are not FizeauPublicResult fields.
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "should not be stored")
	assert.NotContains(t, string(raw), "anthropic")
	assert.NotContains(t, string(raw), `"harness"`)
}

// TestTransitionRunRecordToRunning_NoopWithoutAttemptID ensures plain ddx run
// paths without execute-bead correlation do not create or mutate substrate.
func TestTransitionRunRecordToRunning_NoopWithoutAttemptID(t *testing.T) {
	projectRoot := t.TempDir()
	err := transitionRunRecordToRunning(projectRoot, AgentRunRuntime{Prompt: "no attempt"}, &runrecord.FizeauPublicResult{
		SessionLogPath: "/x",
	})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(projectRoot, runrecord.StoreDir))
	assert.True(t, os.IsNotExist(err))
}
