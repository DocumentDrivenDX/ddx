package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostLadderExhaustion_TriggersDecompose_ReviewTooLarge verifies that a
// TooLarge failure class routes to runDecomposer and, on valid children,
// creates child beads and sets parent execution-eligible=false.
func TestPostLadderExhaustion_TriggersDecompose_ReviewTooLarge(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-decompose-toolarge",
		Title:       "decompose too-large test",
		Description: "PROBLEM\nThis bead is too large.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42 does too much.\n",
		Acceptance:  "1. TestPostLadderExhaustion_TriggersDecompose_ReviewTooLarge passes\n2. cd cli && go test ./internal/agent/... green\n",
	}
	require.NoError(t, store.Create(context.

		// Pre-seed counter to 1 so the next budget exhaustion hits the threshold.
		Background(), b))

	require.NoError(t, incrementConsecutiveLadderExhaustions(store, b.ID))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	validChildren := []map[string]interface{}{
		{
			"title":       "feat(agent): implement child one",
			"description": "PROBLEM\nChild one task.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
			"acceptance":  "1. TestChildOne passes\n2. cd cli && go test ./internal/agent/... green\n",
			"labels":      []string{"phase:6", "area:agent"},
		},
		{
			"title":       "feat(agent): implement child two",
			"description": "PROBLEM\nChild two task.\n\nROOT CAUSE\ncli/internal/agent/bar.go:10.\n",
			"acceptance":  "1. TestChildTwo passes\n2. cd cli && go test ./internal/agent/... green\n",
			"labels":      []string{"phase:6", "area:agent"},
		},
	}

	var decomposeDispatched bool
	decomposeRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		decomposeDispatched = true
		cancel() // stop the loop after the decomposer fires
		out, _ := json.Marshal(validChildren)
		return &Result{
			ExitCode: 0,
			Output:   string(out),
			CostUSD:  0.0123,
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	hook := NewDecomposePostLadderExhaustionHook(store, decomposeRunner, rcfg, t.TempDir())

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed + "_" + ReviewTerminalClassTooLarge,
				Detail:    escalation.PerBeadBudgetExhaustedReason + ": $1.00 billed >= $0.50 per-bead budget",
				SessionID: "sess-decompose-toolarge",
			}, nil
		}),
	}

	_, _ = worker.Run(ctx, rcfg, ExecuteBeadLoopRuntime{
		Mode:                     executeloop.ModeDrain,
		PostLadderExhaustionHook: hook,
		ProjectRoot:              t.TempDir(),
		SessionID:                "sess-decompose-toolarge",
		WorkerID:                 "worker-decompose-toolarge",
	})

	assert.True(t, decomposeDispatched, "decomposer must be dispatched for TooLarge class")

	got, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, false, got.Extra[bead.ExtraExecutionElig], "parent execution-eligible must be false after decompose")

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var childBeads []bead.Bead
	for _, bb := range all {
		if bb.Parent == b.ID {
			childBeads = append(childBeads, bb)
		}
	}
	assert.Len(t, childBeads, 2, "two child beads must be created")

	events, err := store.Events(b.ID)
	require.NoError(t, err)
	var decomposeApplied bool
	for _, ev := range events {
		if ev.Kind == "decompose-applied" {
			decomposeApplied = true
			assert.Contains(t, ev.Body, "child_ids", "decompose-applied event body must contain child_ids")
			assert.Contains(t, ev.Body, "cost_usd", "decompose-applied event body must contain cost_usd")
		}
	}
	assert.True(t, decomposeApplied, "decompose-applied event must be emitted")
}

