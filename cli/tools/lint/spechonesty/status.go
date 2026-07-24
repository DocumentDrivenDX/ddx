// Package spechonesty status parser (WB-1 steps 1-2).
//
// Recognizes body `**Status:**` lines and YAML frontmatter `status:` keys,
// normalizes free-text qualifiers to a base DocStatus, and flags missing
// status on SD/TD/ADR design documents. Read-only: never mutates files.
package spechonesty

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// StatusSource identifies where a status stamp was found.
type StatusSource string

const (
	// StatusSourceBody is a markdown body `**Status:**` line.
	StatusSourceBody StatusSource = "body"
	// StatusSourceFrontmatter is a YAML frontmatter `status:` key.
	StatusSourceFrontmatter StatusSource = "frontmatter"
	// StatusSourceNone means no status stamp was found.
	StatusSourceNone StatusSource = "none"
)

// StatusParseResult is the deterministic status parse for one document.
type StatusParseResult struct {
	// Path is the document path supplied to the parser.
	Path string
	// Status is the normalized base status when recognized; StatusUnknown
	// when missing or when the raw value does not map to a known base.
	Status DocStatus
	// Raw is the unnormalized status text (empty when no stamp).
	Raw string
	// Source indicates body, frontmatter, or none.
	Source StatusSource
	// Line is the 1-based line of the stamp (0 when none).
	Line int
	// IsDesign is true when the path names an SD/TD/ADR document.
	IsDesign bool
	// MissingDesignStatus is true when an SD/TD/ADR document has no stamp.
	MissingDesignStatus bool
}

var (
	// designDocBaseRe matches SD-NNN / TD-NNN / ADR-NNN basenames.
	designDocBaseRe = regexp.MustCompile(`(?i)^(SD|TD|ADR)-\d+`)
	// bodyStatusRe matches a body line `**Status:** <value>` (optional bold close).
	bodyStatusRe = regexp.MustCompile(`(?i)^\s*\*\*Status:\*\*\s*(.+?)\s*$`)
	// frontmatterStatusRe matches a YAML `status:` key inside frontmatter.
	frontmatterStatusRe = regexp.MustCompile(`(?i)^\s*status\s*:\s*(.+?)\s*$`)
)

// IsDesignDocument reports whether path is an SD, TD, or ADR document
// by basename prefix (e.g. SD-013-....md, TD-027-....md, ADR-028-....md).
func IsDesignDocument(path string) bool {
	base := filepath.Base(path)
	// Strip extension for matching.
	ext := filepath.Ext(base)
	name := base
	if ext != "" {
		name = strings.TrimSuffix(base, ext)
	}
	return designDocBaseRe.MatchString(name)
}

// ParseDocumentStatus reads path and returns the status parse result.
// It is read-only: the file is never modified.
func ParseDocumentStatus(path string) (*StatusParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseDocumentStatusMarkdown(path, string(data)), nil
}

// ParseDocumentStatusMarkdown parses markdown content for a status stamp.
// path is recorded on the result for diagnostics; content is not written back.
func ParseDocumentStatusMarkdown(path, content string) *StatusParseResult {
	res := &StatusParseResult{
		Path:     path,
		Status:   StatusUnknown,
		Source:   StatusSourceNone,
		IsDesign: IsDesignDocument(path),
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	// Prefer body `**Status:**` when present (author-visible stamp).
	if raw, line, ok := findBodyStatus(lines); ok {
		res.Raw = raw
		res.Line = line
		res.Source = StatusSourceBody
		res.Status = NormalizeStatus(raw)
		return res
	}

	// Fall back to YAML frontmatter status:.
	if raw, line, ok := findFrontmatterStatus(lines); ok {
		res.Raw = raw
		res.Line = line
		res.Source = StatusSourceFrontmatter
		res.Status = NormalizeStatus(raw)
		return res
	}

	if res.IsDesign {
		res.MissingDesignStatus = true
	}
	return res
}

// NormalizeStatus maps free-text status values to a base DocStatus.
// Unknown values return StatusUnknown; callers still treat Source != none
// as "stamped" for the missing-status rule.
func NormalizeStatus(raw string) DocStatus {
	s := strings.TrimSpace(raw)
	if s == "" {
		return StatusUnknown
	}
	// Strip optional surrounding quotes from YAML.
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	lower := strings.ToLower(s)

	// Longest / multi-word bases first; match as a prefix so free-text
	// qualifiers after the base status are accepted (e.g. "Proposed — ...").
	bases := []struct {
		prefix string
		status DocStatus
	}{
		{"in progress", StatusInProgress},
		{"aspirational", StatusAspirational},
		{"implemented", StatusImplemented},
		{"complete", StatusComplete},
		{"proposed", StatusProposed},
		{"deferred", StatusDeferred},
	}
	for _, b := range bases {
		if !strings.HasPrefix(lower, b.prefix) {
			continue
		}
		// Require end-of-string or a non-letter boundary after the prefix
		// so "complete" does not match "completely..." as a false friend.
		// Qualifiers after the base ("Proposed — note", "Complete (rev 6)")
		// still normalize to the base status.
		rest := lower[len(b.prefix):]
		if rest == "" {
			return b.status
		}
		r := []rune(rest)[0]
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return b.status
		}
	}
	return StatusUnknown
}

func findBodyStatus(lines []string) (raw string, line int, ok bool) {
	for i, l := range lines {
		if m := bodyStatusRe.FindStringSubmatch(l); m != nil {
			return strings.TrimSpace(m[1]), i + 1, true
		}
	}
	return "", 0, false
}

func findFrontmatterStatus(lines []string) (raw string, line int, ok bool) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", 0, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return "", 0, false
		}
		if m := frontmatterStatusRe.FindStringSubmatch(lines[i]); m != nil {
			return strings.TrimSpace(m[1]), i + 1, true
		}
	}
	return "", 0, false
}
