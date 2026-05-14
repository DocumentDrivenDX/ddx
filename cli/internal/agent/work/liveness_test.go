package work_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/work"
	"github.com/DocumentDrivenDX/ddx/internal/workerstatus"
	"github.com/stretchr/testify/require"
)

// TestWorkerLiveness_SidecarRecordsCurrentBeadAttemptAndLastActivity verifies
// AC #4: the SidecarLivenessReporter writes every operator-required field
// (current bead, attempt id, phase, last_activity_at, route/policy summary,
// PID) to .ddx/workers/<worker-id>/status.json on each tick.
func TestWorkerLiveness_SidecarRecordsCurrentBeadAttemptAndLastActivity(t *testing.T) {
	projectRoot := t.TempDir()

	sink := &bytes.Buffer{}
	rep := work.NewSidecarLivenessReporter(projectRoot, "wkr-test", "sess-test", sink)
	rep.SetAttempt(
		"ddx-abc12345",
		"att-2026-001",
		"running",
		"balanced",
		"claude",
		"opus",
		"balanced",
		4242,
	)

	before := time.Now().UTC().Add(-time.Second)
	rep.OnTick(time.Now())

	rec, err := workerstatus.ReadLiveness(projectRoot, "wkr-test")
	require.NoError(t, err)

	require.Equal(t, "wkr-test", rec.WorkerID)
	require.Equal(t, "ddx-abc12345", rec.CurrentBead)
	require.Equal(t, "att-2026-001", rec.AttemptID)
	require.Equal(t, "running", rec.Phase)
	require.Equal(t, "balanced", rec.Route)
	require.Equal(t, "claude", rec.Harness)
	require.Equal(t, "opus", rec.Model)
	require.Equal(t, 4242, rec.ChildPID)
	require.Equal(t, os.Getpid(), rec.PID, "worker PID must be recorded")
	require.True(t, rec.LastActivityAt.After(before), "last_activity_at must be advanced past tick start")

	// The mirrored worker.heartbeat envelope must include the same fields so
	// a connected server probe sees them on each tick.
	require.NotZero(t, sink.Len(), "tick must emit at least one envelope to the sink")
	lines := splitJSONLines(sink.Bytes())
	require.NotEmpty(t, lines)
	var env struct {
		Type string `json:"type"`
		Data struct {
			WorkerID  string `json:"worker_id"`
			BeadID    string `json:"bead_id"`
			AttemptID string `json:"attempt_id"`
			Phase     string `json:"phase"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &env))
	require.Equal(t, "worker.heartbeat", env.Type)
	require.Equal(t, "wkr-test", env.Data.WorkerID)
	require.Equal(t, "ddx-abc12345", env.Data.BeadID)
	require.Equal(t, "att-2026-001", env.Data.AttemptID)
	require.Equal(t, "running", env.Data.Phase)
}

// TestWorkerLiveness_ServerUnavailableDoesNotBlockWork verifies AC #3:
// with a nil sink (no server connection) the reporter still writes the
// sidecar, returns no error, and does not block the calling goroutine.
// The same must hold when sink.Write itself surfaces an error: liveness
// must remain best-effort and never propagate failure into the loop.
func TestWorkerLiveness_ServerUnavailableDoesNotBlockWork(t *testing.T) {
	projectRoot := t.TempDir()

	// Case 1: nil sink (no server probe attached).
	repNoSink := work.NewSidecarLivenessReporter(projectRoot, "wkr-no-sink", "sess", nil)
	repNoSink.SetAttempt("ddx-1", "att-1", "running", "", "", "", "", 0)
	repNoSink.OnTick(time.Now())
	rec, err := workerstatus.ReadLiveness(projectRoot, "wkr-no-sink")
	require.NoError(t, err, "sidecar must be written when no server sink is attached")
	require.Equal(t, "ddx-1", rec.CurrentBead)

	// Case 2: failing sink (simulates a closed pipe / refused connection
	// surfaced via TeeJSONL).
	repFail := work.NewSidecarLivenessReporter(projectRoot, "wkr-fail-sink", "sess", failingWriter{})
	repFail.SetAttempt("ddx-2", "att-2", "running", "", "", "", "", 0)
	done := make(chan struct{})
	go func() {
		repFail.OnTick(time.Now())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnTick blocked on a failing sink — liveness must be fail-open")
	}
	rec2, err := workerstatus.ReadLiveness(projectRoot, "wkr-fail-sink")
	require.NoError(t, err)
	require.Equal(t, "ddx-2", rec2.CurrentBead)
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errClosedPipe
}

var errClosedPipe = &writeErr{msg: "connection refused"}

type writeErr struct{ msg string }

func (e *writeErr) Error() string { return e.msg }

func splitJSONLines(b []byte) [][]byte {
	var out [][]byte
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		out = append(out, line)
	}
	return out
}
