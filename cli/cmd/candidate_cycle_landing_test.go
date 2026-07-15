package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSinglePolicyAttempt_ReviewMalfunctionDoesNotTriggerLand(t *testing.T) {
	assertCandidateLandingCallSiteUsesSafetyGuard(t, "execute_loop_shared.go")
	assertReviewMalfunctionDoesNotTriggerLand(t)
}

func TestTrySinglePolicyAttempt_ReviewMalfunctionDoesNotTriggerLand(t *testing.T) {
	assertCandidateLandingCallSiteUsesSafetyGuard(t, "try.go")
	assertReviewMalfunctionDoesNotTriggerLand(t)
}

func TestPrepareCandidateCycleLanding_SuccessStillTriggersLand(t *testing.T) {
	res := &agent.ExecuteBeadResult{
		BaseRev:   "base-rev",
		ResultRev: "candidate-rev",
		ExitCode:  0,
		Outcome:   agent.ExecuteBeadOutcomeTaskSucceeded,
		Status:    agent.ExecuteBeadStatusSuccess,
	}

	assert.True(t, prepareCandidateCycleLanding(res))
	assert.Equal(t, agent.ExecuteBeadOutcomeTaskSucceeded, res.Outcome)
}

func assertReviewMalfunctionDoesNotTriggerLand(t *testing.T) {
	t.Helper()

	res := &agent.ExecuteBeadResult{
		BaseRev:   "base-rev",
		ResultRev: "candidate-rev",
		ExitCode:  0,
		Outcome:   agent.ExecuteBeadOutcomeTaskSucceeded,
		Status:    agent.ExecuteBeadStatusReviewMalfunction,
	}
	landCalled := prepareCandidateCycleLanding(res)

	assert.False(t, landCalled, "review malfunction must not enter the landing call site")
	assert.Equal(t, "preserved", res.Outcome)
	assert.Equal(t, agent.ExecuteBeadStatusReviewMalfunction, res.Status,
		"preserving the candidate must not erase the review classification")
}

func assertCandidateLandingCallSiteUsesSafetyGuard(t *testing.T, file string) {
	t.Helper()

	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	source := string(raw)
	guard := "if prepareCandidateCycleLanding(res) {"
	oldGate := `res.ResultRev != "" && res.ResultRev != res.BaseRev && res.ExitCode == 0`

	guardIndex := strings.Index(source, guard)
	require.NotEqual(t, -1, guardIndex, "%s must use the candidate-cycle landing guard", file)
	submitIndex := strings.Index(source[guardIndex:], "agent.SubmitWithPreMergeChecks(")
	require.NotEqual(t, -1, submitIndex, "%s guard must control SubmitWithPreMergeChecks", file)
	assert.NotContains(t, source, oldGate, "%s must not restore the ResultRev/ExitCode-only landing gate", file)
}
