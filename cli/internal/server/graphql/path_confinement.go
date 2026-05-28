package graphql

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrDocumentOutsideLibrary is returned by ResolveDocumentPath when the
// requested path canonicalises outside the library root.
var ErrDocumentOutsideLibrary = errors.New("document: path outside library root")

// ResolveDocumentPath canonicalises a user-supplied document path (relative to
// the library root) and returns the absolute filesystem path inside the library.
// It rejects:
//   - absolute paths
//   - paths containing ".." that escape the root
//   - symlink targets that resolve outside the root
//
// All three rejection modes return ErrDocumentOutsideLibrary. The path argument
// must use forward slashes (or be normalisable to them).
func ResolveDocumentPath(libraryRoot, path string) (string, error) {
	if path == "" {
		return "", ErrDocumentOutsideLibrary
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return "", ErrDocumentOutsideLibrary
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrDocumentOutsideLibrary
	}
	full := filepath.Join(libraryRoot, clean)
	// Re-clean and confirm it sits under libraryRoot.
	full = filepath.Clean(full)
	rel, err := filepath.Rel(libraryRoot, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrDocumentOutsideLibrary
	}
	// If the file (or any parent) is a symlink, confirm the eval'd target
	// also remains inside the canonical libraryRoot. Missing files are OK to
	// pass through here — the caller will return 404 when reading.
	if eval, err := filepath.EvalSymlinks(full); err == nil {
		evalClean := filepath.Clean(eval)
		rel2, err := filepath.Rel(libraryRoot, evalClean)
		if err != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
			return "", ErrDocumentOutsideLibrary
		}
	}
	return full, nil
}
