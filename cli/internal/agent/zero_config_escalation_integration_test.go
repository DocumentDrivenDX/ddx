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
// composes correctly when the loop starts from the default standard powerClass.
// Transient failures retry within the same powerClass and substantive failures
// escalate standard -> smart.
//
// The test drives a simulated powerClass ladder using the real escalation
// primitives (InferPowerClass, IsInfrastructureFailure, ShouldEscalate,
// BuildEscalationSummary). It does not require a running
// agent service or harness binary.
func TestZeroConfigRetryEscalationPolicy(t *testing.T) {
	// 1. Unflagged bead: no explicit power hint.
	// InferPowerClass must default to standard; cheap and smart require the
	// single explicit bead metadata hint or later retry escalation.
	b := &bead.Bead{
		ID:          "ddx-zero-config-001",
		Title:       "trivial cleanup, unflagged",
		Description: "minor doc tweak",
	}
	startPowerClass := escalation.InferPowerClass(b)
	require.Equal(t, escalation.PowerStandard, startPowerClass,
		"unflagged bead must infer standard powerClass")
	require.Equal(t, "standard", string(startPowerClass),
		"standard powerClass maps to the ordinary implementation profile")

	// 2. Transient infrastructure failure on standard powerClass (e.g. 502 from
	// the inference host). The policy is: do NOT escalate; defer with a
	// retry-after and try the same powerClass again. The model wasn't given
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
	// loop body defers (retry-after) instead of advancing the powerClass. The
	// powerClass on the next attempt is therefore unchanged.
	powerClassAfterTransient := startPowerClass
	require.Equal(t, escalation.PowerStandard, powerClassAfterTransient,
		"transient failure must not advance the powerClass ladder")

	// 3. Substantive failure on standard powerClass (e.g. tests failed). Policy:
	// escalate to the next powerClass (smart).
	substantiveStatus := ExecuteBeadStatusExecutionFailed
	substantiveDetail := "TestFoo failed: assertion mismatch"
	require.False(t,
		escalation.IsInfrastructureFailure(substantiveStatus, substantiveDetail),
		"a model-capability failure detail must not match infrastructure patterns")
	require.True(t,
		escalation.ShouldEscalate(substantiveStatus),
		"substantive execution_failed must escalate")
	powerClassAfterStandardSubstantive := nextPowerClass(powerClassAfterTransient)
	require.Equal(t, escalation.PowerSmart, powerClassAfterStandardSubstantive,
		"standard -> smart escalation step")

	// 4. Smart powerClass succeeds. Build the escalation summary and verify
	// the full trail is recorded with winning_power_class=smart and the
	// transient attempt counted as deferred but not escalated.
	attempts := []escalation.PowerAttemptRecord{
		{PowerClass: string(escalation.PowerStandard), Harness: "codex", Model: "standard-model", Status: transientStatus, CostUSD: 0.0, DurationMS: 800},
		{PowerClass: string(escalation.PowerStandard), Harness: "codex", Model: "standard-model", Status: substantiveStatus, CostUSD: 0.15, DurationMS: 3400},
		{PowerClass: string(escalation.PowerSmart), Harness: "claude", Model: "smart-model", Status: escalation.SuccessStatus, CostUSD: 0.80, DurationMS: 9000},
	}
	summary := escalation.BuildEscalationSummary(attempts, string(escalation.PowerSmart))
	require.Equal(t, string(escalation.PowerSmart), summary.WinningPowerClass,
		"smart powerClass won the escalation")
	require.Len(t, summary.PowerAttempts, 3,
		"trail records all attempts including the deferred-transient standard attempt")
	assert.InDelta(t, 0.95, summary.TotalCostUSD, 1e-9)
	assert.InDelta(t, 0.15, summary.WastedCostUSD, 1e-9,
		"standard-substantive wasted; standard-transient billed $0; smart succeeded")
	assert.Equal(t, string(escalation.PowerStandard), summary.PowerAttempts[0].PowerClass,
		"first attempt was standard")
	assert.Equal(t, string(escalation.PowerSmart), summary.PowerAttempts[2].PowerClass,
		"final attempt was smart (top of ladder)")
}

// TestZeroConfigEscalationSummaryEventBody verifies the escalation summary
// is emitted as a kind:escalation-summary bead event when the standard-default
// ladder runs to completion. This is the operator-visible artifact of the
// retry/escalation policy.
func TestZeroConfigEscalationSummaryEventBody(t *testing.T) {
	store := bead.NewStore(t.TempDir())
	require.NoError(t, store.Init())
	target := &bead.Bead{ID: "ddx-zero-config-002", Title: "standard-default ladder", Priority: 0}
	require.NoError(t, store.Create(target))

	attempts := []escalation.PowerAttemptRecord{
		{PowerClass: string(escalation.PowerStandard), Status: ExecuteBeadStatusExecutionFailed, CostUSD: 0.10, DurationMS: 2000},
		{PowerClass: string(escalation.PowerSmart), Status: escalation.SuccessStatus, CostUSD: 0.50, DurationMS: 4000},
	}
	require.NoError(t,
		escalation.AppendEscalationSummaryEvent(
			store, target.ID, "test-worker", attempts, string(escalation.PowerSmart), time.Unix(1, 0).UTC()))

	events, err := store.Events(target.ID)
	require.NoError(t, err)
	var found *bead.BeadEvent
	for i := range events {
		if events[i].Kind == "escalation-summary" {
			found = &events[i]
			break
		}
	}
	require.NotNil(t, found, "escalation-summary event must be appended for the standard-default ladder run")
	assert.Contains(t, found.Summary, "winning_power_class=smart")
	assert.Contains(t, found.Summary, "attempts=2")
	assert.Contains(t, found.Body, `"power_class":"standard"`)
	assert.Contains(t, found.Body, `"power_class":"smart"`)
}

// nextPowerClass advances one rung up the cheap -> standard -> smart ladder.
// Mirrors the implicit ladder used by the work when escalating
// beyond a substantive failure. Smart is the top; further calls stay
// at smart (callers exhaust escalation when ShouldEscalate fires at smart).
func nextPowerClass(t escalation.PowerClass) escalation.PowerClass {
	switch t {
	case escalation.PowerCheap:
		return escalation.PowerStandard
	case escalation.PowerStandard:
		return escalation.PowerSmart
	default:
		return escalation.PowerSmart
	}
}
