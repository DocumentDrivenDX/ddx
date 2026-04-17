package agent

import (
	"fmt"
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
// - healthy local embedded (agent)
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
		"agent":    healthyLocalState(),
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

	// Provide state only for agent — all others are absent from the map.
	states := map[string]HarnessState{
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	// agent should be viable
	agentFound := false
	for _, p := range plans {
		if p.Harness == "agent" {
			assert.True(t, p.Viable, "agent should be viable")
			agentFound = true
		} else {
			assert.False(t, p.Viable, "harness %s not in state map should be not viable", p.Harness)
			assert.Equal(t, "not installed", p.RejectReason)
		}
	}
	assert.True(t, agentFound, "agent should be in plans")
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
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "codex":
			assert.False(t, p.Viable)
			assert.Equal(t, "policy restricted", p.RejectReason)
		case "agent":
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
		"agent": healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)

	for _, p := range plans {
		switch p.Harness {
		case "codex":
			assert.False(t, p.Viable)
			assert.Equal(t, "not reachable", p.RejectReason)
		case "agent":
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
		"agent": healthyLocalState(),
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, states)
	ranked := RankCandidates("cheap", plans)

	require.NotEmpty(t, ranked)
	// The top viable candidate should be local (agent).
	best, err := SelectBestCandidate(ranked)
	require.NoError(t, err)
	assert.Equal(t, "local", best.CostClass, "cheap profile should prefer local harness")
}

// TestAgentRoutingFastPrefersFastestViableCandidate verifies that fast profile
// prefers local (lowest latency, no network) over cloud.
func TestAgentRoutingFastPrefersFastestViableCandidate(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"agent": healthyLocalState(),
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
		"agent": healthyLocalState(),
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
		Harness:   "agent",
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
	assert.Equal(t, "agent", best.Harness, "local harness should win on equal scores")
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

	state := r.ProbeHarnessState("agent", 5*time.Second)
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

	// agent supports exact pins.
	capForge, err := r.Capabilities("agent")
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

// TestBuildCandidatePlansHarnessOverrideWithModelPinSetsConcreteModel verifies that
// when HarnessOverride and ModelPin are both set, ConcreteModel is populated from
// the pin value, and that harnesses without ExactPinSupport are rejected.
func TestBuildCandidatePlansHarnessOverrideWithModelPinSetsConcreteModel(t *testing.T) {
	r := newTestRunnerForRouting()

	// Verify positive path: HarnessOverride + ModelPin on codex (ExactPinSupport=true).
	states := map[string]HarnessState{
		"codex": healthyState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "codex",
		ModelPin:        "gpt-5.4-mini",
	}, states)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}

	require.NotNil(t, codexPlan, "codex plan should be present")
	assert.True(t, codexPlan.Viable, "codex with ExactPinSupport should be viable for HarnessOverride+ModelPin")
	assert.Equal(t, "gpt-5.4-mini", codexPlan.ConcreteModel, "ConcreteModel must equal the pin value")

	// Verify rejection path: HarnessOverride + ModelPin on a harness without ExactPinSupport.
	r.Registry.harnesses["no-pin-harness"] = Harness{
		Name:            "no-pin-harness",
		Binary:          "no-pin-harness",
		Surface:         "no-pin",
		CostClass:       "medium",
		ExactPinSupport: false,
		IsLocal:         false,
	}

	statesNoPinOnly := map[string]HarnessState{
		"no-pin-harness": healthyState(),
	}

	plansNoPin := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "no-pin-harness",
		ModelPin:        "gpt-5.4-mini",
	}, statesNoPinOnly)

	var noPinPlan *CandidatePlan
	for i := range plansNoPin {
		if plansNoPin[i].Harness == "no-pin-harness" {
			noPinPlan = &plansNoPin[i]
			break
		}
	}

	require.NotNil(t, noPinPlan, "no-pin-harness plan should be present")
	assert.False(t, noPinPlan.Viable, "harness without ExactPinSupport should be rejected for HarnessOverride+ModelPin")
	assert.Contains(t, noPinPlan.RejectReason, "does not support exact model pins")
}

