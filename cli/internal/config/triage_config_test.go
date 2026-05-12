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
    review_block: [escalate_tier, operator_required]
    lock_contention: [retry_with_backoff, retry_with_backoff, operator_required]
`
	var cfg NewConfig
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	require.NotNil(t, cfg.Triage)
	assert.Equal(t,
		[]string{"escalate_tier", "operator_required"},
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
				"review_block": {"escalate_tier", "operator_required"},
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
				"review_block":    {"not_a_real_action", "operator_required"},
				"not_a_real_mode": {"escalate_tier"},
			},
		},
	}
	policy := cfg.ResolveTriagePolicy()
	// Unknown action filtered out; only operator_required remains for review_block.
	assert.Equal(t,
		triage.ActionOperatorRequired,
		policy.Decide("ddx-test", triage.FailureModeReviewBlock, nil))
}

func TestSchemaAcceptsTopLevelTriageBlock(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	yamlDoc := []byte(`version: "1.0"
triage:
  policies:
    review_block: [re_attempt_with_context, escalate_tier, operator_required]
`)
	require.NoError(t, v.Validate(yamlDoc))
}

func TestSchemaRejectsUnknownAndLegacyTriageActionAndMode(t *testing.T) {
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
    review_block: [needs_human]
`)))
	require.Error(t, v.Validate([]byte(`version: "1.0"
triage:
  policies:
    bogus_mode: [operator_required]
`)))
}

func TestConfigValidateRejectsLegacyNeedsHumanTriageAction(t *testing.T) {
	cfg := &NewConfig{
		Version: "1.0",
		Triage: &TriagePolicyConfig{
			Policies: map[string][]string{
				"review_block": {"needs_human"},
			},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triage.policies.review_block[0]")
	assert.Contains(t, err.Error(), "needs_human")
	assert.Contains(t, err.Error(), "operator_required")
}

func TestResolvedConfig_TriagePolicyAccessor(t *testing.T) {
	cfg := &NewConfig{
		Triage: &TriagePolicyConfig{
			Policies: map[string][]string{
				"review_block": {"operator_required"},
			},
		},
	}
	resolved := cfg.Resolve(CLIOverrides{})
	got := resolved.TriagePolicy().Decide("ddx-test", triage.FailureModeReviewBlock, nil)
	assert.Equal(t, triage.ActionOperatorRequired, got)
}

func TestSchemaAcceptsBeadQualityLintBlockThreshold(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	require.NoError(t, v.Validate([]byte(`version: "1.0"
bead-quality:
  mode: warn-only
  lint:
    block_threshold_score: 5
`)))
}

func TestResolvedConfig_BeadQualityLintBlockThresholdAccessor(t *testing.T) {
	cfg := &NewConfig{
		BeadQuality: &BeadQualityConfig{
			Mode: BeadQualityModeBlock,
			Lint: &BeadQualityLintConfig{
				BlockThresholdScore: intPtr(7),
			},
		},
	}
	resolved := cfg.Resolve(CLIOverrides{})
	assert.Equal(t, BeadQualityModeBlock, resolved.BeadQualityMode())
	assert.Equal(t, 7, resolved.BeadQualityLintBlockThresholdScore())
}

func intPtr(v int) *int { return &v }
