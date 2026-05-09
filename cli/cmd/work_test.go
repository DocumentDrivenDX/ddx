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
	setFlag("poll-interval", "45s")
	setFlag("json", "true")
	setFlag("local", "true")
	setFlag("no-review", "true")
	setFlag("no-review-i-know-what-im-doing", "true")
	setFlag("review-harness", "codex")
	setFlag("review-model", "gpt-5.4")
	setFlag("max-cost", "12.5")
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
	require.NoError(t, workCmd.Flags().Set("poll-interval", "45s"))

	spec, _, err := parseExecuteLoopSpec(workCmd, true)
	require.NoError(t, err)

	assert.Equal(t, executeloop.ModeOnce, spec.Mode)
	assert.Zero(t, spec.IdleInterval.Duration)
}

// TestAgentExecuteLoopCommandRemoved verifies that the old nested command name
// is no longer part of the public CLI surface.
func TestAgentExecuteLoopCommandRemoved(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	found, remaining, err := root.Find([]string{"agent", "execute-loop"})
	require.NoError(t, err)
	assert.NotEqual(t, "execute-loop", found.Name())
	assert.Equal(t, []string{"execute-loop"}, remaining)

	_, err = executeCommand(root, "agent", "execute-loop")
	require.Error(t, err)
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

func TestWorkDefaultPollIntervalExitsOnEmptyQueue(t *testing.T) {
	env := NewTestEnvironment(t)
	root := NewCommandFactory(env.Dir).NewRootCommand()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	root.SetContext(ctx)

	start := time.Now()
	out, err := executeCommand(root, "work", "--json")
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Less(t, elapsed, 500*time.Millisecond,
		"ddx work must not wait for the long-running worker poll interval by default")

	var res struct {
		NoReadyWork bool `json:"no_ready_work"`
		Attempts    int  `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.NoReadyWork)
	assert.Equal(t, 0, res.Attempts)
}

func TestWorkLongRunningPollIntervalRemainsOptIn(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")
	flag := workCmd.Flags().Lookup("poll-interval")
	require.NotNil(t, flag)
	assert.Equal(t, "0s", flag.DefValue)
	assert.Contains(t, flag.Usage, "Set 30s to keep the worker alive")
}

func TestWorkDefaultOutput_PrintsSelectedRouteEconomics(t *testing.T) {
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
	expected := "route: harness=agent provider=openai model=gpt-5.2 power=82 speed=35.5 tok/s cost=$0.012345/1k tok source=catalog"
	assert.Contains(t, got, expected)
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
