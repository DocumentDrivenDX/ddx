package agent

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/escalation"
	"github.com/stretchr/testify/assert"
)

func TestResolveProfileLadderDefaults(t *testing.T) {
	assert.Equal(t,
		[]escalation.ModelTier{"cheap", "standard", "smart"},
		ResolveProfileLadder(nil, "default", "", ""),
	)
	assert.Equal(t,
		[]escalation.ModelTier{"cheap"},
		ResolveProfileLadder(nil, "cheap", "", ""),
	)
	assert.Equal(t,
		[]escalation.ModelTier{"fast", "smart"},
		ResolveProfileLadder(nil, "fast", "", ""),
	)
	assert.Equal(t,
		[]escalation.ModelTier{"smart"},
		ResolveProfileLadder(nil, "smart", "", ""),
	)
}

func TestResolveProfileLadderMaxTierCapsDefault(t *testing.T) {
	assert.Equal(t,
		[]escalation.ModelTier{"cheap", "standard"},
		ResolveProfileLadder(nil, "default", "", "standard"),
	)
}

func TestResolveProfileLadderCheapNeverEscalates(t *testing.T) {
	assert.Equal(t,
		[]escalation.ModelTier{"cheap"},
		ResolveProfileLadder(nil, "cheap", "", "smart"),
	)
}

func TestResolveProfileLadderUsesConfiguredLadderAndLegacyFallback(t *testing.T) {
	routing := &config.RoutingConfig{
		ProfileLadders: map[string][]string{
			"cheap": {"cheap"},
		},
		ProfilePriority: []string{"standard"},
	}
	assert.Equal(t,
		[]escalation.ModelTier{"standard"},
		ResolveProfileLadder(routing, "default", "", ""),
	)
	assert.Equal(t,
		[]escalation.ModelTier{"smart"},
		ResolveProfileLadder(routing, "smart", "", ""),
	)
}

func TestResolveTierModelRefUsesOverrides(t *testing.T) {
	routing := &config.RoutingConfig{
		ModelOverrides: map[string]string{
			"standard": "codex/gpt-5.4",
		},
	}
	assert.Equal(t, "codex/gpt-5.4", ResolveTierModelRef(routing, "standard"))
	assert.Equal(t, "cheap", ResolveTierModelRef(routing, "cheap"))
}

func TestProfileLadderExecutionPaths(t *testing.T) {
	cases := []struct {
		name     string
		profile  string
		maxTier  string
		statuses map[escalation.ModelTier]string
		want     []escalation.ModelTier
		final    escalation.ModelTier
	}{
		{
			name:     "success at cheap",
			profile:  "default",
			statuses: map[escalation.ModelTier]string{"cheap": ExecuteBeadStatusSuccess},
			want:     []escalation.ModelTier{"cheap"},
			final:    "cheap",
		},
		{
			name:    "escalate cheap to standard",
			profile: "default",
			statuses: map[escalation.ModelTier]string{
				"cheap":    ExecuteBeadStatusExecutionFailed,
				"standard": ExecuteBeadStatusSuccess,
			},
			want:  []escalation.ModelTier{"cheap", "standard"},
			final: "standard",
		},
		{
			name:    "escalate all tiers then exhaustion",
			profile: "default",
			statuses: map[escalation.ModelTier]string{
				"cheap":    ExecuteBeadStatusExecutionFailed,
				"standard": ExecuteBeadStatusLandConflict,
				"smart":    ExecuteBeadStatusPostRunCheckFailed,
			},
			want:  []escalation.ModelTier{"cheap", "standard", "smart"},
			final: "smart",
		},
		{
			name:    "max tier caps ladder",
			profile: "default",
			maxTier: "standard",
			statuses: map[escalation.ModelTier]string{
				"cheap":    ExecuteBeadStatusExecutionFailed,
				"standard": ExecuteBeadStatusExecutionFailed,
			},
			want:  []escalation.ModelTier{"cheap", "standard"},
			final: "standard",
		},
		{
			name:    "cheap profile never escalates",
			profile: "cheap",
			statuses: map[escalation.ModelTier]string{
				"cheap": ExecuteBeadStatusExecutionFailed,
			},
			want:  []escalation.ModelTier{"cheap"},
			final: "cheap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, final := simulateProfileLadder(tc.profile, tc.maxTier, tc.statuses)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.final, final)
		})
	}
}

func TestDefaultProfileRepresentativeRunMostlyClosesAtCheap(t *testing.T) {
	representative := []map[escalation.ModelTier]string{
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusExecutionFailed, "standard": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusPostRunCheckFailed, "standard": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusLandConflict, "standard": ExecuteBeadStatusExecutionFailed, "smart": ExecuteBeadStatusSuccess},
		{"cheap": ExecuteBeadStatusStructuralValidationFailed, "standard": ExecuteBeadStatusSuccess},
	}

	cheapClosures := 0
	for _, statuses := range representative {
		_, final := simulateProfileLadder("default", "", statuses)
		if final == escalation.TierCheap {
			cheapClosures++
		}
	}

	assert.GreaterOrEqual(t, cheapClosures, len(representative)/2)
}

func simulateProfileLadder(profile, maxTier string, statuses map[escalation.ModelTier]string) ([]escalation.ModelTier, escalation.ModelTier) {
	ladder := ResolveProfileLadder(nil, profile, "", maxTier)
	attempted := []escalation.ModelTier{}
	var final escalation.ModelTier
	for _, tier := range ladder {
		attempted = append(attempted, tier)
		final = tier
		status := statuses[tier]
		if status == "" || status == ExecuteBeadStatusSuccess || !escalation.ShouldEscalate(status) {
			break
		}
	}
	return attempted, final
}
