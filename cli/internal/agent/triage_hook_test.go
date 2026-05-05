package agent

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type triageHookRunnerStub struct {
	lastOpts RunArgs
	calls    int32
	run      func(opts RunArgs) (*Result, error)
}

func (r *triageHookRunnerStub) Run(opts RunArgs) (*Result, error) {
	r.lastOpts = opts
	atomic.AddInt32(&r.calls, 1)
	if r.run != nil {
		return r.run(opts)
	}
	return &Result{ExitCode: 0, Output: `{"classification":"transport","recommended_action":"retry","rationale":"transient","suggested_amendments":"none","suggested_followup_beads":[]}`}, nil
}

func newTriageHookTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".ddx"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ResolveLogDir(root, "")), 0o755))
	return root
}

func newTriageHookTestStore(t *testing.T, root string) (*bead.Store, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ".ddx"))
	require.NoError(t, store.Init())
	b := &bead.Bead{
		ID:          "ddx-triage-1",
		Title:       "agent: invoke runner-backed post-attempt triage hook",
		IssueType:   bead.DefaultType,
		Status:      bead.StatusOpen,
		Priority:    2,
		Labels:      []string{"phase:2", "area:agent", "kind:feature"},
		Description: "PROBLEM\npost-attempt triage is missing\n\nROOT CAUSE\nexecute_bead_loop.go does not invoke a triage hook\n\nPROPOSED FIX\nadd a runner-backed hook\n\nNON-SCOPE\nreview triage\n",
		Acceptance:  "1. TestTriageHook_UsesRunnerLibrary\n2. cd cli && go test ./internal/agent/... green\n3. lefthook run pre-commit passes",
		Notes:       "triage hook fixture",
	}
	require.NoError(t, store.Create(b))
	return store, b
}

func triageHookTestConfig() config.ResolvedConfig {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{Harness: "codex"})
	return cfg.Resolve(config.CLIOverrides{})
}

func TestTriageHook_UsesRunnerLibrary(t *testing.T) {
	root := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, root)
	require.NoError(t, store.AppendEvent(b.ID, bead.BeadEvent{
		Kind:      "bead-quality.lint",
		Summary:   "score=9",
		Body:      "first event",
		Source:    "test",
		CreatedAt: time.Now().UTC(),
	}))

	runner := &triageHookRunnerStub{}
	hook := NewPostAttemptTriageHook(root, store, triageHookTestConfig(), nil, runner, nil)

	got, err := hook(context.Background(), b.ID, ExecuteBeadReport{
		BeadID:    b.ID,
		Status:    ExecuteBeadStatusExecutionFailed,
		Detail:    "transport error",
		BaseRev:   "feedface",
		ResultRev: "feedface",
		SessionID: "sess-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "transport", got.Classification)
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls))

	assert.Equal(t, postAttemptTriagePromptSource, runner.lastOpts.PromptSource)
	assert.Equal(t, root, runner.lastOpts.WorkDir)
	assert.True(t, strings.HasPrefix(runner.lastOpts.Prompt, "MODE: triage"))
	assert.Contains(t, runner.lastOpts.Prompt, postAttemptTriageSkillName)
	assert.Contains(t, runner.lastOpts.Prompt, `"execute_bead_report":`)
	assert.Contains(t, runner.lastOpts.Prompt, `"classification"`)
	assert.Contains(t, runner.lastOpts.Prompt, `"recommended_action"`)
	assert.Contains(t, runner.lastOpts.Prompt, `"suggested_followup_beads"`)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	triageHookPath := filepath.Join(filepath.Dir(thisFile), "triage_hook.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, triageHookPath, nil, parser.ImportsOnly)
	require.NoError(t, err)
	for _, imp := range parsed.Imports {
		assert.NotEqual(t, `"os/exec"`, imp.Path.Value, "triage_hook.go must not import os/exec")
	}
	body, err := os.ReadFile(triageHookPath)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "ddx agent run", "triage_hook.go must not shell out to ddx agent run")
}

