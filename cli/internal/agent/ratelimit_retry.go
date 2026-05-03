package agent

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// RateLimitRetryDefaultBudget is the default per-bead total wait budget for
// rate-limit retries. AC #4 of ddx-c6e3db02 names 5 minutes.
const RateLimitRetryDefaultBudget = 5 * time.Minute

// RateLimitRetryDefaultPerWaitCap caps any single retry wait so a misbehaving
// provider cannot pin the worker with a one-hour Retry-After. The bead
// description names 60 seconds as the per-wait cap.
const RateLimitRetryDefaultPerWaitCap = 60 * time.Second

// RateLimitBudgetExhaustedReason is the canonical reason string written into
// Result.Error when the per-bead retry budget is exhausted. The execute-bead
// status mapping translates this into FailureModeAuthError so it surfaces in
// the standard execution_failed pathway (TD-031 §8.4).
const RateLimitBudgetExhaustedReason = "rate-limited beyond budget"

// RateLimitRetryEventKind is the bead event kind appended on each retry per
// TD-031 §4 / §8.4 RateLimitRetryContract. The body carries the retry count
// and wait duration.
const RateLimitRetryEventKind = "rate-limit-retry"

// rateLimitBackoffSchedule is the exponential-backoff fallback when a 429
// response carries no parseable Retry-After value. The schedule names the
// wait for the 1st, 2nd, … retry; entries past the end repeat the last value.
// AC #3 of ddx-c6e3db02 specifies 1s, 5s, 15s, 30s, 60s.
var rateLimitBackoffSchedule = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// RateLimitWaitDecision is the policy output for one rate-limit response.
// The policy never mutates state; the caller decides whether to wait + retry
// or to give up and surface RateLimitBudgetExhaustedReason.
type RateLimitWaitDecision struct {
	// ShouldRetry is true when the budget allows another wait + retry.
	ShouldRetry bool
	// Wait is the duration the caller should sleep before retrying.
	// Only meaningful when ShouldRetry is true.
	Wait time.Duration
	// Source classifies how Wait was derived: "retry-after" when the upstream
	// response named a specific delay, "exponential-backoff" when the schedule
	// fallback was used. Empty when ShouldRetry is false.
	Source string
	// Reason is populated when ShouldRetry is false; carries
	// RateLimitBudgetExhaustedReason in the budget-exhausted case.
	Reason string
}

