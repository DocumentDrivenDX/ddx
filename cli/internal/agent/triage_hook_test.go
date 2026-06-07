package agent

import (
	"bytes"
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
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
	agentlib "github.com/easel/fizeau"
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
	return &Result{ExitCode: 0, Output: `{"classification":"transport","recommended_action":"release_claim_retry","rationale":"transient","suggested_amendments":[],"suggested_followup_beads":[]}`}, nil
}

func newTriageHookTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ResolveLogDir(root, "")), 0o755))
	skillDir := filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("triage"), 0o644))
	return root
}

func newTriageHookTestStore(t *testing.T, root string) (*bead.Store, *bead.Bead) {
	t.Helper()
	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
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
	require.NoError(t, store.Create(context.Background(), b))
	return store, b
}

func triageHookTestConfig() config.ResolvedConfig {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{})
	return cfg.Resolve(config.CLIOverrides{Harness: "codex"})
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
	assert.Contains(t, runner.lastOpts.Prompt, `"suggested_amendments":[]`)
	assert.Contains(t, runner.lastOpts.Prompt, `"recommended_action":"release_claim_retry"`)
	assert.NotContains(t, runner.lastOpts.Prompt, `"suggested_amendments":"none"`)
	assert.NotContains(t, runner.lastOpts.Prompt, `"recommended_action":"retry"`)

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
	assert.NotContains(t, string(body), "legacy agent run", "triage_hook.go must not shell out to legacy agent run")
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

func TestTriageHook_DecodeToleratesLegacyNoneArraySentinels(t *testing.T) {
	got := decodeTriageResult(`{"classification":"transport","recommended_action":"release_claim_retry","rationale":"transient","suggested_amendments":"none","suggested_followup_beads":"none"}`)

	require.Empty(t, got.DecodeWarnings)
	assert.False(t, got.Malformed)
	assert.Equal(t, "transport", got.Classification)
	assert.Empty(t, got.SuggestedAmendments)
	assert.Empty(t, got.SuggestedFollowupBeads)
}

func TestTriageHook_DecodeDropsNonEmptyStringArrayFields(t *testing.T) {
	got := decodeTriageResult(`{"classification":"transport","recommended_action":"release_claim_retry","rationale":"transient","suggested_amendments":"tighten AC","suggested_followup_beads":"create a follow-up bead for websocket coverage"}`)

	require.Len(t, got.DecodeWarnings, 2)
	assert.True(t, got.Malformed)
	assert.Equal(t, "transport", got.Classification)
	assert.Empty(t, got.SuggestedAmendments)
	assert.Empty(t, got.SuggestedFollowupBeads)
	assert.Equal(t, "suggested_amendments", got.DecodeWarnings[0].Field)
	assert.Equal(t, "suggested_followup_beads", got.DecodeWarnings[1].Field)
	assert.Contains(t, got.DecodeWarnings[1].RawExcerpt, "websocket coverage")
}

func TestTriageHook_NoJSONOutputReturnsWarningResult(t *testing.T) {
	root := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, root)
	runner := &triageHookRunnerStub{
		run: func(opts RunArgs) (*Result, error) {
			return &Result{ExitCode: 0, Output: "I could not classify this attempt."}, nil
		},
	}
	hook := NewPostAttemptTriageHook(root, store, triageHookTestConfig(), nil, runner, nil)

	got, err := hook(context.Background(), b.ID, ExecuteBeadReport{
		BeadID:    b.ID,
		Status:    ExecuteBeadStatusNoChanges,
		BaseRev:   "abc",
		ResultRev: "abc",
	})

	require.NoError(t, err)
	assert.Empty(t, got.Classification)
	require.NotEmpty(t, got.DecodeWarnings)
	assert.True(t, got.Malformed)
	assert.Equal(t, "output", got.DecodeWarnings[0].Field)
	assert.Contains(t, got.DecodeWarnings[0].Warning, "no JSON object")
}

