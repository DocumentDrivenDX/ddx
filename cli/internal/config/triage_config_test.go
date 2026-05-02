package config

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/triage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNewConfigParsesTopLevelTriageBlock(t *testing.T) {
	raw := `
version: "1.0"
triage:
  policies:
    review_block: [escalate_tier, needs_human]
    lock_contention: [retry_with_backoff, retry_with_backoff, needs_human]
`
	var cfg NewConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.NotNil(t, cfg.Triage)
	assert.Equal(t,
		[]string{"escalate_tier", "needs_human"},
		cfg.Triage.Policies["review_block"])
}

func TestResolveTriagePolicy_DefaultsWhenAbsent(t *testing.T) {
	var cfg NewConfig
	policy := cfg.ResolveTriagePolicy()
	got := policy.Decide("ddx-test", triage.FailureModeReviewBlock, nil)
	assert.Equal(t, triage.ActionReAttemptWithContext, got)
}

func TestResolveTriagePolicy_ConfigOverridesDefault(t *testing.T) {
	cfg := &NewConfig{
		Triage: &TriagePolicyConfig{
			Policies: map[string][]string{
				"review_block": {"escalate_tier", "needs_human"},
			},
		},
	}
	policy := cfg.ResolveTriagePolicy()
	// review_block first rung is now escalate_tier (overridden).
	assert.Equal(t,
		triage.ActionEscalateTier,
		policy.Decide("ddx-test", triage.FailureModeReviewBlock, nil))
	// Other modes still inherit defaults.
	assert.Equal(t,
		triage.ActionRetryWithBackoff,
		policy.Decide("ddx-test", triage.FailureModeLockContention, nil))
}

func TestResolveTriagePolicy_DropsUnknownNames(t *testing.T) {
	cfg := &NewConfig{
		Triage: &TriagePolicyConfig{
			Policies: map[string][]string{
				"review_block":    {"not_a_real_action", "needs_human"},
				"not_a_real_mode": {"escalate_tier"},
			},
		},
	}
	policy := cfg.ResolveTriagePolicy()
	// Unknown action filtered out; only needs_human remains for review_block.
	assert.Equal(t,
		triage.ActionNeedsHuman,
		policy.Decide("ddx-test", triage.FailureModeReviewBlock, nil))
}

func TestSchemaAcceptsTopLevelTriageBlock(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	yamlDoc := []byte(`version: "1.0"
triage:
  policies:
    review_block: [re_attempt_with_context, escalate_tier, needs_human]
`)
	require.NoError(t, v.Validate(yamlDoc))
}

func TestSchemaRejectsUnknownTriageActionAndMode(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	require.Error(t, v.Validate([]byte(`version: "1.0"
triage:
  policies:
    review_block: [bogus_action]
`)))
	require.Error(t, v.Validate([]byte(`version: "1.0"
triage:
  policies:
    bogus_mode: [needs_human]
`)))
}

func TestResolvedConfig_TriagePolicyAccessor(t *testing.T) {
	cfg := &NewConfig{
		Triage: &TriagePolicyConfig{
			Policies: map[string][]string{
				"review_block": {"needs_human"},
			},
		},
	}
	resolved := cfg.Resolve(CLIOverrides{})
	got := resolved.TriagePolicy().Decide("ddx-test", triage.FailureModeReviewBlock, nil)
	assert.Equal(t, triage.ActionNeedsHuman, got)
}
