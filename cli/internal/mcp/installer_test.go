package mcp_test

import (
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/mcp"
	"github.com/stretchr/testify/assert"
)

// TestInstaller tests have been removed as they tested the old config file approach
// The installer now uses Claude CLI integration which is tested through acceptance tests

func TestValidator(t *testing.T) {
	v := mcp.NewValidator()

	t.Run("validate server name", func(t *testing.T) {
		tests := []struct {
			name    string
			input   string
			wantErr bool
		}{
			{"valid", "github", false},
			{"with hyphen", "github-enterprise", false},
			{"numbers", "server123", false},
			{"empty", "", true},
			{"uppercase", "GitHub", true},
			{"spaces", "git hub", true},
			{"path traversal", "../etc/passwd", true},
			{"path separator", "servers/github", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.ValidateServerName(tt.input)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validate environment", func(t *testing.T) {
		tests := []struct {
			name    string
			env     map[string]string
			wantErr bool
		}{
			{"valid", map[string]string{"TOKEN": "value"}, false},
			{"underscore", map[string]string{"API_TOKEN": "value"}, false},
			{"lowercase key", map[string]string{"token": "value"}, true},
			{"shell injection", map[string]string{"TOKEN": "$(rm -rf /)"}, true},
			{"backticks", map[string]string{"TOKEN": "`echo hacked`"}, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.ValidateEnvironment(tt.env)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validate path", func(t *testing.T) {
		tests := []struct {
			name    string
			path    string
			wantErr bool
		}{
			{"absolute", "/home/user/config.json", false},
			{"relative", "config.json", true},
			{"path traversal", "/home/../etc/passwd", true},
			{"double dots", "/home/user/../../../etc", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := v.ValidatePath(tt.path)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		sensitive bool
		expected  string
	}{
		{"not sensitive", "regular", false, "regular"},
		{"sensitive long", "ghp_secrettoken123", true, "ghp_***"},
		{"sensitive short", "secret", true, "***"},
		{"empty", "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mcp.MaskSensitive(tt.value, tt.sensitive)
			assert.Equal(t, tt.expected, result)
		})
	}
}