func TestPostLadderDecomposerDispatchesOutsideProjectRoot(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	projectRoot := t.TempDir()

	b := &bead.Bead{
		ID:          "ddx-post-ladder-dispatch",
		Title:       "decompose dispatch isolation test",
		Description: "PROBLEM\nToo large.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
		Acceptance:  "1. TestPostLadderDecomposerDispatchesOutsideProjectRoot\n2. cd cli && go test ./internal/agent/...",
	}
	require.NoError(t, store.Create(context.Background(), b))

	children := []map[string]any{
		{
			"title":       "child dispatch one",
			"description": "PROBLEM\nChild one.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
			"acceptance":  "1. TestChildDispatchOne\n2. cd cli && go test ./internal/agent/...",
			"labels":      []string{"phase:iterate", "area:agent"},
		},
	}
	raw, err := json.Marshal(children)
	require.NoError(t, err)

	var gotWorkDir string
	var gotPermissions string
	var childCountBefore int
	var parentEligSetBefore bool
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		gotWorkDir = opts.WorkDir
		gotPermissions = opts.Permissions

		all, readErr := store.ReadAll(context.Background())
		require.NoError(t, readErr)
		for _, bb := range all {
			if bb.Parent == b.ID {
				childCountBefore++
			}
		}
		parent, getErr := store.Get(context.Background(), b.ID)
		require.NoError(t, getErr)
		if parent.Extra != nil {
			_, parentEligSetBefore = parent.Extra[bead.ExtraExecutionElig]
		}

		return &Result{ExitCode: 0, Output: string(raw), CostUSD: 0.0123}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result := runDecomposer(context.Background(), store, runner, rcfg, projectRoot, b.ID)
	assert.False(t, result.Failed)
	assert.Len(t, result.ChildIDs, 1)
	assert.Equal(t, 0, childCountBefore, "children must not exist until after validated output is returned")
	assert.False(t, parentEligSetBefore, "parent execution-eligible flag must not be updated before validated output is returned")
	assert.NotEqual(t, projectRoot, gotWorkDir)
	assert.False(t, isPathWithin(gotWorkDir, projectRoot))
	assert.Equal(t, "safe", gotPermissions)
	assert.NotEqual(t, PermissionsReadOnlyReviewer, gotPermissions)
	assert.True(t, strings.HasPrefix(filepath.Base(gotWorkDir), lifecycleScratchDirPrefix))

	parent, err := store.Get(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, false, parent.Extra[bead.ExtraExecutionElig])

	all, err := store.ReadAll(context.Background())
	require.NoError(t, err)
	var childBeads []bead.Bead
	for _, bb := range all {
		if bb.Parent == b.ID {
			childBeads = append(childBeads, bb)
		}
	}
	assert.Len(t, childBeads, 1)
}

// TestDecomposerInvalidChildren_CountsAsFailure verifies that a stub agent
// returning more than 5 children yields DecomposeResult{Failed:true,
// Reason:"invalid_count"}.
func TestDecomposerInvalidChildren_CountsAsFailure(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-decompose-invalid",
		Title:       "decompose invalid count test",
		Description: "PROBLEM\nInvalid count test.",
		Acceptance:  "1. TestDecomposerInvalidChildren_CountsAsFailure\n",
	}
	require.NoError(t, store.Create(context.

		// Return 6 children — exceeds max of 5.
		Background(), b))

	tooManyChildren := make([]map[string]interface{}, 6)
	for i := range tooManyChildren {
		tooManyChildren[i] = map[string]interface{}{
			"title":       fmt.Sprintf("child %d", i+1),
			"description": "description",
			"acceptance":  "1. test",
			"labels":      []string{},
		}
	}

	overflowRunner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		out, _ := json.Marshal(tooManyChildren)
		return &Result{
			ExitCode: 0,
			Output:   string(out),
		}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	result := runDecomposer(context.Background(), store, overflowRunner, rcfg, t.TempDir(), b.ID)
	assert.True(t, result.Failed, "must be marked as failed for >5 children")
	assert.Equal(t, "invalid_count", result.Reason)
}

func TestPreClaimDecompositionHook_ParsesAgentSplit(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-preclaim-decompose",
		Title:       "preclaim split test",
		Description: "PROBLEM\nToo large.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
		Acceptance:  "1. TestPreClaimDecompositionHook_ParsesAgentSplit\n2. cd cli && go test ./internal/agent/...",
		Labels:      []string{"phase:iterate", "area:agent"},
	}
	require.NoError(t, store.Create(context.Background(), b))

	payload := map[string]any{
		"children": []map[string]any{
			{
				"title":       "agent: split preclaim child",
				"description": "PROBLEM\nChild.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n\nPROPOSED FIX\nDo child work.\n\nNON-SCOPE\nOther work.",
				"acceptance":  "1. TestPreClaimChild passes\n2. cd cli && go test ./internal/agent/...\n3. lefthook run pre-commit",
				"labels":      []string{"phase:iterate", "area:agent"},
			},
		},
		"ac_map": []map[string]any{
			{"parent_ac": "1. TestPreClaimDecompositionHook_ParsesAgentSplit", "coverage": "agent: split preclaim child AC 1"},
			{"parent_ac": "2. cd cli && go test ./internal/agent/...", "coverage": "agent: split preclaim child AC 2"},
		},
		"rationale": "one executable child covers the parent verification slice",
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var gotPrompt string
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		gotPrompt = opts.Prompt
		return &Result{ExitCode: 0, Output: string(raw)}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	hook := NewPreClaimDecompositionHook(store, runner, rcfg, t.TempDir())

	decomp, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	require.NotNil(t, decomp)
	require.Len(t, decomp.Children, 1)
	assert.Equal(t, "agent: split preclaim child", decomp.Children[0].Title)
	assert.Len(t, decomp.ACMap, 2)
	assert.Contains(t, gotPrompt, "MODE: preclaim-decompose")
	assert.Contains(t, gotPrompt, "ac_map")
}

