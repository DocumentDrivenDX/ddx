package agent

import (
	"net"
	"net/url"
	"strings"
)

const (
	BillingModePaid         = "paid"
	BillingModeSubscription = "subscription"
	BillingModeLocal        = "local"
)

// BillingModeFor classifies a session's cost basis. It is the single
// write-time source of truth for the sharded session index and legacy reindex.
func BillingModeFor(harness, surface, baseURL string) string {
	return billingModeFor(harness, surface, baseURL)
}

func billingModeFor(harness, surface, baseURL string) string {
	h := normalizeBillingPart(harness)
	s := normalizeBillingPart(surface)

	if isLocalBaseURL(baseURL) {
		return BillingModeLocal
	}

	switch h {
	case "agent", "virtual", "script", "lmstudio", "ollama", "vllm", "omlx":
		return BillingModeLocal
	case "claude", "claude-code", "codex", "gemini", "gemini-cli":
		return BillingModeSubscription
	case "openrouter", "openai", "anthropic":
		return BillingModePaid
	}

	switch s {
	case "claude", "codex", "gemini":
		return BillingModeSubscription
	case "local", "virtual", "script":
		return BillingModeLocal
	case "openai-compat", "embedded-openai", "embedded-anthropic", "anthropic":
		return BillingModePaid
	}

	// Unknown non-local work is treated as paid rather than local so cash-like
	// API paths do not disappear from the paid bucket silently.
	return BillingModePaid
}

func ValidateBillingMode(mode string) bool {
	switch mode {
	case BillingModePaid, BillingModeSubscription, BillingModeLocal:
		return true
	default:
		return false
	}
}

func normalizeBillingPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func isLocalBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		host = u.Host
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
