package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReviewMaxRetriesConfig covers FEAT-022 §14: the review_max_retries
// field in .ddx/config.yaml overrides the binary default (3) used by the
// execute-loop's bounded reviewer-retry policy. Verified end-to-end: the
// YAML loader populates the field, the resolver returns the override, and
// the loop respects it via the same accessor (DefaultReviewMaxRetries
// constant in cli/internal/agent kept in sync with this default).
func TestReviewMaxRetriesConfig(t *testing.T) {
	t.Run("default_when_unset", func(t *testing.T) {
		c := &NewConfig{Version: "1.0"}
		if got := c.ResolveReviewMaxRetries(); got != 3 {
			t.Errorf("default ResolveReviewMaxRetries = %d, want 3", got)
		}
	})

	t.Run("default_when_zero_or_negative", func(t *testing.T) {
		zero := 0
		neg := -7
		for _, v := range []*int{&zero, &neg} {
			c := &NewConfig{Version: "1.0", ReviewMaxRetries: v}
			if got := c.ResolveReviewMaxRetries(); got != 3 {
				t.Errorf("non-positive ReviewMaxRetries (%d) should fall back to 3, got %d", *v, got)
			}
		}
	})

	t.Run("override_loaded_from_yaml", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.MkdirAll(filepath.Join(tmp, ".ddx"), 0o755); err != nil {
			t.Fatal(err)
		}
		yaml := `version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: https://github.com/DocumentDrivenDX/ddx-library
    branch: main
review_max_retries: 5
`
		if err := os.WriteFile(filepath.Join(tmp, ".ddx", "config.yaml"), []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		loader, err := NewConfigLoaderWithWorkingDir(tmp)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := loader.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ReviewMaxRetries == nil {
			t.Fatal("ReviewMaxRetries was not populated from YAML")
		}
		if *cfg.ReviewMaxRetries != 5 {
			t.Errorf("loaded ReviewMaxRetries = %d, want 5", *cfg.ReviewMaxRetries)
		}
		if got := cfg.ResolveReviewMaxRetries(); got != 5 {
			t.Errorf("ResolveReviewMaxRetries = %d, want 5 (override must reach the loop)", got)
		}
	})
}