func TestLoop_TriageHook_FiresPostOutcome(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)
	triageCalled := false

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "execution error",
				BaseRev:   "abc123",
				ResultRev: "def456",
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
			assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
			assert.Equal(t, "execution error", report.Detail)
			assert.Equal(t, "abc123", report.BaseRev)
			assert.Equal(t, "def456", report.ResultRev)
			return TriageResult{}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.True(t, triageCalled)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
	assert.Empty(t, result.Results[0].OutcomeReason)
	assert.Empty(t, result.Results[0].RetryAfter)
}

func TestTriageHook_RecordsButDoesNotMutateOutcome(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "fff111",
				ResultRev: "aaa222",
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
				Classification:    "transport",
				RecommendedAction: "release_claim_retry",
				Rationale:         "transient outage",
			}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Equal(t, "transport", report.OutcomeReason)
	assert.Empty(t, report.RetryAfter, "transient classification should bypass the no-progress cooldown")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "bead-quality.triage" {
			found = true
			assert.Contains(t, ev.Body, `"classification":"transport"`)
			assert.Contains(t, ev.Body, `"recommended_action":"release_claim_retry"`)
			break
		}
	}
	assert.True(t, found, "triage event must be recorded when classification is valid")
}

func TestTriageHook_WarningOnlyRecordsEvidenceWithoutOutcomeReason(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)
	var log bytes.Buffer

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "abc123",
				ResultRev: "def456",
				SessionID: "sess-warning",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &log,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{
				DecodeWarnings: []TriageDecodeWarning{
					newTriageDecodeWarning("output", "no JSON object found", "plain text response"),
				},
				Malformed: true,
			}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
	assert.Empty(t, result.Results[0].OutcomeReason)
	assert.Empty(t, result.Results[0].RetryAfter)
	assert.Contains(t, log.String(), "post-attempt triage warning")
	assert.NotContains(t, log.String(), "post-attempt triage error")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "bead-quality.triage-warning" {
			found = true
			assert.Contains(t, ev.Body, `"decode_warnings"`)
			assert.Contains(t, ev.Body, `"malformed":true`)
			assert.Contains(t, ev.Body, `"suggested_amendments":[]`)
			assert.Contains(t, ev.Body, `"suggested_followup_beads":[]`)
			break
		}
	}
	assert.True(t, found, "warning-only triage must be durable evidence")
}

func TestTriageHook_RecognizedClassificationWithWarningSetsOutcomeAndRecordsOneTriageEvent(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "abc123",
				ResultRev: "def456",
				SessionID: "sess-warning-classified",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{
				Classification:    "transport",
				RecommendedAction: "release_claim_retry",
				Rationale:         "transient",
				DecodeWarnings: []TriageDecodeWarning{
					newTriageDecodeWarning("suggested_followup_beads", "expected array; got string; dropped field", "write a follow-up"),
				},
				Malformed: true,
			}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, "transport", result.Results[0].OutcomeReason)

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	triageEvents := 0
	warningEvents := 0
	for _, ev := range events {
		switch ev.Kind {
		case "bead-quality.triage":
			triageEvents++
			assert.Contains(t, ev.Body, `"decode_warnings"`)
			assert.Contains(t, ev.Summary, "warnings=1")
		case "bead-quality.triage-warning":
			warningEvents++
		}
	}
	assert.Equal(t, 1, triageEvents)
	assert.Equal(t, 0, warningEvents)
}

func TestTriageHook_UnknownClassificationRecordsWarningWithoutOutcomeReason(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "abc123",
				ResultRev: "def456",
				SessionID: "sess-unknown-class",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{
				Classification:    "success",
				RecommendedAction: "close_already_satisfied",
				Rationale:         "legacy vocabulary",
			}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].OutcomeReason)

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "bead-quality.triage-warning" {
			found = true
			assert.Contains(t, ev.Body, `"classification":"success"`)
			assert.Contains(t, ev.Body, `"unknown classification"`)
			break
		}
	}
	assert.True(t, found, "unknown classification must be reviewable without steering scheduling")
}

