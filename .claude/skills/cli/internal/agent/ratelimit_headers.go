package agent

import (
	"strconv"
	"strings"
	"time"
)

// RateLimitSignal is a normalized rate-limit snapshot parsed from harness
// HTTP response headers. All fields are optional; an unset integer field is
// represented by -1 so callers can distinguish "unknown" from "zero".
type RateLimitSignal struct {
	CeilingTokens        int       // -1 when unknown
	CeilingWindowSeconds int       // -1 when unknown
	Remaining            int       // -1 when unknown
	ResetAt              time.Time // zero when unknown
}

// HasAny reports whether any field is populated.
func (s RateLimitSignal) HasAny() bool {
	if s.CeilingTokens >= 0 {
		return true
	}
	if s.CeilingWindowSeconds >= 0 {
		return true
	}
	if s.Remaining >= 0 {
		return true
	}
	if !s.ResetAt.IsZero() {
		return true
	}
	return false
}

// ParseRateLimitHeaders parses the common rate-limit headers emitted by
// subprocess harnesses that wrap provider HTTP calls. The harness name selects
// the header family. Headers not recognised by the mapping are ignored.
//
// Supported families:
//   - "claude": anthropic-ratelimit-tokens-{limit,remaining,reset} and
//     anthropic-ratelimit-requests-{limit,remaining,reset}. The tokens family
//     wins when both are present.
//   - "codex":  x-ratelimit-{limit,remaining,reset}-tokens (OpenAI/codex).
//
// Unknown harnesses return an empty signal.
func ParseRateLimitHeaders(harness string, headers map[string][]string) RateLimitSignal {
	h := normalizeHeaders(headers)
	sig := emptyRateLimitSignal()
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "claude", "anthropic":
		sig = parseClaudeRateLimit(h)
	case "codex", "openai":
		sig = parseCodexRateLimit(h)
	}
	return sig
}

func emptyRateLimitSignal() RateLimitSignal {
	return RateLimitSignal{
		CeilingTokens:        -1,
		CeilingWindowSeconds: -1,
		Remaining:            -1,
	}
}

// normalizeHeaders lowercases and flattens map[string][]string → map[string]string.
// The first value for each header wins.
func normalizeHeaders(headers map[string][]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if len(v) == 0 {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v[0])
	}
	return out
}

// parseClaudeRateLimit reads the anthropic-ratelimit-* header family.
// Anthropic publishes separate windows for tokens and requests; tokens is the
// more useful signal for "will we run out soon", so we prefer it.
func parseClaudeRateLimit(h map[string]string) RateLimitSignal {
	sig := emptyRateLimitSignal()
	if limit, ok := parseIntHeader(h, "anthropic-ratelimit-tokens-limit"); ok {
		sig.CeilingTokens = limit
	}
	if rem, ok := parseIntHeader(h, "anthropic-ratelimit-tokens-remaining"); ok {
		sig.Remaining = rem
	}
	if resetStr, ok := h["anthropic-ratelimit-tokens-reset"]; ok {
		if t, ok2 := parseResetTimestamp(resetStr); ok2 {
			sig.ResetAt = t
		}
	}
	// If tokens family not populated, fall back to requests family for resetAt/window.
	if sig.CeilingTokens < 0 && sig.Remaining < 0 {
		if limit, ok := parseIntHeader(h, "anthropic-ratelimit-requests-limit"); ok {
			sig.CeilingTokens = limit
		}
		if rem, ok := parseIntHeader(h, "anthropic-ratelimit-requests-remaining"); ok {
			sig.Remaining = rem
		}
		if resetStr, ok := h["anthropic-ratelimit-requests-reset"]; ok {
			if t, ok2 := parseResetTimestamp(resetStr); ok2 {
				sig.ResetAt = t
			}
		}
	}
	if sig.HasAny() {
		// Anthropic rate-limit windows are published as 1-minute buckets for
		// token-level quotas. Record the window so clients can render "/ min".
		sig.CeilingWindowSeconds = 60
	}
	return sig
}

// parseCodexRateLimit reads the x-ratelimit-*-tokens header family.
func parseCodexRateLimit(h map[string]string) RateLimitSignal {
	sig := emptyRateLimitSignal()
	if limit, ok := parseIntHeader(h, "x-ratelimit-limit-tokens"); ok {
		sig.CeilingTokens = limit
	}
	if rem, ok := parseIntHeader(h, "x-ratelimit-remaining-tokens"); ok {
		sig.Remaining = rem
	}
	if resetStr, ok := h["x-ratelimit-reset-tokens"]; ok {
		if t, ok2 := parseResetTimestamp(resetStr); ok2 {
			sig.ResetAt = t
		}
	}
	if sig.HasAny() {
		sig.CeilingWindowSeconds = 60
	}
	return sig
}

func parseIntHeader(h map[string]string, key string) (int, bool) {
	v, ok := h[key]
	if !ok || v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseResetTimestamp accepts either an absolute RFC3339 timestamp (Anthropic
// style) or a relative "30s" / "2m" / "1h" duration (codex style).
func parseResetTimestamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().UTC().Add(d), true
	}
	if secs, err := strconv.Atoi(s); err == nil {
		return time.Unix(int64(secs), 0).UTC(), true
	}
	return time.Time{}, false
}
