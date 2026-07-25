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

	// Still present after the call returns.
	loaded, err := runrecord.Read(projectRoot, attemptID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, runrecord.PhaseDispatching, loaded.Phase)
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
