package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecuteStatusAndReadinessProviderTablesRemovedOrAuditOnly proves that
// the provider/routing text lists in execute_bead_status.go,
// readiness_classification.go, and execute_bead_loop.go no longer control
// retry, repair, cooldown, park, land, close, or escalation outcomes once a
// report carries typed Fizeau lifecycle evidence (FizeauOutcome/Cause/Stage)
// or a typed provider-failure error. Free-text stderr/detail/error scraping
// remains solely as an audit-evidence fallback for reports that carry no
// typed lifecycle data at all (ddx-32b0c5b3, WB-1).
func TestExecuteStatusAndReadinessProviderTablesRemovedOrAuditOnly(t *testing.T) {
	t.Run("isNoViableProviderReport ignores conflicting text once typed lifecycle is present", func(t *testing.T) {
		typedReport := ExecuteBeadReport{
			Status:        ExecuteBeadStatusExecutionFailed,
			OutcomeReason: FailureModeAuthError,
			FizeauOutcome: "failed",
			FizeauCause:   "agent_gave_up",
			FizeauStage:   "execute",
			Detail:        "routing: no viable routing candidate: 3 candidates rejected",
			Error:         "no viable provider",
		}
		assert.False(t, isNoViableProviderReport(typedReport),
			"typed Fizeau lifecycle must own the outcome; stderr/detail text must not relitigate it")

		untypedReport := typedReport
		untypedReport.FizeauOutcome = ""
		untypedReport.FizeauCause = ""
		untypedReport.FizeauStage = ""
		untypedReport.OutcomeReason = ""
		assert.True(t, isNoViableProviderReport(untypedReport),
			"without any typed lifecycle evidence the text fallback still classifies legacy/incomplete reports")
	})

	t.Run("isProviderConnectivityFailureReport ignores conflicting text once typed lifecycle is present", func(t *testing.T) {
		typedReport := ExecuteBeadReport{
			Status:        ExecuteBeadStatusExecutionFailed,
			OutcomeReason: FailureModeBuildFailure,
			FizeauOutcome: "failed",
			FizeauCause:   "harness_failed",
			FizeauStage:   "dispatch",
			Error:         "Post \"http://opaque.invalid/v1\": dial tcp: connection refused",
		}
		assert.False(t, isProviderConnectivityFailureReport(typedReport),
			"typed Fizeau lifecycle must own the outcome; transport substrings must not relitigate it")

		untypedReport := typedReport
		untypedReport.FizeauOutcome = ""
		untypedReport.FizeauCause = ""
		untypedReport.FizeauStage = ""
		untypedReport.OutcomeReason = "execution_failed"
		assert.True(t, isProviderConnectivityFailureReport(untypedReport),
			"without any typed lifecycle evidence the text fallback still classifies legacy/incomplete reports")
	})

	t.Run("isRoutingInfrastructureReport ignores conflicting text once typed lifecycle is present", func(t *testing.T) {
		typedReport := ExecuteBeadReport{
			Status:        ExecuteBeadStatusExecutionFailed,
			OutcomeReason: FailureModeTestFailure,
			FizeauOutcome: "failed",
			FizeauCause:   "agent_gave_up",
			FizeauStage:   "execute",
			Detail:        "resolveroute: missing-harness for requested pin",
		}
		assert.False(t, isRoutingInfrastructureReport(typedReport),
			"typed Fizeau lifecycle must own the outcome; routing substrings must not relitigate it")

		untypedReport := typedReport
		untypedReport.FizeauOutcome = ""
		untypedReport.FizeauCause = ""
		untypedReport.FizeauStage = ""
		assert.True(t, isRoutingInfrastructureReport(untypedReport),
			"without any typed lifecycle evidence the text fallback still classifies legacy/incomplete reports")
	})

	t.Run("classifyLoopReportFailure does not stamp OutcomeReason/Disrupted from text once typed lifecycle is present", func(t *testing.T) {
		report := &ExecuteBeadReport{
			Status:        ExecuteBeadStatusExecutionFailed,
			FizeauOutcome: "failed",
			FizeauCause:   "agent_gave_up",
			FizeauStage:   "execute",
			Detail:        "dial tcp 1.2.3.4:80: i/o timeout; no viable routing candidate; rate limit exceeded",
			Error:         "resolveroute: no viable routing candidate",
			Stderr:        "429 rate_limit quota exceeded",
		}
		classifyLoopReportFailure(report)
		assert.Empty(t, report.OutcomeReason,
			"a report with typed Fizeau lifecycle must not have its outcome synthesized from stderr/detail/error text")
		assert.False(t, report.Disrupted,
			"a report with typed Fizeau lifecycle must not have Disrupted stamped from stderr/detail/error text")
	})

	t.Run("MarkResultExecutionError prefers the typed provider-failure wrapper over ClassifyFailureMode text tables", func(t *testing.T) {
		textErr := errors.New("routing: no viable routing candidate: 3 candidates rejected")
		typedErr := &ProviderFailureError{
			Failure: ProviderFailure{Reason: FailureModeProviderAuth, Retryable: true, Disruption: FailureModeProviderAuth},
			Err:     textErr,
		}
		res := &ExecuteBeadResult{}
		MarkResultExecutionError(res, typedErr)
		assert.Equal(t, FailureModeProviderAuth, res.FailureMode,
			"typed provider-failure classification must win over the text-derived no_viable_provider guess")

		untypedRes := &ExecuteBeadResult{}
		MarkResultExecutionError(untypedRes, textErr)
		assert.Equal(t, FailureModeNoViableProvider, untypedRes.FailureMode,
			"without a typed provider-failure wrapper the text fallback still classifies legacy/incomplete errors")
	})
}
