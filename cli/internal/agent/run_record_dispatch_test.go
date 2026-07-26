package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent/runrecord"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tripwireService observes Fizeau Execute and loads the durable run record at
// the exact moment Execute is entered, proving the substrate was published first.
type tripwireService struct {
	passthroughTestService
	projectRoot string
	// recordKey is the DDx attempt identifier used as the substrate directory name.
	recordKey string

	executeEntered   bool
	recordAtExecute  *runrecord.Record
	recordReadErr    error
	recordPathExists bool
	rawAtExecute     []byte
}

func (s *tripwireService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeEntered = true
	path := runrecord.RecordPath(s.projectRoot, s.recordKey)
	if raw, err := os.ReadFile(path); err == nil {
		s.recordPathExists = true
		s.rawAtExecute = append([]byte(nil), raw...)
	}
	rec, err := runrecord.Read(s.projectRoot, s.recordKey)
	s.recordAtExecute = rec
	s.recordReadErr = err
	return s.passthroughTestService.Execute(ctx, req)
}

// recordIdentityFromJSON extracts durable identity fields via JSON keys so
// this package does not reintroduce legacy Go identifier vocabulary.
func recordIdentityFromJSON(t *testing.T, raw []byte) (recordKey, attemptID, beadID, phase string) {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	recordKey, _ = m["run_id"].(string)
	attemptID, _ = m["attempt_id"].(string)
	beadID, _ = m["bead_id"].(string)
	phase, _ = m["phase"].(string)
	return recordKey, attemptID, beadID, phase
}

// TestRunRecordExistsBeforeFizeauDispatch installs a Fizeau dispatch tripwire
// and proves .ddx/runs/<run-id>/record.json exists with lifecycle phase
// dispatching before svc.Execute is called (AC1).
func TestRunRecordExistsBeforeFizeauDispatch(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID = "20260725T132000-6af9a0f3"
		beadID    = "ddx-6af9a0f3"
		sessionID = "eb-provider-session-should-not-be-run-id"
	)

	svc := &tripwireService{
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
		Prompt: "publish dispatching record before Fizeau",
		Correlation: map[string]string{
			"bead_id":     beadID,
			"attempt_id":  attemptID,
			"session_id":  sessionID,
			"worker_id":   "worker-test",
			"bundle_path": ".ddx/executions/" + attemptID,
			"prompt_file": ".ddx/executions/" + attemptID + "/prompt.md",
			"base_rev":    "abc123deadbeef",
		},
		CorrelationID: beadID + ":" + attemptID,
	})
	require.NoError(t, err)
	require.True(t, svc.executeEntered, "Fizeau Execute must be invoked")
	require.True(t, svc.executeCalled)
	require.NoError(t, svc.recordReadErr)
	require.True(t, svc.recordPathExists, "record.json must exist when Execute enters")
	require.NotNil(t, svc.recordAtExecute, "durable run record must be readable at Execute entry")
	assert.Equal(t, runrecord.PhaseDispatching, svc.recordAtExecute.Phase)
	assert.Equal(t, attemptID, svc.recordAtExecute.AttemptID)
	assert.Equal(t, beadID, svc.recordAtExecute.BeadID)
	assert.Nil(t, svc.recordAtExecute.Fizeau, "pre-dispatch Fizeau fields must stay empty")
	require.True(t, json.Valid(svc.rawAtExecute), "record.json must be complete JSON at Execute")

	key, gotAttempt, gotBead, phase := recordIdentityFromJSON(t, svc.rawAtExecute)
	assert.Equal(t, attemptID, key)
	assert.Equal(t, attemptID, gotAttempt)
	assert.Equal(t, beadID, gotBead)
	assert.Equal(t, string(runrecord.PhaseDispatching), phase)

	// Still present after the call returns. Public final event advances phase
	// to running (ddx-a44bfc5b); pre-dispatch snapshot above remains dispatching.
	loaded, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, runrecord.PhaseRunning, loaded.Phase)
	assert.Equal(t, attemptID, loaded.AttemptID)
	assert.FileExists(t, runrecord.RecordPath(projectRoot, attemptID))
}

