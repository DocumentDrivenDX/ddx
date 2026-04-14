package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SharedCatalogPath returns the standard path for the shared ddx-agent catalog.
// ddx-agent installs and updates this file as part of its distribution.
// Returns empty string if the home directory cannot be determined.
func SharedCatalogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "agent", "models.yaml")
}

// sharedManifest is the on-disk representation of the shared ddx-agent catalog
// (schema versions 2–3). DDx reads this file but never writes it. The file is
// authoritative for tier/profile/alias assignments and surface projections.
type sharedManifest struct {
	Version        int                      `yaml:"version"`
	GeneratedAt    string                   `yaml:"generated_at"`
	CatalogVersion string                   `yaml:"catalog_version,omitempty"`
	Profiles       map[string]sharedProfile `yaml:"profiles"`
	Targets        map[string]sharedTarget  `yaml:"targets"`
}

type sharedProfile struct {
	Target string `yaml:"target"`
}

type sharedTarget struct {
	Family      string            `yaml:"family"`
	Aliases     []string          `yaml:"aliases,omitempty"`
	Status      string            `yaml:"status"`
	Replacement string            `yaml:"replacement,omitempty"`
	Surfaces    map[string]string `yaml:"surfaces"`
}

// sharedSurfaceToDDx translates ddx-agent surface names to DDx catalog surface
// names. agent.anthropic is absent: the embedded agent harness in DDx uses the
// "embedded-openai" surface regardless of which provider backend the agent
// runtime selects internally. DDx resolves the model hint from agent.openai;
// internal provider/backend choice belongs to the embedded runtime.
var sharedSurfaceToDDx = map[string]string{
	"agent.openai": "embedded-openai",
	"claude-code":  "claude",
	"codex":        "codex",
}

// maxSharedManifestVersion is the highest catalog schema version DDx can consume.
const maxSharedManifestVersion = 3

// LoadSharedCatalog reads the shared ddx-agent catalog from path.
// Returns nil with no error if the file does not exist — callers fall back to
// built-in defaults. Returns an error if the file exists but is malformed or
// uses an unsupported schema version.
func LoadSharedCatalog(path string) (*sharedManifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("shared catalog: read %s: %w", path, err)
	}
	var m sharedManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("shared catalog: parse %s: %w", path, err)
	}
	if m.Version <= 0 || m.Version > maxSharedManifestVersion {
		return nil, fmt.Errorf("shared catalog: %s: unsupported schema version %d (supported 1..%d)",
			path, m.Version, maxSharedManifestVersion)
	}
	return &m, nil
}

// ApplySharedCatalog overlays the shared ddx-agent catalog onto a DDx Catalog.
//
// All profiles, target IDs, and target aliases are translated to DDx surface
// names and written via AddOrReplace. Deprecated targets carry Deprecated=true
// and their replacement ref.
//
// The shared catalog is authoritative for aliases, profiles, canonical targets,
// and surface projections when present. The built-in DDx catalog remains the
// deterministic fallback when ~/.config/agent/models.yaml is absent or
// unreadable — in that case routing continues with the DefaultModelCatalogYAML
// seed for smart/standard/cheap tiers without any other behaviour change.
func ApplySharedCatalog(cat *Catalog, m *sharedManifest) {
	if m == nil || cat == nil {
		return
	}

	// Apply target-ID entries (target ID = stable canonical ref).
	for targetID, target := range m.Targets {
		if entry := sharedTargetToEntry(targetID, target); len(entry.Surfaces) > 0 || entry.Deprecated {
			cat.AddOrReplace(entry)
		}
	}

	// Apply profile entries. A profile may use a different name than its target
	// (e.g. "smart" → "code-high"), so each profile name gets its own entry with
	// the same surface mappings as the referenced target.
	for profile, p := range m.Profiles {
		target, ok := m.Targets[p.Target]
		if !ok {
			continue
		}
		if entry := sharedTargetToEntry(profile, target); len(entry.Surfaces) > 0 || entry.Deprecated {
			cat.AddOrReplace(entry)
		}
	}

	// Apply alias entries.
	for _, target := range m.Targets {
		for _, alias := range target.Aliases {
			if entry := sharedTargetToEntry(alias, target); len(entry.Surfaces) > 0 || entry.Deprecated {
				cat.AddOrReplace(entry)
			}
		}
	}
}

// sharedTargetToEntry converts a sharedTarget to a DDx CatalogEntry using ref
// as the entry name. When both agent.openai and agent.anthropic are present,
// agent.openai wins for the embedded-openai DDx surface because the agent harness
// surface in DDx is "embedded-openai" (OpenAI-compatible backend path).
func sharedTargetToEntry(ref string, t sharedTarget) CatalogEntry {
	surfaces := make(map[string]string)

	// First: map non-agent.openai sources (agent.openai handled below to ensure priority).
	for src, model := range t.Surfaces {
		dst, ok := sharedSurfaceToDDx[src]
		if !ok || src == "agent.openai" {
			continue
		}
		surfaces[dst] = model
	}
	// Second: agent.openai unconditionally sets embedded-openai, overriding any
	// prior value (e.g. from agent.anthropic if it were mapped).
	if model, ok := t.Surfaces["agent.openai"]; ok {
		surfaces["embedded-openai"] = model
	}

	status := strings.ToLower(strings.TrimSpace(t.Status))
	deprecated := status != "" && status != "active"

	return CatalogEntry{
		Ref:        ref,
		Surfaces:   surfaces,
		Deprecated: deprecated,
		ReplacedBy: t.Replacement,
	}
}
