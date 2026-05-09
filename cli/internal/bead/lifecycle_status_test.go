package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allLifecycleStatusesForTests() []string {
	return append([]string(nil), CanonicalStatuses...)
}

func lifecycleTransitionOptionsForTests(status string) LifecycleTransitionOptions {
	switch status {
	case StatusOpen:
		return LifecycleTransitionOptions{ManualReopen: true, Reason: "test reopen", Source: "test"}
	case StatusClosed:
		return LifecycleTransitionOptions{ManualClose: true, Reason: "test close", Source: "test"}
	case StatusBlocked:
		return LifecycleTransitionOptions{ExternalBlockerReason: "test external blocker", Reason: "test block", Source: "test"}
	case StatusProposed:
		return LifecycleTransitionOptions{OperatorRequired: true, Reason: "test operator required", Source: "test"}
	default:
		return LifecycleTransitionOptions{Reason: "test transition", Source: "test"}
	}
}

func setLifecycleStatusForTest(t *testing.T, s Backend, id string, status string) error {
	t.Helper()
	setter, ok := s.(interface {
		SetLifecycleStatus(string, string, LifecycleTransitionOptions) error
	})
	require.True(t, ok, "backend must expose lifecycle transition API")
	return setter.SetLifecycleStatus(id, status, lifecycleTransitionOptionsForTests(status))
}

func TestChaosAllowedStatusesIncludesSixLifecycleValues(t *testing.T) {
	assert.ElementsMatch(t, []string{
		StatusOpen,
		StatusInProgress,
		StatusClosed,
		StatusBlocked,
		StatusProposed,
		StatusCancelled,
	}, allLifecycleStatusesForTests())
}
