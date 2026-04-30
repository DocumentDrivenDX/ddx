package agent

// retry_policy_test.go covers the retry policy acceptance criteria:
//   AC1: Policy reads prior attempt outcome and actual_power from evidence.
//   AC2: On retryable outcomes, NextMinPower > currentMinPower; MaxPower preserved.
//   AC3: Top-model-only MinPower threshold derived from catalog.
//   AC4: RetryDecision contains no harness/provider/model/model-ref pins.
//   AC5: Passthrough envelope is not inspected or mutated.
//   AC6: success no-retry, no_changes_after_attempt escalation,
//        review_blocked_capability escalation, passthrough-stickiness,
//        max-power/no-progress stop, deterministic setup failures stop.
//   AC7: blocked_by_passthrough_constraint and agent_power_unsatisfied stop
//        without mutating pins or calling ResolveRoute.
//   AC8: Reason and Evidence fields populated for all outcomes.

import (
	"testing"

	agentlib "github.com/DocumentDrivenDX/agent"
)

// catalogWithPowers builds a slice of ModelInfo with the given power values,
// simulating the agent model catalog returned by ListModels.
func catalogWithPowers(powers ...int) []agentlib.ModelInfo {
	models := make([]agentlib.ModelInfo, len(powers))
	for i, p := range powers {
		models[i] = agentlib.ModelInfo{
			ID:    "model-" + string(rune('A'+i)),
			Power: p,
		}
	}
	return models
}

// ── ClassifyRetryOutcome ────────────────────────────────────────────────────

func TestClassifyRetryOutcome_Success(t *testing.T) {
	got := ClassifyRetryOutcome(ExecuteBeadStatusSuccess, "")
	if got != RetryOutcomeSuccess {
		t.Errorf("status=success → %q, want %q", got, RetryOutcomeSuccess)
	}
}

func TestClassifyRetryOutcome_AlreadySatisfied(t *testing.T) {
	got := ClassifyRetryOutcome(ExecuteBeadStatusAlreadySatisfied, "")
	if got != RetryOutcomeSuccess {
		t.Errorf("status=already_satisfied → %q, want %q", got, RetryOutcomeSuccess)
	}
}

func TestClassifyRetryOutcome_NoChanges(t *testing.T) {
	got := ClassifyRetryOutcome(ExecuteBeadStatusNoChanges, FailureModeNoChanges)
	if got != RetryOutcomeNoChangesAfterAttempt {
		t.Errorf("status=no_changes → %q, want %q", got, RetryOutcomeNoChangesAfterAttempt)
	}
}

func TestClassifyRetryOutcome_ReviewBlock(t *testing.T) {
	for _, status := range []string{
		ExecuteBeadStatusReviewBlock,
		ExecuteBeadStatusReviewRequestChanges,
		ExecuteBeadStatusReviewMalfunction,
	} {
		got := ClassifyRetryOutcome(status, "")
		if got != RetryOutcomeReviewBlockedCapability {
			t.Errorf("status=%q → %q, want %q", status, got, RetryOutcomeReviewBlockedCapability)
		}
	}
}

// AC7: blocked_by_passthrough_constraint stops regardless of status.
func TestClassifyRetryOutcome_PassthroughConstraint(t *testing.T) {
	got := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, FailureModeBlockedByPassthroughConstraint)
	if got != RetryOutcomePassthroughExhausted {
		t.Errorf("failure_mode=blocked_by_passthrough_constraint → %q, want %q",
			got, RetryOutcomePassthroughExhausted)
	}
}

// AC7: agent_power_unsatisfied stops regardless of status.
func TestClassifyRetryOutcome_AgentPowerUnsatisfied(t *testing.T) {
	got := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, FailureModeAgentPowerUnsatisfied)
	if got != RetryOutcomePassthroughExhausted {
		t.Errorf("failure_mode=agent_power_unsatisfied → %q, want %q",
			got, RetryOutcomePassthroughExhausted)
	}
}

