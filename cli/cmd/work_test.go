package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/agent/executeloop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkCommandHasPassthroughFlags verifies that ddx work exposes the
// harness/provider/model/min-power/max-power passthrough flags that operators
// use to constrain agent routing. These flags are forwarded opaquely; ddx work
// does not validate or branch on their string values.
func TestWorkCommandHasPassthroughFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	require.NotNil(t, workCmd)

	for _, name := range []string{"harness", "provider", "model", "min-power", "max-power"} {
		f := workCmd.Flags().Lookup(name)
		assert.NotNil(t, f, "ddx work must have --%s passthrough flag", name)
	}
}

func TestParseExecuteLoopFlags_AllFlagsPopulateSpec(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)

	setFlag := func(name, value string) {
		t.Helper()
		require.NoError(t, workCmd.Flags().Set(name, value))
	}
	setFlag("project", dir)
	setFlag("from", "HEAD~1")
	setFlag("harness", "claude")
	setFlag("model", "sonnet")
	setFlag("profile", "smart")
	setFlag("provider", "anthropic")
	setFlag("model-ref", "code-medium")
	setFlag("effort", "high")
	setFlag("watch", "true")
	setFlag("idle-interval", "45s")
	setFlag("json", "true")
	setFlag("local", "true")
	setFlag("no-review", "true")
	setFlag("no-review-i-know-what-im-doing", "true")
	setFlag("review-harness", "codex")
	setFlag("review-model", "gpt-5.4")
	setFlag("max-cost", "12.5")
	setFlag("max-recovery-cost", "2.5")
	setFlag("request-timeout", "2m")
	setFlag("rate-limit-max-wait", "90s")
	setFlag("min-power", "7")
	setFlag("max-power", "8")

	spec, dispatch, err := parseExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, dir, spec.ProjectRoot)
	assert.Equal(t, "HEAD~1", spec.FromRev)
	assert.Equal(t, "claude", spec.Harness)
	assert.Equal(t, "sonnet", spec.Model)
	assert.Equal(t, "smart", spec.Profile)
	assert.Equal(t, "anthropic", spec.Provider)
	assert.Equal(t, "code-medium", spec.ModelRef)
	assert.Equal(t, "high", spec.Effort)
	assert.Equal(t, executeloop.ModeWatch, spec.Mode)
	assert.Equal(t, 45*time.Second, spec.IdleInterval.Duration)
	assert.True(t, spec.NoReview)
	assert.Equal(t, "codex", spec.ReviewHarness)
	assert.Equal(t, "gpt-5.4", spec.ReviewModel)
	assert.True(t, spec.OpaquePassthrough)
	assert.Equal(t, 12.5, spec.MaxCostUSD)
	assert.Equal(t, 2.5, spec.MaxRecoveryCostUSD)
	assert.Equal(t, 2*time.Minute, spec.RequestTimeout.Duration)
	assert.Equal(t, 90*time.Second, spec.RateLimitMaxWait.Duration)
	assert.Equal(t, 7, spec.MinPower)
	assert.Equal(t, 8, spec.MaxPower)
	assert.Equal(t, executeloop.SpecCurrentVersion, spec.SpecVersion)
	assert.Equal(t, "true", dispatch.JSON)
	assert.True(t, dispatch.Local)
}

func TestParseExecuteLoopSpec_RateLimitMaxWait(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want time.Duration
	}{
		{name: "default", want: agent.RateLimitRetryDefaultBudget},
		{name: "explicit positive", set: "90s", want: 90 * time.Second},
		{name: "zero uses runtime default", set: "0", want: 0},
		{name: "negative disables retry", set: "-1s", want: -1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			root := NewCommandFactory(dir).NewRootCommand()
			workCmd, _, err := root.Find([]string{"work"})
			require.NoError(t, err)
			if tt.set != "" {
				require.NoError(t, workCmd.Flags().Set("rate-limit-max-wait", tt.set))
			}

			spec, _, err := parseExecuteLoopSpec(workCmd, true)
			require.NoError(t, err)
			assert.Equal(t, tt.want, spec.RateLimitMaxWait.Duration)
		})
	}
}

func TestRunAgentExecuteLoopImpl_PassesRateLimitMaxWait(t *testing.T) {
	var out bytes.Buffer
	budget := 42 * time.Second

	runtime := executeLoopAttemptRuntime(
		executeloop.ExecuteLoopSpec{
			FromRev:          "HEAD~1",
			RateLimitMaxWait: executeloop.Duration{Duration: budget},
		},
		&out,
		nil,
		nil,
		nil,
	)

	assert.Equal(t, "HEAD~1", runtime.FromRev)
	assert.Same(t, &out, runtime.Output)
	assert.Equal(t, budget, runtime.RateLimitMaxWait)
}

