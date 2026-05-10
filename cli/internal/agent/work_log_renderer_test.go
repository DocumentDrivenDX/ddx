package agent

import (
	"strings"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedWorkLogTime() time.Time {
	return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
}

func TestWorkLogRenderer_FormatLifecycleLine(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now:           fixedWorkLogTime,
		CurrentBeadID: "ddx-current",
	})

	got := renderer.FormatLifecycleLine(WorkLogLifecycleLine{
		Phase:       "readiness",
		Message:     "waiting on readiness gate",
		Harness:     "agent",
		Provider:    "openrouter",
		Model:       "gpt-5.4-mini",
		RouteReason: "profile",
	})

	require.Equal(t, "12:34:56 readiness waiting on readiness gate route: harness=agent provider=openrouter model=gpt-5.4-mini reason=profile\n", got)
	assert.NotContains(t, got, ".789")
}

func TestWorkLogRenderer_PreservesHeaderTitlePunctuation(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{Now: fixedWorkLogTime})

	got := renderer.FormatHeader("ddx-a1b2c3d4", "spec: define full DDx temp cleanup in work cycle")

	assert.Equal(t, "\n▶ ddx-a1b2c3d4: spec: define full DDx temp cleanup in work cycle\n", got)
}

func TestWorkLogRenderer_SuppressesOnlyCurrentScopedBeadID(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now:           fixedWorkLogTime,
		CurrentBeadID: "ddx-current",
	})

	current := renderer.FormatLifecycleLine(WorkLogLifecycleLine{
		Phase:   "progress",
		BeadID:  "ddx-current",
		Message: "follow-on text",
	})
	other := renderer.FormatLifecycleLine(WorkLogLifecycleLine{
		Phase:   "progress",
		BeadID:  "ddx-other",
		Message: "follow-on text",
	})

	assert.NotContains(t, current, "ddx-current")
	assert.Contains(t, current, "follow-on text")
	assert.Contains(t, other, "ddx-other")
	assert.Contains(t, other, "follow-on text")
}

func TestWorkLogRenderer_WrapsServiceRouteAndProgress(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now:           fixedWorkLogTime,
		CurrentBeadID: "ddx-live",
		WorkPhase:     "do",
	})

	route := renderer.FormatRoutingDecision(&agentlib.ServiceRoutingDecisionData{
		Harness:  "agent",
		Provider: "openrouter",
		Model:    "gpt-5.4-mini",
		Reason:   "profile",
	})
	progress := renderer.FormatServiceProgressEntries([]agentlib.ServiceProgressData{
		{
			Phase:       "tool",
			State:       "complete",
			TaskID:      "ddx-live",
			TurnIndex:   2,
			Action:      "run tests",
			Target:      "cli/internal/bead",
			OutputBytes: 42,
			OutputLines: 3,
		},
	})

	assert.Equal(t, "12:34:56 do route agent/gpt-5.4-mini provider=openrouter reason=profile\n", route)
	assert.Equal(t, "12:34:56 do ok 2 run tests to cli/internal/bead < out=42B 3 lines\n", progress)
	assert.NotContains(t, strings.TrimSpace(progress), "ddx-live")
}
