package agent

import (
	"testing"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
)

// makePowerCatalog builds a []agentlib.ModelInfo with only the Power field set.
// Use this to build test catalogs without pinning model IDs or provider names.
func makePowerCatalog(powers ...int) []agentlib.ModelInfo {
	out := make([]agentlib.ModelInfo, len(powers))
	for i, p := range powers {
		out[i] = agentlib.ModelInfo{Power: p}
	}
	return out
}

func TestTopNPowerThreshold_EmptyCatalog(t *testing.T) {
	assert.Equal(t, 0, TopNPowerThreshold(nil, 1))
	assert.Equal(t, 0, TopNPowerThreshold([]agentlib.ModelInfo{}, 1))
}

func TestTopNPowerThreshold_AllZeroPower(t *testing.T) {
	// Models with zero power are unrated; catalog effectively empty.
	assert.Equal(t, 0, TopNPowerThreshold(makePowerCatalog(0, 0), 1))
}

func TestTopNPowerThreshold_Top1(t *testing.T) {
	// Distinct levels desc: [9, 8, 7, 5]. Top-1 → 9.
	assert.Equal(t, 9, TopNPowerThreshold(makePowerCatalog(9, 7, 5, 8), 1))
}

func TestTopNPowerThreshold_Top2(t *testing.T) {
	// Distinct levels desc: [9, 8, 7, 5]. Top-2 → 8.
	assert.Equal(t, 8, TopNPowerThreshold(makePowerCatalog(9, 7, 5, 8), 2))
}

func TestTopNPowerThreshold_Top3(t *testing.T) {
	// Distinct levels desc: [9, 8, 7, 5]. Top-3 → 7.
	assert.Equal(t, 7, TopNPowerThreshold(makePowerCatalog(9, 7, 5, 8), 3))
}

func TestTopNPowerThreshold_DuplicatePowers(t *testing.T) {
	// Multiple models at the same power count as one distinct level.
	models := makePowerCatalog(9, 9, 7, 7)
	// Distinct: [9, 7]. Top-1 → 9.
	assert.Equal(t, 9, TopNPowerThreshold(models, 1))
	// Top-2 → 7 (second distinct level).
	assert.Equal(t, 7, TopNPowerThreshold(models, 2))
}

func TestTopNPowerThreshold_NExceedsDistinctLevels(t *testing.T) {
	// Only 2 distinct levels: n=10 returns the lowest level so the threshold
	// remains satisfiable — it does not return 0.
	assert.Equal(t, 7, TopNPowerThreshold(makePowerCatalog(9, 7), 10))
}

func TestTopNPowerThreshold_StableAcrossOrderChange(t *testing.T) {
	// Catalog order must not affect the threshold.
	a := makePowerCatalog(5, 9, 7, 8)
	b := makePowerCatalog(9, 8, 7, 5)
	c := makePowerCatalog(7, 5, 9, 8)
	want := TopNPowerThreshold(a, 1)
	assert.Equal(t, want, TopNPowerThreshold(b, 1), "order b")
	assert.Equal(t, want, TopNPowerThreshold(c, 1), "order c")
	want2 := TopNPowerThreshold(a, 2)
	assert.Equal(t, want2, TopNPowerThreshold(b, 2), "order b n=2")
	assert.Equal(t, want2, TopNPowerThreshold(c, 2), "order c n=2")
}

func TestTopNPowerThreshold_NoBranchingOnModelOrProvider(t *testing.T) {
	// Identical power values with different IDs and providers must yield the
	// same threshold — the function must not inspect names.
	named := []agentlib.ModelInfo{
		{ID: "claude-opus-4-6", Provider: "anthropic", Power: 9},
		{ID: "gpt-5.4", Provider: "openai", Power: 7},
	}
	anonymous := []agentlib.ModelInfo{
		{ID: "any-model", Provider: "any-provider", Power: 9},
		{ID: "other-model", Provider: "other-provider", Power: 7},
	}
	assert.Equal(t, TopNPowerThreshold(named, 1), TopNPowerThreshold(anonymous, 1))
	assert.Equal(t, TopNPowerThreshold(named, 2), TopNPowerThreshold(anonymous, 2))
}
