// Package docconvert renders binary office documents into deterministic
// Markdown text.
//
// It exists so `ddx artifact check-in` can land the body of an artifact that
// is authored in an external tool: the exported original stays the artifact of
// record, and the Markdown rendering is the read surface committed beside it.
//
// Determinism is a hard requirement. The same input file must produce
// byte-identical output on every run and every platform, or the committed
// rendering produces meaningless diffs. Converters must therefore order every
// collection they emit by an explicit rule rather than by map or archive
// iteration order.
package docconvert

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Converter renders one family of source documents into Markdown.
//
// Implementations are registered by file extension; adding .docx or .pdf is a
// matter of implementing this interface and appending to the registry.
type Converter interface {
	// Extensions returns the lower-case file extensions this converter
	// handles, each including the leading dot.
	Extensions() []string

	// Convert renders the file at absPath into Markdown. sourceLabel names the
	// file in the generated header; callers pass the repository-relative path
	// so the rendering records where the original lives. Output is
	// deterministic for a given (file contents, sourceLabel) pair.
	Convert(absPath, sourceLabel string) (string, error)
}

// registry holds every known converter. Order is fixed so SupportedExtensions
// is stable.
var registry = []Converter{pptxConverter{}}

// For returns the converter registered for the extension of name.
func For(name string) (Converter, error) {
	ext := strings.ToLower(filepath.Ext(name))
	for _, converter := range registry {
		for _, candidate := range converter.Extensions() {
			if candidate == ext {
				return converter, nil
			}
		}
	}
	if ext == "" {
		return nil, fmt.Errorf("cannot convert %q: no file extension; supported formats are %s", name, strings.Join(SupportedExtensions(), ", "))
	}
	return nil, fmt.Errorf("cannot convert %q: no converter for %s files; supported formats are %s", name, ext, strings.Join(SupportedExtensions(), ", "))
}

// Convert renders the file at absPath into Markdown using the converter
// registered for its extension.
func Convert(absPath, sourceLabel string) (string, error) {
	converter, err := For(sourceLabel)
	if err != nil {
		return "", err
	}
	return converter.Convert(absPath, sourceLabel)
}

// SupportedExtensions lists every extension a converter is registered for,
// sorted for stable error messages and help text.
func SupportedExtensions() []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, converter := range registry {
		for _, ext := range converter.Extensions() {
			if _, ok := seen[ext]; ok {
				continue
			}
			seen[ext] = struct{}{}
			out = append(out, ext)
		}
	}
	sort.Strings(out)
	return out
}
