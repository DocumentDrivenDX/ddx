package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fixtures ---

// healthyState returns a fully viable harness state.
func healthyState() HarnessState {
	return HarnessState{
		Installed:     true,
		Reachable:     true,
		Authenticated: true,
		QuotaOK:       true,
		PolicyOK:      true,
		LastChecked:   time.Now(),
	}
}

// healthyLocalState returns a healthy state for a local/embedded harness.
func healthyLocalState() HarnessState {
	s := healthyState()
	return s
}

// stateFixtures returns a harness-state fixture matrix covering SD-015 test scenarios:
// - healthy local embedded (forge)
// - healthy cloud codex
// - degraded cloud claude
// - quota-blocked harness (gemini)
// - unreachable harness (opencode)
func stateFixtures() map[string]HarnessState {
	degradedClaude := healthyState()
	degradedClaude.Degraded = true

	quotaBlocked := healthyState()
	quotaBlocked.QuotaOK = false
	quotaBlocked.Quota = &QuotaInfo{PercentUsed: 97, LimitWindow: "5h"}

	unreachable := healthyState()
	unreachable.Reachable = false

	policyRestricted := healthyState()
	policyRestricted.PolicyOK = false

	return map[string]HarnessState{
		"forge":    healthyLocalState(),
		"codex":    healthyState(),
		"claude":   degradedClaude,
		"gemini":   quotaBlocked,
		"opencode": unreachable,
		"pi":       policyRestricted,
		"virtual":  healthyLocalState(),
	}
}

// newTestRunnerForRouting returns a minimal runner for routing tests (no real exec).
func newTestRunnerForRouting() *Runner {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath // all binaries "found"
	return r
}

// --- Candidate Planning and Rejection ---

// TestAgentRoutingRejectsHarnessWithoutSurfaceMapping verifies that a harness
// not in the state map is rejected as not installed.
func TestAgentRoutingRejectsHarnessWithoutSurfaceMapping(t *testing.T) {
	r := newTestRunnerForRouting()

	// Provide state only for forge — all others are absent from the map.
	states := map[string]HarnessState{
		"forge": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	// forge should be viable
	forgeFound := false
	for _, p := range plans {
		if p.Harness == "forge" {
			assert.True(t, p.Viable, "forge should be viable")
			forgeFound = true
		} else {
			assert.False(t, p.Viable, "harness %s not in state map should be not viable", p.Harness)
			assert.Equal(t, "not installed", p.RejectReason)
		}
	}
	assert.True(t, forgeFound, "forge should be in plans")
}

// TestAgentRoutingRejectsUnsupportedEffort verifies that harnesses without an
// effort flag are rejected when an effort level is requested.
func TestAgentRoutingRejectsUnsupportedEffort(t *testing.T) {
	r := newTestRunnerForRouting()

	// gemini has no EffortFlag — it should be rejected when effort is requested.
	states := map[string]HarnessState{
		"gemini": healthyState(),
		"codex":  healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Effort: "high"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "gemini":
			assert.False(t, p.Viable)
			assert.Contains(t, p.RejectReason, "effort")
		case "codex":
			assert.True(t, p.Viable)
		}
	}
}

// TestAgentRoutingRejectsPolicyRestrictedHarness verifies PolicyOK=false causes rejection.
func TestAgentRoutingRejectsPolicyRestrictedHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	restricted := healthyState()
	restricted.PolicyOK = false

	states := map[string]HarnessState{
		"codex": restricted,
		"forge": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "codex":
			assert.False(t, p.Viable)
			assert.Equal(t, "policy restricted", p.RejectReason)
		case "forge":
			assert.True(t, p.Viable)
		}
	}
}

// TestAgentRoutingRejectsQuotaBlockedHarness verifies QuotaOK=false causes rejection.
func TestAgentRoutingRejectsQuotaBlockedHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	quotaBlocked := healthyState()
	quotaBlocked.QuotaOK = false

	states := map[string]HarnessState{
		"claude": quotaBlocked,
		"codex":  healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "smart"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "claude":
			assert.False(t, p.Viable)
			assert.Equal(t, "quota exceeded", p.RejectReason)
		case "codex":
			assert.True(t, p.Viable)
		}
	}
}

