package agent

// execute_bead_land_conflict_recover_test.go — regression coverage for
// ddx-0097af14: execute-loop conflict-recovery path.
//
// AC #6a: mechanical conflict auto-resolves via ort -X ours → bead lands.
// AC #6b: structural conflict escalates to focused-resolve agent.
// AC #6c: execution_failed with preserved commit → recovery is attempted.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteBeadLoopLandConflict_AutoRecoverySucceeds_BeadCloses exercises
// AC #6a at the loop level: when the 3-way ort auto-merge succeeds, the
// preserved iteration lands and the bead is closed as a success.
func TestExecuteBeadLoopLandConflict_AutoRecoverySucceeds_BeadCloses(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-0001/20260429T000000-aabbccddeeff",
				ResultRev:   "feedface",
				BaseRev:     "badc0de",
				SessionID:   "sess-recover",
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			return "recovereddeadbeef1234", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(), // non-empty triggers recovery path
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.Successes, "auto-recovered bead must count as a success")
	assert.Equal(t, 0, result.Failures)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusClosed, got.Status, "auto-recovered bead must be closed")
	assert.Equal(t, "recovereddeadbeef1234", got.Extra["closing_commit_sha"],
		"closing_commit_sha must be the recovered merge tip")

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var sawRecovered bool
	for _, ev := range events {
		if ev.Kind == "land-conflict-auto-recovered" {
			sawRecovered = true
			assert.Contains(t, ev.Body, "preserve_ref=")
			assert.Contains(t, ev.Body, "recovereddeadbeef1234")
		}
	}
	assert.True(t, sawRecovered, "must emit kind:land-conflict-auto-recovered event")
}

// TestExecuteBeadLoopLandConflict_AutoRecoverFails_EscalatesResolver exercises
// AC #6b at the loop level: when the 3-way auto-merge fails, the loop calls
// ConflictResolver. When that also fails, the bead parks with the structured
// land_conflict_unresolvable outcome and a short cooldown.
func TestExecuteBeadLoopLandConflict_AutoRecoverFails_EscalatesResolver(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	resolverCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-0001/20260429T000000-aabbccddeeff",
				ResultRev:   "feedface",
				BaseRev:     "badc0de",
				SessionID:   "sess-conflict",
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			return "", fmt.Errorf("structural conflict: ort cannot auto-resolve")
		},
		ConflictResolver: func(ctx context.Context, beadID, preserveRef, projectRoot string) (string, bool, error) {
			resolverCalled = true
			return "", false, fmt.Errorf("focused resolve also failed")
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	startedAt := time.Now().UTC()
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, resolverCalled,
		"ConflictResolver must be called after 3-way auto-recovery fails (AC #6b)")
	assert.Equal(t, ExecuteBeadStatusLandConflictUnresolvable, result.LastFailureStatus)
	assert.Equal(t, 1, result.Failures)
	assert.Equal(t, 0, result.Successes)

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status,
		"land_conflict_unresolvable must NOT close the bead")
	assert.Empty(t, got.Owner, "bead must be unclaimed")

	// Cooldown must be set and within LandConflictCooldown window.
	retryAfter, _ := got.Extra["execute-loop-retry-after"].(string)
	require.NotEmpty(t, retryAfter, "land_conflict_unresolvable must park the bead")
	parsed, perr := time.Parse(time.RFC3339, retryAfter)
	require.NoError(t, perr)
	delta := parsed.Sub(startedAt)
	assert.LessOrEqual(t, delta, LandConflictCooldown+time.Minute,
		"cooldown must be at most LandConflictCooldown (15 min)")
	assert.GreaterOrEqual(t, delta, 5*time.Minute-time.Second,
		"cooldown must be at least ~5 min (within 5-30 min range)")

	// Structured event.
	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var sawUnresolvable bool
	for _, ev := range events {
		if ev.Kind == "land-conflict-unresolvable" {
			sawUnresolvable = true
			assert.Contains(t, ev.Body, "preserve_ref",
				"land-conflict-unresolvable event body must record preserve_ref")
		}
	}
	assert.True(t, sawUnresolvable, "must emit kind:land-conflict-unresolvable event")
}

// TestExecuteBeadLoopLandConflict_BlockingResolver_NeedsHuman verifies that
// when ConflictResolver signals isBlocking=true, the bead parks with
// land_conflict_needs_human and a kind:land-conflict-needs-human event.
func TestExecuteBeadLoopLandConflict_BlockingResolver_NeedsHuman(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-0001/20260429T000000-aabbccddeeff",
				ResultRev:   "feedface",
				BaseRev:     "badc0de",
				SessionID:   "sess-block",
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			return "", fmt.Errorf("cannot auto-resolve")
		},
		ConflictResolver: func(ctx context.Context, beadID, preserveRef, projectRoot string) (string, bool, error) {
			return "", true, fmt.Errorf("BLOCKING: human review required")
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)
	assert.Equal(t, ExecuteBeadStatusLandConflictNeedsHuman, result.LastFailureStatus)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	var sawNeedsHuman bool
	for _, ev := range events {
		if ev.Kind == "land-conflict-needs-human" {
			sawNeedsHuman = true
		}
	}
	assert.True(t, sawNeedsHuman, "must emit kind:land-conflict-needs-human event on blocking resolver")
}

