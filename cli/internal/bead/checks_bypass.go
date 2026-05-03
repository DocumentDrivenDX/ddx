package bead

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChecksBypassExtraKey is the bead.Extra key under which the checks_bypass
// annotation is persisted. The value is a JSON array of objects matching
// ChecksBypassEntry. Stored under Extra (not a dedicated struct field) so
// schema-unaware tools (bd, br, JSON dumps) round-trip the field unchanged.
const ChecksBypassExtraKey = "checks_bypass"

// ChecksBypassEntry skips one named pre-merge check for one bead. Each entry
// MUST carry a non-empty reason — bypasses without justification are rejected
// loudly by ChecksBypasses below so a check cannot be silently disabled.
//
// Bead is optional and serves as a cross-reference (for example, the bead ID
// where the bypass is being tracked for later removal).
type ChecksBypassEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Bead   string `json:"bead,omitempty"`
}

// ChecksBypasses parses the checks_bypass annotation from a bead's Extra map.
// Returns (nil, nil) when no annotation is present. Returns a non-nil error
// when the annotation exists but is malformed, has an entry with an empty
// name, or has an entry with an empty/whitespace-only reason. Empty reasons
// are rejected loudly per the slice contract: a check cannot be bypassed
// without recording why.
func ChecksBypasses(b *Bead) ([]ChecksBypassEntry, error) {
	if b == nil || b.Extra == nil {
		return nil, nil
	}
	raw, ok := b.Extra[ChecksBypassExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	// Re-marshal then unmarshal to convert the generic any tree (which json
	// unmarshalled into map[string]any) into a typed slice without writing a
	// custom walker.
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("checks_bypass: re-marshal: %w", err)
	}
	var entries []ChecksBypassEntry
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return nil, fmt.Errorf("checks_bypass: must be a JSON array of {name,reason,bead?}: %w", err)
	}
	for i, e := range entries {
		if strings.TrimSpace(e.Name) == "" {
			return nil, fmt.Errorf("checks_bypass[%d]: missing required field: name", i)
		}
		if strings.TrimSpace(e.Reason) == "" {
			return nil, fmt.Errorf("checks_bypass[%d] (%s): missing required field: reason — bypasses must record why the check is being skipped", i, e.Name)
		}
	}
	return entries, nil
}