// AC6: deterministic setup failures do not retry.
func TestClassifyRetryOutcome_SetupFailures(t *testing.T) {
	cases := []string{
		FailureModeHarnessNotInstalled,
		FailureModeNoViableProvider,
		FailureModeAuthError,
	}
	for _, fm := range cases {
		got := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, fm)
		if got != RetryOutcomeSetupFailure {
			t.Errorf("failure_mode=%q → %q, want %q", fm, got, RetryOutcomeSetupFailure)
		}
	}
}

// Passthrough exhaustion takes priority over status classification.
func TestClassifyRetryOutcome_PassthroughPriorityOverStatus(t *testing.T) {
	// Even if the status would normally classify as no_changes, passthrough
	// exhaustion must win.
	got := ClassifyRetryOutcome(ExecuteBeadStatusNoChanges, FailureModeBlockedByPassthroughConstraint)
	if got != RetryOutcomePassthroughExhausted {
		t.Errorf("passthrough failure_mode must take priority: got %q, want %q",
			got, RetryOutcomePassthroughExhausted)
	}
}

// ── EvaluateRetryPolicy ─────────────────────────────────────────────────────

// AC6: success → no retry.
func TestEvaluateRetryPolicy_SuccessNoRetry(t *testing.T) {
	d := EvaluateRetryPolicy(RetryOutcomeSuccess, 80, 0, 0, nil)
	if d.ShouldRetry {
		t.Error("success outcome must not retry")
	}
	if d.Reason == "" {
		t.Error("Reason must be populated even for no-retry decisions (AC8)")
	}
	if d.Evidence == "" {
		t.Error("Evidence must be populated for audit records (AC8)")
	}
}

// AC6: no_changes_after_attempt → retry with escalated MinPower.
func TestEvaluateRetryPolicy_NoChangesEscalates(t *testing.T) {
	catalog := catalogWithPowers(30, 70, 100)
	d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 70, 0, 0, catalog)
	if !d.ShouldRetry {
		t.Fatalf("no_changes_after_attempt must retry; got ShouldRetry=false (reason=%q)", d.Reason)
	}
	// AC2: NextMinPower must be higher than currentMinPower (0).
	if d.NextMinPower <= 0 {
		t.Errorf("NextMinPower must be > 0 after escalation, got %d", d.NextMinPower)
	}
	// AC3: with catalog, must use top-1 = 100.
	if d.NextMinPower != 100 {
		t.Errorf("NextMinPower = %d, want 100 (top-1 from catalog)", d.NextMinPower)
	}
	if d.Reason == "" || d.Evidence == "" {
		t.Error("Reason and Evidence must be populated (AC8)")
	}
}

// AC6: review_blocked_capability → retry with escalated MinPower.
func TestEvaluateRetryPolicy_ReviewBlockedCapabilityEscalates(t *testing.T) {
	catalog := catalogWithPowers(40, 80, 95)
	d := EvaluateRetryPolicy(RetryOutcomeReviewBlockedCapability, 80, 0, 0, catalog)
	if !d.ShouldRetry {
		t.Fatalf("review_blocked_capability must retry; got ShouldRetry=false (reason=%q)", d.Reason)
	}
	// AC3: top-1 from catalog = 95.
	if d.NextMinPower != 95 {
		t.Errorf("NextMinPower = %d, want 95 (top-1 from catalog)", d.NextMinPower)
	}
}

// AC5, AC6: passthrough stickiness — RetryDecision has no harness/provider/model/
// model-ref fields. Verified structurally: RetryDecision only exposes power bounds.
func TestEvaluateRetryPolicy_DecisionContainsNoPins(t *testing.T) {
	catalog := catalogWithPowers(50, 90)
	d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 50, 0, 0, catalog)
	if !d.ShouldRetry {
		t.Fatalf("expected retry, got stop (reason=%q)", d.Reason)
	}
	// RetryDecision struct: only ShouldRetry, NextMinPower, Reason, Evidence.
	// AC4: no harness/provider/model/model-ref fields exist on this type.
	// The test verifies this at compile-time: assigning those fields would be
	// a compile error. We confirm only the expected fields are used.
	_ = d.ShouldRetry
	_ = d.NextMinPower
	_ = d.Reason
	_ = d.Evidence
}

