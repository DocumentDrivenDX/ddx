// Duplicate user-story-id detection across feature documents (WB-1 steps 5-6).
//
// Scans docs/helix/01-frame/features/ only. Failures of kind
// FindingDuplicateUSID are non-waivable (WB-1 step 5). Read-only: never writes.
package spechonesty

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FeaturesRelativeDir is the sole directory under a helix root where
// user-story ids are scanned for cross-feature collisions.
const FeaturesRelativeDir = "01-frame/features"

// usHeadingRe matches a markdown heading that defines a user story id
// (e.g. "### US-087: Operator Hands Off…" or "## US-082b Title").
// Letter suffixes such as US-082b are part of the id.
var usHeadingRe = regexp.MustCompile(`(?i)^#{1,6}\s+(US-\d+[A-Za-z]*)\b`)

// usIDFormRe normalizes a raw US-id capture to canonical form.
var usIDFormRe = regexp.MustCompile(`(?i)^US-(\d+)([A-Za-z]*)$`)

// USRef records one occurrence of a user-story id for diagnostics.
type USRef struct {
	// ID is the canonical user-story id (e.g. US-087, US-082b).
	ID string
	// Path is the absolute or supplied feature file path.
	Path string
	// Line is the 1-based line of the heading when known; 1 otherwise.
	Line int
}

// CanonicalUserStoryID normalizes a raw US-id to US-<digits>[suffix]
// with an uppercase US prefix and lowercased letter suffix so US-082B
// and US-082b collide. Empty raw returns empty.
func CanonicalUserStoryID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	m := usIDFormRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	return "US-" + m[1] + strings.ToLower(m[2])
}

// ExtractUserStoryIDs returns every user-story id defined as a markdown
// heading in content, in document order. Duplicate headings in the same
// file yield multiple refs with the same Path.
func ExtractUserStoryIDs(path, content string) []USRef {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	var refs []USRef
	for i, line := range lines {
		m := usHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := CanonicalUserStoryID(m[1])
		if id == "" {
			continue
		}
		refs = append(refs, USRef{ID: id, Path: path, Line: i + 1})
	}
	return refs
}

// isUnderFeatures reports whether path is under helixRoot/01-frame/features.
func isUnderFeatures(helixRoot, path string) bool {
	root, err := filepath.Abs(helixRoot)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	prefix := FeaturesRelativeDir + "/"
	return rel == FeaturesRelativeDir || strings.HasPrefix(rel, prefix)
}

// FindDuplicateUserStoryIDs walks feature documents under helixRoot and
// returns one CoverageFinding per user-story id that appears in two or more
// distinct feature files (SeverityError, Kind FindingDuplicateUSID). Each
// message names every conflicting feature path. Findings are non-waivable;
// ApplyWaiverPolicy will not downgrade them. Read-only.
func FindDuplicateUserStoryIDs(helixRoot string) ([]CoverageFinding, error) {
	// id -> first ref per distinct path (stable first-seen line)
	byID := make(map[string][]USRef)
	seenPath := make(map[string]map[string]bool) // id -> path -> seen

	err := WalkHelixLintDocs(helixRoot, func(path string) error {
		if !isUnderFeatures(helixRoot, path) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, ref := range ExtractUserStoryIDs(path, string(data)) {
			if seenPath[ref.ID] == nil {
				seenPath[ref.ID] = make(map[string]bool)
			}
			if seenPath[ref.ID][ref.Path] {
				continue
			}
			seenPath[ref.ID][ref.Path] = true
			byID[ref.ID] = append(byID[ref.ID], ref)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(byID))
	for id, refs := range byID {
		if len(refs) < 2 {
			continue
		}
		ids = append(ids, id)
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].Path < refs[j].Path
		})
		byID[id] = refs
	}
	sort.Strings(ids)

	var findings []CoverageFinding
	for _, id := range ids {
		refs := byID[id]
		paths := make([]string, len(refs))
		for i, r := range refs {
			paths[i] = r.Path
		}
		msg := fmt.Sprintf("duplicate user-story id %s in: %s", id, strings.Join(paths, ", "))
		findings = append(findings, CoverageFinding{
			Path:     refs[0].Path,
			Line:     refs[0].Line,
			Kind:     FindingDuplicateUSID,
			Severity: SeverityError,
			Message:  msg,
		})
	}
	return findings, nil
}

// ScanDuplicateUserStoryIDs is the Diagnostic-shaped wrapper used by the
// docs-directory CLI scan. Same scope and non-waivable semantics as
// FindDuplicateUserStoryIDs.
func ScanDuplicateUserStoryIDs(helixRoot string) ([]Diagnostic, error) {
	findings, err := FindDuplicateUserStoryIDs(helixRoot)
	if err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		return nil, nil
	}
	diags := make([]Diagnostic, len(findings))
	for i, f := range findings {
		diags[i] = Diagnostic{
			Path:    f.Path,
			Line:    f.Line,
			Kind:    string(f.Kind),
			Message: f.Message,
		}
	}
	return diags, nil
}
