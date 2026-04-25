package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunWithConfigDelegation verifies that ExecuteBeadWorker.RunWithConfig
// resolves a *Config built via config.NewTestConfigForLoop and threads every
// loop-relevant durable knob through to the running loop's observable
// behavior. SD-024 Stage 1 / Bead 6.
func TestRunWithConfigDelegation(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	opts := config.TestLoopConfigOpts{
		Assignee:                "rwc-worker",
		ReviewMaxRetries:        7,
		NoProgressCooldown:      42 * time.Minute,
		MaxNoChangesBeforeClose: 5,
		HeartbeatInterval:       2 * time.Second,
		Harness:                 "rwc-harness",
		Model:                   "rwc-model",
	}
	cfg := config.NewTestConfigForLoop(opts)
	rcfg := cfg.Resolve(config.TestLoopOverrides(opts))

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusSuccess,
				Detail:    "merged cleanly",
				SessionID: "sess-rwc",
				ResultRev: "rwcdead",
				Harness:   "rwc-harness",
				Model:     "rwc-model",
			}, nil
		}),
	}

	var sink bytes.Buffer
	runtime := ExecuteBeadLoopRuntime{
		Once:      true,
		EventSink: &sink,
		SessionID: "sess-rt",
		WorkerID:  "wkr-rt",
	}

	result, err := worker.RunWithConfig(context.Background(), rcfg, runtime)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Attempts)
	assert.Equal(t, 1, result.Successes)

	// Decode loop.start event from the EventSink. The loop emits structured
	// JSONL there; the harness, model, and assignee fields prove the durable
	// rcfg values flowed through RunWithConfig into the running loop.
	var startData map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(sink.Bytes()), []byte("\n")) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry))
		if entry["type"] == "loop.start" {
			startData, _ = entry["data"].(map[string]any)
			break
		}
	}
	require.NotNil(t, startData, "loop.start event missing from EventSink")
	assert.Equal(t, "rwc-harness", startData["harness"])
	assert.Equal(t, "rwc-model", startData["model"])
	assert.Equal(t, "rwc-worker", startData["assignee"])
	assert.Equal(t, "wkr-rt", startData["worker_id"])
	assert.Equal(t, "sess-rt", startData["session_id"])
	assert.Equal(t, true, startData["once"])

	// Bead-level evidence: claim used the configured assignee, and the
	// late execute-bead event records that assignee as Actor.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	var sawExecuteBead bool
	for _, ev := range events {
		if ev.Kind == "execute-bead" {
			assert.Equal(t, "rwc-worker", ev.Actor, "execute-bead event Actor must be rcfg assignee")
			sawExecuteBead = true
		}
		if ev.Kind == "routing" {
			assert.True(t, strings.Contains(ev.Body, "rwc-harness"),
				"routing event body must reference rcfg harness; body=%s", ev.Body)
		}
	}
	assert.True(t, sawExecuteBead, "expected an execute-bead event recording the configured assignee")
}

// TestRunWithConfigDelegation_ZeroValueRcfgPanics confirms that
// RunWithConfig refuses to operate on an unsealed ResolvedConfig — the
// SD-024 sealed-construction invariant flows through to the loop.
func TestRunWithConfigDelegation_ZeroValueRcfgPanics(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{}, nil
		}),
	}

	defer func() {
		r := recover()
		require.NotNil(t, r, "RunWithConfig with zero-value ResolvedConfig must panic via requireSealed")
		msg, _ := r.(string)
		assert.Contains(t, msg, "ResolvedConfig used without going through")
	}()

	var rcfg config.ResolvedConfig
	_, _ = worker.RunWithConfig(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
}
