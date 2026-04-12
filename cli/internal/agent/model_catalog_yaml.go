package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ModelCatalogYAML is the on-disk format for the model catalog.
// Stored at ~/.ddx/model-catalog.yaml.
// Drives tier→model assignments and pricing without requiring a rebuild.
type ModelCatalogYAML struct {
	Version   int                    `yaml:"version"`
	UpdatedAt time.Time              `yaml:"updated_at"`
	Tiers     map[string]TierDefYAML `yaml:"tiers"`  // tier name → surface→model
	Models    []ModelEntryYAML       `yaml:"models"` // per-model metadata and pricing
}

// TierDefYAML maps surfaces to concrete model strings for a tier.
type TierDefYAML struct {
	Description string            `yaml:"description"`
	Surfaces    map[string]string `yaml:"surfaces"` // surface → concrete model
}

// ModelEntryYAML holds metadata for one model.
type ModelEntryYAML struct {
	ID       string `yaml:"id"` // e.g. "claude-sonnet-4-6"
	Provider string `yaml:"provider"`
	Tier     string `yaml:"tier"`              // smart, standard, cheap
	Blocked  bool   `yaml:"blocked,omitempty"` // if true, routing never selects this model
	Notes    string `yaml:"notes,omitempty"`
}

// DefaultModelCatalogPath returns the default path for the model catalog YAML.
func DefaultModelCatalogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".ddx", "model-catalog.yaml")
}

// LoadModelCatalogYAML loads the model catalog from the given path.
// Returns nil (no error) if the file does not exist — callers fall back to built-in defaults.
func LoadModelCatalogYAML(path string) (*ModelCatalogYAML, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cat ModelCatalogYAML
	if err := yaml.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parse model catalog: %w", err)
	}
	return &cat, nil
}

// WriteModelCatalogYAML writes the catalog to the given path, creating parent dirs.
func WriteModelCatalogYAML(path string, cat *ModelCatalogYAML) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cat)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ApplyModelCatalogYAML overlays the YAML catalog onto a Catalog, adding or
// replacing entries for each tier defined in the YAML.
func ApplyModelCatalogYAML(cat *Catalog, yml *ModelCatalogYAML) {
	if yml == nil || cat == nil {
		return
	}
	for tierName, tierDef := range yml.Tiers {
		entry := CatalogEntry{
			Ref:      tierName,
			Surfaces: make(map[string]string, len(tierDef.Surfaces)),
		}
		for surface, model := range tierDef.Surfaces {
			entry.Surfaces[surface] = model
		}
		cat.AddOrReplace(entry)
	}
	for _, m := range yml.Models {
		if m.Blocked {
			cat.AddBlockedModelID(m.ID)
		}
	}
}

// DefaultModelCatalogYAML returns the built-in seed catalog.
// Used when no ~/.ddx/model-catalog.yaml exists yet.
func DefaultModelCatalogYAML() *ModelCatalogYAML {
	return &ModelCatalogYAML{
		Version:   1,
		UpdatedAt: time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
		Tiers: map[string]TierDefYAML{
			"smart": {
				Description: "Hard/broad tasks, user interactive sessions, HELIX document alignment",
				Surfaces: map[string]string{
					"codex":           "gpt-5.4",
					"claude":          "claude-opus-4-6",
					"embedded-openai": "minimax/minimax-m2.7",
				},
			},
			"standard": {
				Description: "Default for most builds — refactoring, feature work, test writing",
				Surfaces: map[string]string{
					"codex":           "gpt-5.4",
					"claude":          "claude-sonnet-4-6",
					"embedded-openai": "minimax/minimax-m2.7",
				},
			},
			"cheap": {
				Description: "Mechanical tasks — extraction, formatting, simple transforms",
				Surfaces: map[string]string{
					"codex":           "gpt-5.4-mini",
					"claude":          "claude-haiku-4-5",
					"embedded-openai": "qwen3.5-27b",
				},
			},
		},
		Models: []ModelEntryYAML{
			// Smart tier
			{ID: "claude-opus-4-6", Provider: "anthropic", Tier: "smart"},
			{ID: "gpt-5.4", Provider: "openai", Tier: "smart", Notes: "codex harness"},
			// Standard tier
			{ID: "claude-sonnet-4-6", Provider: "anthropic", Tier: "standard"},
			{ID: "minimax/minimax-m2.7", Provider: "minimax", Tier: "standard"},
			{ID: "minimax/minimax-m2.5", Provider: "minimax", Tier: "standard"},
			{ID: "moonshot/kimi-k2.5", Provider: "moonshot", Tier: "standard"},
			{ID: "openai/gpt-4.1", Provider: "openai", Tier: "standard"},
			{ID: "openai/gpt-oss-120b", Provider: "openai", Tier: "standard", Notes: "local inference on vidar"},
			// Cheap tier
			{ID: "claude-haiku-4-5", Provider: "anthropic", Tier: "cheap"},
			{ID: "gpt-5.4-mini", Provider: "openai", Tier: "cheap", Notes: "codex harness"},
			{ID: "qwen3.5-27b", Provider: "qwen", Tier: "cheap", Notes: "local inference on vidar"},
			{ID: "qwen/qwen3-coder-next", Provider: "qwen", Tier: "cheap", Notes: "local; 80B MoE (3B active)"},
			{ID: "openai/gpt-oss-20b", Provider: "openai", Tier: "cheap", Notes: "local inference on vidar"},
			// Blocked: deprecated/retired models — routing must never select these.
			{ID: "gpt-3.5-turbo", Provider: "openai", Tier: "cheap", Blocked: true, Notes: "retired; use cheap tier"},
			{ID: "gpt-3.5-turbo-16k", Provider: "openai", Tier: "cheap", Blocked: true, Notes: "retired; use cheap tier"},
			{ID: "claude-opus-4-5", Provider: "anthropic", Tier: "smart", Blocked: true, Notes: "superseded by claude-opus-4-6"},
			{ID: "claude-3-opus-20240229", Provider: "anthropic", Tier: "smart", Blocked: true, Notes: "superseded by claude-opus-4-6"},
			{ID: "claude-3-5-sonnet-20241022", Provider: "anthropic", Tier: "standard", Blocked: true, Notes: "superseded by claude-sonnet-4-6"},
		},
	}
}