func TestParseExecuteLoopFlags_OnceFlag(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("once", "true"))

	spec, _, err := parseExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, executeloop.ModeOnce, spec.Mode)
	assert.Zero(t, spec.IdleInterval.Duration)
}

func TestParseExecuteLoopFlags_OnceAndWatchAreMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("once", "true"))
	require.NoError(t, workCmd.Flags().Set("watch", "true"))

	_, _, err = parseExecuteLoopSpec(workCmd, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--once and --watch are mutually exclusive")
}

func TestParseExecuteLoopFlags_IdleIntervalRequiresWatch(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()
	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err)
	require.NoError(t, workCmd.Flags().Set("idle-interval", "15s"))

	_, _, err = parseExecuteLoopSpec(workCmd, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--idle-interval only applies in watch mode")
}

// TestLegacyAgentNamespaceRemoved verifies that the old nested command name is
// no longer part of the public CLI surface.
func TestLegacyAgentNamespaceRemoved(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	_, err := executeCommand(root, "agent", "execute-loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestWork_PreservesJSONResultSchema_SameAsExecuteLoop(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  2,
		Successes: 1,
		Failures:  1,
		Results: []agent.ExecuteBeadReport{
			{BeadID: "ddx-1", Status: agent.ExecuteBeadStatusSuccess},
			{BeadID: "ddx-2", Status: agent.ExecuteBeadStatusExecutionFailed},
		},
	}, true)
	require.NoError(t, err)

	var payload struct {
		ProjectRoot string `json:"project_root"`
		*agent.ExecuteBeadLoopResult
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	assert.Equal(t, "/tmp/project", payload.ProjectRoot)
	require.NotNil(t, payload.ExecuteBeadLoopResult)
	assert.Equal(t, 2, payload.Attempts)
	assert.Len(t, payload.Results, 2)
}

// TestWorkHelpDocumentsReviewGuardrails verifies that the work help text
// documents the break-glass review override and the review:skip rationale
// requirement so operators can discover both guardrails from the CLI.
func TestWorkHelpDocumentsReviewGuardrails(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	out, err := executeCommand(root, "work", "--help")
	require.NoError(t, err)

	assert.Contains(t, out, "--no-review-i-know-what-im-doing")
	assert.Contains(t, out, "review:skip-reason:*")
	assert.Contains(t, out, "break-glass")
}

// TestWorkPassthroughNotValidated is the primary AC test: ddx work must NOT
// call ValidateForExecuteLoopViaService. When an unknown harness is provided
// and no ready beads exist, work must succeed with no_ready_work rather than
// failing on harness validation. This proves harness/provider/model are opaque
// passthrough — ddx work does not validate or branch on their string values.
func TestWorkPassthroughNotValidated(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()

	// "bogus_harness_xyz_notreal" does not exist in any harness registry.
	// ValidateForExecuteLoopViaService would fail with "unknown harness: ..."
	// if called. ddx work must skip that call and proceed to the queue scan,
	// finding no ready beads and returning successfully.
	out, err := executeCommand(root, "work",
		"--json", "--once",
		"--harness=bogus_harness_xyz_notreal",
	)
	require.NoError(t, err,
		"ddx work must not fail on unknown harness when there are no ready beads; "+
			"harness is opaque passthrough, not validated by ddx work")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork,
		"ddx work with no ready beads must report no_ready_work=true")
}

