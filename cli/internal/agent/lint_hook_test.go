package agent

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lintHookRunnerStub struct {
	lastOpts RunArgs
	run      func(opts RunArgs) (*Result, error)
}

func (r *lintHookRunnerStub) Run(opts RunArgs) (*Result, error) {
	r.lastOpts = opts
	if r.run != nil {
		return r.run(opts)
	}
	return &Result{ExitCode: 0}, nil
}

func newLintHookTestRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	skillPath := filepath.Join(root, ".agents", "skills", "ddx", "bead-lifecycle")
	require.NoError(t, os.MkdirAll(skillPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("lint"), 0o644))
	return root
}

func newLintHookTestStore(t *testing.T, root string) (*bead.Store, *bead.Bead) {
	t.Helper()

	store := bead.NewStore(filepath.Join(root, ddxroot.DirName))
	require.NoError(t, store.Init(context.Background()))

	dep := &bead.Bead{
		ID:        "ddx-lint-dep",
		Title:     "dependency bead for lint hook fixture",
		IssueType: bead.DefaultType,
		Status:    bead.StatusOpen,
		Priority:  2,
	}
	require.NoError(t, store.Create(context.Background(), dep))

	b := &bead.Bead{
		ID:        "ddx-lint-001",
		Title:     "agent: implement runner-backed bead lifecycle lint hook",
		IssueType: bead.DefaultType,
		Status:    bead.StatusOpen,
		Priority:  2,
		Labels:    []string{"phase:2", "area:agent", "kind:feature"},
		Parent:    "ddx-parent",
		Dependencies: []bead.Dependency{
			{IssueID: "ddx-lint-001", DependsOnID: "ddx-lint-dep", Type: "blocks"},
		},
		Description: "PROBLEM\nmissing lint hook\n\nROOT CAUSE\nno hook\n\nPROPOSED FIX\nadd hook\n\nNON-SCOPE\ntriage\n",
		Acceptance:  "1. TestLintHook_UsesRunnerLibrary\n2. cd cli && go test ./internal/agent/... green\n3. lefthook run pre-commit passes",
		Notes:       "lint hook test fixture",
		Extra: map[string]any{
			"spec-id":            "FEAT-999",
			"execution-eligible": true,
		},
	}
	require.NoError(t, store.Create(context.Background(), b))
	return store, b
}

func lintHookTestConfig() config.ResolvedConfig {
	cfg := config.NewTestConfigForRun(config.TestRunConfigOpts{})
	return cfg.Resolve(config.CLIOverrides{})
}

func TestLintHook_UsesRunnerLibrary(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{
			ExitCode: 0,
			Output:   `{"score":7,"rationale":"ok","suggested_fixes":["none"],"waivers_applied":[]}`,
		}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, got.Score)

	assert.Equal(t, "bead-lifecycle-lint", runner.lastOpts.PromptSource)
	assert.Equal(t, root, runner.lastOpts.WorkDir)
	assert.Contains(t, runner.lastOpts.Prompt, "MODE: lint")
	assert.Contains(t, runner.lastOpts.Prompt, "bead-lifecycle")
	assert.Contains(t, runner.lastOpts.Prompt, `"spec-id": "FEAT-999"`)

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	lintHookPath := filepath.Join(filepath.Dir(thisFile), "lint_hook.go")
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, lintHookPath, nil, parser.ImportsOnly)
	require.NoError(t, err)
	for _, imp := range parsed.Imports {
		assert.NotEqual(t, `"os/exec"`, imp.Path.Value, "lint_hook.go must not import os/exec")
	}
	body, err := os.ReadFile(lintHookPath)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "legacy agent run", "lint_hook.go must not shell out to legacy agent run")
}