func TestPostAttemptTriageHook_EmptyOutputRecordsWarningEvent(t *testing.T) {
	root := newTriageHookTestRoot(t)

	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))
	candidate := &bead.Bead{ID: "ddx-triage-empty", Title: "empty output triage", Priority: 0}
	require.NoError(t, store.Create(context.Background(), candidate))

	runner := &triageHookRunnerStub{
		run: func(_ RunArgs) (*Result, error) {
			return &Result{ExitCode: 0, Output: "", CondensedOutput: ""}, nil
		},
	}
	triageHook := NewPostAttemptTriageHook(root, store, triageHookTestConfig(), nil, runner, nil)

	var log bytes.Buffer
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "abc123",
				ResultRev: "def456",
				SessionID: "sess-empty",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:                  true,
		Log:                   &log,
		PostAttemptTriageHook: triageHook,
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.Results[0].Status)
	assert.Empty(t, result.Results[0].OutcomeReason, "empty triage output must not steer scheduling")
	assert.Empty(t, result.Results[0].RetryAfter)
	assert.Equal(t, int32(1), atomic.LoadInt32(&runner.calls))

	assert.Contains(t, log.String(), "post-attempt triage warning")
	assert.NotContains(t, log.String(), "post-attempt triage error")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	found := false
	for _, ev := range events {
		if ev.Kind == "bead-quality.triage-warning" {
			found = true
			assert.Contains(t, ev.Body, "empty output")
			assert.Contains(t, ev.Body, `"malformed":true`)
			break
		}
	}
	assert.True(t, found, "empty triage output must be durable evidence as a triage-warning event")
}

func TestPostAttemptTriageHookErrorDoesNotOverwriteReport(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)
	var log bytes.Buffer

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "zero diff",
				BaseRev:   "abc123",
				ResultRev: "def456",
				SessionID: "sess-err",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		Log:  &log,
		PostAttemptTriageHook: func(_ context.Context, _ string, _ ExecuteBeadReport) (TriageResult, error) {
			return TriageResult{}, assert.AnError
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	report := result.Results[0]
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, report.Status)
	assert.Empty(t, report.OutcomeReason, "dispatch errors must not steer OutcomeReason")

	assert.Contains(t, log.String(), "post-attempt triage error")
	assert.NotContains(t, log.String(), "post-attempt triage warning")

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	for _, ev := range events {
		assert.NotEqual(t, "bead-quality.triage-warning", ev.Kind, "dispatch errors must not produce a triage-warning event")
		assert.NotEqual(t, "bead-quality.triage", ev.Kind, "dispatch errors must not produce a triage event")
	}
}

func TestTriageHook_HookError_DoesNotCreateDefaultCooldown(t *testing.T) {
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
	assert.Empty(t, report.RetryAfter, "hook failure must not create default no_changes cooldown")

	got, err := store.Get(context.Background(), candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Extra)
	_, ok := got.Extra["work-retry-after"]
	assert.False(t, ok)
}

// TestNoChanges_SkipsReviewer verifies that when the implementer returns
// no_changes (BaseRev == ResultRev, zero commits), the PostAttemptTriageHook
// (reviewer) is NOT invoked. Per ADR-024 P1: paying a reviewer to evaluate an
// empty diff is unjustified cost.
func TestNoChanges_SkipsReviewer(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)
	reviewerCalled := false

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				BaseRev:   "abc123",
				ResultRev: "abc123",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(_ context.Context, _ string, _ ExecuteBeadReport) (TriageResult, error) {
			reviewerCalled = true
			return TriageResult{}, nil
		},
	})
	require.NoError(t, err)
	assert.False(t, reviewerCalled, "reviewer must not be invoked when implementer produced no commits")
}

