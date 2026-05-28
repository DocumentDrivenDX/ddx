package artifacttypes

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Example describes a worked example for an artifact type.
type Example struct {
	Path        string
	Description string
}

// Type is the normalized artifact-type record produced by the loader.
type Type struct {
	Plugin         string
	TypeID         string
	Name           string
	Description    string
	Prefix         string
	PrefixExplicit bool
	Pattern        string
	Phase          string
	TemplatePath   string
	PromptPath     string
	Examples       []Example
	SourceMetaPath string
}

// Index is the normalized in-memory lookup index for discovered artifact types.
type Index struct {
	Types    []Type
	byKey    map[string]int
	byPrefix map[string]int
	byPlugin map[string][]int
}

// Clone returns a deep copy of the index.
func (i *Index) Clone() *Index {
	if i == nil {
		return nil
	}
	clone := &Index{Types: make([]Type, len(i.Types))}
	for idx, typ := range i.Types {
		clone.Types[idx] = cloneType(typ)
	}
	clone.rebuildLookups()
	return clone
}

func buildIndex(types []Type) (*Index, error) {
	sorted := make([]Type, len(types))
	copy(sorted, types)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Plugin != sorted[j].Plugin {
			return sorted[i].Plugin < sorted[j].Plugin
		}
		if sorted[i].TypeID != sorted[j].TypeID {
			return sorted[i].TypeID < sorted[j].TypeID
		}
		return sorted[i].SourceMetaPath < sorted[j].SourceMetaPath
	})

	idx := &Index{
		Types:    sorted,
		byKey:    make(map[string]int, len(sorted)),
		byPrefix: make(map[string]int, len(sorted)),
		byPlugin: make(map[string][]int, len(sorted)),
	}

	for i, typ := range sorted {
		if typ.Plugin == "" {
			return nil, fmt.Errorf("artifact type %q missing plugin name", typ.TypeID)
		}
		if typ.TypeID == "" {
			return nil, fmt.Errorf("artifact type in plugin %q missing type id", typ.Plugin)
		}
		key := keyFor(typ.Plugin, typ.TypeID)
		if _, exists := idx.byKey[key]; exists {
			return nil, fmt.Errorf("duplicate artifact type %s/%s", typ.Plugin, typ.TypeID)
		}
		idx.byKey[key] = i
		idx.byPlugin[typ.Plugin] = append(idx.byPlugin[typ.Plugin], i)
		if typ.Prefix != "" {
			if existingIdx, exists := idx.byPrefix[typ.Prefix]; exists {
				existing := sorted[existingIdx]
				if typ.PrefixExplicit && existing.PrefixExplicit {
					return nil, fmt.Errorf("duplicate artifact type prefix %q", typ.Prefix)
				}
				log.Printf("artifacttypes: prefix collision %q: keeping %s/%s (%s); shadowing %s/%s (%s)",
					typ.Prefix,
					existing.Plugin, existing.TypeID, existing.SourceMetaPath,
					typ.Plugin, typ.TypeID, typ.SourceMetaPath,
				)
				continue
			}
			idx.byPrefix[typ.Prefix] = i
		}
	}
	return idx, nil
}

func (i *Index) rebuildLookups() {
	i.byKey = make(map[string]int, len(i.Types))
	i.byPrefix = make(map[string]int, len(i.Types))
	i.byPlugin = make(map[string][]int, len(i.Types))
	for idx, typ := range i.Types {
		i.byKey[keyFor(typ.Plugin, typ.TypeID)] = idx
		i.byPlugin[typ.Plugin] = append(i.byPlugin[typ.Plugin], idx)
		if typ.Prefix != "" {
			i.byPrefix[typ.Prefix] = idx
		}
	}
}

func cloneType(typ Type) Type {
	clone := typ
	if len(typ.Examples) > 0 {
		clone.Examples = append([]Example(nil), typ.Examples...)
	}
	return clone
}

func keyFor(plugin, typeID string) string {
	return plugin + "\x00" + typeID
}

func humanizeTypeID(typeID string) string {
	if typeID == "" {
		return ""
	}
	fields := strings.FieldsFunc(typeID, func(r rune) bool {
		return r == '-' || r == '_' || r == '/'
	})
	if len(fields) == 0 {
		return titleWord(typeID)
	}
	for i, field := range fields {
		fields[i] = titleWord(field)
	}
	return strings.Join(fields, " ")
}

func titleWord(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + strings.ToLower(s[size:])
}