// TestExecuteBeadLoopExecutionFailed_WithPreservedCommit_AttemptsRecovery
// exercises AC #6c: when execution_failed carries a PreserveRef and a ResultRev
// that differs from BaseRev (i.e. a commit was produced before failure, as
// happens on a timeout), the loop must attempt conflict recovery before
// discarding the run.
func TestExecuteBeadLoopExecutionFailed_WithPreservedCommit_AttemptsRecovery(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	autoRecoverCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusExecutionFailed,
				Detail:      "agent timed out at test phase",
				PreserveRef: "refs/ddx/iterations/ddx-0001/20260429T175646Z-aa7b31785a44",
				ResultRev:   "feedface",
				BaseRev:     "badc0de",
				SessionID:   "sess-timeout",
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			autoRecoverCalled = true
			return "", fmt.Errorf("recovery failed: not a git repo")
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(), // non-empty triggers recovery path
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, autoRecoverCalled,
		"landConflictAutoRecover must be attempted for execution_failed with preserved commit (AC #6c)")
	assert.Equal(t, ExecuteBeadStatusLandConflictUnresolvable, result.LastFailureStatus,
		"after recovery failure the bead must park as land_conflict_unresolvable")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status, "bead must stay open")
	assert.Empty(t, got.Owner, "bead must be unclaimed")
	assert.NotEmpty(t, got.Extra["execute-loop-retry-after"],
		"timeout-with-preserved-commit must park bead via execute-loop-retry-after")
}

// TestExecuteBeadLoopExecutionFailed_NoPreserveRef_SkipsRecovery ensures that
// execution_failed without a PreserveRef takes the existing fallback path (no
// cooldown, no recovery attempt) and is not accidentally promoted to the new
// recovery branch.
func TestExecuteBeadLoopExecutionFailed_NoPreserveRef_SkipsRecovery(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	autoRecoverCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:    beadID,
				Status:    ExecuteBeadStatusExecutionFailed,
				Detail:    "harness crashed",
				ResultRev: "feedface",
				BaseRev:   "badc0de",
				// PreserveRef is intentionally absent
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			autoRecoverCalled = true
			return "", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once:        true,
		ProjectRoot: t.TempDir(),
	})
	require.NoError(t, err)

	assert.False(t, autoRecoverCalled,
		"recovery must NOT be attempted when PreserveRef is absent")
	assert.Equal(t, ExecuteBeadStatusExecutionFailed, result.LastFailureStatus,
		"status must remain execution_failed when recovery is skipped")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
}

// TestExecuteBeadLoopLandConflict_EmptyProjectRoot_SkipsRecovery verifies that
// when ProjectRoot is empty (e.g. loop invoked without project context), the
// recovery path is bypassed and the bead falls through to the existing generic
// failure handler — preserving pre-bead-0097af14 behavior for callers that do
// not supply a project root.
func TestExecuteBeadLoopLandConflict_EmptyProjectRoot_SkipsRecovery(t *testing.T) {
	store, first, _ := newExecuteLoopTestStore(t)

	autoRecoverCalled := false
	worker := &ExecuteBeadWorker{
		Store: store,
		Executor: ExecuteBeadExecutorFunc(func(ctx context.Context, beadID string) (ExecuteBeadReport, error) {
			return ExecuteBeadReport{
				BeadID:      beadID,
				Status:      ExecuteBeadStatusLandConflict,
				Detail:      "merge conflict",
				PreserveRef: "refs/ddx/iterations/ddx-0001/20260429T000000-aabbccddeeff",
				ResultRev:   "feedface",
				BaseRev:     "badc0de",
			}, nil
		}),
		conflictAutoRecoverFn: func(wd, preserveRef string, gitOps LandingGitOps) (string, error) {
			autoRecoverCalled = true
			return "", nil
		},
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))
	result, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{
		Once: true,
		// ProjectRoot intentionally empty
	})
	require.NoError(t, err)

	assert.False(t, autoRecoverCalled, "recovery must be skipped when ProjectRoot is empty")
	assert.Equal(t, ExecuteBeadStatusLandConflict, result.LastFailureStatus,
		"status must remain land_conflict when recovery is bypassed")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	assert.Equal(t, bead.StatusOpen, got.Status)
	assert.Empty(t, got.Owner)

	events, err := store.Events(first.ID)
	require.NoError(t, err)
	require.Len(t, events, 1, "exactly one execute-bead event when recovery is skipped")
	assert.Equal(t, ExecuteBeadStatusLandConflict, events[0].Summary)
	assert.True(t, strings.Contains(events[0].Body, "preserve_ref="),
		"event body must contain preserve_ref")
}
