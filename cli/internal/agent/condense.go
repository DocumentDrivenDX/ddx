package agent

import (
	"strings"
)

// CondenseOutput filters raw agent output to keep only progress-relevant lines.
//
// Keeps:
//   - Lines starting with namespacePrefix (e.g. "helix:") — caller progress
//   - Tool call lines starting with "$ "
//   - First line following a tool call ("$ cmd") — the result
//   - Error/warning/fail/panic lines
//   - Lines containing issue IDs, commit SHAs, or status keywords
//   - ALLCAPS label lines (e.g. "PHASE 1:", "STATUS:")
//   - Markdown headings (#), table rows (|), bold markers (**)
//   - Phase/step markers (Phase, Step, ---)
//
// Drops:
//   - Raw diff hunks (diff --, @@ headers and +/-/context lines)
//   - Codex boilerplate ("Commands run:", "tokens used" footer)
//   - Consecutive blank lines (at most one emitted between kept sections)
//   - All other verbose output
//
// Full raw output should be preserved separately before condensing.
// namespacePrefix is the caller-specific prefix (e.g. "helix:"). Pass empty
// string to disable namespace-prefix matching.
func CondenseOutput(input, namespacePrefix string) string {
	var kept []string

	skippingTokens := false
	skippingDiff := false
	blankRun := 0
	lastWasKept := false
	keepNextResult := false

	lines := strings.Split(input, "\n")
	// Split adds a trailing empty element when input ends with \n; drop it.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for _, line := range lines {
		// Always skip codex boilerplate.
		if strings.HasPrefix(line, "Commands run:") {
			continue
		}
		if line == "tokens used" {
			skippingTokens = true
			continue
		}
		if skippingTokens {
			skippingTokens = false
			continue
		}

		// Detect diff header lines; enter diff-skip mode.
		if isDiffHeader(line) {
			skippingDiff = true
			continue
		}
		if skippingDiff {
			if isDiffHeader(line) {
				continue
			}
			// Skip diff content lines (+, -, or context lines starting with space).
			if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
				continue
			}
			// Any other line ends the diff block.
			skippingDiff = false
		}

		// Blank line accounting — emit at most one separator between kept sections.
		if line == "" {
			blankRun++
			continue
		}

		keep := false

		if keepNextResult {
			keep = true
			keepNextResult = false
		}

		// Namespace-prefixed progress lines.
		if namespacePrefix != "" && strings.HasPrefix(line, namespacePrefix) {
			keep = true
		}

		// Tool call lines ("$ cmd"); keep the following result line too.
		if strings.HasPrefix(line, "$ ") {
			keep = true
			keepNextResult = true
		}

		// Error / warning / failure keywords (case-insensitive variants).
		if containsKeyword(line, []string{
			"error", "Error", "ERROR",
			"warning", "Warning", "WARN",
			"FAIL", "fail", "panic",
		}) {
			keep = true
		}

		// Issue IDs, commit SHAs, and status keywords.
		if containsKeyword(line, []string{
			"hx-", "helix-", "FEAT-", "US-",
			"COMPLETE", "BLOCKED", "CLOSED", "closed",
			"commit ",
		}) {
			keep = true
		}

		// ALLCAPS-label lines: start with A-Z/0-9/_ and contain a colon.
		// Matches patterns like "PHASE 1:", "STATUS:", "Selected Issue:".
		if len(line) > 0 && isAlphaNumUnderscore(rune(line[0])) && strings.Contains(line, ":") {
			keep = true
		}

		// Markdown structure: headings (#), table rows (|, " |"), bold (**).
		if strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "|") ||
			strings.HasPrefix(line, " |") ||
			strings.HasPrefix(line, "**") {
			keep = true
		}

		// Phase / step markers.
		if strings.HasPrefix(line, "Phase") ||
			strings.HasPrefix(line, "Step") ||
			strings.HasPrefix(line, "---") {
			keep = true
		}

		if keep {
			if lastWasKept && blankRun > 0 {
				kept = append(kept, "")
			}
			blankRun = 0
			lastWasKept = true
			kept = append(kept, line)
		}
	}

	if len(kept) == 0 {
		return ""
	}

	result := strings.Join(kept, "\n")
	return trimBlankLines(result)
}

// isDiffHeader returns true for git diff header lines.
func isDiffHeader(line string) bool {
	return strings.HasPrefix(line, "diff --git ") ||
		strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "--- a/") ||
		strings.HasPrefix(line, "+++ b/") ||
		strings.HasPrefix(line, "@@ ")
}

// containsKeyword reports whether s contains any of the keywords.
func containsKeyword(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// isAlphaNumUnderscore mirrors the bash glob [A-Z0-9_] character class.
func isAlphaNumUnderscore(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// trimBlankLines strips leading and trailing blank lines from s.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}