// TestAgentRoutingRejectsUnreachableHarness verifies Reachable=false causes rejection.
func TestAgentRoutingRejectsUnreachableHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	unreachable := healthyState()
	unreachable.Reachable = false

	states := map[string]HarnessState{
		"codex": unreachable,
		"forge": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "codex":
			assert.False(t, p.Viable)
			assert.Equal(t, "not reachable", p.RejectReason)
		case "forge":
			assert.True(t, p.Viable)
		}
	}
}

// --- Ranking ---

// TestAgentRoutingCheapPrefersLowestCostHealthyCandidate verifies that the
// cheap profile ranks local/cheap harnesses above cloud harnesses.
func TestAgentRoutingCheapPrefersLowestCostHealthyCandidate(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"forge": healthyLocalState(),
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)
	ranked := RankCandidates("cheap", plans)

	require.NotEmpty(t, ranked)
	// The top viable candidate should be local (forge).
	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	assert.Equal(t, "local", best.CostClass, "cheap profile should prefer local harness")
}

// TestAgentRoutingFastPrefersFastestViableCandidate verifies that fast profile
// prefers local (lowest latency, no network) over cloud.
func TestAgentRoutingFastPrefersFastestViableCandidate(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"forge": healthyLocalState(),
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "fast"}, states)
	ranked := RankCandidates("fast", plans)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	// fast still prefers local (no network round-trip) when otherwise equal.
	assert.Equal(t, "local", best.CostClass)
}

// TestAgentRoutingSmartPrefersHighestQualityCandidate verifies that smart profile
// prefers cloud/expensive over local.
func TestAgentRoutingSmartPrefersHighestQualityCandidate(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"forge": healthyLocalState(),
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "smart"}, states)
	ranked := RankCandidates("smart", plans)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	// smart prefers cloud (medium/expensive) over local for quality.
	assert.NotEqual(t, "local", best.CostClass, "smart profile should prefer non-local (cloud) harness")
}

// TestAgentRoutingPrefersLocalWhenOtherwiseEquivalent verifies that when two
// candidates have equal scores, the local one wins.
func TestAgentRoutingPrefersLocalWhenOtherwiseEquivalent(t *testing.T) {
	// Create two plans with equal scores but different cost classes.
	local := CandidatePlan{
		Harness:   "forge",
		CostClass: "local",
		Viable:    true,
		Score:     100,
	}
	cloud := CandidatePlan{
		Harness:   "codex",
		CostClass: "medium",
		Viable:    true,
		Score:     100,
	}

	ranked := RankCandidates("", []CandidatePlan{cloud, local})
	require.Len(t, ranked, 2)

	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	assert.Equal(t, "forge", best.Harness, "local harness should win on equal scores")
}

// TestAgentRoutingUsesStableTieBreaker verifies that equal-score non-local
// candidates are ordered alphabetically (stable).
func TestAgentRoutingUsesStableTieBreaker(t *testing.T) {
	plans := []CandidatePlan{
		{Harness: "zharness", CostClass: "medium", Viable: true, Score: 50},
		{Harness: "aharness", CostClass: "medium", Viable: true, Score: 50},
	}

	ranked := RankCandidates("", plans)
	require.Len(t, ranked, 2)
	assert.Equal(t, "aharness", ranked[0].Harness, "alphabetical tie-breaker: a before z")
}

// --- SelectBestCandidate ---