func TestPreClaimDecomposerDispatchesOutsideProjectRoot(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	projectRoot := t.TempDir()

	b := &bead.Bead{
		ID:          "ddx-preclaim-dispatch",
		Title:       "preclaim dispatch isolation test",
		Description: "PROBLEM\nToo large.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
		Acceptance:  "1. TestPreClaimDecomposerDispatchesOutsideProjectRoot\n2. cd cli && go test ./internal/agent/...",
		Labels:      []string{"phase:iterate", "area:agent"},
	}
	require.NoError(t, store.Create(context.Background(), b))

	payload := map[string]any{
		"children": []map[string]any{
			{
				"title":       "preclaim isolated child",
				"description": "PROBLEM\nChild.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n\nPROPOSED FIX\nDo child work.\n\nNON-SCOPE\nOther work.",
				"acceptance":  "1. TestPreClaimIsolatedChild\n2. cd cli && go test ./internal/agent/...\n3. lefthook run pre-commit",
				"labels":      []string{"phase:iterate", "area:agent"},
			},
		},
		"ac_map": []map[string]any{
			{"parent_ac": "1. TestPreClaimDecomposerDispatchesOutsideProjectRoot", "coverage": "covered by preclaim isolated child AC 1"},
			{"parent_ac": "2. cd cli && go test ./internal/agent/...", "coverage": "covered by preclaim isolated child AC 2"},
		},
		"rationale": "isolation still preserves validated child proposals",
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var gotWorkDir string
	var gotPermissions string
	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		gotWorkDir = opts.WorkDir
		gotPermissions = opts.Permissions
		return &Result{ExitCode: 0, Output: string(raw)}, nil
	})

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	decomp, err := runPreClaimDecomposer(context.Background(), store, runner, rcfg, projectRoot, b.ID)
	require.NoError(t, err)
	require.NotNil(t, decomp)
	require.Len(t, decomp.Children, 1)
	assert.Equal(t, "preclaim isolated child", decomp.Children[0].Title)
	assert.Len(t, decomp.ACMap, 2)
	assert.NotEqual(t, projectRoot, gotWorkDir)
	assert.False(t, isPathWithin(gotWorkDir, projectRoot))
	assert.Equal(t, "safe", gotPermissions)
	assert.NotEqual(t, PermissionsReadOnlyReviewer, gotPermissions)
	assert.True(t, strings.HasPrefix(filepath.Base(gotWorkDir), lifecycleScratchDirPrefix))
}

func TestPreClaimDecompositionHook_FallsBackWhenAgentOutputEmpty(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	b := &bead.Bead{
		ID:          "ddx-preclaim-fallback",
		Title:       "fallback split test",
		Description: "PROBLEM\nToo large.\n\nROOT CAUSE\ncli/internal/agent/foo.go:42.\n",
		Acceptance: strings.Join([]string{
			"1. TestOne covers first slice",
			"2. TestTwo covers second slice",
			"3. TestThree covers third slice",
			"4. TestFour covers fourth slice",
			"5. cd cli && go test ./internal/agent/... passes",
			"6. lefthook run pre-commit passes",
		}, "\n"),
		Labels: []string{"phase:iterate", "area:agent"},
	}
	require.NoError(t, store.Create(context.Background(), b))

	runner := reframeRunnerFunc(func(opts RunArgs) (*Result, error) {
		return &Result{ExitCode: 0, Output: ""}, nil
	})
	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	hook := NewPreClaimDecompositionHook(store, runner, rcfg, t.TempDir())

	decomp, err := hook(context.Background(), b.ID)
	require.NoError(t, err)
	require.NotNil(t, decomp)
	assert.Len(t, decomp.Children, 3)
	assert.Len(t, decomp.ACMap, 6)
	assert.Contains(t, decomp.Rationale, "deterministic fallback split")
	for _, child := range decomp.Children {
		assert.Contains(t, child.Description, "PROBLEM")
		assert.Contains(t, child.Labels, "decomposed")
	}
}
