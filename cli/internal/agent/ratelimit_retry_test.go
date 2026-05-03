package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestEvaluateRateLimitWait_RetryAfterRespected covers AC #3 of ddx-c6e3db02:
// when the response carries a Retry-After value, the policy honours it
// (subject to the per-wait cap and remaining budget).
func TestEvaluateRateLimitWait_RetryAfterRespected(t *testing.T) {
	d := EvaluateRateLimitWait(20*time.Second, 1, 0, 5*time.Minute, 60*time.Second)
	if !d.ShouldRetry {
		t.Fatalf("ShouldRetry: got false; want true")
	}
	if d.Wait != 20*time.Second {
		t.Fatalf("Wait: got %v; want 20s", d.Wait)
	}
	if d.Source != "retry-after" {
		t.Fatalf("Source: got %q; want retry-after", d.Source)
	}
}

// Per-wait cap clamps an over-large Retry-After value.
func TestEvaluateRateLimitWait_RetryAfterClampedByPerWaitCap(t *testing.T) {
	d := EvaluateRateLimitWait(10*time.Minute, 1, 0, 5*time.Minute, 60*time.Second)
	if !d.ShouldRetry {
		t.Fatalf("ShouldRetry: got false; want true")
	}
	if d.Wait != 60*time.Second {
		t.Fatalf("Wait: got %v; want clamped 60s", d.Wait)
	}
}

// TestEvaluateRateLimitWait_ExponentialBackoffFallback covers AC #3: when
// Retry-After is absent the policy falls back to the published schedule
// (1s, 5s, 15s, 30s, 60s).
func TestEvaluateRateLimitWait_ExponentialBackoffFallback(t *testing.T) {
	want := []time.Duration{
		1 * time.Second,
		5 * time.Second,
		15 * time.Second,
		30 * time.Second,
		60 * time.Second,
		// Beyond the schedule, repeat the last entry.
		60 * time.Second,
	}
	for i, expected := range want {
		d := EvaluateRateLimitWait(0, i+1, 0, 0, 0)
		if !d.ShouldRetry {
			t.Fatalf("attempt %d: ShouldRetry false", i+1)
		}
		if d.Source != "exponential-backoff" {
			t.Fatalf("attempt %d: Source = %q; want exponential-backoff", i+1, d.Source)
		}
		if d.Wait != expected {
			t.Fatalf("attempt %d: Wait = %v; want %v", i+1, d.Wait, expected)
		}
	}
}

// Budget exhaustion: when elapsed already meets/exceeds budget, no retry.
func TestEvaluateRateLimitWait_BudgetExhausted(t *testing.T) {
	d := EvaluateRateLimitWait(5*time.Second, 3, 5*time.Minute, 5*time.Minute, 60*time.Second)
	if d.ShouldRetry {
		t.Fatalf("ShouldRetry: got true; want false (budget exhausted)")
	}
	if d.Reason != RateLimitBudgetExhaustedReason {
		t.Fatalf("Reason: got %q; want %q", d.Reason, RateLimitBudgetExhaustedReason)
	}
}

// Final wait is trimmed so it doesn't push elapsed past the budget.
func TestEvaluateRateLimitWait_TrimsFinalWaitToBudget(t *testing.T) {
	d := EvaluateRateLimitWait(60*time.Second, 5, 4*time.Minute+30*time.Second, 5*time.Minute, 60*time.Second)
	if !d.ShouldRetry {
		t.Fatalf("ShouldRetry: got false; want true")
	}
	if d.Wait != 30*time.Second {
		t.Fatalf("Wait: got %v; want trimmed 30s", d.Wait)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30", 30 * time.Second},
		{"  60 ", 60 * time.Second},
		{"-5", 0}, // negative seconds → zero
		{"", 0},
		{"15s", 15 * time.Second},
		{"2m", 2 * time.Minute},
	}
	for _, c := range cases {
		got := ParseRetryAfter(c.in, now)
		if got != c.want {
			t.Errorf("ParseRetryAfter(%q): got %v; want %v", c.in, got, c.want)
		}
	}
	// HTTP-date 90s in the future (RFC1123 with GMT, the canonical HTTP form)
	httpDate := now.Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if got := ParseRetryAfter(httpDate, now); got != 90*time.Second {
		t.Errorf("ParseRetryAfter(http-date %q): got %v; want 90s", httpDate, got)
	}
	// HTTP-date in the past → zero
	pastDate := now.Add(-1 * time.Hour).UTC().Format(http.TimeFormat)
	if got := ParseRetryAfter(pastDate, now); got != 0 {
		t.Errorf("ParseRetryAfter(past http-date): got %v; want 0", got)
	}
}

