package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// TestResolveEvidenceCaps_DefaultsWhenNil verifies that an empty config
// resolves to the built-in defaults.
func TestResolveEvidenceCaps_DefaultsWhenNil(t *testing.T) {
	c := &NewConfig{Version: "1.0"}
	got := c.ResolveEvidenceCaps("any")
	want := evidence.DefaultCaps()
	if got != want {
		t.Errorf("default caps not returned: %+v vs %+v", got, want)
	}
}

// TestResolveEvidenceCaps_LoadedFromYAML loads a real .ddx/config.yaml
// containing project-level caps and a per-harness override and verifies
// the resolved caps for each harness.
func TestResolveEvidenceCaps_LoadedFromYAML(t *testing.T) {
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
evidence_caps:
  max_prompt_bytes: 1000
  max_diff_bytes: 500
  per_harness:
    reviewer:
      max_prompt_bytes: 200
    grader:
      max_diff_bytes: 100
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

	if cfg.EvidenceCaps == nil {
		t.Fatal("EvidenceCaps not populated")
	}

	// Default harness: project-level overrides only.
	caps := cfg.ResolveEvidenceCaps("")
	if caps.MaxPromptBytes != 1000 {
		t.Errorf("project MaxPromptBytes = %d, want 1000", caps.MaxPromptBytes)
	}
	if caps.MaxDiffBytes != 500 {
		t.Errorf("project MaxDiffBytes = %d, want 500", caps.MaxDiffBytes)
	}
	if caps.MaxInlinedFileBytes != evidence.DefaultMaxInlinedFileBytes {
		t.Errorf("unset cap not at default: %d", caps.MaxInlinedFileBytes)
	}

	// Reviewer override wins for MaxPromptBytes; project still applies elsewhere.
	caps = cfg.ResolveEvidenceCaps("reviewer")
	if caps.MaxPromptBytes != 200 {
		t.Errorf("reviewer MaxPromptBytes = %d, want 200", caps.MaxPromptBytes)
	}
	if caps.MaxDiffBytes != 500 {
		t.Errorf("reviewer MaxDiffBytes = %d, want 500 (project layer)", caps.MaxDiffBytes)
	}

	// Grader override applies to MaxDiffBytes only.
	caps = cfg.ResolveEvidenceCaps("grader")
	if caps.MaxDiffBytes != 100 {
		t.Errorf("grader MaxDiffBytes = %d, want 100", caps.MaxDiffBytes)
	}
	if caps.MaxPromptBytes != 1000 {
		t.Errorf("grader MaxPromptBytes = %d, want 1000 (project layer)", caps.MaxPromptBytes)
	}
}