// TestBuildCandidatePlansHarnessOverrideWithModelRefResolvesCatalogAndDeprecation
// verifies that when HarnessOverride and ModelRef are both set, ConcreteModel is
// resolved from the catalog for the harness's surface, and deprecation warnings
// are propagated when the ref is deprecated.
func TestBuildCandidatePlansHarnessOverrideWithModelRefResolvesCatalogAndDeprecation(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"codex": healthyState(),
	}

	// "codex-mini" is deprecated in BuiltinCatalog and mapped to the codex surface.
	plans := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "codex",
		ModelRef:        "codex-mini",
	}, states)

	var codexPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "codex" {
			codexPlan = &plans[i]
			break
		}
	}

	require.NotNil(t, codexPlan, "codex plan should be present")
	assert.True(t, codexPlan.Viable, "codex should be viable for HarnessOverride+ModelRef with a known ref")
	assert.NotEmpty(t, codexPlan.ConcreteModel, "ConcreteModel must be populated via catalog resolution")
	assert.NotEmpty(t, codexPlan.DeprecationWarning, "deprecated ref should produce a deprecation warning")
	assert.Contains(t, codexPlan.DeprecationWarning, "codex-mini")

	// Also verify rejection when the ModelRef is unknown on the harness surface.
	plansUnknown := r.BuildCandidatePlans(RouteRequest{
		HarnessOverride: "codex",
		ModelRef:        "qwen3", // qwen3 is embedded-openai only, not on codex surface
	}, states)

	var codexUnknown *CandidatePlan
	for i := range plansUnknown {
		if plansUnknown[i].Harness == "codex" {
			codexUnknown = &plansUnknown[i]
			break
		}
	}

	require.NotNil(t, codexUnknown, "codex plan should be present for unknown ref test")
	assert.False(t, codexUnknown.Viable, "codex should be rejected when ModelRef is not available on its surface")
	assert.Contains(t, codexUnknown.RejectReason, "not available on surface")
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

// TestProviderAgentPinsHarnessToAgent verifies that --provider agent (or
// --provider local) forces HarnessOverride = "agent" so the routing engine
// never falls through to claude/codex.
func TestProviderAgentPinsHarnessToAgent(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{"agent provider pins agent harness", "agent", "agent"},
		{"local alias pins agent harness", "local", "agent"},
		{"vidar is not a harness — no override", "vidar", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NormalizeRouteRequest(
				RouteFlags{Provider: tt.provider},
				Config{},
				BuiltinCatalog,
			)
			assert.Equal(t, tt.want, req.HarnessOverride)
		})
	}
}

// TestResolveHarnessAliasLocal verifies the "local" → "agent" alias.
func TestResolveHarnessAliasLocal(t *testing.T) {
	assert.Equal(t, "agent", resolveHarnessAlias("local"))
	assert.Equal(t, "agent", resolveHarnessAlias("agent"))
	assert.Equal(t, "claude", resolveHarnessAlias("claude"))
	assert.Equal(t, "vidar", resolveHarnessAlias("vidar"))
}

// --- Historical success-rate scoring ---

// TestSuccessRateHighBeatsLow verifies that a candidate with a high historical
// success rate (>= 0.8) scores higher than one with a low rate (< 0.5) when
// cost class is otherwise equal.
func TestSuccessRateHighBeatsLow(t *testing.T) {
	high := CandidatePlan{
		Harness:               "claude",
		CostClass:             "medium",
		State:                 healthyState(),
		HistoricalSuccessRate: 0.9,
		Viable:                true,
	}
	low := CandidatePlan{
		Harness:               "local-qwen",
		CostClass:             "medium",
		State:                 healthyState(),
		HistoricalSuccessRate: 0.3,
		Viable:                true,
	}

	highScore := scoreCandidate("standard", high)
	lowScore := scoreCandidate("standard", low)

	assert.Greater(t, highScore, lowScore,
		"candidate with 90%% success should score higher than 30%% at same cost class")
	// Verify the gap matches the spec: +20 for high, -30 for low = 50 delta.
	assert.InDelta(t, 50.0, highScore-lowScore, 0.0001,
		"delta between high (+20) and low (-30) should be 50")
}

