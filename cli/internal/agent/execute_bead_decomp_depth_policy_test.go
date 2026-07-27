package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/config"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecompositionDepthPolicyPromptUsesResolvedConfig proves that
// executeBeadDynamicStep0Hints renders the same depth number returned by
// ResolvedConfig.MaxDecompositionDepth() for unset, zero, and explicit
// agent.triage.max_decomposition_depth values — so the implementer prompt
// cannot drift from the queue-level policy.
func TestDecompositionDepthPolicyPromptUsesResolvedConfig(t *testing.T) {
	t.Parallel()

	// zero: non-positive pointer resolves to the binary default. Schema
	// minimum is 1, so zero is not loadable from YAML; exercise the
	// ResolveMaxDecompositionDepth branch directly, then confirm the
	// prompt for a workDir without the field still matches that default.
	zero := 0
	zeroCfg := &config.NewConfig{
		Agent: &config.AgentConfig{
			Triage: &config.TriageConfig{MaxDecompositionDepth: &zero},
		},
	}
	zeroResolved := zeroCfg.Resolve(config.CLIOverrides{}).MaxDecompositionDepth()
	assert.Equal(t, config.DefaultMaxDecompositionDepth, zeroResolved,
		"zero max_decomposition_depth must resolve to DefaultMaxDecompositionDepth")
	assert.LessOrEqual(t, zeroResolved, 2,
		"default/zero policy must be at most two generated child levels")

	cases := []struct {
		name           string
		yaml           string // empty means no config file (unset)
		wantExact      int    // when non-zero, resolved depth must equal this
		assertAtMostTwo bool
	}{
		{
			name:           "unset",
			yaml:           "",
			assertAtMostTwo: true,
		},
		{
			// "zero" workDir has no explicit field; resolver-side zero is
			// covered above. Prompt must still state the same default the
			// zero branch resolves to.
			name:           "zero",
			yaml:           "",
			wantExact:      zeroResolved,
			assertAtMostTwo: true,
		},
		{
			name: "explicit",
			yaml: `version: "1.0"
agent:
  triage:
    max_decomposition_depth: 5
`,
			wantExact: 5,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workDir := t.TempDir()
			if tc.yaml != "" {
				ddxDir := filepath.Join(workDir, ddxroot.DirName)
				require.NoError(t, os.MkdirAll(ddxDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(ddxDir, "config.yaml"), []byte(tc.yaml), 0o644))
			}

			rcfg, err := config.LoadAndResolve(workDir, config.CLIOverrides{})
			// LoadAndResolve may return an error when config is absent; the
			// sealed defaults-backed ResolvedConfig is still usable.
			if tc.yaml != "" {
				require.NoError(t, err)
			}
			want := rcfg.MaxDecompositionDepth()
			require.Greater(t, want, 0, "resolved max depth must be positive")
			if tc.assertAtMostTwo {
				assert.LessOrEqual(t, want, 2,
					"default/unset policy must be at most two generated child levels")
				assert.Equal(t, config.DefaultMaxDecompositionDepth, want,
					"unset/zero must resolve to DefaultMaxDecompositionDepth")
			}
			if tc.wantExact > 0 {
				assert.Equal(t, tc.wantExact, want,
					"resolved depth must match expected policy value")
			}

			b := &bead.Bead{ID: "ddx-depth-policy-test", Title: "depth policy prompt"}
			hints := executeBeadDynamicStep0Hints(workDir, b)
			require.NotEmpty(t, hints, "dynamic step0 hints must include the depth policy")

			// Prompt must state the resolved number (not a prompt-only hard-coded cap).
			needle := fmt.Sprintf("Auto-decomposition is capped at depth %d", want)
			assert.Contains(t, hints, needle,
				"executeBeadDynamicStep0Hints must render ResolvedConfig.MaxDecompositionDepth()=%d", want)
			assert.Contains(t, hints, "agent.triage.max_decomposition_depth")

			// Guard against reintroducing an independent prompt-only constant by
			// ensuring no other "capped at depth N" appears with a different N.
			for other := 1; other <= 10; other++ {
				if other == want {
					continue
				}
				bad := fmt.Sprintf("Auto-decomposition is capped at depth %d", other)
				assert.NotContains(t, hints, bad,
					"prompt must not state a different hard-coded depth cap")
			}

			// Sanity: the helper used by the prompt matches the same accessor.
			assert.Equal(t, want, resolveMaxDecompositionDepthForPrompt(workDir))
		})
	}
}
