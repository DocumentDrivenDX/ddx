// Package violations is the routinglint analyzer-test stub. Each
// declaration or reference below introduces exactly one forbidden
// pattern; the wanted-diagnostic comment on the same line is pinned
// by the analysistest framework.
package violations

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