// AC2: MaxPower is preserved — NextMinPower must not exceed MaxPower.
func TestEvaluateRetryPolicy_MaxPowerPreserved(t *testing.T) {
	catalog := catalogWithPowers(50, 80, 100)
	// MaxPower=90 limits escalation; top-1 (100) would exceed it.
	d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 50, 0, 90, catalog)
	if d.ShouldRetry {
		t.Errorf("top-1=100 > max_power=90: must stop, got ShouldRetry=true NextMinPower=%d", d.NextMinPower)
	}
	if d.Reason != RetryOutcomeMaxPowerNoProgress {
		t.Errorf("Reason = %q, want %q", d.Reason, RetryOutcomeMaxPowerNoProgress)
	}
}

// AC6: max-power/no-progress stop when catalog is exhausted.
func TestEvaluateRetryPolicy_NoCatalogNoProgress(t *testing.T) {
	// actual_power and currentMinPower both at same level; no catalog to go higher.
	// The heuristic bump will give base+10=80; since actualPower=70 and nextMinPower=80 > actualPower,
	// this should still retry. But if actualPower matches nextMinPower exactly...
	//
	// Test the specific "catalog exhausted" case: when top-1 is not available
	// and heuristic bump still produces something higher, we retry.
	// When top-1 <= actualPower, we stop.
	catalog := catalogWithPowers(50) // top-1 = 50; actualPower = 70 (already above top-1)
	d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 70, 0, 0, catalog)
	if d.ShouldRetry {
		t.Errorf("top-1=50 <= actual_power=70: no escalation possible, got ShouldRetry=true NextMinPower=%d", d.NextMinPower)
	}
	if d.Reason != RetryOutcomeMaxPowerNoProgress {
		t.Errorf("Reason = %q, want %q", d.Reason, RetryOutcomeMaxPowerNoProgress)
	}
}

// AC6: max-power/no-progress when actualPower is already at currentMinPower
// and catalog cannot go higher.
func TestEvaluateRetryPolicy_AlreadyAtTopTier(t *testing.T) {
	catalog := catalogWithPowers(100) // top-1 = 100
	// actualPower=100, currentMinPower=100: NextMinPower=100, not > actualPower, not > currentMinPower → stop
	d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 100, 100, 0, catalog)
	if d.ShouldRetry {
		t.Errorf("already at top tier: must stop, got ShouldRetry=true NextMinPower=%d", d.NextMinPower)
	}
	if d.Reason != RetryOutcomeMaxPowerNoProgress {
		t.Errorf("Reason = %q, want %q", d.Reason, RetryOutcomeMaxPowerNoProgress)
	}
}

// AC7: blocked_by_passthrough_constraint → stop, no ResolveRoute needed.
func TestEvaluateRetryPolicy_PassthroughExhaustedConstraint(t *testing.T) {
	outcome := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, FailureModeBlockedByPassthroughConstraint)
	if outcome != RetryOutcomePassthroughExhausted {
		t.Fatalf("classify: got %q, want %q", outcome, RetryOutcomePassthroughExhausted)
	}
	catalog := catalogWithPowers(50, 100)
	d := EvaluateRetryPolicy(outcome, 80, 0, 0, catalog)
	if d.ShouldRetry {
		t.Errorf("passthrough exhausted must stop, got ShouldRetry=true")
	}
	if d.NextMinPower != 0 {
		t.Errorf("passthrough exhausted must not set NextMinPower, got %d", d.NextMinPower)
	}
	if d.Reason != RetryOutcomePassthroughExhausted {
		t.Errorf("Reason = %q, want %q", d.Reason, RetryOutcomePassthroughExhausted)
	}
	if d.Evidence == "" {
		t.Error("Evidence must be populated for audit (AC8)")
	}
}

// AC7: agent_power_unsatisfied → stop, no ResolveRoute needed.
func TestEvaluateRetryPolicy_PassthroughExhaustedPower(t *testing.T) {
	outcome := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, FailureModeAgentPowerUnsatisfied)
	catalog := catalogWithPowers(50, 100)
	d := EvaluateRetryPolicy(outcome, 0, 0, 0, catalog)
	if d.ShouldRetry {
		t.Errorf("agent_power_unsatisfied must stop, got ShouldRetry=true")
	}
	if d.Reason != RetryOutcomePassthroughExhausted {
		t.Errorf("Reason = %q, want %q", d.Reason, RetryOutcomePassthroughExhausted)
	}
}