func TestLintHook_PromptIncludesStandaloneBead(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{
			ExitCode: 0,
			Output:   `{"score":9,"rationale":"ready","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.NoError(t, err)

	prompt := runner.lastOpts.Prompt
	assert.True(t, strings.HasPrefix(prompt, "MODE: lint"))
	assert.Contains(t, prompt, "bead-lifecycle")
	assert.Contains(t, prompt, `"title": "agent: implement runner-backed bead lifecycle lint hook"`)
	assert.Contains(t, prompt, `"type": "task"`)
	assert.Contains(t, prompt, `"labels":`)
	assert.Contains(t, prompt, `"parent": "ddx-parent"`)
	assert.Contains(t, prompt, `"deps":`)
	assert.Contains(t, prompt, `"description":`)
	assert.Contains(t, prompt, `"acceptance":`)
	assert.Contains(t, prompt, `"score"`)
	assert.Contains(t, prompt, `"rationale"`)
	assert.Contains(t, prompt, `"suggested_fixes"`)
	assert.Contains(t, prompt, `"waivers_applied"`)
}

func TestLintHook_PromptOmitsVolatileExecutionFields(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)
	require.NoError(t, store.Update(context.Background(), b.ID, func(bb *bead.Bead) {
		bb.Extra["events"] = []map[string]any{
			{"kind": "execute-bead", "body": strings.Repeat("large runtime event", 1000)},
		}
		bb.Extra["events_attachment"] = "ddx-lint-001/events.jsonl"
		bb.Extra["work-last-detail"] = strings.Repeat("large last detail", 1000)
		bb.Extra["claimed-at"] = "2026-05-07T04:00:00Z"
	}))

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{
			ExitCode: 0,
			Output:   `{"score":9,"rationale":"ready","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.NoError(t, err)

	prompt := runner.lastOpts.Prompt
	assert.Contains(t, prompt, `"spec-id": "FEAT-999"`)
	assert.NotContains(t, prompt, `"events":`)
	assert.NotContains(t, prompt, `"events_attachment":`)
	assert.NotContains(t, prompt, `"work-last-detail":`)
	assert.NotContains(t, prompt, `"claimed-at":`)
	assert.NotContains(t, prompt, "large runtime event")
}

func TestLintHook_AllowsEmptyHarnessForAutoRouting(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{
			ExitCode: 0,
			Output:   `{"score":8,"rationale":"auto route","suggested_fixes":[],"waivers_applied":[]}`,
		}, nil
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{})
	got, err := NewPreDispatchLintHook(root, store, rcfg, nil, runner)(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 8, got.Score)
	assert.Empty(t, runner.lastOpts.Harness, "empty harness must pass through as auto-routing")
}

func TestLintHook_BadJSON_ReturnsInfrastructureError(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Output: "not-json"}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLintHookBadJSON)

	var lintErr *LintHookError
	require.ErrorAs(t, err, &lintErr)
	assert.Equal(t, LintHookErrorKindBadJSON, lintErr.Kind)
}

func TestLintHook_Timeout_ReturnsInfrastructureError(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		<-opts.Context.Done()
		return nil, opts.Context.Err()
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := hook(ctx, b.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLintHookCanceled)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLintHook_EmptyOutput_ReturnsInfrastructureError(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Output: "", CondensedOutput: ""}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLintHookEmptyOutput)
}

func TestLintHook_EmptyOutputWithRunnerError_ReturnsDispatchFailure(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	runner := &lintHookRunnerStub{}
	runner.run = func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 1, Output: "", CondensedOutput: "", Error: "argument list too long"}, nil
	}

	hook := NewPreDispatchLintHook(root, store, lintHookTestConfig(), nil, runner)
	_, err := hook(context.Background(), b.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLintHookDispatch)
	assert.Contains(t, err.Error(), "argument list too long")
}

func TestPreDispatchLintHook_LeavesPolicyToFizeau(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

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
				Data: []byte(`{"status":"success","final_text":"{\"score\":8,\"rationale\":\"clear-profile\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`),
			},
		},
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{Profile: "default"})
	require.Equal(t, "default", rcfg.Profile())

	hook := NewPreDispatchLintHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 8, got.Score)
	assert.Empty(t, svc.lastReq.Policy, "lint dispatch must let Fizeau choose policy")
	assert.Empty(t, svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Empty(t, svc.lastReq.Model)
	assert.Zero(t, svc.lastReq.MinPower, "lint dispatch must not inherit implementation min_power pins")
	assert.Zero(t, svc.lastReq.MaxPower, "lint dispatch must not inherit implementation max_power pins")
}

func TestPreDispatchLintHook_PreservesExplicitRoutingPins(t *testing.T) {
	root := newLintHookTestRoot(t)
	store, b := newLintHookTestStore(t, root)

	svc := &passthroughTestService{
		executeEvents: []agentlib.ServiceEvent{
			{
				Type: "final",
				Data: []byte(`{"status":"success","final_text":"{\"score\":8,\"rationale\":\"pins intact\",\"suggested_fixes\":[],\"waivers_applied\":[]}"}`),
			},
		},
	}

	rcfg := config.NewTestConfigForRun(config.TestRunConfigOpts{}).Resolve(config.CLIOverrides{
		Harness: "codex",
		Model:   "gpt-5.4-mini",
	})

	hook := NewPreDispatchLintHook(root, store, rcfg, svc, nil)
	got, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 8, got.Score)
	assert.Equal(t, "codex", svc.lastReq.Harness)
	assert.Empty(t, svc.lastReq.Provider)
	assert.Equal(t, "gpt-5.4-mini", svc.lastReq.Model)
	assert.Empty(t, svc.lastReq.Policy)
	assert.Zero(t, svc.lastReq.MinPower, "lint dispatch must not inherit implementation min_power pins")
	assert.Zero(t, svc.lastReq.MaxPower, "lint dispatch must not inherit implementation max_power pins")
}