// TestRunRecordUsesStableAttemptIdentifier verifies the pre-dispatch record
// uses the DDx attempt/run identifiers from execute_bead.go rather than a
// provider session ID or generated Fizeau route identifier (AC2).
func TestRunRecordUsesStableAttemptIdentifier(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		// Matches GenerateAttemptID format used by execute_bead.go.
		attemptID = "20260725T132100-deadbeef"
		beadID    = "ddx-bead-stable-id"
		sessionID = "eb-cafebabe" // GenerateSessionID shape — must NOT become record key
		routeID   = "fizeau-route-xyz-generated"
	)

	svc := &tripwireService{
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
		Prompt: "stable attempt identifiers",
		Correlation: map[string]string{
			"bead_id":    beadID,
			"attempt_id": attemptID,
			"session_id": sessionID,
			// Deliberate poison values that must not become the substrate key.
			"route_id":          routeID,
			"fizeau_session_id": "provider-sess-999",
		},
		CorrelationID: beadID + ":" + attemptID,
	})
	require.NoError(t, err)
	require.NotNil(t, svc.recordAtExecute)
	require.True(t, json.Valid(svc.rawAtExecute))

	key, gotAttempt, gotBead, _ := recordIdentityFromJSON(t, svc.rawAtExecute)
	assert.Equal(t, attemptID, key, "record key must be the DDx attempt identifier")
	assert.Equal(t, attemptID, gotAttempt)
	assert.Equal(t, beadID, gotBead)
	assert.NotEqual(t, sessionID, key, "record key must not be the provider session ID")
	assert.NotEqual(t, routeID, key, "record key must not be a Fizeau route identifier")
	assert.NotContains(t, key, "provider-sess")
	assert.NotContains(t, key, "fizeau-route")
	assert.True(t, strings.HasPrefix(key, "20260725T"), "record key should match attempt-id timestamp shape")
	assert.Nil(t, svc.recordAtExecute.Fizeau)
	assert.Equal(t, attemptID, svc.recordAtExecute.AttemptID)

	// Directory name under .ddx/runs/ must match the stable attempt id.
	entries, err := os.ReadDir(filepath.Join(projectRoot, runrecord.StoreDir))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, attemptID, entries[0].Name())

	// Session id must not own a sibling run directory.
	_, err = os.Stat(runrecord.RecordPath(projectRoot, sessionID))
	assert.True(t, os.IsNotExist(err), "must not publish under session_id path")
}

// TestOnExecuteStartRunsAfterDurableRunRecord verifies OnExecuteStart still
// fires for watchdog timing only after the dispatching record has been
// durably published (AC3).
func TestOnExecuteStartRunsAfterDurableRunRecord(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID = "20260725T132200-watchdog"
		beadID    = "ddx-watchdog-order"
	)

	svc := &tripwireService{
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	var (
		onStartFired           bool
		recordAtOnStart        *runrecord.Record
		recordAtOnStartErr     error
		executeCalledAtOnStart bool
		order                  []string
	)

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
		Prompt: "watchdog after durable record",
		Correlation: map[string]string{
			"bead_id":    beadID,
			"attempt_id": attemptID,
			"session_id": "eb-should-not-matter",
		},
		CorrelationID: beadID + ":" + attemptID,
		OnExecuteStart: func() {
			onStartFired = true
			order = append(order, "OnExecuteStart")
			executeCalledAtOnStart = svc.executeCalled || svc.executeEntered
			rec, err := runrecord.Read(projectRoot, attemptID)
			recordAtOnStart = rec
			recordAtOnStartErr = err
		},
	})
	require.NoError(t, err)

	order = append(order, "ExecuteReturned")
	require.True(t, onStartFired, "OnExecuteStart must still fire")
	require.NoError(t, recordAtOnStartErr)
	require.NotNil(t, recordAtOnStart, "dispatching record must exist when OnExecuteStart fires")
	assert.Equal(t, runrecord.PhaseDispatching, recordAtOnStart.Phase)
	assert.Equal(t, attemptID, recordAtOnStart.AttemptID)
	assert.False(t, executeCalledAtOnStart, "OnExecuteStart must fire before Fizeau Execute")
	require.True(t, svc.executeEntered)
	// Tripwire also saw the record at Execute entry (after OnExecuteStart).
	require.NotNil(t, svc.recordAtExecute)
	assert.Equal(t, runrecord.PhaseDispatching, svc.recordAtExecute.Phase)
	assert.Equal(t, []string{"OnExecuteStart", "ExecuteReturned"}, order)
}