func TestExtractRetryAfterFromStderr(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	stderr := "Error: 429 Too Many Requests\nRetry-After: 45\nplease retry\n"
	if got := ExtractRetryAfterFromStderr(stderr, now); got != 45*time.Second {
		t.Errorf("got %v; want 45s", got)
	}
	if got := ExtractRetryAfterFromStderr("retry_after=12\n", now); got != 12*time.Second {
		t.Errorf("retry_after=12: got %v; want 12s", got)
	}
	if got := ExtractRetryAfterFromStderr("nothing here", now); got != 0 {
		t.Errorf("missing marker: got %v; want 0", got)
	}
}

func TestIsRateLimitResult(t *testing.T) {
	cases := []struct {
		name string
		res  *Result
		want bool
	}{
		{"nil", nil, false},
		{"empty", &Result{}, false},
		{
			name: "early-cancel 429 in stderr",
			res:  &Result{Error: "cancelled: auth/rate-limit detected (\\b429\\b)", Stderr: "HTTP/1.1 429 Too Many Requests"},
			want: true,
		},
		{
			name: "rate limit phrase in stderr",
			res:  &Result{Stderr: "Rate limit exceeded; back off"},
			want: true,
		},
		{
			name: "ratelimit phrase in error",
			res:  &Result{Error: "ratelimit hit"},
			want: true,
		},
		{
			name: "quota wording must NOT match (QuotaPauseContract owns it)",
			res:  &Result{Error: "quota exceeded"},
			want: false,
		},
		{
			name: "no viable provider must NOT match",
			res:  &Result{Error: "no viable provider for now"},
			want: false,
		},
		{
			name: "isolated 429 yes",
			res:  &Result{Error: "remote returned 429"},
			want: true,
		},
		{
			name: "embedded 4290 must not match",
			res:  &Result{Error: "received port 4290 from upstream"},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsRateLimitResult(c.res)
			if got != c.want {
				t.Errorf("IsRateLimitResult: got %v; want %v", got, c.want)
			}
		})
	}
}

// TestRunWithRateLimitRetry_RetryAfterRespected covers AC #1, #3, #6, #7:
// a rate-limited result is retried with the exact Retry-After-derived wait,
// the OnRetry hook fires (this is the channel callers use to invoke
// RecordRouteAttempt), and a subsequent non-rate-limit result terminates the
// loop.
func TestRunWithRateLimitRetry_RetryAfterRespected(t *testing.T) {
	var sleepCalls []time.Duration
	var hookCalls []RateLimitRetryInfo

	cfg := RateLimitRetryConfig{
		Budget:     5 * time.Minute,
		PerWaitCap: 60 * time.Second,
		Sleep: func(_ context.Context, d time.Duration) error {
			sleepCalls = append(sleepCalls, d)
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) },
		OnRetry: func(_ context.Context, info RateLimitRetryInfo) {
			hookCalls = append(hookCalls, info)
		},
	}

	calls := 0
	attempt := func(ctx context.Context) (*Result, error) {
		calls++
		if calls == 1 {
			return &Result{
				Error:  "cancelled: auth/rate-limit detected",
				Stderr: "HTTP 429\nRetry-After: 7\n",
			}, nil
		}
		return &Result{Output: "ok"}, nil
	}

	res, err := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Output != "ok" {
		t.Fatalf("final result not propagated; got %+v", res)
	}
	if calls != 2 {
		t.Fatalf("attempt invoked %d times; want 2", calls)
	}
	if len(sleepCalls) != 1 || sleepCalls[0] != 7*time.Second {
		t.Fatalf("sleeps = %v; want [7s]", sleepCalls)
	}
	if len(hookCalls) != 1 {
		t.Fatalf("hookCalls = %d; want 1", len(hookCalls))
	}
	if hookCalls[0].Source != "retry-after" {
		t.Errorf("hook Source = %q; want retry-after", hookCalls[0].Source)
	}
	if hookCalls[0].Wait != 7*time.Second {
		t.Errorf("hook Wait = %v; want 7s", hookCalls[0].Wait)
	}
}

