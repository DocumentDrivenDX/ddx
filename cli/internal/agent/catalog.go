package agent

// CatalogEntry defines a logical model ref or profile's surface mappings.
// DDx uses this to project a ref (alias, profile, or canonical name) onto
// harness-specific surfaces and to surface deprecation/replacement metadata.
type CatalogEntry struct {
	// Ref is the logical name (e.g. "qwen3", "cheap", "fast", "smart").
	Ref string
	// Surfaces maps a harness surface identifier to the concrete model string
	// to pass to that harness. A ref absent from a surface's map means that
	// surface cannot serve the ref.
	Surfaces map[string]string
	// Deprecated marks this ref as deprecated.
	Deprecated bool
	// ReplacedBy is the canonical replacement ref when Deprecated is true.
	ReplacedBy string
}

// Catalog holds the shared DDx model catalog used for harness routing.
// It maps logical refs to harness-surface-specific concrete model strings.
// This is the authoritative source for aliases, profiles, canonical targets,
// and deprecation metadata across harness surfaces.
type Catalog struct {
	entries map[string]CatalogEntry
}

// NewCatalog creates a Catalog from a slice of entries.
func NewCatalog(entries []CatalogEntry) *Catalog {
	c := &Catalog{entries: make(map[string]CatalogEntry, len(entries))}
	for _, e := range entries {
		c.entries[e.Ref] = e
	}
	return c
}

// Resolve returns the concrete model string for a ref on the given surface.
// Returns ("", false) if the ref is unknown or not mapped to this surface.
func (c *Catalog) Resolve(ref, surface string) (string, bool) {
	e, ok := c.entries[ref]
	if !ok {
		return "", false
	}
	model, ok := e.Surfaces[surface]
	return model, ok
}

// Entry returns the full catalog entry for a ref.
func (c *Catalog) Entry(ref string) (CatalogEntry, bool) {
	e, ok := c.entries[ref]
	return e, ok
}

// KnownOnAnySurface returns true if the ref has a mapping on at least one surface.
func (c *Catalog) KnownOnAnySurface(ref string) bool {
	e, ok := c.entries[ref]
	if !ok {
		return false
	}
	return len(e.Surfaces) > 0
}

// NormalizeModelRef resolves a raw --model input:
//   - If the value is known in the catalog on at least one surface, it is
//     treated as a logical ModelRef.
//   - Otherwise it is treated as an exact ModelPin (bypasses catalog).
//
// Exactly one of modelRef or modelPin will be non-empty.
func (c *Catalog) NormalizeModelRef(model string) (modelRef, modelPin string) {
	if model == "" {
		return "", ""
	}
	if c.KnownOnAnySurface(model) {
		return model, ""
	}
	return "", model
}

// BuiltinCatalog is the DDx shared routing catalog.
// It contains the initial transitional entries used for harness-surface
// projection while the full shared ddx-agent catalog is integrated.
//
// Rule: entries here supersede DefaultModelTiers for routing decisions.
// DefaultModelTiers remains as explicit transitional fallback for surfaces
// or tiers not yet covered by catalog entries.
var BuiltinCatalog = NewCatalog([]CatalogEntry{
	// --- Profiles (available across cloud and embedded surfaces) ---
	{
		Ref: "cheap",
		Surfaces: map[string]string{
			"codex":           "gpt-5.4-mini",
			"claude":          "claude-sonnet-4-6",
			"embedded-openai": "qwen3.5-27b",
		},
	},
	{
		Ref: "fast",
		Surfaces: map[string]string{
			"codex":           "gpt-5.4-mini",
			"claude":          "claude-sonnet-4-6",
			"embedded-openai": "qwen3.5-27b",
		},
	},
	{
		Ref: "smart",
		Surfaces: map[string]string{
			"codex":           "gpt-5.4",
			"claude":          "claude-opus-4-6",
			"embedded-openai": "qwen/qwen3-coder-next",
		},
	},

	// --- Embedded-only refs ---
	// qwen3 is only available via the embedded OpenAI-compatible surface.
	// DDx selects the embedded harness; ddx-agent resolves the provider/backend.
	{
		Ref: "qwen3",
		Surfaces: map[string]string{
			"embedded-openai": "qwen/qwen3-coder-next",
		},
	},

	// --- Deprecated aliases ---
	{
		Ref: "codex-mini",
		Surfaces: map[string]string{
			"codex": "gpt-5.4-mini",
		},
		Deprecated: true,
		ReplacedBy: "gpt-5.4-mini",
	},
})