// EvaluateRateLimitWait decides how long to wait before retrying after a
// rate-limit response, and whether the per-bead budget allows the retry.
//
// retryAfter is the duration parsed from the response (zero when absent /
// unparseable). attempt is 1-indexed (first retry = 1). elapsed is the total
// wait already spent on this bead. budget is the total cap (zero = unbounded,
// disabling budget enforcement). perWaitCap caps any single wait (zero =
// uncapped).
//
// The function is pure: no I/O, no time.Now, no global state.
func EvaluateRateLimitWait(retryAfter time.Duration, attempt int, elapsed, budget, perWaitCap time.Duration) RateLimitWaitDecision {
	if budget > 0 && elapsed >= budget {
		return RateLimitWaitDecision{
			ShouldRetry: false,
			Reason:      RateLimitBudgetExhaustedReason,
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
		// Trim the final wait so the bead doesn't block beyond the budget.
		// If trimming leaves nothing, treat the budget as exhausted.
		remaining := budget - elapsed
		if remaining <= 0 {
			return RateLimitWaitDecision{
				ShouldRetry: false,
				Reason:      RateLimitBudgetExhaustedReason,
			}
		}
		wait = remaining
	}

	return RateLimitWaitDecision{
		ShouldRetry: true,
		Wait:        wait,
		Source:      source,
	}
}

// IsRateLimitResult reports whether a Runner Result indicates a rate-limit
// (HTTP 429) event that the per-bead retry policy should handle.
//
// Quota exhaustion (NoViableProviderForNow) is OUT of scope for
// RateLimitRetryContract per ddx-c6e3db02 AC #8 — it belongs to
// QuotaPauseContract (ddx-aede917d). The detection here matches the canonical
// 429 / "rate limit" / "ratelimit" markers without matching "quota exceeded".
func IsRateLimitResult(result *Result) bool {
	if result == nil {
		return false
	}
	combined := strings.ToLower(result.Error + "\n" + result.Stderr)
	if combined == "\n" {
		return false
	}
	// Quota wording must NOT match: that path is QuotaPauseContract
	// (ddx-aede917d), not RateLimitRetryContract.
	if strings.Contains(combined, "quota exceeded") ||
		strings.Contains(combined, "insufficient quota") ||
		strings.Contains(combined, "no viable provider") {
		return false
	}
	if strings.Contains(combined, "rate limit") ||
		strings.Contains(combined, "ratelimit") {
		return true
	}
	// Match HTTP 429 with word boundary around the digits — avoid matching
	// "port 4290" or fragment timestamps. The conservative form is to look
	// for "429" framed by non-digits or as an HTTP status marker.
	if hasIsolated429(combined) {
		return true
	}
	return false
}

// hasIsolated429 returns true when the input contains "429" framed so it
// cannot be a substring of a longer number (e.g. "4290", "12429"). A simple
// scan is enough — the haystack is short.
func hasIsolated429(s string) bool {
	for i := 0; i+3 <= len(s); i++ {
		if s[i:i+3] != "429" {
			continue
		}
		// Check left boundary.
		if i > 0 {
			c := s[i-1]
			if c >= '0' && c <= '9' {
				continue
			}
		}
		// Check right boundary.
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

// ParseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3:
// either a non-negative integer ("delta-seconds") or an HTTP-date.
// Returns zero duration when the value is empty or unparseable.
//
// now is supplied so tests can pin the clock; production callers pass
// time.Now().
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
	// HTTP-date: RFC 1123 / 850 / asctime per http.ParseTime.
	if t, err := http.ParseTime(value); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0
		}
		return d
	}
	// Last resort: a Go duration literal ("30s", "2m"). Some harnesses emit
	// this style on stderr; treat it as a courtesy.
	if d, err := time.ParseDuration(value); err == nil && d >= 0 {
		return d
	}
	return 0
}

// ExtractRetryAfterFromStderr scans subprocess stderr for a Retry-After
// indication and returns the parsed wait. Returns zero when no indication is
// present so the caller falls back to exponential backoff.
//
// Recognised forms (case-insensitive):
//   - "Retry-After: <value>" (HTTP header echoed by the harness)
//   - "retry_after=<value>" (structured key=value form some harnesses emit)
//   - "retry after <value>"
//
// now is used as the reference for HTTP-date values.
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
		// Read up to the end of the line.
		if eol := strings.IndexAny(rest, "\r\n"); eol >= 0 {
			rest = rest[:eol]
		}
		// Strip surrounding quotes/spaces and an optional comma terminator.
		rest = strings.TrimSpace(rest)
		rest = strings.Trim(rest, "\"',")
		if d := ParseRetryAfter(rest, now); d > 0 {
			return d
		}
	}
	return 0
}

// RateLimitRetryConfig configures the retry wrapper. Zero values mean
// "use the package default".
type RateLimitRetryConfig struct {
	// Budget caps the total wait this bead may spend on rate-limit retries.
	// Zero uses RateLimitRetryDefaultBudget. Negative disables retries entirely.
	Budget time.Duration
	// PerWaitCap caps a single retry wait. Zero uses
	// RateLimitRetryDefaultPerWaitCap.
	PerWaitCap time.Duration
	// Sleep is the function the wrapper calls between attempts. Defaults to
	// a context-aware time.Sleep. Tests inject a fake to avoid real waits.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now returns the current time for HTTP-date arithmetic. Defaults to
	// time.Now.
	Now func() time.Time
	// OnRetry, when non-nil, is invoked after each wait + before the retry
	// attempt fires. Used by integration to emit the rate-limit-retry bead
	// event and the routing-engine RecordRouteAttempt feedback.
	OnRetry func(ctx context.Context, info RateLimitRetryInfo)
}

