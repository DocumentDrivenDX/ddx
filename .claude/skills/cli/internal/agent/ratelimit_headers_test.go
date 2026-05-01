package agent

import (
	"testing"
	"time"
)

func TestParseRateLimitHeaders_ClaudeTokensFixture(t *testing.T) {
	// Captured from a real Anthropic 200 response body; only headers retained.
	headers := map[string][]string{
		"Anthropic-Ratelimit-Tokens-Limit":       {"80000"},
		"Anthropic-Ratelimit-Tokens-Remaining":   {"79200"},
		"Anthropic-Ratelimit-Tokens-Reset":       {"2026-04-23T05:00:00Z"},
		"Anthropic-Ratelimit-Requests-Limit":     {"50"},
		"Anthropic-Ratelimit-Requests-Remaining": {"49"},
	}
	sig := ParseRateLimitHeaders("claude", headers)
	if sig.CeilingTokens != 80000 {
		t.Fatalf("ceiling tokens: got %d want 80000", sig.CeilingTokens)
	}
	if sig.Remaining != 79200 {
		t.Fatalf("remaining: got %d want 79200", sig.Remaining)
	}
	if sig.CeilingWindowSeconds != 60 {
		t.Fatalf("window seconds: got %d want 60", sig.CeilingWindowSeconds)
	}
	if sig.ResetAt.IsZero() {
		t.Fatalf("reset at should be populated")
	}
	if got, want := sig.ResetAt.Format(time.RFC3339), "2026-04-23T05:00:00Z"; got != want {
		t.Fatalf("reset at: got %q want %q", got, want)
	}
}

func TestParseRateLimitHeaders_ClaudeFallsBackToRequestsFamily(t *testing.T) {
	headers := map[string][]string{
		"Anthropic-Ratelimit-Requests-Limit":     {"50"},
		"Anthropic-Ratelimit-Requests-Remaining": {"10"},
		"Anthropic-Ratelimit-Requests-Reset":     {"2026-04-23T05:00:00Z"},
	}
	sig := ParseRateLimitHeaders("claude", headers)
	if sig.CeilingTokens != 50 {
		t.Fatalf("ceiling (from requests family): got %d want 50", sig.CeilingTokens)
	}
	if sig.Remaining != 10 {
		t.Fatalf("remaining: got %d want 10", sig.Remaining)
	}
}

func TestParseRateLimitHeaders_CodexFixture(t *testing.T) {
	// Captured from an OpenAI codex response.
	headers := map[string][]string{
		"X-Ratelimit-Limit-Tokens":     {"200000"},
		"X-Ratelimit-Remaining-Tokens": {"194312"},
		"X-Ratelimit-Reset-Tokens":     {"30s"},
	}
	sig := ParseRateLimitHeaders("codex", headers)
	if sig.CeilingTokens != 200000 {
		t.Fatalf("ceiling: got %d want 200000", sig.CeilingTokens)
	}
	if sig.Remaining != 194312 {
		t.Fatalf("remaining: got %d want 194312", sig.Remaining)
	}
	if sig.CeilingWindowSeconds != 60 {
		t.Fatalf("window seconds: got %d want 60", sig.CeilingWindowSeconds)
	}
	if sig.ResetAt.IsZero() {
		t.Fatalf("expected relative reset to populate future timestamp")
	}
	if time.Until(sig.ResetAt) > 45*time.Second {
		t.Fatalf("reset-at is more than 45s in future, got %v", sig.ResetAt)
	}
}

func TestParseRateLimitHeaders_UnknownHarness(t *testing.T) {
	sig := ParseRateLimitHeaders("gemini", map[string][]string{
		"X-Ratelimit-Limit-Tokens": {"100"},
	})
	if sig.HasAny() {
		t.Fatalf("unknown harness should return empty signal, got %+v", sig)
	}
}

func TestParseRateLimitHeaders_MissingHeaders(t *testing.T) {
	sig := ParseRateLimitHeaders("claude", map[string][]string{})
	if sig.HasAny() {
		t.Fatalf("empty headers should return empty signal, got %+v", sig)
	}
}

func TestParseResetTimestamp_UnixSeconds(t *testing.T) {
	t0, ok := parseResetTimestamp("1800000000")
	if !ok {
		t.Fatal("expected unix seconds to parse")
	}
	if t0.Unix() != 1800000000 {
		t.Fatalf("unix: got %d", t0.Unix())
	}
}

func TestParseResetTimestamp_Invalid(t *testing.T) {
	_, ok := parseResetTimestamp("nonsense")
	if ok {
		t.Fatal("expected invalid to fail")
	}
}
