// Package violations is the routinglint analyzer-test stub. Each
// declaration or reference below introduces exactly one forbidden
// pattern; the wanted-diagnostic comment on the same line is pinned
// by the analysistest framework.
package violations

import (
	agentlib "github.com/easel/fizeau"
	"github.com/spf13/cobra"
)

// Pattern A: forbidden identifier as a function declaration name.

func ResolveProfileLadder() string { return "" } // want `routinglint: forbidden identifier ResolveProfileLadder`

func ResolveTierModelRef() string { return "" } // want `routinglint: forbidden identifier ResolveTierModelRef`

func ResolveProfileLadderCallCount() int { return 0 } // want `routinglint: forbidden identifier ResolveProfileLadderCallCount`

func AdaptiveMinTier() int { return 0 } // want `routinglint: forbidden identifier AdaptiveMinTier`

func workersByHarness() map[string]int { return nil } // want `routinglint: forbidden identifier workersByHarness`

func queueByHarness() []string { return nil } // want `routinglint: forbidden identifier queueByHarness`

func retryByPassthrough() int { return 0 } // want `routinglint: forbidden identifier retryByPassthrough`

func endpointByPassthrough() string { return "" } // want `routinglint: forbidden identifier endpointByPassthrough`

func catalogThresholdForHarness() float64 { return 0 } // want `routinglint: forbidden identifier catalogThresholdForHarness`

// Pattern B: forbidden string literals.

const escalateFlag = "--escalate" // want `routinglint: forbidden string literal`

const overrideModelFlag = "--override-model" // want `routinglint: forbidden string literal`

const profileLaddersKey = "profile_ladders" // want `routinglint: forbidden string literal`

const modelOverridesKey = "model_overrides" // want `routinglint: forbidden string literal`

const dottedProfileKey = "agent.routing.profile_ladders" // want `routinglint: forbidden string literal`

const dottedOverrideKey = "agent.routing.model_overrides" // want `routinglint: forbidden string literal`

// Pattern C: forbidden public DDx command surface.
var retiredAgentCommand = cobra.Command{Use: "agent"} // want `routinglint: forbidden public command surface Use:"agent"`

// Pattern D: config-derived / normalized route sources inside a
// ServiceExecuteRequest literal are forbidden because explicit
// passthrough flags must stay opaque.
type resolvedConfig struct{}

func (resolvedConfig) Model() string    { return "qwen36" }
func (resolvedConfig) Provider() string { return "openrouter" }
func (resolvedConfig) Harness() string  { return "claude" }

func normalizeHarness(name string) string { return name }
func fuzzyMatchModel(name string) string  { return name }

var rcfg resolvedConfig
var cfg resolvedConfig
var model string
var provider string
var harness string

var forbiddenExecuteRequest = agentlib.ServiceExecuteRequest{
	Model:    rcfg.Model(),               // want `routinglint: forbidden Model source for ServiceExecuteRequest`
	Provider: cfg.Provider(),             // want `routinglint: forbidden Provider source for ServiceExecuteRequest`
	Harness:  normalizeHarness("claude"), // want `routinglint: forbidden Harness source for ServiceExecuteRequest`
}

var forbiddenExecuteRequest2 = agentlib.ServiceExecuteRequest{
	Model:    fuzzyMatchModel("qwen36"), // want `routinglint: forbidden Model source for ServiceExecuteRequest`
	Provider: "openrouter",
	Harness:  "claude",
}

var allowedExecuteRequest = agentlib.ServiceExecuteRequest{
	Model:    model,
	Provider: provider,
	Harness:  harness,
}
