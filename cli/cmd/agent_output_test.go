package cmd

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/stretchr/testify/assert"
)

func TestFormatAttemptRouteEconomics(t *testing.T) {
	got := formatAttemptRouteEconomics(agent.ExecuteBeadReport{
		Harness:                     "agent",
		Provider:                    "openai",
		Model:                       "gpt-5.2",
		PredictedPower:              82,
		PredictedSpeedTPS:           35.5,
		PredictedCostUSDPer1kTokens: 0.012345,
		PredictedCostSource:         "catalog",
	})

	assert.Equal(t, "harness=agent provider=openai model=gpt-5.2 power=82 speed=35.5 tok/s cost=$0.012345/1k tok source=catalog", got)
}
