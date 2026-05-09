package cmd

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	tierescalation "github.com/DocumentDrivenDX/ddx/internal/agent/escalation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryPolicy_ReviewFixableGapRaisesMinPowerAndPreservesPins proves that
// reviewFixableGapRepairArgs:
//   - Raises MinPower strictly above the implementer's actual power by
//     consulting the escalation ladder.
//   - Preserves explicit harness/model/provider/profile pins unchanged.
//   - Leaves MaxPower unset (0) when the operator did not set it, so the
//     repair attempt does not acquire a power cap the operator never requested.
func TestRetryPolicy_ReviewFixableGapRaisesMinPowerAndPreservesPins(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{{next: 90}}}

	repairCtx := &agent.RepairContextFromReviewGroup{
		ImplementerActualPower: 70,
		ResultRev:              "abc123",
		ReviewRationale:        "error handling missing",
	}

	args := reviewFixableGapRepairArgs(
		repairCtx,
		"claude",          // harness
		"claude-opus-4-6", // model
		"anthropic",       // provider
		"",                // profile (unset)
		0,                 // maxPower (operator did not set)
		ladder,
	)

	assert.Greater(t, args.MinPower, repairCtx.ImplementerActualPower,
		"repair MinPower must exceed implementer actual power")
	assert.Equal(t, 90, args.MinPower, "repair MinPower must match ladder's next floor")
	assert.Equal(t, "claude", args.Harness, "harness pin must be preserved")
	assert.Equal(t, "claude-opus-4-6", args.Model, "model pin must be preserved")
	assert.Equal(t, "anthropic", args.Provider, "provider pin must be preserved")
	assert.Equal(t, "", args.Profile, "profile pin must be preserved")
	assert.Equal(t, 0, args.MaxPower,
		"MaxPower must remain unset when the operator did not set it")

	assert.Equal(t, []int{70}, ladder.CalledWith(),
		"ladder must be consulted with implementer actual power")
}

func TestRepairRouting_FixableGapCanRaiseMinPower(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{{next: 90}}}
	repairCtx := &agent.RepairContextFromReviewGroup{
		ImplementerActualPower: 70,
		ResultRev:              "abc123",
		ReviewRationale:        "missing regression test",
	}

	args := reviewFixableGapRepairArgs(
		repairCtx,
		"claude",
		"claude-opus-4-6",
		"anthropic",
		"smart",
		100,
		ladder,
	)

	assert.Equal(t, 90, args.MinPower)
	assert.Greater(t, args.MinPower, repairCtx.ImplementerActualPower)
	assert.Equal(t, "claude", args.Harness)
	assert.Equal(t, "claude-opus-4-6", args.Model)
	assert.Equal(t, "anthropic", args.Provider)
	assert.Equal(t, "smart", args.Profile)
	assert.Equal(t, 100, args.MaxPower)
}

// TestRetryPolicy_ReviewFixableGapRaisesMinPower_LadderExhausted verifies
// that when the escalation ladder has no higher floor, reviewFixableGapRepairMinPower
// returns the implementer's actual power rather than zero so the repair attempt
// still carries the review rationale as context.
func TestRetryPolicy_ReviewFixableGapRaisesMinPower_LadderExhausted(t *testing.T) {
	ladder := &ladderSpy{responses: []ladderResponse{
		{err: tierescalation.ErrLadderExhausted},
	}}

	repairCtx := &agent.RepairContextFromReviewGroup{
		ImplementerActualPower: 90,
	}

	minPower := reviewFixableGapRepairMinPower(repairCtx, ladder)
	require.Equal(t, 90, minPower,
		"exhausted ladder must fall back to implementer actual power")
}
