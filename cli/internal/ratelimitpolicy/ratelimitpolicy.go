package ratelimitpolicy

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBudget is the default total wait budget for rate-limit retries.
const DefaultBudget = 5 * time.Minute

// DefaultPerWaitCap caps any single retry wait so a misbehaving upstream
// cannot pin the worker with an arbitrarily long Retry-After.
const DefaultPerWaitCap = 60 * time.Second

// BudgetExhaustedReason is the canonical reason string emitted when the retry
// budget is exhausted.
const BudgetExhaustedReason = "rate-limited beyond budget"

// rateLimitBackoffSchedule is the fallback wait sequence when no Retry-After value can
// be parsed from the upstream response.
var rateLimitBackoffSchedule = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// WaitDecision is the policy output for one rate-limit response.
type WaitDecision struct {
	ShouldRetry bool
	Wait        time.Duration
	Source      string
	Reason      string
}

// EvaluateRateLimitWait decides how long to wait before retrying after a
// rate-limit response, and whether the per-bead budget allows the retry.
func EvaluateRateLimitWait(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) WaitDecision {
	if budget > 0 && elapsed >= budget {
		return WaitDecision{
			ShouldRetry: false,
			Reason:      BudgetExhaustedReason,
		}
	}

	var wait time.Duration
	var source string
	if retryAfter > 0 {
		wait = retryAfter
		source = "retry-after"
	} else {
		idx := attempt - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(rateLimitBackoffSchedule) {
			idx = len(rateLimitBackoffSchedule) - 1
		}
		wait = rateLimitBackoffSchedule[idx]
		source = "exponential-backoff"
	}

	if perWaitCap > 0 && wait > perWaitCap {
		wait = perWaitCap
	}

	if budget > 0 && elapsed+wait > budget {
		remaining := budget - elapsed
		if remaining <= 0 {
			return WaitDecision{
				ShouldRetry: false,
				Reason:      BudgetExhaustedReason,
			}
		}
		wait = remaining
	}

	return WaitDecision{
		ShouldRetry: true,
		Wait:        wait,
		Source:      source,
	}
}

// IsRateLimitText reports whether the supplied text indicates a rate-limit
// event. Quota exhaustion wording stays out of scope.
func IsRateLimitText(text string) bool {
	if text == "" {
		return false
	}
	combined := strings.ToLower(text)
	if combined == "\n" {
		return false
	}
	if strings.Contains(combined, "quota exceeded") ||
		strings.Contains(combined, "insufficient quota") ||
		strings.Contains(combined, "no viable provider") {
		return false
	}
	if strings.Contains(combined, "rate limit") || strings.Contains(combined, "ratelimit") {
		return true
	}
	return hasIsolated429(combined)
}

// ParseRetryAfter parses a Retry-After value per RFC 7231 §7.1.3.
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0
		}
		return d
	}
	if d, err := time.ParseDuration(value); err == nil && d >= 0 {
		return d
	}
	return 0
}

// ExtractRetryAfterFromStderr scans stderr for a Retry-After indication.
func ExtractRetryAfterFromStderr(stderr string, now time.Time) time.Duration {
	if stderr == "" {
		return 0
	}
	lower := strings.ToLower(stderr)
	for _, marker := range []string{"retry-after:", "retry_after=", "retry-after ", "retry after "} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		rest := stderr[idx+len(marker):]
		if eol := strings.IndexAny(rest, "\r\n"); eol >= 0 {
			rest = rest[:eol]
		}
		rest = strings.TrimSpace(rest)
		rest = strings.Trim(rest, "\"',")
		if d := ParseRetryAfter(rest, now); d > 0 {
			return d
		}
	}
	return 0
}

func hasIsolated429(s string) bool {
	for i := 0; i+3 <= len(s); i++ {
		if s[i:i+3] != "429" {
			continue
		}
		if i > 0 {
			c := s[i-1]
			if c >= '0' && c <= '9' {
				continue
			}
		}
		if i+3 < len(s) {
			c := s[i+3]
			if c >= '0' && c <= '9' {
				continue
			}
		}
		return true
	}
	return false
}
