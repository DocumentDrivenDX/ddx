package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/evidence"
)

// TestResolveEvidenceCaps_DefaultsWhenNil verifies that an empty config
// resolves to the built-in defaults.
func TestResolveEvidenceCaps_DefaultsWhenNil(t *testing.T) {
	c := &NewConfig{Version: "1.0"}
	got := c.ResolveEvidenceCapsForRole(EvidenceRoleReviewer)
	want := evidence.DefaultCaps()
	if got != want {
		t.Errorf("default caps not returned: %+v vs %+v", got, want)
	}
}

// TestResolveEvidenceCaps_LoadedFromYAML loads a real .ddx/config.yaml
// containing project-level caps and semantic-role overrides. Route identity
// must never alter the resolved role caps.
func TestEvidenceCapsResolveOnlyByDDXRole(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ddxroot.DirName), 0o755); err != nil {
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
  per_role:
    implementer:
      max_diff_bytes: 100
    reviewer:
      max_prompt_bytes: 200
    lifecycle:
      max_inlined_file_bytes: 80
`
	if err := os.WriteFile(filepath.Join(tmp, ddxroot.DirName, "config.yaml"), []byte(yaml), 0o600); err != nil {
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

	// Unknown/empty roles receive project-level overrides only.
	caps := cfg.ResolveEvidenceCaps()
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
	caps = cfg.ResolveEvidenceCapsForRole(EvidenceRoleReviewer)
	if caps.MaxPromptBytes != 200 {
		t.Errorf("reviewer MaxPromptBytes = %d, want 200", caps.MaxPromptBytes)
	}
	if caps.MaxDiffBytes != 500 {
		t.Errorf("reviewer MaxDiffBytes = %d, want 500 (project layer)", caps.MaxDiffBytes)
	}

	// Implementer and lifecycle overrides are independent.
	caps = cfg.ResolveEvidenceCapsForRole(EvidenceRoleImplementer)
	if caps.MaxDiffBytes != 100 {
		t.Errorf("implementer MaxDiffBytes = %d, want 100", caps.MaxDiffBytes)
	}
	if caps.MaxPromptBytes != 1000 {
		t.Errorf("implementer MaxPromptBytes = %d, want 1000 (project layer)", caps.MaxPromptBytes)
	}
	caps = cfg.ResolveEvidenceCapsForRole(EvidenceRoleLifecycle)
	if caps.MaxInlinedFileBytes != 80 {
		t.Errorf("lifecycle MaxInlinedFileBytes = %d, want 80", caps.MaxInlinedFileBytes)
	}

	// Harness, provider, and model overrides describe a Fizeau request. They
	// cannot affect any DDx role's evidence budget.
	baseline := cfg.Resolve(CLIOverrides{})
	for _, route := range []CLIOverrides{
		{Harness: "claude", Provider: "anthropic", Model: "opus"},
		{Harness: "codex", Provider: "openai", Model: "gpt"},
	} {
		resolved := cfg.Resolve(route)
		for _, role := range []string{EvidenceRoleImplementer, EvidenceRoleReviewer, EvidenceRoleLifecycle} {
			want := baseline.EvidenceCapsForRole(role)
			if got := resolved.EvidenceCapsForRole(role); got != want {
				t.Fatalf("route %+v changed %s caps: got %+v, want %+v", route, role, got, want)
			}
		}
	}
}

func TestEvidenceCapsSchemaAllowsOnlyDDXRoles(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ddxroot.DirName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	contents := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.invalid/library
    branch: main
evidence_caps:
  per_role:
    claude:
      max_prompt_bytes: 100
`
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := NewConfigLoaderWithWorkingDir(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loader.LoadConfig()
	if err == nil {
		t.Fatal("expected schema rejection for route-keyed per_role entry")
	}
	if got := err.Error(); !strings.Contains(got, "claude") && !strings.Contains(got, "additionalProperties") {
		t.Fatalf("schema error does not identify invalid role key: %v", err)
	}
}
