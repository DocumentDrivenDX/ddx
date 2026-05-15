package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
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

func TestReadinessClassification_DecodesEstimatedDifficulty(t *testing.T) {
	canonical, err := decodePreClaimIntakePayloadResult(`{"outcome":"actionable_atomic","reason":"ready","difficulty":{"estimated_difficulty":"easy","confidence":0.8,"reason":"mechanical docs edit"}}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, canonical.Outcome)
	assert.Equal(t, "easy", canonical.EstimatedDifficulty)
}

func TestReadinessClassification_LegacyDecodesEstimatedDifficulty(t *testing.T) {
	got, err := decodePreClaimIntakePayloadResult(`{"classification":"ready","rationale":"ready","difficulty":{"estimated_difficulty":"hard","confidence":0.74,"reason":"multi-subsystem risk"},"readiness_checks":[]}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "hard", got.EstimatedDifficulty)
}

func TestReadinessUsesBeadDifficultyPrecedence(t *testing.T) {
	b := &bead.Bead{
		Extra: map[string]any{
			escalation.BeadEstimatedDifficultyKey: string(escalation.DifficultyEasy),
		},
	}

	got := resolveReadinessEstimatedDifficulty(b, string(escalation.DifficultyHard))
	assert.Equal(t, escalation.DifficultyEasy, got)
	assert.Equal(t, escalation.DifficultyMedium, resolveReadinessEstimatedDifficulty(&bead.Bead{}, "bogus"))
}

func TestPreClaimReadiness_AcceptsStringSuggestedFixes(t *testing.T) {
	got, err := decodePreClaimIntakePayloadResult(`{"classification":"needs_refine","rationale":"prompt polish only","readiness_checks":[{"reason":"missing_verification","verdict":"pass","evidence":"AC names tests"}],"suggested_fixes":["tighten title","add file:line evidence"],"waivers_applied":[]}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Empty(t, got.SystemReason)
	assert.Contains(t, got.Detail, "prompt polish only")
}

func TestPreClaimReadiness_DecodesWaiversAppliedStringList(t *testing.T) {
	payload := `{"classification":"ready","tractability":"tractable","score":0.86,"rationale":"single slice","readiness_checks":[],"waivers_applied":["none"]}`

	var out preClaimReadinessClassificationPromptResult
	require.NoError(t, json.Unmarshal([]byte(payload), &out))
	require.Len(t, out.WaiversApplied, 1)
	assert.Equal(t, "none", out.WaiversApplied[0].Reason)
	assert.Empty(t, out.WaiversApplied[0].Criteria)
	assert.Empty(t, out.WaiversApplied[0].Evidence)

	got, err := decodePreClaimIntakePayloadResult(payload)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "single slice", got.Detail)
}

func TestPreClaimReadiness_DecodesWaiversAppliedObjectList(t *testing.T) {
	payload := `{"classification":"ready","tractability":"tractable","score":0.86,"rationale":"single slice","readiness_checks":[],"waivers_applied":[{"reason":"doc-only","criteria":["docs-only"],"evidence":"docs-only bead"}]}`

	var out preClaimReadinessClassificationPromptResult
	require.NoError(t, json.Unmarshal([]byte(payload), &out))
	require.Len(t, out.WaiversApplied, 1)
	assert.Equal(t, "doc-only", out.WaiversApplied[0].Reason)
	assert.Equal(t, []string{"docs-only"}, out.WaiversApplied[0].Criteria)
	assert.Equal(t, "docs-only bead", out.WaiversApplied[0].Evidence)

	got, err := decodePreClaimIntakePayloadResult(payload)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "single slice", got.Detail)
}

func TestPreClaimReadiness_DecodesSuggestedChildAcceptanceString(t *testing.T) {
	payload := `{"classification":"ready","tractability":"tractable","score":0.86,"rationale":"single slice","readiness_checks":[],"suggested_child_beads":[{"title":"Split docs","acceptance":"1. TestFoo passes\n2. cd cli && go test ./internal/agent/... green"}]}`

	var out preClaimReadinessClassificationPromptResult
	require.NoError(t, json.Unmarshal([]byte(payload), &out))
	require.Len(t, out.SuggestedChildren, 1)
	assert.Equal(t, []string{
		"1. TestFoo passes",
		"2. cd cli && go test ./internal/agent/... green",
	}, out.SuggestedChildren[0].Acceptance)

	got, err := decodePreClaimIntakePayloadResult(payload)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "single slice", got.Detail)
}

func TestPreClaimReadiness_DecodesSuggestedChildAcceptanceList(t *testing.T) {
	payload := `{"classification":"ready","tractability":"tractable","score":0.86,"rationale":"single slice","readiness_checks":[],"suggested_child_beads":[{"title":"Split docs","acceptance":["1. TestFoo passes","2. cd cli && go test ./internal/agent/... green"]}]}`

	var out preClaimReadinessClassificationPromptResult
	require.NoError(t, json.Unmarshal([]byte(payload), &out))
	require.Len(t, out.SuggestedChildren, 1)
	assert.Equal(t, []string{
		"1. TestFoo passes",
		"2. cd cli && go test ./internal/agent/... green",
	}, out.SuggestedChildren[0].Acceptance)

	got, err := decodePreClaimIntakePayloadResult(payload)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, "single slice", got.Detail)
}

func TestPreClaimReadiness_NormalizesSingletonReadinessChecksObject(t *testing.T) {
	got, err := decodePreClaimIntakePayloadResult(`{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":{"reason":"missing_verification","verdict":"fail","evidence":"AC lacks go test"}}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeActionableAtomic, got.Outcome)
	assert.Equal(t, ReadinessReasonMissingVerification, got.Reason)
	assert.Contains(t, got.Detail, "verification is absent")
	assert.Contains(t, got.Detail, ReadinessReasonMissingVerification)
	assert.Empty(t, got.SystemReason)
}

func TestPreClaimReadiness_NormalizesScalarReadinessChecksString(t *testing.T) {
	got, err := decodePreClaimIntakePayloadResult(`{"classification":"needs_refine","rationale":"verification is absent","readiness_checks":"missing_verification"}`)
	require.NoError(t, err)
	assert.Equal(t, PreClaimIntakeError, got.Outcome)
	assert.Equal(t, ReadinessReasonSystemUnready, got.Reason)
	assert.Equal(t, ReadinessSystemReasonUnavailable, got.SystemReason)
	assert.Contains(t, got.Detail, "readiness_checks")
	assert.Contains(t, got.Detail, "missing_verification")
	assert.NotContains(t, got.Detail, "Go struct field")
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