func TestWorkDefaultDrainModeExitsOnEmptyQueue(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()
	// Use a generous safety-net timeout so the race detector's overhead
	// under parallel test runs does not fire this cancel before the command
	// can respond. The actual performance assertion is the elapsed < 5s
	// check below, which catches the bug (ddx work hanging on empty queue).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root.SetContext(ctx)

	start := time.Now()
	out, err := executeCommand(root, "work", "--json")
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Less(t, elapsed, 5*time.Second,
		"ddx work must not wait for the long-running worker poll interval by default")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
		Attempts    int  `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.Equal(t, 0, res.Attempts)
}

func TestWorkWatchModeFlagsArePresent(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	watch := workCmd.Flags().Lookup("watch")
	require.NotNil(t, watch)
	assert.Contains(t, watch.Usage, "Keep watching")
	idle := workCmd.Flags().Lookup("idle-interval")
	require.NotNil(t, idle)
	assert.Equal(t, "30s", idle.DefValue)
	assert.Contains(t, idle.Usage, "watch mode")
}

func TestWorkPollIntervalRemoved(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	assert.Nil(t, workCmd.Flags().Lookup("poll-interval"))

	_, err = executeCommand(root, "work", "--poll-interval=30s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag: --poll-interval")
}

func TestWorkHelpDocumentsWatchMode(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	out, err := executeCommand(root, "work", "--help")
	require.NoError(t, err)

	assert.Contains(t, out, "ddx work")
	assert.Contains(t, out, "ddx work --once")
	assert.Contains(t, out, "ddx work --watch")
	assert.Contains(t, out, "ddx work --watch --idle-interval 15s")
	assert.Contains(t, out, "--watch")
	assert.Contains(t, out, "--idle-interval")
	assert.NotContains(t, out, "--poll-interval")
}

func TestWorkDefaultOutput_DoesNotPrintBareRouteSummaryAfterScopedRoutes(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  1,
		Successes: 1,
		Failures:  0,
		Results: []agent.ExecuteBeadReport{{
			Harness:                     "agent",
			Provider:                    "openai",
			Model:                       "gpt-5.2",
			ActualPower:                 78,
			PredictedPower:              82,
			PredictedSpeedTPS:           35.5,
			PredictedCostUSDPer1kTokens: 0.012345,
			PredictedCostSource:         "catalog",
		}},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.NotContains(t, got, "\nroute:")
	assert.NotContains(t, got, "route: harness=agent")
	assert.Contains(t, got, "attempts: 1")
}

func TestWorkDefaultOutput_PrintsCompactTerminalSummary(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:      2,
		Successes:     2,
		Failures:      0,
		StopCondition: "Drained",
		ExitReason:    "drained",
		Results: []agent.ExecuteBeadReport{
			{BeadID: "ddx-changed", Status: agent.ExecuteBeadStatusSuccess},
			{BeadID: "ddx-satisfied", Status: agent.ExecuteBeadStatusAlreadySatisfied},
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "worker exited: drained current execution-ready queue")
	assert.Contains(t, got, "attempts: 2  |  closed: 2  |  changed: 1  |  already-satisfied: 1  |  failures: 0")
	assert.NotContains(t, got, "completed:")
}

func TestWorkDefaultOutput_PrintsQueueStateAndHumanBlockers(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:    1,
		Successes:   1,
		NoReadyWork: true,
		ExitReason:  "drained",
		Results: []agent.ExecuteBeadReport{{
			BeadID: "ddx-done",
			Status: agent.ExecuteBeadStatusSuccess,
		}},
		QueueSnapshot: &agent.QueueSnapshot{
			ExecutionReadyCount:            0,
			BlockedCount:                   30,
			ProposedOperatorAttentionCount: 3,
			HumanReviewBlockerCount:        3,
			HumanReviewBlockedTotal:        30,
			RetryCooldownCount:             2,
			NextRetryAfter:                 "2026-05-09T12:00:00Z",
			ExecutionIneligibleCount:       1,
			SupersededCount:                1,
			SkippedEpicsCount:              1,
			EpicClosureCandidatesCount:     1,
			HumanReviewBlockers: []agent.HumanReviewBlockerSnapshot{
				{ID: "ddx-human-1", Title: "Needs human 1", DownstreamBlockedCount: 10},
				{ID: "ddx-human-2", Title: "Needs human 2", DownstreamBlockedCount: 10},
				{ID: "ddx-human-3", Title: "Needs human 3", DownstreamBlockedCount: 10},
			},
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "remaining queue: execution-ready=0 blocked=30 operator-attention=3 needs-human/investigation=3 cooldown/deferred=2 next-retry=2026-05-09T12:00:00Z execution-ineligible=1 superseded=1 epics=1 epic-closure-candidates=1")
	assert.Contains(t, got, "30 beads blocked behind 3 needs-human blockers:")
	assert.Contains(t, got, "1. ddx-human-1 Needs human 1 (10 downstream)")
	assert.Contains(t, got, "2. ddx-human-2 Needs human 2 (10 downstream)")
	assert.Contains(t, got, "3. ddx-human-3 Needs human 3 (10 downstream)")
}

func TestWorkStopSummary_ProposedVsDependencyWaiting(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		NoReadyWork: true,
		NoReadyWorkDetail: agent.NoReadyWorkBreakdown{
			ProposedOperatorAttention: []string{"ddx-proposed"},
			DependencyWaiting:         []string{"ddx-waiting"},
			ExternalBlocked:           []string{"ddx-external"},
			RetryCooldown:             []string{"ddx-cooldown"},
			NotEligible:               []string{"ddx-ineligible"},
			Superseded:                []string{"ddx-superseded"},
			Epics:                     []string{"ddx-epic"},
			NextRetryAfter:            "2026-05-09T12:00:00Z",
		},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "operator attention: 1 proposed bead(s) stop autonomous work and may block downstream dependents: ddx-proposed")
	assert.Contains(t, got, "waiting on dependencies: 1 open bead(s): ddx-waiting")
	assert.Contains(t, got, "external blocked: 1 bead(s) with explicit blocked status: ddx-external")
	assert.Contains(t, got, "retry cooldown: 1 bead(s) (next retry-after: 2026-05-09T12:00:00Z): ddx-cooldown")
	assert.Contains(t, got, "not execution eligible: 1 bead(s): ddx-ineligible")
	assert.Contains(t, got, "superseded: 1 bead(s): ddx-superseded")
	assert.Contains(t, got, "epic containers: 1 ready epic(s) with open children")
	assert.NotContains(t, got, "needs investigation")
	assert.NotContains(t, got, "blocked/proposed")
}

func TestWorkStopSummary_CompletedThenDrainedIncludesNoReadyDetail(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:    1,
		Successes:   1,
		NoReadyWork: true,
		NoReadyWorkDetail: agent.NoReadyWorkBreakdown{
			ProposedOperatorAttention: []string{"ddx-proposed"},
			DependencyWaiting:         []string{"ddx-waiting"},
		},
		Results: []agent.ExecuteBeadReport{{
			BeadID: "ddx-done",
			Status: agent.ExecuteBeadStatusSuccess,
			Detail: "merged cleanly",
		}},
	}, false)
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "attempts: 1  |  closed: 1  |  changed: 1  |  already-satisfied: 0  |  failures: 0")
	assert.Contains(t, got, "No execution-ready beads.")
	assert.Contains(t, got, "operator attention: 1 proposed bead(s) stop autonomous work")
	assert.Contains(t, got, "waiting on dependencies: 1 open bead(s): ddx-waiting")
}

func TestWorkStopSummary_DrainedEmptyQueue(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		NoReadyWork: true,
	}, false)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "queue drained: no open work remains in lifecycle queues.")
}

func TestWorkJSONOutput_IncludesRouteEconomicsWithoutHumanLines(t *testing.T) {
	var out bytes.Buffer
	err := writeExecuteLoopResult(&out, "/tmp/project", &agent.ExecuteBeadLoopResult{
		Attempts:  1,
		Successes: 1,
		Failures:  0,
		Results: []agent.ExecuteBeadReport{{
			Harness:                     "agent",
			Provider:                    "openai",
			Model:                       "gpt-5.2",
			ActualPower:                 78,
			PredictedPower:              82,
			PredictedSpeedTPS:           35.5,
			PredictedCostUSDPer1kTokens: 0.012345,
			PredictedCostSource:         "catalog",
		}},
	}, true)
	require.NoError(t, err)

	jsonStart := strings.Index(out.String(), "{")
	require.NotEqual(t, -1, jsonStart, "output should contain JSON: %s", out)
	assert.NotContains(t, out.String()[:jsonStart], "route: ")

	var res struct {
		Attempts int `json:"attempts"`
		Results  []struct {
			Harness                     string  `json:"harness"`
			Provider                    string  `json:"provider"`
			Model                       string  `json:"model"`
			ActualPower                 int     `json:"actual_power"`
			PredictedPower              int     `json:"predicted_power"`
			PredictedSpeedTPS           float64 `json:"predicted_speed_tps"`
			PredictedCostUSDPer1kTokens float64 `json:"predicted_cost_usd_per_1k_tokens"`
			PredictedCostSource         string  `json:"predicted_cost_source"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()[jsonStart:]), &res))
	require.Equal(t, 1, res.Attempts)
	require.Len(t, res.Results, 1)
	assert.Equal(t, "agent", res.Results[0].Harness)
	assert.Equal(t, "openai", res.Results[0].Provider)
	assert.Equal(t, "gpt-5.2", res.Results[0].Model)
	assert.Equal(t, 78, res.Results[0].ActualPower)
	assert.Equal(t, 82, res.Results[0].PredictedPower)
	assert.Equal(t, 35.5, res.Results[0].PredictedSpeedTPS)
	assert.Equal(t, 0.012345, res.Results[0].PredictedCostUSDPer1kTokens)
	assert.Equal(t, "catalog", res.Results[0].PredictedCostSource)
}
