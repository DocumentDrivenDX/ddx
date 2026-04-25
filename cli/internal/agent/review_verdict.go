package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ReviewVerdictSchemaVersion is the current schema version for the
// machine-readable reviewer contract emitted as the model's final response.
const ReviewVerdictSchemaVersion = 1

// Finding is one item in ReviewVerdict.Findings — a single review comment
// with severity, free-text summary, and an optional location pointer
// ("path:line"). Severity must be one of "info", "warn", "block".
type Finding struct {
	Severity string `json:"severity,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Location string `json:"location,omitempty"`
}

// ReviewVerdict is the parsed JSON contract emitted by the reviewer agent.
// It replaces the markdown extractor that silently mis-parsed `### Verdict:
// APPROVE` outputs whenever the model echoed the prompt's options-header
// line (the upstream-report regression). The parser is strict: unknown
// verdict values, missing verdict field, and truncated/unfenced JSON all
// surface as ErrReviewVerdictUnparseable.
type ReviewVerdict struct {
	SchemaVersion int       `json:"schema_version,omitempty"`
	Verdict       Verdict   `json:"verdict"`
	Summary       string    `json:"summary,omitempty"`
	Findings      []Finding `json:"findings,omitempty"`
}

// ErrReviewVerdictUnparseable is the sentinel returned by ParseReviewVerdict
// when the reviewer output cannot be decoded into a valid ReviewVerdict.
// The execute-loop classifies it as evidence.OutcomeReviewUnparseable
// (retryable review-error) rather than mis-recording a BLOCK verdict.
var ErrReviewVerdictUnparseable = errors.New("reviewer output: unparseable JSON verdict")

// ParseReviewVerdict decodes a reviewer agent's raw output into a
// ReviewVerdict. It accepts:
//   - raw JSON object;
//   - a single ```json …``` fenced block;
//   - mixed prose and fenced blocks where the LAST ```json block is the
//     verdict.
//
// It rejects:
//   - empty input;
//   - inputs containing no JSON object at all;
//   - JSON missing the "verdict" field;
//   - unknown verdict values (anything other than APPROVE,
//     REQUEST_CHANGES, BLOCK);
//   - truncated/malformed JSON.
//
// Unknown fields outside the schema are tolerated (forward-compat).
func ParseReviewVerdict(raw []byte) (ReviewVerdict, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return ReviewVerdict{}, fmt.Errorf("%w: empty input", ErrReviewVerdictUnparseable)
	}
	candidate, ok := extractJSONCandidate(string(raw))
	if !ok {
		return ReviewVerdict{}, fmt.Errorf("%w: no JSON object found", ErrReviewVerdictUnparseable)
	}

	// First decode into a generic shape so we can validate the verdict
	// field's presence and value before binding to ReviewVerdict.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(candidate), &probe); err != nil {
		return ReviewVerdict{}, fmt.Errorf("%w: %v", ErrReviewVerdictUnparseable, err)
	}
	rawVerdict, ok := probe["verdict"]
	if !ok {
		return ReviewVerdict{}, fmt.Errorf("%w: missing verdict field", ErrReviewVerdictUnparseable)
	}
	var verdictStr string
	if err := json.Unmarshal(rawVerdict, &verdictStr); err != nil {
		return ReviewVerdict{}, fmt.Errorf("%w: verdict not a string: %v", ErrReviewVerdictUnparseable, err)
	}
	switch Verdict(verdictStr) {
	case VerdictApprove, VerdictRequestChanges, VerdictBlock:
	default:
		return ReviewVerdict{}, fmt.Errorf("%w: unknown verdict %q", ErrReviewVerdictUnparseable, verdictStr)
	}

	var rv ReviewVerdict
	if err := json.Unmarshal([]byte(candidate), &rv); err != nil {
		return ReviewVerdict{}, fmt.Errorf("%w: %v", ErrReviewVerdictUnparseable, err)
	}
	return rv, nil
}

// extractJSONCandidate returns the JSON object payload from a raw reviewer
// output. Resolution order:
//  1. The last ```json … ``` fenced block (handles models that wrap output).
//  2. The last ``` … ``` fenced block when its body parses as a JSON object.
//  3. The substring from the first '{' that balances braces (handles raw
//     JSON or JSON with leading prose).
//
// Returns ok=false when no plausible JSON object is found.
func extractJSONCandidate(s string) (string, bool) {
	if c, ok := lastFencedBlock(s, "json"); ok {
		return strings.TrimSpace(c), true
	}
	if c, ok := lastFencedBlock(s, ""); ok {
		trimmed := strings.TrimSpace(c)
		if strings.HasPrefix(trimmed, "{") {
			return trimmed, true
		}
	}
	if c, ok := firstBalancedObject(s); ok {
		return c, true
	}
	return "", false
}

// lastFencedBlock returns the body of the LAST ```<lang> … ``` fenced block
// in s. lang="" matches any fence ("``` … ```"). Returns ok=false when no
// matching fence is found or the fence is unterminated.
func lastFencedBlock(s, lang string) (string, bool) {
	open := "```"
	if lang != "" {
		open = "```" + lang
	}
	var found string
	var ok bool
	idx := 0
	for {
		i := indexFromAny(s, idx, open)
		if i < 0 {
			break
		}
		// Ensure the fence is followed by newline or whitespace (avoid
		// matching ```jsonc when looking for ```json).
		afterOpen := i + len(open)
		if afterOpen < len(s) {
			c := s[afterOpen]
			if lang != "" && c != '\n' && c != '\r' && c != ' ' && c != '\t' {
				idx = i + len(open)
				continue
			}
		}
		bodyStart := afterOpen
		// Skip a single trailing newline after the opening fence.
		if bodyStart < len(s) && s[bodyStart] == '\n' {
			bodyStart++
		} else if bodyStart+1 < len(s) && s[bodyStart] == '\r' && s[bodyStart+1] == '\n' {
			bodyStart += 2
		}
		// Find closing fence.
		j := strings.Index(s[bodyStart:], "```")
		if j < 0 {
			break
		}
		body := s[bodyStart : bodyStart+j]
		// Strip a single trailing newline before the closing fence.
		body = strings.TrimRight(body, "\n\r")
		found = body
		ok = true
		idx = bodyStart + j + 3
	}
	return found, ok
}

// indexFromAny is strings.Index but starting at the given offset.
func indexFromAny(s string, from int, sub string) int {
	if from >= len(s) {
		return -1
	}
	i := strings.Index(s[from:], sub)
	if i < 0 {
		return -1
	}
	return from + i
}

// firstBalancedObject scans for the first '{' in s and returns the substring
// that closes its outermost JSON object, respecting strings and escapes.
// Returns ok=false on no opening brace or on an unterminated object.
func firstBalancedObject(s string) (string, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
