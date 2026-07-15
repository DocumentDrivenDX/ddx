package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCandidateCycleNonMergeable_CoversAllNonSuccessCycleStatuses(t *testing.T) {
	for _, status := range []string{
		ExecuteBeadStatusReviewMalfunction,
		ExecuteBeadStatusReviewRequestChanges,
		ExecuteBeadStatusReviewBlock,
		ExecuteBeadStatusRepairCycleExhausted,
		ExecuteBeadStatusReviewFixableGap,
	} {
		t.Run(status, func(t *testing.T) {
			assert.True(t, IsCandidateCycleNonMergeable(status))
		})
	}

	for _, status := range []string{
		ExecuteBeadStatusSuccess,
		ExecuteBeadStatusAlreadySatisfied,
	} {
		t.Run(status, func(t *testing.T) {
			assert.False(t, IsCandidateCycleNonMergeable(status))
		})
	}
}
