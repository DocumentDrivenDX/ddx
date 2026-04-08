package agent

import (
	"fmt"
	"strings"
)

// ModelTier represents a quality/cost tier for model selection.
type ModelTier string

const (
	TierFast      ModelTier = "fast"      // good quality, lower cost/latency
	TierSmart     ModelTier = "smart"     // highest quality, higher cost/latency
	TierReasoning ModelTier = "reasoning" // deep chain-of-thought, maximum reasoning effort
)

// Preset is a named execution policy that maps to concrete model + effort + timeout.
type Preset string

const (
	PresetFast      Preset = "fast"
	PresetSmart     Preset = "smart"
	PresetReasoning Preset = "reasoning"
)

// PresetConfigs returns the canonical preset list.
func PresetConfigs() []Preset { return []Preset{PresetFast, PresetSmart, PresetReasoning} }

// PresetDefinition is the concrete resolved config for a preset on a given harness.
type PresetDefinition struct {
	Preset      Preset `json:"preset"`
	Harness     string `json:"harness,omitempty"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	TimeoutMS   int    `json:"timeout_ms,omitempty"`
	Permissions string `json:"permissions"`
}

// DefaultPresetConfigs maps harness → preset → PresetDefinition.
var DefaultPresetConfigs = map[string]map[Preset]PresetDefinition{
	"codex": {
		PresetFast:      {Preset: PresetFast, Model: "gpt-5.4-mini", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "gpt-5.4", Effort: "high", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "gpt-5.4", Effort: "high", Permissions: "unrestricted"},
	},
	"claude": {
		PresetFast:      {Preset: PresetFast, Model: "claude-sonnet-4-6", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "claude-opus-4-6", Effort: "high", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "claude-opus-4-6", Effort: "high", Permissions: "unrestricted"},
	},
	"forge": {
		PresetFast:      {Preset: PresetFast, Model: "qwen3.5-coder-32b", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "qwen/qwen3-coder-next", Effort: "medium", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "qwen3.5-coder-32b", Effort: "medium", Permissions: "unrestricted"},
	},
	"pi": {
		PresetFast:      {Preset: PresetFast, Model: "", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "", Effort: "high", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "", Effort: "high", Permissions: "unrestricted"},
	},
	"gemini": {
		PresetFast:      {Preset: PresetFast, Model: "", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "", Effort: "high", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "", Effort: "high", Permissions: "unrestricted"},
	},
	"opencode": {
		PresetFast:      {Preset: PresetFast, Model: "", Effort: "low", Permissions: "unrestricted"},
		PresetSmart:     {Preset: PresetSmart, Model: "", Effort: "high", Permissions: "unrestricted"},
		PresetReasoning: {Preset: PresetReasoning, Model: "", Effort: "high", Permissions: "unrestricted"},
	},
}

// ResolvePreset returns the preset definition for a harness and preset.
func ResolvePreset(harness string, preset Preset) (PresetDefinition, error) {
	configs, ok := DefaultPresetConfigs[harness]
	if !ok {
		return PresetDefinition{}, fmt.Errorf("unknown harness: %q (known: %s)", harness, knownHarnesses())
	}
	def, ok := configs[preset]
	if !ok {
		return PresetDefinition{}, fmt.Errorf("unknown preset: %q (known: %s)", preset, strings.Join(presetStrings(), ", "))
	}
	return def, nil
}

func knownHarnesses() string {
	var names []string
	for name := range DefaultPresetConfigs {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func presetStrings() []string {
	return []string{string(PresetFast), string(PresetSmart), string(PresetReasoning)}
}

// DefaultModelTiers maps harness → tier → model.
// These are the current production defaults as of 2026-04.
var DefaultModelTiers = map[string]map[ModelTier]string{
	"codex": {
		TierSmart: "gpt-5.4",
		TierFast:  "gpt-5.4-mini",
	},
	"claude": {
		TierSmart: "claude-opus-4-6",
		TierFast:  "claude-sonnet-4-6",
	},
	"forge": {
		TierSmart: "qwen/qwen3-coder-next",
		TierFast:  "qwen3.5-27b",
	},
	"opencode": {
		TierSmart: "anthropic/claude-opus-4-6",
		TierFast:  "anthropic/claude-sonnet-4-6",
	},
}

// ResolveModelTier returns the model for a given harness and tier.
func ResolveModelTier(harness string, tier ModelTier) string {
	if tiers, ok := DefaultModelTiers[harness]; ok {
		if model, ok := tiers[tier]; ok {
			return model
		}
	}
	return ""
}

// ResolvePresetWithOverrides applies config.yaml overrides to a resolved preset.
func ResolvePresetWithOverrides(def PresetDefinition, cfg Config) PresetDefinition {
	out := def
	if cfg.Models != nil {
		if m, ok := cfg.Models[def.Harness]; ok && m != "" {
			out.Model = m
		}
	}
	if cfg.Model != "" && out.Model == "" {
		out.Model = cfg.Model
	}
	if cfg.TimeoutMS > 0 {
		out.TimeoutMS = cfg.TimeoutMS
	}
	if cfg.Permissions != "" {
		out.Permissions = cfg.Permissions
	}
	out.Harness = def.Harness
	return out
}

// BenchmarkArm defines one arm in a benchmark run.
type BenchmarkArm struct {
	Label   string    `json:"label"`
	Harness string    `json:"harness"`
	Tier    ModelTier `json:"tier"`
	Model   string    `json:"model,omitempty"` // explicit override; empty = resolve from tier
}

// DefaultBenchmarkArms returns the standard set of arms for a full comparison.
func DefaultBenchmarkArms() []BenchmarkArm {
	return []BenchmarkArm{
		{Label: "forge-smart", Harness: "forge", Tier: TierSmart},
		{Label: "forge-fast", Harness: "forge", Tier: TierFast},
		{Label: "codex-smart", Harness: "codex", Tier: TierSmart},
		{Label: "codex-fast", Harness: "codex", Tier: TierFast},
		{Label: "claude-smart", Harness: "claude", Tier: TierSmart},
		{Label: "claude-fast", Harness: "claude", Tier: TierFast},
	}
}

// ResolveArm fills in the model from the tier if not explicitly set.
func (a *BenchmarkArm) ResolveArm() {
	if a.Model == "" {
		a.Model = ResolveModelTier(a.Harness, a.Tier)
	}
}

// ToCompareOptions converts a slice of BenchmarkArms into CompareOptions fields.
func BenchmarkArmsToCompare(arms []BenchmarkArm, baseOpts RunOptions) CompareOptions {
	harnesses := make([]string, len(arms))
	armModels := make(map[int]string, len(arms))
	armLabels := make(map[int]string, len(arms))

	for i, arm := range arms {
		arm.ResolveArm()
		harnesses[i] = arm.Harness
		if arm.Model != "" {
			armModels[i] = arm.Model
		}
		armLabels[i] = arm.Label
	}

	return CompareOptions{
		RunOptions: baseOpts,
		Harnesses:  harnesses,
		ArmModels:  armModels,
		ArmLabels:  armLabels,
	}
}
