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
	// Blocked marks this ref as blocked; Resolve() returns ok=false for blocked entries.
	Blocked bool
}

// DeprecatedPin records a deprecated explicit model version string.
// Use this for exact concrete model pins (e.g. "claude-opus-4-5") that are
// stale — as opposed to logical alias deprecations in CatalogEntry.
type DeprecatedPin struct {
	// Pin is the deprecated exact model string (e.g. "claude-opus-4-5").
	Pin string
	// ReplacedBy is the canonical replacement: a catalog ref (e.g. "smart")
	// or a newer exact model string (e.g. "claude-opus-4-6").
	ReplacedBy string
	// Surface constrains the deprecation to a specific harness surface.
	// Empty means the pin is deprecated across all surfaces.
	Surface string
}

// Catalog holds the shared DDx model catalog used for harness routing.
// It maps logical refs to harness-surface-specific concrete model strings.
// This is the authoritative source for aliases, profiles, canonical targets,
// and deprecation metadata across harness surfaces.
type Catalog struct {
	entries         map[string]CatalogEntry
	deprecatedPins  map[string]DeprecatedPin // keyed by Pin
	blockedModelIDs map[string]bool          // concrete model IDs that routing must never select
}

// catalog returns the catalog to use for runtime model metadata checks.
// Defaults to BuiltinCatalog.
func (r *Runner) catalog() *Catalog {
	if r.Catalog != nil {
		return r.Catalog
	}
	return BuiltinCatalog
}

// NewCatalog creates a Catalog from a slice of entries.
func NewCatalog(entries []CatalogEntry) *Catalog {
	c := &Catalog{entries: make(map[string]CatalogEntry, len(entries))}
	for _, e := range entries {
		c.entries[e.Ref] = e
	}
	return c
}

// NewCatalogWithPins creates a Catalog from entries and deprecated pin records.
func NewCatalogWithPins(entries []CatalogEntry, pins []DeprecatedPin) *Catalog {
	c := NewCatalog(entries)
	c.deprecatedPins = make(map[string]DeprecatedPin, len(pins))
	for _, p := range pins {
		c.deprecatedPins[p.Pin] = p
	}
	return c
}

// Clone returns an independent copy of the catalog.
func (c *Catalog) Clone() *Catalog {
	if c == nil {
		return NewCatalog(nil)
	}
	clone := &Catalog{
		entries:         make(map[string]CatalogEntry, len(c.entries)),
		deprecatedPins:  make(map[string]DeprecatedPin, len(c.deprecatedPins)),
		blockedModelIDs: make(map[string]bool, len(c.blockedModelIDs)),
	}
	for ref, entry := range c.entries {
		surfaces := make(map[string]string, len(entry.Surfaces))
		for surface, model := range entry.Surfaces {
			surfaces[surface] = model
		}
		entry.Surfaces = surfaces
		clone.entries[ref] = entry
	}
	for pin, entry := range c.deprecatedPins {
		clone.deprecatedPins[pin] = entry
	}
	for id, blocked := range c.blockedModelIDs {
		clone.blockedModelIDs[id] = blocked
	}
	return clone
}

// AddOrReplace inserts or replaces a catalog entry by Ref.
func (c *Catalog) AddOrReplace(entry CatalogEntry) {
	if c.entries == nil {
		c.entries = make(map[string]CatalogEntry)
	}
	c.entries[entry.Ref] = entry
}

// AddBlockedModelID marks a concrete model ID as blocked.
// Resolve() returns ok=false for any model that resolves to a blocked ID.
func (c *Catalog) AddBlockedModelID(id string) {
	if c.blockedModelIDs == nil {
		c.blockedModelIDs = make(map[string]bool)
	}
	c.blockedModelIDs[id] = true
}

// IsBlockedModelID reports whether a concrete model ID is blocked.
func (c *Catalog) IsBlockedModelID(id string) bool {
	return c.blockedModelIDs[id]
}

// CheckDeprecatedPin returns the DeprecatedPin entry for an explicit model
// string, or ok=false if the pin is not deprecated.
// If surface is non-empty and the pin entry has a Surface set, the match is
// narrowed to that surface only.
func (c *Catalog) CheckDeprecatedPin(pin, surface string) (DeprecatedPin, bool) {
	if c.deprecatedPins == nil {
		return DeprecatedPin{}, false
	}
	dp, ok := c.deprecatedPins[pin]
	if !ok {
		return DeprecatedPin{}, false
	}
	if dp.Surface != "" && surface != "" && dp.Surface != surface {
		return DeprecatedPin{}, false
	}
	return dp, true
}

// Resolve returns the concrete model string for a ref on the given surface.
// Returns ("", false) if the ref is unknown, not mapped to this surface,
// the entry is blocked, or the resolved concrete model is blocked.
func (c *Catalog) Resolve(ref, surface string) (string, bool) {
	e, ok := c.entries[ref]
	if !ok {
		return "", false
	}
	if e.Blocked {
		return "", false
	}
	model, ok := e.Surfaces[surface]
	if !ok {
		return "", false
	}
	if c.blockedModelIDs[model] {
		return "", false
	}
	return model, true
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

// BuiltinCatalog is the DDx shared routing catalog built from the YAML seed.
// Tier→surface→model assignments come from DefaultModelCatalogYAML so the data
// lives in one place. Policy aliases (qwen3, codex-mini) and deprecated pins
// are added here as they are routing policy decisions, not model metadata.
var BuiltinCatalog = buildBuiltinCatalog()

func buildBuiltinCatalog() *Catalog {
	yml := DefaultModelCatalogYAML()

	// Build tier entries from YAML seed.
	var entries []CatalogEntry
	for tierName, tierDef := range yml.Tiers {
		entry := CatalogEntry{
			Ref:      tierName,
			Surfaces: make(map[string]string, len(tierDef.Surfaces)),
		}
		for surface, model := range tierDef.Surfaces {
			entry.Surfaces[surface] = model
		}
		entries = append(entries, entry)
	}

	// Policy aliases: logical names not tied to a tier profile.
	entries = append(entries,
		// qwen3 is only available via the embedded OpenAI-compatible surface.
		CatalogEntry{
			Ref:      "qwen3",
			Surfaces: map[string]string{"embedded-openai": "qwen/qwen3-coder-next"},
		},
		// codex-mini is a deprecated alias for the cheap codex model.
		CatalogEntry{
			Ref:        "codex-mini",
			Surfaces:   map[string]string{"codex": "gpt-5.4-mini"},
			Deprecated: true,
			ReplacedBy: "cheap",
		},
	)

	pins := []DeprecatedPin{
		// Stale exact version strings for the claude (Anthropic) family.
		{Pin: "claude-opus-4-5", Surface: "claude", ReplacedBy: "claude-opus-4-6"},
		{Pin: "claude-3-5-sonnet-20241022", Surface: "claude", ReplacedBy: "claude-sonnet-4-6"},
		{Pin: "claude-3-opus-20240229", Surface: "claude", ReplacedBy: "claude-opus-4-6"},
		{Pin: "claude-3-sonnet-20240229", Surface: "claude", ReplacedBy: "claude-sonnet-4-6"},
		// Stale exact version strings for the codex (OpenAI) family.
		{Pin: "gpt-4o", Surface: "codex", ReplacedBy: "gpt-5.4-mini"},
		{Pin: "gpt-4-turbo", Surface: "codex", ReplacedBy: "gpt-5.4"},
		{Pin: "o1-2024-12-17", Surface: "codex", ReplacedBy: "gpt-5.4"},
	}

	return NewCatalogWithPins(entries, pins)
}
