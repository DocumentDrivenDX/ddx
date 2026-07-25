// Duplicate document-id detection (WB-1 step 6).
//
// Scans exactly docs/helix/01-frame/features/ and the five
// docs/helix/02-design/{adr,concepts,contracts,solution-designs,technical-designs}/
// subdirectories. Failures of kind FindingDuplicateID are non-waivable
// (WB-1 step 5). Read-only: never writes.
package spechonesty

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// HelixLintRelativeDirs lists the directories under a helix root
// (typically docs/helix/) that the document-id scan covers.
// No other paths are walked. Relative separators are always '/'.
var HelixLintRelativeDirs = []string{
	"01-frame/features",
	"02-design/adr",
	"02-design/concepts",
	"02-design/contracts",
	"02-design/solution-designs",
	"02-design/technical-designs",
}

// shortDocIDRe matches a TYPE-NNN prefix (e.g. TD-040, FEAT-001, API-002).
// Longer slugs such as TD-040-cross-repo-blocker-recheck normalize to TD-040.
var shortDocIDRe = regexp.MustCompile(`(?i)^([A-Za-z][A-Za-z0-9]*)-(\d+)\b`)

// frontmatterIDRe matches an `id:` key inside YAML frontmatter.
var frontmatterIDRe = regexp.MustCompile(`(?i)^\s*id\s*:\s*(.+?)\s*$`)

// DocRef records one occurrence of a document id for diagnostics.
type DocRef struct {
	// ID is the canonical document id (e.g. TD-040).
	ID string
	// Path is the absolute or supplied file path.
	Path string
	// Line is the 1-based line of the id stamp when known; 1 otherwise.
	Line int
}

// WalkHelixLintDocs walks helixRoot for Markdown files under exactly
// HelixLintRelativeDirs. Missing subdirectories are skipped. The tree is
// never modified. fn is called once per .md file; a non-nil error from fn
// aborts the walk.
func WalkHelixLintDocs(helixRoot string, fn func(path string) error) error {
	if helixRoot == "" {
		return fmt.Errorf("helix root is empty")
	}
	root, err := filepath.Abs(helixRoot)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("helix root is not a directory: %s", root)
	}

	for _, rel := range HelixLintRelativeDirs {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		st, statErr := os.Stat(dir)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return statErr
		}
		if !st.IsDir() {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			return fn(path)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// CanonicalDocumentID normalizes a raw id or basename stem to a short
// TYPE-NNN form when possible (TD-040-cross-repo → TD-040). Otherwise
// returns the trimmed raw string.
func CanonicalDocumentID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip optional YAML quotes.
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	if m := shortDocIDRe.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2]
	}
	return s
}

// ExtractDocumentID derives the canonical document id for a markdown file.
// Prefers frontmatter `id:` (typically under ddx:), falls back to the
// basename stem. Returns ok=false when no id can be determined.
func ExtractDocumentID(path, content string) (id string, line int, ok bool) {
	if raw, ln, found := findFrontmatterID(content); found {
		id = CanonicalDocumentID(raw)
		if id != "" {
			return id, ln, true
		}
	}
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	id = CanonicalDocumentID(stem)
	if id == "" {
		return "", 0, false
	}
	return id, 1, true
}

func findFrontmatterID(content string) (raw string, line int, ok bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", 0, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return "", 0, false
		}
		if m := frontmatterIDRe.FindStringSubmatch(lines[i]); m != nil {
			return strings.TrimSpace(m[1]), i + 1, true
		}
	}
	return "", 0, false
}

// FindDuplicateDocumentIDs walks the helix lint scope under helixRoot and
// returns one CoverageFinding per duplicate id (SeverityError,
// Kind FindingDuplicateID). Each message names every conflicting path.
// Findings are non-waivable; ApplyWaiverPolicy will not downgrade them.
// Read-only.
func FindDuplicateDocumentIDs(helixRoot string) ([]CoverageFinding, error) {
	// id -> refs in stable path order
	byID := make(map[string][]DocRef)

	err := WalkHelixLintDocs(helixRoot, func(path string) error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		id, line, ok := ExtractDocumentID(path, string(data))
		if !ok || id == "" {
			return nil
		}
		byID[id] = append(byID[id], DocRef{ID: id, Path: path, Line: line})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Stable ordering: sort ids, then paths within each id.
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
		// One finding anchored at the first conflicting file; message
		// enumerates all paths so both (or more) documents are identifiable.
		msg := fmt.Sprintf("duplicate document id %s in: %s", id, strings.Join(paths, ", "))
		findings = append(findings, CoverageFinding{
			Path:     refs[0].Path,
			Line:     refs[0].Line,
			Kind:     FindingDuplicateID,
			Severity: SeverityError,
			Message:  msg,
		})
	}
	return findings, nil
}

// ScanDuplicateDocumentIDs is the Diagnostic-shaped wrapper used by the
// docs-directory CLI scan. Same scope and non-waivable semantics as
// FindDuplicateDocumentIDs.
func ScanDuplicateDocumentIDs(helixRoot string) ([]Diagnostic, error) {
	findings, err := FindDuplicateDocumentIDs(helixRoot)
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