// TestRunWithRateLimitRetry_ExponentialBackoffWhenRetryAfterAbsent covers
// AC #3 (exponential backoff fallback) end-to-end.
func TestRunWithRateLimitRetry_ExponentialBackoffWhenRetryAfterAbsent(t *testing.T) {
	var sleepCalls []time.Duration
	cfg := RateLimitRetryConfig{
		Budget:     5 * time.Minute,
		PerWaitCap: 60 * time.Second,
		Sleep: func(_ context.Context, d time.Duration) error {
			sleepCalls = append(sleepCalls, d)
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) },
	}

	calls := 0
	attempt := func(ctx context.Context) (*Result, error) {
		calls++
		if calls < 3 {
			// Two consecutive rate-limit responses, no Retry-After.
			return &Result{Stderr: "Rate limit exceeded; please slow down"}, nil
		}
		return &Result{Output: "done"}, nil
	}

	res, err := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Output != "done" {
		t.Fatalf("final result not propagated; got %+v", res)
	}
	if calls != 3 {
		t.Fatalf("attempt invoked %d times; want 3", calls)
	}
	want := []time.Duration{1 * time.Second, 5 * time.Second}
	if len(sleepCalls) != len(want) {
		t.Fatalf("sleeps = %v; want %v", sleepCalls, want)
	}
	for i := range want {
		if sleepCalls[i] != want[i] {
			t.Errorf("sleep[%d] = %v; want %v", i, sleepCalls[i], want[i])
		}
	}
}

// TestRunWithRateLimitRetry_BudgetExhausted covers AC #5: past budget, the
// final result carries the canonical RateLimitBudgetExhaustedReason in
// Error so the standard execution_failed status mapping fires (TD-031 §5).
// Per AC #2, no provider state is mutated by the wrapper itself.
func TestRunWithRateLimitRetry_BudgetExhausted(t *testing.T) {
	var hookCalls []RateLimitRetryInfo
	cfg := RateLimitRetryConfig{
		// Tiny budget so two rate-limited responses exhaust it.
		Budget:     2 * time.Second,
		PerWaitCap: 1 * time.Second,
		Sleep:      func(_ context.Context, _ time.Duration) error { return nil },
		Now:        func() time.Time { return time.Now() },
		OnRetry: func(_ context.Context, info RateLimitRetryInfo) {
			hookCalls = append(hookCalls, info)
		},
	}

	attempt := func(ctx context.Context) (*Result, error) {
		// Always rate-limited; no Retry-After.
		return &Result{Stderr: "rate limit exceeded"}, nil
	}

	res, err := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(res.Error, RateLimitBudgetExhaustedReason) {
		t.Fatalf("final Error = %q; want to contain %q", res.Error, RateLimitBudgetExhaustedReason)
	}
	if len(hookCalls) == 0 {
		t.Fatal("OnRetry never invoked")
	}
	// Last hook call must mark OverBudget so RecordRouteAttempt can record
	// the budget-exhausted terminal event.
	last := hookCalls[len(hookCalls)-1]
	if !last.OverBudget {
		t.Fatalf("last hook OverBudget = false; want true")
	}
}