// TestPublishDispatchingRunRecord_NoopWithoutAttemptID ensures plain ddx run
// paths without execute-bead correlation do not create run substrate dirs.
func TestPublishDispatchingRunRecord_NoopWithoutAttemptID(t *testing.T) {
	projectRoot := t.TempDir()
	svc := &passthroughTestService{}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg, AgentRunRuntime{
		Prompt: "no bead attempt",
	})
	require.NoError(t, err)
	require.True(t, svc.executeCalled)

	_, err = os.Stat(filepath.Join(projectRoot, runrecord.StoreDir))
	assert.True(t, os.IsNotExist(err), "must not create .ddx/runs without attempt_id")
}

// preDispatchFailureTripwireService snapshots the durable run record at
// Execute entry then returns a typed pre-dispatch Fizeau error so the failure
// path can be proven not to clobber the substrate (ddx-02270d66).
type preDispatchFailureTripwireService struct {
	passthroughTestService
	projectRoot string
	recordKey   string

	executeEntered   bool
	recordAtExecute  *runrecord.Record
	rawAtExecute     []byte
	recordPathExists bool
	recordReadErr    error
}

func (s *preDispatchFailureTripwireService) Execute(ctx context.Context, req agentlib.ServiceExecuteRequest) (<-chan agentlib.ServiceEvent, error) {
	s.executeEntered = true
	path := runrecord.RecordPath(s.projectRoot, s.recordKey)
	if raw, err := os.ReadFile(path); err == nil {
		s.recordPathExists = true
		s.rawAtExecute = append([]byte(nil), raw...)
	}
	rec, err := runrecord.Read(s.projectRoot, s.recordKey)
	s.recordAtExecute = rec
	s.recordReadErr = err
	// Always fail pre-dispatch with a typed Fizeau error after the snapshot.
	if s.executeErr != nil {
		return nil, s.executeErr
	}
	return nil, &agentlib.ErrHarnessModelIncompatible{Harness: "claude", Model: "gpt-incompatible"}
}

// preDispatchFailureRuntime builds the execute-bead-shaped correlation used by
// the pre-dispatch failure preservation tests.
func preDispatchFailureRuntime(attemptID, beadID, workerID, baseRev, sessionID, promptRel string) AgentRunRuntime {
	return AgentRunRuntime{
		Prompt: "pre-dispatch failure must preserve dispatching record",
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
	}
}

func evidencePathByName(evidence []runrecord.EvidenceLink, name string) string {
	for _, e := range evidence {
		if e.Name == name {
			return e.Path
		}
	}
	return ""
}

