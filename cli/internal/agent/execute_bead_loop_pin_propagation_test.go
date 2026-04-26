package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinPropagationSpec mirrors the worker-level pins captured by the executor
// closures in cmd/agent_cmd.go (CLI dispatch) and internal/server/workers.go
// (server dispatch). Production code captures these into the closure body and
// reads them on every invocation; this struct lets a regression test simulate
// the same wiring and verify per-iteration values.
type pinPropagationSpec struct {
	Harness  string
	Model    string
	Profile  string
	Provider string
}

type capturedRequest struct {
	pinPropagationSpec
	BeadID string
}

// pinCapturingExecutor models the production executor closures: it records
// the worker-level pins observed on every invocation. Production closures
// build a service.Execute / RouteRequest from these pins, so any iteration
// that drops a pin would surface as an empty captured value.
func pinCapturingExecutor(spec pinPropagationSpec, captured *[]capturedRequest, calls *atomic.Int32) ExecuteBeadExecutorFunc {
	return ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
		n := calls.Add(1)
		*captured = append(*captured, capturedRequest{
			pinPropagationSpec: spec,
			BeadID:             beadID,
		})
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    ExecuteBeadStatusSuccess,
			SessionID: fmt.Sprintf("sess-pin-%d", n),
			ResultRev: fmt.Sprintf("rev-pin-%d", n),
			Harness:   spec.Harness,
			Model:     spec.Model,
			Provider:  spec.Provider,
		}, nil
	})
}

// alternatingReviewer returns REQUEST_CHANGES (or BLOCK) on the first call
// and APPROVE on every subsequent call. Used to drive the loop into a
// review-driven reopen on the first iteration and let the second iteration
// close cleanly.
func alternatingReviewer(firstVerdict Verdict, calls *atomic.Int32) BeadReviewerFunc {
	return BeadReviewerFunc(func(_ context.Context, _, resultRev, _, _ string) (*ReviewResult, error) {
		n := calls.Add(1)
		verdict := VerdictApprove
		rationale := "looks good"
		if n == 1 {
			verdict = firstVerdict
			rationale = "needs more work"
		}
		return &ReviewResult{
			Verdict:         verdict,
			Rationale:       rationale,
			ResultRev:       resultRev,
			ReviewerHarness: "reviewer",
			ReviewerModel:   "reviewer-model",
		}, nil
	})
}

// driveLoopAcrossReopen runs the loop twice with Once=true, asserting that
// the bead reopens between iterations and the executor is invoked on both
// passes. Returns the captured requests so callers can assert pin values.
func driveLoopAcrossReopen(t *testing.T, firstVerdict Verdict, spec pinPropagationSpec) []capturedRequest {
	t.Helper()

	store, first, _ := newExecuteLoopTestStore(t)
	var captured []capturedRequest
	var execCalls atomic.Int32
	var reviewCalls atomic.Int32

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: pinCapturingExecutor(spec, &captured, &execCalls),
		Reviewer: alternatingReviewer(firstVerdict, &reviewCalls),
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	// Iteration 1: success → reviewer returns firstVerdict → bead reopens.
	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err, "iteration 1: worker.Run")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusOpen, got.Status,
		"bead must be reopened after %s so the loop re-picks it on the next iteration",
		firstVerdict)

	// Iteration 2: bead is open and unclaimed; loop re-picks it and the
	// executor closure is invoked again. Production wiring must pass the
	// same worker-level pins (harness, model, profile, provider) on this
	// retry — that is the invariant under test.
	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err, "iteration 2: worker.Run")

	require.Equalf(t, int32(2), execCalls.Load(),
		"executor must run on both the initial attempt and the post-%s retry", firstVerdict)
	require.Lenf(t, captured, 2,
		"expected exactly 2 captured executor invocations after %s reopen", firstVerdict)
	return captured
}

// TestExecuteLoopRetryReusesWorkerHarness is the FEAT-006 regression test
// for ddx-c67969d5: when the worker is spawned with explicit pins (harness,
// model, profile, provider), every retry triggered by a review-driven reopen
// must re-issue the next iteration's executor invocation with the same pins.
//
// The executor closure here mirrors the production wiring in
// cmd/agent_cmd.go and internal/server/workers.go: the spec values are
// captured by reference and read on each invocation. A regression that
// recreated the closure with empty pins between iterations — or otherwise
// dropped the worker-level overrides on retry — would surface as an empty
// captured value on the second attempt.
func TestExecuteLoopRetryReusesWorkerHarness(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossReopen(t, VerdictRequestChanges, spec)

	// AC1 (REQUEST_CHANGES path): the post-REQUEST_CHANGES retry must
	// carry the same Harness as the initial attempt — not "".
	assert.Equal(t, spec.Harness, captured[0].Harness, "iteration 1: harness pin must be set on the initial attempt")
	assert.Equal(t, spec.Harness, captured[1].Harness,
		"iteration 2 (REQUEST_CHANGES retry): harness pin must propagate (not empty)")

	// AC2: --model, --profile, --provider must propagate on retry too.
	assert.Equal(t, spec.Model, captured[1].Model, "retry must reuse worker-level model pin")
	assert.Equal(t, spec.Profile, captured[1].Profile, "retry must reuse worker-level profile pin")
	assert.Equal(t, spec.Provider, captured[1].Provider, "retry must reuse worker-level provider pin")
}