func TestTriageHook_PromptIncludesOutcomeAndLogExcerpt(t *testing.T) {
	root := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, root)
	for i := 0; i < 10; i++ {
		require.NoError(t, store.AppendEvent(b.ID, bead.BeadEvent{
			Kind:      "note",
			Summary:   "event-" + string(rune('0'+i)),
			Body:      strings.Repeat("body-", 200),
			Source:    "test",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
		}))
	}

	logDir := ResolveLogDir(root, "")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	logPath := filepath.Join(logDir, "agent-sess-123.jsonl")
	longLine := strings.Repeat(`{"type":"progress","data":{"message":"tail"}}`, 120)
	require.NoError(t, os.WriteFile(logPath, []byte(longLine+"\n{\"type\":\"final\",\"data\":{\"ok\":true}}\n"), 0o644))

	runner := &triageHookRunnerStub{}
	hook := NewPostAttemptTriageHook(root, store, triageHookTestConfig(), nil, runner, nil)

	_, err := hook(context.Background(), b.ID, ExecuteBeadReport{
		BeadID:    b.ID,
		Status:    ExecuteBeadStatusExecutionFailed,
		Detail:    "context canceled",
		BaseRev:   "deadbeef",
		ResultRev: "deadbeef",
		SessionID: "sess-123",
	})
	require.NoError(t, err)

	prompt := runner.lastOpts.Prompt
	assert.True(t, strings.HasPrefix(prompt, "MODE: triage"))
	assert.Contains(t, prompt, postAttemptTriageSkillName)
	assert.Contains(t, prompt, `"status": "execution_failed"`)
	assert.Contains(t, prompt, `"detail": "context canceled"`)
	assert.Contains(t, prompt, `"session_id": "sess-123"`)
	assert.Contains(t, prompt, `"recent_bead_events"`)
	assert.Contains(t, prompt, "event-9")
	assert.NotContains(t, prompt, "event-0")
	assert.Contains(t, prompt, "tail clipped to")
	assert.Contains(t, prompt, `"session_log_excerpt"`)
}

func TestLoop_TriageHook_FiresPostOutcome(t *testing.T) {
	realStore, candidate, _ := newExecuteLoopTestStore(t)
	triageCalled := false
	store := &errorInjectingStore{ExecuteBeadLoopStore: realStore}
	store.onSetCooldown = func(id string, until time.Time, status, detail string) error {
		require.True(t, triageCalled, "triage hook must fire before SetExecutionCooldown")
		return realStore.SetExecutionCooldown(id, until, status, detail)
	}

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				Detail:    "nothing changed",
				BaseRev:   "abc123",
				ResultRev: "abc123",
				SessionID: "sess-order",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			triageCalled = true
			assert.Equal(t, candidate.ID, beadID)
			assert.Equal(t, ExecuteBeadStatusNoChanges, report.Status)
			assert.Equal(t, "nothing changed", report.Detail)
			assert.Equal(t, "abc123", report.BaseRev)
			assert.Equal(t, "abc123", report.ResultRev)
			return TriageResult{}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.True(t, triageCalled)
	assert.Equal(t, ExecuteBeadStatusNoChanges, result.Results[0].Status)
	assert.Empty(t, result.Results[0].OutcomeReason)
	assert.NotEmpty(t, result.Results[0].RetryAfter)
}

func TestTriageHook_RecordsButDoesNotMutateOutcome(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				Detail:    "zero diff",
				BaseRev:   "fff111",
				ResultRev: "fff111",
				SessionID: "sess-record",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{
				Classification:      "transport",
				RecommendedAction:   "retry",
				Rationale:           "transient outage",
				SuggestedAmendments: "none",
			}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusNoChanges, report.Status)
	assert.Equal(t, "transport", report.OutcomeReason)
	assert.Empty(t, report.RetryAfter, "transient classification should bypass the no-progress cooldown")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "bead-quality.triage" {
			found = true
			assert.Contains(t, ev.Body, `"classification":"transport"`)
			assert.Contains(t, ev.Body, `"recommended_action":"retry"`)
			break
		}
	}
	assert.True(t, found, "triage event must be recorded when classification is valid")
}

func TestTriageHook_HookError_FallsThroughToLegacyCooldown(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				Detail:    "no change",
				BaseRev:   "bada55",
				ResultRev: "bada55",
				SessionID: "sess-error",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{}, assert.AnError
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusNoChanges, report.Status)
	assert.Empty(t, report.OutcomeReason)
	assert.NotEmpty(t, report.RetryAfter, "hook failure must preserve legacy cooldown behavior")

	got, err := store.Get(candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	_, ok := got.Extra["execute-loop-retry-after"]
	assert.True(t, ok)
}