// TestSomeChanges_StillInvokesReviewer verifies that when the implementer
// produces commits (ResultRev != BaseRev), the PostAttemptTriageHook is still
// invoked for non-success outcomes.
func TestSomeChanges_StillInvokesReviewer(t *testing.T) {
	store, _, _ := newExecuteLoopTestStore(t)
	reviewerCalled := false

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "test failure",
				BaseRev:   "abc123",
				ResultRev: "def456",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(_ context.Context, _ string, _ ExecuteBeadReport) (TriageResult, error) {
			reviewerCalled = true
			return TriageResult{}, nil
		},
	})
	require.NoError(t, err)
	assert.True(t, reviewerCalled, "reviewer must be invoked when implementer produced commits")
}

// TestNoChangesWithReviewSpecGapClassification_RoutesToProposed verifies that
// when the implementer signals no_changes with status: proposed (spec gap),
// the bead routes to proposed status without reviewer gating.
func TestNoChangesWithReviewSpecGapClassification_RoutesToProposed(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{ID: "ddx-specgap1", Title: "Spec gap bead"}
	require.NoError(t, store.Create(context.Background(), b))

	reviewerCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:             id,
				Status:             ExecuteBeadStatusNoChanges,
				NoChangesRationale: "status: proposed\nreason: AC conflicts with governing spec — spec update required",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		PostAttemptTriageHook: func(_ context.Context, _ string, _ ExecuteBeadReport) (TriageResult, error) {
			reviewerCalled = true
			return TriageResult{}, nil
		},
	})
	require.NoError(t, err)

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusProposed, got.Status, "spec-gap no_changes must route bead to proposed without reviewer")
	assert.False(t, reviewerCalled, "reviewer must not gate spec-gap routing")
}

// TestReviewerSkippedEmptyDiffEvent verifies that a structured
// "reviewer-skipped-empty-diff" event is emitted each time reviewer dispatch
// is skipped due to an empty diff.
func TestReviewerSkippedEmptyDiffEvent(t *testing.T) {
	store, candidate, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, id string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    id,
				Status:    ExecuteBeadStatusNoChanges,
				BaseRev:   "abc123",
				ResultRev: "abc123",
			}, nil
		}),
	}
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	events, err := store.Events(candidate.ID)
	require.NoError(t, err)
	var found bool
	for _, ev := range events {
		if ev.Kind == ReviewerSkippedEmptyDiffEventKind {
			found = true
			assert.Equal(t, "reviewer skipped: empty diff (no commits produced)", ev.Summary)
			break
		}
	}
	assert.True(t, found, "reviewer-skipped-empty-diff event must be emitted for no_changes outcomes")
}

func TestPostAttemptTriageHook_LeavesPolicyToFizeau(t *testing.T) {
	root := newTriageHookTestRoot(t)
	store, b := newTriageHookTestStore(t, root)

	svc := &passthroughTestService{
		listPolicies: []agentlib.PolicyInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8},
			{Name: "cheap", MinPower: 5, MaxPower: 5},
		},
		listModels: []agentlib.ModelInfo{
			{ID: "cheap-model", Power: 5, Available: true, AutoRoutable: true},
		},
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"classification\":\"transport\",\"recommended_action\":\"release_claim_retry\",\"rationale\":\"transient\",\"suggested_amendments\":[],\"suggested_followup_beads\":[]}"}`),
			},
		},
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{Profile: "default"})
	require.Equal(t, "default", rcfg.Profile())

	hook := NewPostAttemptTriageHook(root, store, rcfg, svc, nil, nil)
	got, err := hook(context.Background(), b.ID, ExecuteBeadReport{
		BeadID:    b.ID,
		Status:    ExecuteBeadStatusNoChanges,
		Detail:    "nothing changed",
		BaseRev:   "abc123",
		ResultRev: "abc123",
	})
	require.NoError(t, err)
	assert.Equal(t, "transport", got.Classification)
	assert.Empty(t, svc.lastReq.Policy, "triage dispatch must let Fizeau choose policy")
	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Empty(t, svc.lastReq.Model)
	assert.Zero(t, svc.lastReq.MinPower, "triage dispatch must not inherit implementation min_power pins")
	assert.Zero(t, svc.lastReq.MaxPower, "triage dispatch must not inherit implementation max_power pins")
}