// TestExecuteLoopRetryReusesWorkerHarness_BlockReopen covers the second
// review-driven reopen source: BLOCK. Same invariant — every reopen path
// must re-issue the executor with the worker-level pins still attached.
func TestExecuteLoopRetryReusesWorkerHarness_BlockReopen(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossReopen(t, VerdictBlock, spec)

	assert.Equal(t, spec.Harness, captured[1].Harness,
		"iteration 2 (BLOCK retry): harness pin must propagate (not empty)")
	assert.Equal(t, spec.Model, captured[1].Model,
		"iteration 2 (BLOCK retry): model pin must propagate")
	assert.Equal(t, spec.Profile, captured[1].Profile,
		"iteration 2 (BLOCK retry): profile pin must propagate")
	assert.Equal(t, spec.Provider, captured[1].Provider,
		"iteration 2 (BLOCK retry): provider pin must propagate")
}

// TestExecuteLoopRetryNoChangesReopen covers the no_changes-into-reopen
// path: when MaxNoChangesBeforeClose is reached, the loop closes the bead
// via the satisfied path. Until then, the bead is left open (Unclaim'd)
// with a cooldown — but on cooldown expiry the next iteration must still
// see the same worker-level pins. This test drives MaxNoChangesBeforeClose=2
// so the second attempt produces the closure-or-cooldown decision; either
// way the executor must observe the original pins on the retry.
func TestExecuteLoopRetryNoChangesReopen(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	store, first, _ := newExecuteLoopTestStore(t)
	var captured []capturedRequest
	var execCalls atomic.Int32

	executor := ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
		n := execCalls.Add(1)
		captured = append(captured, capturedRequest{pinPropagationSpec: spec, BeadID: beadID})
		// no_changes: ResultRev == BaseRev (both empty here), and the loop
		// applies cooldown / counts toward MaxNoChangesBeforeClose. A
		// rationale is required so the loop doesn't trip the missing-
		// rationale gate.
		return ExecuteBeadReport{
			BeadID:             beadID,
			Status:             ExecuteBeadStatusNoChanges,
			SessionID:          fmt.Sprintf("sess-noc-%d", n),
			NoChangesRationale: "no progress this attempt",
		}, nil
	})

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: executor,
	}

	// MaxNoChangesBeforeClose=10, NoProgressCooldown=0 so the loop does not
	// suspend the bead between iterations and we can drive multiple attempts
	// back-to-back.
	cfgOpts := config.TestLoopConfigOpts{
		Assignee:                "worker",
		MaxNoChangesBeforeClose: 10,
	}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)
	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err)

	require.Equal(t, int32(2), execCalls.Load(),
		"no_changes leaves the bead open; the next iteration must re-invoke the executor")
	require.Len(t, captured, 2)
	assert.Equal(t, spec.Harness, captured[1].Harness,
		"no_changes retry must carry the worker-level harness pin")
	assert.Equal(t, spec.Model, captured[1].Model,
		"no_changes retry must carry the worker-level model pin")
	assert.Equal(t, spec.Profile, captured[1].Profile,
		"no_changes retry must carry the worker-level profile pin")
	assert.Equal(t, spec.Provider, captured[1].Provider,
		"no_changes retry must carry the worker-level provider pin")
	_ = first
}

// TestExecuteLoopRetryReusesWorkerHarness_NoPinAutoRoutes is the negative
// control for AC4: when the worker is spawned WITHOUT --harness, retries
// must continue to defer to auto-routing — i.e., the captured pin stays
// empty on every iteration. This guards against an over-eager "fix" that
// pins a routed harness from the first attempt onto subsequent retries.
func TestExecuteLoopRetryReusesWorkerHarness_NoPinAutoRoutes(t *testing.T) {
	spec := pinPropagationSpec{} // no pins → auto-routing

	captured := driveLoopAcrossReopen(t, VerdictRequestChanges, spec)

	assert.Empty(t, captured[0].Harness, "no --harness on initial attempt → empty pin")
	assert.Empty(t, captured[1].Harness, "no --harness on retry → still empty (auto-routing preserved)")
	assert.Empty(t, captured[1].Model, "no --model on retry → still empty")
	assert.Empty(t, captured[1].Provider, "no --provider on retry → still empty")
}
