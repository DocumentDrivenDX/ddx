package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAutoRemediationsDefaultTrue covers AC #2 (config): the
// work.autoRemediations toggles default to true when unset, both at the
// WorkersConfig resolver layer and on the ResolvedConfig.
func TestAutoRemediationsDefaultTrue(t *testing.T) {
	// Nil receiver and nil AutoRemediations both default to true.
	var nilWorkers *WorkersConfig
	assert.True(t, nilWorkers.ResolveAutoSupersedeClose())
	assert.True(t, nilWorkers.ResolveAutoEpicDecompose())
	assert.True(t, nilWorkers.ResolveAutoClosureReclassify())

	empty := &WorkersConfig{}
	assert.True(t, empty.ResolveAutoSupersedeClose())
	assert.True(t, empty.ResolveAutoEpicDecompose())
	assert.True(t, empty.ResolveAutoClosureReclassify())

	// Resolved default config surfaces the same defaults.
	rcfg := DefaultNewConfig().Resolve(CLIOverrides{})
	assert.True(t, rcfg.AutoSupersedeClose())
	assert.True(t, rcfg.AutoEpicDecompose())
	assert.True(t, rcfg.AutoClosureReclassify())
}

// TestAutoRemediationsExplicitFalse confirms an explicit false in config is
// honored (per-project config can disable a remediation).
func TestAutoRemediationsExplicitFalse(t *testing.T) {
	no := false
	w := &WorkersConfig{AutoRemediations: &AutoRemediationsConfig{
		AutoSupersedeClose:    &no,
		AutoEpicDecompose:     &no,
		AutoClosureReclassify: &no,
	}}
	assert.False(t, w.ResolveAutoSupersedeClose())
	assert.False(t, w.ResolveAutoEpicDecompose())
	assert.False(t, w.ResolveAutoClosureReclassify())
}
