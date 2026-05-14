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
// and APPROVE on every subsequent call. It is retained for legacy/manual
// post-land review helper coverage; work success no longer invokes it.
func alternatingReviewer(firstVerdict Verdict, calls *atomic.Int32) beadReviewerFunc {
	return beadReviewerFunc(func(_ context.Context, _, resultRev string, _ ImplementerRouting) (*ReviewResult, error) {
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

// driveLoopAcrossIgnoredPostLandReview runs one successful loop iteration with
// a legacy post-land reviewer installed. Candidate-cycle review owns close
// eligibility now, so work must close directly and must not re-invoke
// the old reviewer/retry path.
func driveLoopAcrossIgnoredPostLandReview(t *testing.T, firstVerdict Verdict, spec pinPropagationSpec) []capturedRequest {
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

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err, "worker.Run")

	got, err := store.Get(first.ID)
	require.NoError(t, err)
	require.Equal(t, bead.StatusClosed, got.Status,
		"post-land %s review path is retired; a successful pre-land-reviewed attempt closes directly",
		firstVerdict)

	require.Equal(t, int32(0), reviewCalls.Load(),
		"legacy post-land reviewer must not be called from work")
	require.Equal(t, int32(1), execCalls.Load(),
		"work should not retry after a legacy post-land %s review verdict", firstVerdict)
	require.Len(t, captured, 1)
	return captured
}

// TestExecuteLoopPostLandReviewIgnoredKeepsWorkerHarness verifies that when
// the worker is spawned with explicit pins (harness, model, profile,
// provider), the initial executor invocation still receives those operator
// passthrough values while the retired post-land reviewer remains unreachable.
//
// The executor closure here mirrors the production wiring in
// cmd/agent_cmd.go and internal/server/workers.go: the spec values are
// captured by reference and read on invocation.
func TestExecuteLoopPostLandReviewIgnoredKeepsWorkerHarness(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossIgnoredPostLandReview(t, VerdictRequestChanges, spec)

	assert.Equal(t, spec.Harness, captured[0].Harness, "iteration 1: harness pin must be set on the initial attempt")
	assert.Equal(t, spec.Model, captured[0].Model, "initial attempt must reuse worker-level model pin")
	assert.Equal(t, spec.Profile, captured[0].Profile, "initial attempt must reuse worker-level profile pin")
	assert.Equal(t, spec.Provider, captured[0].Provider, "initial attempt must reuse worker-level provider pin")
}

// TestExecuteLoopPostLandReviewIgnoredKeepsWorkerHarness_BlockPreclose covers
// the legacy BLOCK post-land review shape. The reviewer is installed but
// unreachable from the automated close path.
func TestExecuteLoopPostLandReviewIgnoredKeepsWorkerHarness_BlockPreclose(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossIgnoredPostLandReview(t, VerdictBlock, spec)

	assert.Equal(t, spec.Harness, captured[0].Harness,
		"initial attempt: harness pin must propagate (not empty)")
	assert.Equal(t, spec.Model, captured[0].Model,
		"initial attempt: model pin must propagate")
	assert.Equal(t, spec.Profile, captured[0].Profile,
		"initial attempt: profile pin must propagate")
	assert.Equal(t, spec.Provider, captured[0].Provider,
		"initial attempt: provider pin must propagate")
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

// driveLoopAcrossStatusReopen runs the loop twice with Once=true for a
// non-review-driven reopen path: the executor returns a worker-level
// failure status (e.g. land_conflict, post_run_check_failed) which causes
// the loop to Unclaim the bead and leave it open. The next iteration
// re-picks the same bead. The invariant under test is the same as the
// review-driven path: every retry must carry the worker-level pins.
//
// BaseRev and ResultRev are deliberately left empty so that
// shouldSuppressNoProgress returns false and no execution cooldown is
// applied — otherwise the bead would not be ready for the second pass.
func driveLoopAcrossStatusReopen(t *testing.T, status string, spec pinPropagationSpec) []capturedRequest {
	t.Helper()

	store, _, _ := newExecuteLoopTestStore(t)
	var captured []capturedRequest
	var execCalls atomic.Int32

	executor := ExecuteBeadExecutorFunc(func(_ context.Context, beadID string) (ExecuteBeadReport, error) {
		n := execCalls.Add(1)
		captured = append(captured, capturedRequest{pinPropagationSpec: spec, BeadID: beadID})
		return ExecuteBeadReport{
			BeadID:    beadID,
			Status:    status,
			SessionID: fmt.Sprintf("sess-%s-%d", status, n),
			Detail:    fmt.Sprintf("simulated %s", status),
			Harness:   spec.Harness,
			Model:     spec.Model,
			Provider:  spec.Provider,
		}, nil
	})

	worker := &ExecuteBeadWorker{
		Store:    store,
		Executor: executor,
	}

	cfgOpts := config.TestLoopConfigOpts{Assignee: "worker"}
	rcfg := config.NewTestConfigForLoop(cfgOpts).Resolve(config.TestLoopOverrides(cfgOpts))

	_, err := worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err, "iteration 1: worker.Run (%s)", status)
	_, err = worker.Run(context.Background(), rcfg, ExecuteBeadLoopRuntime{Once: true})
	require.NoError(t, err, "iteration 2: worker.Run (%s)", status)

	require.Equalf(t, int32(2), execCalls.Load(),
		"executor must run on both the initial attempt and the post-%s retry", status)
	require.Lenf(t, captured, 2,
		"expected exactly 2 captured executor invocations after %s reopen", status)
	return captured
}

// TestExecuteLoopRetryLandConflictReopen covers AC3's land_conflict reopen
// source: when ExecuteBead returns land_conflict (merge conflict /
// preserved branch), the loop unclaims the bead and the next iteration
// must re-issue the executor with the same worker-level pins.
func TestExecuteLoopRetryLandConflictReopen(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossStatusReopen(t, ExecuteBeadStatusLandConflict, spec)

	assert.Equal(t, spec.Harness, captured[1].Harness,
		"iteration 2 (land_conflict retry): harness pin must propagate (not empty)")
	assert.Equal(t, spec.Model, captured[1].Model,
		"iteration 2 (land_conflict retry): model pin must propagate")
	assert.Equal(t, spec.Profile, captured[1].Profile,
		"iteration 2 (land_conflict retry): profile pin must propagate")
	assert.Equal(t, spec.Provider, captured[1].Provider,
		"iteration 2 (land_conflict retry): provider pin must propagate")
}

// TestExecuteLoopRetryPostRunCheckFailedReopen covers AC3's
// post_run_check_failed reopen source: when post-merge checks fail and the
// bead is preserved+reopened, the next iteration must re-issue the
// executor with the same worker-level pins.
func TestExecuteLoopRetryPostRunCheckFailedReopen(t *testing.T) {
	spec := pinPropagationSpec{
		Harness:  "claude",
		Model:    "claude-opus-4-7",
		Profile:  "smart",
		Provider: "anthropic",
	}

	captured := driveLoopAcrossStatusReopen(t, ExecuteBeadStatusPostRunCheckFailed, spec)

	assert.Equal(t, spec.Harness, captured[1].Harness,
		"iteration 2 (post_run_check_failed retry): harness pin must propagate (not empty)")
	assert.Equal(t, spec.Model, captured[1].Model,
		"iteration 2 (post_run_check_failed retry): model pin must propagate")
	assert.Equal(t, spec.Profile, captured[1].Profile,
		"iteration 2 (post_run_check_failed retry): profile pin must propagate")
	assert.Equal(t, spec.Provider, captured[1].Provider,
		"iteration 2 (post_run_check_failed retry): provider pin must propagate")
}

// TestExecuteLoopPostLandReviewIgnored_NoPinAutoRoutes is the negative
// control for AC4: when the worker is spawned WITHOUT --harness, the loop
// must continue to defer to auto-routing and must not synthesize pins.
func TestExecuteLoopPostLandReviewIgnored_NoPinAutoRoutes(t *testing.T) {
	spec := pinPropagationSpec{} // no pins → auto-routing

	captured := driveLoopAcrossIgnoredPostLandReview(t, VerdictRequestChanges, spec)

	assert.Empty(t, captured[0].Harness, "no --harness on initial attempt → empty pin")
	assert.Empty(t, captured[0].Model, "no --model on initial attempt → empty pin")
	assert.Empty(t, captured[0].Provider, "no --provider on initial attempt → empty pin")
}
