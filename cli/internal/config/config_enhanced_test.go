package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidate exercises (*Config).Validate against the current schema
// (NewConfig with Library.Repository).
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Version: "1.0",
				Library: &LibraryConfig{
					Path: ".ddx/plugins/ddx",
					Repository: &RepositoryConfig{
						URL:    "https://github.com/test/repo",
						Branch: "main",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config without library block",
			config: &Config{
				Version: "1.0",
			},
			wantErr: false,
		},
		{
			name: "missing version",
			config: &Config{
				Library: &LibraryConfig{
					Repository: &RepositoryConfig{
						URL:    "https://github.com/test/repo",
						Branch: "main",
					},
				},
			},
			wantErr: true,
			errMsg:  "version is required",
		},
		{
			name: "missing library repository URL",
			config: &Config{
				Version: "1.0",
				Library: &LibraryConfig{
					Repository: &RepositoryConfig{
						Branch: "main",
					},
				},
			},
			wantErr: true,
			errMsg:  "library repository URL is required",
		},
		{
			name: "missing library repository branch",
			config: &Config{
				Version: "1.0",
				Library: &LibraryConfig{
					Repository: &RepositoryConfig{
						URL: "https://github.com/test/repo",
					},
				},
			},
			wantErr: true,
			errMsg:  "library repository branch is required",
		},
		{
			name: "multiple errors",
			config: &Config{
				Library: &LibraryConfig{
					Repository: &RepositoryConfig{},
				},
			},
			wantErr: true,
			errMsg:  "configuration errors found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateResolve covers the Resolve path's validation/normalisation
// behaviour: defaults are applied for absent config blocks, overrides take
// precedence, and a nil config still produces a sealed ResolvedConfig
// populated from package defaults.
func TestValidateResolve(t *testing.T) {
	t.Run("nil config resolves with defaults", func(t *testing.T) {
		var cfg *NewConfig
		rcfg := cfg.Resolve(CLIOverrides{})

		assert.Equal(t, 3, rcfg.ReviewMaxRetries(), "default review retries")
		assert.Equal(t, 6*time.Hour, rcfg.NoProgressCooldown(), "default no-progress cooldown")
		assert.Equal(t, 3, rcfg.MaxNoChangesBeforeClose(), "default max no_changes")
		assert.Equal(t, 30*time.Second, rcfg.HeartbeatInterval(), "default heartbeat")
		assert.Empty(t, rcfg.Harness())
		assert.Empty(t, rcfg.ContextBudget())
	})

	t.Run("agent config supplies harness/model fallback", func(t *testing.T) {
		cfg := &NewConfig{
			Version: "1.0",
			Agent: &AgentConfig{
				Harness:     "claude",
				Model:       "claude-opus",
				Permissions: "ask",
				TimeoutMS:   5000,
			},
		}
		rcfg := cfg.Resolve(CLIOverrides{})

		assert.Equal(t, "claude", rcfg.Harness())
		assert.Equal(t, "claude-opus", rcfg.Model())
		assert.Equal(t, "ask", rcfg.Permissions())
		assert.Equal(t, 5*time.Second, rcfg.Timeout())
	})

	t.Run("CLI overrides win over config", func(t *testing.T) {
		cfg := &NewConfig{
			Version: "1.0",
			Agent: &AgentConfig{
				Harness:     "claude",
				Model:       "claude-opus",
				Permissions: "ask",
			},
			EvidenceCaps: &EvidenceCapsConfig{
				ContextBudget: "minimal",
			},
		}
		timeout := 30 * time.Second
		rcfg := cfg.Resolve(CLIOverrides{
			Harness:       "codex",
			Model:         "gpt-5",
			Permissions:   "yolo",
			Timeout:       &timeout,
			ContextBudget: "full",
			Profile:       "fast",
			Assignee:      "user@example.com",
		})

		assert.Equal(t, "codex", rcfg.Harness())
		assert.Equal(t, "gpt-5", rcfg.Model())
		assert.Equal(t, "yolo", rcfg.Permissions())
		assert.Equal(t, 30*time.Second, rcfg.Timeout())
		assert.Equal(t, "full", rcfg.ContextBudget())
		assert.Equal(t, "fast", rcfg.Profile())
		assert.Equal(t, "user@example.com", rcfg.Assignee())
	})

	t.Run("evidence caps context budget falls through when no override", func(t *testing.T) {
		cfg := &NewConfig{
			Version: "1.0",
			EvidenceCaps: &EvidenceCapsConfig{
				ContextBudget: "minimal",
			},
		}
		rcfg := cfg.Resolve(CLIOverrides{})
		assert.Equal(t, "minimal", rcfg.ContextBudget())
	})

	t.Run("workers config resolves cooldown and heartbeat", func(t *testing.T) {
		maxNoChanges := 7
		cfg := &NewConfig{
			Version: "1.0",
			Workers: &WorkersConfig{
				NoProgressCooldown:      "2h",
				MaxNoChangesBeforeClose: &maxNoChanges,
				HeartbeatInterval:       "15s",
			},
		}
		rcfg := cfg.Resolve(CLIOverrides{})

		assert.Equal(t, 2*time.Hour, rcfg.NoProgressCooldown())
		assert.Equal(t, 7, rcfg.MaxNoChangesBeforeClose())
		assert.Equal(t, 15*time.Second, rcfg.HeartbeatInterval())
	})

	t.Run("review max retries override", func(t *testing.T) {
		retries := 5
		cfg := &NewConfig{
			Version:          "1.0",
			ReviewMaxRetries: &retries,
		}
		rcfg := cfg.Resolve(CLIOverrides{})
		assert.Equal(t, 5, rcfg.ReviewMaxRetries())
	})

	t.Run("zero-value ResolvedConfig panics on access", func(t *testing.T) {
		assert.Panics(t, func() {
			var r ResolvedConfig
			_ = r.Harness()
		})
	})
}
