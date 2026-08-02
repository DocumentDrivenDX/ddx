package bead

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtraLifecycleLocalBlockerRef stores a structured local-resource blocker ref
// as JSON {"kind":"local_resource_exhaustion",...}.
const ExtraLifecycleLocalBlockerRef = "lifecycle-local-blocker-ref"

// LocalBlockerKind identifies the closed set of typed local blockers.
type LocalBlockerKind string

const (
	// LocalBlockerKindLocalResourceExhaustion indicates the bead was blocked by
	// a host-local resource exhaustion condition.
	LocalBlockerKindLocalResourceExhaustion LocalBlockerKind = "local_resource_exhaustion"
)

// LocalBlockerRef records the typed local blocker classification and optional
// evidence used to recheck or explain the block.
type LocalBlockerRef struct {
	Kind          LocalBlockerKind `json:"kind"`
	ResourceRoots []string         `json:"resource_roots,omitempty"`
	Fingerprint   string           `json:"fingerprint,omitempty"`
}

// NewLocalBlockerRef validates and constructs a typed local blocker ref.
func NewLocalBlockerRef(kind LocalBlockerKind, resourceRoots []string, fingerprint string) (LocalBlockerRef, error) {
	ref, ok := localBlockerRefFromParts(kind, resourceRoots, fingerprint)
	if !ok {
		return LocalBlockerRef{}, fmt.Errorf("local blocker ref requires a known kind")
	}
	return ref, nil
}

// ParseLocalBlockerRef attempts to decode a structured local blocker ref from
// an extra-field value or raw JSON object.
func ParseLocalBlockerRef(v any) (LocalBlockerRef, bool) {
	switch value := v.(type) {
	case nil:
		return LocalBlockerRef{}, false
	case LocalBlockerRef:
		return localBlockerRefFromParts(value.Kind, value.ResourceRoots, value.Fingerprint)
	case *LocalBlockerRef:
		if value == nil {
			return LocalBlockerRef{}, false
		}
		return localBlockerRefFromParts(value.Kind, value.ResourceRoots, value.Fingerprint)
	case map[string]any:
		return localBlockerRefFromParts(
			LocalBlockerKind(extraStringVal(value, "kind")),
			extraStringSliceVal(value, "resource_roots"),
			extraStringVal(value, "fingerprint"),
		)
	case map[string]string:
		return localBlockerRefFromParts(
			LocalBlockerKind(value["kind"]),
			singletonStrings(value["resource_roots"]),
			value["fingerprint"],
		)
	case json.RawMessage:
		var ref LocalBlockerRef
		if err := json.Unmarshal(value, &ref); err != nil {
			return LocalBlockerRef{}, false
		}
		return localBlockerRefFromParts(ref.Kind, ref.ResourceRoots, ref.Fingerprint)
	case []byte:
		var ref LocalBlockerRef
		if err := json.Unmarshal(value, &ref); err != nil {
			return LocalBlockerRef{}, false
		}
		return localBlockerRefFromParts(ref.Kind, ref.ResourceRoots, ref.Fingerprint)
	default:
		return LocalBlockerRef{}, false
	}
}

func localBlockerRefFromParts(kind LocalBlockerKind, resourceRoots []string, fingerprint string) (LocalBlockerRef, bool) {
	normalizedKind := LocalBlockerKind(strings.TrimSpace(string(kind)))
	if normalizedKind != LocalBlockerKindLocalResourceExhaustion {
		return LocalBlockerRef{}, false
	}

	roots := normalizeLocalBlockerResourceRoots(resourceRoots)
	ref := LocalBlockerRef{
		Kind:          normalizedKind,
		ResourceRoots: roots,
		Fingerprint:   strings.TrimSpace(fingerprint),
	}
	return ref, true
}

func normalizeLocalBlockerResourceRoots(resourceRoots []string) []string {
	roots := make([]string, 0, len(resourceRoots))
	for _, root := range resourceRoots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		roots = append(roots, trimmed)
	}
	return roots
}

func extraStringSliceVal(v map[string]any, key string) []string {
	raw, ok := v[key]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		roots := make([]string, 0, len(value))
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				continue
			}
			roots = append(roots, s)
		}
		return roots
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{value}
	default:
		return nil
	}
}

func singletonStrings(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return []string{v}
}
