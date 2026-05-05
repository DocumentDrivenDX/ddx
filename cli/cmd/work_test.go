package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/spf13/pflag"
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

// TestWorkCommandHasAllExecuteLoopFlags verifies that ddx work exposes the
// full set of flags that operators need to control queue drain behavior.
// All execute-loop flags must be present so operators can use work as the
// primary queue drain surface without switching commands.
func TestWorkCommandHasAllExecuteLoopFlags(t *testing.T) {
	dir := t.TempDir()
	root := NewCommandFactory(dir).NewRootCommand()

	workCmd, _, err := root.Find([]string{"work"})
	require.NoError(t, err, "ddx work must exist")

	loopCmd, _, err := root.Find([]string{"agent", "execute-loop"})
	require.NoError(t, err, "ddx agent execute-loop must exist")

	loopFlags := map[string]bool{}
	loopCmd.Flags().VisitAll(func(f *pflag.Flag) {
		loopFlags[f.Name] = true
	})

	for name := range loopFlags {
		if workCmd.Flags().Lookup(name) == nil {
			t.Errorf("ddx work missing flag --%s from execute-loop", name)
		}
	}
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