// TestSuccessRateInsufficientDataNoAdjustment verifies that a candidate with
// HistoricalSuccessRate == -1 (fewer than 3 samples) receives no bonus or
// penalty from the success-rate adjustment.
func TestSuccessRateInsufficientDataNoAdjustment(t *testing.T) {
	insufficient := CandidatePlan{
		Harness:               "new-harness",
		CostClass:             "medium",
		State:                 healthyState(),
		HistoricalSuccessRate: -1,
		Viable:                true,
	}
	// Same plan without the success-rate field populated (default zero value
	// is ambiguous, so we compare against the insufficient-data case with an
	// explicit baseline: a plan where rate is exactly the mid-band value).
	baseline := insufficient
	baseline.HistoricalSuccessRate = 0.65 // between 0.5 and 0.8 — no adjustment

	insufficientScore := scoreCandidate("standard", insufficient)
	baselineScore := scoreCandidate("standard", baseline)

	assert.Equal(t, baselineScore, insufficientScore,
		"insufficient data (-1) and mid-band (0.5-0.8) should both yield no adjustment")
}

// TestSuccessRateBuildCandidatePlansPopulatesFromStore verifies that
// BuildCandidatePlans reads routing outcomes from the configured metrics store
// and populates HistoricalSuccessRate on each plan.
func TestSuccessRateBuildCandidatePlansPopulatesFromStore(t *testing.T) {
	logDir := t.TempDir()
	store := NewRoutingMetricsStore(logDir)

	// claude: 3 successes, 0 failures -> rate 1.0
	for i := 0; i < 3; i++ {
		require.NoError(t, store.AppendOutcome(RoutingOutcome{
			Harness:    "claude",
			ObservedAt: time.Now().UTC(),
			Success:    true,
		}))
	}
	// codex: 1 success, 2 failures -> rate 0.33
	require.NoError(t, store.AppendOutcome(RoutingOutcome{
		Harness: "codex", ObservedAt: time.Now().UTC(), Success: true,
	}))
	for i := 0; i < 2; i++ {
		require.NoError(t, store.AppendOutcome(RoutingOutcome{
			Harness: "codex", ObservedAt: time.Now().UTC(), Success: false,
		}))
	}
	// agent: only 2 samples — insufficient data
	for i := 0; i < 2; i++ {
		require.NoError(t, store.AppendOutcome(RoutingOutcome{
			Harness: "agent", ObservedAt: time.Now().UTC(), Success: true,
		}))
	}

	r := NewRunner(Config{SessionLogDir: logDir})
	r.LookPath = mockLookPath

	states := map[string]HarnessState{
		"claude": healthyState(),
		"codex":  healthyState(),
		"agent":  healthyLocalState(),
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "standard"}, states)

	byName := make(map[string]CandidatePlan)
	for _, p := range plans {
		byName[p.Harness] = p
	}

	require.Contains(t, byName, "claude")
	require.Contains(t, byName, "codex")
	require.Contains(t, byName, "agent")

	assert.InDelta(t, 1.0, byName["claude"].HistoricalSuccessRate, 0.0001,
		"claude should have 100%% success rate")
	assert.InDelta(t, 1.0/3.0, byName["codex"].HistoricalSuccessRate, 0.0001,
		"codex should have 33%% success rate")
	assert.Equal(t, float64(-1), byName["agent"].HistoricalSuccessRate,
		"agent with < 3 samples should have -1 (insufficient data)")
}

// TestFastHarnessStateHTTPProviderViableWithoutBinary verifies that HTTP-only
// providers (lmstudio, openrouter) are marked installed by fastHarnessState
// even when no binary exists in PATH.
func TestFastHarnessStateHTTPProviderViableWithoutBinary(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	// LookPath always fails — no binaries on PATH.
	r.LookPath = func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	}

	for _, name := range []string{"lmstudio", "openrouter"} {
		harness, ok := r.Registry.Get(name)
		require.True(t, ok, "harness %s must be registered", name)
		require.True(t, harness.IsHTTPProvider, "harness %s must be HTTP", name)

		state := r.fastHarnessState(name, harness)
		assert.True(t, state.Installed, "%s should be installed", name)
		assert.True(t, state.Reachable, "%s should be reachable", name)
		assert.Empty(t, state.Error, "%s should have no error", name)
	}
}

