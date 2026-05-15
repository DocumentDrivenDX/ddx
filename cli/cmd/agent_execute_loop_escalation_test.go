package cmd

import (
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/stretchr/testify/assert"
)

type testEscalationLadder struct {
	floors []int
}

func (l testEscalationLadder) Next(actualPower int) (int, error) {
	for _, floor := range l.floors {
		if floor > actualPower {
			return floor, nil
		}
	}
	return 0, errTestLadderExhausted{}
}

type errTestLadderExhausted struct{}

func (errTestLadderExhausted) Error() string { return "test ladder exhausted" }

func TestRecentProviderConnectivityMinPowerFromEventsUsesRoutedFailure(t *testing.T) {
	now := time.Date(2026, 5, 14, 8, 40, 0, 0, time.UTC)
	events := []bead.BeadEvent{
		{
			Kind:      "routing",
			Body:      `{"resolved_provider":"bragi","resolved_model":"qwen3.5-27b","actual_power":5}`,
			CreatedAt: now.Add(-2 * time.Minute),
		},
		{
			Kind:      "execute-bead",
			Summary:   "execution_failed",
			Body:      `agent: provider error: dial tcp 100.127.38.115:1234: i/o timeout`,
			CreatedAt: now.Add(-2 * time.Minute),
		},
	}

	floor, ok := recentProviderConnectivityMinPowerFromEvents(events, now, 0, 0, testEscalationLadder{floors: []int{5, 70, 90}})

	assert.True(t, ok)
	assert.Equal(t, 70, floor)
}

func TestRecentProviderConnectivityMinPowerFromEventsIgnoresExpiredEvidence(t *testing.T) {
	now := time.Date(2026, 5, 14, 8, 40, 0, 0, time.UTC)
	events := []bead.BeadEvent{
		{
			Kind:      "routing",
			Body:      `{"resolved_provider":"bragi","resolved_model":"qwen3.5-27b","actual_power":5}`,
			CreatedAt: now.Add(-20 * time.Minute),
		},
		{
			Kind:      "execute-bead",
			Summary:   "execution_failed",
			Body:      `agent: provider error: dial tcp 100.127.38.115:1234: i/o timeout`,
			CreatedAt: now.Add(-20 * time.Minute),
		},
	}

	floor, ok := recentProviderConnectivityMinPowerFromEvents(events, now, 0, 0, testEscalationLadder{floors: []int{5, 70, 90}})

	assert.False(t, ok)
	assert.Equal(t, 0, floor)
}

func TestInvestigationRetryIgnoresNumericBeadPowerHint(t *testing.T) {
	b := &bead.Bead{
		ID:    "ddx-ignore-numeric-hint",
		Title: "Ignore numeric retry floor",
		Extra: map[string]any{
			agent.TriagePowerHintKey: 90,
		},
	}

	floor, report, unavailable := investigationRetryInitialMinPower(b, 7, 8, nil)

	assert.False(t, unavailable)
	assert.Equal(t, 7, floor)
	assert.Equal(t, agent.ExecuteBeadReport{}, report)
}