// TestSelectBestCandidateReturnsErrorWhenAllRejected verifies that an error is
// returned when no viable candidates exist.
func TestSelectBestCandidateReturnsErrorWhenAllRejected(t *testing.T) {
	plans := []CandidatePlan{
		{Harness: "codex", Viable: false, RejectReason: "not installed"},
		{Harness: "claude", Viable: false, RejectReason: "quota exceeded"},
	}
	_, err := SelectBestCandidate(plans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no viable harness candidate")
}

// --- Quota Parsing ---

// TestParseQuotaOutputBasic verifies basic quota string parsing.
func TestParseQuotaOutputBasic(t *testing.T) {
	output := "83% of 5h limit"
	q := ParseQuotaOutput(output)
	require.NotNil(t, q)
	assert.Equal(t, 83, q.PercentUsed)
	assert.Equal(t, "5h", q.LimitWindow)
	assert.Empty(t, q.ResetDate)
}

// TestParseQuotaOutputWithResetDate verifies parsing with reset date.
func TestParseQuotaOutputWithResetDate(t *testing.T) {
	output := "75% of 7 day limit, resets April 12"
	q := ParseQuotaOutput(output)
	require.NotNil(t, q)
	assert.Equal(t, 75, q.PercentUsed)
	assert.Equal(t, "7 day", q.LimitWindow)
	assert.Equal(t, "April 12", q.ResetDate)
}

// TestParseQuotaOutputNoMatch verifies nil return when no quota data present.
func TestParseQuotaOutputNoMatch(t *testing.T) {
	q := ParseQuotaOutput("some other output\nno quota here")
	assert.Nil(t, q)
}

// TestParseQuotaOutputMultiline verifies that quota is found in multi-line output.
func TestParseQuotaOutputMultiline(t *testing.T) {
	output := `Model: claude-sonnet-4-6
Context window: 200000 tokens
Usage:
  83% of 5h limit, resets April 15
`
	q := ParseQuotaOutput(output)
	require.NotNil(t, q)
	assert.Equal(t, 83, q.PercentUsed)
	assert.Equal(t, "5h", q.LimitWindow)
	assert.Equal(t, "April 15", q.ResetDate)
}

// --- HarnessState Probing ---

// TestProbeHarnessStateEmbeddedAlwaysReachable verifies that embedded harnesses
// are always installed/reachable without binary lookup.
func TestProbeHarnessStateEmbeddedAlwaysReachable(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	// LookPath that always fails — should not matter for embedded harnesses.
	r.LookPath = func(string) (string, error) {
		return "", assert.AnError
	}

	state := r.ProbeHarnessState("forge", 5*time.Second)
	assert.True(t, state.Installed)
	assert.True(t, state.Reachable)
	assert.True(t, state.Authenticated)
	assert.True(t, state.QuotaOK)
	assert.Empty(t, state.Error)
}

// TestProbeHarnessStateNotInstalled verifies not-installed state when binary missing.
func TestProbeHarnessStateNotInstalled(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = func(string) (string, error) {
		return "", assert.AnError
	}

	state := r.ProbeHarnessState("codex", 5*time.Second)
	assert.False(t, state.Installed)
	assert.NotEmpty(t, state.Error)
}

// quotaTestHarness returns a harness fixture with a non-interactive quota CLI command
// configured, used to exercise the probeQuota code path in tests.
func quotaTestHarness() Harness {
	return Harness{
		Name:         "quota-test",
		Binary:       "quota-test",
		PromptMode:   "arg",
		QuotaCommand: "usage",
	}
}

// TestProbeQuotaUsesDirectInvocationNotLLMFlags verifies that probeQuota invokes the
// binary with only the QuotaCommand args — not LLM invocation flags like
// --no-session-persistence, --print, or --output-format.
func TestProbeQuotaUsesDirectInvocationNotLLMFlags(t *testing.T) {
	mock := &mockExecutor{output: "83% of 5h limit"}
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Executor = mock
	r.Registry.harnesses["quota-test"] = quotaTestHarness()

	r.ProbeHarnessState("quota-test", 5*time.Second)

	// Quota probe must use only the QuotaCommand args — no LLM invocation flags.
	assert.Equal(t, []string{"usage"}, mock.lastArgs,
		"probeQuota must not pass LLM invocation flags as args")
	assert.NotContains(t, mock.lastArgs, "--print")
	assert.NotContains(t, mock.lastArgs, "--output-format")
}

// TestProbeHarnessStateQuotaOKWhenUnder95(t *testing.T) verifies that quota is
// still OK when usage is below the 95% threshold.
func TestProbeHarnessStateQuotaOKWhenUnder95(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Executor = &mockExecutor{
		output: "83% of 5h limit",
	}
	r.Registry.harnesses["quota-test"] = quotaTestHarness()

	state := r.ProbeHarnessState("quota-test", 5*time.Second)
	assert.True(t, state.Installed)
	assert.True(t, state.Reachable)
	assert.True(t, state.QuotaOK)
	require.NotNil(t, state.Quota)
	assert.Equal(t, 83, state.Quota.PercentUsed)
}

// TestProbeHarnessStateQuotaBlockedAt95(t *testing.T) verifies quota is blocked
// when at or above 95%.
func TestProbeHarnessStateQuotaBlockedAt95(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	r.Executor = &mockExecutor{
		output: "97% of 5h limit",
	}
	r.Registry.harnesses["quota-test"] = quotaTestHarness()

	state := r.ProbeHarnessState("quota-test", 5*time.Second)
	assert.True(t, state.Installed)
	assert.False(t, state.QuotaOK, "quota should be blocked at 97% usage")
	require.NotNil(t, state.Quota)
	assert.Equal(t, 97, state.Quota.PercentUsed)
}

// --- Capabilities and Doctor routing-relevant output ---

// TestAgentCapabilitiesShowsEffectiveProfileMappings verifies that Capabilities
// populates ProfileMappings for harnesses that have them.
func TestAgentCapabilitiesShowsEffectiveProfileMappings(t *testing.T) {
	r := newTestRunnerForRouting()

	caps, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.Equal(t, "codex", caps.Surface)
	assert.False(t, caps.IsLocal)
	assert.True(t, caps.ExactPinSupport)
	assert.True(t, caps.SupportsEffort)
	// codex has profile mappings in the catalog.
	assert.NotEmpty(t, caps.ProfileMappings, "codex should have profile mappings")
}

// TestAgentCapabilitiesShowsExactPinSupport verifies that ExactPinSupport is
// correctly reported per harness.
func TestAgentCapabilitiesShowsExactPinSupport(t *testing.T) {
	r := newTestRunnerForRouting()

	// forge supports exact pins.
	capForge, err := r.Capabilities("forge")
	require.NoError(t, err)
	assert.True(t, capForge.ExactPinSupport)
	assert.True(t, capForge.IsLocal)

	// codex supports exact pins.
	capCodex, err := r.Capabilities("codex")
	require.NoError(t, err)
	assert.True(t, capCodex.ExactPinSupport)
	assert.False(t, capCodex.IsLocal)
}

// TestAgentDoctorReportsRoutingRelevantHarnessState verifies that ProbeHarnessState
// returns all routing-relevant fields, not just binary availability.
func TestAgentDoctorReportsRoutingRelevantHarnessState(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	r.LookPath = mockLookPath
	// Executor returns quota data.
	r.Executor = &mockExecutor{
		output: "75% of 7 day limit, resets April 12",
	}
	r.Registry.harnesses["quota-test"] = quotaTestHarness()

	state := r.ProbeHarnessState("quota-test", 5*time.Second)

	// All routing-relevant fields should be populated.
	assert.True(t, state.Installed)
	assert.True(t, state.Reachable)
	assert.True(t, state.Authenticated)
	assert.True(t, state.QuotaOK)
	assert.False(t, state.Degraded)
	assert.False(t, state.LastChecked.IsZero())
	require.NotNil(t, state.Quota)
	assert.Equal(t, 75, state.Quota.PercentUsed)
	assert.Equal(t, "7 day", state.Quota.LimitWindow)
	assert.Equal(t, "April 12", state.Quota.ResetDate)
}

// TestBuildCandidatePlansHarnessOverrideWithProfileResolvesConcreteModel verifies
// that when both HarnessOverride and Profile are set, evaluateCandidate still
// populates ConcreteModel via the catalog. Regression test for the bug where
// the HarnessOverride branch only set RequestedRef and left ConcreteModel empty.
func TestBuildCandidatePlansHarnessOverrideWithProfileResolvesConcreteModel(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "codex",
		Profile:         "cheap",
	}, states)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}

	require.NotNil(t, codexPlan, "codex plan should be present")
	assert.True(t, codexPlan.Viable, "codex plan should be viable")
	assert.NotEmpty(t, codexPlan.ConcreteModel, "ConcreteModel must not be empty when HarnessOverride+Profile are set")
	// catalog: cheap on codex surface -> gpt-5.4-mini
	assert.Equal(t, "gpt-5.4-mini", codexPlan.ConcreteModel)
}
