package bead

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func allLifecycleStatusesForTests() []string {
	return append([]string(nil), CanonicalStatuses...)
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
