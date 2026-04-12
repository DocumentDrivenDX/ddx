package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// ModelEntryYAML holds metadata and pricing for one model.
type ModelEntryYAML struct {
	ID                 string    `yaml:"id"` // e.g. "claude-sonnet-4-6"
	Name               string    `yaml:"name,omitempty"`
	Provider           string    `yaml:"provider"`
	Tier               string    `yaml:"tier"`                        // smart, standard, cheap
	ModelFamily        string    `yaml:"model_family,omitempty"`      // e.g. "claude-opus", "gpt-5"
	ModelVersion       string    `yaml:"model_version,omitempty"`     // e.g. "4.6", "4.5" — semver-comparable
	OpenRouterRefID    string    `yaml:"openrouter_ref_id,omitempty"` // OpenRouter model ID for pricing lookup
	Blocked            bool      `yaml:"blocked,omitempty"`           // if true, routing never selects this model
	SWEBenchVerified   float64   `yaml:"swe_bench_verified,omitempty"`
	LiveCodeBench      float64   `yaml:"live_code_bench,omitempty"`
	CostInputPerM      float64   `yaml:"cost_input_per_m,omitempty"`       // USD per 1M input tokens
	CostOutputPerM     float64   `yaml:"cost_output_per_m,omitempty"`      // USD per 1M output tokens
	CostCacheWritePerM float64   `yaml:"cost_cache_write_per_m,omitempty"` // USD per 1M cache-write tokens
	CostCacheReadPerM  float64   `yaml:"cost_cache_read_per_m,omitempty"`  // USD per 1M cache-read tokens
	ContextWindow      int       `yaml:"context_window,omitempty"`
	Notes              string    `yaml:"notes,omitempty"`
	BenchmarkAsOf      string    `yaml:"benchmark_as_of,omitempty"`
	PricingUpdatedAt   time.Time `yaml:"pricing_updated_at,omitempty"`
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

// openrouterModel is the subset of the OpenRouter model response we need.
type openrouterModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pricing struct {
		Prompt          string `json:"prompt"`            // USD per token as string
		Completion      string `json:"completion"`        // USD per token as string
		InputCacheRead  string `json:"input_cache_read"`  // USD per token as string
		InputCacheWrite string `json:"input_cache_write"` // USD per token as string
	} `json:"pricing"`
	ContextLength int `json:"context_length"`
}

// FetchOpenRouterPricing fetches current model pricing from OpenRouter.
// Returns a map of model ID → openrouterModel.
func FetchOpenRouterPricing(timeout time.Duration) (map[string]openrouterModel, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	apiKey, baseURL := resolveOpenRouterCredentials()
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	url := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch openrouter models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openrouter /models HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Data []openrouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse openrouter models: %w", err)
	}

	out := make(map[string]openrouterModel, len(parsed.Data))
	for _, m := range parsed.Data {
		out[m.ID] = m
	}
	return out, nil
}

// mergeDefaultRefIDs copies OpenRouterRefID from DefaultModelCatalogYAML into cat
// for any model entry that lacks one. This lets saved catalogs that predate the
// OpenRouterRefID field pick up the correct lookup IDs on next update.
func mergeDefaultRefIDs(cat *ModelCatalogYAML) {
	defaults := DefaultModelCatalogYAML()
	refByID := make(map[string]string, len(defaults.Models))
	for _, d := range defaults.Models {
		if d.OpenRouterRefID != "" {
			refByID[d.ID] = d.OpenRouterRefID
		}
	}
	for i := range cat.Models {
		if cat.Models[i].OpenRouterRefID == "" {
			if ref, ok := refByID[cat.Models[i].ID]; ok {
				cat.Models[i].OpenRouterRefID = ref
			}
		}
	}
}

