package agent

import (
	"context"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadinessClassification_DoesNotMisclassifyInfrastructureAsBeadDefect(t *testing.T) {
	cases := []struct {
		name           string
		classification string
		detail         string
		wantSystem     string
		wantTriage     string
	}{
		{
			name:       "missing_harness",
			detail:     "lint hook: missing-harness: no harness configured",
			wantSystem: ReadinessSystemReasonRouting,
			wantTriage: "routing",
		},
		{
			name:       "provider_quota",
			detail:     "provider returned 429 Too Many Requests: quota exceeded",
			wantSystem: ReadinessSystemReasonQuota,
			wantTriage: "quota",
		},
		{
			name:       "transport",
			detail:     "transport failed: connection reset by peer",
			wantSystem: ReadinessSystemReasonTransport,
			wantTriage: "transport",
		},
		{
			name:       "enospc",
			detail:     "resource preflight failed: ENOSPC: no space left on device",
			wantSystem: ReadinessSystemReasonResourceExhausted,
			wantTriage: "recoverable",
		},
		{
			name:       "worktree_setup",
			detail:     "worktree setup failed: no space left on device",
			wantSystem: ReadinessSystemReasonResourceExhausted,
			wantTriage: "recoverable",
		},
		{
			name:       "evidence_write",
			detail:     "evidence bundle write failed: permission denied",
			wantSystem: ReadinessSystemReasonResourceExhausted,
			wantTriage: "recoverable",
		},
		{
			name:       "git_index_lock",
			detail:     "fatal: Unable to create '/repo/.git/index.lock': File exists",
			wantSystem: ReadinessSystemReasonRepoConcurrency,
			wantTriage: "recoverable",
		},
		{
			name:           "readiness_runner_system_unready",
			classification: ReadinessClassificationSystemUnready,
			detail:         "readiness runner exited with empty output",
			wantSystem:     ReadinessSystemReasonUnavailable,
			wantTriage:     "recoverable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyReadiness(tc.classification, nil, tc.detail)
			require.Equal(t, ReadinessClassificationSystemUnready, got.Classification)
			assert.Equal(t, ReadinessReasonSystemUnready, got.Reason)
			assert.Equal(t, tc.wantSystem, got.SystemReason)
			assert.Equal(t, tc.wantTriage, got.TriageClassification)
			assert.Equal(t, PreClaimIntakeError, got.IntakeOutcome)
			assert.NotEqual(t, ReadinessClassificationNeedsRefine, got.Classification)
			assert.NotEqual(t, ReadinessClassificationNeedsSplit, got.Classification)
		})
	}
}

func TestReadinessClassification_BeadDefectsUseReadinessReasons(t *testing.T) {
	cases := []struct {
		name               string
		reasons            []string
		wantClassification string
		wantReason         string
		wantOutcome        PreClaimIntakeOutcome
	}{
		{
			name:               "too_large",
			reasons:            []string{ReadinessReasonTooLarge},
			wantClassification: ReadinessClassificationNeedsSplit,
			wantReason:         ReadinessReasonTooLarge,
			wantOutcome:        PreClaimIntakeTooLargeDecomposed,
		},
		{
			name:               "ambiguous_scope",
			reasons:            []string{ReadinessReasonAmbiguousScope},
			wantClassification: ReadinessClassificationOperatorRequired,
			wantReason:         ReadinessReasonAmbiguousScope,
			wantOutcome:        PreClaimIntakeOperatorRequired,
		},
		{
			name:               "missing_verification",
			reasons:            []string{ReadinessReasonMissingVerification},
			wantClassification: ReadinessClassificationNeedsRefine,
			wantReason:         ReadinessReasonMissingVerification,
			wantOutcome:        PreClaimIntakeActionableAtomic,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyReadiness("", tc.reasons, "bead body failed readiness rubric")
			assert.Equal(t, tc.wantClassification, got.Classification)
			assert.Equal(t, tc.wantReason, got.Reason)
			assert.Equal(t, tc.wantOutcome, got.IntakeOutcome)
			assert.Empty(t, got.SystemReason)
			assert.NotEqual(t, ReadinessClassificationSystemUnready, got.Classification)
		})
	}
}

func TestReadinessClassification_DecodesReadinessSchema(t *testing.T) {
	system, err := decodePreClaimIntakePayloadResult(`{"classification":"system_unready","rationale":"ResolveRoute: no viable routing candidate"}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeError, system.Outcome)
	assert.Equal(t, ReadinessReasonSystemUnready, system.Reason)
	assert.Equal(t, ReadinessSystemReasonRouting, system.SystemReason)

	split, err := decodePreClaimIntakePayloadResult(`{"classification":"needs_split","rationale":"too broad","readiness_checks":[{"reason":"too_large","verdict":"fail","evidence":"AC spans three subsystems"}]}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeTooLargeDecomposed, split.Outcome)
	assert.Equal(t, ReadinessReasonTooLarge, split.Reason)
	assert.Contains(t, split.Detail, ReadinessReasonTooLarge)

	refine, err := decodePreClaimIntakePayloadResult(`{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":[{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}]}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, refine.Outcome)
	assert.Equal(t, ReadinessReasonMissingVerification, refine.Reason)
	assert.Empty(t, refine.SystemReason)
}

func TestReadinessClassification_NeedsRefineBlocksInBlockMode(t *testing.T) {
	got := ClassifyReadinessWithMode(
		ReadinessClassificationNeedsRefine,
		[]string{ReadinessReasonMissingVerification},
		"bead body failed readiness rubric",
		config.BeadQualityModeBlock,
	)
	assert.Equal(t, ReadinessClassificationNeedsRefine, got.Classification)
	assert.Equal(t, ReadinessReasonMissingVerification, got.Reason)
	assert.Equal(t, PreClaimIntakeOperatorRequired, got.IntakeOutcome)
}

func TestReadinessClassification_DeterministicSystemReasonBypassesModelTriage(t *testing.T) {
	worker := &ExecuteBeadWorker{}
	report := ExecuteBeadReport{
		BeadID:        "ddx-system",
		Status:        ExecuteBeadStatusNoChanges,
		OutcomeReason: "quota",
		BaseRev:       "same",
		ResultRev:     "same",
	}

	called := false
	got := worker.runPostAttemptTriage(context.Background(), bead.Bead{ID: "ddx-system"}, report, ExecuteBeadLoopRuntime{
		PostAttemptTriageHook: func(ctx context.Context, beadID string, report ExecuteBeadReport) (TriageResult, error) {
			called = true
			return TriageResult{Classification: "needs_investigation", RecommendedAction: "release_claim_needs_investigation"}, nil
		},
	}, "worker", time.Now)

	assert.False(t, called, "deterministic system reasons must not be overwritten by model triage")
	assert.Equal(t, "quota", got.OutcomeReason)
}
