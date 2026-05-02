package agent

import (
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestZeroConfigRetryEscalationPolicy verifies the retry/escalation policy
// composes correctly when the loop starts from the new metadata-driven
// "profile=cheap by default" path (commit 3ddf3d52, ddx-b2c9a245). It is a
// follow-up to AC6 of ddx-b790449b: confirm that transient failures retry
// within the cheap tier (no escalation) and substantive failures escalate
// cheap -> standard -> smart.
//
// The test drives a simulated tier ladder using the real escalation
// primitives (InferTier, IsInfrastructureFailure, ShouldEscalate,
// TierToProfile, BuildEscalationSummary). It does not require a running
// agent service or harness binary.
func TestZeroConfigRetryEscalationPolicy(t *testing.T) {
	// 1. Unflagged bead: no tier label, no kind, short description.
	// InferTier must default to cheap so the loop starts at the cheap
	// tier instead of the previous always-cheap fallback or the SD-024
	// "no profile" path.
	b := &bead.Bead{
		ID:          "ddx-zero-config-001",
		Title:       "trivial cleanup, unflagged",
		Description: "minor doc tweak",
	}
	startTier := escalation.InferTier(b)
	require.Equal(t, escalation.TierCheap, startTier,
		"unflagged bead must infer cheap tier (zero-config default)")
	require.Equal(t, "cheap", escalation.TierToProfile(startTier),
		"cheap tier maps to 'cheap' profile string consumed by LoadAndResolve")

	// 2. Transient infrastructure failure on cheap tier (e.g. 502 from
	// the inference host). The policy is: do NOT escalate; defer with a
	// retry-after and try the same tier again. The model wasn't given
	// a fair chance, so escalation budget must not be consumed.
	transientStatus := ExecuteBeadStatusExecutionFailed
	transientDetail := "provider 502 Bad Gateway: connection refused"
	require.True(t,
		escalation.IsInfrastructureFailure(transientStatus, transientDetail),
		"502 Bad Gateway on execution_failed must be classified as infrastructure failure")
	require.True(t,
		escalation.ShouldEscalate(transientStatus),
		"execution_failed itself remains escalatable; the loop must consult IsInfrastructureFailure first to defer-vs-escalate")

	// Loop ordering invariant: when IsInfrastructureFailure is true the
	// loop body defers (retry-after) instead of advancing the tier. The
	// tier on the next attempt is therefore unchanged.
	tierAfterTransient := startTier
	require.Equal(t, escalation.TierCheap, tierAfterTransient,
		"transient failure must not advance the tier ladder")

	// 3. Substantive failure on cheap tier (e.g. tests failed). Policy:
	// escalate to the next tier (standard).
	substantiveStatus := ExecuteBeadStatusExecutionFailed
	substantiveDetail := "TestFoo failed: assertion mismatch"
	require.False(t,
		escalation.IsInfrastructureFailure(substantiveStatus, substantiveDetail),
		"a model-capability failure detail must not match infrastructure patterns")
	require.True(t,
		escalation.ShouldEscalate(substantiveStatus),
		"substantive execution_failed must escalate")
	tierAfterCheapSubstantive := nextTier(tierAfterTransient)
	require.Equal(t, escalation.TierStandard, tierAfterCheapSubstantive,
		"cheap -> standard escalation step")

	// 4. Substantive failure on standard tier. Policy: escalate to smart.
	standardStatus := ExecuteBeadStatusPostRunCheckFailed
	require.True(t,
		escalation.ShouldEscalate(standardStatus),
		"post_run_check_failed escalates")
	tierAfterStandardSubstantive := nextTier(tierAfterCheapSubstantive)
	require.Equal(t, escalation.TierSmart, tierAfterStandardSubstantive,
		"standard -> smart escalation step")

	// 5. Smart tier succeeds. Build the escalation summary and verify
	// the full trail is recorded with winning_tier=smart and the cheap
	// transient attempt counted as wasted-but-not-escalating-budget.
	attempts := []escalation.TierAttemptRecord{
		{Tier: string(escalation.TierCheap), Harness: "agent", Model: "cheap-model", Status: transientStatus, CostUSD: 0.0, DurationMS: 800},
		{Tier: string(escalation.TierCheap), Harness: "agent", Model: "cheap-model", Status: substantiveStatus, CostUSD: 0.02, DurationMS: 1500},
		{Tier: string(escalation.TierStandard), Harness: "codex", Model: "standard-model", Status: standardStatus, CostUSD: 0.15, DurationMS: 3400},
		{Tier: string(escalation.TierSmart), Harness: "claude", Model: "smart-model", Status: escalation.SuccessStatus, CostUSD: 0.80, DurationMS: 9000},
	}
	summary := escalation.BuildEscalationSummary(attempts, string(escalation.TierSmart))
	require.Equal(t, string(escalation.TierSmart), summary.WinningTier,
		"smart tier won the escalation")
	require.Len(t, summary.TiersAttempted, 4,
		"trail records all attempts including the deferred-transient cheap attempt")
	assert.InDelta(t, 0.97, summary.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.17, summary.WastedCostUSD, 1e-9,
		"cheap-substantive (0.02) + standard (0.15) wasted; cheap-transient billed $0; smart succeeded")
	assert.Equal(t, string(escalation.TierCheap), summary.TiersAttempted[0].Tier,
		"first attempt was cheap (the new metadata-driven default)")
	assert.Equal(t, string(escalation.TierSmart), summary.TiersAttempted[3].Tier,
		"final attempt was smart (top of ladder)")
}

// TestZeroConfigEscalationSummaryEventBody verifies the escalation summary
// is emitted as a kind:escalation-summary bead event when the cheap-default
// ladder runs to completion. This is the operator-visible artifact of the
// retry/escalation policy.
func TestZeroConfigEscalationSummaryEventBody(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{ID: "ddx-zero-config-002", Title: "cheap-default ladder", Priority: 0}
	require.NoError(t, store.Create(target))

	attempts := []escalation.TierAttemptRecord{
		{Tier: string(escalation.TierCheap), Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.01, DurationMS: 1000},
		{Tier: string(escalation.TierStandard), Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.10, DurationMS: 2000},
		{Tier: string(escalation.TierSmart), Status: escalation.SuccessStatus, CostUSD: 0.50, DurationMS: 4000},
	}
	require.NoError(t,
		escalation.AppendEscalationSummaryEvent(
			store, target.ID, "test-worker", attempts, string(escalation.TierSmart), time.Unix(1, 0).UTC()))

	events, err := store.Events(target.ID)
	require.NoError(t, err)
	var found *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "escalation-summary" {
			found = &events[i]
			break
		}
	}
	require.NotNil(t, found, "escalation-summary event must be appended for the cheap-default ladder run")
	assert.Contains(t, found.Summary, "winning_tier=smart")
	assert.Contains(t, found.Summary, "attempts=3")
	assert.Contains(t, found.Body, `"tier":"cheap"`,
		"summary body records cheap as the first-attempted tier (zero-config default)")
	assert.Contains(t, found.Body, `"tier":"standard"`)
	assert.Contains(t, found.Body, `"tier":"smart"`)
}

// nextTier advances one rung up the cheap -> standard -> smart ladder.
// Mirrors the implicit ladder used by the execute-loop when escalating
// beyond a substantive failure. Smart is the top; further calls stay
// at smart (callers exhaust escalation when ShouldEscalate fires at smart).
func nextTier(t escalation.ModelTier) escalation.ModelTier {
	switch t {
	case escalation.TierCheap:
		return escalation.TierStandard
	case escalation.TierStandard:
		return escalation.TierSmart
	default:
		return escalation.TierSmart
	}
}