// UpdateCatalogPricing fetches current OpenRouter pricing and updates the
// CostInputPerM / CostOutputPerM fields on matching ModelEntryYAML entries.
// Returns the number of models updated and a list of model IDs not found on OpenRouter.
func UpdateCatalogPricing(cat *ModelCatalogYAML, timeout time.Duration) (updated int, notFound []string, err error) {
	mergeDefaultRefIDs(cat)

	prices, err := FetchOpenRouterPricing(timeout)
	if err != nil {
		return 0, nil, err
	}

	now := time.Now().UTC()
	for i := range cat.Models {
		m := &cat.Models[i]
		or, ok := prices[m.ID]
		if !ok && m.OpenRouterRefID != "" {
			or, ok = prices[m.OpenRouterRefID]
		}
		if !ok {
			notFound = append(notFound, m.ID)
			continue
		}
		inPerToken := parseFloat(or.Pricing.Prompt)
		outPerToken := parseFloat(or.Pricing.Completion)
		m.CostInputPerM = inPerToken * 1_000_000
		m.CostOutputPerM = outPerToken * 1_000_000
		if s := strings.TrimSpace(or.Pricing.InputCacheWrite); s != "" && s != "0" {
			m.CostCacheWritePerM = parseFloat(s) * 1_000_000
		}
		if s := strings.TrimSpace(or.Pricing.InputCacheRead); s != "" && s != "0" {
			m.CostCacheReadPerM = parseFloat(s) * 1_000_000
		}
		if or.ContextLength > 0 {
			m.ContextWindow = or.ContextLength
		}
		if m.Name == "" && or.Name != "" {
			m.Name = or.Name
		}
		m.PricingUpdatedAt = now
		updated++
	}
	cat.UpdatedAt = now
	return updated, notFound, nil
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
			{ID: "claude-opus-4-6", Provider: "anthropic", Tier: "smart", ModelFamily: "claude-opus", ModelVersion: "4.6", OpenRouterRefID: "anthropic/claude-opus-4.6", SWEBenchVerified: 80.8, CostInputPerM: 15.0, CostOutputPerM: 75.0, CostCacheWritePerM: 18.75, CostCacheReadPerM: 1.50, ContextWindow: 1000000, BenchmarkAsOf: "2026-04-12"},
			{ID: "gpt-5.4", Provider: "openai", Tier: "smart", ModelFamily: "gpt-5", ModelVersion: "5.4", OpenRouterRefID: "openai/gpt-5.4", SWEBenchVerified: 78.2, CostInputPerM: 2.50, CostOutputPerM: 15.0, CostCacheReadPerM: 0.25, ContextWindow: 1050000, BenchmarkAsOf: "2026-04-12", Notes: "codex harness; OpenRouter pricing"},
			// Standard tier
			{ID: "claude-sonnet-4-6", Provider: "anthropic", Tier: "standard", ModelFamily: "claude-sonnet", ModelVersion: "4.6", OpenRouterRefID: "anthropic/claude-sonnet-4.6", SWEBenchVerified: 79.6, CostInputPerM: 3.0, CostOutputPerM: 15.0, CostCacheWritePerM: 3.75, CostCacheReadPerM: 0.30, ContextWindow: 1000000, BenchmarkAsOf: "2026-04-12"},
			{ID: "minimax/minimax-m2.7", Provider: "minimax", Tier: "standard", SWEBenchVerified: 78.0, CostInputPerM: 0.30, CostOutputPerM: 1.20, CostCacheReadPerM: 0.06, ContextWindow: 204800, BenchmarkAsOf: "2026-04-12"},
			{ID: "minimax/minimax-m2.5", Provider: "minimax", Tier: "standard", SWEBenchVerified: 80.2, LiveCodeBench: 65.0, CostInputPerM: 0.12, CostOutputPerM: 0.99, CostCacheReadPerM: 0.059, ContextWindow: 196608, BenchmarkAsOf: "2026-04-12"},
			{ID: "moonshot/kimi-k2.5", Provider: "moonshot", Tier: "standard", OpenRouterRefID: "moonshotai/kimi-k2.5", SWEBenchVerified: 76.8, LiveCodeBench: 85.0, CostInputPerM: 0.38, CostOutputPerM: 1.72, CostCacheReadPerM: 0.191, ContextWindow: 262144, BenchmarkAsOf: "2026-04-12"},
			{ID: "openai/gpt-4.1", Provider: "openai", Tier: "standard", SWEBenchVerified: 78.0, CostInputPerM: 2.0, CostOutputPerM: 8.0, CostCacheReadPerM: 0.50, ContextWindow: 1047576, BenchmarkAsOf: "2026-04-12"},
			{ID: "openai/gpt-oss-120b", Provider: "openai", Tier: "standard", CostInputPerM: 0.04, CostOutputPerM: 0.19, ContextWindow: 131072, Notes: "local inference on vidar; no SWE-bench published", BenchmarkAsOf: "2026-04-12"},
			// Cheap tier
			{ID: "claude-haiku-4-5", Provider: "anthropic", Tier: "cheap", ModelFamily: "claude-haiku", ModelVersion: "4.5", OpenRouterRefID: "anthropic/claude-haiku-4.5", SWEBenchVerified: 73.3, CostInputPerM: 0.80, CostOutputPerM: 4.0, CostCacheWritePerM: 1.00, CostCacheReadPerM: 0.08, ContextWindow: 200000, BenchmarkAsOf: "2026-04-12"},
			{ID: "gpt-5.4-mini", Provider: "openai", Tier: "cheap", ModelFamily: "gpt-5-mini", ModelVersion: "5.4", OpenRouterRefID: "openai/gpt-5.4-mini", CostInputPerM: 0.75, CostOutputPerM: 4.50, CostCacheReadPerM: 0.075, ContextWindow: 400000, BenchmarkAsOf: "2026-04-12", Notes: "codex harness; OpenRouter pricing"},
			{ID: "qwen3.5-27b", Provider: "qwen", Tier: "cheap", OpenRouterRefID: "qwen/qwen3.5-27b", SWEBenchVerified: 72.4, CostInputPerM: 0.20, CostOutputPerM: 1.56, ContextWindow: 262144, Notes: "local inference on vidar; benchmark for full precision — quantization reduces scores", BenchmarkAsOf: "2026-04-12"},
			{ID: "qwen/qwen3-coder-next", Provider: "qwen", Tier: "cheap", SWEBenchVerified: 70.6, LiveCodeBench: 70.7, CostInputPerM: 0.15, CostOutputPerM: 0.80, CostCacheReadPerM: 0.12, ContextWindow: 262144, Notes: "local; 80B MoE (3B active); benchmark for full precision — quantization reduces scores", BenchmarkAsOf: "2026-04-12"},
			{ID: "openai/gpt-oss-20b", Provider: "openai", Tier: "cheap", CostInputPerM: 0.03, CostOutputPerM: 0.14, ContextWindow: 131072, Notes: "local inference on vidar; no SWE-bench published", BenchmarkAsOf: "2026-04-12"},
			// Blocked: deprecated/retired models — routing must never select these.
			{ID: "gpt-3.5-turbo", Provider: "openai", Tier: "cheap", Blocked: true, Notes: "retired; use cheap tier"},
			{ID: "gpt-3.5-turbo-16k", Provider: "openai", Tier: "cheap", Blocked: true, Notes: "retired; use cheap tier"},
			{ID: "claude-opus-4-5", Provider: "anthropic", Tier: "smart", Blocked: true, Notes: "superseded by claude-opus-4-6"},
			{ID: "claude-3-opus-20240229", Provider: "anthropic", Tier: "smart", Blocked: true, Notes: "superseded by claude-opus-4-6"},
			{ID: "claude-3-5-sonnet-20241022", Provider: "anthropic", Tier: "standard", Blocked: true, Notes: "superseded by claude-sonnet-4-6"},
		},
	}
}

// CompareModelVersions compares two ModelEntryYAML entries by version within
// their model family. Returns negative if a < b, 0 if equal or different families,
// positive if a > b. Version strings are compared component-by-component numerically
// (e.g. "4.6" > "4.5").
func CompareModelVersions(a, b ModelEntryYAML) int {
	if a.ModelFamily == "" || b.ModelFamily == "" || a.ModelFamily != b.ModelFamily {
		return 0
	}
	aParts := strings.Split(a.ModelVersion, ".")
	bParts := strings.Split(b.ModelVersion, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			_, _ = fmt.Sscanf(strings.TrimSpace(aParts[i]), "%d", &aNum)
		}
		if i < len(bParts) {
			_, _ = fmt.Sscanf(strings.TrimSpace(bParts[i]), "%d", &bNum)
		}
		if aNum != bNum {
			return aNum - bNum
		}
	}
	return 0
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}
