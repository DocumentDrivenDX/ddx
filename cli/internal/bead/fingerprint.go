package bead

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// PromptFingerprint returns a stable hash of the bead fields that shape the
// operator-facing prompt/readiness surface. It intentionally excludes
// lifecycle status and execution evidence so operator acceptance can be tied
// to the bead content rather than queue churn.
func PromptFingerprint(b Bead) string {
	parts := []string{
		strings.TrimSpace(b.IssueType),
		strings.TrimSpace(b.Title),
		strings.TrimSpace(b.Description),
		strings.TrimSpace(b.Acceptance),
		strings.TrimSpace(b.Notes),
		strings.TrimSpace(b.Parent),
		joinSorted(trimmedStrings(b.Labels)),
	}
	if strings.Join(parts, "") == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func trimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func joinSorted(values []string) string {
	if len(values) == 0 {
		return ""
	}
	items := append([]string(nil), values...)
	sort.Strings(items)
	return strings.Join(items, "\x00")
}
