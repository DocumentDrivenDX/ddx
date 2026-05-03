package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// AC (ddx-b69f04f8): the spoke-mode CLI exposes --hub-address as the
// hub URL flag. Pin the name so a future rename triggers a test failure
// rather than silently breaking systemd units / scripts that use it.
func TestServerCommandExposesHubAddressFlag(t *testing.T) {
	f := &CommandFactory{WorkingDir: t.TempDir()}
	cmd := f.newServerCommand()

	flag := cmd.Flags().Lookup("hub-address")
	if flag == nil {
		t.Fatalf("server command missing --hub-address flag")
	}
	if flag.Value.Type() != "string" {
		t.Errorf("--hub-address should be a string flag, got %q", flag.Value.Type())
	}
	if flag.DefValue != "" {
		t.Errorf("--hub-address default should be empty, got %q", flag.DefValue)
	}

	// The legacy --hub flag must not exist; we renamed to --hub-address.
	if cmd.Flags().Lookup("hub") != nil {
		t.Errorf("legacy --hub flag should be removed in favour of --hub-address")
	}
}

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