// TestRunRecordPreDispatchFailureLeavesDispatchingRecord makes svc.Execute
// return a typed pre-dispatch error and proves the pre-existing
// .ddx/runs/<run-id>/record.json remains present and valid JSON (AC1).
func TestRunRecordPreDispatchFailureLeavesDispatchingRecord(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID = "20260726T052022-02270d66"
		beadID    = "ddx-02270d66"
		workerID  = "worker-preserve-1"
		baseRev   = "abc123deadbeef01"
		sessionID = "eb-provider-session-must-not-own-record"
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

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg,
		preDispatchFailureRuntime(attemptID, beadID, workerID, baseRev, sessionID, promptRel))
	require.Error(t, err)
	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr, "typed pre-dispatch error must surface as ProviderFailureError")
	require.True(t, svc.executeEntered, "Fizeau Execute must have been invoked")
	require.NoError(t, svc.recordReadErr)
	require.True(t, svc.recordPathExists, "dispatching record must exist at Execute entry")
	require.NotNil(t, svc.recordAtExecute)
	assert.Equal(t, runrecord.PhaseDispatching, svc.recordAtExecute.Phase)
	require.True(t, json.Valid(svc.rawAtExecute), "record.json must be valid JSON at Execute entry")

	// After the failure returns, the same path must still hold valid JSON with
	// phase dispatching — not deleted, not torn.
	path := runrecord.RecordPath(projectRoot, attemptID)
	assert.FileExists(t, path)
	rawAfter, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, json.Valid(rawAfter), "record.json must remain valid JSON after pre-dispatch failure")

	loaded, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, loaded, "dispatching record must still be readable after failure")
	assert.Equal(t, runrecord.PhaseDispatching, loaded.Phase)
	assert.Equal(t, attemptID, loaded.AttemptID)
	assert.Equal(t, beadID, loaded.BeadID)
	keyAfter, _, _, _ := recordIdentityFromJSON(t, rawAfter)
	assert.Equal(t, attemptID, keyAfter, "durable record key must remain the attempt id")

	// Substrate bytes at Execute entry remain the durable record after failure
	// (no rewrite/replace on the typed pre-dispatch path).
	assert.Equal(t, string(svc.rawAtExecute), string(rawAfter),
		"pre-dispatch failure must not rewrite the dispatching record")
}

// TestRunRecordPreDispatchFailureDoesNotProjectProviderSession proves the
// failure path does not replace the DDx dispatching record with
// provider-session-derived canonical state (AC2).
func TestRunRecordPreDispatchFailureDoesNotProjectProviderSession(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID = "20260726T052100-noproject"
		beadID    = "ddx-02270d66-noproject"
		workerID  = "worker-no-project"
		baseRev   = "def456deadbeef02"
		sessionID = "eb-cafebabe-provider-session"
		promptRel = ".ddx/executions/" + attemptID + "/prompt.md"
	)

	svc := &preDispatchFailureTripwireService{
		passthroughTestService: passthroughTestService{
			executeErr: &agentlib.ErrHarnessModelIncompatible{Harness: "codex", Model: "not-a-model"},
		},
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg,
		preDispatchFailureRuntime(attemptID, beadID, workerID, baseRev, sessionID, promptRel))
	require.Error(t, err)
	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr)

	loaded, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, runrecord.PhaseDispatching, loaded.Phase)
	assert.Nil(t, loaded.Fizeau, "pre-dispatch failure must not populate Fizeau public fields")
	assert.Nil(t, loaded.Outcome, "pre-dispatch failure must not invent a terminal outcome from provider state")
	assert.NotEqual(t, sessionID, loaded.AttemptID)

	// No sibling run directory keyed by the provider session id.
	_, err = os.Stat(runrecord.RecordPath(projectRoot, sessionID))
	assert.True(t, os.IsNotExist(err), "must not publish a record under provider session_id")

	// Exactly one run substrate directory: the DDx attempt id.
	entries, err := os.ReadDir(filepath.Join(projectRoot, runrecord.StoreDir))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, attemptID, entries[0].Name())

	raw, err := os.ReadFile(runrecord.RecordPath(projectRoot, attemptID))
	require.NoError(t, err)
	require.True(t, json.Valid(raw))
	key, gotAttempt, _, _ := recordIdentityFromJSON(t, raw)
	assert.Equal(t, attemptID, key)
	assert.Equal(t, attemptID, gotAttempt)
	assert.NotEqual(t, sessionID, key)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	// Forbidden provider-session / process canonical-state keys must be absent.
	for _, forbidden := range []string{
		"provider_session_state",
		"provider_session_canonical_state",
		"session_canonical_state",
		"canonical_state",
		"raw_output",
		"provider_output",
		"provider_pid",
		"process_tree",
		"pid",
		"harness",
		"provider",
		"model",
		"route_reason",
	} {
		_, ok := asMap[forbidden]
		assert.False(t, ok, "record must not contain provider-session/routing field %q", forbidden)
	}
	// Session id must not become the durable identity.
	assert.NotContains(t, string(raw), `"attempt_id":"`+sessionID+`"`)
}

