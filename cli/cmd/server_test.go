package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveTsnetAuthKey(t *testing.T) {
	tests := []struct {
		name      string
		envKey    string
		flagKey   string
		configKey string
		want      string
	}{
		{
			name:      "env var overrides CLI flag",
			envKey:    "env-key",
			flagKey:   "flag-key",
			configKey: "",
			want:      "env-key",
		},
		{
			name:      "env var overrides config file",
			envKey:    "env-key",
			flagKey:   "",
			configKey: "config-key",
			want:      "env-key",
		},
		{
			name:      "env var overrides both CLI flag and config file",
			envKey:    "env-key",
			flagKey:   "flag-key",
			configKey: "config-key",
			want:      "env-key",
		},
		{
			name:      "CLI flag overrides config file when env var absent",
			envKey:    "",
			flagKey:   "flag-key",
			configKey: "config-key",
			want:      "flag-key",
		},
		{
			name:      "config file used when env var and flag absent",
			envKey:    "",
			flagKey:   "",
			configKey: "config-key",
			want:      "config-key",
		},
		{
			name:      "empty result when all sources absent",
			envKey:    "",
			flagKey:   "",
			configKey: "",
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveTsnetAuthKey(tc.envKey, tc.flagKey, tc.configKey)
			assert.Equal(t, tc.want, got)
		})
	}
}