// TestRunWithRateLimitRetry_NonRateLimitErrorPassesThrough covers the
// negative case: a result that is not a rate-limit signal must NOT be
// retried. This guards against accidentally swallowing other failures.
func TestRunWithRateLimitRetry_NonRateLimitErrorPassesThrough(t *testing.T) {
	cfg := RateLimitRetryConfig{
		Sleep: func(_ context.Context, _ time.Duration) error {
			t.Fatal("Sleep called for non-rate-limit result")
			return nil
		},
	}
	calls := 0
	attempt := func(ctx context.Context) (*Result, error) {
		calls++
		return &Result{Error: "compile failed", Stderr: "syntax error"}, nil
	}
	res, err := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("attempt invoked %d times; want 1", calls)
	}
	if res.Error != "compile failed" {
		t.Fatalf("Error mutated: got %q", res.Error)
	}
}

// Negative budget disables the wrapper entirely (operator opt-out).
func TestRunWithRateLimitRetry_NegativeBudgetDisables(t *testing.T) {
	cfg := RateLimitRetryConfig{Budget: -1}
	calls := 0
	attempt := func(ctx context.Context) (*Result, error) {
		calls++
		return &Result{Stderr: "rate limit exceeded"}, nil
	}
	res, _ := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if calls != 1 {
		t.Fatalf("attempt invoked %d times; want 1 (wrapper disabled)", calls)
	}
	if res.Error == RateLimitBudgetExhaustedReason {
		t.Fatal("disabled wrapper must not rewrite Error")
	}
}

// Context cancellation propagates from Sleep, terminating the retry loop.
func TestRunWithRateLimitRetry_ContextCancel(t *testing.T) {
	cfg := RateLimitRetryConfig{
		Budget: 5 * time.Minute,
		Sleep: func(ctx context.Context, _ time.Duration) error {
			return errors.New("ctx cancelled")
		},
	}
	attempt := func(ctx context.Context) (*Result, error) {
		return &Result{Stderr: "rate limit exceeded"}, nil
	}
	_, err := RunWithRateLimitRetry(context.Background(), cfg, attempt)
	if err == nil {
		t.Fatal("expected error from cancelled sleep")
	}
}

// TestBuildRateLimitRouteAttempt_ProvidesTransparency covers AC #6: the
// helper that callers feed into svc.RecordRouteAttempt fills in Status,
// Reason, Harness/Provider/Model so the routing engine has signal even
// though provider availability is intentionally unchanged.
func TestBuildRateLimitRouteAttempt_ProvidesTransparency(t *testing.T) {
	res := &Result{
		Harness:  "claude",
		Provider: "anthropic",
		Model:    "claude-opus-4-7",
		Error:    "cancelled: auth/rate-limit detected",
	}
	att := BuildRateLimitRouteAttempt(RateLimitRetryInfo{
		Attempt: 2,
		Wait:    7 * time.Second,
		Source:  "retry-after",
		Result:  res,
	})
	if att.Status != "rate_limited" {
		t.Errorf("Status = %q; want rate_limited", att.Status)
	}
	if att.Reason != "retry-after" {
		t.Errorf("Reason = %q; want retry-after", att.Reason)
	}
	if att.Harness != "claude" || att.Provider != "anthropic" || att.Model != "claude-opus-4-7" {
		t.Errorf("provenance not propagated: %+v", att)
	}
	if att.Duration != 7*time.Second {
		t.Errorf("Duration = %v; want 7s", att.Duration)
	}
}

func TestBuildRateLimitRouteAttempt_OverBudgetReason(t *testing.T) {
	att := BuildRateLimitRouteAttempt(RateLimitRetryInfo{
		Source:     "retry-after",
		Result:     &Result{Harness: "claude"},
		OverBudget: true,
	})
	if att.Reason != RateLimitBudgetExhaustedReason {
		t.Errorf("Reason = %q; want %q", att.Reason, RateLimitBudgetExhaustedReason)
	}
}
