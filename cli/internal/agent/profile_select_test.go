package agent

import (
	"testing"

	agentlib "github.com/DocumentDrivenDX/fizeau"
	"github.com/stretchr/testify/assert"
)

func TestSelectCheapestProfile_LowestBandWithAvailableModel(t *testing.T) {
	snap := testProfileSnapshot()
	assert.Equal(t, "cheap", SelectCheapestProfile(snap))
}

func TestSelectStrongestProfile_HighestBandWithAvailableModel(t *testing.T) {
	snap := testProfileSnapshot()
	assert.Equal(t, "smart", SelectStrongestProfile(snap))
}

func TestSelectStrongestProfileAbove_RespectsFloor(t *testing.T) {
	snap := testProfileSnapshot()
	assert.Equal(t, "smart", SelectStrongestProfileAbove(snap, 8))
	assert.Empty(t, SelectStrongestProfileAbove(snap, 11))
}

func TestSelectProfile_ReturnsEmptyWhenNothingSatisfies(t *testing.T) {
	snap := ProfileSnapshot{
		Profiles: []agentlib.ProfileInfo{{Name: "cheap", MinPower: 5, MaxPower: 5}},
		Models:   []agentlib.ModelInfo{{ID: "cheap", Power: 5, Available: false, AutoRoutable: true}},
	}
	assert.Empty(t, SelectCheapestProfile(snap))
	assert.Empty(t, SelectStrongestProfile(snap))
	assert.Empty(t, SelectStrongestProfileAbove(snap, 1))
}

func testProfileSnapshot() ProfileSnapshot {
	return ProfileSnapshot{
		Profiles: []agentlib.ProfileInfo{
			{Name: "standard", MinPower: 7, MaxPower: 8, ProviderPreference: "local-first"},
			{Name: "smart", MinPower: 9, MaxPower: 10, ProviderPreference: "subscription-first"},
			{Name: "cheap", MinPower: 5, MaxPower: 5, ProviderPreference: "local-first"},
		},
		Models: []agentlib.ModelInfo{
			{ID: "small", Power: 5, Available: true, AutoRoutable: true},
			{ID: "medium", Power: 8, Available: true, AutoRoutable: true},
			{ID: "large", Power: 9, Available: true, AutoRoutable: true},
		},
	}
}