// TestRunRecordPreDispatchFailureKeepsCorrelationFields verifies bead ID,
// attempt ID, worker ID, prompt evidence pointer, base revision, timestamps,
// and absent concrete harness-routing fields survive the failure path (AC3).
func TestRunRecordPreDispatchFailureKeepsCorrelationFields(t *testing.T) {
	projectRoot := t.TempDir()
	const (
		attemptID = "20260726T052200-corrfields"
		beadID    = "ddx-02270d66-corr"
		workerID  = "worker-corr-fields"
		baseRev   = "fedcba9876543210"
		sessionID = "eb-should-not-replace-corr"
		promptRel = ".ddx/executions/" + attemptID + "/prompt.md"
	)

	svc := &preDispatchFailureTripwireService{
		passthroughTestService: passthroughTestService{
			executeErr: &agentlib.ErrHarnessModelIncompatible{Harness: "claude-tui", Model: "incompatible-model"},
		},
		projectRoot: projectRoot,
		recordKey:   attemptID,
	}
	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})

	_, err := executeOnService(context.Background(), svc, projectRoot, rcfg,
		preDispatchFailureRuntime(attemptID, beadID, workerID, baseRev, sessionID, promptRel))
	require.Error(t, err)
	var pfErr *ProviderFailureError
	require.ErrorAs(t, err, &pfErr)
	require.NotNil(t, svc.recordAtExecute, "record must exist at Execute entry for correlation snapshot")

	before := svc.recordAtExecute
	after, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, after)

	// Identity / correlation fields survive unchanged.
	assert.Equal(t, before.BeadID, after.BeadID)
	assert.Equal(t, beadID, after.BeadID)
	assert.Equal(t, before.AttemptID, after.AttemptID)
	assert.Equal(t, attemptID, after.AttemptID)
	assert.Equal(t, runrecord.PhaseDispatching, after.Phase)
	assert.Equal(t, before.Version, after.Version)
	assert.False(t, after.StartedAt.IsZero(), "started_at must survive")
	assert.False(t, after.UpdatedAt.IsZero(), "updated_at must survive")
	assert.True(t, before.StartedAt.Equal(after.StartedAt), "started_at must not be rewritten on failure")
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at must not be rewritten on failure")

	rawAfter, err := os.ReadFile(runrecord.RecordPath(projectRoot, attemptID))
	require.NoError(t, err)
	keyAfter, gotAttempt, gotBead, _ := recordIdentityFromJSON(t, rawAfter)
	assert.Equal(t, attemptID, keyAfter)
	assert.Equal(t, attemptID, gotAttempt)
	assert.Equal(t, beadID, gotBead)

	// Worker ID, base revision, and prompt evidence pointer (DDx correlation).
	assert.Equal(t, workerID, evidencePathByName(after.Evidence, "worker_id"),
		"worker_id correlation must survive on the dispatching record")
	assert.Equal(t, baseRev, evidencePathByName(after.Evidence, "base_rev"),
		"base_rev correlation must survive on the dispatching record")
	assert.Equal(t, promptRel, evidencePathByName(after.Evidence, "prompt"),
		"prompt evidence pointer must survive")
	assert.Equal(t, evidencePathByName(before.Evidence, "worker_id"), evidencePathByName(after.Evidence, "worker_id"))
	assert.Equal(t, evidencePathByName(before.Evidence, "base_rev"), evidencePathByName(after.Evidence, "base_rev"))
	assert.Equal(t, evidencePathByName(before.Evidence, "prompt"), evidencePathByName(after.Evidence, "prompt"))

	// Concrete harness-routing fields stay absent.
	assert.Nil(t, after.Fizeau)
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(rawAfter, &asMap))
	for _, forbidden := range []string{
		"harness", "provider", "model", "route_reason",
		"routing_policy", "min_power", "max_power", "profile",
	} {
		_, ok := asMap[forbidden]
		assert.False(t, ok, "correlation-preserving failure path must not add routing field %q", forbidden)
	}
}