// RateLimitRetryInfo describes one rate-limit retry event for telemetry hooks.
type RateLimitRetryInfo struct {
	Attempt    int
	Wait       time.Duration
	Source     string
	Result     *Result
	Elapsed    time.Duration
	OverBudget bool
}

// resolved returns a copy of cfg with zero fields filled in from defaults.
func (cfg RateLimitRetryConfig) resolved() RateLimitRetryConfig {
	out := cfg
	if out.Budget == 0 {
		out.Budget = RateLimitRetryDefaultBudget
	}
	if out.PerWaitCap == 0 {
		out.PerWaitCap = RateLimitRetryDefaultPerWaitCap
	}
	if out.Sleep == nil {
		out.Sleep = ctxSleep
	}
	if out.Now == nil {
		out.Now = time.Now
	}
	return out
}

func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RunWithRateLimitRetry invokes attempt and, when its result is a rate-limit
// signal per IsRateLimitResult, waits per EvaluateRateLimitWait and retries
// until either attempt returns a non-rate-limit result or the per-bead budget
// is exhausted. Budget exhaustion mutates the final result's Error to
// RateLimitBudgetExhaustedReason and returns it without further retries.
//
// The wrapper does NOT change provider availability state — the provider
// stays in rotation per AC #2 of ddx-c6e3db02. Per AC #6, callers wire
// RecordRouteAttempt via OnRetry for transparency.
func RunWithRateLimitRetry(ctx context.Context, cfg RateLimitRetryConfig, attempt func(ctx context.Context) (*Result, error)) (*Result, error) {
	if cfg.Budget < 0 {
		// Negative budget disables the wrapper entirely (parity with
		// "feature off"). Useful for tests and operator opt-out.
		return attempt(ctx)
	}
	cfg = cfg.resolved()

	var (
		elapsed    time.Duration
		retryCount int
	)
	for {
		res, err := attempt(ctx)
		if err != nil || res == nil {
			return res, err
		}
		if !IsRateLimitResult(res) {
			return res, nil
		}

		retryCount++
		retryAfter := ExtractRetryAfterFromStderr(res.Stderr, cfg.Now())
		decision := EvaluateRateLimitWait(retryAfter, retryCount, elapsed, cfg.Budget, cfg.PerWaitCap)
		if !decision.ShouldRetry {
			// Budget exhausted: surface the canonical reason so the standard
			// execution_failed mapping fires (TD-031 §5).
			res.Error = RateLimitBudgetExhaustedReason
			if cfg.OnRetry != nil {
				cfg.OnRetry(ctx, RateLimitRetryInfo{
					Attempt:    retryCount,
					Source:     decision.Source,
					Result:     res,
					Elapsed:    elapsed,
					OverBudget: true,
				})
			}
			return res, nil
		}

		if cfg.OnRetry != nil {
			cfg.OnRetry(ctx, RateLimitRetryInfo{
				Attempt: retryCount,
				Wait:    decision.Wait,
				Source:  decision.Source,
				Result:  res,
				Elapsed: elapsed,
			})
		}

		if err := cfg.Sleep(ctx, decision.Wait); err != nil {
			return res, err
		}
		elapsed += decision.Wait
	}
}

// BuildRateLimitRouteAttempt constructs a fizeau.RouteAttempt that records a
// rate-limit retry on the routing-engine feedback channel. The Status is
// "rate_limited" so the engine has signal even though provider availability
// remains unchanged per RateLimitRetryContract.
func BuildRateLimitRouteAttempt(info RateLimitRetryInfo) agentlib.RouteAttempt {
	att := agentlib.RouteAttempt{
		Status:    "rate_limited",
		Reason:    info.Source,
		Duration:  info.Wait,
		Timestamp: time.Now().UTC(),
	}
	if info.Result != nil {
		att.Harness = info.Result.Harness
		att.Provider = info.Result.Provider
		att.Model = info.Result.Model
		att.Error = info.Result.Error
	}
	if info.OverBudget {
		att.Reason = RateLimitBudgetExhaustedReason
	}
	return att
}
