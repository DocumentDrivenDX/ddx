package agent

import (
	"context"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ratelimitpolicy"
	agentlib "github.com/easel/fizeau"
)

// RateLimitRetryDefaultBudget is the default per-bead total wait budget for
// rate-limit retries. AC #4 of ddx-c6e3db02 names 5 minutes.
const RateLimitRetryDefaultBudget = ratelimitpolicy.DefaultBudget

// RateLimitRetryDefaultPerWaitCap caps any single retry wait so a misbehaving
// provider cannot pin the worker with a one-hour Retry-After. The bead
// description names 60 seconds as the per-wait cap.
const RateLimitRetryDefaultPerWaitCap = ratelimitpolicy.DefaultPerWaitCap

// RateLimitBudgetExhaustedReason is the canonical reason string written into
// Result.Error when the per-bead retry budget is exhausted. The execute-bead
// status mapping translates this into FailureModeAuthError so it surfaces in
// the standard execution_failed pathway (TD-031 §8.4).
const RateLimitBudgetExhaustedReason = ratelimitpolicy.BudgetExhaustedReason

// RateLimitRetryEventKind is the bead event kind appended on each retry per
// TD-031 §4 / §8.4 RateLimitRetryContract. The body carries the retry count
// and wait duration.
const RateLimitRetryEventKind = "rate-limit-retry"

type RateLimitWaitDecision = ratelimitpolicy.WaitDecision

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
	return ratelimitpolicy.EvaluateRateLimitWait(retryAfter, attempt, elapsed, budget, perWaitCap)
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
	return ratelimitpolicy.IsRateLimitText(result.Error + "\n" + result.Stderr)
}

// ParseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3:
// either a non-negative integer ("delta-seconds") or an HTTP-date.
// Returns zero duration when the value is empty or unparseable.
//
// now is supplied so tests can pin the clock; production callers pass
// time.Now().
func ParseRetryAfter(value string, now time.Time) time.Duration {
	return ratelimitpolicy.ParseRetryAfter(value, now)
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
	return ratelimitpolicy.ExtractRetryAfterFromStderr(stderr, now)
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