// TestResolveHarnessHTTPProviderSkipsPathLookup verifies that resolveHarness
// does not reject HTTP-only providers for missing binaries.
func TestResolveHarnessHTTPProviderSkipsPathLookup(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	// LookPath always fails — no binaries on PATH.
	r.LookPath = func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	}

	for _, name := range []string{"lmstudio", "openrouter"} {
		harness, resolvedName, err := r.resolveHarness(RunOptions{Harness: name})
		require.NoError(t, err, "resolveHarness(%s) should not fail", name)
		assert.Equal(t, name, resolvedName)
		assert.True(t, harness.IsHTTPProvider)
	}
}

// TestHTTPProviderCandidateViableForCheapProfile verifies that when a catalog
// maps cheap.embedded-openai to a model, and an HTTP provider with surface
// embedded-openai is registered, the candidate is viable.
func TestHTTPProviderCandidateViableForCheapProfile(t *testing.T) {
	r := NewRunner(Config{SessionLogDir: ""})
	// LookPath always fails — only HTTP/local harnesses should survive.
	r.LookPath = func(file string) (string, error) {
		return "", fmt.Errorf("%s not found", file)
	}

	plans := r.BuildCandidatePlans(RouteRequest{Profile: "cheap"}, nil)

	// lmstudio should be viable — it's HTTP with surface embedded-openai
	// and the builtin catalog maps cheap.embedded-openai.
	var lmPlan *CandidatePlan
	for i := range plans {
		if plans[i].Harness == "lmstudio" {
			lmPlan = &plans[i]
			break
		}
	}
	require.NotNil(t, lmPlan, "lmstudio candidate must exist")
	assert.True(t, lmPlan.Viable, "lmstudio should be viable; got reject: %s", lmPlan.RejectReason)
	assert.NotEmpty(t, lmPlan.ConcreteModel, "lmstudio should have a resolved model")
}

// TestAgentRoutingExcludesTestOnlyHarnessesFromTierRouting verifies that
// TestOnly harnesses (script, virtual) are filtered out of production tier
// routing — they must never be selected by --profile cheap/standard/smart,
// even when real harnesses are unavailable. Explicit --harness <name>
// override remains the only way to reach them. Regression for ddx-869848ec:
// `script` showed up in a standard-tier fallback chain because it registered
// as IsLocal=true and scored a +25/+40 bonus in scoreCandidate.
func TestAgentRoutingExcludesTestOnlyHarnessesFromTierRouting(t *testing.T) {
	r := newTestRunnerForRouting()

	// script and virtual are "healthy" as far as state is concerned —
	// this mimics the failure scenario (they're always "available" because
	// they don't need an LLM or network). Real harnesses unavailable.
	states := map[string]HarnessState{
		"script":  healthyLocalState(),
		"virtual": healthyLocalState(),
	}

	for _, profile := range []string{"cheap", "standard", "smart"} {
		plans := r.BuildCandidatePlans(RouteRequest{Profile: profile}, states)
		for _, p := range plans {
			if p.Harness == "script" || p.Harness == "virtual" {
				t.Errorf("profile=%s: TestOnly harness %s leaked into candidate list (should have been filtered)", profile, p.Harness)
			}
		}
	}
}

// TestAgentRoutingAllowsExplicitTestOnlyHarness verifies that an explicit
// --harness override still reaches TestOnly harnesses (script is heavily
// used by the integration test suite via `--harness script`). The filter
// must apply only to profile-based selection, not to explicit overrides.
func TestAgentRoutingAllowsExplicitTestOnlyHarness(t *testing.T) {
	r := newTestRunnerForRouting()

	states := map[string]HarnessState{
		"script":  healthyLocalState(),
		"virtual": healthyLocalState(),
	}

	// Request with HarnessOverride=script should surface the script candidate.
	req := RouteRequest{Profile: "cheap", HarnessOverride: "script"}
	plans := r.BuildCandidatePlans(req, states)
	var scriptFound bool
	for _, p := range plans {
		if p.Harness == "script" {
			scriptFound = true
		}
	}
	assert.True(t, scriptFound, "explicit --harness script must surface the script candidate")
}