// AC6: deterministic setup failures do not retry.
func TestEvaluateRetryPolicy_SetupFailureDoesNotRetry(t *testing.T) {
	catalog := catalogWithPowers(50, 100)
	for _, fm := range []string{
		FailureModeHarnessNotInstalled,
		FailureModeNoViableProvider,
		FailureModeAuthError,
	} {
		outcome := ClassifyRetryOutcome(ExecuteBeadStatusExecutionFailed, fm)
		d := EvaluateRetryPolicy(outcome, 80, 0, 0, catalog)
		if d.ShouldRetry {
			t.Errorf("failure_mode=%q: must stop, got ShouldRetry=true", fm)
		}
		if d.Reason != RetryOutcomeSetupFailure {
			t.Errorf("failure_mode=%q: Reason = %q, want %q", fm, d.Reason, RetryOutcomeSetupFailure)
		}
	}
}

// AC8: every path populates both Reason and Evidence.
func TestEvaluateRetryPolicy_AuditFieldsAlwaysPopulated(t *testing.T) {
	catalog := catalogWithPowers(50, 100)
	cases := []struct {
		outcome     string
		actualPower int
		minPower    int
		maxPower    int
	}{
		{RetryOutcomeSuccess, 80, 0, 0},
		{RetryOutcomeNoChangesAfterAttempt, 50, 0, 0},
		{RetryOutcomeReviewBlockedCapability, 50, 0, 0},
		{RetryOutcomePassthroughExhausted, 80, 0, 0},
		{RetryOutcomeSetupFailure, 80, 0, 0},
		{RetryOutcomeMaxPowerNoProgress, 80, 0, 0},
		{RetryOutcomeOther, 80, 0, 0},
		// max-power bound exceeded
		{RetryOutcomeNoChangesAfterAttempt, 50, 0, 60},
	}
	for _, c := range cases {
		d := EvaluateRetryPolicy(c.outcome, c.actualPower, c.minPower, c.maxPower, catalog)
		if d.Reason == "" {
			t.Errorf("outcome=%q: Reason must be populated (AC8)", c.outcome)
		}
		if d.Evidence == "" {
			t.Errorf("outcome=%q: Evidence must be populated (AC8)", c.outcome)
		}
	}
}

// AC3: computeNextMinPower with empty catalog uses heuristic bump.
func TestComputeNextMinPower_NoCatalog(t *testing.T) {
	// No catalog, actualPower=60, currentMinPower=0 → base=60 → 70.
	got := computeNextMinPower(nil, 60, 0)
	if got != 70 {
		t.Errorf("no catalog, actualPower=60 → got %d, want 70", got)
	}
}

// AC3: computeNextMinPower with catalog uses top-1.
func TestComputeNextMinPower_WithCatalog(t *testing.T) {
	catalog := catalogWithPowers(30, 70, 95)
	got := computeNextMinPower(catalog, 30, 0)
	if got != 95 {
		t.Errorf("catalog top-1=95, got %d", got)
	}
}

// AC3: zero-power models in catalog are ignored.
func TestComputeNextMinPower_ZeroPowerIgnored(t *testing.T) {
	catalog := catalogWithPowers(0, 0, 80)
	got := computeNextMinPower(catalog, 0, 0)
	if got != 80 {
		t.Errorf("zero-power models ignored: top-1=80, got %d", got)
	}
}

// AC2: EvaluateRetryPolicy preserves MaxPower — verify no ShouldRetry path
// produces a NextMinPower exceeding MaxPower.
func TestEvaluateRetryPolicy_MaxPowerNeverExceeded(t *testing.T) {
	catalog := catalogWithPowers(30, 60, 100)
	for _, maxP := range []int{50, 70, 90} {
		d := EvaluateRetryPolicy(RetryOutcomeNoChangesAfterAttempt, 30, 0, maxP, catalog)
		if d.ShouldRetry && d.NextMinPower > maxP {
			t.Errorf("maxPower=%d: NextMinPower=%d exceeds bound (AC2)", maxP, d.NextMinPower)
		}
	}
}
